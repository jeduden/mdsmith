package parser

import "testing"

// TestTagInAllowedSet_CaseInsensitive mirrors the case-insensitive lookup
// strings.ToLower(string(b)) used to perform, without allocating.
func TestTagInAllowedSet_CaseInsensitive(t *testing.T) {
	cases := []struct {
		tag  string
		want bool
	}{
		{"div", true},
		{"DIV", true},
		{"DiV", true},
		{"blockquote", true},
		{"notarealtag", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := tagInAllowedSet([]byte(tc.tag)); got != tc.want {
			t.Errorf("tagInAllowedSet(%q) = %v; want %v", tc.tag, got, tc.want)
		}
	}
}

// TestIsRawTextTag_CaseInsensitive mirrors the tagName != "script" &&
// tagName != "style" && tagName != "pre" checks the type-7 opener used to
// perform against a lowercased string.
func TestIsRawTextTag_CaseInsensitive(t *testing.T) {
	cases := []struct {
		tag  string
		want bool
	}{
		{"script", true},
		{"SCRIPT", true},
		{"Style", true},
		{"pre", true},
		{"div", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := isRawTextTag([]byte(tc.tag)); got != tc.want {
			t.Errorf("isRawTextTag(%q) = %v; want %v", tc.tag, got, tc.want)
		}
	}
}

// TestTagLookups_ZeroAlloc pins the allocation-free lookup path
// (docs/development/high-performance-go.md): strings.ToLower(string(b))
// allocated a new string on every HTML-block trigger candidate line;
// lowering into a stack buffer removes it.
func TestTagLookups_ZeroAlloc(t *testing.T) {
	b := []byte("BLOCKQUOTE")
	allocs := testing.AllocsPerRun(100, func() {
		_ = tagInAllowedSet(b)
		_ = isRawTextTag(b)
	})
	if allocs != 0 {
		t.Errorf("tagInAllowedSet+isRawTextTag allocs/op = %.0f; want 0", allocs)
	}
}
