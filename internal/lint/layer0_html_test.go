package lint

import "testing"

// TestTagInAllowedSet verifies that tagInAllowedSet recognises the CommonMark
// type-6 HTML block tag set and rejects non-block tags. The test protects
// against regressions when allowedBlockTags is changed from map[string]bool
// to map[string]struct{}: the boolean return value of tagInAllowedSet must be
// identical.
func TestTagInAllowedSet_KnownTags(t *testing.T) {
	known := []string{
		"div", "p", "ul", "ol", "table", "thead", "tbody", "tr", "td", "th",
		"blockquote", "aside", "article", "section", "header", "footer",
		"nav", "main", "address", "dl", "dt", "dd", "figure", "figcaption",
		"h1", "h2", "h3", "h4", "h5", "h6", "hr", "html", "body", "form", "li",
	}
	for _, tag := range known {
		if !tagInAllowedSet([]byte(tag)) {
			t.Errorf("tagInAllowedSet(%q) = false; want true", tag)
		}
		// Case-insensitive: upper-case input must also match.
		upper := make([]byte, len(tag))
		for i, c := range []byte(tag) {
			if c >= 'a' && c <= 'z' {
				upper[i] = c - ('a' - 'A')
			} else {
				upper[i] = c
			}
		}
		if !tagInAllowedSet(upper) {
			t.Errorf("tagInAllowedSet(%q) = false; want true (case-insensitive)", upper)
		}
	}
}

func TestTagInAllowedSet_UnknownTags(t *testing.T) {
	notAllowed := []string{"a", "span", "em", "strong", "code", "img", "br", "nothtml"}
	for _, tag := range notAllowed {
		if tagInAllowedSet([]byte(tag)) {
			t.Errorf("tagInAllowedSet(%q) = true; want false", tag)
		}
	}
}
