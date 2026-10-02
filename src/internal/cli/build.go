package cli

import (
	"errors"
	"fmt"
	"io"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/spf13/cobra"

	"github.com/Desvelao/cubby/internal/export"
	"github.com/Desvelao/cubby/internal/export/assembly"
	"github.com/Desvelao/cubby/internal/export/console"
	"github.com/Desvelao/cubby/internal/export/csv"
	"github.com/Desvelao/cubby/internal/export/dxf"
	"github.com/Desvelao/cubby/internal/export/isometric"
	"github.com/Desvelao/cubby/internal/export/step"
	"github.com/Desvelao/cubby/internal/export/stl"
	"github.com/Desvelao/cubby/internal/export/svg"
	"github.com/Desvelao/cubby/internal/geometry"
	"github.com/Desvelao/cubby/internal/manifest"
	"github.com/Desvelao/cubby/internal/pack"
	"github.com/Desvelao/cubby/internal/render"
	"github.com/Desvelao/cubby/internal/render/slotted"
)

// exporters returns a fresh registry of available exporters. Formats that
// still need panel geometry implemented (stl/step) are added here as they land.
func exporters(sheetWidth, nestGap float64, boxCase bool) map[string]export.Exporter {
	return map[string]export.Exporter{
		"console":          console.Exporter{BoxCase: boxCase},
		"csv":              csv.Exporter{},
		"svg":              svg.Exporter{SheetWidth: sheetWidth, NestGap: &nestGap, BoxCase: boxCase},
		"dxf":              dxf.Exporter{SheetWidth: sheetWidth, NestGap: &nestGap, BoxCase: boxCase},
		"stl":              stl.Exporter{BoxCase: boxCase},
		"step":             step.Exporter{BoxCase: boxCase},
		"iso-svg":          isometric.SVGExporter{BoxCase: boxCase},
		"iso-svg-exploded": isometric.SVGExporter{Exploded: true, BoxCase: boxCase},
		"iso-png":          isometric.PNGExporter{BoxCase: boxCase},
		"iso-png-exploded": isometric.PNGExporter{Exploded: true, BoxCase: boxCase},
		"assembly":         assembly.Exporter{BoxCase: boxCase},
	}
}

func needsGeometry(formats []string) bool {
	for _, f := range formats {
		if f != "console" {
			return true
		}
	}
	return false
}

// hasReduction reports whether any compartment sets a height reduction.
func hasReduction(box pack.BoxResult) bool {
	for _, cr := range box.Compartments {
		for _, r := range []pack.HeightReduction{cr.ExternalReduction, cr.DividerReduction} {
			if r.AmountMM != 0 || r.Percent != 0 || len(r.Panels) > 0 {
				return true
			}
		}
	}
	return false
}

// reductionWarnings returns the non-fatal height-reduction warnings for a laid
// out and rendered box: panels entries matching no panel, and external
// reductions on trays without external panels. build and validate --strict
// share it so both raise the same warnings.
func reductionWarnings(box pack.BoxResult, panels []geometry.Panel) []string {
	warnings := slotted.UnmatchedReductions(box, panels)
	return append(warnings, slotted.UnusedExternalReductions(box)...)
}

