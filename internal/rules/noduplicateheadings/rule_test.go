package noduplicateheadings

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/pkg/goldmark/ast"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCheck_NilASTMatchesAST pins the parse-skipped path byte-identical to
// the AST path over headings whose text comes from inline markup: emphasis
// at a heading's end, a link inside emphasis, and a code span holding
// bracket text. The flattened heading text drives both the duplicate key
// and the diagnostic message, so any divergence in the inline re-parse
// would surface as a different diagnostic set.
func TestCheck_NilASTMatchesAST(t *testing.T) {
	cases := map[string]string{
		"emphasis at end":      "# Title *one*\n\n## Body\n\n# Title *one*\n",
		"link inside emphasis": "# See *[home](/h)*\n\n## Body\n\n# See *[home](/h)*\n",
		"code span brackets":   "# Use `a[0]`\n\n## Body\n\n# Use `a[0]`\n",
		"no duplicates":        "# Alpha *x*\n\n## Beta `y`\n",
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			r := &Rule{}

			fAST, err := lint.NewFile("test.md", []byte(src))
			require.NoError(t, err)
			astDiags := r.Check(fAST)

			fNil, err := lint.NewFile("test.md", []byte(src))
			require.NoError(t, err)
			fNil.AST = nil
			nilDiags := r.Check(fNil)

			assert.Equal(t, astDiags, nilDiags)
		})
	}
}

