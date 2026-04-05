package fieldinterp

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// --- ContainsField additional tests ---

func TestContainsField_EmptyString(t *testing.T) {
	assert.False(t, ContainsField(""))
}

func TestContainsField_OnlyEscapedClosingBrace(t *testing.T) {
	assert.False(t, ContainsField("foo}}bar"))
}

func TestContainsField_NestedField(t *testing.T) {
	assert.True(t, ContainsField("{a.b.c}"))
}

func TestContainsField_QuotedKey(t *testing.T) {
	assert.True(t, ContainsField(`{"my-key"}`))
}

// --- SplitOnFields additional tests ---

func TestSplitOnFields_EmptyString(t *testing.T) {
	parts := SplitOnFields("")
	assert.Equal(t, []string{""}, parts)
}

func TestSplitOnFields_EscapedBraces(t *testing.T) {
	parts := SplitOnFields("{{literal}} {title}")
	// The escaped braces become literal { and }, then the field is removed.
	assert.Len(t, parts, 2)
	assert.Equal(t, "{literal} ", parts[0])
	assert.Equal(t, "", parts[1])
}

func TestSplitOnFields_TrailingText(t *testing.T) {
	parts := SplitOnFields("{title} end")
	assert.Equal(t, []string{"", " end"}, parts)
}

func TestSplitOnFields_LeadingText(t *testing.T) {
	parts := SplitOnFields("start {title}")
	assert.Equal(t, []string{"start ", ""}, parts)
}

func TestSplitOnFields_NestedField(t *testing.T) {
	parts := SplitOnFields("[{a.b}]({filename})")
	assert.Equal(t, []string{"[", "](", ")"}, parts)
}

func TestSplitOnFields_EscapedClosingBrace(t *testing.T) {
	parts := SplitOnFields("before}} {title}")
	assert.Len(t, parts, 2)
	assert.Equal(t, "before} ", parts[0])
}

// --- Stringify additional tests ---

func TestStringify_String(t *testing.T) {
	assert.Equal(t, "hello", Stringify("hello"))
}

func TestStringify_Nil(t *testing.T) {
	assert.Equal(t, "", Stringify(nil))
}

func TestStringify_Bool(t *testing.T) {
	assert.Equal(t, "true", Stringify(true))
	assert.Equal(t, "false", Stringify(false))
}

func TestStringify_Int(t *testing.T) {
	assert.Equal(t, "42", Stringify(42))
}

func TestStringify_Int64(t *testing.T) {
	assert.Equal(t, "100", Stringify(int64(100)))
}

func TestStringify_Float64(t *testing.T) {
	assert.Equal(t, "3.14", Stringify(3.14))
}

func TestStringify_Map(t *testing.T) {
	assert.Equal(t, "", Stringify(map[string]any{"a": "b"}))
}

func TestStringify_Slice(t *testing.T) {
	assert.Equal(t, "", Stringify([]any{"x", "y"}))
}

func TestStringify_CustomType(t *testing.T) {
	type custom struct{ X int }
	result := Stringify(custom{X: 42})
	assert.Contains(t, result, "42")
}

// --- ResolvePath additional tests ---

func TestResolvePath_NilData(t *testing.T) {
	_, err := ResolvePath(nil, []string{"key"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestResolvePath_CompositeLeafMap(t *testing.T) {
	data := map[string]any{"a": map[string]any{"b": "val"}}
	_, err := ResolvePath(data, []string{"a"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "composite value")
}

func TestResolvePath_CompositeLeafSlice(t *testing.T) {
	data := map[string]any{"a": []any{"x", "y"}}
	_, err := ResolvePath(data, []string{"a"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "composite value")
}

func TestResolvePath_BoolValue(t *testing.T) {
	data := map[string]any{"active": true}
	val, err := ResolvePath(data, []string{"active"})
	assert.NoError(t, err)
	assert.Equal(t, "true", val)
}

func TestResolvePath_IntValue(t *testing.T) {
	data := map[string]any{"count": 42}
	val, err := ResolvePath(data, []string{"count"})
	assert.NoError(t, err)
	assert.Equal(t, "42", val)
}

func TestResolvePath_NilValue(t *testing.T) {
	data := map[string]any{"key": nil}
	val, err := ResolvePath(data, []string{"key"})
	assert.NoError(t, err)
	assert.Equal(t, "", val)
}

// --- DiagnoseYAMLQuoting additional tests ---

func TestDiagnoseYAMLQuoting_Nil(t *testing.T) {
	assert.Equal(t, "", DiagnoseYAMLQuoting("row", nil))
}

func TestDiagnoseYAMLQuoting_MapWithNonNilValue(t *testing.T) {
	// A map with non-nil values is likely genuine, not a YAML quoting issue.
	val := map[string]any{"key": "value"}
	assert.Equal(t, "", DiagnoseYAMLQuoting("row", val))
}

func TestDiagnoseYAMLQuoting_MultipleKeys(t *testing.T) {
	val := map[string]any{"a": nil, "b": nil}
	msg := DiagnoseYAMLQuoting("row", val)
	assert.Contains(t, msg, "{...}")
	assert.Contains(t, msg, "quote")
}

// --- ParseCUEPath additional tests ---

func TestParseCUEPath_SimpleIdentifier(t *testing.T) {
	segments := ParseCUEPath("title")
	assert.Equal(t, []string{"title"}, segments)
}

func TestParseCUEPath_Nested(t *testing.T) {
	segments := ParseCUEPath("a.b.c")
	assert.Equal(t, []string{"a", "b", "c"}, segments)
}

func TestParseCUEPath_QuotedKey(t *testing.T) {
	segments := ParseCUEPath(`"my-key"`)
	assert.Equal(t, []string{"my-key"}, segments)
}

func TestParseCUEPath_MixedQuotedAndIdent(t *testing.T) {
	segments := ParseCUEPath(`params."my-key"`)
	assert.Equal(t, []string{"params", "my-key"}, segments)
}
