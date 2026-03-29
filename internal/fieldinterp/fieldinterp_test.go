package fieldinterp

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =====================================================================
// Interpolate
// =====================================================================

func TestInterpolate_SimpleField(t *testing.T) {
	got := Interpolate("{title}", map[string]any{"title": "Hello"})
	assert.Equal(t, "Hello", got)
}

func TestInterpolate_MultipleFields(t *testing.T) {
	got := Interpolate("{id}: {name}", map[string]any{"id": "MDS001", "name": "line-length"})
	assert.Equal(t, "MDS001: line-length", got)
}

func TestInterpolate_MissingFieldEmpty(t *testing.T) {
	got := Interpolate("{title}", map[string]any{})
	assert.Equal(t, "", got)
}

func TestInterpolate_MixedPresentMissing(t *testing.T) {
	got := Interpolate("- [{title}]({filename})", map[string]any{"filename": "a.md"})
	assert.Equal(t, "- [](a.md)", got)
}

func TestInterpolate_NoPlaceholders(t *testing.T) {
	got := Interpolate("plain text", map[string]any{"title": "Hello"})
	assert.Equal(t, "plain text", got)
}

func TestInterpolate_EmptyString(t *testing.T) {
	got := Interpolate("", map[string]any{"title": "Hello"})
	assert.Equal(t, "", got)
}

func TestInterpolate_EscapedBrace(t *testing.T) {
	got := Interpolate("{{literal}} {title}", map[string]any{"title": "Hello"})
	assert.Equal(t, "{literal} Hello", got)
}

func TestInterpolate_EscapedClosingBrace(t *testing.T) {
	got := Interpolate("{title} end}}", map[string]any{"title": "Hello"})
	assert.Equal(t, "Hello end}", got)
}

func TestInterpolate_OnlyEscapedBraces(t *testing.T) {
	got := Interpolate("{{no}} {{fields}}", map[string]any{})
	assert.Equal(t, "{no} {fields}", got)
}

func TestInterpolate_NilData(t *testing.T) {
	got := Interpolate("{title}", nil)
	assert.Equal(t, "", got)
}

func TestInterpolate_AdjacentPlaceholders(t *testing.T) {
	got := Interpolate("{a}{b}", map[string]any{"a": "X", "b": "Y"})
	assert.Equal(t, "XY", got)
}

func TestInterpolate_FieldWithHyphen(t *testing.T) {
	got := Interpolate("{my-field}", map[string]any{"my-field": "value"})
	assert.Equal(t, "value", got)
}

// =====================================================================
// Fields (parse)
// =====================================================================

func TestFields_SingleField(t *testing.T) {
	fields := Fields("{title}")
	require.Len(t, fields, 1)
	assert.Equal(t, "title", fields[0])
}

func TestFields_MultipleFields(t *testing.T) {
	fields := Fields("{id}: {name}")
	require.Len(t, fields, 2)
	assert.Equal(t, "id", fields[0])
	assert.Equal(t, "name", fields[1])
}

func TestFields_NoFields(t *testing.T) {
	fields := Fields("plain text")
	assert.Empty(t, fields)
}

func TestFields_EscapedBracesIgnored(t *testing.T) {
	fields := Fields("{{literal}} {title}")
	require.Len(t, fields, 1)
	assert.Equal(t, "title", fields[0])
}

func TestFields_EmptyString(t *testing.T) {
	fields := Fields("")
	assert.Empty(t, fields)
}

// =====================================================================
// ContainsField
// =====================================================================

func TestContainsField_True(t *testing.T) {
	assert.True(t, ContainsField("{title}"))
}

func TestContainsField_False(t *testing.T) {
	assert.False(t, ContainsField("plain text"))
}

func TestContainsField_EscapedNotField(t *testing.T) {
	assert.False(t, ContainsField("{{literal}}"))
}

func TestContainsField_MixedEscapedAndReal(t *testing.T) {
	assert.True(t, ContainsField("{{literal}} {title}"))
}

// =====================================================================
// SplitOnFields (for regex building)
// =====================================================================

func TestSplitOnFields_Simple(t *testing.T) {
	parts := SplitOnFields("{id}: {name}")
	assert.Equal(t, []string{"", ": ", ""}, parts)
}

func TestSplitOnFields_NoFields(t *testing.T) {
	parts := SplitOnFields("plain text")
	assert.Equal(t, []string{"plain text"}, parts)
}

func TestSplitOnFields_FieldOnly(t *testing.T) {
	parts := SplitOnFields("{title}")
	assert.Equal(t, []string{"", ""}, parts)
}

func TestSplitOnFields_MultipleAdjacentFields(t *testing.T) {
	parts := SplitOnFields("{a}{b}")
	assert.Equal(t, []string{"", "", ""}, parts)
}

// =====================================================================
// Validate (template syntax check)
// =====================================================================

func TestValidate_Valid(t *testing.T) {
	assert.NoError(t, Validate("{title}"))
}

func TestValidate_ValidComplex(t *testing.T) {
	assert.NoError(t, Validate("- [{title}]({filename})"))
}

