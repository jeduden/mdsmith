package fencedcodestyle

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"

	"github.com/stretchr/testify/require"
)

func TestCheck_BacktickDefault_NoViolation(t *testing.T) {
	src := []byte("# Hello\n\n```go\nfmt.Println()\n```\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Style: "backtick"}
	diags := r.Check(f)
	require.Len(t, diags, 0, "expected 0 diagnostics, got %d", len(diags))
}

func TestCheck_TildeWhenBacktickExpected(t *testing.T) {
	src := []byte("# Hello\n\n~~~go\nfmt.Println()\n~~~\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Style: "backtick"}
	diags := r.Check(f)
	require.Len(t, diags, 1, "expected 1 diagnostic, got %d", len(diags))
	d := diags[0]
	if d.RuleID != "MDS010" {
		t.Errorf("expected rule ID MDS010, got %s", d.RuleID)
	}
	if d.Line != 3 {
		t.Errorf("expected line 3, got %d", d.Line)
	}
	if d.Message != "fenced code block should use backtick style" {
		t.Errorf("unexpected message: %s", d.Message)
	}
}

func TestCheck_TildeStyle_NoViolation(t *testing.T) {
	src := []byte("~~~python\nprint()\n~~~\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Style: "tilde"}
	diags := r.Check(f)
	require.Len(t, diags, 0, "expected 0 diagnostics, got %d", len(diags))
}

func TestCheck_BacktickWhenTildeExpected(t *testing.T) {
	src := []byte("```go\nfmt.Println()\n```\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Style: "tilde"}
	diags := r.Check(f)
	require.Len(t, diags, 1, "expected 1 diagnostic, got %d", len(diags))
	d := diags[0]
	if d.Line != 1 {
		t.Errorf("expected line 1, got %d", d.Line)
	}
	if d.Message != "fenced code block should use tilde style" {
		t.Errorf("unexpected message: %s", d.Message)
	}
}

func TestCheck_EmptyCodeBlock(t *testing.T) {
	src := []byte("```\n```\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Style: "backtick"}
	diags := r.Check(f)
	require.Len(t, diags, 0, "expected 0 diagnostics, got %d", len(diags))
}

func TestCheck_EmptyCodeBlockWrongStyle(t *testing.T) {
	src := []byte("~~~\n~~~\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Style: "backtick"}
	diags := r.Check(f)
	require.Len(t, diags, 1, "expected 1 diagnostic, got %d", len(diags))
}

func TestCheck_FourBackticks(t *testing.T) {
	src := []byte("````\ncode\n````\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Style: "backtick"}
	diags := r.Check(f)
	require.Len(t, diags, 0, "expected 0 diagnostics, got %d", len(diags))
}

func TestCheck_MultipleBlocks_MixedStyles(t *testing.T) {
	src := []byte("```go\nfmt.Println()\n```\n\n~~~python\nprint()\n~~~\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Style: "backtick"}
	diags := r.Check(f)
	require.Len(t, diags, 1, "expected 1 diagnostic, got %d", len(diags))
	if diags[0].Line != 5 {
		t.Errorf("expected line 5, got %d", diags[0].Line)
	}
}

func TestFix_TildeToBacktick(t *testing.T) {
	src := []byte("~~~go\nfmt.Println()\n~~~\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Style: "backtick"}
	result := r.Fix(f)
	expected := "```go\nfmt.Println()\n```\n"
	if string(result) != expected {
		t.Errorf("expected %q, got %q", expected, string(result))
	}
}

func TestFix_BacktickToTilde(t *testing.T) {
	src := []byte("```go\nfmt.Println()\n```\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Style: "tilde"}
	result := r.Fix(f)
	expected := "~~~go\nfmt.Println()\n~~~\n"
	if string(result) != expected {
		t.Errorf("expected %q, got %q", expected, string(result))
	}
}

func TestFix_PreservesLanguageTag(t *testing.T) {
	src := []byte("~~~python\nprint()\n~~~\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Style: "backtick"}
	result := r.Fix(f)
	expected := "```python\nprint()\n```\n"
	if string(result) != expected {
		t.Errorf("expected %q, got %q", expected, string(result))
	}
}

func TestFix_PreservesContent(t *testing.T) {
	src := []byte("# Title\n\n~~~go\nline1\nline2\n~~~\n\ntext\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Style: "backtick"}
	result := r.Fix(f)
	expected := "# Title\n\n```go\nline1\nline2\n```\n\ntext\n"
	if string(result) != expected {
		t.Errorf("expected %q, got %q", expected, string(result))
	}
}

func TestFix_NoChangeNeeded(t *testing.T) {
	src := []byte("```go\ncode\n```\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Style: "backtick"}
	result := r.Fix(f)
	if string(result) != string(src) {
		t.Errorf("expected no change, got %q", string(result))
	}
}

func TestFix_FourTildesToFourBackticks(t *testing.T) {
	src := []byte("~~~~go\ncode\n~~~~\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Style: "backtick"}
	result := r.Fix(f)
	expected := "````go\ncode\n````\n"
	if string(result) != expected {
		t.Errorf("expected %q, got %q", expected, string(result))
	}
}

func TestFix_EmptyCodeBlock(t *testing.T) {
	src := []byte("~~~\n~~~\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Style: "backtick"}
	result := r.Fix(f)
	expected := "```\n```\n"
	if string(result) != expected {
		t.Errorf("expected %q, got %q", expected, string(result))
	}
}

func TestCheck_ID(t *testing.T) {
	r := &Rule{Style: "backtick"}
	if r.ID() != "MDS010" {
		t.Errorf("expected ID MDS010, got %s", r.ID())
	}
}

func TestCheck_Name(t *testing.T) {
	r := &Rule{Style: "backtick"}
	if r.Name() != "fenced-code-style" {
		t.Errorf("expected name fenced-code-style, got %s", r.Name())
	}
}

// --- Configurable tests ---

func TestApplySettings_ValidStyle(t *testing.T) {
	r := &Rule{Style: "backtick"}
	if err := r.ApplySettings(map[string]any{"style": "tilde"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Style != "tilde" {
		t.Errorf("expected Style=tilde, got %s", r.Style)
	}
}

func TestApplySettings_InvalidStyle(t *testing.T) {
	r := &Rule{Style: "backtick"}
	err := r.ApplySettings(map[string]any{"style": "invalid"})
	require.Error(t, err, "expected error for invalid style")
}

func TestApplySettings_InvalidStyleType(t *testing.T) {
	r := &Rule{Style: "backtick"}
	err := r.ApplySettings(map[string]any{"style": 42})
	require.Error(t, err, "expected error for non-string style")
}

func TestApplySettings_UnknownKey(t *testing.T) {
	r := &Rule{Style: "backtick"}
	err := r.ApplySettings(map[string]any{"unknown": true})
	require.Error(t, err, "expected error for unknown key")
}

func TestDefaultSettings_FencedCodeStyle(t *testing.T) {
	r := &Rule{}
	ds := r.DefaultSettings()
	if ds["style"] != "backtick" {
		t.Errorf("expected style=backtick, got %v", ds["style"])
	}
}
