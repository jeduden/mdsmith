package lint

import "testing"

// TestTagInAllowedSet verifies that tagInAllowedSet recognises the CommonMark
// type-6 HTML block tag set and rejects non-block tags.
func TestTagInAllowedSet_KnownTags(t *testing.T) {
	// Full CommonMark 0.31.2 type-6 block tag set.
	known := []string{
		"address", "article", "aside", "base", "basefont", "blockquote",
		"body", "caption", "center", "col", "colgroup", "dd", "details",
		"dialog", "dir", "div", "dl", "dt", "fieldset", "figcaption",
		"figure", "footer", "form", "frame", "frameset", "h1", "h2", "h3",
		"h4", "h5", "h6", "head", "header", "hr", "html", "iframe",
		"legend", "li", "link", "main", "menu", "menuitem", "meta", "nav",
		"noframes", "ol", "optgroup", "option", "p", "param", "search",
		"section", "summary", "table", "tbody", "td", "tfoot", "th",
		"thead", "title", "tr", "track", "ul",
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
