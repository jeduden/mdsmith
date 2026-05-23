package ast_test

// Kitchen-sink coverage for AST node marker/getter/Dump methods.
// Upstream goldmark's own ast_test.go only exercises a slice
// utility, leaving every node type's Type/Kind/IsRaw/Inline/Dump
// methods at 0 %. This file constructs each node concretely and
// drives every interface method on it. The Dump output is
// redirected via os.Pipe so it does not pollute test stdout.

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// captureStdout runs fn with os.Stdout redirected to a buffer and
// returns the captured output. Used because ast.Dump writes
// directly to os.Stdout.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	done := make(chan struct{})
	var buf bytes.Buffer
	go func() {
		_, _ = io.Copy(&buf, r)
		close(done)
	}()
	fn()
	_ = w.Close()
	<-done
	os.Stdout = orig
	return buf.String()
}

func TestBlockNodes_TypeAndKindAndDump(t *testing.T) {
	src := []byte("hi")
	cases := []struct {
		name string
		node ast.Node
		kind ast.NodeKind
	}{
		{"Document", ast.NewDocument(), ast.KindDocument},
		{"TextBlock", ast.NewTextBlock(), ast.KindTextBlock},
		{"Paragraph", ast.NewParagraph(), ast.KindParagraph},
		{"Heading", ast.NewHeading(2), ast.KindHeading},
		{"ThematicBreak", ast.NewThematicBreak(), ast.KindThematicBreak},
		{"CodeBlock", ast.NewCodeBlock(), ast.KindCodeBlock},
		{"FencedCodeBlock", ast.NewFencedCodeBlock(ast.NewTextSegment(text.NewSegment(0, 2))), ast.KindFencedCodeBlock},
		{"Blockquote", ast.NewBlockquote(), ast.KindBlockquote},
		{"List", ast.NewList('-'), ast.KindList},
		{"ListItem", ast.NewListItem(2), ast.KindListItem},
		{"HTMLBlock", ast.NewHTMLBlock(ast.HTMLBlockType1), ast.KindHTMLBlock},
		{"LinkReferenceDefinition", ast.NewLinkReferenceDefinition([]byte("a"), []byte("/"), nil), ast.KindLinkReferenceDefinition},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.node.Type() != ast.TypeBlock && tc.node.Type() != ast.TypeDocument {
				t.Errorf("%s Type() = %v, want TypeBlock or TypeDocument", tc.name, tc.node.Type())
			}
			if tc.node.Kind() != tc.kind {
				t.Errorf("%s Kind() = %v, want %v", tc.name, tc.node.Kind(), tc.kind)
			}
			out := captureStdout(t, func() { tc.node.Dump(src, 0) })
			if out == "" {
				t.Errorf("%s Dump produced no output", tc.name)
			}
		})
	}
}

func TestInlineNodes_TypeAndKindAndDump(t *testing.T) {
	src := []byte("body")
	textInfo := ast.NewTextSegment(text.NewSegment(0, 4))
	cases := []struct {
		name string
		node ast.Node
		kind ast.NodeKind
	}{
		{"Text", ast.NewText(), ast.KindText},
		{"TextSegment", ast.NewTextSegment(text.NewSegment(0, 4)), ast.KindText},
		{"RawTextSegment", ast.NewRawTextSegment(text.NewSegment(0, 4)), ast.KindText},
		{"String", ast.NewString([]byte("x")), ast.KindString},
		{"CodeSpan", ast.NewCodeSpan(), ast.KindCodeSpan},
		{"Emphasis-1", ast.NewEmphasis(1), ast.KindEmphasis},
		{"Emphasis-2", ast.NewEmphasis(2), ast.KindEmphasis},
		{"Link", ast.NewLink(), ast.KindLink},
		{"Image", ast.NewImage(ast.NewLink()), ast.KindImage},
		{"AutoLink", ast.NewAutoLink(ast.AutoLinkURL, textInfo), ast.KindAutoLink},
		{"AutoLink-email", ast.NewAutoLink(ast.AutoLinkEmail, textInfo), ast.KindAutoLink},
		{"RawHTML", ast.NewRawHTML(), ast.KindRawHTML},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.node.Type() != ast.TypeInline {
				t.Errorf("%s Type() = %v, want TypeInline", tc.name, tc.node.Type())
			}
			if tc.node.Kind() != tc.kind {
				t.Errorf("%s Kind() = %v, want %v", tc.name, tc.node.Kind(), tc.kind)
			}
			out := captureStdout(t, func() { tc.node.Dump(src, 0) })
			if out == "" {
				t.Errorf("%s Dump produced no output", tc.name)
			}
		})
	}
}

