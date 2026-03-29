package output

import (
	"bytes"
	"strings"
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTextFormatter_SingleDiagnostic(t *testing.T) {
	f := &TextFormatter{Color: false}
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

	expected := "README.md:10:5 MDS001 line too long (120 > 80)\n"
	assert.Equal(t, expected, buf.String())
}

func TestTextFormatter_MultipleDiagnostics(t *testing.T) {
	f := &TextFormatter{Color: false}
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

	err := f.Format(&buf, diagnostics)
	require.NoError(t, err, "unexpected error: %v", err)

	lines := strings.Split(strings.TrimSuffix(buf.String(), "\n"), "\n")
	require.Len(t, lines, 2, "expected 2 lines, got %d: %q", len(lines), buf.String())

	assert.Equal(t, "README.md:10:5 MDS001 line too long (120 > 80)", lines[0])
	assert.Equal(t, "docs/guide.md:3:1 MDS002 first line should be a heading", lines[1])
}

func TestTextFormatter_WithColor(t *testing.T) {
	f := &TextFormatter{Color: true}
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

	output := buf.String()

	// Verify ANSI escape sequences are present
	assert.Contains(t, output, "\033[36m", "expected cyan ANSI escape sequence (\\033[36m) in output")
	assert.Contains(t, output, "\033[33m", "expected yellow ANSI escape sequence (\\033[33m) in output")
	assert.Contains(t, output, "\033[0m", "expected reset ANSI escape sequence (\\033[0m) in output")

	// Verify exact colored output
	expected := "\033[36mREADME.md:10:5\033[0m \033[33mMDS001\033[0m line too long (120 > 80)\n"
	assert.Equal(t, expected, output, "got %q, want %q", output, expected)
}

func TestTextFormatter_WithoutColor(t *testing.T) {
	f := &TextFormatter{Color: false}
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

	output := buf.String()

	// Verify no ANSI escape sequences
	assert.NotContains(t, output, "\033[", "expected no ANSI escape sequences in output, but found some")

	expected := "README.md:10:5 MDS001 line too long (120 > 80)\n"
	assert.Equal(t, expected, output, "got %q, want %q", output, expected)
}

func TestTextFormatter_EmptyDiagnostics(t *testing.T) {
	f := &TextFormatter{Color: false}
	var buf bytes.Buffer

	err := f.Format(&buf, []lint.Diagnostic{})
	require.NoError(t, err, "unexpected error: %v", err)

	assert.Empty(t, buf.String(), "expected empty output for no diagnostics")
}

func TestTextFormatter_ImplementsFormatter(t *testing.T) {
	var _ Formatter = &TextFormatter{}
}

func TestTextFormatter_SnippetContext(t *testing.T) {
	f := &TextFormatter{Color: false}
	var buf bytes.Buffer

	diagnostics := []lint.Diagnostic{
		{
			File:     "README.md",
			Line:     10,
			Column:   81,
			RuleID:   "MDS001",
			RuleName: "line-length",
			Severity: lint.Error,
			Message:  "line too long (120 > 80)",
			SourceLines: []string{
				"Some normal line before.",
				"Another normal line.",
				"This is a very long line that exceeds the 80 character limit and keeps going on and on",
				"The line after the issue.",
				"Another line after.",
			},
			SourceStartLine: 8,
		},
	}

	err := f.Format(&buf, diagnostics)
	require.NoError(t, err)

	expected := "README.md:10:81 MDS001 line too long (120 > 80)\n" +
		" 8 | Some normal line before.\n" +
		" 9 | Another normal line.\n" +
		"10 | This is a very long line that exceeds the 80 character limit and keeps going on and on\n" +
		"   | " + strings.Repeat("·", 80) + "^\n" +
		"11 | The line after the issue.\n" +
		"12 | Another line after.\n"

	assert.Equal(t, expected, buf.String())
}

func TestTextFormatter_SnippetWithColor(t *testing.T) {
	f := &TextFormatter{Color: true}
	var buf bytes.Buffer

	diagnostics := []lint.Diagnostic{
		{
			File:     "README.md",
			Line:     3,
			Column:   5,
			RuleID:   "MDS001",
			RuleName: "line-length",
			Severity: lint.Error,
			Message:  "line too long",
			SourceLines: []string{
				"line one",
				"line two",
				"line three",
			},
			SourceStartLine: 1,
		},
	}

	err := f.Format(&buf, diagnostics)
	require.NoError(t, err)

	output := buf.String()

	// Caret should be red
	assert.Contains(t, output, "\033[31m^\033[0m", "expected red caret")
	// Context lines should be dim
	assert.Contains(t, output, "\033[2m", "expected dim ANSI code for context lines")
}

func TestTextFormatter_SnippetEmptySourceLines(t *testing.T) {
	f := &TextFormatter{Color: false}
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
	require.NoError(t, err)

	// No snippet lines — just the diagnostic header
	expected := "README.md:10:5 MDS001 line too long (120 > 80)\n"
	assert.Equal(t, expected, buf.String())
}

func TestTextFormatter_SnippetColumnOne(t *testing.T) {
	f := &TextFormatter{Color: false}
	var buf bytes.Buffer

	diagnostics := []lint.Diagnostic{
		{
			File:     "test.md",
			Line:     1,
			Column:   1,
			RuleID:   "MDS009",
			RuleName: "first-heading",
			Severity: lint.Error,
			Message:  "first line should be a heading",
			SourceLines: []string{
				"Some paragraph text.",
				"Next line.",
			},
			SourceStartLine: 1,
		},
	}

	err := f.Format(&buf, diagnostics)
	require.NoError(t, err)

	expected := "test.md:1:1 MDS009 first line should be a heading\n" +
		"1 | Some paragraph text.\n" +
		"  | ^\n" +
		"2 | Next line.\n"

	assert.Equal(t, expected, buf.String())
}

func TestTextFormatter_SnippetAtFileStart(t *testing.T) {
	f := &TextFormatter{Color: false}
	var buf bytes.Buffer

	diagnostics := []lint.Diagnostic{
		{
			File:     "test.md",
			Line:     2,
			Column:   3,
			RuleID:   "MDS010",
			RuleName: "some-rule",
			Severity: lint.Error,
			Message:  "some issue",
			SourceLines: []string{
				"# Title",
				"  bad indent",
				"normal line",
			},
			SourceStartLine: 1,
		},
	}

	err := f.Format(&buf, diagnostics)
	require.NoError(t, err)

	expected := "test.md:2:3 MDS010 some issue\n" +
		"1 | # Title\n" +
		"2 |   bad indent\n" +
		"  | ··^\n" +
		"3 | normal line\n"

	assert.Equal(t, expected, buf.String())
}

func TestTextFormatter_SnippetColumnZero(t *testing.T) {
	f := &TextFormatter{Color: false}
	var buf bytes.Buffer

	diagnostics := []lint.Diagnostic{
		{
			File:     "test.md",
			Line:     2,
			Column:   0,
			RuleID:   "MDS010",
			RuleName: "some-rule",
			Severity: lint.Error,
			Message:  "some issue",
			SourceLines: []string{
				"line one",
				"line two",
				"line three",
			},
			SourceStartLine: 1,
		},
	}

	err := f.Format(&buf, diagnostics)
	require.NoError(t, err)

	// Column 0: no caret line
	expected := "test.md:2:0 MDS010 some issue\n" +
		"1 | line one\n" +
		"2 | line two\n" +
		"3 | line three\n"

	assert.Equal(t, expected, buf.String())
}

func TestTextFormatter_SnippetThreeDigitLineNumbers(t *testing.T) {
	f := &TextFormatter{Color: false}
	var buf bytes.Buffer

	diagnostics := []lint.Diagnostic{
		{
			File:     "big.md",
			Line:     100,
			Column:   3,
			RuleID:   "MDS010",
			RuleName: "some-rule",
			Severity: lint.Error,
			Message:  "some issue",
			SourceLines: []string{
				"line 98",
				"line 99",
				"line 100",
				"line 101",
				"line 102",
			},
			SourceStartLine: 98,
		},
	}

	err := f.Format(&buf, diagnostics)
	require.NoError(t, err)

	// 3-digit gutter width for all lines
	expected := "big.md:100:3 MDS010 some issue\n" +
		" 98 | line 98\n" +
		" 99 | line 99\n" +
		"100 | line 100\n" +
		"    | ··^\n" +
		"101 | line 101\n" +
		"102 | line 102\n"

	assert.Equal(t, expected, buf.String())
}
