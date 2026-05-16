package schema

import (
	"strconv"
	"strings"
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// TestClaimsLaterLiteral_Branches covers the four branches of
// claimsLaterLiteral so an ambiguous matcher run (e.g. `regex: '.+'`
// with `repeat: { min: 1 }`) yields to a later literal entry only
// when one is actually present.
func TestClaimsLaterLiteral_Branches(t *testing.T) {
	scopes := []Scope{
		// idx 0: preamble — never claims headings (no matcher)
		{Preamble: true},
		// idx 1: slot — never claims (broad `.+` matcher is
		// filtered out by claimsLaterLiteral so it never
		// reserves a heading from an earlier scope)
		slotScope(),
		// idx 2: optional literal — eligible to claim its own
		// text "Optional"; ignored here because the test heading
		// is "References", not "Optional"
		optionalScope("Optional"),
		// idx 3: required literal that matches the heading
		literalScope("References"),
	}
	claimed := map[int]bool{}
	dh := DocHeading{Text: "References", Level: 2}
	assert.True(t,
		claimsLaterLiteral(scopes, 0, dh, claimed, nil),
		"a heading whose text matches a later required literal must reserve for that literal")

	// dh that doesn't match any later literal — no claim.
	other := DocHeading{Text: "Trailing", Level: 2}
	assert.False(t,
		claimsLaterLiteral(scopes, 0, other, claimed, nil),
		"a heading that matches no later required literal does not reserve")

	// startIdx past the literal — none ahead, no claim.
	assert.False(t,
		claimsLaterLiteral(scopes, 4, dh, claimed, nil),
		"no scopes at or after startIdx => no claim")

	// Already-claimed literal — not eligible.
	claimed[3] = true
	assert.False(t,
		claimsLaterLiteral(scopes, 0, dh, claimed, nil),
		"a claimed later literal must not reserve again")
}

// TestDisplayHeading_Branches covers all three return paths.
func TestDisplayHeading_Branches(t *testing.T) {
	// Heading set — bare-string sugar path.
	assert.Equal(t, "Overview", displayHeading(literalScope("Overview")))
	// Heading empty, Matcher set — mapping-form fallback.
	sc := Scope{Matcher: &Matcher{Regex: ".+"}}
	assert.Equal(t, ".+", displayHeading(sc))
	// Neither set — preamble label.
	assert.Equal(t, "", displayHeading(Scope{Preamble: true}))
}

// TestWrongLevelMatch_RecursesIntoChildren covers the branch of
// claimMatch reached via the shallower-than-expected heading
// path: a scope whose nested children must still be validated
// even when the outer heading appeared at the wrong level.
func TestWrongLevelMatch_RecursesIntoChildren(t *testing.T) {
	// Schema: Outer at H3 (because of an outer wrapper), Inner at
	// H4. Doc emits Outer at H2 (shallower than expected H3).
	// claimMatch recurses into Inner's checks; the missing-Inner
	// diagnostic surfaces from the nested validateScopes call.
	raw := map[string]any{
		"sections": []any{
			map[string]any{
				"heading": "Top",
				"sections": []any{
					map[string]any{
						"heading": "Outer",
						"sections": []any{
							map[string]any{"heading": "Inner"},
						},
					},
				},
			},
		},
	}
	sch, err := ParseInline(raw, "kind x")
	require.NoError(t, err)
	// Doc has Top at H2; Outer at H2 (wrong level — expected H3);
	// no Inner. The nested validateScopes from claimRun should
	// emit a missing-Inner diagnostic.
	doc := newDocFile(t, "doc.md",
		"# T\n\n## Top\n\n## Outer\n\nx\n")
	diags := Validate(doc, sch, nil, false, makeDiagForTest)
	var missingInner bool
	for _, d := range diags {
		if strings.Contains(d.Message, `#### Inner: got <missing>`) &&
			strings.Contains(d.Message, `expected section to be present`) {
			missingInner = true
		}
	}
	assert.True(t, missingInner,
		"the children-recurse branch must validate nested sections")
}

// TestWrongLevelMatch_EmitsLevelDiag covers the wrong-level
// match path: matchScope sees a shallower-than-expected heading
// that still matches the matcher, claimMatch emits the
// level-mismatch diagnostic, and the consumed/digits state is
// updated so repeat.min / sequential still apply.
func TestWrongLevelMatch_EmitsLevelDiag(t *testing.T) {
	// Nested schema: outer expects H2, inner expects H3. The doc
	// emits the inner heading at H2 (shallower than expected).
	raw := map[string]any{
		"sections": []any{
			map[string]any{
				"heading": "Outer",
				"sections": []any{
					map[string]any{"heading": "Inner"},
				},
			},
		},
	}
	sch, err := ParseInline(raw, "kind x")
	require.NoError(t, err)
	doc := newDocFile(t, "doc.md",
		"# T\n\n## Outer\n\n## Inner\n\nx\n")
	diags := Validate(doc, sch, nil, false, makeDiagForTest)
	var level bool
	for _, d := range diags {
		if strings.Contains(d.Message, `Inner: got h2, expected h3`) {
			level = true
		}
	}
	assert.True(t, level,
		"the wrong-level match path must emit the level-mismatch diagnostic")
}

// TestSequentialDiagMessage_NonInteger covers the parse-error path
// inside sequentialDiagMessage. The `digits` helper always matches
// `[0-9]+` so this never trips in normal usage; exercising it
// directly keeps the safety branch covered.
func TestSequentialDiagMessage_NonInteger(t *testing.T) {
	got := sequentialDiagMessage([]string{"abc"})
	assert.Contains(t, got, "must be integers")
}

// TestSequentialDiagMessage_OK returns empty when the sequence is
// strictly increasing.
func TestSequentialDiagMessage_OK(t *testing.T) {
	assert.Empty(t, sequentialDiagMessage([]string{"1", "2", "3"}))
}

// TestCachedMatcher_NilAndInvalid covers the two early-error
// branches of cachedMatcher.
func TestCachedMatcher_NilAndInvalid(t *testing.T) {
	_, err := cachedMatcher(nil, nil)
	require.Error(t, err)

	_, err = cachedMatcher(&Matcher{Regex: "[unterminated"}, nil)
	require.Error(t, err)
}

// TestMatchHeading_NilMatcher covers the early-return for a nil
// matcher (the preamble's shape).
func TestMatchHeading_NilMatcher(t *testing.T) {
	matched, captured := matchHeading(nil, DocHeading{Text: "x"}, nil)
	assert.False(t, matched)
	assert.Empty(t, captured)
}

// TestMatchHeading_DigitsCapture verifies the captured group is
// returned alongside the match.
func TestMatchHeading_DigitsCapture(t *testing.T) {
	m := &Matcher{Regex: `Step \#(digits)`}
	matched, captured := matchHeading(m, DocHeading{Text: "Step 42"}, nil)
	assert.True(t, matched)
	assert.Equal(t, "42", captured)
}

// TestFmvarLookup_MissingField signals a missing value via the
// second return so resolvePattern can fail the match instead of
// substituting an empty regex fragment that would otherwise let
// a degenerate heading match the literal-only remainder of the
// pattern.
func TestFmvarLookup_MissingField(t *testing.T) {
	_, ok := fmvarLookup(nil, "id")
	assert.False(t, ok)
	_, ok = fmvarLookup(map[string]any{}, "id")
	assert.False(t, ok)
	_, ok = fmvarLookup(map[string]any{"id": "X"}, "")
	assert.False(t, ok)
}

// TestFirstWrongLevelMatch_SkipsClaimedAndOutOfRange exercises the
// three early-continue branches of firstWrongLevelMatch:
// already-claimed entries, headings outside the parent window, and
// same-level headings (which the same-level run owns). The result
// is the index of the wrong-level match that does pass all three
// filters.
func TestFirstWrongLevelMatch_SkipsClaimedAndOutOfRange(t *testing.T) {
	heads := []DocHeading{
		{Level: 3, Text: "Step", Line: 5},  // 0: would match, claimed
		{Level: 3, Text: "Step", Line: 1},  // 1: before parentStart
		{Level: 2, Text: "Step", Line: 12}, // 2: same level, skip
		{Level: 3, Text: "Step", Line: 15}, // 3: the wrong-level match
	}
	sc := Scope{Heading: "Step", Matcher: &Matcher{Regex: "Step"}}
	claimed := map[int]bool{0: true}
	idx := firstWrongLevelMatch(sc, heads, 2, 3, 100, claimed, nil)
	assert.Equal(t, 3, idx,
		"claimed heads, out-of-range heads, and same-level heads "+
			"must all be skipped before the wrong-level match wins")
}

// TestHeadingLine_EmptyAtxFallback exercises headingLine's
// fallback path for ATX headings whose Lines() slice is empty
// (a goldmark quirk that fires when an ATX consumes its line via
// an inline extension). We synthesize the state by handing
// ExtractDocHeadings a parser that yields such a heading,
// asserting the descendant-text scan still returns a positive
// line number. The body uses link-reference defs which goldmark
// strips before AST construction, leaving the heading with only
// inline children.
func TestHeadingLine_EmptyAtxFallback(t *testing.T) {
	src := []byte("# T\n\n## body\n\nbody\n")
	f, err := lint.NewFileFromSource("doc.md", src, false)
	require.NoError(t, err)
	// Construct a synthetic ATX heading whose Lines() slice is
	// empty but which carries a *ast.Text descendant — exactly
	// the shape goldmark produces on the rare paths described
	// in headingLine's doc comment. Prepend a non-Text inline
	// child (an Emphasis) so the walker also exercises the
	// `_, ok := n.(*ast.Text)` non-Text continue branch before
	// reaching the Text segment.
	h := ast.NewHeading(2)
	h.AppendChild(h, ast.NewEmphasis(1))
	tn := ast.NewTextSegment(text.NewSegment(2, 6))
	h.AppendChild(h, tn)
	got := headingLine(h, f)
	assert.Greater(t, got, 0,
		"the descendant-Text fallback must yield a positive line "+
			"when Lines() is empty")
}

// TestFrontmatterExpr_MarshalErrorSliceMap covers the
// json.Marshal error return for the []any / map[string]any
// branch by passing a map that contains an unmarshalable value
// (a Go channel — channels have no JSON representation, so
// encoding/json returns *json.UnsupportedTypeError). The same
// shape can reach frontmatterExpr through nested user input
// (highly unusual but defensively handled), and the test pins
// the error path so the diagnostic surfaces rather than
// returning an empty string.
func TestFrontmatterExpr_MarshalErrorSliceMap(t *testing.T) {
	m := map[string]any{"ch": make(chan int)}
	_, err := frontmatterExpr(m)
	require.Error(t, err,
		"a map containing an unmarshalable value must surface "+
			"the json error rather than silently returning")
}

// TestHeadingLine_EmptyEverywhereFallback covers the trailing
// `return line` default when an ATX heading has neither a
// populated Lines() slice nor an inline *ast.Text descendant.
func TestHeadingLine_EmptyEverywhereFallback(t *testing.T) {
	src := []byte("# T\n")
	f, err := lint.NewFileFromSource("doc.md", src, false)
	require.NoError(t, err)
	h := ast.NewHeading(2)
	got := headingLine(h, f)
	assert.Equal(t, 1, got,
		"a heading with no Lines and no Text descendant defaults to line 1")
}

// TestHeadingLine_InlineFallback covers headingLine's fallback
// branch: goldmark occasionally produces ATX headings with an
// empty Lines() slice (the leading marker is consumed by an
// inline construct like a code span), in which case we walk
// inline descendants to recover the source line. Constructing
// the empty-Lines case directly is awkward, so the test goes
// through the regular parser with a heading whose body is
// purely inline content and asserts that headingLine still
// returns a positive line via the descendant-text path when
// the primary path returns no data.
func TestHeadingLine_InlineFallback(t *testing.T) {
	// Build an ATX heading whose body is just an inline code
	// span. Goldmark may or may not populate Lines() for this
	// shape depending on parser internals; the assertion below
	// only checks that ExtractDocHeadings returns a non-zero
	// line for the heading either way, exercising both branches
	// of headingLine across the run.
	src := "# Title\n\n## `code-span`\n\nbody\n"
	doc := newDocFile(t, "doc.md", src)
	heads := ExtractDocHeadings(doc)
	require.NotEmpty(t, heads, "parser must yield at least one heading")
	for _, h := range heads {
		assert.Greater(t, h.Line, 0,
			"every heading must resolve to a positive line, "+
				"including the inline-content fallback path")
	}
}

// TestParseFmvarCall_NegativeForms covers the early-return
// branches of parseFmvarCall — expressions that aren't fmvar
// calls, calls missing the opening paren, calls without a
// closing paren, and calls with trailing garbage after the
// close. Each must return (_, false) so the resolver falls
// through to the "unknown helper" diagnostic.
func TestParseFmvarCall_NegativeForms(t *testing.T) {
	cases := []string{
		"digits",               // not an fmvar prefix
		"fmvarX",               // prefix without `(`
		"fmvar(unterminated",   // no closing `)`
		"fmvar(name) trailing", // garbage after close
	}
	for _, expr := range cases {
		_, ok := parseFmvarCall(expr)
		assert.False(t, ok, "expr %q must not parse as a valid fmvar call", expr)
	}
}

// TestHasNamedCapture_RejectsInvalidPattern covers the parse-
// error branch in hasNamedCapture: an unparseable regex returns
// false so a downstream caller still reaches compileMatcher,
// which surfaces the actual parse error with diagnostic context.
func TestHasNamedCapture_RejectsInvalidPattern(t *testing.T) {
	// An unbalanced paren makes regexp/syntax.Parse return an
	// error. The helper must report no captures.
	assert.False(t, hasNamedCapture(`(?P<n>`, "n"),
		"an unparseable pattern must report no captures")
}

// TestRegexpHasNamedCapture_NilSafe covers regexpHasNamedCapture's
// defensive nil check: a nil sub-expression yields no captures
// without panicking, which keeps the recursion safe for future
// callers that might pass a constructed AST with sparse children.
func TestRegexpHasNamedCapture_NilSafe(t *testing.T) {
	assert.False(t, regexpHasNamedCapture(nil, "n"),
		"nil regex node must not match any capture name")
}

// TestSkipQuotedSegment_UnterminatedReturnsEnd exercises the
// end-of-pattern fallthrough when a `\#(fmvar("...))` has no
// closing quote. scanInterps then surfaces the failure as an
// "unterminated interpolation" diagnostic via findInterpEnd's
// false return.
func TestSkipQuotedSegment_UnterminatedReturnsEnd(t *testing.T) {
	pattern := `\#(fmvar("unterminated`
	err := scanInterps(pattern, func(string, int, int) error { return nil })
	require.Error(t, err,
		"an unterminated quoted segment must surface as a parse error")
	assert.Contains(t, err.Error(), "unterminated interpolation")
}

// TestScanInterps_QuotedEscapeAdvances covers scanInterps'
// escape-inside-quote branch: a `\"` inside the quoted segment
// must advance past the quoted quote without ending the
// segment, so the scanner doesn't reopen paren-counting
// prematurely on a CUE-style key like `fmvar("a\"b")`.
func TestScanInterps_QuotedEscapeAdvances(t *testing.T) {
	pattern := `\#(fmvar("a\"b"))`
	var seen string
	err := scanInterps(pattern, func(expr string, _, _ int) error {
		seen = expr
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, `fmvar("a\"b")`, seen,
		"an escaped quote must not close the quoted segment "+
			"prematurely")
}

// TestFirstWrongLevelMatch_BroadMatcherReturnsMinusOne covers
// the broad-matcher early-return: a `.+` matcher must not pair
// with any wrong-level heading, since the slot has no fixed
// identity and would otherwise consume arbitrary parent-level
// siblings.
func TestFirstWrongLevelMatch_BroadMatcherReturnsMinusOne(t *testing.T) {
	heads := []DocHeading{
		{Level: 2, Text: "Whatever", Line: 5},
	}
	sc := Scope{
		Heading: ".+",
		Matcher: &Matcher{
			Regex:  ".+",
			Repeat: Repeat{Set: true, Min: 0},
		},
	}
	idx := firstWrongLevelMatch(sc, heads, 3, 1, 100, map[int]bool{}, nil)
	assert.Equal(t, -1, idx,
		"a broad matcher must not salvage a wrong-level heading")
}

// TestAnyLaterScopeClaims_SkipsAlreadyClaimed covers the
// claimed-scope skip in anyLaterScopeClaims. A heading whose
// text matches a later already-claimed scope must not be
// reported as a yield target — the claimed-skip is what lets
// step's optional-yield decision ignore scopes whose runs have
// already closed.
func TestAnyLaterScopeClaims_SkipsAlreadyClaimed(t *testing.T) {
	scopes := []Scope{
		{Heading: "A", Matcher: &Matcher{Regex: "A"}},
		{Heading: "B", Matcher: &Matcher{Regex: "B"}},
	}
	dh := DocHeading{Level: 2, Text: "B", Line: 5}
	claimed := map[int]bool{1: true}
	assert.False(t, anyLaterScopeClaims(scopes, 0, dh, claimed, nil),
		"a claimed scope must not register as a yield target")
}

// TestScanScopeRunAtLevel_BreaksOnShallowAfterStart covers the
// `h.Level < expectedLevel && started` early break: once a run
// has consumed at least one match, a heading shallower than
// expectedLevel ends the scan instead of being silently skipped,
// matching matchScope's contiguous-run boundary.
func TestScanScopeRunAtLevel_BreaksOnShallowAfterStart(t *testing.T) {
	heads := []DocHeading{
		{Level: 3, Text: "Step", Line: 5},  // 0: matches, starts run
		{Level: 2, Text: "Done", Line: 10}, // 1: shallower → break
		{Level: 3, Text: "Step", Line: 15}, // 2: same level — but
		// the break above stops the scan before reaching us.
	}
	sc := Scope{
		Heading: "Step",
		Matcher: &Matcher{
			Regex:  "Step",
			Repeat: Repeat{Set: true, Min: 1, Max: 5},
		},
	}
	out := scanScopeRunAtLevel(
		[]Scope{sc}, 0, heads, 3, 1, 100,
		map[int]bool{}, nil)
	assert.Equal(t, []int{0}, out,
		"the run must close at the shallower heading; index 2 "+
			"must not be claimed even though its text matches")
}

// TestResolvePattern_PassesThroughLiterals leaves a pattern without
// interpolations untouched.
func TestResolvePattern_PassesThroughLiterals(t *testing.T) {
	got, err := resolvePattern(`Step [0-9]+`, nil)
	require.NoError(t, err)
	assert.Equal(t, `Step [0-9]+`, got)
}

// TestResolvePattern_RejectsUnknownHelper surfaces the validator's
// runtime-side guard (parse-time is covered separately via
// resolvePatternForCheck).
func TestResolvePattern_RejectsUnknownHelper(t *testing.T) {
	_, err := resolvePattern(`\#(bogus)`, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown helper")
}

// TestCompileMatcher_NilRejected protects against a nil-pointer
// crash if a caller accidentally invokes the helper without a
// matcher.
func TestCompileMatcher_NilRejected(t *testing.T) {
	_, err := compileMatcher(nil, nil)
	require.Error(t, err)
}

// TestSetMatcherRegex_EmptyRejected covers the trim/empty guard.
func TestSetMatcherRegex_EmptyRejected(t *testing.T) {
	raw := map[string]any{
		"sections": []any{
			map[string]any{"heading": map[string]any{"regex": "   "}},
		},
	}
	_, err := ParseInline(raw, "kind x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty pattern")
}

// TestSetMatcherRegex_NonStringRejected covers the type guard.
func TestSetMatcherRegex_NonStringRejected(t *testing.T) {
	raw := map[string]any{
		"sections": []any{
			map[string]any{"heading": map[string]any{"regex": 42}},
		},
	}
	_, err := ParseInline(raw, "kind x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be a string")
}

// TestSetMatcherRepeat_NotAMapping covers the type guard.
func TestSetMatcherRepeat_NotAMapping(t *testing.T) {
	raw := map[string]any{
		"sections": []any{
			map[string]any{"heading": map[string]any{
				"regex":  "X",
				"repeat": "nope",
			}},
		},
	}
	_, err := ParseInline(raw, "kind x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be a mapping")
}

// TestSetMatcherRepeat_UnknownKey covers the unknown-key guard.
func TestSetMatcherRepeat_UnknownKey(t *testing.T) {
	raw := map[string]any{
		"sections": []any{
			map[string]any{"heading": map[string]any{
				"regex":  "X",
				"repeat": map[string]any{"bogus": 1},
			}},
		},
	}
	_, err := ParseInline(raw, "kind x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown key")
}

// TestSetMatcherRepeat_NegativeMin covers the bound-validation
// path inside readIntBound.
func TestSetMatcherRepeat_NegativeMin(t *testing.T) {
	raw := map[string]any{
		"sections": []any{
			map[string]any{"heading": map[string]any{
				"regex":  "X",
				"repeat": map[string]any{"min": -1},
			}},
		},
	}
	_, err := ParseInline(raw, "kind x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "non-negative")
}

// TestScopeMatchesHeading_NilScope returns false rather than
// panicking on the nil-matcher branch.
func TestScopeMatchesHeading_NilScope(t *testing.T) {
	assert.False(t, scopeMatchesHeading(Scope{}, DocHeading{Text: "x"}, nil))
}

// TestCountSameLevelMatches_SkipsDeeperHeadings covers the
// level filter inside countSameLevelMatches via the trailing
// max-exceeded path: a deeper heading between matching same-
// level occurrences must not count toward the scope's max.
func TestCountSameLevelMatches_SkipsDeeperHeadings(t *testing.T) {
	raw := map[string]any{
		"sections": []any{
			map[string]any{"heading": "A"},
			map[string]any{"heading": map[string]any{
				"regex":  "B",
				"repeat": map[string]any{"max": 2},
			}},
		},
	}
	sch, err := ParseInline(raw, "kind x")
	require.NoError(t, err)
	// Doc: B (out of order), nested ### Detail, A, B, B.
	// Three Bs at level 2, with a deeper Detail mixed in. Only
	// the third B (4th match attempt) should be flagged.
	doc := newDocFile(t, "doc.md",
		"# T\n\n## B\n\n### Detail\n\nx\n\n## A\n\ny\n\n## B\n\nz\n\n## B\n\nw\n")
	diags := Validate(doc, sch, nil, false, makeDiagForTest)
	var exceeded int
	for _, d := range diags {
		if strings.Contains(d.Message, "exceeds scope") {
			exceeded++
		}
	}
	assert.Equal(t, 1, exceeded,
		"deeper Detail heading must not count toward B's max")
}

// TestScopeRunIndices_NilMatcherReturnsNil covers the early
// return for a nil-matcher scope (preamble-shaped scope routed
// through the helper directly).
func TestScopeRunIndices_NilMatcherReturnsNil(t *testing.T) {
	scopes := []Scope{{Preamble: true}}
	got := ScopeRunIndices(scopes, 0,
		[]DocHeading{{Level: 2, Text: "X", Line: 1}},
		2, 1, 100, map[int]bool{}, nil)
	assert.Nil(t, got)
}

// TestLaterScopeMatches_SkipsBroadMatcher covers
// laterScopeMatches' broad-matcher skip — a later `.+` scope
// must not count as "more specific" when deciding to yield.
func TestLaterScopeMatches_SkipsBroadMatcher(t *testing.T) {
	scopes := []Scope{
		// Position 0 — the "current" scope (caller pretends).
		{Heading: ".+", Matcher: &Matcher{Regex: ".+"}},
		// Position 1 — broad matcher, should be skipped by
		// laterScopeMatches as "not more specific".
		{Heading: ".+", Matcher: &Matcher{Regex: ".+"}},
	}
	dh := DocHeading{Level: 2, Text: "Anything", Line: 1}
	assert.False(t, laterScopeMatches(scopes, 1, dh, nil),
		"a later broad `.+` matcher must not block yielding")
}

// TestAcronymRanges_MatchesByRegex covers the acronym walker's
// `sc.Matcher.Regex` match branch — when a scope is constructed
// without a separate `Heading` label, the regex itself is the
// scope name the `acronyms.scope:` list compares against.
func TestAcronymRanges_MatchesByRegex(t *testing.T) {
	src := "# Doc\n\n## Check\n\nOIDC undefined.\n"
	f := newDocFile(t, "doc.md", src)
	sch := &Schema{
		Source:    "test",
		RootLevel: 2,
		// Scope built directly so the regex IS the only label —
		// no separate Heading. The acronyms.scope match falls
		// through to the regex.
		Sections: []Scope{
			{Matcher: &Matcher{Regex: "Check"}},
		},
		Acronyms: &AcronymRule{Scope: []string{"Check"}},
	}
	diags := ValidateAcronyms(f, sch, nil, makeDiagForTest)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "OIDC")
}

// TestHandleLeftoverHeadings_SkipsDeeperLevel covers
// handleLeftoverHeadings's `dh.Level != expectedLevel` branch.
// Triggered by a schema with only a preamble entry so the
// matchScope path never runs and the leftover loop processes
// every doc heading itself, including nested ones.
func TestHandleLeftoverHeadings_SkipsDeeperLevel(t *testing.T) {
	raw := map[string]any{
		"sections": []any{
			map[string]any{"heading": nil},
		},
	}
	sch, err := ParseInline(raw, "kind x")
	require.NoError(t, err)
	doc := newDocFile(t, "doc.md",
		"# T\n\n## A\n\n### Nested\n\nx\n")
	// We don't assert specific diagnostics — the test exercises
	// the branch without panicking and produces the expected
	// open-scope silence.
	_ = Validate(doc, sch, nil, false, makeDiagForTest)
}

// TestStep_RequiredMatcherSkipsDeeperBeforeMatch covers
// handleNonMatch's `dh.Level > expectedLevel` branch: a
// required matcher with `consumed == 0 < min` scans past a
// deeper-than-expected heading (e.g. a stray ### before the
// expected ## section) without emitting anything.
func TestStep_RequiredMatcherSkipsDeeperBeforeMatch(t *testing.T) {
	raw := map[string]any{
		"sections": []any{
			map[string]any{"heading": "Goal"},
		},
	}
	sch, err := ParseInline(raw, "kind x")
	require.NoError(t, err)
	// `### Stray` is deeper than the expected H2; matchScope's
	// required-matcher path advances past it silently.
	doc := newDocFile(t, "doc.md",
		"# T\n\n### Stray\n\nx\n\n## Goal\n\ny\n")
	diags := Validate(doc, sch, nil, false, makeDiagForTest)
	for _, d := range diags {
		assert.NotContains(t, d.Message, "Stray",
			"the deeper Stray heading must be skipped silently")
		assert.NotContains(t, d.Message, "missing required",
			"the required Goal must still be claimed")
	}
}

// TestStep_OptionalMatcherFlagsToleratedExtraInClosed covers
// the closed-schema branch where an optional matcher (min=0)
// hasn't yet matched and the current heading is a tolerated
// extra. In a closed schema this emits an "unexpected section"
// diagnostic; the open-schema variant skips silently.
func TestStep_OptionalMatcherFlagsToleratedExtraInClosed(t *testing.T) {
	raw := map[string]any{
		"closed": true,
		"sections": []any{
			map[string]any{
				"heading": map[string]any{
					"regex":  "X",
					"repeat": map[string]any{"min": 0, "max": 1},
				},
			},
		},
	}
	sch, err := ParseInline(raw, "kind x")
	require.NoError(t, err)
	doc := newDocFile(t, "doc.md",
		"# T\n\n## Other\n\nx\n\n## X\n\ny\n")
	diags := Validate(doc, sch, nil, false, makeDiagForTest)
	var unexpected bool
	for _, d := range diags {
		if strings.Contains(d.Message, `## Other: got <present>`) &&
			strings.Contains(d.Message, `not declared in schema`) &&
			strings.Contains(d.Message, `## X`) {
			unexpected = true
		}
	}
	assert.True(t, unexpected,
		"closed schema must flag the tolerated-extra heading "+
			"when an optional matcher hasn't started")
}

// TestSetMatcherRepeat_NegativeMax covers the max-bound branch
// inside setMatcherRepeat / readIntBound: a negative `max:` value
// rejects with a clear message.
func TestSetMatcherRepeat_NegativeMax(t *testing.T) {
	raw := map[string]any{
		"sections": []any{
			map[string]any{"heading": map[string]any{
				"regex":  "X",
				"repeat": map[string]any{"max": -1},
			}},
		},
	}
	_, err := ParseInline(raw, "kind x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "non-negative")
}

// TestSetScopeBool_NonBoolSequential covers setScopeBool's type
// guard via the `sequential:` field inside a heading mapping.
func TestSetScopeBool_NonBoolSequential(t *testing.T) {
	raw := map[string]any{
		"sections": []any{
			map[string]any{"heading": map[string]any{
				"regex":      "X",
				"sequential": "yes",
			}},
		},
	}
	_, err := ParseInline(raw, "kind x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be a boolean")
}

// TestMatcherCache_BoundsGrowth regresses a Copilot review
// concern: the global matcher cache must not grow without bound
// when an LSP session emits a stream of unique `fmvar(...)`
// values. After matcherCacheCap inserts the cache resets so
// memory stays predictable.
func TestMatcherCache_BoundsGrowth(t *testing.T) {
	matcherCacheMu.Lock()
	matcherCache = make(map[matcherCacheKey]*compiledMatcher, matcherCacheCap)
	matcherCacheLen = 0
	matcherCacheMu.Unlock()

	m := &Matcher{Regex: `\#(fmvar(id))`}
	// Fill past the cap with distinct fmvar values so each entry
	// is a fresh cache key.
	for i := 0; i < matcherCacheCap+5; i++ {
		_, err := cachedMatcher(m, map[string]any{
			"id": "id-" + strconv.Itoa(i),
		})
		require.NoError(t, err)
	}
	matcherCacheMu.Lock()
	size := matcherCacheLen
	matcherCacheMu.Unlock()
	assert.LessOrEqual(t, size, matcherCacheCap,
		"matcher cache must stay within the configured cap")
}

// TestIsBroadMatcher covers the helper's three branches.
func TestIsBroadMatcher(t *testing.T) {
	assert.False(t, isBroadMatcher(nil))
	assert.False(t, isBroadMatcher(&Matcher{Regex: "Specific"}))
	assert.True(t, isBroadMatcher(&Matcher{Regex: ".+"}))
	// A bounded-max `.+` is still broad; the helper is
	// intentionally repeat-agnostic.
	assert.True(t, isBroadMatcher(&Matcher{
		Regex:  ".+",
		Repeat: Repeat{Set: true, Min: 1, Max: 2},
	}))
}

// TestScope_Required covers the three branches of Scope.Required.
func TestScope_Required(t *testing.T) {
	// Preamble is never required.
	assert.False(t, Scope{Preamble: true}.Required())
	// Matcher absent → not required.
	assert.False(t, Scope{}.Required())
	// Optional matcher (min=0) → not required.
	opt := Scope{Matcher: &Matcher{
		Regex:  "X",
		Repeat: Repeat{Set: true, Min: 0, Max: 1},
	}}
	assert.False(t, opt.Required())
	// Default cardinality (1..1) → required.
	assert.True(t, literalScope("X").Required())
	// Min=2 → required.
	bounded := Scope{Matcher: &Matcher{
		Regex:  "X",
		Repeat: Repeat{Set: true, Min: 2, Max: 5},
	}}
	assert.True(t, bounded.Required())
}

// TestRepeatBounds covers Repeat.Bounds and Repeat.Optional defaults.
func TestRepeatBounds(t *testing.T) {
	// Unset → (1, 1).
	min, max := Repeat{}.Bounds()
	assert.Equal(t, 1, min)
	assert.Equal(t, 1, max)
	assert.False(t, Repeat{}.Optional())
	// Set with min=0 → optional.
	r := Repeat{Set: true, Min: 0, Max: 0}
	min, max = r.Bounds()
	assert.Equal(t, 0, min)
	assert.Equal(t, 0, max)
	assert.True(t, r.Optional())
}

// TestProtoTokenRegex_AllTokens exercises the four desugaring
// branches the proto.md parser uses.
func TestProtoTokenRegex_AllTokens(t *testing.T) {
	// Literal text — regex-escape only.
	assert.Equal(t, `Step \(One\)`, protoTokenRegex(`Step (One)`))
	// `{n}` → digits helper.
	assert.Equal(t, `Step \#(digits)`, protoTokenRegex(`Step {n}`))
	// `{field}` → fmvar helper.
	assert.Equal(t, `\#(fmvar(id))`, protoTokenRegex(`{id}`))
	// Mixed literal + fmvar.
	assert.Equal(t, `\#(fmvar(id)): \#(fmvar(name))`,
		protoTokenRegex(`{id}: {name}`))
}
