package rule

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/pkg/goldmark/ast"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

// blockCheckerStub emits one diagnostic per thematic-break span and
// records every span kind it is shown, so the test can assert WalkBlocks
// dispatches only the kinds BlockKinds declares, in document order.
type blockCheckerStub struct {
	nodeCheckerStub
	seenKinds []lint.BlockKind
}

func (s *blockCheckerStub) CheckBlock(span lint.BlockSpan, f *lint.File) []lint.Diagnostic {
	s.seenKinds = append(s.seenKinds, span.Kind)
	return []lint.Diagnostic{{RuleID: s.ID(), Line: span.Start, Message: "block seen"}}
}

func (s *blockCheckerStub) BlockKinds() []lint.BlockKind {
	return []lint.BlockKind{lint.BlockThematicBreak}
}

var _ BlockChecker = (*blockCheckerStub)(nil)

// TestWalkBlocks_DispatchesScopedKindsInOrder pins that WalkBlocks drives
// CheckBlock only for spans whose kind is in BlockKinds, in document
// order, on a nil-AST File served from the Layer 0 scan.
func TestWalkBlocks_DispatchesScopedKindsInOrder(t *testing.T) {
	f := lint.NewFileLines("t.md", []byte("# Heading\n\ntext\n\n---\n\nmore\n\n***\n"))

	s := &blockCheckerStub{}
	diags := WalkBlocks(s, f)

	require.Len(t, diags, 2, "one diagnostic per thematic-break span")
	assert.Equal(t, 5, diags[0].Line, "first break at line 5")
	assert.Equal(t, 9, diags[1].Line, "second break at line 9")
	for _, k := range s.seenKinds {
		assert.Equal(t, lint.BlockThematicBreak, k,
			"only thematic-break spans are dispatched")
	}
}

// TestWalkBlocks_NilFile pins the defensive nil guard so unit-test stubs
// that pass a nil File do not crash.
func TestWalkBlocks_NilFile(t *testing.T) {
	assert.Nil(t, WalkBlocks(&blockCheckerStub{}, nil))
}

// TestWalkNodes_NilFileAndNilAST pins the defensive nil guard:
// unit-test stubs that construct `&lint.File{}` literals must not
// crash WalkNodes. The engine path never produces such files (NewFile
// always parses to a Document node), but rule tests build literals to
// exercise short-circuits and would panic without the guard.
func TestWalkNodes_NilFileAndNilAST(t *testing.T) {
	stub := &nodeCheckerStub{}

	// nil *lint.File — should return nil, not panic.
	assert.Nil(t, WalkNodes(stub, nil))
	assert.Empty(t, stub.visits, "no nodes visited when file is nil")

	// non-nil File with nil AST — same.
	assert.Nil(t, WalkNodes(stub, &lint.File{Path: "t.md"}))
	assert.Empty(t, stub.visits, "no nodes visited when AST is nil")
}

// fileResetterStub extends nodeCheckerStub with FileResetter so the
// BeginFile branch in WalkNodes is reached and covered.
type fileResetterStub struct {
	nodeCheckerStub
	beginCalls int
}

func (s *fileResetterStub) BeginFile(_ *lint.File) { s.beginCalls++ }

var _ FileResetter = (*fileResetterStub)(nil)

// TestWalkNodes_CallsBeginFileOnFileResetter pins that WalkNodes calls
// BeginFile exactly once per Call when the rule implements FileResetter.
// This covers the fr.BeginFile branch that plain NodeChecker stubs skip.
func TestWalkNodes_CallsBeginFileOnFileResetter(t *testing.T) {
	f, err := lint.NewFile("t.md", []byte("# Hello\n\ntext\n"))
	require.NoError(t, err)

	s := &fileResetterStub{}
	_ = WalkNodes(s, f)

	assert.Equal(t, 1, s.beginCalls, "BeginFile must be called once per WalkNodes call")
}

// TestBlockKindInSet pins the linear scan used by WalkBlocks to filter
// spans: empty set returns false, present kind returns true, absent
// kind returns false.
func TestBlockKindInSet(t *testing.T) {
	t.Run("emptySet", func(t *testing.T) {
		assert.False(t, blockKindInSet(lint.BlockParagraph, nil))
		assert.False(t, blockKindInSet(lint.BlockParagraph, []lint.BlockKind{}))
	})
	t.Run("present", func(t *testing.T) {
		kinds := []lint.BlockKind{lint.BlockParagraph, lint.BlockThematicBreak}
		assert.True(t, blockKindInSet(lint.BlockParagraph, kinds))
		assert.True(t, blockKindInSet(lint.BlockThematicBreak, kinds))
	})
	t.Run("absent", func(t *testing.T) {
		kinds := []lint.BlockKind{lint.BlockParagraph}
		assert.False(t, blockKindInSet(lint.BlockATXHeading, kinds))
	})
}

