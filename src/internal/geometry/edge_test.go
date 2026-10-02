package geometry

import (
	"math"
	"testing"
)

func TestEdgeConstantsKeepSerializedValues(t *testing.T) {
	if string(EdgeTop) != "top" || string(EdgeBottom) != "bottom" {
		t.Fatalf("edge constants changed: %q %q", EdgeTop, EdgeBottom)
	}
	top := BuildOutline(100, 40, []Notch{{Pos: 40, Width: 3, Edge: EdgeTop}})
	lit := BuildOutline(100, 40, []Notch{{Pos: 40, Width: 3, Edge: "top"}})
	if len(top) != len(lit) {
		t.Fatalf("constant and literal edge disagree")
	}
}

func TestValidateNotchEdges(t *testing.T) {
	if err := ValidateNotchEdges(nil); err != nil {
		t.Fatalf("empty list: %v", err)
	}
	if err := ValidateNotchEdges([]Notch{{Edge: EdgeTop}, {Pos: 9, Edge: EdgeTop}}); err != nil {
		t.Fatalf("single edge: %v", err)
	}
	if err := ValidateNotchEdges([]Notch{{Edge: "bottm"}}); err == nil {
		t.Fatal("unknown edge accepted")
	}
	if err := ValidateNotchEdges([]Notch{{Edge: ""}}); err == nil {
		t.Fatal("empty edge accepted")
	}
	if err := ValidateNotchEdges([]Notch{{Edge: EdgeTop}, {Pos: 9, Edge: EdgeBottom}}); err == nil {
		t.Fatal("mixed edges accepted")
	}
}

func TestValidateNotches(t *testing.T) {
	n := func(pos, w float64, e Edge) Notch { return Notch{Pos: pos, Width: w, Edge: e} }
	cases := []struct {
		name    string
		notches []Notch
		height  float64
		ok      bool
	}{
		{"none", nil, 40, true},
		{"valid flush ends", []Notch{n(0, 3, EdgeTop), n(97, 3, EdgeTop)}, 40, true},
		{"touching", []Notch{n(10, 3, EdgeBottom), n(13, 3, EdgeBottom)}, 40, true},
		{"negative pos", []Notch{n(-1, 3, EdgeTop)}, 40, false},
		{"past end", []Notch{n(98, 3, EdgeTop)}, 40, false},
		{"overlap", []Notch{n(20, 3, EdgeTop), n(10, 11, EdgeTop)}, 40, false},
		{"mixed edges", []Notch{n(10, 3, EdgeTop), n(50, 3, EdgeBottom)}, 40, false},
		{"unknown edge", []Notch{n(10, 3, "sideways")}, 40, false},
		{"zero width", []Notch{n(10, 0, EdgeTop)}, 40, false},
		{"negative width", []Notch{n(10, -3, EdgeTop)}, 40, false},
		{"NaN pos", []Notch{n(math.NaN(), 3, EdgeTop)}, 40, false},
		{"NaN width", []Notch{n(10, math.NaN(), EdgeTop)}, 40, false},
		{"Inf width", []Notch{n(10, math.Inf(1), EdgeTop)}, 40, false},
		{"Inf pos", []Notch{n(math.Inf(-1), 3, EdgeTop)}, 40, false},
		{"within eps flush", []Notch{n(-notchEps/2, 3, EdgeTop), n(97+notchEps/2, 3, EdgeTop)}, 40, true},
		{"zero height", []Notch{n(10, 3, EdgeTop)}, 0, false},
	}
	for _, c := range cases {
		err := ValidateNotches(100, c.height, c.notches)
		if (err == nil) != c.ok {
			t.Errorf("%s: err = %v, want ok=%v", c.name, err, c.ok)
		}
	}
}

func TestValidateCut(t *testing.T) {
	cases := []struct {
		name     string
		cut      float64
		notched  bool
		wantFail bool
	}{
		{"no cut", 0, true, false},
		{"negative", -5, true, false},
		{"below notch depth", 19.9, true, false},
		{"at notch depth", 20, true, true},
		{"above notch depth", 25, true, true},
		{"plain large cut", 25, false, false},
		{"plain cut equals height", 40, false, true},
		{"plain cut over height", 50, false, true},
	}
	for _, c := range cases {
		err := ValidateCut(40, c.cut, c.notched)
		if (err != nil) != c.wantFail {
			t.Errorf("%s: err = %v, wantFail=%v", c.name, err, c.wantFail)
		}
	}
}

func TestPanelValidateOutline(t *testing.T) {
	notch := []Notch{{Pos: 10, Width: 3, Edge: EdgeTop}}
	ok := Panel{Length: 100, Height: 30, Cut: 10, Notches: notch}
	if err := ok.ValidateOutline(); err != nil {
		t.Errorf("valid fallback panel: %v", err)
	}
	bad := Panel{Length: 100, Height: 15, Cut: 25, Notches: notch}
	if err := bad.ValidateOutline(); err == nil {
		t.Error("cut >= notch depth accepted on fallback path")
	}
	oob := Panel{Length: 5, Height: 30, Notches: notch}
	if err := oob.ValidateOutline(); err == nil {
		t.Error("out-of-bounds notch accepted on fallback path")
	}
	// A populated Outline is trusted; only edges are checked.
	pre := Panel{Length: 5, Height: 30, Notches: notch, Outline: BuildOutline(100, 30, nil)}
	if err := pre.ValidateOutline(); err != nil {
		t.Errorf("populated outline: %v", err)
	}
}
