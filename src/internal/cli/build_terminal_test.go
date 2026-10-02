package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// runBuild executes the root command with stdout captured in a buffer and
// returns stdout, stderr and the exit code.
func runBuild(t *testing.T, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	root := NewRootCmd()
	var outBuf, errBuf bytes.Buffer
	root.SetOut(&outBuf)
	root.SetErr(&errBuf)
	root.SetArgs(args)
	err := root.Execute()
	if err != nil {
		// main prints the returned error to stderr; mirror that here.
		errBuf.WriteString("error: " + err.Error() + "\n")
	}
	return outBuf.String(), errBuf.String(), ExitCode(err)
}

func setTerminal(t *testing.T, isTerminal bool) {
	t.Helper()
	orig := stdoutIsTerminal
	stdoutIsTerminal = func(*cobra.Command) bool { return isTerminal }
	t.Cleanup(func() { stdoutIsTerminal = orig })
}

const terminalManifest = "../../testdata/manifests/two-compartment.yaml"

func TestBuildRefusesBinaryFormatsOnTerminal(t *testing.T) {
	setTerminal(t, true)
	for f := range binaryFormats {
		for _, extra := range [][]string{nil, {"--out", "-"}} {
			args := append([]string{"build", terminalManifest, "--format", f}, extra...)
			out, errOut, code := runBuild(t, args...)
			if code != ExitUsageError {
				t.Errorf("%v: exit code = %d, want %d", args, code, ExitUsageError)
			}
			if out != "" {
				t.Errorf("%v: expected no stdout, got %d bytes", args, len(out))
			}
			if !strings.Contains(errOut, "refusing to write binary "+f+" output to a terminal") {
				t.Errorf("%v: unexpected stderr %q", args, errOut)
			}
		}
	}
}

func TestBuildAllowsTextFormatsOnTerminal(t *testing.T) {
	setTerminal(t, true)
	for _, f := range []string{"console", "csv", "svg", "dxf", "stl", "step", "iso-svg", "iso-svg-exploded"} {
		if binaryFormats[f] {
			t.Fatalf("%s unexpectedly listed as binary", f)
		}
		out, errOut, code := runBuild(t, "build", terminalManifest, "--format", f)
		if code != ExitOK || out == "" {
			t.Errorf("%s: exit %d, stdout %d bytes (stderr: %s)", f, code, len(out), errOut)
		}
	}
}

func TestBuildBinaryFormatOnTerminalAllowedWithFileOutput(t *testing.T) {
	setTerminal(t, true)
	dir := t.TempDir()
	if _, errOut, code := runBuild(t, "build", terminalManifest, "--format", "iso-png", "--out-dir", dir); code != ExitOK {
		t.Fatalf("--out-dir: exit %d (stderr: %s)", code, errOut)
	}
	if _, errOut, code := runBuild(t, "build", terminalManifest, "--format", "iso-png", "--out", dir+"/x.png"); code != ExitOK {
		t.Fatalf("--out file: exit %d (stderr: %s)", code, errOut)
	}
}

func TestBuildBinaryFormatToStdoutDashNonTerminalAllowed(t *testing.T) {
	setTerminal(t, false)
	for f := range binaryFormats {
		out, errOut, code := runBuild(t, "build", terminalManifest, "--format", f, "--out", "-")
		if code != ExitOK || out == "" {
			t.Errorf("%s --out -: exit %d, stdout %d bytes (stderr: %s)", f, code, len(out), errOut)
		}
	}
	out, _, _ := runBuild(t, "build", terminalManifest, "--format", "iso-png", "--out", "-")
	if !strings.HasPrefix(out, "\x89PNG") {
		t.Errorf("expected PNG data on stdout for --out -")
	}
}
