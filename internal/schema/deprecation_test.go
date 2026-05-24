package schema

import (
	"strings"
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestValidateFrontmatterDiags_DeprecatedFieldEmitsWarning covers
// plan 136's first acceptance criterion: a deprecated field
// present in a document's front matter produces exactly one
// Warning-severity diagnostic carrying the `message:` payload.
func TestValidateFrontmatterDiags_DeprecatedFieldEmitsWarning(t *testing.T) {
	raw := map[string]any{
		"frontmatter": map[string]any{
			"legacy_owner": map[string]any{
				"type":       "string",
				"deprecated": true,
				"message":    `use "owner" instead`,
			},
			"owner": "string",
		},
	}
	sch, err := ParseInline(raw, "kind test")
	require.NoError(t, err)
	doc := newDocFile(t, "doc.md", "# T\n")
	docFM := map[string]any{
		"legacy_owner": "alice",
		"owner":        "alice",
	}
	diags := ValidateFrontmatterDiags(doc, sch, docFM, makeDiagForTest)
	require.Len(t, diags, 1)
	assert.Equal(t, lint.Warning, diags[0].Severity)
	assert.True(t, diags[0].Deprecated,
		"Deprecated flag should route the diagnostic on the wire")
	assert.Contains(t, diags[0].Message, "legacy_owner: deprecated field")
	assert.Contains(t, diags[0].Message, `use "owner" instead`)
}

// TestValidateFrontmatterDiags_ReplacedByCanonicalSentence covers
// acceptance criterion #3 part B: a deprecation that sets
// `replaced-by:` without `message:` renders the canonical
// "replaced by `name`" sentence on the first message line.
func TestValidateFrontmatterDiags_ReplacedByCanonicalSentence(t *testing.T) {
	raw := map[string]any{
		"frontmatter": map[string]any{
			"legacy_owner": map[string]any{
				"type":        "string",
				"deprecated":  true,
				"replaced-by": "owner",
			},
		},
	}
	sch, err := ParseInline(raw, "kind test")
	require.NoError(t, err)
	doc := newDocFile(t, "doc.md", "# T\n")
	docFM := map[string]any{"legacy_owner": "alice"}
	diags := ValidateFrontmatterDiags(doc, sch, docFM, makeDiagForTest)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "legacy_owner: deprecated field; replaced by `owner`")
	assert.Equal(t, "owner", diags[0].ReplacedBy,
		"ReplacedBy rides on the structured diagnostic for LSP routing")
}

// TestValidateFrontmatterDiags_MessageWinsOverReplacedBy covers
// the precedence rule: when both `message:` and `replaced-by:`
// are set, the human-facing text honours `message:` but the
// structured ReplacedBy still rides on the diagnostic so tools
// can route it.
func TestValidateFrontmatterDiags_MessageWinsOverReplacedBy(t *testing.T) {
	raw := map[string]any{
		"frontmatter": map[string]any{
			"legacy_owner": map[string]any{
				"type":        "string",
				"deprecated":  true,
				"message":     "see the migration notes",
				"replaced-by": "owner",
			},
		},
	}
	sch, err := ParseInline(raw, "kind test")
	require.NoError(t, err)
	doc := newDocFile(t, "doc.md", "# T\n")
	diags := ValidateFrontmatterDiags(doc, sch,
		map[string]any{"legacy_owner": "alice"}, makeDiagForTest)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "see the migration notes")
	assert.NotContains(t, diags[0].Message, "replaced by `owner`",
		"`message:` should suppress the canonical sentence")
	assert.Equal(t, "owner", diags[0].ReplacedBy,
		"structured ReplacedBy is still exposed for tooling")
}

// TestValidateFrontmatterDiags_DeprecatedFieldAbsentNoDiagnostic
// is the silent-when-removed case: dropping the deprecated key
// from the document makes the warning go away. The schema's
// `legacy_owner` is still declared, so removing the field from a
// document is the natural cleanup path.
//
// Exercises both the empty-docFM short circuit and the
// per-field "not present in docFM" skip so neither path becomes
// dead code under refactoring.
func TestValidateFrontmatterDiags_DeprecatedFieldAbsentNoDiagnostic(t *testing.T) {
	raw := map[string]any{
		"frontmatter": map[string]any{
			"legacy_owner?": map[string]any{
				"type":       "string",
				"deprecated": true,
			},
			"owner?": "string",
		},
	}
	sch, err := ParseInline(raw, "kind test")
	require.NoError(t, err)
	doc := newDocFile(t, "doc.md", "# T\n")
	// Empty docFM — the walker short-circuits before the loop.
	diags := ValidateFrontmatterDiags(doc, sch, map[string]any{}, makeDiagForTest)
	assert.Empty(t, diags,
		"a deprecated field absent from an empty docFM should not warn")
	// Non-empty docFM that omits the deprecated key — the walker
	// reaches the per-field check and skips the entry.
	diags = ValidateFrontmatterDiags(doc, sch,
		map[string]any{"owner": "alice"}, makeDiagForTest)
	assert.Empty(t, diags,
		"a deprecated field absent from a non-empty docFM should not warn")
}

