package manifest

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestHeightReductionParsing(t *testing.T) {
	tests := []struct {
		in      string
		want    HeightReduction
		wantErr bool
	}{
		{in: "5", want: HeightReduction{Amount: 5}},
		{in: "2.5", want: HeightReduction{Amount: 2.5}},
		{in: `"30%"`, want: HeightReduction{Percent: 30}},
		{in: "{by: 4, panels: [h-1, 'iv-*']}", want: HeightReduction{Amount: 4, Panels: []string{"h-1", "iv-*"}}},
		{in: `{by: "10%"}`, want: HeightReduction{Percent: 10}},
		{in: "{panels: [h-1]}", wantErr: true},
		{in: "{by: 1, extra: 2}", wantErr: true},
		{in: "{by: 1, panels: []}", wantErr: true},
		{in: "{by: 1, panels: null}", wantErr: true},
		{in: "{by: 1, panels:}", wantErr: true},
		{in: "{by: 1, by: 2}", wantErr: true},
		{in: "{by: 1, panels: [a], panels: [b]}", wantErr: true},
		{in: "abc", wantErr: true},
		{in: "[1]", wantErr: true},
	}
	for _, tc := range tests {
		var got HeightReduction
		err := yaml.Unmarshal([]byte(tc.in), &got)
		if tc.wantErr {
			if err == nil {
				t.Errorf("%s: expected an error", tc.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: %v", tc.in, err)
			continue
		}
		if got.Amount != tc.want.Amount || got.Percent != tc.want.Percent || len(got.Panels) != len(tc.want.Panels) {
			t.Errorf("%s: got %+v, want %+v", tc.in, got, tc.want)
		}
	}
}

func TestHeightReductionValidate(t *testing.T) {
	bad := []HeightReduction{{Amount: -1}, {Percent: 100}, {Percent: -5}, {Amount: 1, Panels: []string{" "}}}
	for _, r := range bad {
		if r.Validate(ExternalPanels) == "" {
			t.Errorf("%+v: expected a validation message", r)
		}
	}
	if msg := (HeightReduction{Amount: 3, Percent: 0}).Validate(DividerPanels); msg != "" {
		t.Errorf("unexpected message %q", msg)
	}
}

func TestHeightReductionValidatePatterns(t *testing.T) {
	tests := []struct {
		class ReductionClass
		pat   string
		ok    bool
	}{
		{ExternalPanels, "*", true},
		{DividerPanels, "*", true},
		{ExternalPanels, "w-*", true},
		{ExternalPanels, "wv-3", true},
		{ExternalPanels, "rw-*-front", true},
		{ExternalPanels, "rv-?", true},
		{ExternalPanels, "w*", true},
		{ExternalPanels, "r*", true},
		{ExternalPanels, "[wr]*", true},
		{DividerPanels, "iv-*", true},
		{DividerPanels, "h-?", true},
		{DividerPanels, "ie-v-*", true},
		{DividerPanels, "i*", true},
		{DividerPanels, "v-2", true},
		{DividerPanels, "w-*", false},
		{DividerPanels, "w*", false},
		{ExternalPanels, "iv-*", false},
		{ExternalPanels, "h-1", false},
		{ExternalPanels, "w", false},
		{ExternalPanels, "W-1", false},
		{ExternalPanels, "W-*", false},
		{DividerPanels, "foo-*", false},
		{DividerPanels, "iv", false},
		{DividerPanels, "[", false},
	}
	for _, tc := range tests {
		msg := (HeightReduction{Amount: 1, Panels: []string{tc.pat}}).Validate(tc.class)
		if (msg == "") != tc.ok {
			t.Errorf("class %d pattern %q: message %q, want ok=%v", tc.class, tc.pat, msg, tc.ok)
		}
	}
}

func TestHeightReductionTrimsPanels(t *testing.T) {
	var got HeightReduction
	if err := yaml.Unmarshal([]byte("{by: 1, panels: [' w-1 ', \"iv-*\\t\"]}"), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Panels) != 2 || got.Panels[0] != "w-1" || got.Panels[1] != "iv-*" {
		t.Errorf("panels not trimmed: %q", got.Panels)
	}
}

// reductionManifest returns a loadable manifest whose defaults and first
// group carry the given extra YAML lines (already indented for their block).
func reductionManifest(defaults, group string) string {
	return loadValid + `material: {name: Foamboard, thickness: 3}
components:
  - {id: a, name: A, width: 10, depth: 10, height: 5, qty: 1}
defaults:
` + defaults + `groups:
  - id: g1
    name: G1
    components: [a]
` + group
}

func TestManifestHeightReductionLoadAndValidate(t *testing.T) {
	tests := []struct {
		name     string
		defaults string
		group    string
		loadErr  string // substring of the Load error; empty means Load succeeds
		wantPath string // Issue.Path expected from Validate; empty means valid
	}{
		{name: "defaults ok", defaults: "  externalHeightReduction: 2\n  dividerHeightReduction: \"10%\"\n"},
		{name: "group ok", group: "    externalHeightReduction: {by: 1, panels: [w-*]}\n    dividerHeightReduction: 3\n"},
		{name: "defaults external negative", defaults: "  externalHeightReduction: -1\n", wantPath: "defaults.externalHeightReduction"},
		{name: "defaults divider negative", defaults: "  dividerHeightReduction: -0.5\n", wantPath: "defaults.dividerHeightReduction"},
		{name: "defaults external bad panel class", defaults: "  externalHeightReduction: {by: 1, panels: [h-1]}\n", wantPath: "defaults.externalHeightReduction"},
		{name: "defaults divider bad panel class", defaults: "  dividerHeightReduction: {by: 1, panels: [w-1]}\n", wantPath: "defaults.dividerHeightReduction"},
		{name: "defaults divider bad glob", defaults: "  dividerHeightReduction: {by: 1, panels: ['h-[']}\n", wantPath: "defaults.dividerHeightReduction"},
		{name: "defaults blank panel id", defaults: "  externalHeightReduction: {by: 1, panels: ['  ']}\n", wantPath: "defaults.externalHeightReduction"},
		{name: "group external negative", group: "    externalHeightReduction: -2\n", wantPath: "groups[0].externalHeightReduction"},
		{name: "group divider percent 100", group: "    dividerHeightReduction: \"100%\"\n", wantPath: "groups[0].dividerHeightReduction"},
		{name: "group divider bad panel class", group: "    dividerHeightReduction: {by: 1, panels: [rw-1]}\n", wantPath: "groups[0].dividerHeightReduction"},
		{name: "group external bad panel class", group: "    externalHeightReduction: {by: 1, panels: [iv-1]}\n", wantPath: "groups[0].externalHeightReduction"},
		{name: "percent 99.99 accepted", defaults: "  externalHeightReduction: \"99.99%\"\n"},
		{name: "percent 100 rejected", defaults: "  externalHeightReduction: \"100%\"\n", wantPath: "defaults.externalHeightReduction"},
		{name: "percent 99.99 accepted in group", group: "    dividerHeightReduction: {by: \"99.99%\"}\n"},
		{name: "percent over 100 rejected", defaults: "  dividerHeightReduction: \"150%\"\n", wantPath: "defaults.dividerHeightReduction"},
		{name: "inf percent rejected", defaults: "  externalHeightReduction: \"inf%\"\n", wantPath: "defaults.externalHeightReduction"},
		{name: "NaN percent rejected", defaults: "  externalHeightReduction: \"NaN%\"\n", wantPath: "defaults.externalHeightReduction"},
		{name: "inf amount rejected", defaults: "  dividerHeightReduction: inf\n", wantPath: "defaults.dividerHeightReduction"},
		{name: "NaN amount rejected", group: "    dividerHeightReduction: NaN\n", wantPath: "groups[0].dividerHeightReduction"},
		{name: "group inf percent rejected", group: "    externalHeightReduction: \"-inf%\"\n", wantPath: "groups[0].externalHeightReduction"},
		{name: "null defaults is zero", defaults: "  externalHeightReduction: null\n  dividerHeightReduction: ~\n"},
		{name: "null group is unset", group: "    externalHeightReduction: null\n"},
		{name: "empty panels list", defaults: "  externalHeightReduction: {by: 1, panels: []}\n", loadErr: "panels must not be empty"},
		{name: "empty panels list in group", group: "    dividerHeightReduction: {by: 1, panels: []}\n", loadErr: "panels must not be empty"},
		{name: "non-list panels", defaults: "  externalHeightReduction: {by: 1, panels: w-1}\n", loadErr: "panels must be a list"},
		{name: "mapping panels", group: "    dividerHeightReduction: {by: 1, panels: {a: b}}\n", loadErr: "panels must be a list"},
		{name: "duplicate by", defaults: "  externalHeightReduction: {by: 1, by: 2}\n", loadErr: "duplicate key"},
		{name: "duplicate panels in group", group: "    externalHeightReduction: {by: 1, panels: [w-1], panels: [w-2]}\n", loadErr: "duplicate key"},
		{name: "missing by", defaults: "  dividerHeightReduction: {panels: [h-1]}\n", loadErr: "missing required key"},
		{name: "unparsable amount", defaults: "  dividerHeightReduction: abc\n", loadErr: "invalid reduction"},
		{name: "unparsable percent", group: "    dividerHeightReduction: \"x%\"\n", loadErr: "invalid percentage"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m, err := Load(writeTemp(t, reductionManifest(tc.defaults, tc.group)))
			if tc.loadErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.loadErr) {
					t.Fatalf("Load error = %v, want containing %q", err, tc.loadErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			res := Validate(m)
			if tc.wantPath == "" {
				if !res.OK() {
					t.Fatalf("unexpected issues: %+v", res.Issues)
				}
				return
			}
			if len(res.Issues) != 1 || res.Issues[0].Path != tc.wantPath {
				t.Fatalf("issues = %+v, want exactly one at %q", res.Issues, tc.wantPath)
			}
		})
	}
}

func TestManifestHeightReductionNullLeavesUnset(t *testing.T) {
	m, err := Load(writeTemp(t, reductionManifest(
		"  externalHeightReduction: null\n", "    externalHeightReduction: null\n")))
	if err != nil {
		t.Fatal(err)
	}
	if !m.Defaults.ExternalHeightReduction.IsZero() {
		t.Errorf("defaults = %+v, want zero", m.Defaults.ExternalHeightReduction)
	}
	if m.Groups[0].ExternalHeightReduction != nil {
		t.Errorf("group = %+v, want nil (inherit defaults)", m.Groups[0].ExternalHeightReduction)
	}
}

func TestManifestHeightReductionValidatesEveryGroup(t *testing.T) {
	m := validManifest()
	bad := HeightReduction{Amount: -1}
	m.Groups = append(m.Groups, Group{ID: "g2", Name: "G2", Components: []string{"b"}, DividerHeightReduction: &bad})
	res := Validate(m)
	if len(res.Issues) != 1 || res.Issues[0].Path != "groups[1].dividerHeightReduction" {
		t.Fatalf("issues = %+v", res.Issues)
	}
}
