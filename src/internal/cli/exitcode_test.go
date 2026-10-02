package cli

import (
	"errors"
	"fmt"
	"testing"

	"github.com/Desvelao/cubby/internal/pack"
)

func TestManifestErrorCode(t *testing.T) {
	reduction := &pack.ReductionError{Err: errors.New("bad reduction")}
	tests := []struct {
		name string
		err  error
		want int
	}{
		{"reduction error", reduction, ExitValidationFailed},
		{"wrapped reduction error", fmt.Errorf("layout: %w", reduction), ExitValidationFailed},
		{"plain error", errors.New("disk on fire"), ExitUsageError},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := manifestErrorCode(tc.err); got != tc.want {
				t.Fatalf("manifestErrorCode() = %d, want %d", got, tc.want)
			}
		})
	}
}
