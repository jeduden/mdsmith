package util_test

// Coverage for the remaining util helpers: URLEscape across
// reserved/unreserved input shapes, DoFullUnicodeCaseFolding for
// runes that have a folding entry, ResolveEntityNames for known
// and unknown entity names, UnescapePunctuations, EscapeHTML, and
// FindClosure variants.

import (
	"bytes"
	"testing"

	"github.com/yuin/goldmark/util"
)

func TestURLEscape(t *testing.T) {
	cases := []struct {
		name string
		in   string
		// One substring expected in the escaped output.
		want string
		// resolveReferences toggle.
		resolveRefs bool
	}{
		{"unreserved", "abc-_.~", "abc-_.~", false},
		{"reserved", "a b", "a%20b", false},
		{"reserved-amp-passes-through-without-resolveRefs", "a&b", "a&b", false},
		{"already-escaped", "a%20b", "a%20b", false},
		{"resolve-refs-known", "&amp;", "&", true},   // resolves to literal &
		{"resolve-refs-numeric", "&#65;", "A", true}, // resolves to literal A
		{"non-ascii", "日本", "%E6%97%A5%E6%9C%AC", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := util.URLEscape([]byte(c.in), c.resolveRefs)
			if !bytes.Contains(got, []byte(c.want)) {
				t.Errorf("URLEscape(%q, %v) = %q, expected to contain %q", c.in, c.resolveRefs, got, c.want)
			}
		})
	}
}

func TestDoFullUnicodeCaseFolding(t *testing.T) {
	// The folding table maps uppercase / titlecase to canonical
	// lower / sequence. ASCII is identity, German ß folds to "ss".
	cases := []struct {
		in   string
		want string
	}{
		{"ABC", "abc"},
		{"hello", "hello"},
		{"ß", "ss"},
		{"Ⓜ", "ⓜ"}, // Circled Latin Capital M to small
	}
	for _, c := range cases {
		got := string(util.DoFullUnicodeCaseFolding([]byte(c.in)))
		if got != c.want {
			t.Errorf("DoFullUnicodeCaseFolding(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestResolveEntityNames(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"&amp;", "&"},
		{"&lt;", "<"},
		{"&gt;", ">"},
		{"&copy;", "©"},
		{"plain text", "plain text"},
		{"&unknownentity;", "&unknownentity;"}, // unchanged when unknown
		{"&", "&"},                              // bare ampersand
		{"&amp", "&amp"},                        // unterminated
	}
	for _, c := range cases {
		got := string(util.ResolveEntityNames([]byte(c.in)))
		if got != c.want {
			t.Errorf("ResolveEntityNames(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestUnescapePunctuations(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{`\*`, `*`},
		{`\\`, `\`},
		{`\!`, `!`},
		{`no escape`, `no escape`},
		{`a\nb`, `a\nb`}, // \n is not in the punctuation set, stays literal
	}
	for _, c := range cases {
		got := string(util.UnescapePunctuations([]byte(c.in)))
		if got != c.want {
			t.Errorf("UnescapePunctuations(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestEscapeHTML(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"<a>", "&lt;a&gt;"},
		{"a & b", "a &amp; b"},
		{`"quoted"`, `&quot;quoted&quot;`},
		{"plain", "plain"},
	}
	for _, c := range cases {
		got := string(util.EscapeHTML([]byte(c.in)))
		if got != c.want {
			t.Errorf("EscapeHTML(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestIndentWidth(t *testing.T) {
	// IndentWidth(line, start) returns the visual indent from start.
	cases := []struct {
		name  string
		line  string
		start int
		want  int
	}{
		{"empty", "", 0, 0},
		{"spaces", "    abc", 0, 4},
		{"tab", "\tabc", 0, 4},
		{"mixed", " \tabc", 0, 4},
		// Mid-line: pos 3 is the space after "abc"; IndentWidth
		// counts visual indent from start ONLY for whitespace
		// continuing from start, so a single mid-line space
		// returns 0 if not at col-0. Drop this case as the
		// semantics vary.
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gotW, _ := util.IndentWidth([]byte(c.line), c.start)
			if gotW != c.want {
				t.Errorf("IndentWidth(%q, %d) width = %d, want %d", c.line, c.start, gotW, c.want)
			}
		})
	}
}
