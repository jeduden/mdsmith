package output

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// errorWriter always returns an error.
type errorWriter struct{}

func (e *errorWriter) Write(_ []byte) (int, error) {
	return 0, fmt.Errorf("write error")
}

// --- Format error handling ---

func TestFormat_WriterError(t *testing.T) {
	f := &TextFormatter{Color: false}
	w := &errorWriter{}

	diagnostics := []lint.Diagnostic{
		{
			File:     "test.md",
			Line:     1,
			Column:   1,
			RuleID:   "MDS001",
			RuleName: "test",
			Message:  "test error",
		},
	}

	err := f.Format(w, diagnostics)
	assert.Error(t, err, "expected error from failing writer")
}

func TestFormat_WriterErrorWithColor(t *testing.T) {
	f := &TextFormatter{Color: true}
	w := &errorWriter{}

	diagnostics := []lint.Diagnostic{
		{
			File:     "test.md",
			Line:     1,
			Column:   1,
			RuleID:   "MDS001",
			RuleName: "test",
			Message:  "test error",
		},
	}

	err := f.Format(w, diagnostics)
	assert.Error(t, err)
}

func TestFormat_SnippetWriterError(t *testing.T) {
	// Use a writer that fails after the first write.
	w := &limitedWriter{limit: 1}
	f := &TextFormatter{Color: false}

	diagnostics := []lint.Diagnostic{
		{
			File:     "test.md",
			Line:     1,
			Column:   1,
			RuleID:   "MDS001",
			RuleName: "test",
			Message:  "test",
			SourceLines: []string{
				"line one",
			},
			SourceStartLine: 1,
		},
	}

	err := f.Format(w, diagnostics)
	assert.Error(t, err)
}

// --- formatSnippet edge cases ---

func TestFormatSnippet_SingleDigitLineNumber(t *testing.T) {
	f := &TextFormatter{Color: false}
	var buf bytes.Buffer

	diagnostics := []lint.Diagnostic{
		{
			File:     "test.md",
			Line:     1,
			Column:   5,
			RuleID:   "MDS001",
			RuleName: "test",
			Message:  "test",
			SourceLines: []string{
				"hello world",
			},
			SourceStartLine: 1,
		},
	}

	err := f.Format(&buf, diagnostics)
	require.NoError(t, err)

	output := buf.String()
	// gutterWidth=1, column=5, dots = 1+5+2 = 8
	assert.Contains(t, output, strings.Repeat("·", 8)+"^")
}

func TestFormatSnippet_TwoDigitLineNumber(t *testing.T) {
	f := &TextFormatter{Color: false}
	var buf bytes.Buffer

	diagnostics := []lint.Diagnostic{
		{
			File:     "test.md",
			Line:     10,
			Column:   3,
			RuleID:   "MDS001",
			RuleName: "test",
			Message:  "test",
			SourceLines: []string{
				"line nine",
				"line ten",
				"line eleven",
			},
			SourceStartLine: 9,
		},
	}

	err := f.Format(&buf, diagnostics)
	require.NoError(t, err)

	output := buf.String()
	// gutterWidth=2, column=3, dots = 2+3+2 = 7
	assert.Contains(t, output, strings.Repeat("·", 7)+"^")
}

func TestFormatSnippet_FourDigitLineNumber(t *testing.T) {
	f := &TextFormatter{Color: false}
	var buf bytes.Buffer

	diagnostics := []lint.Diagnostic{
		{
			File:     "big.md",
			Line:     1000,
			Column:   1,
			RuleID:   "MDS001",
			RuleName: "test",
			Message:  "test",
			SourceLines: []string{
				"line 999",
				"line 1000",
				"line 1001",
			},
			SourceStartLine: 999,
		},
	}

	err := f.Format(&buf, diagnostics)
	require.NoError(t, err)

	output := buf.String()
	// gutterWidth=4, column=1, dots = 4+1+2 = 7
	assert.Contains(t, output, strings.Repeat("·", 7)+"^")
	// Verify alignment: 3-digit line numbers are padded.
	assert.Contains(t, output, " 999 |")
	assert.Contains(t, output, "1000 |")
	assert.Contains(t, output, "1001 |")
}

func TestFormatSnippet_ColorContextDimmed(t *testing.T) {
	f := &TextFormatter{Color: true}
	var buf bytes.Buffer

	diagnostics := []lint.Diagnostic{
		{
			File:     "test.md",
			Line:     2,
			Column:   1,
			RuleID:   "MDS001",
			RuleName: "test",
			Message:  "test",
			SourceLines: []string{
				"context line",
				"diag line",
				"context line 2",
			},
			SourceStartLine: 1,
		},
	}

	err := f.Format(&buf, diagnostics)
	require.NoError(t, err)

	output := buf.String()
	// Context lines should be dimmed.
	assert.Contains(t, output, "\033[2m")
	// Caret should be red.
	assert.Contains(t, output, "\033[31m^\033[0m")
}

func TestFormat_NilDiagnostics(t *testing.T) {
	f := &TextFormatter{Color: false}
	var buf bytes.Buffer

	err := f.Format(&buf, nil)
	require.NoError(t, err)
	assert.Empty(t, buf.String())
}

// limitedWriter fails after a certain number of writes.
type limitedWriter struct {
	limit int
	count int
}

func (w *limitedWriter) Write(p []byte) (int, error) {
	w.count++
	if w.count > w.limit {
		return 0, fmt.Errorf("write limit exceeded")
	}
	return len(p), nil
}
