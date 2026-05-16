package requiredstructure

import (
	"strings"
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	// Imported for their init-time rule registration; the scope-
	// rule override fixtures rely on these rules being resolvable
	// via rule.ByName at run time.
	_ "github.com/jeduden/mdsmith/internal/rules/blanklinearoundfencedcode"
	_ "github.com/jeduden/mdsmith/internal/rules/forbiddenparagraphstarts"
	_ "github.com/jeduden/mdsmith/internal/rules/forbiddentext"
	_ "github.com/jeduden/mdsmith/internal/rules/linelength"
	_ "github.com/jeduden/mdsmith/internal/rules/requiredmentions"
	_ "github.com/jeduden/mdsmith/internal/rules/requiredtextpatterns"
)

// inlineSchema is a test helper that mirrors how the config merge
// hands an inline schema to the rule: as a YAML-decoded map under
// the rule's `inline-schema` setting.
func inlineSchema(t *testing.T, m map[string]any) *schema.Schema {
	t.Helper()
	sch, err := schema.ParseInline(m, "test inline kind")
	require.NoError(t, err)
	return sch
}

func TestApplySettings_InlineSchemaParses(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{
		"inline-schema": map[string]any{
			"sections": []any{
				map[string]any{"heading": "Overview"},
			},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, r.InlineSchema)
	require.Len(t, r.InlineSchema.Sections, 1)
	assert.Equal(t, "Overview", r.InlineSchema.Sections[0].Heading)
}

func TestApplySettings_InlineSchemaWrongType(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{
		"inline-schema": "not-a-map",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "inline-schema must be a mapping")
}

func TestApplySettings_InlineSchemaInvalid(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{
		"inline-schema": map[string]any{
			"sections": []any{map[string]any{"unknown": true}},
		},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid inline-schema")
}

// TestCheck_InlineSchema_MissingSection is the inline-schema mirror
// of TestCheck_MissingHeading (which uses the legacy file-based
// path). Both must emit the same canonical message text so docs and
// fixtures stay in sync across the two sources.
func TestCheck_InlineSchema_MissingSection(t *testing.T) {
	r := &Rule{InlineSchema: inlineSchema(t, map[string]any{
		"closed": true,
		"sections": []any{
			map[string]any{"heading": "Goal"},
			map[string]any{"heading": "Tasks"},
		},
	})}
	f := newTestFile(t, "doc.md", "# My Plan\n\n## Goal\n\nGoal text.\n")
	diags := r.Check(f)
	expectDiagMsg(t, diags, `## Tasks: got <missing>, expected section to be present`)
}

func TestCheck_InlineSchema_ParityWithFileSchema(t *testing.T) {
	// File-based and inline schemas with equivalent structure must
	// emit the same diagnostic for the same document — this is
	// acceptance criterion #1 of plan 146.
	fileSchema := writeSchema(t, "# ?\n\n## Goal\n\n## Tasks\n")
	rFile := &Rule{Schema: fileSchema}
	rInline := &Rule{InlineSchema: inlineSchema(t, map[string]any{
		"closed": true,
		"sections": []any{
			map[string]any{"heading": "Goal"},
			map[string]any{"heading": "Tasks"},
		},
	})}
	doc := "# Plan\n\n## Goal\n\nx\n"
	fFile := newTestFile(t, "doc.md", doc)
	fInline := newTestFile(t, "doc.md", doc)
	fileDiags := rFile.Check(fFile)
	inlineDiags := rInline.Check(fInline)
	require.Len(t, fileDiags, 1)
	require.Len(t, inlineDiags, 1)
	// The two paths emit identical violation bodies (field /
	// actual / expected); only the trailing schema-reference
	// line differs because one points at a file path and the
	// other at the kind label.
	firstLine := func(s string) string {
		if i := strings.Index(s, "\n"); i >= 0 {
			return s[:i]
		}
		return s
	}
	assert.Equal(t, firstLine(fileDiags[0].Message), firstLine(inlineDiags[0].Message))
}

func TestCheck_InlineSchema_OpenByDefault(t *testing.T) {
	// With no `closed:` field, an inline schema tolerates unlisted
	// headings between listed sections — acceptance criterion #3.
	r := &Rule{InlineSchema: inlineSchema(t, map[string]any{
		"sections": []any{
			map[string]any{"heading": "Symptoms"},
			map[string]any{"heading": "Diagnosis"},
		},
	})}
	f := newTestFile(t, "doc.md",
		"# Runbook\n\n## Symptoms\n\nx\n\n## Notes\n\ny\n\n## Diagnosis\n\nz\n")
	diags := r.Check(f)
	assert.Empty(t, diags, "open scope should not flag the unlisted Notes section")
}

func TestCheck_InlineSchema_ClosedFlagsUnlisted(t *testing.T) {
	r := &Rule{InlineSchema: inlineSchema(t, map[string]any{
		"closed": true,
		"sections": []any{
			map[string]any{"heading": "Symptoms"},
			map[string]any{"heading": "Diagnosis"},
		},
	})}
	f := newTestFile(t, "doc.md",
		"# Runbook\n\n## Symptoms\n\nx\n\n## Notes\n\ny\n\n## Diagnosis\n\nz\n")
	diags := r.Check(f)
	require.NotEmpty(t, diags)
	expectDiagMsg(t, diags, `## Notes: got <present>, expected not declared in schema`)
}

func TestCheck_InlineSchema_WildcardSlot(t *testing.T) {
	r := &Rule{InlineSchema: inlineSchema(t, map[string]any{
		"closed": true,
		"sections": []any{
			map[string]any{"heading": "Overview"},
			map[string]any{"heading": map[string]any{
				"regex":  ".+",
				"repeat": map[string]any{"min": 0},
			}},
			map[string]any{"heading": "References"},
		},
	})}
	f := newTestFile(t, "doc.md",
		"# RFC\n\n## Overview\n\nx\n\n## Decision\n\ny\n\n## References\n\nz\n")
	diags := r.Check(f)
	assert.Empty(t, diags,
		"wildcard slot should tolerate the unlisted Decision section")
}

// TestCheck_InlineSchema_NestedThreeLevels exercises acceptance
// criterion #2 — recursion to at least three levels of depth.
func TestCheck_InlineSchema_NestedThreeLevels(t *testing.T) {
	r := &Rule{InlineSchema: inlineSchema(t, map[string]any{
		"sections": []any{
			map[string]any{
				"heading": "Diagnosis",
				"sections": []any{
					map[string]any{
						"heading": "Step",
						"sections": []any{
							map[string]any{"heading": "Check"},
							map[string]any{"heading": "Expected"},
						},
					},
				},
			},
		},
	})}
	f := newTestFile(t, "doc.md", `# Runbook

## Diagnosis

### Step

#### Check

x

#### Expected

y
`)
	diags := r.Check(f)
	assert.Empty(t, diags, "three-level nested tree should validate cleanly")
}

func TestCheck_InlineSchema_LevelMismatch(t *testing.T) {
	// Acceptance criterion #6: mismatched heading depths flag a
	// diagnostic naming expected vs actual levels.
	r := &Rule{InlineSchema: inlineSchema(t, map[string]any{
		"sections": []any{
			map[string]any{
				"heading": "Diagnosis",
				"sections": []any{
					map[string]any{"heading": "Step"},
				},
			},
		},
	})}
	f := newTestFile(t, "doc.md",
		"# Runbook\n\n## Diagnosis\n\n## Step\n\nx\n")
	diags := r.Check(f)
	// Filter to MDS020 diagnostics.
	var our []lint.Diagnostic
	for _, d := range diags {
		if d.RuleID == "MDS020" {
			our = append(our, d)
		}
	}
	require.NotEmpty(t, our)
	expectDiagMsg(t, our, "Step: got h2")
	expectDiagMsg(t, our, "expected h3")
}

func TestCheck_InlineSchema_DisjunctionMatches(t *testing.T) {
	r := &Rule{InlineSchema: inlineSchema(t, map[string]any{
		"sections": []any{
			map[string]any{
				"heading": map[string]any{"regex": "Symptoms|Indicators"},
			},
		},
	})}
	f := newTestFile(t, "doc.md", "# Runbook\n\n## Indicators\n\nx\n")
	diags := r.Check(f)
	assert.Empty(t, diags, "regex disjunction should match the alternate text")
}

func TestCheck_InlineSchema_FilenamePattern(t *testing.T) {
	r := &Rule{InlineSchema: inlineSchema(t, map[string]any{
		"filename": "RFC-[0-9][0-9][0-9][0-9].md",
	})}
	f := newTestFile(t, "draft.md", "# Draft\n")
	diags := r.Check(f)
	require.Len(t, diags, 1)
	expectDiagMsg(t, diags, `filename: got "draft.md"`)
}

func TestCheck_InlineSchema_FrontmatterCUE(t *testing.T) {
	r := &Rule{InlineSchema: inlineSchema(t, map[string]any{
		"frontmatter": map[string]any{
			"id": `=~"^RFC-[0-9]{4}$"`,
		},
	})}
	// Document FM has the wrong shape.
	f := newTestFile(t, "doc.md",
		"---\nid: NOT-AN-RFC\n---\n# Doc\n")
	diags := r.Check(f)
	require.NotEmpty(t, diags)
	expectDiagMsg(t, diags, `id: got "NOT-AN-RFC"`)
	expectDiagMsg(t, diags, `string matching ^RFC-[0-9]{4}$`)
}

// TestCheck_InlineSchema_ScopeRuleDeterministicOrdering exercises
// the sorted iteration over sc.Rules so unknown-rule and invalid-
// settings diagnostics emit in a stable order regardless of Go map
// iteration randomness. The fixture deliberately provides two
// misconfigured rules so we can observe the order of their
// diagnostics.
func TestCheck_InlineSchema_ScopeRuleDeterministicOrdering(t *testing.T) {
	r := &Rule{InlineSchema: inlineSchema(t, map[string]any{
		"sections": []any{
			map[string]any{
				"heading": "Section",
				"rules": map[string]any{
					"zzz-not-a-rule": map[string]any{},
					"aaa-not-a-rule": map[string]any{},
					"mmm-not-a-rule": map[string]any{},
				},
			},
		},
	})}
	f := newTestFile(t, "doc.md", "# T\n\n## Section\n\nx\n")
	// Run many times; the order of the three unknown-rule messages
	// must be the same on every run.
	var first []string
	for i := 0; i < 20; i++ {
		diags := r.Check(f)
		var names []string
		for _, d := range diags {
			if strings.Contains(d.Message, "unknown rule") {
				names = append(names, d.Message)
			}
		}
		if i == 0 {
			first = names
			require.Equal(t, 3, len(first),
				"expected three unknown-rule diagnostics")
		} else {
			require.Equal(t, first, names,
				"scope rule iteration must be deterministic")
		}
	}
}

// TestCheck_InlineSchema_ScopeRuleDoesNotLeakAcrossSiblings
// regresses a scopeEndLine bug: when a scope matched via the
// level-mismatch fallback (schema at H2, doc at H3), the section
// window must follow the doc heading's actual level so a sibling
// H3 section doesn't get folded into the override's range.
func TestCheck_InlineSchema_ScopeRuleDoesNotLeakAcrossSiblings(t *testing.T) {
	r := &Rule{InlineSchema: inlineSchema(t, map[string]any{
		"sections": []any{
			map[string]any{
				"heading": "Strict",
				"rules": map[string]any{
					"line-length": map[string]any{
						"max":     20,
						"stern":   true,
						"exclude": []any{},
					},
				},
			},
		},
	})}
	// Strict at H3 (level-mismatch fallback), then a sibling H3
	// section that holds a long line. The long line must NOT fire
	// the strict cap because it lives in a different section.
	src := "# T\n\n" +
		"### Strict\n\n" +
		"Short.\n\n" +
		"### Sibling\n\n" +
		"This sibling line is well over twenty chars and stays loose.\n"
	f := newTestFile(t, "doc.md", src)
	diags := r.Check(f)
	var leak bool
	for _, d := range diags {
		if d.RuleID == "MDS001" && d.Line >= 7 {
			leak = true
		}
	}
	assert.False(t, leak,
		"strict override must not extend into the sibling H3 section")
}

// TestCheck_InlineSchema_RuleOverrideAppliesToEveryRepeatedOccurrence
// verifies the per-scope rule override path through `Rule.Check`:
// when a repeated scope (`repeat: { min: 1, max: N }`) carries a
// `rules:` block, the override must apply inside every matched
// section, not just the first occurrence. The plan156 acceptance
// suite covers the structural walker via acronym ranges as a
// proxy; this integration test exercises the MDS020 rule directly
// so a regression in `runScopeRules` would surface.
func TestCheck_InlineSchema_RuleOverrideAppliesToEveryRepeatedOccurrence(t *testing.T) {
	r := &Rule{InlineSchema: inlineSchema(t, map[string]any{
		"sections": []any{
			map[string]any{
				"heading": map[string]any{
					"regex":  "Strict",
					"repeat": map[string]any{"min": 1, "max": 3},
				},
				"rules": map[string]any{
					"line-length": map[string]any{
						"max":     20,
						"stern":   true,
						"exclude": []any{},
					},
				},
			},
		},
	})}
	// Two Strict sections, each with a long line. The override
	// must fire in both.
	src := "# Doc\n\n" +
		"## Strict\n\n" +
		"First strict body line is well over twenty chars and should fire.\n\n" +
		"## Strict\n\n" +
		"Second strict body line is well over twenty chars and should fire.\n"
	f := newTestFile(t, "doc.md", src)
	diags := r.Check(f)
	var lineLength int
	for _, d := range diags {
		if d.RuleID == "MDS001" {
			lineLength++
		}
	}
	assert.GreaterOrEqual(t, lineLength, 2,
		"the rule override must fire inside every Strict occurrence")
}

// TestCheck_InlineSchema_BroadRepeatedScopeYieldsToLaterNamed
// covers the per-scope rule walker's yield path: a broad
// repeated matcher must hand a heading over to a later named
// scope so the named scope's rule override fires on its own
// section. Mirrors the structural validator's claimsLaterLiteral
// behavior at the walker level.
func TestCheck_InlineSchema_BroadRepeatedScopeYieldsToLaterNamed(t *testing.T) {
	r := &Rule{InlineSchema: inlineSchema(t, map[string]any{
		"sections": []any{
			map[string]any{
				"heading": map[string]any{
					"regex":  ".+",
					"repeat": map[string]any{"min": 1},
				},
			},
			map[string]any{
				"heading": "Strict",
				"rules": map[string]any{
					"line-length": map[string]any{
						"max":     20,
						"stern":   true,
						"exclude": []any{},
					},
				},
			},
		},
	})}
	src := "# Doc\n\n" +
		"## Body\n\nx\n\n" +
		"## Strict\n\n" +
		"This strict line is well over twenty chars and should fire.\n"
	f := newTestFile(t, "doc.md", src)
	diags := r.Check(f)
	var lineLength []lint.Diagnostic
	for _, d := range diags {
		if d.RuleID == "MDS001" {
			lineLength = append(lineLength, d)
		}
	}
	require.NotEmpty(t, lineLength,
		"broad repeated matcher must yield `## Strict` so the "+
			"named scope's rule override fires inside its range")
}

// TestApplyScopeRules_NilSchemaShortCircuits covers the defensive
// guard at the top of applyScopeRules so coverage reflects the
// fact that nil schemas are handled.
// TestApplySettings_RejectsBothSources covers the rule-level
// mutual-exclusion guard: a single settings map that names both
// `schema` and `inline-schema` is rejected.
func TestApplySettings_RejectsBothSources(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{
		"schema": "schemas/rfc.md",
		"inline-schema": map[string]any{
			"sections": []any{map[string]any{"heading": "X"}},
		},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot set both")
}

func TestApplySettings_AllowsEmptySchemaWithInline(t *testing.T) {
	// An empty `schema:""` next to a real `inline-schema` is the
	// merge-clears-prior-state state; the rule must still accept it.
	r := &Rule{}
	err := r.ApplySettings(map[string]any{
		"schema": "",
		"inline-schema": map[string]any{
			"sections": []any{map[string]any{"heading": "X"}},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, r.InlineSchema)
}

func TestApplyScopeRules_NilSchemaShortCircuits(t *testing.T) {
	r := &Rule{}
	f := newTestFile(t, "doc.md", "# T\n\n## Foo\n")
	diags := r.applyScopeRules(f, nil, nil)
	assert.Empty(t, diags)
}

// TestCheck_InlineSchema_ScopeRulesWrongLevelStillPairs covers
// scanHeads' wrong-level skip + fallback branches. Schema expects
// Parent at H2; doc has Parent at H3 — the walker's fallback still
// pairs them so the rule override fires.
func TestCheck_InlineSchema_ScopeRulesWrongLevelStillPairs(t *testing.T) {
	r := &Rule{InlineSchema: inlineSchema(t, map[string]any{
		"sections": []any{
			map[string]any{
				"heading": "Strict",
				"rules": map[string]any{
					"line-length": map[string]any{
						"max":     20,
						"stern":   true,
						"exclude": []any{},
					},
				},
			},
		},
	})}
	// Strict appears at H3 instead of the expected H2.
	src := "# T\n\n" +
		"### Strict\n\n" +
		"This line is well over twenty chars and should fire under the cap.\n"
	f := newTestFile(t, "doc.md", src)
	diags := r.Check(f)
	var lineLength []lint.Diagnostic
	for _, d := range diags {
		if d.RuleID == "MDS001" {
			lineLength = append(lineLength, d)
		}
	}
	require.NotEmpty(t, lineLength,
		"level-mismatch fallback should still claim Strict for rule overrides")
}

// TestCheck_InlineSchema_PreambleRuleOverride exercises the
// preamble's `rules:` block. A scope with `heading: null` covers
// the content from line 1 to the first heading; an override on
// that scope should re-run the named rule only inside that range.
func TestCheck_InlineSchema_PreambleRuleOverride(t *testing.T) {
	r := &Rule{InlineSchema: inlineSchema(t, map[string]any{
		"sections": []any{
			map[string]any{
				"heading": nil,
				"rules": map[string]any{
					"line-length": map[string]any{
						"max":     20,
						"stern":   true,
						"exclude": []any{},
					},
				},
			},
			map[string]any{"heading": "Goal"},
		},
	})}
	// Long line in the preamble; identical-length line under Goal.
	src := "This preamble line is well over twenty chars and should fire.\n\n" +
		"## Goal\n\n" +
		"This goal line is well over twenty chars and stays loose.\n"
	f := newTestFile(t, "doc.md", src)
	diags := r.Check(f)
	var lineLength []lint.Diagnostic
	for _, d := range diags {
		if d.RuleID == "MDS001" {
			lineLength = append(lineLength, d)
		}
	}
	require.Len(t, lineLength, 1,
		"preamble override should fire only on the preamble line")
	assert.Equal(t, 1, lineLength[0].Line,
		"diagnostic should land on the preamble line, not under Goal")
}

// TestCheck_InlineSchema_NestedScopeRuleOverride covers walkScopes'
// recursion branch: a parent scope with a `rules:` block AND nested
// children, both with overrides.
func TestCheck_InlineSchema_NestedScopeRuleOverride(t *testing.T) {
	r := &Rule{InlineSchema: inlineSchema(t, map[string]any{
		"sections": []any{
			map[string]any{
				"heading": "Parent",
				"sections": []any{
					map[string]any{
						"heading": "Child",
						"rules": map[string]any{
							"line-length": map[string]any{
								"max":     20,
								"stern":   true,
								"exclude": []any{},
							},
						},
					},
				},
			},
		},
	})}
	// Long line lives inside the Child section.
	src := "# Doc\n\n" +
		"## Parent\n\n" +
		"### Child\n\n" +
		"This child line is well over twenty chars and should fire.\n"
	f := newTestFile(t, "doc.md", src)
	diags := r.Check(f)
	var lineLength []lint.Diagnostic
	for _, d := range diags {
		if d.RuleID == "MDS001" {
			lineLength = append(lineLength, d)
		}
	}
	require.NotEmpty(t, lineLength,
		"nested-scope rule override should be applied inside the Child window")
}

// TestCheck_InlineSchema_ScopeRuleNonConfigurable surfaces a
// diagnostic when an override is targeted at a rule that does not
// implement rule.Configurable. blank-line-around-fenced-code is
// non-configurable in the project, and a non-empty override map
// would silently no-op without this guard.
func TestCheck_InlineSchema_ScopeRuleNonConfigurable(t *testing.T) {
	r := &Rule{InlineSchema: inlineSchema(t, map[string]any{
		"sections": []any{
			map[string]any{
				"heading": "Section",
				"rules": map[string]any{
					"blank-line-around-fenced-code": map[string]any{
						"max": 1,
					},
				},
			},
		},
	})}
	f := newTestFile(t, "doc.md", "# T\n\n## Section\n\nx\n")
	diags := r.Check(f)
	expectDiagMsg(t, diags,
		`scope rule override for "blank-line-around-fenced-code" has no effect`)
}

// TestCheck_InlineSchema_ScopeRuleNonConfigurableEmptyOverride
// regresses the empty-override-on-non-Configurable case: an empty
// map means "re-run this rule inside the scope with its defaults",
// so the rule's Check must fire (and diagnostics inside the scope
// are kept). Without this, a `rules: { blank-line-around-fenced-
// code: {} }` entry would silently no-op.
func TestCheck_InlineSchema_ScopeRuleNonConfigurableEmptyOverride(t *testing.T) {
	r := &Rule{InlineSchema: inlineSchema(t, map[string]any{
		"sections": []any{
			map[string]any{
				"heading": "Section",
				"rules": map[string]any{
					"blank-line-around-fenced-code": map[string]any{},
				},
			},
		},
	})}
	// Place a fenced block right against a heading inside Section.
	// blank-line-around-fenced-code flags missing blank lines
	// around fenced code; we just verify it runs (no MDS020 "no
	// effect" diagnostic, and the rule's own diagnostics flow when
	// applicable).
	f := newTestFile(t, "doc.md",
		"# T\n\n## Section\n```\nx\n```\n")
	diags := r.Check(f)
	for _, d := range diags {
		assert.NotContains(t, d.Message, "has no effect",
			"empty override on non-configurable rule should re-run, not no-op")
	}
}

// TestCheck_InlineSchema_ScopeRulesUnderFmvarHeading regresses a
// Copilot review finding: a per-scope rule override under a
// `\#(fmvar(...))` heading must apply to the matching doc section.
// The walker has to resolve the fmvar against the document's
// frontmatter the same way the structural validator does.
func TestCheck_InlineSchema_ScopeRulesUnderFmvarHeading(t *testing.T) {
	r := &Rule{InlineSchema: inlineSchema(t, map[string]any{
		"frontmatter": map[string]any{
			"id": `string & != ""`,
		},
		"sections": []any{
			map[string]any{
				"heading": map[string]any{
					"regex": `\#(fmvar(id))`,
				},
				"rules": map[string]any{
					"line-length": map[string]any{
						"max":     20,
						"stern":   true,
						"exclude": []any{},
					},
				},
			},
		},
	})}
	src := "---\nid: MDS001\n---\n# Runbook\n\n" +
		"## MDS001\n\n" +
		"This step body has a deliberately long line that exceeds twenty.\n"
	f := newTestFile(t, "doc.md", src)
	diags := r.Check(f)
	var lineLength []lint.Diagnostic
	for _, d := range diags {
		if d.RuleID == "MDS001" {
			lineLength = append(lineLength, d)
		}
	}
	require.NotEmpty(t, lineLength,
		"fmvar-resolved scope heading must claim its match so the rule override fires")
}

// TestCheck_InlineSchema_ScopeRulesUnderFieldHeading exercises
// the per-scope walker's matching path for a `\#(digits)`
// heading. The walker must still pair the scope with the
// matching doc heading so its `rules:` block applies inside
// the right line range.
func TestCheck_InlineSchema_ScopeRulesUnderFieldHeading(t *testing.T) {
	r := &Rule{InlineSchema: inlineSchema(t, map[string]any{
		"sections": []any{
			map[string]any{
				"heading": map[string]any{
					"regex": `Step \#(digits)`,
				},
				"rules": map[string]any{
					"line-length": map[string]any{
						"max":     20,
						"stern":   true,
						"exclude": []any{},
					},
				},
			},
		},
	})}
	src := "# Runbook\n\n" +
		"## Step 1\n\n" +
		"This step body has a deliberately long line that exceeds twenty.\n"
	f := newTestFile(t, "doc.md", src)
	diags := r.Check(f)
	var lineLength []lint.Diagnostic
	for _, d := range diags {
		if d.RuleID == "MDS001" {
			lineLength = append(lineLength, d)
		}
	}
	require.NotEmpty(t, lineLength,
		"field-interpolated scope heading should still claim its match for rule overrides")
}

func TestCheck_InlineSchema_ScopeRuleUnknownName(t *testing.T) {
	r := &Rule{InlineSchema: inlineSchema(t, map[string]any{
		"sections": []any{
			map[string]any{
				"heading": "Loose",
				"rules": map[string]any{
					"definitely-not-a-rule": map[string]any{},
				},
			},
		},
	})}
	f := newTestFile(t, "doc.md", "# Doc\n\n## Loose\n\nx\n")
	diags := r.Check(f)
	expectDiagMsg(t, diags,
		`scope rule override for unknown rule "definitely-not-a-rule"`)
}

func TestCheck_InlineSchema_ScopeRuleInvalidSettings(t *testing.T) {
	// line-length expects max as int; supplying a non-int triggers
	// the ApplySettings error path inside runScopeRules.
	r := &Rule{InlineSchema: inlineSchema(t, map[string]any{
		"sections": []any{
			map[string]any{
				"heading": "Loose",
				"rules": map[string]any{
					"line-length": map[string]any{
						"max": "twenty",
					},
				},
			},
		},
	})}
	f := newTestFile(t, "doc.md", "# Doc\n\n## Loose\n\nx\n")
	diags := r.Check(f)
	expectDiagMsg(t, diags,
		`scope rule override for "line-length" is invalid`)
}

func TestCheck_InlineSchema_ScopeRuleOutOfOrderStillFires(t *testing.T) {
	// Doc has sections in the wrong order. The structural validator
	// emits an out-of-order diagnostic but the scope-rule walker
	// still claims the Strict section and applies its override —
	// regression for the Copilot review note about walkScopes
	// missing out-of-order matches.
	r := &Rule{InlineSchema: inlineSchema(t, map[string]any{
		"closed": true,
		"sections": []any{
			map[string]any{"heading": "Loose"},
			map[string]any{
				"heading": "Strict",
				"rules": map[string]any{
					"line-length": map[string]any{
						"max":     20,
						"stern":   true,
						"exclude": []any{},
					},
				},
			},
		},
	})}
	// Strict appears first; Loose second.
	src := "# Doc\n\n" +
		"## Strict\n\n" +
		"This line is well over twenty chars and should fire under the strict cap.\n\n" +
		"## Loose\n\n" +
		"This line is well over twenty chars but the loose scope tolerates it.\n"
	f := newTestFile(t, "doc.md", src)
	diags := r.Check(f)
	var lineLength []lint.Diagnostic
	for _, d := range diags {
		if d.RuleID == "MDS001" {
			lineLength = append(lineLength, d)
		}
	}
	require.Len(t, lineLength, 1,
		"out-of-order Strict section should still pick up its scope override")
	assert.Equal(t, 5, lineLength[0].Line,
		"diagnostic should land on the long line inside Strict")
}

// TestCheck_InlineSchema_PerScopeRuleOverride covers acceptance
// criterion #7: a schema `rules:` block on a section applies the
// override to that section only. The fixture puts the same prose
// under two sections; only the section with the stricter override
// emits a diagnostic.
func TestCheck_InlineSchema_PerScopeRuleOverride(t *testing.T) {
	r := &Rule{InlineSchema: inlineSchema(t, map[string]any{
		"sections": []any{
			map[string]any{
				"heading": "Loose",
			},
			map[string]any{
				"heading": "Strict",
				"rules": map[string]any{
					"line-length": map[string]any{
						"max":     20,
						"stern":   true,
						"exclude": []any{},
					},
				},
			},
		},
	})}
	// Same long line in both sections; only the Strict scope should fire.
	src := "# Doc\n\n" +
		"## Loose\n\n" +
		"This line is well over twenty chars but the loose scope tolerates it.\n\n" +
		"## Strict\n\n" +
		"This line is well over twenty chars but the strict scope rejects it.\n"
	f := newTestFile(t, "doc.md", src)
	diags := r.Check(f)
	// We expect exactly one line-length diagnostic, scoped to the Strict section.
	var lineLength []lint.Diagnostic
	for _, d := range diags {
		if d.RuleID == "MDS001" {
			lineLength = append(lineLength, d)
		}
	}
	require.Len(t, lineLength, 1,
		"expected one line-length diagnostic from the Strict scope override")
	// The diagnostic must point at a line inside the Strict section
	// (line 7 of the source has the offending content).
	assert.GreaterOrEqual(t, lineLength[0].Line, 7,
		"diagnostic should land inside the Strict scope (line %d)", lineLength[0].Line)
}

// TestCheck_InlineSchema_PerScopeForbiddenText covers plan 142
// acceptance: a per-section `rules:` block on MDS056 applies only to
// the named section, leaving the rest of the document untouched. Same
// prose ("should") appears in two sections; only the Strict section
// fires.
func TestCheck_InlineSchema_PerScopeForbiddenText(t *testing.T) {
	r := &Rule{InlineSchema: inlineSchema(t, map[string]any{
		"sections": []any{
			map[string]any{"heading": "Loose"},
			map[string]any{
				"heading": "Strict",
				"rules": map[string]any{
					"forbidden-text": map[string]any{
						"contains": []any{"should"},
					},
				},
			},
		},
	})}
	src := "# Doc\n\n" +
		"## Loose\n\n" +
		"This should not fire here.\n\n" +
		"## Strict\n\n" +
		"This should fire only here.\n"
	f := newTestFile(t, "doc.md", src)
	diags := r.Check(f)
	var forbidden []lint.Diagnostic
	for _, d := range diags {
		if d.RuleID == "MDS056" {
			forbidden = append(forbidden, d)
		}
	}
	require.Len(t, forbidden, 1,
		"expected one forbidden-text diagnostic from the Strict scope override")
	assert.GreaterOrEqual(t, forbidden[0].Line, 7,
		"diagnostic should land inside the Strict scope (line %d)", forbidden[0].Line)
}

// TestCheck_InlineSchema_PerScopeRequiredMentions covers plan 142
// acceptance: a per-section `rules:` block on MDS058 fires only when
// the named section lacks the mention. The Strict section requires
// "rollback"; the Loose section's body has no such requirement.
func TestCheck_InlineSchema_PerScopeRequiredMentions(t *testing.T) {
	r := &Rule{InlineSchema: inlineSchema(t, map[string]any{
		"sections": []any{
			map[string]any{"heading": "Loose"},
			map[string]any{
				"heading": "Strict",
				"rules": map[string]any{
					"required-mentions": map[string]any{
						"mentions": []any{"rollback"},
					},
				},
			},
		},
	})}
	// Strict body lacks "rollback".
	src := "# Doc\n\n" +
		"## Loose\n\n" +
		"Loose prose with no mentions required.\n\n" +
		"## Strict\n\n" +
		"Strict prose without the keyword.\n"
	f := newTestFile(t, "doc.md", src)
	diags := r.Check(f)
	var mentions []lint.Diagnostic
	for _, d := range diags {
		if d.RuleID == "MDS058" {
			mentions = append(mentions, d)
		}
	}
	require.Len(t, mentions, 1,
		"expected one required-mentions diagnostic from the Strict scope override")
	// The diagnostic anchors at the Strict heading line (line 7).
	assert.Equal(t, 7, mentions[0].Line)
}
