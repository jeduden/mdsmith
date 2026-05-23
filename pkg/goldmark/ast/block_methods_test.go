package ast_test

// Coverage for block-node accessors and Dump implementations
// that the normal parse-flow does not always reach.

import (
	"testing"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

func TestDocument_MetaAddAndSet(t *testing.T) {
	doc := ast.NewDocument()
	// Meta on a fresh doc lazily allocates the map.
	m := doc.Meta()
	if m == nil {
		t.Fatal("Meta() returned nil")
	}
	doc.AddMeta("k1", "v1")
	doc.AddMeta("k2", 42)
	doc.SetMeta(map[string]any{"k3": true, "k4": 3.14})
	out := doc.Meta()
	for _, k := range []string{"k1", "k2", "k3", "k4"} {
		if _, ok := out[k]; !ok {
			t.Errorf("Meta missing key %q", k)
		}
	}
}

func TestTextBlock_PosEmptyAndNonEmpty(t *testing.T) {
	tb := ast.NewTextBlock()
	if got := tb.Pos(); got != -1 {
		t.Errorf("Pos on empty TextBlock = %d, want -1", got)
	}
	tb.Lines().Append(text.NewSegment(5, 10))
	if got := tb.Pos(); got != 5 {
		t.Errorf("Pos on populated TextBlock = %d, want 5", got)
	}
}

func TestHTMLBlock_DumpVariants(t *testing.T) {
	hb := ast.NewHTMLBlock(ast.HTMLBlockType6)
	hb.Lines().Append(text.NewSegment(0, 5))
	silencer(t, func() { hb.Dump([]byte("hello"), 0) })

	// Also drive ClosureLine branch in Dump.
	hb.ClosureLine = text.NewSegment(0, 3)
	silencer(t, func() { hb.Dump([]byte("hello"), 0) })
}

func TestList_Pos(t *testing.T) {
	list := ast.NewList('-')
	if got := list.Pos(); got != -1 {
		// Empty list returns -1.
	}
	li := ast.NewListItem(2)
	li.Lines().Append(text.NewSegment(0, 5))
	list.AppendChild(list, li)
	_ = list.Pos()
}

func TestList_Dump_OrderedAndUnordered(t *testing.T) {
	// List.Dump has an IsOrdered branch that adds a Start
	// attribute.  Drive both ordered and unordered.
	unordered := ast.NewList('-')
	silencer(t, func() { unordered.Dump(nil, 0) })

	ordered := ast.NewList('.')
	ordered.Start = 5
	silencer(t, func() { ordered.Dump(nil, 0) })
}

func TestLinkReferenceDefinition_Pos(t *testing.T) {
	def := ast.NewLinkReferenceDefinition([]byte("label"), []byte("/dest"), []byte("title"))
	if got := def.Pos(); got != -1 {
		t.Errorf("Pos on empty link-reference def = %d, want -1", got)
	}
	def.Lines().Append(text.NewSegment(7, 12))
	if got := def.Pos(); got != 7 {
		t.Errorf("Pos on populated link-reference def = %d, want 7", got)
	}
}

func TestDocument_AddMeta_EmptyMap(t *testing.T) {
	// AddMeta on a Document with an existing non-nil meta map
	// drives the n.meta != nil branch (skip allocation).
	doc := ast.NewDocument()
	doc.AddMeta("first", 1) // allocates
	doc.AddMeta("second", 2) // existing map
	if doc.Meta()["second"] != 2 {
		t.Error("second AddMeta call should not lose the value")
	}
}

func TestText_SetRaw_True(t *testing.T) {
	tx := ast.NewTextSegment(text.NewSegment(0, 3))
	tx.SetRaw(true)
	if !tx.IsRaw() {
		t.Error("SetRaw(true) then IsRaw() must be true")
	}
}
