package manifest

import (
	"fmt"
	"math"
	"path"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// HeightReduction lowers the height of a class of panels. In YAML it is
// either a scalar (a number of box.units, or a "N%" string) that applies to
// every panel of the class, or a mapping `{by: <scalar>, panels: [ids]}`
// restricting it to the named panels (ids may use `*` globs).
type HeightReduction struct {
	// Amount is the reduction in box.units; zero when Percent is used.
	Amount float64
	// Percent is the reduction as a percentage of the panel height (0-100).
	Percent float64
	// Panels optionally limits the reduction to matching panel ids; empty
	// means every panel of the class.
	Panels []string
}

// IsZero reports whether the reduction changes nothing.
func (r HeightReduction) IsZero() bool { return r.Amount == 0 && r.Percent == 0 }

// UnmarshalYAML accepts the scalar and mapping forms described on the type.
func (r *HeightReduction) UnmarshalYAML(n *yaml.Node) error {
	switch n.Kind {
	case yaml.ScalarNode:
		return r.setBy(n)
	case yaml.MappingNode:
		seenBy, seenPanels := false, false
		for i := 0; i+1 < len(n.Content); i += 2 {
			key, val := n.Content[i], n.Content[i+1]
			switch key.Value {
			case "by":
				if seenBy {
					return fmt.Errorf("line %d: duplicate key %q", key.Line, key.Value)
				}
				seenBy = true
				if err := r.setBy(val); err != nil {
					return err
				}
			case "panels":
				if seenPanels {
					return fmt.Errorf("line %d: duplicate key %q", key.Line, key.Value)
				}
				seenPanels = true
				if err := val.Decode(&r.Panels); err != nil {
					return fmt.Errorf("line %d: panels must be a list of panel ids", val.Line)
				}
				if len(r.Panels) == 0 {
					return fmt.Errorf("line %d: panels must not be empty (omit it to reduce every panel)", val.Line)
				}
				for i, p := range r.Panels {
					r.Panels[i] = strings.TrimSpace(p)
				}
			default:
				return fmt.Errorf("line %d: unknown key %q (expected by, panels)", key.Line, key.Value)
			}
		}
		if !seenBy {
			return fmt.Errorf("line %d: missing required key \"by\"", n.Line)
		}
		return nil
	}
	return fmt.Errorf("line %d: expected a number, a \"N%%\" string or a {by, panels} mapping", n.Line)
}

func (r *HeightReduction) setBy(n *yaml.Node) error {
	if n.Kind != yaml.ScalarNode {
		return fmt.Errorf("line %d: \"by\" must be a number or a \"N%%\" string", n.Line)
	}
	s := strings.TrimSpace(n.Value)
	if pct, ok := strings.CutSuffix(s, "%"); ok {
		v, err := strconv.ParseFloat(strings.TrimSpace(pct), 64)
		if err != nil {
			return fmt.Errorf("line %d: invalid percentage %q", n.Line, n.Value)
		}
		r.Amount, r.Percent = 0, v
		return nil
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return fmt.Errorf("line %d: invalid reduction %q (use a number or \"N%%\")", n.Line, n.Value)
	}
	r.Amount, r.Percent = v, 0
	return nil
}

// ReductionClass is the class of panels a HeightReduction applies to.
type ReductionClass int

const (
	// ExternalPanels are boundary walls and removable tray walls.
	ExternalPanels ReductionClass = iota
	// DividerPanels are grid and internal dividers.
	DividerPanels
)

// idPrefixes lists the panel id prefixes (with their trailing "-") of each class.
var idPrefixes = map[ReductionClass][]string{
	ExternalPanels: {"w-", "wv-", "rw-", "rv-"},
	DividerPanels:  {"h-", "v-", "iv-", "ih-", "ie-v-"},
}

// patternCanMatch reports whether pattern could match at least one panel id
// of class by its prefix alone. It looks at the literal text before the first
// glob metacharacter: that text must either start with one of the class
// prefixes or, when the pattern has a wildcard, be a leading part of one
// (so `w*` is accepted for external panels but not for divider panels).
func patternCanMatch(class ReductionClass, pattern string) bool {
	lit := pattern
	wild := false
	if i := strings.IndexAny(pattern, `*?[\`); i >= 0 {
		lit, wild = pattern[:i], true
	}
	for _, q := range idPrefixes[class] {
		if strings.HasPrefix(lit, q) || (wild && strings.HasPrefix(q, lit)) {
			return true
		}
	}
	return false
}

// Validate returns a message for the first problem with r, or "". class is
// the class of panels r is attached to; every panels entry must be a valid
// glob that could match at least one id of that class.
func (r HeightReduction) Validate(class ReductionClass) string {
	switch {
	case math.IsNaN(r.Amount) || math.IsInf(r.Amount, 0) || r.Amount < 0:
		return "must be finite and >= 0"
	case math.IsNaN(r.Percent) || r.Percent < 0 || r.Percent >= 100:
		return "percentage must be >= 0 and < 100"
	}
	for _, p := range r.Panels {
		if strings.TrimSpace(p) == "" {
			return "panels must not contain empty ids"
		}
		if _, err := path.Match(p, ""); err != nil {
			return fmt.Sprintf("panels entry %q is not a valid pattern", p)
		}
		if !patternCanMatch(class, p) {
			return fmt.Sprintf("panels entry %q can never match a panel of this class (ids are lowercase and start with %s)",
				p, strings.Join(idPrefixes[class], ", "))
		}
	}
	return ""
}
