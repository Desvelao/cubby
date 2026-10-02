package slotted

import (
	"fmt"
	"path"
	"strings"

	"github.com/Desvelao/cubby/internal/geometry"
	"github.com/Desvelao/cubby/internal/pack"
)

type panelClass int

const (
	classNone panelClass = iota
	classExternal
	classDivider
)

func (c panelClass) String() string {
	if c == classExternal {
		return "external"
	}
	return "divider"
}

// UnmatchedReductions returns one warning per height-reduction `panels`
// entry that matches no panel it could reduce. It runs after rendering,
// because panel ids (h-N, iv-<tray>-<row>-<n>, ...) only exist then.
//
// An entry matches when a panel of the setting's own class has a matching id
// and, for tray-specific ids (iv-, ih-, ie-v-, rw-, rv-), belongs to a tray
// whose setting holds the entry. Grid ids (h-N, v-N, w-N, wv-N) are shared
// between trays and match for any tray holding the entry. An entry that
// several trays inherit (e.g. from defaults) is reported once, and only when
// none of them matched it.
func UnmatchedReductions(box pack.BoxResult, panels []geometry.Panel) []string {
	type key struct {
		class   panelClass
		pattern string
	}
	var order []key
	matched := map[key]bool{}
	for _, cr := range box.Compartments {
		for _, s := range []struct {
			class panelClass
			red   pack.HeightReduction
		}{{classExternal, cr.ExternalReduction}, {classDivider, cr.DividerReduction}} {
			for _, pat := range s.red.Panels {
				k := key{s.class, pat}
				if _, seen := matched[k]; !seen {
					matched[k] = false
					order = append(order, k)
				}
				if matched[k] {
					continue
				}
				for _, p := range panels {
					class, tray, ok := classify(p.ID)
					if !ok || class != s.class || (tray && !ownedByTray(p.ID, cr.ID)) {
						continue
					}
					if m, _ := path.Match(pat, p.ID); m {
						matched[k] = true
						break
					}
				}
			}
		}
	}

	var warnings []string
	for _, k := range order {
		if matched[k] {
			continue
		}
		msg := fmt.Sprintf("warning: %s panels entry %q matches no %s panel",
			settingName(k.class), k.pattern, k.class)
		other := classExternal
		if k.class == classExternal {
			other = classDivider
		}
		for _, p := range panels {
			if c, _, ok := classify(p.ID); ok && c == other {
				if m, _ := path.Match(k.pattern, p.ID); m {
					msg += fmt.Sprintf(" (it only matches %s panels)", other)
					break
				}
			}
		}
		warnings = append(warnings, msg)
	}
	return warnings
}

func settingName(c panelClass) string {
	if c == classExternal {
		return "externalHeightReduction"
	}
	return "dividerHeightReduction"
}

// classify returns a panel id's reduction class and whether the id is
// tray-specific. ok is false for panels no reduction applies to (floors).
func classify(id string) (class panelClass, tray, ok bool) {
	switch {
	case isGridID(id, "h-"), isGridID(id, "v-"):
		return classDivider, false, true
	case isGridID(id, "w-"), isGridID(id, "wv-"):
		return classExternal, false, true
	case strings.HasPrefix(id, "iv-"), strings.HasPrefix(id, "ih-"), strings.HasPrefix(id, "ie-v-"):
		return classDivider, true, true
	case strings.HasPrefix(id, "rw-"), strings.HasPrefix(id, "rv-"):
		return classExternal, true, true
	}
	return classNone, false, false
}

// isGridID reports whether id is prefix followed only by digits.
func isGridID(id, prefix string) bool {
	rest, ok := strings.CutPrefix(id, prefix)
	if !ok || rest == "" {
		return false
	}
	for _, r := range rest {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// ownedByTray reports whether tray-specific panel id was generated for the
// tray with the given id. Tray ids may contain hyphens, so the id is matched
// against each exact generated form rather than split.
func ownedByTray(id, tray string) bool {
	if id == "ie-v-"+tray || id == "rw-"+tray+"-front" || id == "rw-"+tray+"-back" ||
		id == "rv-"+tray+"-left" || id == "rv-"+tray+"-right" {
		return true
	}
	if rest, ok := strings.CutPrefix(id, "iv-"+tray+"-"); ok {
		a, b, found := strings.Cut(rest, "-")
		return found && allDigits(a) && allDigits(b)
	}
	if rest, ok := strings.CutPrefix(id, "ih-"+tray+"-"); ok {
		return allDigits(rest)
	}
	return false
}

func allDigits(s string) bool { return isGridID("x-"+s, "x-") }

// UnusedExternalReductions returns one warning per distinct non-zero
// externalHeightReduction setting that no tray holding it can apply. External
// panels only exist for removable trays (rw-*/rv-*) and fullWalls boundary
// walls (w-*/wv-*), so on any other tray the setting is silently ignored.
//
// Settings are compared by value, so one inherited from defaults by several
// trays is reported once, and only when none of them is removable or
// fullWalls; a tray where it does take effect keeps it quiet. A tray that
// overrides it with a different value is judged on its own.
func UnusedExternalReductions(box pack.BoxResult) []string {
	type group struct {
		trays   []string
		applies bool
	}
	var order []string
	groups := map[string]*group{}
	for _, cr := range box.Compartments {
		r := cr.ExternalReduction
		if r.AmountMM == 0 && r.Percent == 0 {
			continue
		}
		k := fmt.Sprintf("%v", r)
		g, ok := groups[k]
		if !ok {
			g = &group{}
			groups[k] = g
			order = append(order, k)
		}
		g.trays = append(g.trays, cr.ID)
		if cr.Removable || cr.FullWalls {
			g.applies = true
		}
	}

	var warnings []string
	for _, k := range order {
		g := groups[k]
		if g.applies {
			continue
		}
		warnings = append(warnings, fmt.Sprintf(
			"warning: externalHeightReduction on tray(s) %s has no effect: they are neither removable nor fullWalls, so they have no external panels",
			strings.Join(g.trays, ", ")))
	}
	return warnings
}