// TestValidateFrontmatterDiags_DeprecationCoExistsWithTypeError
// covers acceptance criterion #4: a deprecated field that
// violates its `type:` produces TWO diagnostics — one Warning
// for the deprecation and one (default-severity) type-mismatch
// diagnostic. Neither suppresses the other.
func TestValidateFrontmatterDiags_DeprecationCoExistsWithTypeError(t *testing.T) {
	raw := map[string]any{
		"frontmatter": map[string]any{
			"legacy_owner": map[string]any{
				"type":       "string",
				"deprecated": true,
			},
		},
	}
	sch, err := ParseInline(raw, "kind test")
	require.NoError(t, err)
	doc := newDocFile(t, "doc.md", "# T\n")
	docFM := map[string]any{"legacy_owner": float64(42)}
	diags := ValidateFrontmatterDiags(doc, sch, docFM, makeDiagForTest)
	require.Len(t, diags, 2)
	var sawTypeViolation, sawDeprecation bool
	for _, d := range diags {
		if d.Deprecated {
			sawDeprecation = true
			assert.Equal(t, lint.Warning, d.Severity)
			assert.Contains(t, d.Message, "deprecated field")
		} else {
			sawTypeViolation = true
			assert.Contains(t, d.Message, "legacy_owner")
			assert.Contains(t, d.Message, "got 42")
			assert.NotEqual(t, lint.Warning, d.Severity,
				"a type violation should not be downgraded to Warning")
		}
	}
	assert.True(t, sawTypeViolation, "type violation should produce a diagnostic")
	assert.True(t, sawDeprecation, "deprecation should produce a Warning")
}

// TestValidateFrontmatterDiags_RemovingFieldFromSchema regresses
// acceptance criterion #6: when the field is dropped from the
// schema entirely (the maintainer finishes the migration), the
// posture reverts to the schema's closed/open default. The
// document carrying the now-unknown field surfaces the usual
// "not declared in schema" diagnostic rather than a deprecation
// warning.
func TestValidateFrontmatterDiags_RemovingFieldFromSchema(t *testing.T) {
	raw := map[string]any{
		"frontmatter": map[string]any{
			"owner": "string",
			// legacy_owner is gone from the schema entirely.
		},
	}
	sch, err := ParseInline(raw, "kind test")
	require.NoError(t, err)
	doc := newDocFile(t, "doc.md", "# T\n")
	docFM := map[string]any{
		"owner":        "alice",
		"legacy_owner": "alice",
	}
	diags := ValidateFrontmatterDiags(doc, sch, docFM, makeDiagForTest)
	require.NotEmpty(t, diags)
	var sawClosedError bool
	for _, d := range diags {
		assert.False(t, d.Deprecated,
			"no deprecation warning should fire once the field is dropped")
		assert.NotEqual(t, lint.Warning, d.Severity,
			"only deprecations carry Warning severity in MDS020 today")
		if strings.Contains(d.Message, "legacy_owner") &&
			strings.Contains(d.Message, "not declared in schema") {
			sawClosedError = true
		}
	}
	assert.True(t, sawClosedError,
		"removed-field shape falls under the schema's closed/open posture")
}

