package schema

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestExtractFieldMeta_PlainStringIsNotMeta covers the most
// common path: a frontmatter value that's a CUE expression string
// flows through with isMeta=false so the caller falls back to
// frontmatterExpr.
func TestExtractFieldMeta_PlainStringIsNotMeta(t *testing.T) {
	_, _, isMeta, err := ExtractFieldMeta("string")
	require.NoError(t, err)
	assert.False(t, isMeta)
}

// TestExtractFieldMeta_NestedMapWithoutTypeIsNotMeta keeps the
// existing CUE-struct-constraint path working: a frontmatter
// mapping without a `type:` key continues to be JSON-encoded into
// CUE rather than claimed as deprecation metadata.
func TestExtractFieldMeta_NestedMapWithoutTypeIsNotMeta(t *testing.T) {
	in := map[string]any{
		"owner":   "string",
		"created": "date",
	}
	_, _, isMeta, err := ExtractFieldMeta(in)
	require.NoError(t, err)
	assert.False(t, isMeta)
}

// TestExtractFieldMeta_DeprecatedMapping covers the full plan 136
// shape: type + deprecated + message + replaced-by.
func TestExtractFieldMeta_DeprecatedMapping(t *testing.T) {
	in := map[string]any{
		"type":        "string",
		"deprecated":  true,
		"message":     `use "owner" instead`,
		"replaced-by": "owner",
	}
	expr, meta, isMeta, err := ExtractFieldMeta(in)
	require.NoError(t, err)
	require.True(t, isMeta)
	assert.Equal(t, "string", expr)
	assert.True(t, meta.Deprecated)
	assert.Equal(t, `use "owner" instead`, meta.Message)
	assert.Equal(t, "owner", meta.ReplacedBy)
}

// TestExtractFieldMeta_TypeOnly accepts the minimal metadata
// form — just `type:` — and returns an empty FieldMeta. Callers
// use IsZero() to skip empty entries.
func TestExtractFieldMeta_TypeOnly(t *testing.T) {
	in := map[string]any{"type": "string"}
	expr, meta, isMeta, err := ExtractFieldMeta(in)
	require.NoError(t, err)
	require.True(t, isMeta)
	assert.Equal(t, "string", expr)
	assert.True(t, meta.IsZero())
}

// TestExtractFieldMeta_UnknownKeyRejected catches typos at parse
// time so a misspelled `replaced_by:` doesn't silently drop the
// hint.
func TestExtractFieldMeta_UnknownKeyRejected(t *testing.T) {
	in := map[string]any{
		"type":        "string",
		"deprecated":  true,
		"replaced_by": "owner",
	}
	_, _, _, err := ExtractFieldMeta(in)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "replaced_by")
}

// TestExtractFieldMeta_MessageWithoutDeprecatedRejected refuses a
// hint that won't fire. Without `deprecated: true`, neither
// `message:` nor `replaced-by:` ever surfaces to the user, so a
// schema that sets them without the flag is almost certainly a
// mistake.
func TestExtractFieldMeta_MessageWithoutDeprecatedRejected(t *testing.T) {
	in := map[string]any{
		"type":    "string",
		"message": "use owner instead",
	}
	_, _, _, err := ExtractFieldMeta(in)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "deprecated: true")
}

// TestExtractFieldMeta_DeprecatedFalseAcceptsHintFields covers the
// inheritance-undo path: a child kind can re-declare a deprecated
// field with `deprecated: false` to opt out of the parent's
// deprecation. The hint fields stay on the metadata so the
// composed result still knows the parent's replacement intent.
func TestExtractFieldMeta_DeprecatedFalseDropsHints(t *testing.T) {
	in := map[string]any{
		"type":       "string",
		"deprecated": false,
	}
	_, meta, isMeta, err := ExtractFieldMeta(in)
	require.NoError(t, err)
	require.True(t, isMeta)
	assert.False(t, meta.Deprecated)
	assert.True(t, meta.IsZero(),
		"deprecated:false with no hint fields collapses to zero")
}

// TestExtractFieldMeta_DeprecatedWrongType errors on a non-bool
// `deprecated:` so a YAML typo (`deprecated: "true"`) doesn't
// silently fall through as truthy.
func TestExtractFieldMeta_DeprecatedWrongType(t *testing.T) {
	in := map[string]any{
		"type":       "string",
		"deprecated": "true",
	}
	_, _, _, err := ExtractFieldMeta(in)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "boolean")
}

