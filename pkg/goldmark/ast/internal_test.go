package ast

// Internal unit tests for unexported helpers and corner-case
// branches that the public test files (package ast_test) cannot
// reach as easily.

import (
	"testing"

	"github.com/yuin/goldmark/text"
)

func TestWalkHelper_AllReturnPaths(t *testing.T) {
	// walkHelper has branches for: walker returns error,
	// walker returns WalkStop, walker returns WalkSkipChildren,
	// child returns error, and normal completion.

	// Build a small tree: doc -> paragraph -> text.
	doc := NewDocument()
	p := NewParagraph()
	doc.AppendChild(doc, p)
	p.AppendChild(p, NewTextSegment(text.NewSegment(0, 5)))

	// Walker that returns error on entering.
	_ = Walk(doc, func(n Node, entering bool) (WalkStatus, error) {
		if entering && n.Kind() == KindDocument {
			return WalkStop, errSentinel
		}
		return WalkContinue, nil
	})

	// Walker that returns WalkStop on first node.
	_ = Walk(doc, func(n Node, entering bool) (WalkStatus, error) {
		return WalkStop, nil
	})

	// Walker that returns WalkSkipChildren on paragraph.
	_ = Walk(doc, func(n Node, entering bool) (WalkStatus, error) {
		if entering && n.Kind() == KindParagraph {
			return WalkSkipChildren, nil
		}
		return WalkContinue, nil
	})

	// Walker that returns error on exit.
	_ = Walk(doc, func(n Node, entering bool) (WalkStatus, error) {
		if !entering && n.Kind() == KindText {
			return WalkStop, errSentinel
		}
		return WalkContinue, nil
	})
}

var errSentinel = sentinelErr{}

type sentinelErr struct{}

func (sentinelErr) Error() string { return "sentinel" }

func TestCodeBlock_Text_Direct(t *testing.T) {
	cb := NewCodeBlock()
	cb.Lines().Append(text.NewSegment(0, 5))
	_ = cb.Text([]byte("hello world"))
}

func TestHTMLBlock_Text_Direct(t *testing.T) {
	// HTMLBlock.Text branches: no ClosureLine vs ClosureLine set.
	hb := NewHTMLBlock(HTMLBlockType6)
	hb.Lines().Append(text.NewSegment(0, 5))
	_ = hb.Text([]byte("<div>"))

	// With ClosureLine set.
	src := []byte("<script></script>\n")
	hb2 := NewHTMLBlock(HTMLBlockType1)
	hb2.Lines().Append(text.NewSegment(0, 8))
	hb2.ClosureLine = text.NewSegment(8, 18)
	_ = hb2.Text(src)
}

func TestBaseNode_Text_HeadingWithMixedChildren(t *testing.T) {
	// Heading doesn't override Text, so it dispatches to
	// BaseNode.Text.  Drive both branches: a Text child with
	// SoftLineBreak set, and a String child (no SoftLineBreak
	// method -> type assertion fails branch).
	src := []byte("hello world")
	h := NewHeading(1)
	t1 := NewTextSegment(text.NewSegment(0, 5))
	t1.SetSoftLineBreak(true)
	h.AppendChild(h, t1)

	s := NewString([]byte("ignored"))
	h.AppendChild(h, s)

	_ = h.Text(src)
}

func TestBaseNode_Text_SoftLineBreakChild(t *testing.T) {
	// BaseNode.Text iterates children and inserts '\n' between
	// children whose SoftLineBreak() returns true.  Build a
	// Paragraph with two Text children, the first carrying a
	// soft line break.
	src := []byte("hello world")
	p := NewParagraph()
	t1 := NewTextSegment(text.NewSegment(0, 5))
	t1.SetSoftLineBreak(true)
	t2 := NewTextSegment(text.NewSegment(6, 11))
	p.AppendChild(p, t1)
	p.AppendChild(p, t2)

	// Call Text to drive the soft-line-break branch; exact output
	// shape is not asserted (parent.Text dispatches through
	// children's Text, and Text children with segments return
	// their segment value).
	_ = p.Text(src)
}

func TestReferenceLinkType_String_DefaultArm(t *testing.T) {
	// ReferenceLinkType.String has a default arm for unknown
	// values. Not reachable through normal AST construction.
	if got := ReferenceLinkType(99).String(); got != "Unknown(99)" {
		t.Errorf("ReferenceLinkType(99).String() = %q, want Unknown(99)", got)
	}
}

func TestBaseNode_OwnerDocument_NoDocumentInChain(t *testing.T) {
	// OwnerDocument walks up to a Document parent.  When the
	// chain ends without a Document (e.g., orphan Paragraph), it
	// returns nil.
	p := NewParagraph()
	tx := NewTextSegment(text.NewSegment(0, 5))
	p.AppendChild(p, tx)
	if got := tx.OwnerDocument(); got != nil {
		t.Errorf("OwnerDocument on orphan paragraph chain = %v, want nil", got)
	}
}
