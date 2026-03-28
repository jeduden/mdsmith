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
	var result []jsonDiagnostic
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %s", err, buf.String())
	}
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
	if err := json.Unmarshal(buf.Bytes(), &rawResult); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	require.Len(t, rawResult, 1, "expected 1 element, got %d", len(rawResult))

	item := rawResult[0]

	// Verify all expected field names exist
	expectedFields := []string{"file", "line", "column", "rule", "name", "severity", "message"}
	for _, field := range expectedFields {
		if _, ok := item[field]; !ok {
			t.Errorf("missing field %q in JSON output", field)
		}
	}

	// Verify values
	if item["file"] != "README.md" {
		t.Errorf("file: got %v, want %q", item["file"], "README.md")
	}
	// JSON numbers are float64 when unmarshaled into any
	if item["line"] != float64(10) {
		t.Errorf("line: got %v, want %v", item["line"], 10)
	}
	if item["column"] != float64(5) {
		t.Errorf("column: got %v, want %v", item["column"], 5)
	}
	if item["rule"] != "MDS001" {
		t.Errorf("rule: got %v, want %q", item["rule"], "MDS001")
	}
	if item["name"] != "line-length" {
		t.Errorf("name: got %v, want %q", item["name"], "line-length")
	}
	if item["severity"] != "error" {
		t.Errorf("severity: got %v, want %q", item["severity"], "error")
	}
	if item["message"] != "line too long (120 > 80)" {
		t.Errorf("message: got %v, want %q", item["message"], "line too long (120 > 80)")
	}
}

func TestJSONFormatter_EmptyDiagnostics(t *testing.T) {
	f := &JSONFormatter{}
	var buf bytes.Buffer

	err := f.Format(&buf, []lint.Diagnostic{})
	require.NoError(t, err, "unexpected error: %v", err)

	output := buf.String()

	// Verify it produces [] (not null)
	var result []jsonDiagnostic
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %s", err, output)
	}

	assert.NotNil(t, result, "expected non-nil empty slice, got nil")

	assert.Len(t, result, 0, "expected 0 elements, got %d", len(result))

	// Verify the raw output starts with [] (not null)
	trimmed := bytes.TrimSpace(buf.Bytes())
	if string(trimmed) != "[]" {
		t.Errorf("expected raw output to be %q, got %q", "[]", string(trimmed))
	}
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

func formatAndUnmarshal(t *testing.T, f *JSONFormatter, buf *bytes.Buffer, diags []lint.Diagnostic) []jsonDiagnostic {
	t.Helper()
	require.NoError(t, f.Format(buf, diags), "unexpected error")
	var result []jsonDiagnostic
	require.NoError(t, json.Unmarshal(buf.Bytes(), &result), "output is not valid JSON")
	return result
}

func assertJSONDiag(t *testing.T, got jsonDiagnostic, file, ruleID, severity, name string, line int) {
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
	if buf.String() != expected {
		t.Errorf("got:\n%s\nwant:\n%s", buf.String(), expected)
	}
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
