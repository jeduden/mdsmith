package parser

import "testing"

// TestAllowedBlockTags_KnownAndUnknown verifies that allowedBlockTags contains
// the full CommonMark type-6 tag set and rejects non-block tags.
func TestAllowedBlockTags_KnownAndUnknown(t *testing.T) {
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
		if _, ok := allowedBlockTags[tag]; !ok {
			t.Errorf("allowedBlockTags missing known type-6 tag %q", tag)
		}
	}

	notAllowed := []string{"a", "span", "em", "strong", "code", "img", "br"}
	for _, tag := range notAllowed {
		if _, ok := allowedBlockTags[tag]; ok {
			t.Errorf("allowedBlockTags unexpectedly contains inline tag %q", tag)
		}
	}
}
