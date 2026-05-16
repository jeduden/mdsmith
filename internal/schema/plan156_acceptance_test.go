package schema

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPlan156_RegexMatchesRenderedPlainText covers the acceptance
// criterion: regex matching is whole-string anchored against
// rendered plain text. `## **Overview**` strips the emphasis
// before matching `regex: 'Overview'`.
func TestPlan156_RegexMatchesRenderedPlainText(t *testing.T) {
	raw := map[string]any{
		"sections": []any{
			map[string]any{"heading": map[string]any{"regex": "Overview"}},
		},
	}
	sch, err := ParseInline(raw, "kind x")
	require.NoError(t, err)
	doc := newDocFile(t, "doc.md", "# T\n\n## **Overview**\n\nx\n")
	diags := Validate(doc, sch, nil, false, makeDiagForTest)
	assert.Empty(t, diags, "emphasis around the heading text should not affect matching")
}

// TestPlan156_DigitsCaptureSequential covers `digits` plus
// `sequential: true`: out-of-order numeric headings emit a
// diagnostic.
func TestPlan156_DigitsCaptureSequential(t *testing.T) {
	raw := map[string]any{
		"sections": []any{
			map[string]any{
				"heading": map[string]any{
					"regex":      `Step \#(digits)`,
					"repeat":     map[string]any{"min": 1},
					"sequential": true,
				},
			},
		},
	}
	sch, err := ParseInline(raw, "kind x")
	require.NoError(t, err)
	doc := newDocFile(t, "doc.md",
		"# T\n\n## Step 1\n\nx\n\n## Step 3\n\ny\n")
	diags := Validate(doc, sch, nil, false, makeDiagForTest)
	var found bool
	for _, d := range diags {
		if strings.Contains(d.Message, "sequential") ||
			strings.Contains(d.Message, "out of order") {
			found = true
		}
	}
	assert.True(t, found, "expected a sequential-violation diagnostic")
}

// TestPlan156_FmvarInterpolates covers `\#(fmvar(name))`
// substitution from the document's frontmatter.
func TestPlan156_FmvarInterpolates(t *testing.T) {
	raw := map[string]any{
		"sections": []any{
			map[string]any{
				"heading": map[string]any{
					"regex": `\#(fmvar(id)): \#(fmvar(name))`,
				},
			},
		},
	}
	sch, err := ParseInline(raw, "kind x")
	require.NoError(t, err)
	doc := newDocFile(t, "doc.md", "# T\n\n## MDS001: line-length\n\nx\n")
	diags := Validate(doc, sch,
		map[string]any{"id": "MDS001", "name": "line-length"},
		false, makeDiagForTest)
	assert.Empty(t, diags, "fmvar should resolve and match the doc heading")
}

// TestPlan156_RepeatBoundsEnforced covers cardinality bounds.
func TestPlan156_RepeatBoundsEnforced(t *testing.T) {
	// `repeat: { min: 2, max: 3 }` requires 2-3 matches.
	raw := map[string]any{
		"sections": []any{
			map[string]any{
				"heading": map[string]any{
					"regex":  `Step \#(digits)`,
					"repeat": map[string]any{"min": 2, "max": 3},
				},
			},
		},
	}
	sch, err := ParseInline(raw, "kind x")
	require.NoError(t, err)
	// Only one Step heading present — falls below min.
	doc := newDocFile(t, "doc.md", "# T\n\n## Step 1\n\nx\n")
	diags := Validate(doc, sch, nil, false, makeDiagForTest)
	require.NotEmpty(t, diags, "min=2 with one match should flag missing")
}

// TestPlan156_RepeatMaxEnforcedInOpenSchema regresses a Copilot
// review finding: `repeat: { max: 1 }` must enforce its upper
// bound regardless of `closed:`. In an open schema, extra matches
// beyond max would otherwise pass through the trailing-leftover
// loop silently.
func TestPlan156_RepeatMaxEnforcedInOpenSchema(t *testing.T) {
	raw := map[string]any{
		"sections": []any{
			map[string]any{
				"heading": map[string]any{
					"regex":  `Step \#(digits)`,
					"repeat": map[string]any{"max": 1},
				},
			},
		},
	}
	sch, err := ParseInline(raw, "kind x")
	require.NoError(t, err)
	doc := newDocFile(t, "doc.md",
		"# T\n\n## Step 1\n\nx\n\n## Step 2\n\ny\n")
	diags := Validate(doc, sch, nil, false, makeDiagForTest)
	var exceeds bool
	for _, d := range diags {
		if strings.Contains(d.Message, "Step 2") &&
			strings.Contains(d.Message, "at most") {
			exceeds = true
		}
	}
	assert.True(t, exceeds,
		"repeat: {max: 1} with 2 matches should flag the second as exceeding max")
}

