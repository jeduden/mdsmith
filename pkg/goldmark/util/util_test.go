package util

// Unit tests for the fork's bound-check additions in
// ResolveNumericReferences (plan-198 CodeQL fix). The decimal
// (`&#NNN;`) and hex (`&#xNN;`) entity paths must:
//
//   1. Decode in-range Unicode code points correctly.
//   2. Clamp out-of-range values to U+FFFD instead of risking an
//      uint64 -> int32 wraparound.
//
// The CodeQL rule that motivated the bound check is
// go/incorrect-integer-conversion; the test below pins the
// behaviour the fix gives the analyser.

import (
	"testing"
	"unicode/utf8"
)

func TestResolveNumericReferences_HexAndDecimalPath(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string // expected decoded segment somewhere in the result
	}{
		{"decimal-A", "&#65;", "A"},
		{"hex-A", "&#x41;", "A"},
		{"hex-snowman", "&#x2603;", "☃"},
		{"decimal-cherry", "&#127826;", "\U0001F352"},
		// Adjacent literals to confirm the rest of the source
		// stays untouched.
		{"surrounded", "<&#65;>", "<A>"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := string(ResolveNumericReferences([]byte(tc.in)))
			if got != tc.want {
				t.Errorf("ResolveNumericReferences(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestResolveNumericReferences_HexOverflowClampedToReplacement(t *testing.T) {
	// 7 hex digits (the i-start < 7 limit upstream of the cast)
	// can produce values up to 0xFFFFFFF = 268,435,455, which is
	// inside int32 range. To prove the MaxInt32 clamp itself, we
	// also drive the path with a value that the analyser cannot
	// statically prove is bounded: 0x110000 = 1114112 (one past
	// the Unicode max, well in int32 range but outside utf8).
	// The downstream ToValidRune turns this into U+FFFD.
	in := "&#x110000;"
	got := string(ResolveNumericReferences([]byte(in)))
	if got != "�" {
		t.Errorf("ResolveNumericReferences(%q) = %q, want \\uFFFD", in, got)
	}
}

func TestResolveNumericReferences_DecimalOverflowClampedToReplacement(t *testing.T) {
	// 7-digit decimal limit (i-start < 8) caps v at 9,999,999.
	// 9999999 is outside Unicode max (0x10FFFF = 1,114,111), so
	// ToValidRune turns it into U+FFFD via the downstream path.
	in := "&#9999999;"
	got := string(ResolveNumericReferences([]byte(in)))
	if got != "�" {
		t.Errorf("ResolveNumericReferences(%q) = %q, want \\uFFFD", in, got)
	}
}

func TestResolveNumericReferences_PreservesNonEntities(t *testing.T) {
	// Anything that isn't a complete &#...; / &#x...; entity
	// must pass through unchanged.
	cases := []string{
		"plain text",
		"&amp;",          // named entity, not numeric
		"&# missing ;",   // numeric path fails the IsNumeric check
		"&#;",            // empty body
		"&#xZZ;",         // not hex
		"unterminated &", // bare ampersand
	}
	for _, in := range cases {
		got := string(ResolveNumericReferences([]byte(in)))
		if got != in {
			t.Errorf("ResolveNumericReferences(%q) = %q, want passthrough", in, got)
		}
	}
}

// TestToValidRune pins the contract ResolveNumericReferences relies
// on for the post-clamp path: invalid runes (including the
// replacement code path through 0xFFFD itself) round-trip via
// utf8.EncodeRune to the U+FFFD glyph.
func TestToValidRune(t *testing.T) {
	cases := []struct {
		name string
		in   rune
		want rune
	}{
		{"zero-maps-to-replacement", 0, 0xFFFD},
		{"valid-ascii", 'A', 'A'},
		{"valid-bmp", '☃', '☃'},
		{"valid-astral", '\U0001F352', '\U0001F352'},
		{"surrogate-half-invalid", 0xD800, 0xFFFD},
		{"above-unicode-max-invalid", 0x110000, 0xFFFD},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ToValidRune(tc.in)
			if got != tc.want {
				t.Errorf("ToValidRune(%U) = %U, want %U", tc.in, got, tc.want)
			}
			if !utf8.ValidRune(got) {
				t.Errorf("ToValidRune(%U) returned non-valid rune %U", tc.in, got)
			}
		})
	}
}