// TestExtend_PropagatesFrontmatterMeta covers plan 135 + 136
// interaction: a child kind extending its parent inherits the
// parent's deprecation metadata, and a child re-declaring the
// same field with its own metadata wins on key collisions.
func TestExtend_PropagatesFrontmatterMeta(t *testing.T) {
	parent := &Schema{
		Frontmatter: map[string]string{
			"legacy_owner": "string",
			"owner":        "string",
		},
		FrontmatterMeta: map[string]FieldMeta{
			"legacy_owner": {
				Deprecated: true,
				Message:    "use owner",
			},
		},
	}
	child := &Schema{
		Frontmatter: map[string]string{
			"owner":     "string",
			"old_child": "string",
		},
		FrontmatterMeta: map[string]FieldMeta{
			"old_child": {
				Deprecated: true,
				ReplacedBy: "new_child",
			},
		},
	}
	out, err := Extend(parent, child)
	require.NoError(t, err)
	require.Contains(t, out.FrontmatterMeta, "legacy_owner")
	require.Contains(t, out.FrontmatterMeta, "old_child")
	assert.True(t, out.FrontmatterMeta["legacy_owner"].Deprecated)
	assert.Equal(t, "use owner",
		out.FrontmatterMeta["legacy_owner"].Message)
	assert.Equal(t, "new_child",
		out.FrontmatterMeta["old_child"].ReplacedBy)
}

// TestExtend_ChildMetaOverridesParent covers the child-wins rule:
// when both sides declare metadata for the same field, the
// child's entry replaces the parent's so a kind can re-deprecate
// with a fresher message.
func TestExtend_ChildMetaOverridesParent(t *testing.T) {
	parent := &Schema{
		Frontmatter: map[string]string{"legacy_owner": "string"},
		FrontmatterMeta: map[string]FieldMeta{
			"legacy_owner": {Deprecated: true, Message: "old message"},
		},
	}
	child := &Schema{
		Frontmatter: map[string]string{"legacy_owner": "string"},
		FrontmatterMeta: map[string]FieldMeta{
			"legacy_owner": {Deprecated: true, Message: "new message"},
		},
	}
	out, err := Extend(parent, child)
	require.NoError(t, err)
	assert.Equal(t, "new message",
		out.FrontmatterMeta["legacy_owner"].Message)
}

// TestCompose_UnionsFrontmatterMeta covers plan 156 + 136
// interaction: composing two schemas unions their deprecation
// metadata so a file resolving to multiple kinds sees every
// kind's deprecation flags.
func TestCompose_UnionsFrontmatterMeta(t *testing.T) {
	a := &Schema{
		Frontmatter: map[string]string{"old_a": "string"},
		FrontmatterMeta: map[string]FieldMeta{
			"old_a": {Deprecated: true},
		},
	}
	b := &Schema{
		Frontmatter: map[string]string{"old_b": "string"},
		FrontmatterMeta: map[string]FieldMeta{
			"old_b": {Deprecated: true, ReplacedBy: "new_b"},
		},
	}
	out, err := Compose(a, b)
	require.NoError(t, err)
	require.Contains(t, out.FrontmatterMeta, "old_a")
	require.Contains(t, out.FrontmatterMeta, "old_b")
	assert.True(t, out.FrontmatterMeta["old_a"].Deprecated)
	assert.Equal(t, "new_b",
		out.FrontmatterMeta["old_b"].ReplacedBy)
}

// TestValidateFrontmatterDiags_DeprecationStableOrder regresses
// the sorted iteration over FrontmatterMeta: two deprecated
// fields present in the document must produce diagnostics in
// alphabetical order so the LSP and CI logs stay stable
// across runs.
func TestValidateFrontmatterDiags_DeprecationStableOrder(t *testing.T) {
	raw := map[string]any{
		"frontmatter": map[string]any{
			"zzz_old": map[string]any{
				"type":       "string",
				"deprecated": true,
			},
			"aaa_old": map[string]any{
				"type":       "string",
				"deprecated": true,
			},
			"mmm_old": map[string]any{
				"type":       "string",
				"deprecated": true,
			},
		},
	}
	sch, err := ParseInline(raw, "kind test")
	require.NoError(t, err)
	doc := newDocFile(t, "doc.md", "# T\n")
	docFM := map[string]any{
		"zzz_old": "a",
		"aaa_old": "b",
		"mmm_old": "c",
	}
	// Iterate many times; the order of the three deprecation
	// warnings must be the same on every run.
	var first []string
	for i := 0; i < 20; i++ {
		diags := ValidateFrontmatterDiags(doc, sch, docFM, makeDiagForTest)
		var fields []string
		for _, d := range diags {
			if d.Deprecated {
				fields = append(fields, firstField(d.Message))
			}
		}
		if i == 0 {
			require.Equal(t, []string{"aaa_old", "mmm_old", "zzz_old"}, fields,
				"deprecation warnings should be sorted by field name")
			first = fields
		} else {
			require.Equal(t, first, fields,
				"deprecation iteration must be deterministic")
		}
	}
}
