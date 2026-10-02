package export

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/Desvelao/cubby/internal/geometry"
)

func TestGroupPanelsCollapsesIdenticalGeometry(t *testing.T) {
	panels := []geometry.Panel{
		{ID: "h-1", Axis: geometry.AxisWidthRun, Length: 300, Height: 60, Notches: nil},
		{ID: "h-2", Axis: geometry.AxisWidthRun, Length: 300, Height: 60, Notches: nil},
		{ID: "v-1", Axis: geometry.AxisDepthRun, Length: 145, Height: 60, Notches: []geometry.Notch{{Pos: 0, Width: 3}}},
	}

	groups := GroupPanels(panels)
	if len(groups) != 2 {
		t.Fatalf("expected 2 distinct groups, got %d: %+v", len(groups), groups)
	}

	var widthRun, depthRun *PanelGroup
	for i, g := range groups {
		switch g.Panel.Axis {
		case geometry.AxisWidthRun:
			widthRun = &groups[i]
		case geometry.AxisDepthRun:
			depthRun = &groups[i]
		}
	}
	if widthRun == nil || widthRun.Qty != 2 {
		t.Fatalf("expected width-run group with qty 2, got %+v", widthRun)
	}
	if widthRun.Panel.ID != "h-1" {
		t.Fatalf("expected the first-seen panel (h-1) as the representative, got %q", widthRun.Panel.ID)
	}
	if depthRun == nil || depthRun.Qty != 1 {
		t.Fatalf("expected depth-run group with qty 1, got %+v", depthRun)
	}
}

func TestGroupPanelsSortedByAxisThenLengthThenHeight(t *testing.T) {
	panels := []geometry.Panel{
		{ID: "b", Axis: geometry.AxisWidthRun, Length: 200, Height: 60},
		{ID: "a", Axis: geometry.AxisWidthRun, Length: 100, Height: 60},
		{ID: "c", Axis: geometry.AxisDepthRun, Length: 50, Height: 60},
	}

	groups := GroupPanels(panels)
	if len(groups) != 3 {
		t.Fatalf("expected 3 distinct groups, got %d", len(groups))
	}
	if groups[0].Panel.Axis != geometry.AxisDepthRun {
		t.Fatalf("expected depth-run group first (axis sorts before width-run), got %+v", groups[0])
	}
	if groups[1].Panel.ID != "a" || groups[2].Panel.ID != "b" {
		t.Fatalf("expected width-run groups sorted by length ascending, got %+v then %+v", groups[1], groups[2])
	}
}

func TestGroupPanelsSeparatesDifferentNotchPositions(t *testing.T) {
	a := []geometry.Notch{{Pos: 10, Width: 3, Edge: "top"}, {Pos: 50, Width: 3, Edge: "top"}}
	aReordered := []geometry.Notch{{Pos: 50, Width: 3, Edge: "top"}, {Pos: 10, Width: 3, Edge: "top"}}
	b := []geometry.Notch{{Pos: 10, Width: 3, Edge: "top"}, {Pos: 70, Width: 3, Edge: "top"}}
	panels := []geometry.Panel{
		{ID: "p1", Axis: geometry.AxisWidthRun, Length: 100, Height: 60, Notches: a},
		{ID: "p2", Axis: geometry.AxisWidthRun, Length: 100, Height: 60, Notches: b},
		{ID: "p3", Axis: geometry.AxisWidthRun, Length: 100, Height: 60, Notches: aReordered},
	}

	groups := GroupPanels(panels)
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups for different notch positions, got %d: %+v", len(groups), groups)
	}
	qty := map[string]int{}
	for _, g := range groups {
		qty[g.Panel.ID] = g.Qty
	}
	if qty["p1"] != 2 || qty["p2"] != 1 {
		t.Fatalf("expected p1 qty 2 (order-independent) and p2 qty 1, got %+v", qty)
	}
}

func TestGroupPanelsSeparatesDifferentCut(t *testing.T) {
	n := []geometry.Notch{{Pos: 10, Width: 3, Edge: "top"}}
	panels := []geometry.Panel{
		{ID: "full", Axis: geometry.AxisWidthRun, Length: 100, Height: 60, Notches: n},
		{ID: "cut", Axis: geometry.AxisWidthRun, Length: 100, Height: 60, Cut: 20, Notches: n},
		{ID: "cut2", Axis: geometry.AxisWidthRun, Length: 100, Height: 60, Cut: 20, Notches: n},
	}

	groups := GroupPanels(panels)
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups for different cuts, got %d: %+v", len(groups), groups)
	}
	qty := map[string]int{}
	for _, g := range groups {
		qty[g.Panel.ID] = g.Qty
	}
	if qty["full"] != 1 || qty["cut"] != 2 {
		t.Fatalf("expected full qty 1 and cut qty 2, got %+v", qty)
	}
}

func TestGroupPanelsDeterministicOrderForTiedGroups(t *testing.T) {
	mk := func(cut float64, pos float64) geometry.Panel {
		return geometry.Panel{Axis: geometry.AxisWidthRun, Length: 100, Height: 60, Cut: cut,
			Notches: []geometry.Notch{{Pos: pos, Width: 3, Edge: "top"}}}
	}
	base := []geometry.Panel{mk(0, 10), mk(0, 50), mk(0, 30), mk(20, 10), mk(20, 50), mk(0, 10)}
	describe := func(gs []PanelGroup) string {
		out := ""
		for _, g := range gs {
			out += fmt.Sprintf("%v/%s/%d;", g.Panel.Cut, notchSignature(g.Panel.Notches), g.Qty)
		}
		return out
	}
	want := describe(GroupPanels(base))
	if len(GroupPanels(base)) != 5 {
		t.Fatalf("expected 5 groups, got %d", len(GroupPanels(base)))
	}
	rng := rand.New(rand.NewSource(1))
	for i := 0; i < 200; i++ {
		in := append([]geometry.Panel(nil), base...)
		rng.Shuffle(len(in), func(a, b int) { in[a], in[b] = in[b], in[a] })
		if got := describe(GroupPanels(in)); got != want {
			t.Fatalf("order changed with input shuffle:\n got %s\nwant %s", got, want)
		}
	}
	gs := GroupPanels(base)
	if gs[0].Panel.Cut != 0 || gs[len(gs)-1].Panel.Cut != 20 {
		t.Fatalf("expected cut ascending as tie-breaker, got %s", want)
	}
}
