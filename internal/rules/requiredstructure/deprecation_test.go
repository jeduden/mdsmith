package requiredstructure

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCheck_InlineSchema_DeprecatedFieldEmitsWarning covers plan
// 136's first inline-schema acceptance criterion: a deprecated
// field present in a document's FM surfaces as a Warning
// diagnostic carrying the schema's `message:` payload.
func TestCheck_InlineSchema_DeprecatedFieldEmitsWarning(t *testing.T) {
	r := &Rule{InlineSchema: inlineSchema(t, map[string]any{
		"frontmatter": map[string]any{
			"legacy_owner": map[string]any{
				"type":       "string",
				"deprecated": true,
				"message":    `use "owner" instead`,
			},
			"owner": "string",
		},
	})}
	f := newTestFile(t, "doc.md",
		"---\nlegacy_owner: alice\nowner: alice\n---\n# Doc\n")
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Equal(t, lint.Warning, diags[0].Severity)
	assert.True(t, diags[0].Deprecated)
	assert.Contains(t, diags[0].Message, "legacy_owner: deprecated field")
	assert.Contains(t, diags[0].Message, `use "owner" instead`)
}

// TestCheck_FileSchema_DeprecatedFieldEmitsWarning is the legacy
// proto.md mirror of the inline case: the same metadata shape on
// the file source should produce the same Warning diagnostic.
func TestCheck_FileSchema_DeprecatedFieldEmitsWarning(t *testing.T) {
	schemaPath := writeSchema(t,
		"---\n"+
			"legacy_owner:\n"+
			"  type: string\n"+
			"  deprecated: true\n"+
			"  message: 'use \"owner\" instead'\n"+
			"owner: string\n"+
			"---\n"+
			"# ?\n")
	r := &Rule{Schema: schemaPath}
	f := newTestFile(t, "doc.md",
		"---\nlegacy_owner: alice\nowner: alice\n---\n# Doc\n")
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Equal(t, lint.Warning, diags[0].Severity)
	assert.True(t, diags[0].Deprecated)
	assert.Contains(t, diags[0].Message, "legacy_owner: deprecated field")
	assert.Contains(t, diags[0].Message, `use "owner" instead`)
}

// TestCheck_InlineSchema_ReplacedByCanonicalSentence covers the
// `replaced-by:` rendering: without a `message:` payload the
// diagnostic uses the canonical sentence form.
func TestCheck_InlineSchema_ReplacedByCanonicalSentence(t *testing.T) {
	r := &Rule{InlineSchema: inlineSchema(t, map[string]any{
		"frontmatter": map[string]any{
			"legacy_owner": map[string]any{
				"type":        "string",
				"deprecated":  true,
				"replaced-by": "owner",
			},
		},
	})}
	f := newTestFile(t, "doc.md",
		"---\nlegacy_owner: alice\n---\n# Doc\n")
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message,
		"legacy_owner: deprecated field; replaced by `owner`")
	assert.Equal(t, "owner", diags[0].ReplacedBy,
		"ReplacedBy should ride on the wire for LSP routing")
}

// TestCheck_InlineSchema_DeprecationCoExistsWithTypeError covers
// acceptance criterion #4: a deprecated field that violates its
// declared `type:` produces both an Error (type mismatch) and a
// Warning (deprecation), neither suppressing the other.
func TestCheck_InlineSchema_DeprecationCoExistsWithTypeError(t *testing.T) {
	r := &Rule{InlineSchema: inlineSchema(t, map[string]any{
		"frontmatter": map[string]any{
			"legacy_owner": map[string]any{
				"type":       "string",
				"deprecated": true,
			},
		},
	})}
	f := newTestFile(t, "doc.md",
		"---\nlegacy_owner: 42\n---\n# Doc\n")
	diags := r.Check(f)
	require.Len(t, diags, 2)
	var sawError, sawWarning bool
	for _, d := range diags {
		if d.Deprecated {
			sawWarning = true
			assert.Equal(t, lint.Warning, d.Severity)
		} else {
			sawError = true
			assert.Equal(t, lint.Error, d.Severity)
			assert.Contains(t, d.Message, "got 42")
		}
	}
	assert.True(t, sawError, "type violation should produce an Error")
	assert.True(t, sawWarning, "deprecation should produce a Warning")
}

// TestCheck_InlineSchema_DeprecationAbsentNoDiagnostic verifies
// that omitting the deprecated key from the document silences the
// warning. This is the canonical migration: drop the field from
// the document and the build goes green.
func TestCheck_InlineSchema_DeprecationAbsentNoDiagnostic(t *testing.T) {
	r := &Rule{InlineSchema: inlineSchema(t, map[string]any{
		"frontmatter": map[string]any{
			"legacy_owner?": map[string]any{
				"type":       "string",
				"deprecated": true,
			},
			"owner": "string",
		},
	})}
	f := newTestFile(t, "doc.md",
		"---\nowner: alice\n---\n# Doc\n")
	diags := r.Check(f)
	assert.Empty(t, diags,
		"a document without the deprecated field should pass cleanly")
}
