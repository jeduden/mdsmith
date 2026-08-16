package runewidth

import "testing"

// TestRuneWidthEqualsNoLUT is the guard for the fork's single divergence
// from upstream: the deletion of the eager strictWidthLUT (see doc.go).
//
// The LUT was a pure memoization of runeWidthNoLUT — initStrictWidthLUT
// filled strictWidthLUT[0] with runeWidthNoLUT(r, false, true) and
// strictWidthLUT[1] with runeWidthNoLUT(r, true, true), and RuneWidth
// read it back. This test asserts RuneWidth returns exactly
// runeWidthNoLUT(r, eastAsian, strictEmojiNeutral) for every rune in the
// Unicode range and every (eastAsian, strictEmojiNeutral) combination.
//
// It passes on the as-copied fork (RuneWidth reads the LUT, which equals
// runeWidthNoLUT by construction) and must still pass after the LUT is
// deleted (RuneWidth calls runeWidthNoLUT directly). It is an ordinary
// untagged test in the default suite, so — unlike a build-tagged
// upstream-A/B apparatus — it cannot bit-rot when no workflow runs it.
func TestRuneWidthEqualsNoLUT(t *testing.T) {
	for _, ea := range []bool{false, true} {
		for _, strict := range []bool{false, true} {
			c := &Condition{EastAsianWidth: ea, StrictEmojiNeutral: strict}
			for r := rune(0); r <= 0x10FFFF; r++ {
				got := c.RuneWidth(r)
				want := runeWidthNoLUT(r, ea, strict)
				if got != want {
					t.Fatalf("RuneWidth(%#U) with eastAsian=%v strict=%v = %d, want runeWidthNoLUT = %d",
						r, ea, strict, got, want)
				}
			}
		}
	}
}

// Grapheme-path fixtures pinned by TestStringWidthGraphemeCases, written
// with explicit rune escapes so no invisible joiner or precomposed form
// can drift.
const (
	// man ZWJ woman ZWJ girl ZWJ boy — one rendered glyph.
	zwjFamily = "\U0001F468\u200D\U0001F469\u200D\U0001F467\u200D\U0001F466"
	// regional indicators U + S — the flag of the United States.
	flagUS = "\U0001F1FA\U0001F1F8"
	// lowercase e followed by combining acute accent (U+0301).
	combiningAcute = "é"
)

// TestStringWidthGraphemeCases pins the widths mdsmith actually depends
// on through the uax29 grapheme path, so a re-sync that regresses
// grapheme clustering or per-rune widths is caught. Widths are asserted
// under the default condition mdsmith uses (StrictEmojiNeutral, non-CJK).
func TestStringWidthGraphemeCases(t *testing.T) {
	c := &Condition{EastAsianWidth: false, StrictEmojiNeutral: true}
	cases := []struct {
		name string
		in   string
		want int
	}{
		// The status vocabulary mdsmith writes in plan front matter and
		// the PLAN.md status column. Each is a single width-2 glyph.
		{"check", "✅", 2},
		{"white-square-button", "\U0001F532", 2},
		{"black-square-button", "\U0001F533", 2},
		{"no-entry", "⛔", 2},
		{"robot", "\U0001F916", 2},
		// A ZWJ family sequence renders as one glyph; graphemeWidth caps
		// the summed rune widths at 2.
		{"zwj-family", zwjFamily, 2},
		// A regional-indicator flag. Upstream 0.0.27 gives this width 2
		// (it was width 1 in 0.0.24 — the deliberate upgrade).
		{"flag-us", flagUS, 2},
		// A base letter plus a combining acute accent: 1 + 0 = 1.
		{"combining-acute", combiningAcute, 1},
	}
	for _, tc := range cases {
		if got := c.StringWidth(tc.in); got != tc.want {
			t.Errorf("StringWidth(%q) [%s] = %d, want %d", tc.in, tc.name, got, tc.want)
		}
	}
}
