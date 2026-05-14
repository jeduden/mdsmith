package requiredtextpatterns

import (
	"regexp"
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mustFile(t *testing.T, src string) *lint.File {
	t.Helper()
	f, err := lint.NewFile("test.md", []byte(src))
	require.NoError(t, err)
	return f
}

func TestID(t *testing.T) {
	assert.Equal(t, "MDS057", (&Rule{}).ID())
}

func TestName(t *testing.T) {
	assert.Equal(t, "required-text-patterns", (&Rule{}).Name())
}

func TestCategory(t *testing.T) {
	assert.Equal(t, "prose", (&Rule{}).Category())
}

func TestEnabledByDefault(t *testing.T) {
	assert.False(t, (&Rule{}).EnabledByDefault())
}

func TestCheck_NoPatterns_NoDiagnostic(t *testing.T) {
	r := &Rule{}
	assert.Empty(t, r.Check(mustFile(t, "# Title\n\nbody.\n")))
}

func TestCheck_PatternMatch_NoDiagnostic(t *testing.T) {
	r := &Rule{
		Patterns: []Pattern{
			{Source: "expected", Regex: regexp.MustCompile("expected")},
		},
	}
	assert.Empty(t, r.Check(mustFile(t, "# Title\n\nthe expected text.\n")))
}

func TestCheck_PatternMissing_Diagnostic(t *testing.T) {
	r := &Rule{
		Patterns: []Pattern{
			{Source: "expected", Regex: regexp.MustCompile("expected")},
		},
	}
	diags := r.Check(mustFile(t, "# Title\n\nsome other text.\n"))
	require.Len(t, diags, 1)
	assert.Equal(t, 1, diags[0].Line)
	assert.Contains(t, diags[0].Message, "expected")
}

func TestCheck_CustomMessage_Used(t *testing.T) {
	r := &Rule{
		Patterns: []Pattern{
			{
				Source:  "expected",
				Regex:   regexp.MustCompile("expected"),
				Message: "must declare expectation",
			},
		},
	}
	diags := r.Check(mustFile(t, "# Title\n\nsome other text.\n"))
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "must declare expectation")
}

func TestCheck_MultipleHeadings_EachChecked(t *testing.T) {
	src := "# A\n\nfoo here.\n\n## B\n\nno match.\n\n## C\n\nfoo here too.\n"
	r := &Rule{
		Patterns: []Pattern{
			{Source: "foo", Regex: regexp.MustCompile("foo")},
		},
	}
	diags := r.Check(mustFile(t, src))
	// A matches (foo in own body). C matches. B does not.
	// However A's body also includes B and C (nested), so A also matches.
	require.Len(t, diags, 1)
	assert.Equal(t, 5, diags[0].Line) // ## B is on line 5
}

func TestCheck_NestedSectionContributesToParent(t *testing.T) {
	src := "# A\n\n## B\n\nfoo lives here.\n"
	r := &Rule{
		Patterns: []Pattern{
			{Source: "foo", Regex: regexp.MustCompile("foo")},
		},
	}
	// A's body includes B's content, so A matches. B also matches directly.
	assert.Empty(t, r.Check(mustFile(t, src)))
}

func TestCheck_RegexPattern(t *testing.T) {
	r := &Rule{
		Patterns: []Pattern{
			{Source: "^[A-Z]", Regex: regexp.MustCompile("^[A-Z]")},
		},
	}
	// Body starts with uppercase.
	assert.Empty(t, r.Check(mustFile(t, "# Title\n\nProse here.\n")))
}

func TestCheck_NilAST_NoDiagnostic(t *testing.T) {
	r := &Rule{
		Patterns: []Pattern{
			{Source: "x", Regex: regexp.MustCompile("x")},
		},
	}
	assert.Empty(t, r.Check(&lint.File{}))
}

func TestCheck_NoHeadings_NoDiagnostic(t *testing.T) {
	// Without a heading, MDS057 has no scope to check.
	r := &Rule{
		Patterns: []Pattern{
			{Source: "foo", Regex: regexp.MustCompile("foo")},
		},
	}
	assert.Empty(t, r.Check(mustFile(t, "just some text.\n")))
}

func TestApplySettings_Patterns(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{
		"patterns": []any{
			map[string]any{"pattern": "foo", "message": "needs foo"},
		},
	})
	require.NoError(t, err)
	require.Len(t, r.Patterns, 1)
	assert.Equal(t, "foo", r.Patterns[0].Source)
	assert.Equal(t, "needs foo", r.Patterns[0].Message)
	assert.NotNil(t, r.Patterns[0].Regex)
}

func TestApplySettings_PatternsWithSkipIndices(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{
		"patterns": []any{
			map[string]any{
				"pattern":      "foo",
				"skip-indices": []any{-1, 0},
			},
		},
	})
	require.NoError(t, err)
	require.Len(t, r.Patterns, 1)
	assert.Equal(t, []int{-1, 0}, r.Patterns[0].SkipIndices)
}

func TestApplySettings_PatternsMustBeList(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"patterns": "not a list"})
	assert.Error(t, err)
}

func TestApplySettings_MissingPattern(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{
		"patterns": []any{map[string]any{"message": "hi"}},
	})
	assert.Error(t, err)
}

func TestApplySettings_InvalidRegex(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{
		"patterns": []any{map[string]any{"pattern": "[unterminated"}},
	})
	assert.Error(t, err)
}

func TestApplySettings_InvalidSkipIndices(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{
		"patterns": []any{
			map[string]any{"pattern": "foo", "skip-indices": "nope"},
		},
	})
	assert.Error(t, err)
}

func TestApplySettings_UnknownKey(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"unknown": 1})
	assert.Error(t, err)
}

func TestDefaultSettings(t *testing.T) {
	r := &Rule{}
	ds := r.DefaultSettings()
	assert.Equal(t, []any{}, ds["patterns"])
}

func TestApplyDefaultSettings_ClearsPatterns(t *testing.T) {
	r := &Rule{Patterns: []Pattern{{Source: "x", Regex: regexp.MustCompile("x")}}}
	require.NoError(t, r.ApplySettings(r.DefaultSettings()))
	assert.Empty(t, r.Patterns)
}

func TestCheck_TableParagraphSkipped(t *testing.T) {
	// Goldmark parses tables as paragraphs when the table extension is
	// absent; the rule should not consider them part of the body.
	src := "# Title\n\n| foo |\n| --- |\n| bar |\n"
	r := &Rule{
		Patterns: []Pattern{
			{Source: "foo", Regex: regexp.MustCompile("foo")},
		},
	}
	diags := r.Check(mustFile(t, src))
	require.Len(t, diags, 1)
	assert.Equal(t, 1, diags[0].Line)
}

func TestApplySettings_PatternsFromInterfaceKeyedMap(t *testing.T) {
	// YAML decoded into `any` can produce map[any]any for nested maps.
	r := &Rule{}
	err := r.ApplySettings(map[string]any{
		"patterns": []any{
			map[any]any{"pattern": "foo", "message": "must mention foo"},
		},
	})
	require.NoError(t, err)
	require.Len(t, r.Patterns, 1)
	assert.Equal(t, "foo", r.Patterns[0].Source)
	assert.Equal(t, "must mention foo", r.Patterns[0].Message)
}

func TestApplySettings_PatternsEntryNotMap(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{
		"patterns": []any{"not a map"},
	})
	assert.Error(t, err)
}

func TestApplySettings_SkipIndicesNonIntegerEntry(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{
		"patterns": []any{
			map[string]any{"pattern": "foo", "skip-indices": []any{"x"}},
		},
	})
	assert.Error(t, err)
}
