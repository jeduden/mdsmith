package rule

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yuin/goldmark/ast"
)

// nodeCheckerStub emits one diagnostic per Heading node on entering,
// and records every (kind, entering) it is shown so the test can
// assert WalkNodes feeds the full pre-order node stream.
type nodeCheckerStub struct {
	visits []string
}

func (s *nodeCheckerStub) ID() string       { return "MDSX01" }
func (s *nodeCheckerStub) Name() string     { return "stub" }
func (s *nodeCheckerStub) Category() string { return "test" }

func (s *nodeCheckerStub) Check(f *lint.File) []lint.Diagnostic {
	return WalkNodes(s, f)
}

func (s *nodeCheckerStub) CheckNode(n ast.Node, entering bool, f *lint.File) []lint.Diagnostic {
	verb := "exit"
	if entering {
		verb = "enter"
	}
	s.visits = append(s.visits, verb+":"+n.Kind().String())
	if entering && n.Kind() == ast.KindHeading {
		return []lint.Diagnostic{{RuleID: s.ID(), Message: "heading seen"}}
	}
	return nil
}

var _ NodeChecker = (*nodeCheckerStub)(nil)

// TestWalkNodes_FeedsFullPreorderStreamAndConcatenates pins that
// WalkNodes drives one ast.Walk, shows CheckNode every node entering
// then leaving, and concatenates per-node diagnostics in document
// order. This is the contract the engine's multiplexed dispatch and
// a NodeChecker's standalone Check both rely on.
func TestWalkNodes_FeedsFullPreorderStreamAndConcatenates(t *testing.T) {
	f, err := lint.NewFile("t.md", []byte("# A\n\ntext\n\n## B\n"))
	require.NoError(t, err)

	s := &nodeCheckerStub{}
	diags := WalkNodes(s, f)

	require.Len(t, diags, 2, "one diagnostic per heading, in document order")
	assert.Equal(t, "heading seen", diags[0].Message)

	// Document root is entered first and left last; both headings are
	// shown entering.
	require.NotEmpty(t, s.visits)
	assert.Equal(t, "enter:Document", s.visits[0])
	assert.Equal(t, "exit:Document", s.visits[len(s.visits)-1])
	enterHeadings := 0
	for _, v := range s.visits {
		if v == "enter:Heading" {
			enterHeadings++
		}
	}
	assert.Equal(t, 2, enterHeadings)
}

// TestWalkNodes_EqualsManualWalk pins that WalkNodes is exactly a
// single ast.Walk over CheckNode — the equivalence the engine relies
// on so a multiplexed dispatch cannot change a rule's output.
func TestWalkNodes_EqualsManualWalk(t *testing.T) {
	f, err := lint.NewFile("t.md", []byte("# H\n\np\n\n### Deep\n\n- a\n"))
	require.NoError(t, err)

	s := &nodeCheckerStub{}
	viaHelper := WalkNodes(s, f)

	var manual []lint.Diagnostic
	ref := &nodeCheckerStub{}
	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		manual = append(manual, ref.CheckNode(n, entering, f)...)
		return ast.WalkContinue, nil
	})

	assert.Equal(t, manual, viaHelper)
}
