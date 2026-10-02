package export

import "strings"

// XMLText returns s with every character outside the XML 1.0 Char range
// (#x9 | #xA | #xD | #x20-#xD7FF | #xE000-#xFFFD | #x10000-#x10FFFF) removed,
// so manifest-derived strings cannot make an XML/SVG document ill-formed.
// Offending characters are dropped rather than replaced, matching the STEP
// exporter's handling of control characters. Invalid UTF-8 bytes decode as
// U+FFFD (a valid XML character) and are kept as such. It does
// not escape markup; callers still escape &, <, > (and quotes in attributes).
func XMLText(s string) string {
	valid := true
	for _, r := range s {
		if !isXMLChar(r) {
			valid = false
			break
		}
	}
	if valid {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if isXMLChar(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func isXMLChar(r rune) bool {
	return r == 0x9 || r == 0xA || r == 0xD ||
		(r >= 0x20 && r <= 0xD7FF) ||
		(r >= 0xE000 && r <= 0xFFFD) ||
		(r >= 0x10000 && r <= 0x10FFFF)
}
