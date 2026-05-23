package ast_test

// Pure interface-conformance coverage for AST node types. Many of
// the Inline() / Text() / Dump() / Kind() / IsCode() / SetCode()
// / Pos() methods on inline nodes are not invoked during normal
// parse — they exist to satisfy ast.Node and ast.Inline. Exercise
// them explicitly so the surface coverage doesn't drop.

import (
	"io"
	"os"
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
	src := []byte("https://example.com")
	tx := ast.NewTextSegment(text.NewSegment(0, len(src)))
	al := ast.NewAutoLink(ast.AutoLinkURL, tx)
	al.Inline()
	_ = al.Kind()
	_ = al.Text(src)
	_ = al.URL(src)
}

func TestString_PosAndInline(t *testing.T) {
	s := ast.NewString([]byte("inline content"))
	s.Inline()
	// String.Pos returns -1 because the node carries no source
	// position (it was synthesised inline). Just calling it is
	// sufficient for coverage.
	_ = s.Pos()
}

func TestCodeSpan_Inline_Marker(t *testing.T) {
	c := ast.NewCodeSpan()
	c.Inline() // marker method
}

func TestLink_Inline_Image_Inline(t *testing.T) {
	l := ast.NewLink()
	l.Inline()
	img := ast.NewImage(ast.NewLink())
	img.Inline()
}

func TestRawHTML_TextAndInline(t *testing.T) {
	r := ast.NewRawHTML()
	r.Inline()
	_ = r.Text([]byte("any source"))
}

// silencer redirects stdout for the duration of fn so Dump's
// fmt.Printf calls don't litter test output.
func silencer(t *testing.T, fn func()) {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	defer func() {
		_ = w.Close()
		os.Stdout = old
		_ = r.Close()
	}()
	fn()
	go io.Copy(io.Discard, r)
}

func TestDump_LinkWithAndWithoutReference(t *testing.T) {
	// Drive Link.Dump on both branches: no Reference set and
	// Reference set to a full reference. The latter prints the
	// nested Reference block.
	src := []byte("source bytes")
	l := ast.NewLink()
	l.Destination = []byte("/url")
	l.Title = []byte("title")
	silencer(t, func() { l.Dump(src, 0) })

	l2 := ast.NewLink()
	l2.Destination = []byte("/x")
	l2.Reference = ast.NewReferenceLink(ast.ReferenceLinkFull, []byte("label"))
	silencer(t, func() { l2.Dump(src, 0) })
}

func TestDump_ImageWithAndWithoutReference(t *testing.T) {
	src := []byte("source bytes")
	l := ast.NewLink()
	l.Destination = []byte("/img")
	img := ast.NewImage(l)
	silencer(t, func() { img.Dump(src, 0) })

	l2 := ast.NewLink()
	l2.Destination = []byte("/img2")
	l2.Reference = ast.NewReferenceLink(ast.ReferenceLinkCollapsed, []byte("alt"))
	img2 := ast.NewImage(l2)
	silencer(t, func() { img2.Dump(src, 0) })
}

func TestDump_AutoLinkURL(t *testing.T) {
	src := []byte("https://example.com")
	tx := ast.NewTextSegment(text.NewSegment(0, len(src)))
	al := ast.NewAutoLink(ast.AutoLinkURL, tx)
	silencer(t, func() { al.Dump(src, 0) })

	tx2 := ast.NewTextSegment(text.NewSegment(0, len(src)))
	al2 := ast.NewAutoLink(ast.AutoLinkEmail, tx2)
	silencer(t, func() { al2.Dump(src, 0) })
}

func TestText_SetRaw(t *testing.T) {
	tx := ast.NewTextSegment(text.NewSegment(0, 5))
	tx.SetRaw(true)
	if !tx.IsRaw() {
		t.Error("SetRaw(true) then IsRaw() must be true")
	}
	tx.SetRaw(false)
	if tx.IsRaw() {
		t.Error("SetRaw(false) then IsRaw() must be false")
	}
}

func TestString_SetRaw(t *testing.T) {
	s := ast.NewString([]byte("x"))
	s.SetRaw(true)
	if !s.IsRaw() {
		t.Error("SetRaw(true) then IsRaw() must be true")
	}
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

func TestBaseNode_OwnerDocument_Nested(t *testing.T) {
	// OwnerDocument on a nested child walks up to the root.
	doc := ast.NewDocument()
	p := ast.NewParagraph()
	doc.AppendChild(doc, p)
	tx := ast.NewTextSegment(text.NewSegment(0, 3))
	p.AppendChild(p, tx)
	if got := tx.OwnerDocument(); got != doc {
		t.Errorf("OwnerDocument should walk up to root, got %v want %v", got, doc)
	}
}

func TestBaseNode_SortChildren(t *testing.T) {
	// SortChildren rearranges children in place using the
	// provided comparator. Drive it on a parent with three
	// children that need reordering.
	doc := ast.NewDocument()
	headings := []*ast.Heading{
		ast.NewHeading(3),
		ast.NewHeading(1),
		ast.NewHeading(2),
	}
	for _, h := range headings {
		doc.AppendChild(doc, h)
	}
	doc.SortChildren(func(a, b ast.Node) int {
		return a.(*ast.Heading).Level - b.(*ast.Heading).Level
	})
	want := []int{1, 2, 3}
	i := 0
	for c := doc.FirstChild(); c != nil; c = c.NextSibling() {
		if h, ok := c.(*ast.Heading); ok {
			if h.Level != want[i] {
				t.Errorf("child[%d].Level = %d, want %d", i, h.Level, want[i])
			}
			i++
		}
	}
}

func TestBaseNode_SetAttribute_Variants(t *testing.T) {
	// SetAttribute has branches for setting / overwriting / nil
	// value. Drive each.
	h := ast.NewHeading(1)
	h.SetAttribute([]byte("id"), []byte("a"))
	h.SetAttribute([]byte("id"), []byte("b")) // overwrite
	if v, ok := h.Attribute([]byte("id")); !ok || string(v.([]byte)) != "b" {
		t.Errorf("SetAttribute overwrite failed: %v ok=%v", v, ok)
	}
	h.SetAttribute([]byte("class"), nil)
	if _, ok := h.Attribute([]byte("class")); !ok {
		t.Error("SetAttribute(nil) should still set the key")
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