func newBuildCmd() *cobra.Command {
	var formats []string
	var outPath, outDir string
	var strict, boxCase bool
	var sheetWidth, nestGap float64

	cmd := &cobra.Command{
		Use:   "build <manifest.yaml>",
		Short: "Pack components, generate the slotted-panel insert design, and export it",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Flag validation happens before the manifest is loaded, so a bad
			// invocation fails fast without rendering or writing anything.
			seen := map[string]bool{}
			for _, f := range formats {
				if seen[f] {
					return newExitError(ExitUsageError, fmt.Errorf("duplicate --format value %q", f))
				}
				seen[f] = true
			}
			registry := exporters(sheetWidth, nestGap, boxCase)
			for _, f := range formats {
				if _, ok := registry[f]; !ok {
					return newExitError(ExitUsageError, fmt.Errorf("unknown --format %q (available: %s)", f, availableFormats(registry)))
				}
			}
			if outPath != "" && outDir != "" {
				return newExitError(ExitUsageError, fmt.Errorf("--out and --out-dir are mutually exclusive"))
			}
			if len(formats) > 1 && outDir == "" {
				if outPath != "" {
					return newExitError(ExitUsageError, fmt.Errorf("--out can only be used with a single --format; use --out-dir for multiple"))
				}
				return newExitError(ExitUsageError, fmt.Errorf("multiple --format values require --out-dir"))
			}
			for _, nf := range []struct {
				name string
				val  float64
			}{{"--sheet-width", sheetWidth}, {"--nest-gap", nestGap}} {
				if math.IsNaN(nf.val) || math.IsInf(nf.val, 0) {
					return newExitError(ExitUsageError, fmt.Errorf("%s must be a finite number (got %g)", nf.name, nf.val))
				}
			}
			if sheetWidth < 0 {
				return newExitError(ExitUsageError, fmt.Errorf("--sheet-width must not be negative (got %g)", sheetWidth))
			}
			if nestGap < 0 {
				return newExitError(ExitUsageError, fmt.Errorf("--nest-gap must not be negative (got %g)", nestGap))
			}
			if outDir == "" && (outPath == "" || outPath == "-") && stdoutIsTerminal(cmd) {
				for _, f := range formats {
					if binaryFormats[f] {
						return newExitError(ExitUsageError, fmt.Errorf("refusing to write binary %s output to a terminal; use --out or --out-dir, or pipe/redirect stdout", f))
					}
				}
			}

			m, err := manifest.Load(args[0])
			if err != nil {
				return newExitError(ExitUsageError, err)
			}
			res := manifest.Validate(m)
			if !res.OK() {
				for _, iss := range res.Issues {
					_, _ = fmt.Fprintln(cmd.ErrOrStderr(), iss)
				}
				return newExitError(ExitValidationFailed, fmt.Errorf("manifest validation failed"))
			}

			boxResult, err := pack.LayoutBox(m)
			if err != nil {
				return newExitError(manifestErrorCode(err), err)
			}

			var panels []geometry.Panel
			// A height reduction can make rendering fail (cutFor), so render
			// whenever one is set, whatever the formats, and report the error
			// the same way for every format and for `validate`.
			if needsGeometry(formats) || hasReduction(boxResult) {
				panels, err = slotted.Backend{}.Render(boxResult, boxResult.Material, render.RenderOptions{})
				if err != nil {
					return newExitError(manifestErrorCode(err), fmt.Errorf("render geometry: %w", err))
				}
			}
			if panels != nil {
				// Panel ids only exist after rendering, so this is where
				// unmatched height-reduction patterns are known. Checked
				// before any output is written so --strict leaves no files.
				warnings := reductionWarnings(boxResult, panels)
				for _, w := range warnings {
					_, _ = fmt.Fprintln(cmd.ErrOrStderr(), w)
				}
				if strict && len(warnings) > 0 {
					return newExitError(ExitValidationFailed, fmt.Errorf("--strict: %d warning(s)", len(warnings)))
				}
			}

			// Every format is rendered to a staged temp file; the files are
			// renamed into place together only once all formats have been
			// exported, so a failing format leaves the directory as it was.
			var staged []*stagedOutput
			discardAll := func() {
				for _, s := range staged {
					s.discard()
				}
			}
			for _, f := range formats {
				exp := registry[f]
				w, out, err := resolveOutput(f, outPath, outDir, m.Box.Name, cmd)
				if err != nil {
					discardAll()
					return newExitError(ExitUsageError, err)
				}
				err = exp.Export(w, boxResult, panels, boxResult.Material)
				if out != nil {
					err = out.close(err)
				}
				if errors.Is(err, export.ErrNoPanels) && missingCount(boxResult.TotalMissing) > 0 {
					// Nothing was placed because the components did not fit;
					// report that (exit 3) rather than the empty export.
					_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "skipping %s: %v\n", f, err)
					continue
				}
				if err != nil {
					discardAll()
					return newExitError(ExitUsageError, fmt.Errorf("export %s: %w", f, err))
				}
				if out != nil {
					staged = append(staged, out)
				}
			}
			if err := commitOutputs(staged); err != nil {
				return newExitError(ExitUsageError, fmt.Errorf("write output: %w", err))
			}

			if n := missingCount(boxResult.TotalMissing); n > 0 {
				return newExitError(ExitPackingIncomplete, fmt.Errorf("%d component instance(s) did not fit in the box", n))
			}
			return nil
		},
	}

	cmd.Flags().StringSliceVar(&formats, "format", []string{"console"}, "output format(s): console|csv|svg|dxf|stl|step|iso-svg|iso-svg-exploded|iso-png|iso-png-exploded|assembly (repeatable/comma-separated); step is experimental")
	cmd.Flags().StringVar(&outPath, "out", "", "output file path (single format only; '-' for stdout; cannot be combined with --out-dir)")
	cmd.Flags().StringVar(&outDir, "out-dir", "", "output directory (required when using multiple --format values; cannot be combined with --out)")
	cmd.Flags().BoolVar(&strict, "strict", false, "treat warnings as errors")
	cmd.Flags().Float64Var(&sheetWidth, "sheet-width", 0, "SVG/DXF nesting canvas width in mm (default: 600)")
	cmd.Flags().Float64Var(&nestGap, "nest-gap", export.NestGap, "SVG/DXF spacing between nested panels in mm (default: 5; 0 allowed)")
	cmd.Flags().BoolVar(&boxCase, "box-case", false, "also draw the game box (interior outline/case) in the outputs")
	return cmd
}

