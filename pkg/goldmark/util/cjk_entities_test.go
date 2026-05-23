package util

// Coverage for util_cjk.go (East Asian width helpers) and
// html5entities.go (HTML5 named-entity lookup).

import (
	"testing"
	"unicode/utf8"
)

func TestIsEastAsianWideRune(t *testing.T) {
	cases := []struct {
		r    rune
		want bool
	}{
		{'A', false},
		{'☃', false},
		{'日', true},  // CJK Unified Ideograph
		{'한', true},  // Hangul Syllable
		{'　', true}, // Ideographic Space (U+3000)
		{0, false},
	}
	for _, c := range cases {
		if got := IsEastAsianWideRune(c.r); got != c.want {
			t.Errorf("IsEastAsianWideRune(%U) = %v, want %v", c.r, got, c.want)
		}
	}
}

func TestIsSpaceDiscardingUnicodeRune(t *testing.T) {
	cases := []struct {
		r    rune
		want bool
	}{
		{'A', false},
		{'日', true}, // CJK Unified Ideograph
		{' ', false},
	}
	for _, c := range cases {
		if got := IsSpaceDiscardingUnicodeRune(c.r); got != c.want {
			t.Errorf("IsSpaceDiscardingUnicodeRune(%U) = %v, want %v", c.r, got, c.want)
		}
	}
}

func TestEastAsianWidth(t *testing.T) {
	cases := []struct {
		r    rune
		want string
	}{
		{'A', "Na"},     // Narrow — ASCII
		{0x4E00, "W"},   // Wide — CJK Unified Ideograph
		{0x3000, "F"},   // Fullwidth — Ideographic Space
		{0xFF01, "F"},   // Fullwidth — fullwidth exclamation
		{0xFF61, "H"},   // Halfwidth — halfwidth kana punctuation
		{0x20A9, "H"},   // Halfwidth — won sign
		{0x1100, "W"},   // Wide — Hangul jamo
		{0x00A1, "A"},   // Ambiguous — inverted exclamation
		{0x0939, "N"},   // Neutral — Devanagari Ha
	}
	for _, c := range cases {
		if got := EastAsianWidth(c.r); got != c.want {
			t.Errorf("EastAsianWidth(%U) = %q, want %q", c.r, got, c.want)
		}
	}
}

func TestLookUpHTML5EntityByName(t *testing.T) {
	cases := []struct {
		name  string
		found bool
		// First codepoint of the expected entity, when found.
		first rune
	}{
		{"amp", true, '&'},
		{"lt", true, '<'},
		{"gt", true, '>'},
		{"copy", true, '©'},
		{"AElig", true, 'Æ'},
		{"this-is-not-an-entity", false, 0},
		{"", false, 0},
	}
	for _, c := range cases {
		ent, ok := LookUpHTML5EntityByName(c.name)
		if ok != c.found {
			t.Errorf("LookUpHTML5EntityByName(%q) ok = %v, want %v", c.name, ok, c.found)
			continue
		}
		if !c.found {
			continue
		}
		if len(ent.Characters) == 0 {
			t.Errorf("LookUpHTML5EntityByName(%q) returned entity with no chars", c.name)
			continue
		}
		got, _ := utf8.DecodeRune(ent.Characters)
		if got != c.first {
			t.Errorf("LookUpHTML5EntityByName(%q) first rune = %U, want %U", c.name, got, c.first)
		}
	}
}
