package parser

import "testing"

// TestAllowedBlockTags_KnownAndUnknown verifies that allowedBlockTags contains
// the full CommonMark type-6 tag set and rejects non-block tags.
func TestAllowedBlockTags_KnownAndUnknown(t *testing.T) {
	known := []string{
		"div", "p", "ul", "ol", "table", "thead", "tbody", "tr", "td", "th",
		"blockquote", "details", "summary", "aside", "article", "section",
		"header", "footer", "nav", "main", "address", "dl", "dt", "dd",
		"figure", "figcaption", "h1", "h2", "h3", "h4", "h5", "h6",
		"hr", "html", "body", "head", "form", "li",
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
