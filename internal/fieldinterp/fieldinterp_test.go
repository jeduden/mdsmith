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
	got := Interpolate("{title}", map[string]string{"title": "Hello"})
	assert.Equal(t, "Hello", got)
}

func TestInterpolate_MultipleFields(t *testing.T) {
	got := Interpolate("{id}: {name}", map[string]string{"id": "MDS001", "name": "line-length"})
	assert.Equal(t, "MDS001: line-length", got)
}

func TestInterpolate_MissingFieldEmpty(t *testing.T) {
	got := Interpolate("{title}", map[string]string{})
	assert.Equal(t, "", got)
}

func TestInterpolate_MixedPresentMissing(t *testing.T) {
	got := Interpolate("- [{title}]({filename})", map[string]string{"filename": "a.md"})
	assert.Equal(t, "- [](a.md)", got)
}

func TestInterpolate_NoPlaceholders(t *testing.T) {
	got := Interpolate("plain text", map[string]string{"title": "Hello"})
	assert.Equal(t, "plain text", got)
}

func TestInterpolate_EmptyString(t *testing.T) {
	got := Interpolate("", map[string]string{"title": "Hello"})
	assert.Equal(t, "", got)
}

func TestInterpolate_EscapedBrace(t *testing.T) {
	got := Interpolate("{{literal}} {title}", map[string]string{"title": "Hello"})
	assert.Equal(t, "{literal} Hello", got)
}

func TestInterpolate_EscapedClosingBrace(t *testing.T) {
	got := Interpolate("{title} end}}", map[string]string{"title": "Hello"})
	assert.Equal(t, "Hello end}", got)
}

func TestInterpolate_OnlyEscapedBraces(t *testing.T) {
	got := Interpolate("{{no}} {{fields}}", map[string]string{})
	assert.Equal(t, "{no} {fields}", got)
}

func TestInterpolate_NilData(t *testing.T) {
	got := Interpolate("{title}", nil)
	assert.Equal(t, "", got)
}

func TestInterpolate_AdjacentPlaceholders(t *testing.T) {
	got := Interpolate("{a}{b}", map[string]string{"a": "X", "b": "Y"})
	assert.Equal(t, "XY", got)
}

func TestInterpolate_FieldWithHyphen(t *testing.T) {
	got := Interpolate("{my-field}", map[string]string{"my-field": "value"})
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

func TestValidate_NoFields(t *testing.T) {
	assert.NoError(t, Validate("plain text"))
}