func TestText_Flags(t *testing.T) {
	tn := ast.NewTextSegment(text.NewSegment(0, 4))
	tn.SetSoftLineBreak(true)
	if !tn.SoftLineBreak() {
		t.Error("SoftLineBreak setter/getter mismatch")
	}
	tn.SetHardLineBreak(true)
	if !tn.HardLineBreak() {
		t.Error("HardLineBreak setter/getter mismatch")
	}
	tn.SetRaw(true)
	if !tn.IsRaw() {
		t.Error("SetRaw(true)/IsRaw() mismatch")
	}
	// Inline() is the marker method — call it to register coverage.
	tn.Inline()
}

func TestText_Merge(t *testing.T) {
	src := []byte("ab cd")
	// Adjacent (Stop=2, Start=2), matching raw flags, no newline at
	// boundary: Merge returns true and extends the receiver.
	a := ast.NewTextSegment(text.NewSegment(0, 2))
	b := ast.NewTextSegment(text.NewSegment(2, 5))
	if !a.Merge(b, src) {
		t.Fatal("Merge should succeed on adjacent same-flag segments")
	}
	if a.Segment.Stop != 5 {
		t.Errorf("Merge did not extend receiver: Stop=%d, want 5", a.Segment.Stop)
	}
	// Non-adjacent (gap between Stop and Start): Merge returns false.
	c := ast.NewTextSegment(text.NewSegment(0, 2))
	d := ast.NewTextSegment(text.NewSegment(3, 5)) // gap
	if c.Merge(d, src) {
		t.Error("Merge must reject non-adjacent segments")
	}
	// Type mismatch: Merge returns false when target isn't a *Text.
	if a.Merge(ast.NewParagraph(), src) {
		t.Error("Merge must reject non-Text nodes")
	}
}

func TestParagraph_LinesAccessors(t *testing.T) {
	p := ast.NewParagraph()
	p.SetBlankPreviousLines(true)
	if !p.HasBlankPreviousLines() {
		t.Error("Blank-previous-lines setter/getter mismatch")
	}

	lines := text.NewSegments()
	lines.Append(text.NewSegment(0, 2))
	p.SetLines(lines)
	if p.Lines().Len() != 1 {
		t.Errorf("Paragraph lines len = %d, want 1", p.Lines().Len())
	}
}

func TestNode_AttributesLifecycle(t *testing.T) {
	p := ast.NewParagraph()
	p.SetAttribute([]byte("class"), []byte("note"))
	p.SetAttribute([]byte("id"), []byte("p-1"))
	if got, ok := p.Attribute([]byte("class")); !ok || string(got.([]byte)) != "note" {
		t.Errorf("Attribute(class) = %v ok=%v", got, ok)
	}
	if attrs := p.Attributes(); len(attrs) != 2 {
		t.Errorf("Attributes() len = %d, want 2", len(attrs))
	}
	p.RemoveAttributes()
	if attrs := p.Attributes(); attrs != nil && len(attrs) != 0 {
		t.Errorf("RemoveAttributes left %d attrs", len(attrs))
	}
}

func TestDocument_MetaAndOwner(t *testing.T) {
	doc := ast.NewDocument()
	doc.SetMeta(map[string]any{"title": "X"})
	if v := doc.Meta()["title"]; v != "X" {
		t.Errorf("Meta()[title] = %v, want X", v)
	}
	doc.AddMeta("author", "alice")
	if v := doc.Meta()["author"]; v != "alice" {
		t.Errorf("Meta()[author] = %v, want alice", v)
	}
	if doc.OwnerDocument() != doc {
		t.Error("OwnerDocument() must return self for Document")
	}
}

