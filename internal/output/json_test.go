package output

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/jeduden/tidymark/internal/lint"
)

func TestJSONFormatter_ValidJSON(t *testing.T) {
	f := &JSONFormatter{}
	var buf bytes.Buffer

	diagnostics := []lint.Diagnostic{
		{
			File:     "README.md",
			Line:     10,
			Column:   5,
			RuleID:   "TM001",
			RuleName: "line-length",
			Severity: lint.Error,
			Message:  "line too long (120 > 80)",
		},
	}

	err := f.Format(&buf, diagnostics)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

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
			RuleID:   "TM001",
			RuleName: "line-length",
			Severity: lint.Error,
			Message:  "line too long (120 > 80)",
		},
	}

	err := f.Format(&buf, diagnostics)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Unmarshal into a generic structure to verify field names
	var rawResult []map[string]any
	if err := json.Unmarshal(buf.Bytes(), &rawResult); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	if len(rawResult) != 1 {
		t.Fatalf("expected 1 element, got %d", len(rawResult))
	}

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
	if item["rule"] != "TM001" {
		t.Errorf("rule: got %v, want %q", item["rule"], "TM001")
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
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()

	// Verify it produces [] (not null)
	var result []jsonDiagnostic
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %s", err, output)
	}

	if result == nil {
		t.Error("expected non-nil empty slice, got nil")
	}

	if len(result) != 0 {
		t.Errorf("expected 0 elements, got %d", len(result))
	}

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
			RuleID:   "TM001",
			RuleName: "line-length",
			Severity: lint.Error,
			Message:  "line too long (120 > 80)",
		},
		{
			File:     "docs/guide.md",
			Line:     3,
			Column:   1,
			RuleID:   "TM002",
			RuleName: "first-heading",
			Severity: lint.Warning,
			Message:  "first line should be a heading",
		},
	}

	result := formatAndUnmarshal(t, f, &buf, diagnostics)

	if len(result) != 2 {
		t.Fatalf("expected 2 elements, got %d", len(result))
	}

	assertJSONDiag(t, result[0], "README.md", "TM001", "error", "", 10)
	assertJSONDiag(t, result[1], "docs/guide.md", "TM002", "warning", "first-heading", 3)
}

func formatAndUnmarshal(t *testing.T, f *JSONFormatter, buf *bytes.Buffer, diags []lint.Diagnostic) []jsonDiagnostic {
	t.Helper()
	if err := f.Format(buf, diags); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var result []jsonDiagnostic
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	return result
}

func assertJSONDiag(t *testing.T, got jsonDiagnostic, file, ruleID, severity, name string, line int) {
	t.Helper()
	if got.File != file {
		t.Errorf("file: got %q, want %q", got.File, file)
	}
	if got.Line != line {
		t.Errorf("line: got %d, want %d", got.Line, line)
	}
	if got.Rule != ruleID {
		t.Errorf("rule: got %q, want %q", got.Rule, ruleID)
	}
	if got.Severity != severity {
		t.Errorf("severity: got %q, want %q", got.Severity, severity)
	}
	if name != "" && got.Name != name {
		t.Errorf("name: got %q, want %q", got.Name, name)
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
			RuleID:   "TM001",
			RuleName: "line-length",
			Severity: lint.Error,
			Message:  "line too long (120 > 80)",
		},
	}

	err := f.Format(&buf, diagnostics)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := `[
  {
    "file": "README.md",
    "line": 10,
    "column": 5,
    "rule": "TM001",
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

func TestJSONFormatter_ImplementsFormatter(t *testing.T) {
	var _ Formatter = &JSONFormatter{}
}
