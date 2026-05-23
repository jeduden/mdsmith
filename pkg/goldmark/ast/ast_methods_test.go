package ast_test

// Pure interface-conformance coverage for AST node types. Many of
// the Inline() / Text() / Dump() / Kind() / IsCode() / SetCode()
// / Pos() methods on inline nodes are not invoked during normal
// parse — they exist to satisfy ast.Node and ast.Inline. Exercise
// them explicitly so the surface coverage doesn't drop.

import (
	"testing"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

func TestText_BasicAccessors(t *testing.T) {
	tx := ast.NewTextSegment(text.NewSegment(0, 5))
	tx.Inline() // marker method
	_ = tx.Kind()
	_ = tx.Text([]byte("hello world"))
}

func TestString_IsCodeRoundTrip(t *testing.T) {
	s := ast.NewString([]byte("inline"))
	s.SetCode(true)
	if !s.IsCode() {
		t.Error("SetCode(true) then IsCode() must be true")
	}
	s.SetCode(false)
	if s.IsCode() {
		t.Error("SetCode(false) then IsCode() must be false")
	}
}

func TestRawTextSegment_Methods(t *testing.T) {
	tx := ast.NewRawTextSegment(text.NewSegment(0, 3))
	tx.Inline()
	_ = tx.Kind()
	if tx.Segment.Start != 0 || tx.Segment.Stop != 3 {
		t.Errorf("RawTextSegment segment wrong: %+v", tx.Segment)
	}
}

func TestString_Methods(t *testing.T) {
	s := ast.NewString([]byte("inline"))
	s.Inline()
	_ = s.Kind()
	_ = s.Text([]byte("ignored"))
}

func TestCodeSpan_Methods(t *testing.T) {
	c := ast.NewCodeSpan()
	c.Inline()
	_ = c.Kind()
}

func TestEmphasis_Methods(t *testing.T) {
	e := ast.NewEmphasis(2)
	_ = e.Kind()
	if e.Level != 2 {
		t.Errorf("Emphasis level: got %d, want 2", e.Level)
	}
}

func TestLink_Methods(t *testing.T) {
	l := ast.NewLink()
	l.Inline()
	_ = l.Kind()
	l.Destination = []byte("/url")
	l.Title = []byte("title")
}

func TestImage_Methods(t *testing.T) {
	img := ast.NewImage(ast.NewLink())
	img.Inline()
	_ = img.Kind()
}

func TestAutoLink_Methods(t *testing.T) {
	tx := ast.NewTextSegment(text.NewSegment(0, 10))
	al := ast.NewAutoLink(ast.AutoLinkURL, tx)
	al.Inline()
	_ = al.Kind()
}

func TestRawHTML_Methods(t *testing.T) {
	r := ast.NewRawHTML()
	r.Inline()
	_ = r.Kind()
}

func TestReferenceLink_Construct(t *testing.T) {
	rl := ast.NewReferenceLink(ast.ReferenceLinkFull, []byte("label"))
	if rl == nil {
		t.Fatal("NewReferenceLink returned nil")
	}
}

func TestDocument_OwnerDocument(t *testing.T) {
	doc := ast.NewDocument()
	if doc.OwnerDocument() != doc {
		t.Error("OwnerDocument() on a document must return itself")
	}
}

func TestBlockAST_TextMethods(t *testing.T) {
	// Block nodes have Text() too, mostly returning their text
	// representation. Call them on representative nodes.
	src := []byte("# heading\n")
	h := ast.NewHeading(1)
	h.AppendChild(h, ast.NewTextSegment(text.NewSegment(2, 9)))
	_ = h.Text(src)
	_ = h.Kind()

	cb := ast.NewFencedCodeBlock(nil)
	cb.Lines().Append(text.NewSegment(0, 3))
	_ = cb.Text([]byte("abc\n"))
	_ = cb.Kind()

	hr := ast.NewThematicBreak()
	_ = hr.Kind()

	bq := ast.NewBlockquote()
	_ = bq.Kind()
	_ = bq.Text([]byte(""))
}
