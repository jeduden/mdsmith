package parser_test

// Multi-line HTML block close-condition coverage. The single-line
// HTML block tests already drive Open; this file drives the
// Continue + Close paths by spanning the close pattern across
// multiple lines for each of the seven HTMLBlock types.

import (
	"testing"

	"github.com/yuin/goldmark/ast"
)

func walkHTMLBlocks(src string) []*ast.HTMLBlock {
	root := parseWithDefaults(src)
	var out []*ast.HTMLBlock
	_ = ast.Walk(root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering {
			if hb, ok := n.(*ast.HTMLBlock); ok {
				out = append(out, hb)
			}
		}
		return ast.WalkContinue, nil
	})
	return out
}

func TestHTMLBlock_Type1_MultiLine(t *testing.T) {
	// Type 1 closes on </script>, </pre>, or </style>.
	cases := []string{
		"<script>\nbody\nmore body\n</script>\n",
		"<pre>\nblock\n</pre>\n",
		"<style>\ncss\n</style>\n",
	}
	for _, src := range cases {
		blocks := walkHTMLBlocks(src)
		if len(blocks) == 0 {
			t.Errorf("expected HTMLBlock for %q", src)
		}
	}
}

func TestHTMLBlock_Type2_CommentMultiLine(t *testing.T) {
	src := "<!--\nthis is a multi-line\ncomment\n-->\n"
	blocks := walkHTMLBlocks(src)
	if len(blocks) != 1 {
		t.Errorf("expected one HTMLBlock for multi-line comment, got %d", len(blocks))
	}
}

func TestHTMLBlock_Type3_ProcessingInstructionMultiLine(t *testing.T) {
	src := "<?xml\n version=\"1.0\"\n encoding=\"utf-8\"\n?>\n"
	blocks := walkHTMLBlocks(src)
	if len(blocks) != 1 {
		t.Errorf("expected one HTMLBlock for multi-line PI, got %d", len(blocks))
	}
}

func TestHTMLBlock_Type4_DeclarationMultiLine(t *testing.T) {
	src := "<!DOCTYPE\nhtml\n PUBLIC ...\n>\n"
	blocks := walkHTMLBlocks(src)
	if len(blocks) != 1 {
		t.Errorf("expected one HTMLBlock for multi-line declaration, got %d", len(blocks))
	}
}

func TestHTMLBlock_Type5_CDATAMultiLine(t *testing.T) {
	src := "<![CDATA[\nstuff\nmore stuff\n]]>\n"
	blocks := walkHTMLBlocks(src)
	if len(blocks) != 1 {
		t.Errorf("expected one HTMLBlock for multi-line CDATA, got %d", len(blocks))
	}
}

func TestHTMLBlock_Type6_BlockTagClosesOnBlankLine(t *testing.T) {
	src := "<div>\nbody line 1\nbody line 2\n\ncontinuation paragraph\n"
	blocks := walkHTMLBlocks(src)
	if len(blocks) != 1 {
		t.Errorf("expected one HTMLBlock (type 6 closes on blank line), got %d", len(blocks))
	}
}

func TestHTMLBlock_Type7_ParagraphTagClosesOnBlankLine(t *testing.T) {
	src := "<a href=\"x\">\nbody\nbody\n\nparagraph after\n"
	blocks := walkHTMLBlocks(src)
	if len(blocks) != 1 {
		t.Errorf("expected one HTMLBlock (type 7), got %d", len(blocks))
	}
}

func TestHTMLBlock_EndOfFileBeforeClose(t *testing.T) {
	// Block that never closes — must still appear as an HTMLBlock
	// in the AST (consumed through EOF).
	src := "<script>\nnever closes\n"
	blocks := walkHTMLBlocks(src)
	if len(blocks) != 1 {
		t.Errorf("expected one HTMLBlock when EOF before close, got %d", len(blocks))
	}
}
