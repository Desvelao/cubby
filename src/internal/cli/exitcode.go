package cli

import (
	"errors"

	"github.com/Desvelao/cubby/internal/pack"
)

// Exit codes are documented in the README and are part of cubby's
// scripting/CI contract: callers can branch on them without parsing output.
const (
	ExitOK                = 0
	ExitUsageError        = 1
	ExitValidationFailed  = 2
	ExitPackingIncomplete = 3
)

// exitError carries a specific exit code out of a cobra RunE.
type exitError struct {
	code int
	err  error
}

func (e *exitError) Error() string { return e.err.Error() }
func (e *exitError) Unwrap() error { return e.err }

func newExitError(code int, err error) error {
	if err == nil {
		return nil
	}
	return &exitError{code: code, err: err}
}

// ExitCode extracts the intended process exit code from an error returned by
// a command's RunE, defaulting to ExitUsageError for plain errors.
func ExitCode(err error) int {
	if err == nil {
		return ExitOK
	}
	var ee *exitError
	if errors.As(err, &ee) {
		return ee.code
	}
	return ExitUsageError
}

// manifestErrorCode returns ExitValidationFailed for errors caused by the
// manifest's height-reduction settings and ExitUsageError for everything else
// (internal and I/O failures).
func manifestErrorCode(err error) int {
	var re *pack.ReductionError
	if errors.As(err, &re) {
		return ExitValidationFailed
	}
	return ExitUsageError
}
