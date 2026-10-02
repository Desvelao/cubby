package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreateOutputRemovesNewPartialFileOnError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.bin")
	w, finish, err := createOutput(path)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = w.Write([]byte("partial"))
	boom := errors.New("boom")
	if got := finish(boom); !errors.Is(got, boom) {
		t.Fatalf("expected export error to be returned, got %v", got)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected partial file removed, stat err: %v", err)
	}
}

func TestCreateOutputKeepsFileOnSuccess(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.bin")
	w, finish, err := createOutput(path)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = w.Write([]byte("ok"))
	if err := finish(nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected file kept: %v", err)
	}
}

func dirEntries(t *testing.T, dir string) []string {
	t.Helper()
	es, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range es {
		names = append(names, e.Name())
	}
	return names
}

func TestCreateOutputFailureLeavesPreexistingFileUntouched(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.bin")
	if err := os.WriteFile(path, []byte("old"), 0o640); err != nil {
		t.Fatal(err)
	}
	w, finish, err := createOutput(path)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = w.Write([]byte("partial"))
	_ = finish(errors.New("boom"))
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "old" {
		t.Fatalf("pre-existing file must be unchanged, got %q, err %v", got, err)
	}
	if names := dirEntries(t, dir); len(names) != 1 {
		t.Fatalf("expected no temp files left, got %v", names)
	}
}

func TestCreateOutputSuccessReplacesPreexistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.bin")
	if err := os.WriteFile(path, []byte("old"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o640); err != nil {
		t.Fatal(err)
	}
	w, finish, err := createOutput(path)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = w.Write([]byte("new"))
	if got, _ := os.ReadFile(path); string(got) != "old" {
		t.Fatalf("target must not change before finish, got %q", got)
	}
	if err := finish(nil); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "new" {
		t.Fatalf("expected replaced content, got %q", got)
	}
	fi, _ := os.Stat(path)
	if fi.Mode().Perm() != 0o640 {
		t.Fatalf("expected mode preserved, got %v", fi.Mode().Perm())
	}
	if names := dirEntries(t, dir); len(names) != 1 {
		t.Fatalf("expected no temp files left, got %v", names)
	}
}

func TestCreateOutputRenameFailureCleansUp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.bin")
	w, finish, err := createOutput(path)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = w.Write([]byte("x"))
	// Make the target a non-empty directory so the rename fails.
	if err := os.MkdirAll(filepath.Join(path, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := finish(nil); err == nil {
		t.Fatal("expected rename error")
	}
	if names := dirEntries(t, dir); len(names) != 1 {
		t.Fatalf("expected no temp files left, got %v", names)
	}
}

func TestCreateOutputReportsCloseError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.bin")
	w, finish, err := createOutput(path)
	if err != nil {
		t.Fatal(err)
	}
	_ = w.(*os.File).Close() // force the later Close to fail
	if err := finish(nil); err == nil {
		t.Fatal("expected close error to be reported")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected file removed after close failure, stat err: %v", err)
	}
}

func TestCommitOutputsReportsAlreadyReplacedFiles(t *testing.T) {
	dir := t.TempDir()
	var outs []*stagedOutput
	for _, n := range []string{"a", "b", "c"} {
		s, err := stageOutput(filepath.Join(dir, n))
		if err != nil {
			t.Fatal(err)
		}
		_, _ = s.f.Write([]byte(n))
		if err := s.close(nil); err != nil {
			t.Fatal(err)
		}
		outs = append(outs, s)
	}
	// Make the second rename fail.
	if err := os.Remove(outs[1].tmp); err != nil {
		t.Fatal(err)
	}
	err := commitOutputs(outs)
	if err == nil || !strings.Contains(err.Error(), "already replaced: "+filepath.Join(dir, "a")) {
		t.Fatalf("expected already-replaced report, got %v", err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 || entries[0].Name() != "a" {
		t.Fatalf("expected only a to remain, got %v", entries)
	}
}
