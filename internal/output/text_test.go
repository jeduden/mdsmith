package output

import (
	"bytes"
	"strings"
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSanitizeControl(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"plain text", "hello world", "hello world"},
		{"strips tab", "hello\tworld", "helloworld"},
		{"strips newline", "hello\nworld", "helloworld"},
		{"strips carriage return", "hello\rworld", "helloworld"},
		{"strips ESC", "hello\x1bworld", "helloworld"},
		{"strips BEL", "hello\x07world", "helloworld"},
		{"strips CSI UTF-8", "hello\xc2\x9bworld", "helloworld"},
		{"strips DEL", "hello\x7fworld", "helloworld"},
		{"strips null", "hello\x00world", "helloworld"},
		{"strips full ANSI sequence", "\x1b[31mred\x1b[0m", "[31mred[0m"},
		{"C1 range stripped UTF-8", "a\xc2\x80\xc2\x81\xc2\x9e\xc2\x9fb", "ab"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeControl(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestSanitizeSourceLine(t *testing.T) {
	assert.Equal(t, "hello\tworld", sanitizeSourceLine("hello\tworld"), "tabs preserved")
	assert.Equal(t, "helloworld", sanitizeSourceLine("hello\nworld"), "newlines stripped")
	assert.Equal(t, "helloworld", sanitizeSourceLine("hello\x1bworld"), "ESC stripped")
}

func TestTextFormatter_SanitizesHeaderFields(t *testing.T) {
	f := &TextFormatter{Color: false}
	var buf bytes.Buffer

	diagnostics := []lint.Diagnostic{
		{
			File:    "evil\x1b[31m.md",
			Line:    1,
			Column:  1,
			RuleID:  "MDS001",
			Message: "bad\x07stuff\x1b[0m",
		},
	}

	err := f.Format(&buf, diagnostics)
	require.NoError(t, err)

	output := buf.String()
	assert.NotContains(t, output, "\x1b")
	assert.NotContains(t, output, "\x07")
	assert.Contains(t, output, "evil[31m.md")
	assert.Contains(t, output, "bad")
}

func TestTextFormatter_SanitizesSourceLines(t *testing.T) {
	f := &TextFormatter{Color: false}
	var buf bytes.Buffer

	diagnostics := []lint.Diagnostic{
		{
			File:            "test.md",
			Line:            1,
			Column:          1,
			RuleID:          "MDS001",
			Message:         "test",
			SourceLines:     []string{"normal", "injected\x1b[31mred\x1b[0m"},
			SourceStartLine: 1,
		},
	}

	err := f.Format(&buf, diagnostics)
	require.NoError(t, err)

	output := buf.String()
	assert.NotContains(t, output, "\x1b")
	assert.Contains(t, output, "injected[31mred[0m")
}

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

	assert.Contains(t, output, "\033[36m", "expected cyan for file location")
	assert.Contains(t, output, "\033[33m", "expected yellow for rule ID")
	assert.Contains(t, output, "\033[0m", "expected ANSI reset")

	expected := "\033[36mREADME.md:10:5\033[0m \033[33mMDS001\033[0m line too long (120 > 80)\n"
	assert.Equal(t, expected, output)
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

	assert.NotContains(t, buf.String(), "\033[", "expected no ANSI escape sequences")

	expected := "README.md:10:5 MDS001 line too long (120 > 80)\n"
	assert.Equal(t, expected, buf.String())
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

	// gutterWidth=2 (max line=12), column=81
	// caret dots = gutterWidth(2) + column(81) + 2 = 85
	expected := "README.md:10:81 MDS001 line too long (120 > 80)\n" +
		" 8 | Some normal line before.\n" +
		" 9 | Another normal line.\n" +
		"10 | This is a very long line that exceeds the 80 character limit and keeps going on and on\n" +
		strings.Repeat("·", 85) + "^\n" +
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

	// Column=1: dot path from col 0 to col 1
	// gutterWidth=1, dots = 1+1+2 = 4
	expected := "test.md:1:1 MDS009 first line should be a heading\n" +
		"1 | Some paragraph text.\n" +
		"····^\n" +
		"2 | Next line.\n"

	assert.Equal(t, expected, buf.String())
}

func TestTextFormatter_SnippetColumnOneWithColor(t *testing.T) {
	f := &TextFormatter{Color: true}
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

	output := buf.String()

	// Red caret in dot path
	assert.Contains(t, output, "\033[31m^\033[0m", "expected red caret")
	// Context line should be dim
	assert.Contains(t, output, "\033[2m", "expected dim context line")
	// Diagnostic line should NOT be dim
	assert.NotContains(t, output, "\033[2m1 |", "diagnostic line should not be dim")
}

func TestTextFormatter_SnippetMultipleDiagsSameFile(t *testing.T) {
	f := &TextFormatter{Color: false}
	var buf bytes.Buffer

	diagnostics := []lint.Diagnostic{
		{
			File:     "test.md",
			Line:     2,
			Column:   1,
			RuleID:   "MDS004",
			RuleName: "first-heading",
			Severity: lint.Error,
			Message:  "missing heading",
			SourceLines: []string{
				"line one",
				"line two",
				"line three",
			},
			SourceStartLine: 1,
		},
		{
			File:     "test.md",
			Line:     5,
			Column:   10,
			RuleID:   "MDS012",
			RuleName: "no-bare-urls",
			Severity: lint.Error,
			Message:  "bare URL",
			SourceLines: []string{
				"line four",
				"visit at https://example.com",
				"line six",
			},
			SourceStartLine: 4,
		},
	}

	err := f.Format(&buf, diagnostics)
	require.NoError(t, err)

	// Both diagnostics should appear with their own snippets
	// gutterWidth=1 for both (max line 3 and 6 respectively)
	expected := "test.md:2:1 MDS004 missing heading\n" +
		"1 | line one\n" +
		"2 | line two\n" +
		"····^\n" +
		"3 | line three\n" +
		"test.md:5:10 MDS012 bare URL\n" +
		"4 | line four\n" +
		"5 | visit at https://example.com\n" +
		"·············^\n" +
		"6 | line six\n"

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

	// gutterWidth=1, column=3, dots = 1+3+2 = 6
	expected := "test.md:2:3 MDS010 some issue\n" +
		"1 | # Title\n" +
		"2 |   bad indent\n" +
		"······^\n" +
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

	// gutterWidth=3, column=3, dots = 3+3+2 = 8
	expected := "big.md:100:3 MDS010 some issue\n" +
		" 98 | line 98\n" +
		" 99 | line 99\n" +
		"100 | line 100\n" +
		"········^\n" +
		"101 | line 101\n" +
		"102 | line 102\n"

	assert.Equal(t, expected, buf.String())
}

func TestTextFormatter_SnippetDotPathAlignment(t *testing.T) {
	f := &TextFormatter{Color: false}
	var buf bytes.Buffer

	diagnostics := []lint.Diagnostic{
		{
			File:     "test.md",
			Line:     5,
			Column:   10,
			RuleID:   "MDS012",
			RuleName: "no-bare-urls",
			Severity: lint.Error,
			Message:  "bare URL",
			SourceLines: []string{
				"line three",
				"line four",
				"visit at https://example.com today",
				"line six",
				"line seven",
			},
			SourceStartLine: 3,
		},
	}

	err := f.Format(&buf, diagnostics)
	require.NoError(t, err)

	// gutterWidth=1, column=10, dots = 1+10+2 = 13
	expected := "test.md:5:10 MDS012 bare URL\n" +
		"3 | line three\n" +
		"4 | line four\n" +
		"5 | visit at https://example.com today\n" +
		"·············^\n" +
		"6 | line six\n" +
		"7 | line seven\n"

	assert.Equal(t, expected, buf.String())

	// Verify the ^ sits under the right character.
	// "5 | visit at https://..." — content starts at rune position 4.
	// Column 10 → 'h' in "https" → rune position 4+9 = 13.
	lines := strings.Split(buf.String(), "\n")
	diagRunes := []rune(lines[3])  // "5 | visit at https://..."
	caretRunes := []rune(lines[4]) // "·············^"
	caretPos := -1
	for j, r := range caretRunes {
		if r == '^' {
			caretPos = j
			break
		}
	}
	require.GreaterOrEqual(t, caretPos, 0, "no ^ found in caret line")
	assert.Equal(t, 'h', diagRunes[caretPos], "caret should point at 'h' in https")
}
