package headingincrement

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/jeduden/mdsmith/pkg/goldmark/ast"
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
	assert.Nil(t, diags, "Check must return nil, not an empty slice, when nothing is flagged")
}

// TestCheck_NilAST_ProperIncrement_ReturnsNil mirrors
// TestCheck_ProperIncrement_NoViolation for the parse-skip path
// (checkNilAST): a clean heading sequence must return nil, not a
// pre-allocated empty slice, per docs/development/high-performance-go.md
// ("Return nil, not []T{}").
func TestCheck_NilAST_ProperIncrement_ReturnsNil(t *testing.T) {
	src := []byte("# H1\n\n## H2\n\n### H3\n")
	r := &Rule{}
	diags := r.Check(lint.NewFileLines("test.md", src))
	assert.Nil(t, diags, "checkNilAST must return nil, not an empty slice, when nothing is flagged")
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

func TestWordlistTarget(t *testing.T) {
	assert.Equal(t, "placeholders", (&Rule{}).WordlistTarget())
}

// TestCheck_NoStateLeakAcrossFiles pins the per-file reset contract: a
// single rule instance reused across two files (simulating engine worker
// reuse) must produce correct diagnostics for each file independently.
// File A ends with heading level 3; without a reset, File B's first
// heading (level 3) would be treated as a continuation of A's sequence
// rather than the file's first heading, silently dropping the
// "first heading level should be 1" diagnostic.
func TestCheck_NoStateLeakAcrossFiles(t *testing.T) {
	r := &Rule{}

	// File A: clean sequence h1 → h2 → h3. No diagnostics. After this
	// call, if prevLevel were stored on the struct, it would be 3.
	fileA, err := lint.NewFile("a.md", []byte("# H1\n\n## H2\n\n### H3\n"))
	require.NoError(t, err)
	diagsA := r.Check(fileA)
	assert.Empty(t, diagsA, "File A should produce no diagnostics")

	// File B: first heading is h3. With a clean slate (prevLevel=0),
	// Check must diagnose "first heading level should be 1, got 3".
	// With leaked state (prevLevel=3), the first-heading check is
	// skipped and no diagnostic is produced — the stale-state bug.
	fileB, err := lint.NewFile("b.md", []byte("### Third only\n"))
	require.NoError(t, err)
	diagsB := r.Check(fileB)
	require.Len(t, diagsB, 1, "File B must diagnose first-heading violation; got: %v", diagsB)
	assert.Equal(t, "first heading level should be 1, got 3", diagsB[0].Message)
}

// TestEnteringKinds pins the kind scope CheckNode declares: headings only,
// and the same backing array on every call so the engine's per-file table
// build allocates nothing for it.
func TestEnteringKinds(t *testing.T) {
	r := &Rule{}
	assert.Equal(t, []ast.NodeKind{ast.KindHeading}, r.EnteringKinds())
	assert.Equal(t, &r.EnteringKinds()[0], &r.EnteringKinds()[0],
		"EnteringKinds must return a package-level slice, not a fresh allocation")
}

// TestLinesCapable pins the marker that routes this rule to its own Check
// on a parse-skipped File. Without it checker.classifySlot gives the rule
// no dispatch slot at all on a nil-AST File and every heading-increment
// diagnostic silently disappears.
func TestLinesCapable(t *testing.T) {
	assert.True(t, (&Rule{}).LinesCapable())
	assert.True(t, (&Rule{Placeholders: []string{"heading-question"}}).LinesCapable(),
		"the marker is constant; LineCapable is what gates the skip on placeholders")
}

// TestBeginFile_ResetsPrevLevel pins the reset itself, independently of a
// walk: a stale prevLevel from a previous file must be cleared so the next
// file's first heading is judged as a first heading.
func TestBeginFile_ResetsPrevLevel(t *testing.T) {
	r := &Rule{prevLevel: 4}
	r.BeginFile(nil)
	assert.Zero(t, r.prevLevel)
}

// TestCheckNode_IgnoresLeavingVisitsAndNonHeadings pins CheckNode's two
// guards directly. The kind-scoped dispatch never shows it a leaving visit
// or a non-heading node, but rule.WalkNodes' generic path and any future
// caller might, and neither must advance prevLevel.
func TestCheckNode_IgnoresLeavingVisitsAndNonHeadings(t *testing.T) {
	f, err := lint.NewFile("t.md", []byte("### Third\n\ntext\n"))
	require.NoError(t, err)
	heading := f.AST.FirstChild()
	require.Equal(t, ast.KindHeading, heading.Kind())

	r := &Rule{}
	assert.Nil(t, r.CheckNode(heading, false, f), "leaving visits produce nothing")
	assert.Zero(t, r.prevLevel, "a leaving visit must not advance prevLevel")

	assert.Nil(t, r.CheckNode(f.AST, true, f), "a non-heading node produces nothing")
	assert.Zero(t, r.prevLevel, "a non-heading node must not advance prevLevel")
}

// TestCheckNode_ThreadsPrevLevelAcrossHeadings pins the per-node state
// machine: the first heading is judged against "nothing seen yet", each
// later heading against its predecessor, and a placeholder heading updates
// prevLevel without emitting a diagnostic.
func TestCheckNode_ThreadsPrevLevelAcrossHeadings(t *testing.T) {
	f, err := lint.NewFile("t.md", []byte("# One\n\n### Three\n"))
	require.NoError(t, err)
	first := f.AST.FirstChild()
	second := first.NextSibling()
	require.Equal(t, ast.KindHeading, second.Kind())

	r := &Rule{}
	r.BeginFile(f)
	assert.Nil(t, r.CheckNode(first, true, f), "a level-1 first heading is clean")
	assert.Equal(t, 1, r.prevLevel)

	diags := r.CheckNode(second, true, f)
	require.Len(t, diags, 1)
	assert.Equal(t, "heading level incremented from 1 to 3 (expected 2)", diags[0].Message)
	assert.Equal(t, 3, r.prevLevel, "prevLevel advances even when the heading is flagged")
}

// TestCheckNode_PlaceholderHeadingAdvancesPrevLevelSilently pins that an
// opaque placeholder heading suppresses its own diagnostic but still moves
// the sequence forward, so the heading after it is judged against it.
func TestCheckNode_PlaceholderHeadingAdvancesPrevLevelSilently(t *testing.T) {
	f, err := lint.NewFile("t.md", []byte("## ?\n\ntext\n"))
	require.NoError(t, err)
	heading := f.AST.FirstChild()

	r := &Rule{Placeholders: []string{"heading-question"}}
	r.BeginFile(f)
	assert.Nil(t, r.CheckNode(heading, true, f), "a placeholder heading is opaque")
	assert.Equal(t, 2, r.prevLevel, "a placeholder heading still advances the sequence")
}
