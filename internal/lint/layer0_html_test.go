package lint

import (
	"strings"
	"testing"
)

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

func TestOpenHTMLBlock(t *testing.T) {
	tests := []struct {
		line        string
		inParagraph bool
		want        htmlBlockType
	}{
		// Type 1: raw-text tags
		{"<script>", false, htmlType1},
		{"<pre>", false, htmlType1},
		{"<style>", false, htmlType1},
		{"<textarea>", false, htmlType1},
		{"<SCRIPT>", false, htmlType1},
		{"<Script src='a'>", false, htmlType1},
		// Type 2: comment
		{"<!-- comment -->", false, htmlType2},
		{"<!--", false, htmlType2},
		// Type 3: PI
		{"<?php echo 1; ?>", false, htmlType3},
		// Type 4: declaration
		{"<!DOCTYPE html>", false, htmlType4},
		// Type 5: CDATA
		{"<![CDATA[foo]]>", false, htmlType5},
		// Type 6: allowed block tag
		{"<div>", false, htmlType6},
		{"<DIV>", false, htmlType6},
		{"<p>", false, htmlType6},
		// Type 7: generic complete tag, only when not in paragraph
		{"<foo/>", false, htmlType7},
		{"<foo />", false, htmlType7},
		// Type 7 suppressed when inParagraph=true
		{"<foo/>", true, htmlNone},
		// Raw-text tag: type 1 takes precedence over the type-7 path
		{"<script/>", false, htmlType1},
		// Ordinary prose
		{"plain text", false, htmlNone},
		{"not html", true, htmlNone},
	}
	for _, tc := range tests {
		got := openHTMLBlock([]byte(tc.line), tc.inParagraph)
		if got != tc.want {
			t.Errorf("openHTMLBlock(%q, inParagraph=%v) = %v; want %v",
				tc.line, tc.inParagraph, got, tc.want)
		}
	}
}

func TestTagName_LowerInto(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"div", "div"},
		{"DIV", "div"},
		{"Div", "div"},
		{"Script", "script"},
		{"TEXTAREA", "textarea"},
		// Already lowercase
		{"pre", "pre"},
		// Mixed
		{"aBcDeF", "abcdef"},
		// Truncation at 32 bytes
		{strings.Repeat("A", 40), strings.Repeat("a", 32)},
		// Exactly 32 bytes
		{strings.Repeat("B", 32), strings.Repeat("b", 32)},
	}
	for _, tc := range tests {
		var tn tagName
		got := tn.lowerInto([]byte(tc.input))
		if string(got) != tc.want {
			t.Errorf("lowerInto(%q) = %q; want %q", tc.input, got, tc.want)
		}
	}
}

func TestType7TagIsRawText(t *testing.T) {
	tests := []struct {
		line string
		want bool
	}{
		{"<script/>", true},
		{"<SCRIPT/>", true},
		{"<style/>", true},
		{"<STYLE/>", true},
		{"<pre/>", true},
		{"<PRE/>", true},
		{"<textarea/>", true},
		{"<TEXTAREA/>", true},
		{"<div/>", false},
		{"<span/>", false},
		{"<foo/>", false},
	}
	for _, tc := range tests {
		got := type7TagIsRawText([]byte(tc.line))
		if got != tc.want {
			t.Errorf("type7TagIsRawText(%q) = %v; want %v", tc.line, got, tc.want)
		}
	}
}

func TestType7TagBytes(t *testing.T) {
	tests := []struct {
		line string
		want string
	}{
		{"<div>", "div"},
		{"<DIV>", "DIV"},
		{"</div>", "div"},
		{"</ div>", "div"},
		{"<foo/>", "foo"},
		{"   <bar/>", "bar"},
		{"<script src='x'>", "script"},
	}
	for _, tc := range tests {
		got := type7TagBytes([]byte(tc.line))
		if string(got) != tc.want {
			t.Errorf("type7TagBytes(%q) = %q; want %q", tc.line, got, tc.want)
		}
	}
}

func TestIsTagByte(t *testing.T) {
	tests := []struct {
		b    byte
		want bool
	}{
		{'a', true},
		{'z', true},
		{'A', true},
		{'Z', true},
		{'0', true},
		{'9', true},
		{'-', true},
		{'/', false},
		{'>', false},
		{'!', false},
		{' ', false},
		{'_', false},
	}
	for _, tc := range tests {
		got := isTagByte(tc.b)
		if got != tc.want {
			t.Errorf("isTagByte(%q) = %v; want %v", tc.b, got, tc.want)
		}
	}
}