func availableFormats(registry map[string]export.Exporter) string {
	var names []string
	for name := range registry {
		names = append(names, name)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

var formatExt = map[string]string{
	"console":          "txt",
	"csv":              "csv",
	"svg":              "svg",
	"dxf":              "dxf",
	"stl":              "stl",
	"step":             "step",
	"iso-svg":          "svg",
	"iso-svg-exploded": "svg",
	"iso-png":          "png",
	"iso-png-exploded": "png",
	"assembly":         "pdf",
}

// slugify turns a box name into a filename-safe stem. Letters, digits and
// combining marks of any script are kept (lowercased); every other run of
// characters collapses to a single '-', and leading/trailing '-' are trimmed.
// Combining marks are kept so decomposed text (e + U+0301) is not split into
// pieces. No Unicode normalization is applied (the stdlib has none), so
// composed and decomposed spellings of the same name yield distinct,
// individually valid stems. Only letters, digits, marks and '-' survive, so
// path separators and characters reserved on Windows can never appear.
func slugify(s string) string {
	var b strings.Builder
	pendingDash := false
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsMark(r) {
			if pendingDash && b.Len() > 0 {
				b.WriteByte('-')
			}
			pendingDash = false
			b.WriteRune(unicode.ToLower(r))
		} else {
			pendingDash = true
		}
	}
	if b.Len() == 0 {
		return "insert"
	}
	return b.String()
}

// binaryFormats lists the formats whose output is binary and so must not be
// written to an interactive terminal.
var binaryFormats = map[string]bool{
	"iso-png":          true,
	"iso-png-exploded": true,
	"assembly":         true,
}

// stdoutIsTerminal reports whether the command's output is the process's real
// stdout and that stdout is a character device (a terminal). A redirected file
// or pipe is not a character device, and an overridden writer (tests) is never
// a terminal.
// It is a variable so tests can simulate a terminal.
var stdoutIsTerminal = func(cmd *cobra.Command) bool {
	f, ok := cmd.OutOrStdout().(*os.File)
	if !ok {
		return false
	}
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

// stagedOutput is one output file being written. Normally the data goes to a
// hidden temp file next to path and only commit renames it over path, so
// nothing visible changes until every output of a build has succeeded. A
// pre-existing non-regular path (device, symlink, ...) is written in place
// instead, since replacing it would be wrong; that output is not staged, so
// commit and discard have nothing to do for it.
type stagedOutput struct {
	f     *os.File
	path  string
	tmp   string // empty when written in place
	ended bool   // temp file closed and either removed or ready to commit
}

// stageOutput opens the writer for path. An existing file keeps its
// permission bits; a new one gets what os.Create would give (0666 minus
// umask).
func stageOutput(path string) (*stagedOutput, error) {
	fi, statErr := os.Lstat(path)
	if statErr == nil && !fi.Mode().IsRegular() {
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o666)
		if err != nil {
			return nil, err
		}
		return &stagedOutput{f: f, path: path}, nil
	}

	dir, base := filepath.Split(path)
	var f *os.File
	var tmp string
	for i := 0; ; i++ {
		tmp = filepath.Join(dir, fmt.Sprintf(".%s.tmp-%d", base, rand.Uint64()))
		var err error
		f, err = os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o666)
		if err == nil {
			break
		}
		if !os.IsExist(err) || i >= 100 {
			return nil, err
		}
	}
	if statErr == nil {
		if err := f.Chmod(fi.Mode().Perm()); err != nil {
			_ = f.Close()
			_ = os.Remove(tmp)
			return nil, err
		}
	}
	return &stagedOutput{f: f, path: path, tmp: tmp}, nil
}

