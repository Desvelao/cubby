package export

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Desvelao/cubby/internal/geometry"
)

// PanelGroup is one distinct cut-geometry group of panels: a representative
// Panel plus how many physical instances share its exact geometry.
type PanelGroup struct {
	Panel geometry.Panel
	Qty   int
	IDs   []string // ids of every member panel, in first-seen order
}

// panelKey groups panels of identical cut geometry into one PanelGroup.
type panelKey struct {
	axis           geometry.Axis
	length, height float64
	cut            float64 // height lowered below full height; sets the notch depth
	notches        string  // canonical, order-independent notch signature
}

// sortedNotches returns a copy of notches in canonical (edge, pos, width) order.
func sortedNotches(notches []geometry.Notch) []geometry.Notch {
	ns := append([]geometry.Notch(nil), notches...)
	sort.Slice(ns, func(i, j int) bool {
		a, b := ns[i], ns[j]
		if a.Edge != b.Edge {
			return a.Edge < b.Edge
		}
		if a.Pos != b.Pos {
			return a.Pos < b.Pos
		}
		return a.Width < b.Width
	})
	return ns
}

// compareNotches orders two notch lists numerically (after canonical sorting)
// by edge, position and width, then by count; it returns -1, 0 or 1.
func compareNotches(x, y []geometry.Notch) int {
	a, b := sortedNotches(x), sortedNotches(y)
	for i := 0; i < len(a) && i < len(b); i++ {
		switch {
		case a[i].Edge != b[i].Edge:
			if a[i].Edge < b[i].Edge {
				return -1
			}
			return 1
		case a[i].Pos != b[i].Pos:
			if a[i].Pos < b[i].Pos {
				return -1
			}
			return 1
		case a[i].Width != b[i].Width:
			if a[i].Width < b[i].Width {
				return -1
			}
			return 1
		}
	}
	return len(a) - len(b)
}

// notchSignature returns a canonical string of a panel's notches (edge,
// position, width), sorted so notch order does not matter. Floats are compared
// exactly, like length and height in panelKey.
func notchSignature(notches []geometry.Notch) string {
	ns := append([]geometry.Notch(nil), notches...)
	sort.Slice(ns, func(i, j int) bool {
		a, b := ns[i], ns[j]
		if a.Edge != b.Edge {
			return a.Edge < b.Edge
		}
		if a.Pos != b.Pos {
			return a.Pos < b.Pos
		}
		return a.Width < b.Width
	})
	parts := make([]string, len(ns))
	for i, n := range ns {
		parts[i] = fmt.Sprintf("%s:%v:%v", n.Edge, n.Pos, n.Width)
	}
	return strings.Join(parts, "|")
}

// NotchSignature returns the canonical, order-independent "edge:pos:width"
// notch description joined by "|" (empty when there are no notches).
func NotchSignature(notches []geometry.Notch) string { return notchSignature(notches) }

// GroupPanels collapses physical panel instances that share the same axis,
// length, height, cut, and notches (edge, position, width; order-independent) into one PanelGroup each (keeping the
// first-seen panel as the representative), sorted by axis, length, height, cut
// then notch signature (a total order, so output is deterministic). Used for cut lists and other per-distinct-piece reports.
func GroupPanels(panels []geometry.Panel) []PanelGroup {
	counts := map[panelKey]int{}
	representative := map[panelKey]geometry.Panel{}
	ids := map[panelKey][]string{}
	var order []panelKey
	for _, p := range panels {
		k := panelKey{axis: p.Axis, length: p.Length, height: p.Height, cut: p.Cut, notches: notchSignature(p.Notches)}
		if counts[k] == 0 {
			order = append(order, k)
			representative[k] = p
		}
		counts[k]++
		ids[k] = append(ids[k], p.ID)
	}
	sort.Slice(order, func(i, j int) bool {
		a, b := order[i], order[j]
		if a.axis != b.axis {
			return a.axis < b.axis
		}
		if a.length != b.length {
			return a.length < b.length
		}
		if a.height != b.height {
			return a.height < b.height
		}
		if a.cut != b.cut {
			return a.cut < b.cut
		}
		if c := compareNotches(representative[a].Notches, representative[b].Notches); c != 0 {
			return c < 0
		}
		return a.notches < b.notches
	})

	groups := make([]PanelGroup, len(order))
	for i, k := range order {
		groups[i] = PanelGroup{Panel: representative[k], Qty: counts[k], IDs: ids[k]}
	}
	return groups
}