func TestRawHTML_LinesAndDump(t *testing.T) {
	r := ast.NewRawHTML()
	seg := text.NewSegment(0, 5)
	r.Segments.Append(seg)
	out := captureStdout(t, func() { r.Dump([]byte("<x />"), 0) })
	if !strings.Contains(out, "RawHTML") {
		t.Errorf("RawHTML Dump output missing kind name: %q", out)
	}
}

func TestBaseInline_DefaultsOnNonOverridingTypes(t *testing.T) {
	// CodeSpan, Emphasis, Link, Image, AutoLink, RawHTML embed
	// BaseInline but do not override IsRaw or the
	// block-only methods. By contract the block-only methods PANIC
	// on inline nodes ("can not call with inline nodes."); this test
	// drives both branches so the BaseInline defaults are covered.
	cs := ast.NewCodeSpan()
	em := ast.NewEmphasis(2)
	lk := ast.NewLink()
	im := ast.NewImage(ast.NewLink())
	al := ast.NewAutoLink(ast.AutoLinkURL, ast.NewTextSegment(text.NewSegment(0, 1)))
	rh := ast.NewRawHTML()
	inlines := []ast.Node{cs, em, lk, im, al, rh}

	for _, n := range inlines {
		// IsRaw is the only BaseInline method that returns rather
		// than panics; it defaults to false.
		if n.IsRaw() {
			t.Errorf("%s.IsRaw() default must be false", n.Kind())
		}
		// Each block-only method must panic on the inline node.
		assertPanics(t, n.Kind().String()+".HasBlankPreviousLines",
			func() { _ = n.HasBlankPreviousLines() })
		assertPanics(t, n.Kind().String()+".SetBlankPreviousLines",
			func() { n.SetBlankPreviousLines(true) })
		assertPanics(t, n.Kind().String()+".Lines",
			func() { _ = n.Lines() })
		assertPanics(t, n.Kind().String()+".SetLines",
			func() { n.SetLines(text.NewSegments()) })
	}
}

func assertPanics(t *testing.T, label string, fn func()) {
	t.Helper()
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("%s: expected panic, did not get one", label)
		}
	}()
	fn()
}

func TestStringNode_TextAndRaw(t *testing.T) {
	s := ast.NewString([]byte("abc"))
	if got := s.Text(nil); string(got) != "abc" {
		t.Errorf("String.Text() = %q, want abc", got)
	}
	s.SetRaw(true)
	if !s.IsRaw() {
		t.Error("String.SetRaw/IsRaw mismatch")
	}
}

func TestReferenceLinkType_String(t *testing.T) {
	if ast.ReferenceLinkFull.String() != "Full" {
		t.Errorf("ReferenceLinkFull.String() = %q, want Full", ast.ReferenceLinkFull.String())
	}
	if ast.ReferenceLinkCollapsed.String() != "Collapsed" {
		t.Errorf("ReferenceLinkCollapsed.String() = %q, want Collapsed", ast.ReferenceLinkCollapsed.String())
	}
	if ast.ReferenceLinkShortcut.String() != "Shortcut" {
		t.Errorf("ReferenceLinkShortcut.String() = %q, want Shortcut", ast.ReferenceLinkShortcut.String())
	}
}

func TestNode_RemoveChildrenWipesList(t *testing.T) {
	parent := ast.NewParagraph()
	parent.AppendChild(parent, ast.NewText())
	parent.AppendChild(parent, ast.NewText())
	if parent.ChildCount() != 2 {
		t.Fatalf("setup: expected 2 children, got %d", parent.ChildCount())
	}
	parent.RemoveChildren(parent)
	if parent.ChildCount() != 0 {
		t.Errorf("RemoveChildren did not clear, got %d", parent.ChildCount())
	}
}