// close finishes writing: it closes the file and returns exportErr, or the
// close error if there is none. On any error the temp file is removed.
func (s *stagedOutput) close(exportErr error) error {
	err := exportErr
	if cerr := s.f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		s.discard()
	}
	return err
}

// commit renames the temp file over the target path.
func (s *stagedOutput) commit() error {
	if s.tmp == "" || s.ended {
		return nil
	}
	if err := os.Rename(s.tmp, s.path); err != nil {
		s.discard()
		return err
	}
	s.ended = true
	return nil
}

// discard removes the temp file (if any) without touching the target. It is
// safe to call after close, and more than once.
func (s *stagedOutput) discard() {
	_ = s.f.Close()
	if s.tmp != "" && !s.ended {
		_ = os.Remove(s.tmp)
		s.ended = true
	}
}

// commitOutputs renames every staged output over its target, in order. If a
// rename fails, the remaining temp files are removed and the error names the
// files that were already replaced.
func commitOutputs(outs []*stagedOutput) error {
	var done []string
	for i, s := range outs {
		if err := s.commit(); err != nil {
			for _, rest := range outs[i+1:] {
				rest.discard()
			}
			if len(done) > 0 {
				return fmt.Errorf("%w (already replaced: %s)", err, strings.Join(done, ", "))
			}
			return err
		}
		if s.tmp != "" {
			done = append(done, s.path)
		}
	}
	return nil
}

// createOutput returns a writer for path plus a finish function for a single
// output: finish(exportErr) closes the temp file and, when there is no
// error, renames it over path, so a failed export leaves any pre-existing
// file untouched. On any error the temp file is removed and the first error
// (the export error if any) is returned.
func createOutput(path string) (io.Writer, func(error) error, error) {
	s, err := stageOutput(path)
	if err != nil {
		return nil, nil, err
	}
	return s.f, func(exportErr error) error {
		if err := s.close(exportErr); err != nil {
			return err
		}
		return s.commit()
	}, nil
}

// resolveOutput picks the writer for one exported format, given the
// --out/--out-dir flags. Returns the writer, the staged output (nil for
// stdout), and any error.
func resolveOutput(format, outPath, outDir, boxName string, cmd *cobra.Command) (io.Writer, *stagedOutput, error) {
	if outDir != "" {
		if err := os.MkdirAll(outDir, 0o755); err != nil {
			return nil, nil, err
		}
		ext := formatExt[format]
		stem := slugify(boxName)
		if format != ext {
			// Formats whose name differs from their extension (e.g. the
			// iso-* formats, several of which share an extension with each
			// other and with the plain "svg"/"stl" formats) need the format
			// name in the filename too, or multiple requested formats in
			// the same --out-dir would silently overwrite each other.
			stem = fmt.Sprintf("%s.%s", stem, format)
		}
		path := filepath.Join(outDir, fmt.Sprintf("%s.%s", stem, ext))
		return stageResult(path)
	}
	if outPath != "" && outPath != "-" {
		return stageResult(outPath)
	}
	return cmd.OutOrStdout(), nil, nil
}

func stageResult(path string) (io.Writer, *stagedOutput, error) {
	s, err := stageOutput(path)
	if err != nil {
		return nil, nil, err
	}
	return s.f, s, nil
}
