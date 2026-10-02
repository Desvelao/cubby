package manifest

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// Anti-rot tests for docs/manifest.md and docs/manifest.schema.json. They
// fail when the schema structs, the reference page and the JSON Schema drift
// apart. The docs directory defaults to the repository's; set
// CUBBY_DOCS_DIR to point at a scratch copy (used to demonstrate failures).

func docsDir(t *testing.T) string {
	t.Helper()
	dir := os.Getenv("CUBBY_DOCS_DIR")
	if dir == "" {
		dir = filepath.Join("..", "..", "..", "docs")
	}
	if _, err := os.Stat(dir); err != nil {
		t.Skipf("docs directory not available: %v", err)
	}
	return dir
}

func readDoc(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(docsDir(t), name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(b)
}

// manifestBlocks returns the body of every ```yaml manifest fenced block.
func manifestBlocks(doc string) []string {
	var blocks []string
	var cur []string
	in := false
	for _, line := range strings.Split(doc, "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case !in && strings.HasPrefix(trimmed, "```yaml manifest"):
			in, cur = true, nil
		case in && strings.HasPrefix(trimmed, "```"):
			blocks = append(blocks, strings.Join(cur, "\n")+"\n")
			in = false
		case in:
			cur = append(cur, line)
		}
	}
	return blocks
}

func TestDocsManifestBlocksAreValid(t *testing.T) {
	doc := readDoc(t, "manifest.md")
	blocks := manifestBlocks(doc)
	if len(blocks) == 0 {
		t.Fatal("docs/manifest.md has no ```yaml manifest blocks")
	}
	for i, b := range blocks {
		path := filepath.Join(t.TempDir(), "block.yaml")
		if err := os.WriteFile(path, []byte(b), 0o600); err != nil {
			t.Fatal(err)
		}
		m, err := Load(path)
		if err != nil {
			t.Errorf("docs/manifest.md manifest block #%d does not load: %v\n%s", i+1, err, b)
			continue
		}
		if res := Validate(m); !res.OK() {
			t.Errorf("docs/manifest.md manifest block #%d has validation issues: %v\n%s", i+1, res.Issues, b)
		}
	}
}

// structTags returns the yaml key names of a struct type, in field order.
func structTags(t reflect.Type) []string {
	var tags []string
	for i := 0; i < t.NumField(); i++ {
		tag := strings.Split(t.Field(i).Tag.Get("yaml"), ",")[0]
		if tag != "" && tag != "-" {
			tags = append(tags, tag)
		}
	}
	return tags
}

// underlyingStruct unwraps pointers and slices down to a struct type, or nil.
func underlyingStruct(t reflect.Type) reflect.Type {
	for t.Kind() == reflect.Ptr || t.Kind() == reflect.Slice {
		t = t.Elem()
	}
	// Types with a custom YAML parser (scalar-or-mapping values) are
	// documented in their field's row, not as a table of their own.
	if reflect.PointerTo(t).Implements(reflect.TypeOf((*yaml.Unmarshaler)(nil)).Elem()) {
		return nil
	}
	if t.Kind() == reflect.Struct {
		return t
	}
	return nil
}

// docSections maps each struct to the heading of its field table in
// docs/manifest.md.
var docSections = map[reflect.Type]string{
	reflect.TypeOf(Manifest{}):   "Top-level keys",
	reflect.TypeOf(Project{}):    "`project`",
	reflect.TypeOf(Box{}):        "`box`",
	reflect.TypeOf(Dimensions{}): "`box.interior`",
	reflect.TypeOf(Material{}):   "`material`",
	reflect.TypeOf(Defaults{}):   "`defaults`",
	reflect.TypeOf(Component{}):  "`components[]`",
	reflect.TypeOf(Group{}):      "`groups[]`",
}

// docSection returns the text under a "### <heading>" line up to the next
// heading of any level.
func docSection(doc, heading string) (string, bool) {
	lines := strings.Split(doc, "\n")
	for i, l := range lines {
		if l != "### "+heading {
			continue
		}
		var b strings.Builder
		for _, n := range lines[i+1:] {
			if strings.HasPrefix(n, "#") {
				break
			}
			b.WriteString(n + "\n")
		}
		return b.String(), true
	}
	return "", false
}

// walkStructs calls fn for t and every struct nested under it, once each.
func walkStructs(t reflect.Type, seen map[reflect.Type]bool, fn func(reflect.Type)) {
	if seen[t] {
		return
	}
	seen[t] = true
	fn(t)
	for i := 0; i < t.NumField(); i++ {
		if s := underlyingStruct(t.Field(i).Type); s != nil {
			walkStructs(s, seen, fn)
		}
	}
}

func TestDocsDocumentEveryField(t *testing.T) {
	doc := readDoc(t, "manifest.md")
	walkStructs(reflect.TypeOf(Manifest{}), map[reflect.Type]bool{}, func(st reflect.Type) {
		heading, ok := docSections[st]
		if !ok {
			t.Errorf("no docs section registered for %s; add it to docSections and docs/manifest.md", st.Name())
			return
		}
		section, ok := docSection(doc, heading)
		if !ok {
			t.Errorf("docs/manifest.md has no \"### %s\" section for %s", heading, st.Name())
			return
		}
		for _, tag := range structTags(st) {
			if !strings.Contains(doc, "`"+tag+"`") {
				t.Errorf("yaml key %q (%s) is not mentioned in backticks in docs/manifest.md", tag, st.Name())
			}
			if !strings.Contains(section, "| `"+tag+"` |") {
				t.Errorf("yaml key %q (%s) has no table row under \"### %s\" in docs/manifest.md", tag, st.Name(), heading)
			}
		}
	})
}

