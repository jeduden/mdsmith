package firstlineheading

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"

	"github.com/stretchr/testify/require"
)

func TestCheck_FirstLineH1_NoViolation(t *testing.T) {
	src := []byte("# Title\n\nSome text\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Level: 1}
	diags := r.Check(f)
	require.Len(t, diags, 0, "expected 0 diagnostics, got %d: %+v", len(diags), diags)
}

func TestCheck_EmptyFile(t *testing.T) {
	src := []byte("")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Level: 1}
	diags := r.Check(f)
	require.Len(t, diags, 1, "expected 1 diagnostic, got %d", len(diags))
	if diags[0].RuleID != "MDS004" {
		t.Errorf("expected rule ID MDS004, got %s", diags[0].RuleID)
	}
}

func TestCheck_StartsWithParagraph(t *testing.T) {
	src := []byte("Some text\n\n# Title\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Level: 1}
	diags := r.Check(f)
	require.Len(t, diags, 1, "expected 1 diagnostic, got %d: %+v", len(diags), diags)
}

func TestCheck_BlankLineThenHeading(t *testing.T) {
	src := []byte("\n# Title\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Level: 1}
	diags := r.Check(f)
	require.Len(t, diags, 1, "expected 1 diagnostic for heading not on line 1, got %d: %+v", len(diags), diags)
}

func TestCheck_WrongLevel(t *testing.T) {
	src := []byte("## Title\n\nSome text\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Level: 1}
	diags := r.Check(f)
	require.Len(t, diags, 1, "expected 1 diagnostic, got %d: %+v", len(diags), diags)
}

func TestCheck_Level2Config(t *testing.T) {
	src := []byte("## Title\n\nSome text\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Level: 2}
	diags := r.Check(f)
	require.Len(t, diags, 0, "expected 0 diagnostics, got %d: %+v", len(diags), diags)
}

func TestID(t *testing.T) {
	r := &Rule{}
	if r.ID() != "MDS004" {
		t.Errorf("expected MDS004, got %s", r.ID())
	}
}

func TestName(t *testing.T) {
	r := &Rule{}
	if r.Name() != "first-line-heading" {
		t.Errorf("expected first-line-heading, got %s", r.Name())
	}
}

// --- Configurable tests ---

func TestApplySettings_ValidLevel(t *testing.T) {
	r := &Rule{Level: 1}
	if err := r.ApplySettings(map[string]any{"level": 2}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Level != 2 {
		t.Errorf("expected Level=2, got %d", r.Level)
	}
}

func TestApplySettings_InvalidLevelType(t *testing.T) {
	r := &Rule{Level: 1}
	err := r.ApplySettings(map[string]any{"level": "not-a-number"})
	require.Error(t, err, "expected error for non-int level")
}

func TestApplySettings_LevelOutOfRange(t *testing.T) {
	r := &Rule{Level: 1}
	err := r.ApplySettings(map[string]any{"level": 7})
	require.Error(t, err, "expected error for level > 6")
}

func TestApplySettings_UnknownKey(t *testing.T) {
	r := &Rule{Level: 1}
	err := r.ApplySettings(map[string]any{"unknown": true})
	require.Error(t, err, "expected error for unknown key")
}

func TestDefaultSettings_FirstLineHeading(t *testing.T) {
	r := &Rule{}
	ds := r.DefaultSettings()
	if ds["level"] != 1 {
		t.Errorf("expected level=1, got %v", ds["level"])
	}
}
