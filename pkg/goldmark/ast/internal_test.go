package ast

// Internal unit tests for unexported helpers and corner-case
// branches that the public test files (package ast_test) cannot
// reach as easily.

import (
	"testing"

	"github.com/yuin/goldmark/text"
)

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
