package headingincrement

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCheck_NilASTMatchesAST pins the Layer-0 migration: with empty
// placeholders Check on a nil-AST File (the parse-skip path, walking the
// Layer 0 block scan) must produce byte-identical diagnostics to the AST
// path across heading-level sequences, setext headings, missing first-h1,
// and the empty file.
func TestCheck_NilASTMatchesAST(t *testing.T) {
	srcs := [][]byte{
		[]byte("# H1\n\n## H2\n\n### H3\n"),
		[]byte("# Title\n\n### Subsection\n"),
		[]byte("## Starts at two\n\nText\n"),
		[]byte("# A\n\nSub\n---\n\n#### Deep\n"),
		[]byte("Setext one\n==========\n\nSetext two\n----------\n\n### Three\n"),
		[]byte("intro\n\nSetext\n------\n"),
		[]byte(""),
		[]byte("# A\n\n## B\n\n#### D\n\n## E\n"),
		[]byte("###### Six first\n"),
		// Indented ATX/setext headings (1–3 spaces): both paths must agree.
		[]byte("   # Indented one\n\n  ### Jump three\n"),
		[]byte("Title\n=====\n\n  Sub\n  ---\n"),
	}
	for _, src := range srcs {
		astFile, err := lint.NewFile("f.md", src)
		require.NoError(t, err)
		astDiags := (&Rule{}).Check(astFile)
		l0Diags := (&Rule{}).Check(lint.NewFileLines("f.md", src))
		assert.Equal(t, astDiags, l0Diags,
			"nil-AST must match AST for src=%q", string(src))
	}
}

// TestLineCapable reports the rule is line-capable only with no
// placeholder tokens configured.
func TestLineCapable(t *testing.T) {
	assert.True(t, (&Rule{}).LineCapable())
	assert.False(t, (&Rule{Placeholders: []string{"TODO"}}).LineCapable())
}

// TestCheck_NilASTWithPlaceholdersReturnsNil pins the defensive branch:
// with placeholders configured the gate never sends a nil-AST File, but
// Check must not dereference a nil AST if it ever does.
func TestCheck_NilASTWithPlaceholdersReturnsNil(t *testing.T) {
	src := []byte("# Title\n\n### Subsection\n")
	r := &Rule{Placeholders: []string{"TODO"}}
	assert.Nil(t, r.Check(lint.NewFileLines("f.md", src)))
}

func TestCheck_ProperIncrement_NoViolation(t *testing.T) {
	src := []byte("# H1\n\n## H2\n\n### H3\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 0, "expected 0 diagnostics, got %d: %+v", len(diags), diags)
}

func TestCheck_SkipsLevel(t *testing.T) {
	src := []byte("# H1\n\n### H3\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 1, "expected 1 diagnostic, got %d: %+v", len(diags), diags)
	if diags[0].RuleID != "MDS003" {
		t.Errorf("expected rule ID MDS003, got %s", diags[0].RuleID)
	}
}

func TestCheck_FirstHeadingH2_SkipsH1(t *testing.T) {
	src := []byte("## H2 as first heading\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 1, "expected 1 diagnostic, got %d: %+v", len(diags), diags)
	if diags[0].Message != "first heading level should be 1, got 2" {
		t.Errorf("unexpected message: %s", diags[0].Message)
	}
}

func TestCheck_DecreasingLevels_NoViolation(t *testing.T) {
	// Going from h3 back to h2 is fine
	src := []byte("# H1\n\n## H2\n\n### H3\n\n## H2 again\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 0, "expected 0 diagnostics, got %d: %+v", len(diags), diags)
}

func TestCheck_NoHeadings(t *testing.T) {
	src := []byte("Some text without headings.\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 0, "expected 0 diagnostics, got %d", len(diags))
}

func TestID(t *testing.T) {
	r := &Rule{}
	if r.ID() != "MDS003" {
		t.Errorf("expected MDS003, got %s", r.ID())
	}
}

func TestName(t *testing.T) {
	r := &Rule{}
	if r.Name() != "heading-increment" {
		t.Errorf("expected heading-increment, got %s", r.Name())
	}
}

// --- Placeholder tests ---

func TestCheck_PlaceholderHeadingQuestion_SkipsLevel(t *testing.T) {
	// A heading with text "?" configured as heading-question should not
	// produce a diagnostic even when its level skips.
	src := []byte("# H1\n\n### ?\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Placeholders: []string{"heading-question"}}
	diags := r.Check(f)
	require.Empty(t, diags, "heading-question placeholder should suppress skip-level diagnostic")
}

func TestCheck_PlaceholderSection_SkipsLevel(t *testing.T) {
	// A heading with text "..." configured as placeholder-section should not
	// produce a diagnostic even when its level skips.
	src := []byte("# H1\n\n### ...\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Placeholders: []string{"placeholder-section"}}
	diags := r.Check(f)
	require.Empty(t, diags, "placeholder-section should suppress skip-level diagnostic")
}

func TestCheck_PlaceholderHeadingQuestion_EmptyList_StillFlags(t *testing.T) {
	// Without placeholders configured, skipped levels are still flagged.
	src := []byte("# H1\n\n### ?\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Placeholders: []string{}}
	diags := r.Check(f)
	require.Len(t, diags, 1, "should flag skipped level without placeholders configured")
}

func TestCheck_Placeholder_LevelTracked(t *testing.T) {
	// Placeholder headings still update the level tracker.
	// After h1, a placeholder h3, h4 is ok (following placeholder's level).
	src := []byte("# H1\n\n### ?\n\n#### H4\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Placeholders: []string{"heading-question"}}
	diags := r.Check(f)
	require.Empty(t, diags, "h4 after placeholder h3 should be valid")
}

func TestApplySettings_Placeholders_HeadingIncrement(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{
		"placeholders": []any{"heading-question", "placeholder-section"},
	})
	require.NoError(t, err)
	require.Equal(t, []string{"heading-question", "placeholder-section"}, r.Placeholders)
}

func TestApplySettings_Placeholders_UnknownToken_HeadingIncrement(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"placeholders": []any{"bad-token"}})
	require.Error(t, err)
}

func TestApplySettings_Placeholders_NonList_HeadingIncrement(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"placeholders": "not-a-list"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "list of strings")
}

func TestApplySettings_UnknownKey_HeadingIncrement(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"unknownkey": true})
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown setting")
}

func TestDefaultSettings_HeadingIncrement(t *testing.T) {
	r := &Rule{}
	ds := r.DefaultSettings()
	require.Equal(t, []string{}, ds["placeholders"])
}

func TestSettingMergeMode_HeadingIncrement(t *testing.T) {
	r := &Rule{}
	assert.Equal(t, rule.MergeAppend, r.SettingMergeMode("placeholders"))
	assert.Equal(t, rule.MergeReplace, r.SettingMergeMode("unknown"))
}