func loadSchema(t *testing.T) map[string]any {
	t.Helper()
	var schema map[string]any
	if err := json.Unmarshal([]byte(readDoc(t, "manifest.schema.json")), &schema); err != nil {
		t.Fatalf("docs/manifest.schema.json is not valid JSON: %v", err)
	}
	if s, _ := schema["$schema"].(string); s == "" {
		t.Error("docs/manifest.schema.json has no $schema")
	}
	return schema
}

func schemaProps(node map[string]any) map[string]any {
	props, _ := node["properties"].(map[string]any)
	return props
}

// schemaChild returns the schema of a property, descending through array items.
func schemaChild(node map[string]any, key string) map[string]any {
	child, _ := schemaProps(node)[key].(map[string]any)
	if child == nil {
		return nil
	}
	if items, ok := child["items"].(map[string]any); ok {
		return items
	}
	return child
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func TestSchemaMatchesStructs(t *testing.T) {
	schema := loadSchema(t)
	var check func(path string, st reflect.Type, node map[string]any)
	check = func(path string, st reflect.Type, node map[string]any) {
		if node == nil {
			t.Errorf("schema has no object at %s (struct %s)", path, st.Name())
			return
		}
		if ap, ok := node["additionalProperties"].(bool); !ok || ap {
			t.Errorf("schema %s must set additionalProperties: false", path)
		}
		want := structTags(st)
		sort.Strings(want)
		got := sortedKeys(schemaProps(node))
		if !reflect.DeepEqual(want, got) {
			t.Errorf("schema %s properties %v do not match struct %s yaml keys %v", path, got, st.Name(), want)
		}
		for i := 0; i < st.NumField(); i++ {
			f := st.Field(i)
			tag := strings.Split(f.Tag.Get("yaml"), ",")[0]
			if s := underlyingStruct(f.Type); s != nil {
				check(path+"."+tag, s, schemaChild(node, tag))
			}
		}
	}
	check("$", reflect.TypeOf(Manifest{}), schema)
}

// checkAgainstSchema reports keys not allowed by the schema and enum values
// outside it, walking decoded YAML alongside the schema.
func checkAgainstSchema(path string, val any, node map[string]any, problems *[]string) {
	if node == nil {
		return
	}
	switch v := val.(type) {
	case map[string]any:
		// Scalar-or-mapping values describe their mapping form under oneOf.
		if alts, ok := node["oneOf"].([]any); ok {
			for _, a := range alts {
				if alt, _ := a.(map[string]any); alt["type"] == "object" {
					node = alt
				}
			}
		}
		props := schemaProps(node)
		for k, child := range v {
			cs, ok := props[k].(map[string]any)
			if !ok {
				*problems = append(*problems, fmt.Sprintf("%s.%s is not allowed by the schema", path, k))
				continue
			}
			checkAgainstSchema(path+"."+k, child, cs, problems)
		}
	case []any:
		items, _ := node["items"].(map[string]any)
		for i, e := range v {
			checkAgainstSchema(fmt.Sprintf("%s[%d]", path, i), e, items, problems)
		}
	case string:
		if enum, ok := node["enum"].([]any); ok {
			found := false
			for _, e := range enum {
				if e == v {
					found = true
				}
			}
			if !found {
				*problems = append(*problems, fmt.Sprintf("%s = %q is not one of %v", path, v, enum))
			}
		}
	}
}

func TestExamplesUseOnlySchemaKeys(t *testing.T) {
	schema := loadSchema(t)
	examples, err := filepath.Glob(filepath.Join("..", "..", "..", "examples", "*.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(examples) == 0 {
		t.Skip("examples directory not available")
	}
	for _, path := range examples {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var doc map[string]any
		if err := yaml.Unmarshal(data, &doc); err != nil {
			t.Errorf("%s: %v", path, err)
			continue
		}
		var problems []string
		checkAgainstSchema("$", doc, schema, &problems)
		for _, p := range problems {
			t.Errorf("%s: %s", filepath.Base(path), p)
		}
	}
}

func TestDocsManifestBlocksUseOnlySchemaKeys(t *testing.T) {
	schema := loadSchema(t)
	for i, b := range manifestBlocks(readDoc(t, "manifest.md")) {
		var doc map[string]any
		if err := yaml.Unmarshal([]byte(b), &doc); err != nil {
			t.Errorf("block #%d: %v", i+1, err)
			continue
		}
		var problems []string
		checkAgainstSchema("$", doc, schema, &problems)
		for _, p := range problems {
			t.Errorf("docs block #%d: %s", i+1, p)
		}
	}
}

func TestTopLevelFillRemainingIsRejected(t *testing.T) {
	path := filepath.Join(t.TempDir(), "m.yaml")
	if err := os.WriteFile(path, []byte("version: 1\nfillRemaining: true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), "fillRemaining") {
		t.Fatalf("top-level fillRemaining should be an unknown field, got %v", err)
	}
}
