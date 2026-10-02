package slotted

import (
	"testing"

	"github.com/Desvelao/cubby/internal/geometry"
)

func TestCheckNotchesRejectsBadEdges(t *testing.T) {
	if err := checkNotches("p", 100, []geometry.Notch{{Pos: 0, Width: 3, Edge: "sideways"}}); err == nil {
		t.Fatal("unknown edge accepted")
	}
	mixed := []geometry.Notch{{Pos: 0, Width: 3, Edge: geometry.EdgeTop}, {Pos: 50, Width: 3, Edge: geometry.EdgeBottom}}
	if err := checkNotches("p", 100, mixed); err == nil {
		t.Fatal("mixed edges accepted")
	}
}
