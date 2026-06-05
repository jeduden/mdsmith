package rule

import (
	"strconv"
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yuin/goldmark/ast"
)

// headingDepthVisitor is a stateful per-node visitor: it carries
// prevLevel across Heading nodes and emits a diagnostic whenever a
// heading jumps more than one level. It declares only KindHeading, so
// the engine and WalkVisitor must filter the stream to headings.
type headingDepthVisitor struct {
	prevLevel int
	kindCalls map[ast.NodeKind]int
}

func (v *headingDepthVisitor) Kinds() []ast.NodeKind {
	return []ast.NodeKind{ast.KindHeading}
}

func (v *headingDepthVisitor) VisitNode(n ast.Node, entering bool, _ *lint.File) []lint.Diagnostic {
	if v.kindCalls == nil {
		v.kindCalls = map[ast.NodeKind]int{}
	}
	v.kindCalls[n.Kind()]++
	if !entering {
		return nil
	}
	h, ok := n.(*ast.Heading)
	if !ok {
		return nil
	}
	var diags []lint.Diagnostic
	if v.prevLevel != 0 && h.Level > v.prevLevel+1 {
		diags = []lint.Diagnostic{{
			RuleID:  "MDSV01",
			Message: "jump to level " + strconv.Itoa(h.Level),
		}}
	}
	v.prevLevel = h.Level
	return diags
}

// headingDepthRule is a NodeVisitorRule whose per-file worker carries
// state. NewNodeVisitor returns a fresh visitor each walk so no state
// leaks across files or goroutines.
type headingDepthRule struct{}

func (headingDepthRule) ID() string       { return "MDSV01" }
func (headingDepthRule) Name() string     { return "heading-depth-stub" }
func (headingDepthRule) Category() string { return "test" }
func (r headingDepthRule) Check(f *lint.File) []lint.Diagnostic {
	return WalkVisitor(r, f)
}
func (headingDepthRule) NewNodeVisitor(_ *lint.File) NodeVisitor {
	return &headingDepthVisitor{}
}

var _ NodeVisitorRule = headingDepthRule{}

// TestWalkVisitor_CarriesStateAcrossNodes pins that a stateful visitor
// sees headings in document order and keeps prevLevel between calls, so
// a level jump is detected. This is the capability the stateless
// NodeChecker could not express.
func TestWalkVisitor_CarriesStateAcrossNodes(t *testing.T) {
	// h1, then a jump straight to h3 (skips h2) — one diagnostic.
	f, err := lint.NewFile("t.md", []byte("# A\n\ntext\n\n### C\n"))
	require.NoError(t, err)

	diags := WalkVisitor(headingDepthRule{}, f)
	require.Len(t, diags, 1, "the h1->h3 jump must be flagged")
	assert.Equal(t, "jump to level 3", diags[0].Message)
}

// TestWalkVisitor_OnlyDeclaredKinds pins that VisitNode is called only
// for the node kinds the visitor declares — never for paragraphs, text,
// or the document root. The rule's own ast.Walk + type-switch saw every
// node; the engine instead pre-filters by Kinds(), so the visitor must
// observe only its declared kinds.
func TestWalkVisitor_OnlyDeclaredKinds(t *testing.T) {
	f, err := lint.NewFile("t.md", []byte("# A\n\ntext\n\n## B\n\n- item\n"))
	require.NoError(t, err)

	v := &headingDepthVisitor{}
	r := stubVisitorRule{v: v}
	_ = WalkVisitor(r, f)

	require.NotEmpty(t, v.kindCalls)
	for k := range v.kindCalls {
		assert.Equal(t, ast.KindHeading, k,
			"visitor must only be shown its declared kind, saw %s", k)
	}
	// Two headings, each shown entering and leaving.
	assert.Equal(t, 4, v.kindCalls[ast.KindHeading])
}

// stubVisitorRule wraps a pre-built visitor so a test can inspect the
// same instance the walk drives.
type stubVisitorRule struct{ v *headingDepthVisitor }

func (stubVisitorRule) ID() string                                { return "MDSV01" }
func (stubVisitorRule) Name() string                              { return "heading-depth-stub" }
func (stubVisitorRule) Category() string                          { return "test" }
func (r stubVisitorRule) Check(f *lint.File) []lint.Diagnostic    { return WalkVisitor(r, f) }
func (r stubVisitorRule) NewNodeVisitor(_ *lint.File) NodeVisitor { return r.v }

// TestWalkVisitor_NilFileAndNilAST pins the defensive nil guard so
// unit-test &lint.File{} literals do not panic, mirroring WalkNodes.
func TestWalkVisitor_NilFileAndNilAST(t *testing.T) {
	assert.Nil(t, WalkVisitor(headingDepthRule{}, nil))
	assert.Nil(t, WalkVisitor(headingDepthRule{}, &lint.File{Path: "t.md"}))
}

// TestWalkVisitor_NilVisitorSkips pins that a rule returning a nil
// visitor (nothing to do for this file) contributes no diagnostics and
// does not panic.
func TestWalkVisitor_NilVisitorSkips(t *testing.T) {
	f, err := lint.NewFile("t.md", []byte("# A\n\n## B\n"))
	require.NoError(t, err)
	assert.Nil(t, WalkVisitor(nilVisitorRule{}, f))
}

type nilVisitorRule struct{}

func (nilVisitorRule) ID() string                              { return "MDSV02" }
func (nilVisitorRule) Name() string                            { return "nil-visitor-stub" }
func (nilVisitorRule) Category() string                        { return "test" }
func (r nilVisitorRule) Check(f *lint.File) []lint.Diagnostic  { return WalkVisitor(r, f) }
func (nilVisitorRule) NewNodeVisitor(_ *lint.File) NodeVisitor { return nil }
