package export

import (
	"errors"
	"testing"
)

// limitWriter succeeds for the first ok writes, then fails with err. It counts
// every Write call so tests can see that nothing is written after a failure.
type limitWriter struct {
	ok    int
	err   error
	calls int
}

func (l *limitWriter) Write(p []byte) (int, error) {
	l.calls++
	if l.calls > l.ok {
		return 0, l.err
	}
	return len(p), nil
}

func TestErrWriterFirstErrorIsSticky(t *testing.T) {
	first := errors.New("first")
	lw := &limitWriter{ok: 1, err: first}
	ew := &ErrWriter{W: lw}

	ew.Printf("a%d", 1)
	if ew.Err != nil {
		t.Fatalf("unexpected error after successful write: %v", ew.Err)
	}
	ew.Println("b") // fails
	if !errors.Is(ew.Err, first) {
		t.Fatalf("expected first error, got %v", ew.Err)
	}
	callsAtFailure := lw.calls

	// Change what the writer would return: the original error must stick and
	// no further writes may reach the writer.
	lw.err = errors.New("second")
	ew.Printf("c")
	ew.Println("d")
	if !errors.Is(ew.Err, first) {
		t.Fatalf("error was replaced: %v", ew.Err)
	}
	if lw.calls != callsAtFailure {
		t.Fatalf("writes reached the writer after failure: %d calls, want %d", lw.calls, callsAtFailure)
	}
}
