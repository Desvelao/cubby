package manifest

import (
	"math"
	"strings"
	"testing"
)

func validManifest() *Manifest {
	return &Manifest{
		Version: 1,
		Box: Box{
			Name: "Box", Units: UnitsMM,
			Interior: Dimensions{Width: 100, Depth: 100, Height: 50},
		},
		Material: Material{Name: "Foamboard", Thickness: 3, Kerf: 0},
		Components: []Component{
			{ID: "a", Name: "A", Width: 10, Depth: 10, Height: 5, Qty: 1},
			{ID: "b", Name: "B", Width: 10, Depth: 10, Height: 5, Qty: 1},
		},
		Groups: []Group{
			{ID: "g1", Name: "G1", Components: []string{"a"}},
		},
	}
}

func TestValidateOK(t *testing.T) {
	res := Validate(validManifest())
	if !res.OK() {
		t.Fatalf("expected valid manifest, got issues: %+v", res.Issues)
	}
}

func TestValidateDuplicateComponentID(t *testing.T) {
	m := validManifest()
	m.Components[1].ID = "a"
	res := Validate(m)
	if res.OK() {
		t.Fatal("expected duplicate id to be rejected")
	}
}

func TestValidateUnknownGroupReference(t *testing.T) {
	m := validManifest()
	m.Groups[0].Components = []string{"nope"}
	res := Validate(m)
	if res.OK() {
		t.Fatal("expected unknown component reference to be rejected")
	}
}

func TestValidateComponentInMultipleGroups(t *testing.T) {
	m := validManifest()
	m.Groups = append(m.Groups, Group{ID: "g2", Components: []string{"a"}})
	res := Validate(m)
	if res.OK() {
		t.Fatal("expected component-in-two-groups to be rejected")
	}
}

func TestValidateComponentRepeatedInGroup(t *testing.T) {
	m := validManifest()
	m.Groups[0].Components = []string{"a", "b", "a"}
	res := Validate(m)
	if res.OK() {
		t.Fatal("expected repeated component in one group to be rejected")
	}
	if len(res.Issues) != 1 || res.Issues[0].Path != "groups[0].components[2]" ||
		!strings.Contains(res.Issues[0].Message, "more than once") {
		t.Errorf("unexpected issues: %+v", res.Issues)
	}
}

func TestValidateComponentInMultipleGroupsPath(t *testing.T) {
	m := validManifest()
	m.Groups = append(m.Groups, Group{ID: "g2", Components: []string{"a"}})
	res := Validate(m)
	if len(res.Issues) != 1 || res.Issues[0].Path != "groups[1].components[0]" ||
		!strings.Contains(res.Issues[0].Message, "already belongs to group \"g1\"") {
		t.Errorf("unexpected issues: %+v", res.Issues)
	}
}

func TestValidateNonPositiveDimensions(t *testing.T) {
	m := validManifest()
	m.Components[0].Width = 0
	res := Validate(m)
	if res.OK() {
		t.Fatal("expected zero width to be rejected")
	}
}

func TestValidateUnknownDefaultsJointType(t *testing.T) {
	m := validManifest()
	m.Defaults.JointType = JointType("diagonal")
	res := Validate(m)
	if res.OK() {
		t.Fatal("expected unknown defaults.jointType to be rejected")
	}
}

func TestValidateUnknownGroupJointType(t *testing.T) {
	m := validManifest()
	bad := JointType("diagonal")
	m.Groups[0].JointType = &bad
	res := Validate(m)
	if res.OK() {
		t.Fatal("expected unknown groups[].jointType to be rejected")
	}
}

func TestValidateJointTypePlainAccepted(t *testing.T) {
	m := validManifest()
	m.Defaults.JointType = JointTypePlain
	plain := JointTypeNotch
	m.Groups[0].JointType = &plain
	res := Validate(m)
	if !res.OK() {
		t.Fatalf("expected notch/plain jointType values to be accepted, got issues: %+v", res.Issues)
	}
}

func TestUnitsConversion(t *testing.T) {
	d := Dimensions{Width: 1, Depth: 2, Height: 3}
	mm, err := d.ToMM(UnitsCM)
	if err != nil {
		t.Fatal(err)
	}
	if mm.Width != 10 || mm.Depth != 20 || mm.Height != 30 {
		t.Fatalf("unexpected cm->mm conversion: %+v", mm)
	}
	in, err := ScalarToMM(1, UnitsIN)
	if err != nil {
		t.Fatal(err)
	}
	if in != 25.4 {
		t.Fatalf("expected 25.4mm per inch, got %v", in)
	}
}