// TestPlan156_RejectsUnknownInterpHelperAtParseTime regresses a
// Copilot review finding: `resolvePatternForCheck` used to swallow
// errors and accept any non-`digits` interpolation as a probe
// placeholder. Unsupported helpers and unterminated `\#(` must
// fail at parse time.
func TestPlan156_RejectsUnknownInterpHelperAtParseTime(t *testing.T) {
	cases := []struct {
		name, regex, want string
	}{
		{
			name:  "unknown helper",
			regex: `\#(bogus)`,
			want:  `unknown helper "bogus"`,
		},
		{
			name:  "unterminated reference",
			regex: `prefix \#(fmvar(id`,
			want:  "unterminated",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw := map[string]any{
				"sections": []any{
					map[string]any{"heading": map[string]any{"regex": tc.regex}},
				},
			}
			_, err := ParseInline(raw, "kind x")
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}

// TestPlan156_RepeatYieldsToOptionalLaterScope regresses a Copilot
// finding: a bounded greedy matcher must yield to any later listed
// scope (required or optional) that the heading text matches,
// instead of flagging that heading as exceeding `max`. Before the
// fix, claimsLaterLiteral skipped optional scopes, so a greedy
// `.+` matcher with `max: 2` followed by an optional listed entry
// would emit a spurious "matched N times, allowed at most 2" on
// the optional heading.
func TestPlan156_RepeatYieldsToOptionalLaterScope(t *testing.T) {
	raw := map[string]any{
		"sections": []any{
			map[string]any{
				"heading": map[string]any{
					"regex":  ".+",
					"repeat": map[string]any{"min": 1, "max": 2},
				},
			},
			map[string]any{
				"heading": map[string]any{
					"regex":  "Optional",
					"repeat": map[string]any{"min": 0, "max": 1},
				},
			},
		},
	}
	sch, err := ParseInline(raw, "kind x")
	require.NoError(t, err)
	doc := newDocFile(t, "doc.md",
		"# T\n\n## A\n\nx\n\n## B\n\ny\n\n## Optional\n\nz\n")
	diags := Validate(doc, sch, nil, false, makeDiagForTest)
	for _, d := range diags {
		assert.NotContains(t, d.Message, "Optional",
			"the optional scope must claim its heading; "+
				"the greedy matcher must not flag it as exceeding max")
	}
}

// TestPlan156_SequentialDiagOnPartialRun regresses a Copilot
// finding: matchScope used to skip the sequential-numbering check
// when the run terminated by hitting a non-matching heading (not
// by EOF or max). `Step 1, Step 3, Summary` must still emit a
// sequential violation.
func TestPlan156_SequentialDiagOnPartialRun(t *testing.T) {
	raw := map[string]any{
		"sections": []any{
			map[string]any{
				"heading": map[string]any{
					"regex":      `Step \#(digits)`,
					"repeat":     map[string]any{"min": 1},
					"sequential": true,
				},
			},
			map[string]any{"heading": "Summary"},
		},
	}
	sch, err := ParseInline(raw, "kind x")
	require.NoError(t, err)
	doc := newDocFile(t, "doc.md",
		"# T\n\n## Step 1\n\nx\n\n## Step 3\n\ny\n\n## Summary\n\nz\n")
	diags := Validate(doc, sch, nil, false, makeDiagForTest)
	var found bool
	for _, d := range diags {
		if strings.Contains(d.Message, "sequential") ||
			strings.Contains(d.Message, "out of order") {
			found = true
		}
	}
	assert.True(t, found,
		"sequential gap must be diagnosed even when the run ends before EOF/max")
}

// TestPlan156_FmFingerprintCollisionGuard regresses a Copilot
// review concern: a naive `name=value;` join can collide when
// values contain `;` or `=name=`. Two distinct frontmatter values
// must produce distinct fingerprints so the cache returns the
// right compiled regex.
func TestPlan156_FmFingerprintCollisionGuard(t *testing.T) {
	pattern := `\#(fmvar(a)): \#(fmvar(b))`
	// Two FMs whose naive concatenation `a=x;b=y;b=z;` could
	// match: the values themselves contain the separator chars.
	first := fmFingerprint(pattern, map[string]any{
		"a": "x;b=y",
		"b": "z",
	})
	second := fmFingerprint(pattern, map[string]any{
		"a": "x",
		"b": "y;b=z",
	})
	assert.NotEqual(t, first, second,
		"fmFingerprint must distinguish values that contain `;` or `=`")
}

// TestPlan156_RepeatSpansDeeperHeadings regresses a Copilot
// review finding: a repeat run must skip deeper-than-expected
// headings instead of terminating on them. Without this, a
// repeated section with nested children (e.g. `## Step 1`
// followed by `### Details` and then `## Step 2`) only counts
// the first step.
func TestPlan156_RepeatSpansDeeperHeadings(t *testing.T) {
	raw := map[string]any{
		"closed": true,
		"sections": []any{
			map[string]any{
				"heading": map[string]any{
					"regex":      `Step \#(digits)`,
					"repeat":     map[string]any{"min": 1, "max": 5},
					"sequential": true,
				},
			},
		},
	}
	sch, err := ParseInline(raw, "kind x")
	require.NoError(t, err)
	// A nested H3 between two H2 repeats. Step 2 must be counted
	// by the run (not surface as unexpected) and the sequential
	// check must see [1, 2] (not just [1]).
	doc := newDocFile(t, "doc.md",
		"# T\n\n## Step 1\n\n### Details\n\nx\n\n## Step 2\n\ny\n")
	diags := Validate(doc, sch, nil, false, makeDiagForTest)
	for _, d := range diags {
		assert.NotContains(t, d.Message, `unexpected section "## Step 2"`,
			"## Step 2 must not surface as unexpected; the run "+
				"must skip the deeper `### Details` heading")
	}
}

// TestPlan156_FlagExtrasBeyondMaxSkipsDeeper regresses a Copilot
// finding: the max-extras scan must skip headings deeper than the
// expected level instead of treating them as the boundary. A
// repeated section with `### Detail` between the max'd run and a
// later same-level extra would otherwise let the extra through
// silently in an open schema.
func TestPlan156_FlagExtrasBeyondMaxSkipsDeeper(t *testing.T) {
	raw := map[string]any{
		"sections": []any{
			map[string]any{
				"heading": map[string]any{
					"regex":  "Step",
					"repeat": map[string]any{"max": 1},
				},
			},
		},
	}
	sch, err := ParseInline(raw, "kind x")
	require.NoError(t, err)
	doc := newDocFile(t, "doc.md",
		"# T\n\n## Step\n\n### Detail\n\nx\n\n## Step\n\ny\n")
	diags := Validate(doc, sch, nil, false, makeDiagForTest)
	var exceeds bool
	for _, d := range diags {
		if strings.Contains(d.Message, "allowed at most") {
			exceeds = true
		}
	}
	assert.True(t, exceeds,
		"second Step must be flagged even when a deeper heading "+
			"separates it from the first occurrence")
}

// TestPlan156_RejectsMultipleDigitsInline regresses a Copilot
// finding: the matcher runtime reads only the first `n` capture,
// so multiple `\#(digits)` helpers in one pattern make later
// numbers silently ignored. The parser must reject this.
func TestPlan156_RejectsMultipleDigitsInline(t *testing.T) {
	raw := map[string]any{
		"sections": []any{
			map[string]any{"heading": map[string]any{
				"regex": `Step \#(digits) of \#(digits)`,
			}},
		},
	}
	_, err := ParseInline(raw, "kind x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "digits")
	assert.Contains(t, err.Error(), "once")
}

// TestPlan156_OptionalSpecificClaimsOwnSlot regresses a Copilot
// finding: a heading that matches an earlier optional specific
// matcher must claim that scope, not yield to a later broad
// (`.+`) matcher. The previous fix made claimsLaterLiteral
// include optional scopes — necessary for the wildcard-slot
// case — but the yield should only fire when the later
// matcher is *narrower* than the current one (i.e. not a
// broad `.+` regex).
func TestPlan156_OptionalSpecificClaimsOwnSlot(t *testing.T) {
	raw := map[string]any{
		"sections": []any{
			map[string]any{
				"heading": map[string]any{
					"regex":  "Overview",
					"repeat": map[string]any{"min": 0, "max": 1},
				},
				"content": []any{
					map[string]any{"kind": "code-block", "lang": "yaml"},
				},
			},
			map[string]any{
				"heading": map[string]any{
					"regex":  ".+",
					"repeat": map[string]any{"min": 1},
				},
			},
		},
	}
	sch, err := ParseInline(raw, "kind x")
	require.NoError(t, err)
	// Overview is present but missing its required code block.
	// If the broad `.+` matcher claims the heading first, the
	// content check is silenced. We want the missing-content
	// diagnostic to fire.
	doc := newDocFile(t, "doc.md",
		"# T\n\n## Overview\n\nNo code block here.\n\n## Body\n\nx\n")
	diags := Validate(doc, sch, nil, false, makeDiagForTest)
	var contentMissing bool
	for _, d := range diags {
		if strings.Contains(d.Message, "missing required content") &&
			strings.Contains(d.Message, "Overview") {
			contentMissing = true
		}
	}
	assert.True(t, contentMissing,
		"the optional specific scope must claim its own heading; "+
			"a broad `.+` matcher must not absorb it")
}

// TestPlan156_FlagsExtrasMatchingClaimedScope regresses a
// Copilot finding: when an out-of-order claim takes one
// occurrence of a scope and another doc heading matches the
// same scope, the late-claim path used to flag the second as a
// generic "unexpected" (or silently accept it in open
// schemas). The trailing loop now emits a specific
// "exceeds scope's allowed occurrences" diagnostic so the user
// sees which scope was over-filled.
func TestPlan156_FlagsExtrasMatchingClaimedScope(t *testing.T) {
	raw := map[string]any{
		"sections": []any{
			map[string]any{"heading": "A"},
			map[string]any{"heading": map[string]any{
				"regex":  "B",
				"repeat": map[string]any{"max": 1},
			}},
		},
	}
	sch, err := ParseInline(raw, "kind x")
	require.NoError(t, err)
	// Doc: B (claimed late by out-of-order claim against A),
	// A, B again. The second B is the extra occurrence; the
	// late-claim path absorbed only the first B.
	doc := newDocFile(t, "doc.md",
		"# T\n\n## B\n\nx\n\n## A\n\ny\n\n## B\n\nz\n")
	diags := Validate(doc, sch, nil, false, makeDiagForTest)
	var exceeded bool
	for _, d := range diags {
		if strings.Contains(d.Message, "exceeds scope") &&
			strings.Contains(d.Message, "B") {
			exceeded = true
		}
	}
	assert.True(t, exceeded,
		"the second B must surface as exceeding the late-claimed "+
			"scope, not as a generic unexpected section")
}

// TestPlan156_OutOfOrderSequentialDiagFires regresses a Copilot
// finding: when a scope with `sequential: true` is claimed
// out-of-order, the run-style sequential check can't be
// retraced. claimOutOfOrder now emits a dedicated diagnostic
// pointing the author at the in-order schema position.
func TestPlan156_OutOfOrderSequentialDiagFires(t *testing.T) {
	raw := map[string]any{
		"sections": []any{
			map[string]any{"heading": "A"},
			map[string]any{"heading": map[string]any{
				"regex":      `Step \#(digits)`,
				"sequential": true,
			}},
		},
	}
	sch, err := ParseInline(raw, "kind x")
	require.NoError(t, err)
	// Step appears before A — out of order, sequential set.
	doc := newDocFile(t, "doc.md",
		"# T\n\n## Step 1\n\nx\n\n## A\n\ny\n")
	diags := Validate(doc, sch, nil, false, makeDiagForTest)
	var seq bool
	for _, d := range diags {
		if strings.Contains(d.Message, "sequential ordering is not enforced") {
			seq = true
		}
	}
	assert.True(t, seq,
		"out-of-order claim of a sequential scope must surface "+
			"the unenforced-sequential diagnostic")
}

// TestPlan156_OverlappingMatcherDoesNotInflateClaimCount
// regresses a Copilot finding: an earlier `regex: 'A|B'` scope
// that yields a `B` heading to a later named `B` scope used to
// have that yielded heading counted toward its own claim total
// because the trailing pass re-scanned by regex. Now the count
// tracks actual claims, so the early scope's `max` isn't
// over-counted.
func TestPlan156_OverlappingMatcherDoesNotInflateClaimCount(t *testing.T) {
	raw := map[string]any{
		"sections": []any{
			map[string]any{
				"heading": map[string]any{
					"regex":  "A|B",
					"repeat": map[string]any{"min": 1, "max": 2},
				},
			},
			map[string]any{"heading": "B"},
		},
	}
	sch, err := ParseInline(raw, "kind x")
	require.NoError(t, err)
	// Doc: A, B, A. The first scope claims A (counted = 1),
	// yields B to the named B scope, then sees the second A.
	// The trailing A should be its 2nd claim — within max=2.
	doc := newDocFile(t, "doc.md",
		"# T\n\n## A\n\nx\n\n## B\n\ny\n\n## A\n\nz\n")
	diags := Validate(doc, sch, nil, false, makeDiagForTest)
	for _, d := range diags {
		assert.NotContains(t, d.Message, "exceeds scope",
			"yielded headings must not inflate the first scope's claim count")
	}
}

// TestPlan156_BoundedMaxExceededOutOfOrder regresses a Copilot
// finding: the recovery path used to skip already-claimed
// scopes with `max > 1`, so a `repeat: { max: 2 }` scope
// claimed out of order could silently absorb three or more
// occurrences in open schemas. The trailing pass now counts
// same-level matches in the parent window and flags the
// excess (only) when count > max.
func TestPlan156_BoundedMaxExceededOutOfOrder(t *testing.T) {
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
	// Doc: B (out of order), A, B, B. Three Bs total; max=2.
	// The third B must be flagged.
	doc := newDocFile(t, "doc.md",
		"# T\n\n## B\n\nx\n\n## A\n\ny\n\n## B\n\nz\n\n## B\n\nw\n")
	diags := Validate(doc, sch, nil, false, makeDiagForTest)
	var exceededCount int
	for _, d := range diags {
		if strings.Contains(d.Message, "exceeds scope") {
			exceededCount++
		}
	}
	assert.Equal(t, 1, exceededCount,
		"only the third B should be flagged as exceeding max=2 "+
			"(the first two are within bounds)")
}

// TestPlan156_RejectsUserNamedCaptureN regresses a Copilot
// finding: the matcher runtime reads the named capture `n` for
// sequential ordering, but `regex:` is raw RE2 and a user
// could write their own `(?P<n>...)` group that would collide.
// Reject the reserved capture name at parse time.
func TestPlan156_RejectsUserNamedCaptureN(t *testing.T) {
	raw := map[string]any{
		"sections": []any{
			map[string]any{"heading": map[string]any{
				"regex": `(?P<n>[a-z]+) Step \#(digits)`,
			}},
		},
	}
	_, err := ParseInline(raw, "kind x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reserved by")
	assert.Contains(t, err.Error(), "`\\#(digits)`")
}

// TestPlan156_NonBroadRunYieldsAfterMin regresses a Copilot
// finding: the per-scope walkers used to yield only broad
// matchers, but matchScope yields any matcher (broad or not)
// to a later named scope once `min` is satisfied. A scope like
// `regex: 'A|B', repeat: { min: 1 }` followed by a `B` scope
// must let `## B` belong to the named B scope so that scope's
// content / rule / acronym checks fire on its body.
func TestPlan156_NonBroadRunYieldsAfterMin(t *testing.T) {
	raw := map[string]any{
		"sections": []any{
			map[string]any{
				"heading": map[string]any{
					"regex":  "A|B",
					"repeat": map[string]any{"min": 1},
				},
			},
			map[string]any{
				"heading": "B",
				"content": []any{
					map[string]any{"kind": "code-block", "lang": "yaml"},
				},
			},
		},
	}
	sch, err := ParseInline(raw, "kind x")
	require.NoError(t, err)
	// Doc: A, B. The first run claims A (min=1 satisfied) then
	// yields B to the named scope; B is missing its required
	// code block, so the content check fires.
	doc := newDocFile(t, "doc.md",
		"# T\n\n## A\n\nx\n\n## B\n\nno code here\n")
	diags := Validate(doc, sch, nil, false, makeDiagForTest)
	var contentMissing bool
	for _, d := range diags {
		if strings.Contains(d.Message, "missing required content") &&
			strings.Contains(d.Message, "B") {
			contentMissing = true
		}
	}
	assert.True(t, contentMissing,
		"the named B scope must claim its heading and run its "+
			"content check once the earlier run has satisfied min")
}

// TestPlan156_OptionalYieldsToBroadFollower regresses a Copilot
// finding: when an optional scope (`repeat: { min: 0 }`) does
// not match the current heading, the step path used to skip
// the heading if no non-broad later scope claimed it. That
// consumed headings a later broad matcher needed, so a schema
// like [A optional, .+ min=1] against doc [Body] reported the
// broad scope as missing. The unmatched-optional yield check
// must include broad matchers.
func TestPlan156_OptionalYieldsToBroadFollower(t *testing.T) {
	raw := map[string]any{
		"sections": []any{
			map[string]any{
				"heading": map[string]any{
					"regex":  "A",
					"repeat": map[string]any{"min": 0, "max": 1},
				},
			},
			map[string]any{
				"heading": map[string]any{
					"regex":  ".+",
					"repeat": map[string]any{"min": 1},
				},
			},
		},
	}
	sch, err := ParseInline(raw, "kind x")
	require.NoError(t, err)
	doc := newDocFile(t, "doc.md", "# T\n\n## Body\n\nx\n")
	diags := Validate(doc, sch, nil, false, makeDiagForTest)
	for _, d := range diags {
		assert.NotContains(t, d.Message, "missing required",
			"the broad matcher must claim Body; optional A "+
				"yields to it instead of consuming Body silently")
	}
}

// TestPlan156_ScopeRunStopsAtBoundary regresses a Copilot
// finding: per-scope walkers (content, rules, acronyms) used
// to scan the whole parent window for matches, so a repeated
// `Step` scope would silently apply its content / rule
// overrides to a `## Step` that appeared AFTER a `## Summary`
// boundary, even though the structural validator stopped the
// run at Summary. The walkers now use ScopeRunIndices which
// mirrors matchScope's contiguous-run semantics.
func TestPlan156_ScopeRunStopsAtBoundary(t *testing.T) {
	raw := map[string]any{
		"sections": []any{
			map[string]any{
				"heading": map[string]any{
					"regex":  "Step",
					"repeat": map[string]any{"min": 1},
				},
				"content": []any{
					map[string]any{"kind": "code-block", "lang": "yaml"},
				},
			},
			map[string]any{"heading": "Summary"},
		},
	}
	sch, err := ParseInline(raw, "kind x")
	require.NoError(t, err)
	// Two Step sections (each with the required code block),
	// then Summary, then a third Step WITHOUT the code block.
	// The third Step is not part of the run — its missing
	// code-block should NOT be flagged.
	doc := newDocFile(t, "doc.md",
		"# T\n\n"+
			"## Step\n\n```yaml\nfoo: bar\n```\n\n"+
			"## Step\n\n```yaml\nbaz: qux\n```\n\n"+
			"## Summary\n\nsummary text\n\n"+
			"## Step\n\nno code block here\n")
	diags := Validate(doc, sch, nil, false, makeDiagForTest)
	for _, d := range diags {
		assert.NotContains(t, d.Message, "missing required content",
			"the third Step is outside the structural run; "+
				"its content constraint must not fire")
	}
}

// TestPlan156_FlagsExtrasInIterationStream regresses a Copilot
// finding: when a doc has [B, B, A] against schema [A, B], the
// second B used to be silently consumed by handleNonMatch
// (findOutOfOrderIdx skips claimed scopes, open schema → no
// diag). The handler now consults claimedScopeMatches and
// emits "exceeds allowed occurrences" for the duplicate.
func TestPlan156_FlagsExtrasInIterationStream(t *testing.T) {
	raw := map[string]any{
		"sections": []any{
			map[string]any{"heading": "A"},
			map[string]any{"heading": "B"},
		},
	}
	sch, err := ParseInline(raw, "kind x")
	require.NoError(t, err)
	doc := newDocFile(t, "doc.md",
		"# T\n\n## B\n\nx\n\n## B\n\ny\n\n## A\n\nz\n")
	diags := Validate(doc, sch, nil, false, makeDiagForTest)
	var exceeded bool
	for _, d := range diags {
		if strings.Contains(d.Message, "exceeds scope") {
			exceeded = true
		}
	}
	assert.True(t, exceeded,
		"the second B must be flagged as exceeding scope B's "+
			"allowed occurrences, not silently consumed")
}

// TestPlan156_UnboundedRepeatDoesNotFlagAsExceeded regresses a
// Copilot finding: claimedScopeMatches used to fire for any
// claimed scope, including unbounded matchers (`max == 0`). A
// later non-contiguous occurrence of a `repeat: { min: 1 }`
// scope must NOT surface as "exceeds allowed occurrences" —
// the matcher has no upper bound.
func TestPlan156_UnboundedRepeatDoesNotFlagAsExceeded(t *testing.T) {
	raw := map[string]any{
		"sections": []any{
			map[string]any{
				"heading": map[string]any{
					"regex":  "Step",
					"repeat": map[string]any{"min": 1},
				},
			},
			map[string]any{"heading": "Summary"},
		},
	}
	sch, err := ParseInline(raw, "kind x")
	require.NoError(t, err)
	// Two Step sections sandwiching Summary. The second Step
	// is non-contiguous; the unbounded matcher has no max to
	// exceed.
	doc := newDocFile(t, "doc.md",
		"# T\n\n## Step\n\nx\n\n## Summary\n\ny\n\n## Step\n\nz\n")
	diags := Validate(doc, sch, nil, false, makeDiagForTest)
	for _, d := range diags {
		assert.NotContains(t, d.Message, "exceeds scope",
			"an unbounded repeat must not produce a max-exceeded "+
				"diagnostic on a non-contiguous occurrence")
	}
}

// TestPlan156_BroadMatcherYieldsBeforeMin regresses a Copilot
// finding: a broad matcher (`.+`) with `repeat: { min: 2 }`
// used to consume a later named scope's heading to satisfy its
// own min, leaving the named scope flagged as missing. The
// yield check must fire for broad matchers regardless of
// consumed/min so a heading the user named separately always
// claims its named scope.
func TestPlan156_BroadMatcherYieldsBeforeMin(t *testing.T) {
	raw := map[string]any{
		"sections": []any{
			map[string]any{
				"heading": map[string]any{
					"regex":  ".+",
					"repeat": map[string]any{"min": 2},
				},
			},
			map[string]any{"heading": "Named"},
		},
	}
	sch, err := ParseInline(raw, "kind x")
	require.NoError(t, err)
	doc := newDocFile(t, "doc.md",
		"# T\n\n## Body\n\nx\n\n## Named\n\ny\n")
	diags := Validate(doc, sch, nil, false, makeDiagForTest)
	for _, d := range diags {
		assert.NotContains(t, d.Message, `missing required section "## Named"`,
			"the named scope must claim its own heading; "+
				"a broad min=2 matcher must not steal it")
	}
}

// TestPlan156_LateClaimFlagsRepeatMin regresses a Copilot
// finding: when a repeated scope (`repeat.min > 1`) appears
// out-of-order with only one occurrence, the late-claim
// recovery path must still surface a "required at least N"
// diagnostic so the cardinality contract is enforced.
func TestPlan156_LateClaimFlagsRepeatMin(t *testing.T) {
	raw := map[string]any{
		"sections": []any{
			map[string]any{"heading": "A"},
			map[string]any{"heading": map[string]any{
				"regex":  "B",
				"repeat": map[string]any{"min": 2},
			}},
		},
	}
	sch, err := ParseInline(raw, "kind x")
	require.NoError(t, err)
	// B appears once before A — out of order, and only one
	// occurrence so the repeat.min is not satisfied.
	doc := newDocFile(t, "doc.md",
		"# T\n\n## B\n\nx\n\n## A\n\ny\n")
	diags := Validate(doc, sch, nil, false, makeDiagForTest)
	var card bool
	for _, d := range diags {
		if strings.Contains(d.Message, "required at least 2") {
			card = true
		}
	}
	assert.True(t, card,
		"the late-claim path must flag the repeat.min shortfall")
}

// TestPlan156_WrongLevelMatchCountsTowardRepeat regresses a
// Copilot finding: a wrong-level match used to go through a
// claimRun helper that did not update consumed/digits, so a
// repeated matcher could be silently satisfied by a single
// wrong-level occurrence without `repeat.min` or `sequential`
// enforcement. The path now routes through claimMatch so the
// run accounting still fires.
func TestPlan156_WrongLevelMatchCountsTowardRepeat(t *testing.T) {
	raw := map[string]any{
		"sections": []any{
			map[string]any{"heading": "Outer", "sections": []any{
				map[string]any{
					"heading": map[string]any{
						"regex":  "Inner",
						"repeat": map[string]any{"min": 2},
					},
				},
			}},
		},
	}
	sch, err := ParseInline(raw, "kind x")
	require.NoError(t, err)
	// Outer at H2; only one Inner at H2 (wrong level — expected
	// H3). The wrong-level match counts as one occurrence, so the
	// run accounting must emit "matched 1 times, required at
	// least 2".
	doc := newDocFile(t, "doc.md",
		"# T\n\n## Outer\n\n## Inner\n\nx\n")
	diags := Validate(doc, sch, nil, false, makeDiagForTest)
	var shortRun bool
	for _, d := range diags {
		if strings.Contains(d.Message, "required at least 2") {
			shortRun = true
		}
	}
	assert.True(t, shortRun,
		"wrong-level match must contribute to the run's count "+
			"so repeat.min still fires")
}

// TestPlan156_RejectsInvalidFmvarPath regresses a Copilot
// finding: `fmvar(name)` accepted any argument string at parse
// time, so a typo like `fmvar(my-key)` (unquoted hyphenated key)
// only surfaced as a confusing missing-section diagnostic at
// validate-time. Parse-time validation now applies the same CUE
// path rules as the runtime lookup.
func TestPlan156_RejectsInvalidFmvarPath(t *testing.T) {
	raw := map[string]any{
		"sections": []any{
			map[string]any{"heading": map[string]any{
				"regex": `\#(fmvar(my-key))`,
			}},
		},
	}
	_, err := ParseInline(raw, "kind x")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fmvar(my-key)")
	assert.Contains(t, err.Error(), "non-identifier keys must be quoted")
}

// TestPlan156_OptionalMatcherSkipsTolerated regresses a Copilot
// low-confidence finding: an optional matcher (`repeat.min == 0`)
// stopped at the first non-matching heading and left later
// in-order occurrences to be flagged as out-of-order by the
// leftover pass. The matcher should scan past tolerated extras
// in open schemas the same way a required matcher does, only
// terminating once it has actually started a run.
func TestPlan156_OptionalMatcherSkipsTolerated(t *testing.T) {
	raw := map[string]any{
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
	// Doc has a tolerated extra before X. The open-schema walk
	// should silently consume Other and then claim X normally.
	doc := newDocFile(t, "doc.md",
		"# T\n\n## Other\n\nx\n\n## X\n\ny\n")
	diags := Validate(doc, sch, nil, false, makeDiagForTest)
	for _, d := range diags {
		assert.NotContains(t, d.Message, "out of order",
			"optional X must claim itself even after a tolerated extra")
	}
}

// TestPlan156_BroadRepeatYieldsAcronymScope covers the acronym
// walker's broad-matcher yield: an acronym-scoped named section
// after a broad repeated matcher must still get its body
// scanned, not have the broad matcher absorb its heading.
func TestPlan156_BroadRepeatYieldsAcronymScope(t *testing.T) {
	raw := map[string]any{
		"sections": []any{
			map[string]any{
				"heading": map[string]any{
					"regex":  ".+",
					"repeat": map[string]any{"min": 1},
				},
			},
			map[string]any{"heading": "Diagnosis"},
		},
		"acronyms": map[string]any{
			"scope":      []any{"Diagnosis"},
			"known-safe": []any{},
		},
	}
	sch, err := ParseInline(raw, "kind x")
	require.NoError(t, err)
	doc := newDocFile(t, "doc.md",
		"# T\n\n## Body\n\nIgnored.\n\n## Diagnosis\n\nOIDC undefined.\n")
	diags := ValidateAcronyms(doc, sch, nil, makeDiagForTest)
	require.Len(t, diags, 1,
		"acronym scan must reach the named Diagnosis section")
	assert.Contains(t, diags[0].Message, "OIDC")
}

// TestPlan156_BroadRepeatYieldsToLaterScopeInPerScopeWalkers
// regresses a Copilot finding: the per-scope walkers (rules,
// content, acronyms) must yield to later named scopes the same
// way the structural validator does, otherwise a broad repeated
// matcher silently consumes headings the user named separately.
func TestPlan156_BroadRepeatYieldsToLaterScopeInPerScopeWalkers(t *testing.T) {
	raw := map[string]any{
		"sections": []any{
			map[string]any{
				"heading": map[string]any{
					"regex":  ".+",
					"repeat": map[string]any{"min": 1},
				},
			},
			map[string]any{
				"heading": "Diagnosis",
				"content": []any{
					map[string]any{"kind": "code-block", "lang": "yaml"},
				},
			},
		},
	}
	sch, err := ParseInline(raw, "kind x")
	require.NoError(t, err)
	// Doc has Body + Diagnosis. The broad matcher must yield
	// "Diagnosis" so the content check fires there.
	doc := newDocFile(t, "doc.md",
		"# T\n\n## Body\n\nx\n\n## Diagnosis\n\nNo code block.\n")
	diags := Validate(doc, sch, nil, false, makeDiagForTest)
	var missing bool
	for _, d := range diags {
		if strings.Contains(d.Message, "missing required content") &&
			strings.Contains(d.Message, "Diagnosis") {
			missing = true
		}
	}
	assert.True(t, missing,
		"named Diagnosis scope must claim its heading; "+
			"a broad repeated matcher must not absorb it")
}

// TestPlan156_RejectsMultipleNTokensProto regresses the same
// constraint for proto.md heading rows. `## Step {n} of {n}`
// would expand to two `\#(digits)` helpers; the file parser
// must surface the same rejection as the inline form.
func TestPlan156_RejectsMultipleNTokensProto(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "proto.md", "## Step {n} of {n}\n")
	_, err := ParseFile(&FileReader{}, p)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "{n}")
	assert.Contains(t, err.Error(), "at most once")
}

// TestPlan156_AcronymScopeOnRepeatedScope regresses a Copilot
// review finding: a repeated scope with an acronym-scope name
// must scan every occurrence's range, not just the first.
func TestPlan156_AcronymScopeOnRepeatedScope(t *testing.T) {
	raw := map[string]any{
		"sections": []any{
			map[string]any{
				"heading": map[string]any{
					"regex":  "Diagnosis",
					"repeat": map[string]any{"min": 1, "max": 5},
				},
			},
		},
		"acronyms": map[string]any{
			"scope":      []any{"Diagnosis"},
			"known-safe": []any{},
		},
	}
	sch, err := ParseInline(raw, "kind x")
	require.NoError(t, err)
	doc := newDocFile(t, "doc.md",
		"# T\n\n## Diagnosis\n\nOIDC first.\n\n## Diagnosis\n\nLDAP second.\n")
	diags := ValidateAcronyms(doc, sch, nil, makeDiagForTest)
	var seenOIDC, seenLDAP bool
	for _, d := range diags {
		if strings.Contains(d.Message, "OIDC") {
			seenOIDC = true
		}
		if strings.Contains(d.Message, "LDAP") {
			seenLDAP = true
		}
	}
	assert.True(t, seenOIDC, "first Diagnosis range should report OIDC")
	assert.True(t, seenLDAP,
		"second Diagnosis range must also be scanned for first-use acronyms")
}

// TestPlan156_ContentOnRepeatedScope regresses a Copilot review
// finding: content constraints on a repeated scope must fire for
// every occurrence, not just the first.
func TestPlan156_ContentOnRepeatedScope(t *testing.T) {
	raw := map[string]any{
		"sections": []any{
			map[string]any{
				"heading": map[string]any{
					"regex":  "Step",
					"repeat": map[string]any{"min": 1, "max": 5},
				},
				"content": []any{
					map[string]any{"kind": "code-block", "lang": "yaml"},
				},
			},
		},
	}
	sch, err := ParseInline(raw, "kind x")
	require.NoError(t, err)
	// Two Step sections; only the first has a code block.
	doc := newDocFile(t, "doc.md",
		"# T\n\n## Step\n\n```yaml\nfoo: bar\n```\n\n## Step\n\nNo code block here.\n")
	diags := Validate(doc, sch, nil, false, makeDiagForTest)
	var contentMisses int
	for _, d := range diags {
		if strings.Contains(d.Message, "missing required content") {
			contentMisses++
		}
	}
	assert.Equal(t, 1, contentMisses,
		"the second Step's missing code-block must be flagged "+
			"even though the first occurrence satisfied the constraint")
}

// TestPlan156_RulesOnRepeatedScope regresses a Copilot review
// finding: per-scope rule overrides on a repeated scope must
// apply to every occurrence.
func TestPlan156_RulesOnRepeatedScope(t *testing.T) {
	// Build the rule directly so the test stays inside the
	// schema package — the override fires when the per-scope
	// walker visits more than one occurrence. We can't invoke
	// the MDS020 rule engine from here, but we can observe the
	// walker visits via the acronym range count, which mirrors
	// the per-scope walker's visit-count semantics.
	raw := map[string]any{
		"sections": []any{
			map[string]any{
				"heading": map[string]any{
					"regex":  "Diagnosis",
					"repeat": map[string]any{"min": 1, "max": 5},
				},
			},
		},
		"acronyms": map[string]any{
			"scope":      []any{"Diagnosis"},
			"known-safe": []any{},
		},
	}
	sch, err := ParseInline(raw, "kind x")
	require.NoError(t, err)
	doc := newDocFile(t, "doc.md",
		"# T\n\n## Diagnosis\n\nOIDC first.\n\n## Diagnosis\n\nSAML second.\n")
	diags := ValidateAcronyms(doc, sch, nil, makeDiagForTest)
	// Each range produces its own first-use diagnostic; two
	// occurrences => two diagnostics.
	assert.Len(t, diags, 2,
		"each repeated occurrence must contribute its own range "+
			"so per-scope checks fire on every occurrence")
}

// TestPlan156_AcronymScopeMatchesByHeadingText regresses a
// Copilot review finding: a disjunctive matcher like
// `regex: 'Symptoms|Indicators'` paired with
// `acronyms.scope: ["Indicators"]` must scan the matched
// `## Indicators` body. Plan 156 dropped the scope-level
// `aliases:` field, so the walker must compare the configured
// scope name against the actual matched heading text (not just
// the schema label or the raw regex body).
func TestPlan156_AcronymScopeMatchesByHeadingText(t *testing.T) {
	raw := map[string]any{
		"sections": []any{
			map[string]any{
				"heading": map[string]any{"regex": "Symptoms|Indicators"},
			},
		},
		"acronyms": map[string]any{
			"scope":      []any{"Indicators"},
			"known-safe": []any{},
		},
	}
	sch, err := ParseInline(raw, "kind x")
	require.NoError(t, err)
	doc := newDocFile(t, "doc.md",
		"# T\n\n## Indicators\n\nOIDC first.\n")
	diags := ValidateAcronyms(doc, sch, nil, makeDiagForTest)
	var seenOIDC bool
	for _, d := range diags {
		if strings.Contains(d.Message, "OIDC") {
			seenOIDC = true
		}
	}
	assert.True(t, seenOIDC,
		"acronyms.scope must match the actual matched heading text, "+
			"not just the schema label or the raw regex body")
}

// TestPlan156_OutOfOrderRunCountsAvailableMatches regresses a
// Copilot review finding: when an out-of-order recovery occurs on
// a scope with `repeat.min > 1`, the diagnostic must count the
// available run of consecutive matches rather than always
// emitting "matched 1 times". Otherwise a document that contains
// enough occurrences of B (just before A) gets a spurious
// "matched 1 times, required at least 2" diagnostic on top of
// the legitimate out-of-order message.
func TestPlan156_OutOfOrderRunCountsAvailableMatches(t *testing.T) {
	raw := map[string]any{
		"sections": []any{
			map[string]any{"heading": "A"},
			map[string]any{
				"heading": map[string]any{
					"regex":  "B",
					"repeat": map[string]any{"min": 2, "max": 5},
				},
			},
		},
	}
	sch, err := ParseInline(raw, "kind x")
	require.NoError(t, err)
	// Two B's before the expected A — B's min cardinality is
	// satisfied by the run, only the ordering is wrong.
	doc := newDocFile(t, "doc.md",
		"# T\n\n## B\n\n## B\n\n## A\n")
	diags := Validate(doc, sch, nil, false, makeDiagForTest)
	var outOfOrder, falseMin int
	for _, d := range diags {
		if strings.Contains(d.Message, "out of order") {
			outOfOrder++
		}
		if strings.Contains(d.Message, "matched 1 times") {
			falseMin++
		}
	}
	assert.GreaterOrEqual(t, outOfOrder, 1,
		"the out-of-order condition must still be reported")
	assert.Zero(t, falseMin,
		"a contiguous B-run that satisfies min must not get a "+
			"`matched 1 times, required at least 2` diagnostic")
}

// TestPlan156_NonContiguousClaimedScopeFlagged regresses a
// Copilot review finding: a heading that matches an already-
// claimed scope is silently absorbed when it stays within
// `max`. With schema `[A (repeat 1..2), B]` and document
// `A, B, A`, the trailing A is non-contiguous with A's earlier
// run — matcher runs are consecutive, so the open schema
// should still emit a diagnostic instead of letting the
// trailing occurrence pass.
func TestPlan156_NonContiguousClaimedScopeFlagged(t *testing.T) {
	raw := map[string]any{
		"sections": []any{
			map[string]any{
				"heading": map[string]any{
					"regex":  "A",
					"repeat": map[string]any{"min": 1, "max": 2},
				},
			},
			map[string]any{"heading": "B"},
		},
	}
	sch, err := ParseInline(raw, "kind x")
	require.NoError(t, err)
	doc := newDocFile(t, "doc.md",
		"# T\n\n## A\n\n## B\n\n## A\n")
	diags := Validate(doc, sch, nil, false, makeDiagForTest)
	var orderingDiag bool
	for _, d := range diags {
		if strings.Contains(d.Message, "out of order") &&
			strings.Contains(d.Message, "contiguous") {
			orderingDiag = true
		}
	}
	assert.True(t, orderingDiag,
		"a trailing same-scope match after the run closed must be "+
			"flagged as non-contiguous, even within max")
}

// TestPlan156_LateClaimChildRecursionStopsAtParentLevel
// regresses a Copilot review finding: claimLateScope hands
// validateScopes the full remaining doc-heading slice when it
// recurses into the late scope's children. The leftover pass
// inside that child window must break at the first shallower-
// than-child heading so a same-level sibling that follows the
// late section is still visible to the parent's leftover loop.
// Otherwise the trailing duplicate `## A` here would be
// silently absorbed by the child walker instead of getting an
// "exceeds allowed occurrences" diagnostic on the parent A
// scope.
func TestPlan156_LateClaimChildRecursionStopsAtParentLevel(t *testing.T) {
	raw := map[string]any{
		"sections": []any{
			map[string]any{"heading": "A"},
			map[string]any{
				"heading": map[string]any{
					"regex":  "B",
					"repeat": map[string]any{"min": 0, "max": 1},
				},
				"sections": []any{
					map[string]any{"heading": "Detail"},
				},
			},
			map[string]any{"heading": "C"},
		},
	}
	sch, err := ParseInline(raw, "kind x")
	require.NoError(t, err)
	doc := newDocFile(t, "doc.md",
		"# T\n\n## A\n\n## C\n\n## B\n\n### Detail\n\n## A\n")
	diags := Validate(doc, sch, nil, false, makeDiagForTest)
	var lateB, trailingA bool
	for _, d := range diags {
		if strings.Contains(d.Message, `## B: got <out of order>`) &&
			strings.Contains(d.Message, `expected before this position`) {
			lateB = true
		}
		if strings.Contains(d.Message,
			`"## A"`) &&
			(strings.Contains(d.Message, "exceeds") ||
				strings.Contains(d.Message, "out of order")) {
			trailingA = true
		}
	}
	assert.True(t, lateB,
		"late B must still be flagged by claimLateScope")
	assert.True(t, trailingA,
		"trailing ## A at parent level must surface; child "+
			"recursion under B must not consume it")
}

// TestPlan156_CountMatchingRunStopsAtShallowerHeading regresses
// countMatchingRun's early-return on a heading shallower than
// the expected child level. When a nested out-of-order match is
// immediately followed by a parent-level heading, the run-length
// scan must stop at the shallower boundary rather than walking
// past it.
func TestPlan156_CountMatchingRunStopsAtShallowerHeading(t *testing.T) {
	raw := map[string]any{
		"sections": []any{
			map[string]any{
				"heading": "Parent",
				"sections": []any{
					map[string]any{"heading": "A"},
					map[string]any{"heading": "B"},
				},
			},
			map[string]any{"heading": "Sibling"},
		},
	}
	sch, err := ParseInline(raw, "kind x")
	require.NoError(t, err)
	doc := newDocFile(t, "doc.md",
		"# T\n\n## Parent\n\n### B\n\n## Sibling\n")
	diags := Validate(doc, sch, nil, false, makeDiagForTest)
	var sawOO bool
	for _, d := range diags {
		if strings.Contains(d.Message, `### B`) &&
			strings.Contains(d.Message, "out of order") {
			sawOO = true
		}
	}
	assert.True(t, sawOO,
		"### B inside Parent must be flagged as out-of-order "+
			"under A; the run-count must not walk past ## Sibling")
}

// TestPlan156_RunContinuationRecursesChildren regresses a Copilot
// review finding: when claimedScopeMatches absorbs a contiguous
// run-continuation heading (idx >= s.idx, within max), the walker
// must still recurse into the scope's children so every
// occurrence of a repeated scope with required nested sections
// gets its child contract enforced — not just the first one.
func TestPlan156_RunContinuationRecursesChildren(t *testing.T) {
	raw := map[string]any{
		"sections": []any{
			map[string]any{"heading": "Anchor"},
			map[string]any{
				"heading": map[string]any{
					"regex":  "Step",
					"repeat": map[string]any{"min": 1, "max": 5},
				},
				"sections": []any{
					map[string]any{"heading": "Detail"},
				},
			},
		},
	}
	sch, err := ParseInline(raw, "kind x")
	require.NoError(t, err)
	// Two Step occurrences before the listed Anchor — second
	// Step lacks its required Detail child. The first Step is
	// claimed via claimOutOfOrder (and recurses into Detail);
	// the second Step is absorbed as a run continuation and
	// must also recurse so the missing Detail surfaces.
	doc := newDocFile(t, "doc.md",
		"# T\n\n## Step\n\n### Detail\n\n## Step\n\n## Anchor\n")
	diags := Validate(doc, sch, nil, false, makeDiagForTest)
	var missing int
	for _, d := range diags {
		if strings.Contains(d.Message, `### Detail`) &&
			strings.Contains(d.Message, "section to be present") {
			missing++
		}
	}
	assert.GreaterOrEqual(t, missing, 1,
		"the second Step occurrence's missing Detail must surface; "+
			"the run-continuation absorb path must recurse into "+
			"children just like claimMatch does")
}

// TestPlan156_MissingFmvarFailsMatch regresses a Copilot review
// finding: an unresolved `fmvar(name)` used to substitute an
// empty regex fragment, so `regex: 'Topic \#(fmvar(id))'` with
// missing `id` would match a degenerate `## Topic ` heading.
// resolvePattern now errors on a missing fmvar, surfacing the
// section as missing rather than silently matching a partial
// heading.
func TestPlan156_MissingFmvarFailsMatch(t *testing.T) {
	raw := map[string]any{
		"sections": []any{
			map[string]any{
				"heading": map[string]any{
					"regex": `Topic \#(fmvar(id))`,
				},
			},
		},
	}
	sch, err := ParseInline(raw, "kind x")
	require.NoError(t, err)
	// Heading text matches the literal-only remainder of the
	// pattern. With the old behavior this passed silently; the
	// new behavior fails the match because `id` is absent.
	doc := newDocFile(t, "doc.md",
		"# T\n\n## Topic \n\nbody\n")
	diags := Validate(doc, sch, nil, false, makeDiagForTest)
	var sawMissing bool
	for _, d := range diags {
		if strings.Contains(d.Message, "section to be present") {
			sawMissing = true
		}
	}
	assert.True(t, sawMissing,
		"a heading with unresolved fmvar must not match a "+
			"degenerate literal heading; the scope must surface "+
			"as missing instead")
}

// TestPlan156_BroadMatcherIgnoresParentLevelSibling regresses a
// Copilot review finding: when a nested broad matcher
// (`regex: '.+'`) runs out of child headings, it must NOT
// salvage the next parent-level heading via the
// shallower-heading recovery. Otherwise the parent walker's
// next sibling is consumed under the wrong scope and reported
// as a level mismatch.
func TestPlan156_BroadMatcherIgnoresParentLevelSibling(t *testing.T) {
	raw := map[string]any{
		"sections": []any{
			map[string]any{
				"heading": "Parent",
				"sections": []any{
					map[string]any{
						"heading": map[string]any{
							"regex":  ".+",
							"repeat": map[string]any{"min": 0},
						},
					},
				},
			},
			map[string]any{"heading": "Sibling"},
		},
	}
	sch, err := ParseInline(raw, "kind x")
	require.NoError(t, err)
	doc := newDocFile(t, "doc.md",
		"# T\n\n## Parent\n\n## Sibling\n\nbody\n")
	diags := Validate(doc, sch, nil, false, makeDiagForTest)
	var sawLevel bool
	for _, d := range diags {
		if strings.Contains(d.Message, "Sibling") &&
			strings.Contains(d.Message, "got h2") {
			sawLevel = true
		}
	}
	assert.False(t, sawLevel,
		"the nested broad matcher must not salvage `## Sibling` "+
			"as an h3 level-mismatch; the parent walker owns it")
}

// TestPlan156_ScanInterpsHandlesQuotedParens regresses a
// Copilot review finding: scanInterps used to count every `)`
// as closing the interpolation even inside a quoted CUE path
// like `fmvar("release)date")`, truncating the helper. The
// scanner now skips parens inside double-quoted segments.
func TestPlan156_ScanInterpsHandlesQuotedParens(t *testing.T) {
	pattern := `prefix \#(fmvar("release)date")) suffix`
	var seen string
	err := scanInterps(pattern, func(expr string, _, _ int) error {
		seen = expr
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, `fmvar("release)date")`, seen,
		"the parser must keep the quoted argument intact even when "+
			"it contains a literal `)`")
}

// TestPlan156_FmvarAcceptsQuotedCloseParen regresses a Copilot
// review finding: the `fmvar(...)` argument parser used to stop
// at the first `)`, breaking a valid CUE label like
// `fmvar("release)date")` even though scanInterps respected the
// quoted segment. parseFmvarCall now scans past quoted regions
// so the full argument reaches fmvarLookup.
func TestPlan156_FmvarAcceptsQuotedCloseParen(t *testing.T) {
	raw := map[string]any{
		"sections": []any{
			map[string]any{
				"heading": map[string]any{
					"regex": `\#(fmvar("release)date"))`,
				},
			},
		},
	}
	sch, err := ParseInline(raw, "kind x")
	require.NoError(t, err,
		"a quoted CUE label containing `)` must parse cleanly")
	require.Len(t, sch.Sections, 1)
	assert.Equal(t, `\#(fmvar("release)date"))`,
		sch.Sections[0].Matcher.Regex)
}

// TestPlan156_RejectsRealUserNCaptureNotLiteral regresses a
// Copilot review finding: the `(?P<n>...)` reservation used to
// substring-match the literal text, which would reject valid
// regexes that merely contained `(?P<n>` as escaped/character-
// class content. The check now parses the regex via
// regexp/syntax and looks for an actual named capture, so a
// regex like `\(\?P<n>` (a literal-text match of that exact
// string) is now accepted.
func TestPlan156_RejectsRealUserNCaptureNotLiteral(t *testing.T) {
	// A real `(?P<n>...)` collides with the reserved capture and
	// must still be rejected.
	bad := map[string]any{
		"sections": []any{
			map[string]any{
				"heading": map[string]any{
					"regex": `(?P<n>[A-Z]+)`,
				},
			},
		},
	}
	_, err := ParseInline(bad, "kind x")
	require.Error(t, err, "real `(?P<n>...)` capture must be rejected")
	assert.Contains(t, err.Error(), "reserved")

	// An escaped literal `(?P<n>` is not a capture group and
	// must be accepted.
	good := map[string]any{
		"sections": []any{
			map[string]any{
				"heading": map[string]any{
					"regex": `\(\?P<n>doc\)`,
				},
			},
		},
	}
	_, err = ParseInline(good, "kind x")
	assert.NoError(t, err,
		"an escaped literal `(?P<n>` text is not a capture and "+
			"must not trip the reservation check")
}
