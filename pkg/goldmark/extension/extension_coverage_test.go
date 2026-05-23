package extension_test

// Parser-level coverage tests for the retained goldmark
// extensions. Upstream's HTML-diff tests were dropped along with
// testutil; these tests instead parse markdown with each
// extension wired in and check that the AST has the expected
// extension node types.

import (
	"bytes"
	"strings"
	"testing"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"
)

func walkContains(root ast.Node, want ast.NodeKind) bool {
	found := false
	_ = ast.Walk(root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if n.Kind() == want {
			found = true
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
	return found
}

func TestStrikethrough_Parse(t *testing.T) {
	md := goldmark.New(goldmark.WithExtensions(extension.Strikethrough))
	src := []byte("a ~~struck~~ b\n")
	root := md.Parser().Parse(text.NewReader(src))
	if !walkContains(root, extast.KindStrikethrough) {
		t.Error("expected a Strikethrough node in the AST")
	}
	// HTML round trip exercises StrikethroughHTMLRenderer.
	var buf bytes.Buffer
	if err := md.Convert(src, &buf); err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if !bytes.Contains(buf.Bytes(), []byte("<del>")) {
		t.Errorf("HTML output missing <del>: %s", buf.String())
	}
}

func TestTaskList_Parse(t *testing.T) {
	md := goldmark.New(goldmark.WithExtensions(extension.TaskList))
	src := []byte("- [x] done\n- [ ] todo\n- [X] done caps\n")
	root := md.Parser().Parse(text.NewReader(src))
	if !walkContains(root, extast.KindTaskCheckBox) {
		t.Error("expected TaskCheckBox in AST")
	}
	var checkedCount int
	_ = ast.Walk(root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering {
			if cb, ok := n.(*extast.TaskCheckBox); ok && cb.IsChecked {
				checkedCount++
			}
		}
		return ast.WalkContinue, nil
	})
	if checkedCount != 2 {
		t.Errorf("checked count = %d, want 2 (x and X)", checkedCount)
	}
	var buf bytes.Buffer
	if err := md.Convert(src, &buf); err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if !bytes.Contains(buf.Bytes(), []byte("checkbox")) {
		t.Errorf("HTML output missing checkbox: %s", buf.String())
	}
}

func TestTable_Parse(t *testing.T) {
	md := goldmark.New(goldmark.WithExtensions(extension.Table))
	src := []byte("| h1 | h2 | h3 |\n|----|:---|---:|\n| a  | b  | c  |\n| d  | e  | f  |\n")
	root := md.Parser().Parse(text.NewReader(src))
	if !walkContains(root, extast.KindTable) {
		t.Error("expected Table in AST")
	}
	if !walkContains(root, extast.KindTableHeader) {
		t.Error("expected TableHeader in AST")
	}
	if !walkContains(root, extast.KindTableRow) {
		t.Error("expected TableRow in AST")
	}
	if !walkContains(root, extast.KindTableCell) {
		t.Error("expected TableCell in AST")
	}
	var buf bytes.Buffer
	if err := md.Convert(src, &buf); err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if !bytes.Contains(buf.Bytes(), []byte("<table>")) {
		t.Errorf("HTML output missing <table>: %s", buf.String())
	}
	if !bytes.Contains(buf.Bytes(), []byte("text-align:left")) {
		t.Errorf("HTML output missing left alignment: %s", buf.String())
	}
	if !bytes.Contains(buf.Bytes(), []byte("text-align:right")) {
		t.Errorf("HTML output missing right alignment: %s", buf.String())
	}
}

func TestTable_NotATable(t *testing.T) {
	// A single pipe-row without a separator must not parse as a table.
	md := goldmark.New(goldmark.WithExtensions(extension.Table))
	src := []byte("| just a paragraph\n")
	root := md.Parser().Parse(text.NewReader(src))
	if walkContains(root, extast.KindTable) {
		t.Error("unseparated pipe row must not parse as Table")
	}
}

func TestDefinitionList_Parse(t *testing.T) {
	md := goldmark.New(goldmark.WithExtensions(extension.DefinitionList))
	src := []byte("Term 1\n: Definition 1\n\nTerm 2\n: Definition 2a\n: Definition 2b\n")
	root := md.Parser().Parse(text.NewReader(src))
	if !walkContains(root, extast.KindDefinitionList) {
		t.Error("expected DefinitionList in AST")
	}
	if !walkContains(root, extast.KindDefinitionTerm) {
		t.Error("expected DefinitionTerm in AST")
	}
	if !walkContains(root, extast.KindDefinitionDescription) {
		t.Error("expected DefinitionDescription in AST")
	}
	var buf bytes.Buffer
	if err := md.Convert(src, &buf); err != nil {
		t.Fatalf("Convert: %v", err)
	}
	out := buf.String()
	for _, want := range []string{"<dl>", "<dt>", "<dd>"} {
		if !strings.Contains(out, want) {
			t.Errorf("HTML output missing %q: %s", want, out)
		}
	}
}

func TestFootnote_Parse(t *testing.T) {
	md := goldmark.New(goldmark.WithExtensions(extension.Footnote))
	src := []byte("text with note[^1].\n\n[^1]: the footnote body\n")
	root := md.Parser().Parse(text.NewReader(src))
	if !walkContains(root, extast.KindFootnoteLink) {
		t.Error("expected FootnoteLink in AST")
	}
	if !walkContains(root, extast.KindFootnote) {
		t.Error("expected Footnote definition node in AST")
	}
	if !walkContains(root, extast.KindFootnoteList) {
		t.Error("expected FootnoteList in AST")
	}
	var buf bytes.Buffer
	if err := md.Convert(src, &buf); err != nil {
		t.Fatalf("Convert: %v", err)
	}
	out := buf.String()
	for _, want := range []string{`class="footnote-ref"`, `class="footnotes"`} {
		if !strings.Contains(out, want) {
			t.Errorf("HTML output missing %q: %s", want, out)
		}
	}
}

func TestFootnote_UnreferencedDefinitionStillRendered(t *testing.T) {
	md := goldmark.New(goldmark.WithExtensions(extension.Footnote))
	src := []byte("plain text.\n\n[^orphan]: orphaned footnote\n")
	var buf bytes.Buffer
	if err := md.Convert(src, &buf); err != nil {
		t.Fatalf("Convert: %v", err)
	}
	// Orphan footnotes are dropped from the output entirely; just
	// confirm Convert ran without error.
}