func TestValidateNewChecks(t *testing.T) {
	nan, inf := math.NaN(), math.Inf(1)
	tr, fl := true, false
	cases := []struct {
		name string
		mod  func(m *Manifest)
		path string // expected issue path; "" means must be valid
	}{
		{"nan interior", func(m *Manifest) { m.Box.Interior.Width = nan }, "box.interior.width"},
		{"inf interior", func(m *Manifest) { m.Box.Interior.Height = inf }, "box.interior.height"},
		{"nan component", func(m *Manifest) { m.Components[0].Depth = nan }, "components[0].depth"},
		{"inf component", func(m *Manifest) { m.Components[1].Width = inf }, "components[1].width"},
		{"nan thickness", func(m *Manifest) { m.Material.Thickness = nan }, "material.thickness"},
		{"inf kerf", func(m *Manifest) { m.Material.Kerf = inf }, "material.kerf"},
		{"kerf equals thickness", func(m *Manifest) { m.Material.Kerf = m.Material.Thickness }, "material.kerf"},
		{"kerf above thickness", func(m *Manifest) { m.Material.Kerf = 3 * m.Material.Thickness }, "material.kerf"},
		{"kerf just below thickness", func(m *Manifest) { m.Material.Kerf = m.Material.Thickness - 0.1 }, ""},
		{"nan padding", func(m *Manifest) { m.Defaults.Padding = nan }, "defaults.padding"},
		{"inf margin", func(m *Manifest) { m.Defaults.Margin = inf }, "defaults.margin"},
		{"nan group padding", func(m *Manifest) { p := nan; m.Groups[0].Padding = &p }, "groups[0].padding"},
		{"inf group margin", func(m *Manifest) { p := inf; m.Groups[0].Margin = &p }, "groups[0].margin"},
		{"finite ok", func(m *Manifest) { p := 1.5; m.Groups[0].Padding = &p }, ""},

		{"auto id collision", func(m *Manifest) { m.Groups[0].ID = "auto-1" }, "groups[0].id"},
		{"auto id large", func(m *Manifest) { m.Groups[0].ID = "auto-42" }, "groups[0].id"},
		{"auto-x ok", func(m *Manifest) { m.Groups[0].ID = "auto-x" }, ""},
		{"auto-0 ok", func(m *Manifest) { m.Groups[0].ID = "auto-0" }, ""},
		{"auto-01 ok", func(m *Manifest) { m.Groups[0].ID = "auto-01" }, ""},
		{"auto ok", func(m *Manifest) { m.Groups[0].ID = "auto" }, ""},

		{"mixed removable", func(m *Manifest) {
			m.Groups = append(m.Groups, Group{ID: "g2", Components: []string{"b"}, Removable: &tr})
		}, "groups[1].removable"},
		{"mixed via defaults", func(m *Manifest) {
			m.Defaults.Removable = true
			m.Groups = append(m.Groups, Group{ID: "g2", Components: []string{"b"}, Removable: &fl})
		}, "groups[1].removable"},
		{"mixed with ungrouped via defaults", func(m *Manifest) {
			m.Groups[0].Removable = &tr
		}, "defaults.removable"},
		{"ungrouped defaults match removable groups ok", func(m *Manifest) {
			m.Defaults.Removable = true
			m.Groups[0].Removable = &tr
		}, ""},
		{"removable groups fully grouped ok", func(m *Manifest) {
			m.Groups[0].Removable = &tr
			m.Groups[0].Components = []string{"a", "b"}
		}, ""},
		{"all removable ok", func(m *Manifest) {
			m.Groups[0].Removable = &tr
			m.Groups = append(m.Groups, Group{ID: "g2", Components: []string{"b"}, Removable: &tr})
		}, ""},
		{"none removable ok", func(m *Manifest) {
			m.Groups = append(m.Groups, Group{ID: "g2", Components: []string{"b"}})
		}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := validManifest()
			tc.mod(m)
			res := Validate(m)
			if tc.path == "" {
				if !res.OK() {
					t.Fatalf("expected valid, got %+v", res.Issues)
				}
				return
			}
			for _, is := range res.Issues {
				if is.Path == tc.path {
					return
				}
			}
			t.Fatalf("expected issue at %q, got %+v", tc.path, res.Issues)
		})
	}
}

