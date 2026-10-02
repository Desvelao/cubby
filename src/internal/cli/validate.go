package cli

import (
	"errors"
	"fmt"
	"io/fs"

	"github.com/spf13/cobra"

	"github.com/Desvelao/cubby/internal/export"
	"github.com/Desvelao/cubby/internal/geometry"
	"github.com/Desvelao/cubby/internal/manifest"
	"github.com/Desvelao/cubby/internal/pack"
	"github.com/Desvelao/cubby/internal/render"
	"github.com/Desvelao/cubby/internal/render/slotted"
)

// checkReductionRender lays out and renders a manifest that sets a height
// reduction, so reductions that only fail while cutting panels (too tall for
// the panel, or past half the height of a notched one) are caught here just
// as `build` reports them. It returns the laid out box and rendered panels
// (nil panels when no reduction is set) and a non-nil issue on failure.
func checkReductionRender(m *manifest.Manifest) (pack.BoxResult, []geometry.Panel, *manifest.Issue) {
	if !manifestSetsReduction(m) {
		return pack.BoxResult{}, nil, nil
	}
	box, err := pack.LayoutBox(m)
	if err != nil {
		return box, nil, &manifest.Issue{Path: "layout", Message: err.Error()}
	}
	panels, err := (slotted.Backend{}).Render(box, box.Material, render.RenderOptions{})
	if err != nil {
		return box, nil, &manifest.Issue{Path: "heightReduction", Message: fmt.Sprintf("render geometry: %v", err)}
	}
	return box, panels, nil
}

// validateOutput is the JSON shape of validate: the validation result plus,
// only when there are any, the build-time reduction warnings.
type validateOutput struct {
	manifest.ValidationResult
	Warnings []string `json:"warnings,omitempty"`
}

// manifestSetsReduction reports whether the defaults or any group set a
// height reduction, so validate packs only when there is something to check.
func manifestSetsReduction(m *manifest.Manifest) bool {
	set := func(r *manifest.HeightReduction) bool {
		return r != nil && (!r.IsZero() || len(r.Panels) > 0)
	}
	if set(&m.Defaults.ExternalHeightReduction) || set(&m.Defaults.DividerHeightReduction) {
		return true
	}
	for _, g := range m.Groups {
		if set(g.ExternalHeightReduction) || set(g.DividerHeightReduction) {
			return true
		}
	}
	return false
}

func newValidateCmd() *cobra.Command {
	var format string
	var strict bool

	cmd := &cobra.Command{
		Use:   "validate <manifest.yaml>",
		Short: "Validate a manifest's schema and cross-references without packing",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := manifest.Load(args[0])
			if err != nil {
				// A manifest that was read but failed to parse still gets a
				// JSON result so --format json consumers always have something
				// to decode; the exit code and stderr message are unchanged.
				// An unreadable file (missing, permissions) stays a plain
				// error: there is no manifest to report on.
				var pathErr *fs.PathError
				if format == "json" && !errors.As(err, &pathErr) {
					loadFail := validateOutput{ValidationResult: manifest.ValidationResult{
						Issues: []manifest.Issue{{Path: "", Message: err.Error()}},
					}}
					if jerr := writeJSON(cmd.OutOrStdout(), loadFail); jerr != nil {
						return newExitError(ExitUsageError, jerr)
					}
				}
				return newExitError(ExitUsageError, err)
			}
			result := manifest.Validate(m)
			var warnings []string
			if result.OK() {
				box, panels, iss := checkReductionRender(m)
				if iss != nil {
					result.Issues = append(result.Issues, *iss)
				} else if panels != nil {
					warnings = reductionWarnings(box, panels)
				}
			}
			for _, w := range warnings {
				_, _ = fmt.Fprintln(cmd.ErrOrStderr(), w)
			}
			failStrict := strict && len(warnings) > 0

			out := cmd.OutOrStdout()
			switch format {
			case "text", "":
				ew := &export.ErrWriter{W: out}
				if result.OK() {
					if failStrict {
						ew.Printf("FAIL: %d warning(s) treated as errors by --strict\n", len(warnings))
					} else {
						ew.Println("OK: manifest is valid")
					}
					if ew.Err != nil {
						return ew.Err
					}
					break
				}
				ew.Printf("FAIL: %d issue(s) found:\n", len(result.Issues))
				for _, iss := range result.Issues {
					ew.Printf("  - %s\n", iss)
				}
				if ew.Err != nil {
					return ew.Err
				}
			case "json":
				if err := writeJSON(out, validateOutput{result, warnings}); err != nil {
					return newExitError(ExitUsageError, err)
				}
			default:
				return newExitError(ExitUsageError, fmt.Errorf("unknown --format %q (expected text or json)", format))
			}

			if !result.OK() {
				return newExitError(ExitValidationFailed, fmt.Errorf("manifest validation failed"))
			}
			if failStrict {
				return newExitError(ExitValidationFailed, fmt.Errorf("--strict: %d warning(s)", len(warnings)))
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&format, "format", "text", "output format: text|json")
	cmd.Flags().BoolVar(&strict, "strict", false, "treat warnings (unmatched or unused height-reduction settings) as errors")
	return cmd
}