func TestCheck_NoDuplicates_NoViolation(t *testing.T) {
	src := []byte("# Title\n\n## Section A\n\n## Section B\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 0, "expected 0 diagnostics, got %d: %+v", len(diags), diags)
}

func TestCheck_DuplicateHeadings(t *testing.T) {
	src := []byte("# Title\n\n## Section\n\n## Section\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 1, "expected 1 diagnostic, got %d: %+v", len(diags), diags)
	if diags[0].RuleID != "MDS005" {
		t.Errorf("expected rule ID MDS005, got %s", diags[0].RuleID)
	}
}

func TestCheck_DuplicatesDifferentLevels(t *testing.T) {
	src := []byte("# Title\n\n## Title\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 1, "expected 1 diagnostic, got %d: %+v", len(diags), diags)
}

func TestCheck_MultipleDuplicates(t *testing.T) {
	src := []byte("# Title\n\n## Title\n\n### Title\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 2, "expected 2 diagnostics, got %d: %+v", len(diags), diags)
}

func TestCheck_NoHeadings(t *testing.T) {
	src := []byte("Some text.\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 0, "expected 0 diagnostics, got %d", len(diags))
}

func TestCheck_DuplicateEllipsisAllowed(t *testing.T) {
	src := []byte("# Title\n\n## ...\n\n## ...\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 0, "expected 0 diagnostics, got %d: %+v", len(diags), diags)
}

func TestID(t *testing.T) {
	r := &Rule{}
	if r.ID() != "MDS005" {
		t.Errorf("expected MDS005, got %s", r.ID())
	}
}

func TestName(t *testing.T) {
	r := &Rule{}
	if r.Name() != "no-duplicate-headings" {
		t.Errorf("expected no-duplicate-headings, got %s", r.Name())
	}
}

// TestInlineCapable pins that InlineCapable returns true, which tells the
// engine to route this rule to its own Check on nil-AST files rather than
// dropping it. This covers the single-line method added by the KindScopedChecker
// conversion and keeps codecov/patch green for the new code.
func TestInlineCapable(t *testing.T) {
	r := &Rule{}
	assert.True(t, r.InlineCapable())
}

// TestCheck_NoStateLeakAcrossFiles pins the per-file reset contract: a
// single rule instance reused across two files (simulating engine worker
// reuse) must produce correct diagnostics for each file independently.
// File A registers heading text "Shared"; without a reset, File B's
// identical heading would be diagnosed as a duplicate even though it is
// the first (and only) occurrence in File B.
func TestCheck_NoStateLeakAcrossFiles(t *testing.T) {
	r := &Rule{}

	// File A: one heading "Shared". After this call, if the seen map were
	// stored on the struct without reset, {"Shared": 1} would persist.
	fileA, err := lint.NewFile("a.md", []byte("# Shared\n"))
	require.NoError(t, err)
	diagsA := r.Check(fileA)
	assert.Empty(t, diagsA, "File A should produce no diagnostics")

	// File B: also has "Shared" as its only heading. It is the first
	// occurrence in File B, so no diagnostic should be produced.
	// With leaked state (seen["Shared"]=1 from File A), the rule would
	// incorrectly flag it as a duplicate — the stale-state bug.
	fileB, err := lint.NewFile("b.md", []byte("# Shared\n"))
	require.NoError(t, err)
	diagsB := r.Check(fileB)
	assert.Empty(t, diagsB, "File B must not flag 'Shared' as duplicate; state leaked from File A: %v", diagsB)
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

// TestBeginFile_ReplacesSeenMap pins two properties of the reset: the
// previous file's heading texts are gone, and the map is a NEW allocation
// rather than a cleared one. Clearing would let two clones shallow-copied
// from one instance keep writing through a shared backing store — a
// concurrent map write, which is a non-recoverable fatal, not a race the
// detector merely reports.
func TestBeginFile_ReplacesSeenMap(t *testing.T) {
	r := &Rule{}
	r.BeginFile(nil)
	first := r.seen
	first["stale"] = 1

	r.BeginFile(nil)
	require.NotNil(t, r.seen)
	assert.Empty(t, r.seen, "the previous file's headings must be gone")
	assert.Len(t, first, 1, "the old map must be left untouched, not cleared in place")
}

// TestCheckNode_IgnoresLeavingVisitsAndNonHeadings pins CheckNode's two
// guards directly: neither a leaving visit nor a non-heading node records
// anything in the seen set.
func TestCheckNode_IgnoresLeavingVisitsAndNonHeadings(t *testing.T) {
	f, err := lint.NewFile("t.md", []byte("# Same\n\ntext\n\n# Same\n"))
	require.NoError(t, err)
	heading := f.AST.FirstChild()
	require.Equal(t, ast.KindHeading, heading.Kind())

	r := &Rule{}
	r.BeginFile(f)
	assert.Nil(t, r.CheckNode(heading, false, f), "leaving visits produce nothing")
	assert.Nil(t, r.CheckNode(f.AST, true, f), "a non-heading node produces nothing")
	assert.Empty(t, r.seen, "neither guard may record a heading")
}

// TestCheckNode_WithoutBeginFileDoesNotPanic pins the defensive lazy
// initialisation of the seen map. verdict writes into the map, and a write
// to a nil map is a non-recoverable panic that would kill the CLI or LSP
// process, so a caller that skips BeginFile must degrade to "no headings
// seen yet" instead.
func TestCheckNode_WithoutBeginFileDoesNotPanic(t *testing.T) {
	f, err := lint.NewFile("t.md", []byte("# Same\n\ntext\n\n# Same\n"))
	require.NoError(t, err)
	first := f.AST.FirstChild()
	second := first.NextSibling().NextSibling()
	require.Equal(t, ast.KindHeading, second.Kind())

	r := &Rule{} // no BeginFile: r.seen is nil
	require.Nil(t, r.seen)

	assert.Nil(t, r.CheckNode(first, true, f), "the first heading is not a duplicate")
	require.NotNil(t, r.seen, "CheckNode must initialise the map it writes into")

	diags := r.CheckNode(second, true, f)
	require.Len(t, diags, 1, "the repeat is still caught once the map exists")
	assert.Equal(t, `duplicate heading "Same" (first defined on line 1)`, diags[0].Message)
}

// TestCheckNode_SkipsWildcardHeading pins the reserved `...` marker used by
// required-structure prototypes: it is never recorded and never flagged,
// however often it repeats.
func TestCheckNode_SkipsWildcardHeading(t *testing.T) {
	f, err := lint.NewFile("t.md", []byte("# ...\n\ntext\n\n# ...\n"))
	require.NoError(t, err)

	r := &Rule{}
	assert.Empty(t, r.Check(f), "the wildcard heading is never a duplicate")
	assert.Empty(t, r.seen, "the wildcard heading is never recorded")
}
