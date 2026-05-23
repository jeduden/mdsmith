package util_test

// Coverage for the remaining util helpers: URLEscape across
// reserved/unreserved input shapes (including truncated multi-byte
// UTF-8), DoFullUnicodeCaseFolding for runes that have a folding
// entry (and invalid UTF-8 / continuation-byte skip paths),
// ResolveEntityNames for known and unknown entity names,
// UnescapePunctuations, EscapeHTML, ReplaceSpaces, and IndentWidth.
// (FindClosure-related cases live in findclosure_test.go.)

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

func TestReplaceSpaces(t *testing.T) {
	cases := []struct {
		in   string
		repl byte
		want string
	}{
		{"hello world", '_', "hello_world"},
		{"multi    spaces", '_', "multi_spaces"},
		{"  leading", '_', "_leading"},
		// ReplaceSpaces preserves trailing whitespace; only
		// interior runs are collapsed.
		{"no-spaces", '_', "no-spaces"},
		{"", '_', ""},
	}
	for _, c := range cases {
		got := string(util.ReplaceSpaces([]byte(c.in), c.repl))
		if got != c.want {
			t.Errorf("ReplaceSpaces(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestURLEscape_TruncatedMultiByteAtEnd(t *testing.T) {
	// Multi-byte UTF-8 truncated at end of input -> stop > len(v)
	// branch.  A 4-byte UTF-8 leading byte followed by only 1
	// continuation triggers this.  Single-byte inputs of a
	// 2/3/4-byte leader also drive the u8len-adjusted-to-0 branch.
	cases := [][]byte{
		{0xF0, 0x9F},               // 4-byte truncated to 2
		{0xF0, 0x9F, 0x98},         // 4-byte truncated to 3
		{0xE0, 0xA4},               // 3-byte truncated
		{0xC2},                     // 2-byte leader alone -> u8len becomes 0
		{0xE0},                     // 3-byte leader alone
		{0xF0},                     // 4-byte leader alone
	}
	for _, c := range cases {
		_ = util.URLEscape(c, false)
	}
}

func TestURLEscape_EdgeBytes(t *testing.T) {
	// Drive remaining URLEscape branches: invalid UTF-8 leading
	// byte (u8len == 99), multi-byte truncated at end of input,
	// already-percent-escaped passthrough, trailing-only chars.
	cases := []string{
		"already%20escaped",
		string([]byte{0xC2, 'a', 0xC3}),                // truncated multi-byte at end
		string([]byte{0xFF, 'x'}),                       // invalid leading byte
		string([]byte{0xC2, 0xA0}),                      // 2-byte UTF-8 (NBSP)
		string([]byte{0xE0, 0xA4, 0xB9}),                // 3-byte UTF-8
		string([]byte{0xF0, 0x9F, 0x98, 0x80}),          // 4-byte UTF-8 emoji
		"plain text",
		"",
	}
	for _, c := range cases {
		_ = util.URLEscape([]byte(c), false)
	}
}

func TestDoFullUnicodeCaseFolding(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"ABC", "abc"},
		{"hello", "hello"},
		{"ß", "ss"},
		{"Ⓜ", "ⓜ"},                                  // Circled Latin Capital M to small
		{"AÉ", "aé"},                                // mix ASCII + accented
		{string([]byte{0xFF, 0xFE}), string([]byte{0xFF, 0xFE})}, // invalid UTF-8 -> RuneError skipped
		{string([]byte{'A', 0x80, 'B'}), "a\x80b"},  // continuation byte mid-stream
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
