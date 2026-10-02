package export

import (
	"fmt"
	"io"
)

// ErrWriter accumulates the first write error across many Printf/Println
// calls so an exporter's report body doesn't need to check each one
// individually.
type ErrWriter struct {
	W   io.Writer
	Err error
}

func (e *ErrWriter) Printf(format string, args ...any) {
	if e.Err != nil {
		return
	}
	_, e.Err = fmt.Fprintf(e.W, format, args...)
}

func (e *ErrWriter) Println(args ...any) {
	if e.Err != nil {
		return
	}
	_, e.Err = fmt.Fprintln(e.W, args...)
}