func TestHTMLBlockCloses(t *testing.T) {
	tests := []struct {
		line string
		t    htmlBlockType
		want bool
	}{
		// Type 1: closes on </script> etc., case-insensitive
		{"</script>", htmlType1, true},
		{"</SCRIPT>", htmlType1, true},
		{"</pre>", htmlType1, true},
		{"</style>", htmlType1, true},
		{"</textarea>", htmlType1, true},
		{"<div>", htmlType1, false},
		// Type 2: closes on -->
		{"-->", htmlType2, true},
		{"text --> more", htmlType2, true},
		{"<!-- still open", htmlType2, false},
		// Type 3: closes on ?>
		{"?>", htmlType3, true},
		{"still <?", htmlType3, false},
		// Type 4: closes on >
		{">", htmlType4, true},
		{"some > here", htmlType4, true},
		{"no close", htmlType4, false},
		// Type 5: closes on ]]>
		{"]]>", htmlType5, true},
		{"text ]]> more", htmlType5, true},
		{"<![CDATA[", htmlType5, false},
		// Types 6 and 7: blank-line close handled by caller; always false here
		{"", htmlType6, false},
		{"anything", htmlType6, false},
		{"", htmlType7, false},
		{"anything", htmlType7, false},
	}
	for _, tc := range tests {
		got := htmlBlockCloses([]byte(tc.line), tc.t)
		if got != tc.want {
			t.Errorf("htmlBlockCloses(%q, type%d) = %v; want %v", tc.line, tc.t, got, tc.want)
		}
	}
}

// newTestScanner builds a minimal scanner from a newline-delimited source
// string suitable for driving tryHTMLBlock directly.
func newTestScanner(src string) *scanner {
	lines := splitLines(src)
	n := len(lines)
	l0 := &Layer0Scan{
		Classes:        make([]lineClass, n),
		CodeBlockLines: make(map[int]struct{}, n),
		PIBlockLines:   make(map[int]struct{}, n),
		BlockSpans:     make([]BlockSpan, 0, n),
	}
	return &scanner{lines: lines, l0: l0}
}

func TestScanner_TryHTMLBlock(t *testing.T) {
	t.Run("comment block consumed with span", func(t *testing.T) {
		s := newTestScanner("<!-- comment -->\nnext line\n")
		ok := s.tryHTMLBlock(false)
		if !ok {
			t.Fatal("tryHTMLBlock returned false; want true")
		}
		if s.i != 1 {
			t.Errorf("cursor = %d; want 1", s.i)
		}
		if len(s.l0.BlockSpans) != 1 {
			t.Fatalf("got %d spans; want 1", len(s.l0.BlockSpans))
		}
		sp := s.l0.BlockSpans[0]
		if sp.Kind != BlockHTML {
			t.Errorf("span kind = %v; want BlockHTML", sp.Kind)
		}
	})

	t.Run("multi-line type-6 block consumed", func(t *testing.T) {
		src := "<div>\ncontent\n</div>\nnext\n"
		s := newTestScanner(src)
		ok := s.tryHTMLBlock(false)
		if !ok {
			t.Fatal("tryHTMLBlock returned false; want true")
		}
		// Type 6 closes on blank line; no blank here so it runs to end
		// of non-blank content — depends on how the source terminates.
		if len(s.l0.BlockSpans) != 1 {
			t.Fatalf("got %d spans; want 1", len(s.l0.BlockSpans))
		}
		sp := s.l0.BlockSpans[0]
		if sp.Kind != BlockHTML {
			t.Errorf("span kind = %v; want BlockHTML", sp.Kind)
		}
	})

	t.Run("non-HTML line returns false", func(t *testing.T) {
		s := newTestScanner("plain text\n")
		ok := s.tryHTMLBlock(false)
		if ok {
			t.Fatal("tryHTMLBlock returned true on plain text; want false")
		}
		if len(s.l0.BlockSpans) != 0 {
			t.Errorf("got %d spans; want 0", len(s.l0.BlockSpans))
		}
	})
}