func TestAutoCompartmentID(t *testing.T) {
	if got := AutoCompartmentID(3); got != "auto-3" || !IsAutoCompartmentID(got) {
		t.Fatalf("unexpected auto id %q", got)
	}
}

func TestValidateFeasibility(t *testing.T) {
	f := func(v float64) *float64 { return &v }
	b := func(v bool) *bool { return &v }
	tests := []struct {
		name     string
		mutate   func(m *Manifest)
		wantPath string // empty: must validate OK
	}{
		{"baseline ok", func(m *Manifest) {}, ""},
		{"defaults margin too large", func(m *Manifest) { m.Defaults.Margin = 50 }, "defaults.margin"},
		{"defaults margin just fits", func(m *Manifest) { m.Defaults.Margin = 49.9 }, ""},
		{"defaults margin applies to ungrouped without groups", func(m *Manifest) { m.Groups = nil; m.Defaults.Margin = 60 }, "defaults.margin"},
		{"defaults margin checked for ungrouped when groups override margin", func(m *Manifest) { m.Defaults.Margin = 60; m.Groups[0].Margin = f(1) }, "defaults.margin"},
		{"defaults margin unused when every component is grouped", func(m *Manifest) {
			m.Defaults.Margin = 60
			m.Groups[0].Margin = f(1)
			m.Groups[0].Components = []string{"a", "b"}
		}, ""},
		{"removable wall counts for ungrouped defaults", func(m *Manifest) {
			m.Defaults.Removable = true
			m.Defaults.Margin = 47
			m.Groups[0].Margin = f(1)
		}, "defaults.margin"},
		{"group margin too large", func(m *Manifest) { m.Groups[0].Margin = f(60) }, "groups[0].margin"},
		{"group margin overrides bad default", func(m *Manifest) {
			m.Defaults.Margin = 60
			m.Groups[0].Margin = f(1)
			m.Groups[0].Components = []string{"a", "b"}
		}, ""},
		{"removable wall counts toward inset", func(m *Manifest) { m.Defaults.Removable = true; m.Defaults.Margin = 47 }, "defaults.margin"},
		{"non-removable wall does not count", func(m *Manifest) { m.Defaults.Margin = 47 }, ""},
		{"padding is not an inset", func(m *Manifest) { m.Defaults.Padding = 500 }, ""},
		{"thickness >= width", func(m *Manifest) { m.Material.Thickness = 100 }, "material.thickness"},
		{"thickness >= depth", func(m *Manifest) { m.Box.Interior.Depth = 3 }, "material.thickness"},
		{"thickness >= height without floor ok", func(m *Manifest) { m.Box.Interior.Height = 3 }, ""},
		{"thickness >= height with default floor", func(m *Manifest) { m.Box.Interior.Height = 3; m.Defaults.Floor = true }, "material.thickness"},
		{"thickness >= height with group floor", func(m *Manifest) { m.Box.Interior.Height = 3; m.Groups[0].Floor = b(true) }, "material.thickness"},
		{"group floor false ok", func(m *Manifest) { m.Box.Interior.Height = 3; m.Groups[0].Floor = b(false) }, ""},
		{"default floor overridden false by every group, none ungrouped", func(m *Manifest) {
			m.Box.Interior.Height = 3
			m.Defaults.Floor = true
			m.Groups[0].Floor = b(false)
			m.Groups[0].Components = []string{"a", "b"}
		}, ""},
		{"default floor still applies to ungrouped components", func(m *Manifest) {
			m.Box.Interior.Height = 3
			m.Defaults.Floor = true
			m.Groups[0].Floor = b(false)
		}, "material.thickness"},
		{"floor thickness fits", func(m *Manifest) { m.Box.Interior.Height = 3.5; m.Defaults.Floor = true }, ""},
		{"cm: thickness 0.3cm fits 10cm", func(m *Manifest) {
			m.Box = Box{Name: "B", Units: UnitsCM, Interior: Dimensions{Width: 10, Depth: 10, Height: 5}}
			m.Material.Thickness = 0.3
		}, ""},
		{"cm: thickness 10 = 100mm too big", func(m *Manifest) {
			m.Box = Box{Name: "B", Units: UnitsCM, Interior: Dimensions{Width: 10, Depth: 10, Height: 5}}
			m.Material.Thickness = 10
		}, "material.thickness"},
		{"cm: margin 5 = 50mm too big", func(m *Manifest) {
			m.Box = Box{Name: "B", Units: UnitsCM, Interior: Dimensions{Width: 10, Depth: 10, Height: 5}}
			m.Material.Thickness = 0.3
			m.Defaults.Margin = 5
		}, "defaults.margin"},
		{"in: margin 2in = 50.8mm too big for 100mm", func(m *Manifest) {
			m.Box = Box{Name: "B", Units: UnitsIN, Interior: Dimensions{Width: 3.937, Depth: 3.937, Height: 2}}
			m.Material.Thickness = 0.1
			m.Groups[0].Margin = f(2)
		}, "groups[0].margin"},
		{"in: margin 1in fits", func(m *Manifest) {
			m.Box = Box{Name: "B", Units: UnitsIN, Interior: Dimensions{Width: 3.937, Depth: 3.937, Height: 2}}
			m.Material.Thickness = 0.1
			m.Groups[0].Margin = f(1)
		}, ""},
		{"component too big is not a validation error", func(m *Manifest) { m.Components[0].Width = 500 }, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := validManifest()
			tt.mutate(m)
			res := Validate(m)
			if tt.wantPath == "" {
				if !res.OK() {
					t.Fatalf("expected OK, got %+v", res.Issues)
				}
				return
			}
			if len(res.Issues) != 1 || res.Issues[0].Path != tt.wantPath {
				t.Fatalf("expected one issue at %s, got %+v", tt.wantPath, res.Issues)
			}
		})
	}
}

