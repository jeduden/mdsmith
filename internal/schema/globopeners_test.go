package schema

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The three `\#(` scanners — scanGlobRefs, PatternHasInterp and
// LiteralFmvarHint — must agree on where each opener starts and how
// the scan moves on from it: past a well-formed reference's closing
// `)`, but only past the `\#(` of a literal opener, so a reference
// inside a literal opener's body is still found. These cases pin that
// shared advance rule for every caller.
func TestGlobOpenerScanners_AgreeOnAdvanceRules(t *testing.T) {
	type ref struct {
		name string
		text string // the reference's own bytes, pattern[start:end]
	}
	cases := []struct {
		pattern string
		refs    []ref
		hint    string // substring of LiteralFmvarHint; "" = no hint
	}{
		{pattern: `a\\#(fmvar(x))b`},
		{pattern: `plain/*.md`},
		{
			pattern: `\#(draft)\#(fmvar(x))`,
			refs:    []ref{{"x", `\#(fmvar(x))`}},
		},
		{
			pattern: `\#(fmvar(x))\#(fmvar(my-key))`,
			refs:    []ref{{"x", `\#(fmvar(x))`}},
			hint:    "`\\#(fmvar(my-key))` is matched literally",
		},
		{
			pattern: `\#(fmvar(my-key))\#(fmvar(x))`,
			refs:    []ref{{"x", `\#(fmvar(x))`}},
			hint:    "`\\#(fmvar(my-key))` is matched literally",
		},
		{
			pattern: `\#(note \#(fmvar(x)))`,
			refs:    []ref{{"x", `\#(fmvar(x))`}},
		},
		{
			pattern: `\#(fmvar(\#(fmvar(x))))`,
			refs:    []ref{{"x", `\#(fmvar(x))`}},
			hint:    "`\\#(fmvar(\\#(fmvar(x))))` is matched literally",
		},
		{
			pattern: `\#(fmvar(x)`,
			hint:    "unterminated",
		},
		{
			pattern: `\#(fmvar(a))/\#(fmvar(b))\`,
			refs:    []ref{{"a", `\#(fmvar(a))`}, {"b", `\#(fmvar(b))`}},
		},
	}
	for _, tc := range cases {
		var got []ref
		require.NoError(t, scanGlobRefs(tc.pattern,
			func(name string, start, end int) error {
				got = append(got, ref{name, tc.pattern[start:end]})
				return nil
			}), tc.pattern)
		assert.Equal(t, tc.refs, got, "scanGlobRefs(%q)", tc.pattern)
		assert.Equal(t, len(tc.refs) > 0, PatternHasInterp(tc.pattern),
			"PatternHasInterp(%q)", tc.pattern)
		h := LiteralFmvarHint(tc.pattern)
		if tc.hint == "" {
			assert.Empty(t, h, "LiteralFmvarHint(%q)", tc.pattern)
		} else {
			assert.Contains(t, h, tc.hint, "LiteralFmvarHint(%q)", tc.pattern)
		}
	}
}

// globOpeners visits every opener in order with its parse result and
// stops as soon as visit returns false.
func TestGlobOpeners(t *testing.T) {
	type visit struct {
		text string
		name string
		err  error
	}
	pat := `\#(draft)/\#(fmvar(a))/\#(fmvar(b))`
	var got []visit
	globOpeners(pat, func(start, end int, name string, err error) bool {
		got = append(got, visit{pat[start:end], name, err})
		return true
	})
	assert.Equal(t, []visit{
		{`\#(draft)`, "", errGlobRefNotFmvar},
		{`\#(fmvar(a))`, "a", nil},
		{`\#(fmvar(b))`, "b", nil},
	}, got)

	calls := 0
	globOpeners(pat, func(int, int, string, error) bool {
		calls++
		return false
	})
	assert.Equal(t, 1, calls, "returning false stops the scan")

	globOpeners("plain/*.md", func(int, int, string, error) bool {
		t.Fatal("a pattern with no opener visits nothing")
		return true
	})
}

// The shared scan's callback costs no allocation: a plain glob, or
// one whose only opener is a deliberate literal, scans for free. (A
// well-formed reference pays for parsing its CUE path; that is why
// parsePathPatterns records the answer once per pattern.)
func TestPatternHasInterp_PlainPatternDoesNotAllocate(t *testing.T) {
	for _, p := range []string{"docs/**/*.md", `notes/\#(draft)*.md`} {
		allocs := testing.AllocsPerRun(100, func() { _ = PatternHasInterp(p) })
		assert.Zero(t, allocs, p)
	}
}
