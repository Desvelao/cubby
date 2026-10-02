package cli

import "testing"

func TestSlugify(t *testing.T) {
	tests := []struct{ in, want string }{
		{"Two Compartment Box", "two-compartment-box"},
		{"  Hello,   World!  ", "hello-world"},
		{"already-slug", "already-slug"},
		{"Ünï Box 2", "ünï-box-2"},
		{"A/B\\C", "a-b-c"},
		{"", "insert"},
		{"!!!", "insert"},
		{"Catán", "catán"},
		{"日本語", "日本語"},
		{"Café Müller 2", "café-müller-2"},
		{"🎲🎲", "insert"},
		{"🎲 Catán 🎲", "catán"},
		{"Cafe\u0301 Box", "cafe\u0301-box"},
		{"ÉCOLE", "école"},
		{"a\xffb", "a-b"},
	}
	for _, tc := range tests {
		if got := slugify(tc.in); got != tc.want {
			t.Errorf("slugify(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestSlugifyNonASCIINamesDoNotCollide(t *testing.T) {
	names := []string{"Catán", "Catan", "日本語", "한국어", "Ünï Box", "Ön Box", "🎲"}
	seen := map[string]string{}
	for _, n := range names {
		got := slugify(n)
		if prev, ok := seen[got]; ok && got != "insert" {
			t.Errorf("slugify(%q) and slugify(%q) both = %q", prev, n, got)
		}
		seen[got] = n
	}
	if slugify("日本語") == slugify("한국어") {
		t.Error("distinct CJK names collapsed to the same stem")
	}
}