func TestValidateProject(t *testing.T) {
	long := func(n int) string { return strings.Repeat("x", n) }
	tags := func(n int) []string {
		out := make([]string, n)
		for i := range out {
			out[i] = "t"
		}
		return out
	}
	tests := []struct {
		name     string
		project  Project
		wantPath string // empty means valid
		wantMsg  string
	}{
		{"absent", Project{}, "", ""},
		{"full", Project{
			Name: "Insert", Description: "Line one\n\tline two", Revision: "1.2.0", Author: "José Ñandú — 東",
			Contact: "jane@example.com", License: "CC-BY-4.0", URL: "https://example.com/x?a=1",
			Tags: []string{"a", "b"}, Created: "2026-09-30", Updated: "2026-09-30", Notes: "n\nm",
			Game: "G", Publisher: "P", Edition: "E",
		}, "", ""},

		{"name at limit", Project{Name: long(120)}, "", ""},
		{"name too long", Project{Name: long(121)}, "project.name", "at most 120"},
		{"name counts runes", Project{Name: strings.Repeat("東", 120)}, "", ""},
		{"name newline", Project{Name: "a\nb"}, "project.name", "control character"},
		{"name tab", Project{Name: "a\tb"}, "project.name", "control character"},
		{"description at limit", Project{Description: long(1000)}, "", ""},
		{"description too long", Project{Description: long(1001)}, "project.description", "at most 1000"},
		{"description control", Project{Description: "a\x00b"}, "project.description", "control character"},
		{"description escape", Project{Description: "a\x1bb"}, "project.description", "control character"},
		{"description CR", Project{Description: "a\rb"}, "project.description", "control character"},
		{"notes at limit", Project{Notes: long(2000)}, "", ""},
		{"notes too long", Project{Notes: long(2001)}, "project.notes", "at most 2000"},
		{"notes control", Project{Notes: "a\x7fb"}, "project.notes", "control character"},
		{"notes noncharacter", Project{Notes: "a￿b"}, "project.notes", "control character"},
		{"author at limit", Project{Author: long(200)}, "", ""},
		{"author too long", Project{Author: long(201)}, "project.author", "at most 200"},
		{"author control", Project{Author: "a\nb"}, "project.author", "control character"},
		{"license too long", Project{License: long(201)}, "project.license", "at most 200"},
		{"game too long", Project{Game: long(201)}, "project.game", "at most 200"},
		{"publisher too long", Project{Publisher: long(201)}, "project.publisher", "at most 200"},
		{"edition too long", Project{Edition: long(201)}, "project.edition", "at most 200"},
		{"contact too long", Project{Contact: long(201)}, "project.contact", "at most 200"},
		{"revision too long", Project{Revision: long(201)}, "project.revision", "at most 200"},
		{"revision blank", Project{Revision: "   "}, "project.revision", "must not be blank"},
		{"revision ok", Project{Revision: "v2 (draft)"}, "", ""},

		{"contact free text", Project{Contact: "the Discord server"}, "", ""},
		{"contact email", Project{Contact: "a.b+c@sub.example.org"}, "", ""},
		{"contact named email", Project{Contact: "Jane <jane@example.com>"}, "", ""},
		{"contact bad email", Project{Contact: "jane@"}, "project.contact", "valid email"},
		{"contact no domain dot", Project{Contact: "jane@localhost"}, "project.contact", "valid email"},
		{"contact two ats", Project{Contact: "a@b@example.com"}, "project.contact", "valid email"},
		{"contact spaces", Project{Contact: "call me @ home"}, "", ""},
		{"contact handle", Project{Contact: "@jane"}, "", ""},
		{"contact fediverse handle", Project{Contact: "@jane@fosstodon.org"}, "", ""},
		{"contact url with at", Project{Contact: "https://medium.com/@jane"}, "", ""},
		{"contact bad named email", Project{Contact: "Jane <jane@localhost>"}, "project.contact", "valid email"},

		{"license free text", Project{License: "Custom terms, see URL"}, "", ""},
		{"license spdx", Project{License: "MIT OR Apache-2.0"}, "", ""},

		{"url http", Project{URL: "http://example.com"}, "", ""},
		{"url ftp", Project{URL: "ftp://example.com/x"}, "project.url", "http or https"},
		{"url no scheme", Project{URL: "example.com/x"}, "project.url", "http or https"},
		{"url javascript", Project{URL: "javascript:alert(1)"}, "project.url", "http or https"},
		{"url no host", Project{URL: "https:///path"}, "project.url", "host"},
		{"url unparsable", Project{URL: "http://exa mple.com"}, "project.url", "valid URL"},

		{"created ok", Project{Created: "2024-02-29"}, "", ""},
		{"created bad leap", Project{Created: "2026-02-29"}, "project.created", "YYYY-MM-DD"},
		{"created bad month", Project{Created: "2026-13-01"}, "project.created", "YYYY-MM-DD"},
		{"created unpadded", Project{Created: "2026-9-3"}, "project.created", "YYYY-MM-DD"},
		{"created datetime", Project{Created: "2026-09-30T10:00:00Z"}, "project.created", "YYYY-MM-DD"},
		{"created words", Project{Created: "yesterday"}, "project.created", "YYYY-MM-DD"},
		{"updated bad", Project{Updated: "30/09/2026"}, "project.updated", "YYYY-MM-DD"},
		{"updated after created", Project{Created: "2026-01-01", Updated: "2026-01-02"}, "", ""},
		{"updated before created", Project{Created: "2026-01-02", Updated: "2026-01-01"}, "project.updated", "before project.created"},
		{"updated only", Project{Updated: "2020-01-01"}, "", ""},
		{"bad created skips order check", Project{Created: "nope", Updated: "2020-01-01"}, "project.created", "YYYY-MM-DD"},

		{"tags at limit", Project{Tags: tags(20)}, "", ""},
		{"too many tags", Project{Tags: tags(21)}, "project.tags", "at most 20"},
		{"tag at limit", Project{Tags: []string{long(50)}}, "", ""},
		{"tag too long", Project{Tags: []string{"ok", long(51)}}, "project.tags[1]", "at most 50"},
		{"tag empty", Project{Tags: []string{"ok", ""}}, "project.tags[1]", "must not be empty"},
		{"tag blank", Project{Tags: []string{" "}}, "project.tags[0]", "must not be empty"},
		{"tag control", Project{Tags: []string{"a\tb"}}, "project.tags[0]", "control character"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := validManifest()
			m.Project = tc.project
			res := Validate(m)
			if tc.wantPath == "" {
				if !res.OK() {
					t.Fatalf("expected valid, got %+v", res.Issues)
				}
				return
			}
			for _, is := range res.Issues {
				if is.Path == tc.wantPath && strings.Contains(is.Message, tc.wantMsg) {
					return
				}
			}
			t.Fatalf("want issue %s containing %q, got %+v", tc.wantPath, tc.wantMsg, res.Issues)
		})
	}
}

