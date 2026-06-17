package notrailingpunctuation

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCheck_NilASTMatchesAST pins the parse-skipped path byte-identical to
// the AST path over headings whose trailing text comes from inline markup:
// emphasis at the end, a link inside emphasis, and a code span holding
// bracket text. The flattened heading text drives the trailing-punctuation
// verdict, so any divergence in the inline re-parse would change the
// diagnostic set.
func TestCheck_NilASTMatchesAST(t *testing.T) {
	cases := map[string]string{
		"emphasis ends in period": "# Done *already.*\n",
		"link inside emphasis":    "# See *[home.](/h)*\n",
		"code span brackets":      "# Use `a[0]:`\n",
		"clean heading":           "# All good *here*\n",
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

func TestCheck_NoPunctuation_NoViolation(t *testing.T) {
	src := []byte("# Title\n\n## Section\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 0, "expected 0 diagnostics, got %d: %+v", len(diags), diags)
}

func TestCheck_TrailingPeriod(t *testing.T) {
	src := []byte("# Title.\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 1, "expected 1 diagnostic, got %d: %+v", len(diags), diags)
	if diags[0].RuleID != "MDS017" {
		t.Errorf("expected rule ID MDS017, got %s", diags[0].RuleID)
	}
}

func TestCheck_TrailingComma(t *testing.T) {
	src := []byte("# Title,\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 1, "expected 1 diagnostic, got %d: %+v", len(diags), diags)
}

func TestCheck_TrailingColon(t *testing.T) {
	src := []byte("# Title:\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 1, "expected 1 diagnostic, got %d: %+v", len(diags), diags)
}

func TestCheck_TrailingSemicolon(t *testing.T) {
	src := []byte("# Title;\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 1, "expected 1 diagnostic, got %d: %+v", len(diags), diags)
}

func TestCheck_TrailingExclamation(t *testing.T) {
	src := []byte("# Title!\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 1, "expected 1 diagnostic, got %d: %+v", len(diags), diags)
}

func TestCheck_QuestionMark_NoViolation(t *testing.T) {
	src := []byte("# Is this a title?\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 0, "expected 0 diagnostics, got %d: %+v", len(diags), diags)
}

func TestCheck_NoHeadings(t *testing.T) {
	src := []byte("Some text.\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 0, "expected 0 diagnostics, got %d", len(diags))
}

func TestCheck_EllipsisHeading_NoViolation(t *testing.T) {
	src := []byte("## ...\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 0, "expected 0 diagnostics, got %d: %+v", len(diags), diags)
}

func TestID(t *testing.T) {
	r := &Rule{}
	if r.ID() != "MDS017" {
		t.Errorf("expected MDS017, got %s", r.ID())
	}
}

func TestName(t *testing.T) {
	r := &Rule{}
	if r.Name() != "no-trailing-punctuation-in-heading" {
		t.Errorf("expected no-trailing-punctuation-in-heading, got %s", r.Name())
	}
}