func TestValidate_ValidEscaped(t *testing.T) {
	assert.NoError(t, Validate("{{literal}} {title}"))
}

func TestValidate_UnclosedBrace(t *testing.T) {
	assert.Error(t, Validate("{title"))
}

func TestValidate_EmptyPlaceholder(t *testing.T) {
	assert.Error(t, Validate("{}"))
}

func TestValidate_StrayClosingBrace(t *testing.T) {
	assert.Error(t, Validate("}"))
}

func TestValidate_StrayClosingBraceInText(t *testing.T) {
	assert.Error(t, Validate("foo } bar"))
}

func TestValidate_FieldWithSpaces(t *testing.T) {
	assert.Error(t, Validate("{field name}"))
}

func TestValidate_NoFields(t *testing.T) {
	assert.NoError(t, Validate("plain text"))
}

func TestValidate_EscapedBracesOnly(t *testing.T) {
	assert.NoError(t, Validate("{{literal}}"))
}

// =====================================================================
// Nested CUE path access
// =====================================================================

func TestInterpolate_NestedField(t *testing.T) {
	data := map[string]any{
		"params": map[string]any{"subtitle": "Overview"},
	}
	assert.Equal(t, "Overview", Interpolate("{params.subtitle}", data))
}

func TestInterpolate_DeepNested(t *testing.T) {
	data := map[string]any{
		"a": map[string]any{"b": map[string]any{"c": "deep"}},
	}
	assert.Equal(t, "deep", Interpolate("{a.b.c}", data))
}

func TestInterpolate_QuotedKey(t *testing.T) {
	data := map[string]any{"my-key": "value"}
	assert.Equal(t, "value", Interpolate(`{"my-key"}`, data))
}

func TestInterpolate_QuotedKeyNested(t *testing.T) {
	data := map[string]any{
		"my-key": map[string]any{"sub": "nested"},
	}
	assert.Equal(t, "nested", Interpolate(`{"my-key".sub}`, data))
}

func TestInterpolate_QuotedKeyWithDot(t *testing.T) {
	data := map[string]any{"a.b": "dotted"}
	assert.Equal(t, "dotted", Interpolate(`{"a.b"}`, data))
}

func TestInterpolate_QuotedKeyDistinctFromNested(t *testing.T) {
	data := map[string]any{
		"a.b": "quoted-dot",
		"a":   map[string]any{"b": "nested"},
	}
	assert.Equal(t, "quoted-dot", Interpolate(`{"a.b"}`, data))
	assert.Equal(t, "nested", Interpolate("{a.b}", data))
}

func TestInterpolate_MissingNestedKey(t *testing.T) {
	data := map[string]any{"a": map[string]any{"b": "val"}}
	assert.Equal(t, "", Interpolate("{a.c}", data))
}

func TestInterpolate_NestedNotMap(t *testing.T) {
	data := map[string]any{"a": "string"}
	assert.Equal(t, "", Interpolate("{a.b}", data))
}

func TestInterpolate_NonStringValue(t *testing.T) {
	data := map[string]any{"count": 42}
	assert.Equal(t, "42", Interpolate("{count}", data))
}

func TestFields_NestedPath(t *testing.T) {
	fields := Fields("{a.b.c}")
	require.Len(t, fields, 1)
	assert.Equal(t, "a.b.c", fields[0])
}

func TestFields_QuotedKey(t *testing.T) {
	fields := Fields(`{"my-key".sub}`)
	require.Len(t, fields, 1)
	assert.Equal(t, `"my-key".sub`, fields[0])
}

func TestValidate_NestedPath(t *testing.T) {
	assert.NoError(t, Validate("{a.b.c}"))
}

func TestValidate_QuotedKey(t *testing.T) {
	assert.NoError(t, Validate(`{"my-key"}`))
}

func TestValidate_QuotedKeyNested(t *testing.T) {
	assert.NoError(t, Validate(`{"my-key".sub}`))
}

func TestResolvePath_Success(t *testing.T) {
	data := map[string]any{"params": map[string]any{"title": "Hello"}}
	val, err := ResolvePath(data, []string{"params", "title"})
	assert.NoError(t, err)
	assert.Equal(t, "Hello", val)
}

func TestResolvePath_MissingKey(t *testing.T) {
	data := map[string]any{"a": map[string]any{"b": "val"}}
	_, err := ResolvePath(data, []string{"a", "c"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "a.c")
}

func TestResolvePath_NotAMap(t *testing.T) {
	data := map[string]any{"a": "string"}
	_, err := ResolvePath(data, []string{"a", "b"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not a map")
}

func TestDiagnoseYAMLQuoting_String(t *testing.T) {
	assert.Equal(t, "", DiagnoseYAMLQuoting("row", "- {title}"))
}

func TestDiagnoseYAMLQuoting_Map(t *testing.T) {
	val := map[string]any{"title": nil}
	msg := DiagnoseYAMLQuoting("row", val)
	assert.Contains(t, msg, "quote")
	assert.Contains(t, msg, "{title}")
}

func TestDiagnoseYAMLQuoting_NonMap(t *testing.T) {
	assert.Equal(t, "", DiagnoseYAMLQuoting("row", 42))
}
