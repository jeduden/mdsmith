package output

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJSONFormatter_ValidJSON(t *testing.T) {
	f := &JSONFormatter{}
	var buf bytes.Buffer

	diagnostics := []lint.Diagnostic{
		{
			File:     "README.md",
			Line:     10,
			Column:   5,
			RuleID:   "MDS001",
			RuleName: "line-length",
			Severity: lint.Error,
			Message:  "line too long (120 > 80)",
		},
	}

	err := f.Format(&buf, diagnostics)
	require.NoError(t, err, "unexpected error: %v", err)

	// Verify the output is valid JSON
	var result []JSONRecord
	require.NoError(t, json.Unmarshal(buf.Bytes(), &result), "output is not valid JSON: %s", buf.String())
}

func TestJSONFormatter_CorrectFieldNamesAndValues(t *testing.T) {
	f := &JSONFormatter{}
	var buf bytes.Buffer

	diagnostics := []lint.Diagnostic{
		{
			File:     "README.md",
			Line:     10,
			Column:   5,
			RuleID:   "MDS001",
			RuleName: "line-length",
			Severity: lint.Error,
			Message:  "line too long (120 > 80)",
		},
	}

	err := f.Format(&buf, diagnostics)
	require.NoError(t, err, "unexpected error: %v", err)

	// Unmarshal into a generic structure to verify field names
	var rawResult []map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &rawResult), "failed to unmarshal JSON")
	require.Len(t, rawResult, 1)

	item := rawResult[0]

	// Verify all expected field names exist
	expectedFields := []string{"file", "line", "column", "rule", "name", "severity", "message"}
	for _, field := range expectedFields {
		assert.Contains(t, item, field, "missing field %q in JSON output", field)
	}

	// Verify values (JSON numbers are float64 when unmarshaled into any)
	assert.Equal(t, "README.md", item["file"])
	assert.Equal(t, float64(10), item["line"])
	assert.Equal(t, float64(5), item["column"])
	assert.Equal(t, "MDS001", item["rule"])
	assert.Equal(t, "line-length", item["name"])
	assert.Equal(t, "error", item["severity"])
	assert.Equal(t, "line too long (120 > 80)", item["message"])
}

func TestJSONFormatter_EmptyDiagnostics(t *testing.T) {
	f := &JSONFormatter{}
	var buf bytes.Buffer

	err := f.Format(&buf, []lint.Diagnostic{})
	require.NoError(t, err, "unexpected error: %v", err)

	output := buf.String()

	// Verify it produces [] (not null)
	var result []JSONRecord
	require.NoError(t, json.Unmarshal(buf.Bytes(), &result), "output is not valid JSON: %s", output)
	require.NotNil(t, result, "expected non-nil empty slice, got nil")
	assert.Empty(t, result)

	// Verify the raw output starts with [] (not null)
	trimmed := bytes.TrimSpace(buf.Bytes())
	assert.Equal(t, "[]", string(trimmed))
}

func TestJSONFormatter_MultipleDiagnostics(t *testing.T) {
	f := &JSONFormatter{}
	var buf bytes.Buffer

	diagnostics := []lint.Diagnostic{
		{
			File:     "README.md",
			Line:     10,
			Column:   5,
			RuleID:   "MDS001",
			RuleName: "line-length",
			Severity: lint.Error,
			Message:  "line too long (120 > 80)",
		},
		{
			File:     "docs/guide.md",
			Line:     3,
			Column:   1,
			RuleID:   "MDS002",
			RuleName: "first-heading",
			Severity: lint.Warning,
			Message:  "first line should be a heading",
		},
	}

	result := formatAndUnmarshal(t, f, &buf, diagnostics)

	require.Len(t, result, 2, "expected 2 elements, got %d", len(result))

	assertJSONDiag(t, result[0], "README.md", "MDS001", "error", "line-length", 10)
	assertJSONDiag(t, result[1], "docs/guide.md", "MDS002", "warning", "first-heading", 3)
}

func formatAndUnmarshal(t *testing.T, f *JSONFormatter, buf *bytes.Buffer, diags []lint.Diagnostic) []JSONRecord {
	t.Helper()
	require.NoError(t, f.Format(buf, diags), "unexpected error")
	var result []JSONRecord
	require.NoError(t, json.Unmarshal(buf.Bytes(), &result), "output is not valid JSON")
	return result
}

func assertJSONDiag(t *testing.T, got JSONRecord, file, ruleID, severity, name string, line int) {
	t.Helper()
	assert.Equal(t, file, got.File, "file mismatch")
	assert.Equal(t, line, got.Line, "line mismatch")
	assert.Equal(t, ruleID, got.Rule, "rule mismatch")
	assert.Equal(t, severity, got.Severity, "severity mismatch")
	if name != "" {
		assert.Equal(t, name, got.Name, "name mismatch")
	}
}

func TestJSONFormatter_ExactOutput(t *testing.T) {
	f := &JSONFormatter{}
	var buf bytes.Buffer

	diagnostics := []lint.Diagnostic{
		{
			File:     "README.md",
			Line:     10,
			Column:   5,
			RuleID:   "MDS001",
			RuleName: "line-length",
			Severity: lint.Error,
			Message:  "line too long (120 > 80)",
		},
	}

	err := f.Format(&buf, diagnostics)
	require.NoError(t, err, "unexpected error: %v", err)

	expected := `[
  {
    "file": "README.md",
    "line": 10,
    "column": 5,
    "rule": "MDS001",
    "name": "line-length",
    "severity": "error",
    "message": "line too long (120 \u003e 80)"
  }
]
`
	assert.Equal(t, expected, buf.String())
}

func TestJSONFormatter_WithSourceContext(t *testing.T) {
	f := &JSONFormatter{}
	var buf bytes.Buffer

	diagnostics := []lint.Diagnostic{
		{
			File:            "README.md",
			Line:            5,
			Column:          81,
			RuleID:          "MDS001",
			RuleName:        "line-length",
			Severity:        lint.Error,
			Message:         "line too long",
			SourceLines:     []string{"before", "the long line", "after"},
			SourceStartLine: 4,
		},
	}

	result := formatAndUnmarshal(t, f, &buf, diagnostics)
	require.Len(t, result, 1)

	assert.Equal(t, []string{"before", "the long line", "after"}, result[0].SourceLines)
	assert.Equal(t, 4, result[0].SourceStartLine)
}

func TestJSONFormatter_SourceContextOmittedWhenEmpty(t *testing.T) {
	f := &JSONFormatter{}
	var buf bytes.Buffer

	diagnostics := []lint.Diagnostic{
		{
			File:     "README.md",
			Line:     1,
			Column:   1,
			RuleID:   "MDS001",
			RuleName: "line-length",
			Severity: lint.Error,
			Message:  "some issue",
		},
	}

	err := f.Format(&buf, diagnostics)
	require.NoError(t, err)

	// source_lines and source_start_line should be omitted from JSON
	var rawResult []map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &rawResult))

	_, hasSourceLines := rawResult[0]["source_lines"]
	_, hasSourceStartLine := rawResult[0]["source_start_line"]
	assert.False(t, hasSourceLines, "source_lines should be omitted when empty")
	assert.False(t, hasSourceStartLine, "source_start_line should be omitted when zero")
}

func TestJSONFormatter_ImplementsFormatter(t *testing.T) {
	var _ Formatter = &JSONFormatter{}
}