func TestValidateReductionFeasibility(t *testing.T) {
	b := func(v bool) *bool { return &v }
	red := func(v float64) *HeightReduction { return &HeightReduction{Amount: v} }
	tests := []struct {
		name     string
		mutate   func(m *Manifest)
		wantPath string // empty: must validate OK
	}{
		{"default divider reduction below height", func(m *Manifest) { m.Defaults.DividerHeightReduction = HeightReduction{Amount: 49.9} }, ""},
		{"default divider reduction equals height", func(m *Manifest) { m.Defaults.DividerHeightReduction = HeightReduction{Amount: 50} }, "defaults.dividerHeightReduction"},
		{"default external reduction on removable trays", func(m *Manifest) {
			m.Defaults.Removable = true
			m.Defaults.ExternalHeightReduction = HeightReduction{Amount: 500}
		}, "defaults.externalHeightReduction"},
		{"external reduction ignored without removable or fullWalls", func(m *Manifest) { m.Defaults.ExternalHeightReduction = HeightReduction{Amount: 500} }, ""},
		{"external reduction applies with fullWalls", func(m *Manifest) {
			m.Defaults.FullWalls = true
			m.Defaults.ExternalHeightReduction = HeightReduction{Amount: 500}
		}, "defaults.externalHeightReduction"},
		{"group divider reduction too large", func(m *Manifest) { m.Groups[0].DividerHeightReduction = red(500) }, "groups[0].dividerHeightReduction"},
		{"group external reduction too large", func(m *Manifest) {
			m.Groups[0].Removable = b(true)
			m.Defaults.Removable = true
			m.Groups[0].ExternalHeightReduction = red(60)
		}, "groups[0].externalHeightReduction"},
		{"floor lowers the base for a removable tray", func(m *Manifest) {
			m.Defaults.Removable = true
			m.Defaults.Floor = true
			m.Groups[0].ExternalHeightReduction = red(47)
		}, "groups[0].externalHeightReduction"},
		{"reduction fits above the floor", func(m *Manifest) {
			m.Defaults.Removable = true
			m.Defaults.Floor = true
			m.Groups[0].ExternalHeightReduction = red(46.9)
		}, ""},
		{"floor on another tray lowers the shared grid", func(m *Manifest) {
			m.Groups[0].Floor = b(true)
			m.Defaults.DividerHeightReduction = HeightReduction{Amount: 48}
		}, "defaults.dividerHeightReduction"},
		{"floor in a removable tray does not lower other trays", func(m *Manifest) {
			m.Defaults.Removable = true
			m.Groups[0].Floor = b(true)
			m.Defaults.DividerHeightReduction = HeightReduction{Amount: 48}
		}, ""},
		{"default skipped when every user overrides it", func(m *Manifest) {
			m.Groups[0].Components = []string{"a", "b"}
			m.Groups[0].DividerHeightReduction = red(1)
			m.Defaults.DividerHeightReduction = HeightReduction{Amount: 500}
		}, ""},
		{"panel-limited reduction is skipped", func(m *Manifest) {
			m.Defaults.DividerHeightReduction = HeightReduction{Amount: 500, Panels: []string{"h-9"}}
		}, ""},
		{"percentage is skipped", func(m *Manifest) { m.Defaults.DividerHeightReduction = HeightReduction{Percent: 99} }, ""},
		{"cm units converted to mm", func(m *Manifest) {
			m.Box = Box{Name: "B", Units: UnitsCM, Interior: Dimensions{Width: 10, Depth: 10, Height: 5}}
			m.Material.Thickness = 0.3
			m.Defaults.DividerHeightReduction = HeightReduction{Amount: 5}
		}, "defaults.dividerHeightReduction"},
		{"cm units below height", func(m *Manifest) {
			m.Box = Box{Name: "B", Units: UnitsCM, Interior: Dimensions{Width: 10, Depth: 10, Height: 5}}
			m.Material.Thickness = 0.3
			m.Defaults.DividerHeightReduction = HeightReduction{Amount: 4.9}
		}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := validManifest()
			tt.mutate(m)
			res := Validate(m)
			if tt.wantPath == "" {
				if !res.OK() {
					t.Fatalf("expected valid, got %+v", res.Issues)
				}
				return
			}
			if len(res.Issues) != 1 || res.Issues[0].Path != tt.wantPath {
				t.Fatalf("expected one issue at %q, got %+v", tt.wantPath, res.Issues)
			}
		})
	}
}
