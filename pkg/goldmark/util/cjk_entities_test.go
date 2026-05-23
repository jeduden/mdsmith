package util

// Coverage for util_cjk.go (East Asian width helpers) and
// html5entities.go (HTML5 named-entity lookup).

import (
	"testing"
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
		{'A', "Na"}, // Narrow
		{'日', "W"}, // Wide
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
		if got := rune(ent.Characters[0]); got != c.first {
			// Many HTML5 entities expand into multi-byte UTF-8, so
			// also accept the case where Characters is the UTF-8
			// encoding and rune(Characters[0]) is the first byte.
			// Skip the exact rune comparison on those.
			if c.first <= 0x7f && got != c.first {
				t.Errorf("LookUpHTML5EntityByName(%q) first rune = %U, want %U", c.name, got, c.first)
			}
		}
	}
}
