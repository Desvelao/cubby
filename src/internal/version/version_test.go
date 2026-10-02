package version

import "testing"

// The values are overridden at build time via -ldflags, so only the
// invariant that they are always populated is checked, not their content.
func TestMetadataNeverEmpty(t *testing.T) {
	for name, v := range map[string]string{"Version": Version, "Commit": Commit, "Date": Date} {
		if v == "" {
			t.Errorf("%s must not be empty (defaults apply when not injected)", name)
		}
	}
}
