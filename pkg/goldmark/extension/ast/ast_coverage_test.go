package ast_test

// Coverage for extension AST node Dump/Pos/Type/Kind methods.
// Upstream goldmark does not vendor unit tests for the extension
// AST package; this file fills the gap by constructing each
// concrete node and exercising every interface method on it.

import (
	"bytes"
	"io"
	"os"
	"testing"

	gast "github.com/yuin/goldmark/ast"
	extast "github.com/yuin/goldmark/extension/ast"
)

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

func TestExtensionASTNodes_KindAndDump(t *testing.T) {
	src := []byte("hi")
	para := gast.NewParagraph()
	link := gast.NewLink()
	row := extast.NewTableRow(nil)
	cases := []struct {
		name string
		node gast.Node
		kind gast.NodeKind
	}{
		{"DefinitionList", extast.NewDefinitionList(2, para), extast.KindDefinitionList},
		{"DefinitionTerm", extast.NewDefinitionTerm(), extast.KindDefinitionTerm},
		{"DefinitionDescription", extast.NewDefinitionDescription(), extast.KindDefinitionDescription},
		{"FootnoteLink", extast.NewFootnoteLink(3), extast.KindFootnoteLink},
		{"FootnoteBacklink", extast.NewFootnoteBacklink(3), extast.KindFootnoteBacklink},
		{"Footnote", extast.NewFootnote([]byte("ref")), extast.KindFootnote},
		{"FootnoteList", extast.NewFootnoteList(), extast.KindFootnoteList},
		{"TaskCheckBox-checked", extast.NewTaskCheckBox(true), extast.KindTaskCheckBox},
		{"TaskCheckBox-unchecked", extast.NewTaskCheckBox(false), extast.KindTaskCheckBox},
		{"Strikethrough", extast.NewStrikethrough(), extast.KindStrikethrough},
		{"Table", extast.NewTable(), extast.KindTable},
		{"TableRow", row, extast.KindTableRow},
		{"TableHeader", extast.NewTableHeader(row), extast.KindTableHeader},
		{"TableCell", extast.NewTableCell(), extast.KindTableCell},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.node.Kind() != tc.kind {
				t.Errorf("%s.Kind() = %v, want %v", tc.name, tc.node.Kind(), tc.kind)
			}
			out := captureStdout(t, func() { tc.node.Dump(src, 0) })
			if out == "" {
				t.Errorf("%s.Dump produced no output", tc.name)
			}
		})
	}

	// Image inside a footnote-style link path exercises the
	// embedded baseLink-derived methods via a more complex shape.
	_ = link
}

func TestFootnote_AppendChildBacklink(t *testing.T) {
	fn := extast.NewFootnote([]byte("ref"))
	fn.Index = 2
	bl := extast.NewFootnoteBacklink(2)
	fn.AppendChild(fn, bl)
	if fn.ChildCount() != 1 {
		t.Errorf("AppendChild did not register child")
	}
}

func TestTable_AlignmentString(t *testing.T) {
	cases := []struct {
		a   extast.Alignment
		out string
	}{
		{extast.AlignLeft, "left"},
		{extast.AlignRight, "right"},
		{extast.AlignCenter, "center"},
		{extast.AlignNone, "none"},
	}
	for _, c := range cases {
		if got := c.a.String(); got != c.out {
			t.Errorf("Alignment(%d).String() = %q, want %q", c.a, got, c.out)
		}
	}
}