// kindScopedStub is a NodeChecker that also declares a kind scope, so
// WalkNodes must feed it only entering visits of the declared kinds.
type kindScopedStub struct {
	nodeCheckerStub
	kinds []ast.NodeKind
}

func (s *kindScopedStub) EnteringKinds() []ast.NodeKind { return s.kinds }

var _ KindScopedChecker = (*kindScopedStub)(nil)

// TestWalkNodes_ScopesToEnteringKinds pins that WalkNodes applies the
// same scoping the engine's dispatchScoped applies: a KindScopedChecker
// sees only entering visits of its declared kinds, never a leaving visit
// and never an unrelated kind. Without it a standalone Check pays two
// no-op CheckNode calls for every node in the document.
func TestWalkNodes_ScopesToEnteringKinds(t *testing.T) {
	f, err := lint.NewFile("t.md", []byte("# A\n\ntext\n\n## B\n"))
	require.NoError(t, err)

	s := &kindScopedStub{kinds: []ast.NodeKind{ast.KindHeading}}
	diags := WalkNodes(s, f)

	assert.Equal(t, []string{"enter:Heading", "enter:Heading"}, s.visits,
		"only entering visits of the declared kinds reach CheckNode")
	assert.Len(t, diags, 2, "the scoped stream still yields every heading diagnostic")
}

// TestWalkNodes_EmptyEnteringKindsDispatchesNothing pins the engine's
// treatment of an empty kind scope: buildKindTable gives such a rule no
// CSR row, so it is never dispatched. WalkNodes must agree rather than
// falling back to the unscoped every-node stream.
func TestWalkNodes_EmptyEnteringKindsDispatchesNothing(t *testing.T) {
	f, err := lint.NewFile("t.md", []byte("# A\n\ntext\n"))
	require.NoError(t, err)

	s := &kindScopedStub{kinds: nil}
	assert.Nil(t, WalkNodes(s, f))
	assert.Empty(t, s.visits, "an empty kind scope means no node is dispatched")
}

// blockFileResetterStub is both a BlockChecker and a FileResetter, the
// combination the block-span dispatch must reset before the first span.
type blockFileResetterStub struct {
	blockCheckerStub
	beginCalls int
}

func (s *blockFileResetterStub) BeginFile(_ *lint.File) {
	s.beginCalls++
	s.seenKinds = nil
}

var (
	_ BlockChecker = (*blockFileResetterStub)(nil)
	_ FileResetter = (*blockFileResetterStub)(nil)
)

// TestWalkBlocks_CallsBeginFileOnFileResetter pins that the block path
// honours FileResetter exactly as the node path does: state left over
// from a previous File is cleared before the first span is dispatched.
func TestWalkBlocks_CallsBeginFileOnFileResetter(t *testing.T) {
	s := &blockFileResetterStub{}
	s.seenKinds = []lint.BlockKind{lint.BlockParagraph} // stale from a previous File

	f := lint.NewFileLines("t.md", []byte("text\n\n---\n"))
	diags := WalkBlocks(s, f)

	assert.Equal(t, 1, s.beginCalls, "BeginFile must be called once per WalkBlocks call")
	require.Len(t, diags, 1)
	assert.Equal(t, []lint.BlockKind{lint.BlockThematicBreak}, s.seenKinds,
		"BeginFile must run before the first CheckBlock")
}

// TestNodeKindInSet pins the linear scan WalkNodes uses to apply a
// rule's kind scope: empty set returns false, present kind returns true,
// absent kind returns false.
func TestNodeKindInSet(t *testing.T) {
	t.Run("emptySet", func(t *testing.T) {
		assert.False(t, nodeKindInSet(ast.KindHeading, nil))
		assert.False(t, nodeKindInSet(ast.KindHeading, []ast.NodeKind{}))
	})
	t.Run("present", func(t *testing.T) {
		kinds := []ast.NodeKind{ast.KindParagraph, ast.KindHeading}
		assert.True(t, nodeKindInSet(ast.KindHeading, kinds))
		assert.True(t, nodeKindInSet(ast.KindParagraph, kinds))
	})
	t.Run("absent", func(t *testing.T) {
		assert.False(t, nodeKindInSet(ast.KindHeading, []ast.NodeKind{ast.KindParagraph}))
	})
}
