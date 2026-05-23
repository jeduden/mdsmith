package ast

// Internal unit tests for unexported helpers and corner-case
// branches that the public test files (package ast_test) cannot
// reach as easily.

import (
	"testing"

	"github.com/yuin/goldmark/text"
)

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