// TestExtractFieldMeta_MessageWrongType catches a non-string
// `message:` at parse time so a YAML typo (`message: 42`) does
// not silently coerce into an empty string downstream.
func TestExtractFieldMeta_MessageWrongType(t *testing.T) {
	in := map[string]any{
		"type":       "string",
		"deprecated": true,
		"message":    42,
	}
	_, _, _, err := ExtractFieldMeta(in)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "message must be a string")
}

// TestExtractFieldMeta_ReplacedByWrongType mirrors the
// message-wrong-type check for the `replaced-by:` field.
func TestExtractFieldMeta_ReplacedByWrongType(t *testing.T) {
	in := map[string]any{
		"type":        "string",
		"deprecated":  true,
		"replaced-by": []any{"owner"},
	}
	_, _, _, err := ExtractFieldMeta(in)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "replaced-by must be a string")
}

// TestExtractFieldMeta_TypeExprError surfaces the
// frontmatterExpr failure path: a non-string `type:` value that
// has no JSON encoding (e.g. an unknown bare-name string)
// bubbles up through ExtractFieldMeta with a "type:" prefix.
func TestExtractFieldMeta_TypeExprError(t *testing.T) {
	in := map[string]any{
		"type":       "",
		"deprecated": true,
	}
	_, _, _, err := ExtractFieldMeta(in)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "type:")
}

// TestExtractFieldMeta_BareTypeNameResolves applies the same
// shortcut resolution the rest of the frontmatter parser uses
// (plan 148): `type: date` returns the canonical CUE pattern
// rather than passing the bare identifier through unresolved.
func TestExtractFieldMeta_BareTypeNameResolves(t *testing.T) {
	in := map[string]any{
		"type":       "date",
		"deprecated": true,
	}
	expr, _, _, err := ExtractFieldMeta(in)
	require.NoError(t, err)
	assert.NotEqual(t, "date", expr,
		"a bare shortcut name should resolve to its CUE pattern")
	// The resolved date pattern is a CUE regex; the exact escape
	// form depends on the shortcut registry, so assert on the
	// stable substring "=~" instead.
	assert.Contains(t, expr, "=~",
		"shortcut should resolve to a CUE regex expression")
}

// TestParseFile_FrontmatterMeta exercises the proto.md path:
// a deprecation mapping in front matter parses through
// `parseFileFrontmatter` into FrontmatterMeta the same way the
// inline parser does.
func TestParseFile_FrontmatterMeta(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "proto.md",
		"---\n"+
			"legacy_owner:\n"+
			"  type: string\n"+
			"  deprecated: true\n"+
			"  message: 'use \"owner\" instead'\n"+
			"owner: string\n"+
			"---\n"+
			"# ?\n")
	sch, err := ParseFile(&FileReader{}, p)
	require.NoError(t, err)
	assert.Equal(t, "string", sch.Frontmatter["legacy_owner"])
	require.Contains(t, sch.FrontmatterMeta, "legacy_owner")
	meta := sch.FrontmatterMeta["legacy_owner"]
	assert.True(t, meta.Deprecated)
	assert.Equal(t, `use "owner" instead`, meta.Message)
}

// TestParseInline_FrontmatterMeta exercises end-to-end parsing of
// the inline metadata form: the resulting Schema carries the CUE
// constraint on Frontmatter and the metadata on FrontmatterMeta.
func TestParseInline_FrontmatterMeta(t *testing.T) {
	raw := map[string]any{
		"frontmatter": map[string]any{
			"legacy_owner": map[string]any{
				"type":        "string",
				"deprecated":  true,
				"message":     `use "owner" instead`,
				"replaced-by": "owner",
			},
			"owner": "string",
		},
	}
	sch, err := ParseInline(raw, "kind test")
	require.NoError(t, err)
	assert.Equal(t, "string", sch.Frontmatter["legacy_owner"])
	assert.Equal(t, "string", sch.Frontmatter["owner"])
	require.Contains(t, sch.FrontmatterMeta, "legacy_owner")
	meta := sch.FrontmatterMeta["legacy_owner"]
	assert.True(t, meta.Deprecated)
	assert.Equal(t, `use "owner" instead`, meta.Message)
	assert.Equal(t, "owner", meta.ReplacedBy)
	// Non-deprecated fields don't show up in FrontmatterMeta.
	_, hasOwner := sch.FrontmatterMeta["owner"]
	assert.False(t, hasOwner)
}
