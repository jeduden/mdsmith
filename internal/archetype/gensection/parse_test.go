package gensection

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseYAMLBody_RejectsAnchor(t *testing.T) {
	mp := MarkerPair{
		StartLine: 1,
		YAMLBody:  "base: &base\n  key: val\n",
	}
	_, diags := ParseYAMLBody("test.md", mp, "MDS999", "test-rule")
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "anchors/aliases are not permitted")
}

func TestParseYAMLBody_AcceptsClean(t *testing.T) {
	mp := MarkerPair{
		StartLine: 1,
		YAMLBody:  "key: value\n",
	}
	raw, diags := ParseYAMLBody("test.md", mp, "MDS999", "test-rule")
	assert.Empty(t, diags)
	assert.Equal(t, "value", raw["key"])
}

func TestToStringSlice_AllStrings(t *testing.T) {
	items := []any{"alpha", "beta", "gamma"}
	result, err := toStringSlice(items)
	require.NoError(t, err)
	assert.Equal(t, []string{"alpha", "beta", "gamma"}, result)
}

func TestToStringSlice_Empty(t *testing.T) {
	result, err := toStringSlice([]any{})
	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestToStringSlice_NonStringElement(t *testing.T) {
	items := []any{"ok", 42, "also-ok"}
	_, err := toStringSlice(items)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "element 1")
}

// =====================================================================
// Phase 5: additional branch coverage
// =====================================================================

// ValidateStringParams: []any with non-string element → toStringSlice error
func TestValidateStringParams_ListWithNonStringElement(t *testing.T) {
	rawMap := map[string]any{
		"glob": []any{"docs/**", 42},
	}
	params, diags := ValidateStringParams("test.md", 1, rawMap, "MDS999", "test-rule")
	assert.Nil(t, params)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "non-string element")
}

// ValidateStringParams: []any all strings → joined with "\n"
func TestValidateStringParams_ListAllStrings(t *testing.T) {
	rawMap := map[string]any{
		"glob": []any{"docs/**", "plan/**"},
	}
	params, diags := ValidateStringParams("test.md", 1, rawMap, "MDS999", "test-rule")
	assert.Empty(t, diags)
	assert.Equal(t, "docs/**\nplan/**", params["glob"])
}

// ValidateStringParams: float64 value → decimal string
func TestValidateStringParams_Float64Value(t *testing.T) {
	rawMap := map[string]any{
		"min-level": float64(2),
	}
	params, diags := ValidateStringParams("test.md", 1, rawMap, "MDS999", "test-rule")
	assert.Empty(t, diags)
	assert.Equal(t, "2", params["min-level"])
}

// ValidateStringParams: non-string non-sequence default case
func TestValidateStringParams_BoolValue(t *testing.T) {
	rawMap := map[string]any{
		"enabled": true,
	}
	params, diags := ValidateStringParams("test.md", 1, rawMap, "MDS999", "test-rule")
	assert.Nil(t, params)
	require.Len(t, diags, 1)
}

// ParseColumnConfig: non-map value for column → continue
func TestParseColumnConfig_NonMapValue(t *testing.T) {
	raw := map[string]any{
		"col1": "not-a-map",
		"col2": map[string]any{"max-width": 30},
	}
	result := ParseColumnConfig(raw)
	require.NotNil(t, result)
	// "col1" is skipped (not a map), "col2" is parsed.
	_, hasCol1 := result["col1"]
	assert.False(t, hasCol1, "non-map column should be skipped")
	col2, hasCol2 := result["col2"]
	assert.True(t, hasCol2)
	assert.Equal(t, 30, col2.MaxWidth)
}

// ParseColumnConfig: float64 max-width
func TestParseColumnConfig_Float64MaxWidth(t *testing.T) {
	raw := map[string]any{
		"col": map[string]any{"max-width": float64(50)},
	}
	result := ParseColumnConfig(raw)
	require.NotNil(t, result)
	col := result["col"]
	assert.Equal(t, 50, col.MaxWidth)
}

// ExtractContent: empty range when ContentFrom > ContentTo
func TestExtractContent_EmptyRange(t *testing.T) {
	mp := MarkerPair{ContentFrom: 5, ContentTo: 3}
	result := ExtractContent(nil, mp)
	assert.Equal(t, "", result)
}

// ExtractContent: range produces no lines (ContentFrom==ContentTo but all past file)
func TestExtractContent_EmptyLines(t *testing.T) {
	f := &lint.File{Path: "test.md", Lines: [][]byte{}}
	mp := MarkerPair{ContentFrom: 1, ContentTo: 1}
	result := ExtractContent(f, mp)
	assert.Equal(t, "", result)
}

// ExtractColumnsRaw: "columns" key exists but is not a map[string]any → return nil
func TestExtractColumnsRaw_NonMapValue(t *testing.T) {
	rawMap := map[string]any{
		"columns": "not-a-map",
		"other":   "keep",
	}
	result := ExtractColumnsRaw(rawMap)
	assert.Nil(t, result, "non-map columns value should return nil")
	// The "columns" key should have been deleted even when the type is wrong.
	_, hasColumns := rawMap["columns"]
	assert.False(t, hasColumns, "columns key should be deleted from rawMap")
}
