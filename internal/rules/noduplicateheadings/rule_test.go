package noduplicateheadings

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"

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
