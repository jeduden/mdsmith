package lint

import (
	"testing"

	"github.com/yuin/goldmark/ast"
)

// findPINodes returns all ProcessingInstruction nodes in the AST,
// searching the full tree recursively.
func findPINodes(root ast.Node) []*ProcessingInstruction {
	var nodes []*ProcessingInstruction
	_ = ast.Walk(root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if pi, ok := n.(*ProcessingInstruction); ok {
			nodes = append(nodes, pi)
		}
		return ast.WalkContinue, nil
	})
	return nodes
}

func TestPI_BasicSingleLine(t *testing.T) {
	src := "<?foo?>\n"
	f, err := NewFile("test.md", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	pis := findPINodes(f.AST)
	if len(pis) != 1 {
		t.Fatalf("expected 1 PI, got %d", len(pis))
	}
	if pis[0].Name != "foo" {
		t.Errorf("expected Name %q, got %q", "foo", pis[0].Name)
	}
	if !pis[0].HasClosure() {
		t.Error("expected HasClosure() == true for single-line PI")
	}
}

func TestPI_MultiLine(t *testing.T) {
	src := "<?foo\nbar\n?>\n"
	f, err := NewFile("test.md", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	pis := findPINodes(f.AST)
	if len(pis) != 1 {
		t.Fatalf("expected 1 PI, got %d", len(pis))
	}
	if pis[0].Name != "foo" {
		t.Errorf("expected Name %q, got %q", "foo", pis[0].Name)
	}
	if !pis[0].HasClosure() {
		t.Error("expected HasClosure() == true")
	}
}

func TestPI_MultiLineEmptyBody(t *testing.T) {
	src := "<?foo\n?>\n"
	f, err := NewFile("test.md", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	pis := findPINodes(f.AST)
	if len(pis) != 1 {
		t.Fatalf("expected 1 PI, got %d", len(pis))
	}
	if !pis[0].HasClosure() {
		t.Error("expected HasClosure() == true")
	}
}

func TestPI_SlashName(t *testing.T) {
	src := "<?/include?>\n"
	f, err := NewFile("test.md", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	pis := findPINodes(f.AST)
	if len(pis) != 1 {
		t.Fatalf("expected 1 PI, got %d", len(pis))
	}
	if pis[0].Name != "/include" {
		t.Errorf("expected Name %q, got %q", "/include", pis[0].Name)
	}
}

func TestPI_HTMLCommentStillHTMLBlock(t *testing.T) {
	src := "<!-- comment -->\n"
	f, err := NewFile("test.md", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	pis := findPINodes(f.AST)
	if len(pis) != 0 {
		t.Errorf("expected 0 PI nodes for HTML comment, got %d", len(pis))
	}
	// Verify it's an HTMLBlock.
	found := false
	for n := f.AST.FirstChild(); n != nil; n = n.NextSibling() {
		if _, ok := n.(*ast.HTMLBlock); ok {
			found = true
		}
	}
	if !found {
		t.Error("expected HTML comment to be parsed as HTMLBlock")
	}
}

func TestPI_DivStillHTMLBlock(t *testing.T) {
	src := "<div>\nhello\n</div>\n"
	f, err := NewFile("test.md", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	pis := findPINodes(f.AST)
	if len(pis) != 0 {
		t.Errorf("expected 0 PI nodes for div, got %d", len(pis))
	}
}

func TestPI_InsideFencedCodeBlock(t *testing.T) {
	src := "```\n<?foo?>\n```\n"
	f, err := NewFile("test.md", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	pis := findPINodes(f.AST)
	if len(pis) != 0 {
		t.Errorf("expected 0 PI nodes inside code block, got %d", len(pis))
	}
}

func TestPI_FourSpaceIndent(t *testing.T) {
	src := "    <?foo?>\n"
	f, err := NewFile("test.md", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	pis := findPINodes(f.AST)
	if len(pis) != 0 {
		t.Errorf("expected 0 PI nodes for 4-space indent, got %d", len(pis))
	}
}

func TestPI_OneToThreeSpaceIndent(t *testing.T) {
	for _, spaces := range []string{" ", "  ", "   "} {
		src := spaces + "<?foo?>\n"
		f, err := NewFile("test.md", []byte(src))
		if err != nil {
			t.Fatal(err)
		}
		pis := findPINodes(f.AST)
		if len(pis) != 1 {
			t.Errorf("indent %q: expected 1 PI, got %d", spaces, len(pis))
		}
	}
}

func TestPI_Unterminated(t *testing.T) {
	src := "<?foo\nbar"
	f, err := NewFile("test.md", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	pis := findPINodes(f.AST)
	if len(pis) != 1 {
		t.Fatalf("expected 1 PI for unterminated, got %d", len(pis))
	}
	if pis[0].HasClosure() {
		t.Error("expected HasClosure() == false for unterminated PI")
	}
}

func TestPI_MultiplePIs(t *testing.T) {
	src := "<?foo?>\n<?bar?>\n"
	f, err := NewFile("test.md", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	pis := findPINodes(f.AST)
	if len(pis) != 2 {
		t.Fatalf("expected 2 PIs, got %d", len(pis))
	}
	if pis[0].Name != "foo" {
		t.Errorf("first PI name: expected %q, got %q", "foo", pis[0].Name)
	}
	if pis[1].Name != "bar" {
		t.Errorf("second PI name: expected %q, got %q", "bar", pis[1].Name)
	}
}

func TestPI_InterruptsParagraph(t *testing.T) {
	src := "some text\n<?foo?>\n"
	f, err := NewFile("test.md", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	pis := findPINodes(f.AST)
	if len(pis) != 1 {
		t.Fatalf("expected 1 PI after paragraph, got %d", len(pis))
	}
}

func TestPI_WhitespaceOnlyBody(t *testing.T) {
	src := "<?foo\n   \n?>\n"
	f, err := NewFile("test.md", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	pis := findPINodes(f.AST)
	if len(pis) != 1 {
		t.Fatalf("expected 1 PI, got %d", len(pis))
	}
	if !pis[0].HasClosure() {
		t.Error("expected HasClosure() == true")
	}
}

func TestPI_InsideBlockquote(t *testing.T) {
	src := "> <?foo?>\n"
	f, err := NewFile("test.md", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	pis := findPINodes(f.AST)
	if len(pis) != 0 {
		t.Errorf("expected 0 PI nodes inside blockquote, got %d", len(pis))
	}
}

func TestPI_EmptyName(t *testing.T) {
	tests := []string{
		"<??>",
		"<? ?>",
	}
	for _, src := range tests {
		f, err := NewFile("test.md", []byte(src+"\n"))
		if err != nil {
			t.Fatal(err)
		}
		pis := findPINodes(f.AST)
		if len(pis) != 0 {
			t.Errorf("input %q: expected 0 PI nodes for empty name, got %d", src, len(pis))
		}
	}
}

func TestPI_ConsecutiveWithoutBlankLine(t *testing.T) {
	src := "<?foo?>\n<?bar?>\n"
	f, err := NewFile("test.md", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	pis := findPINodes(f.AST)
	if len(pis) != 2 {
		t.Fatalf("expected 2 PI nodes, got %d", len(pis))
	}
}

func TestPI_SingleLineClosesInOpen(t *testing.T) {
	src := "<?foo?>\n"
	f, err := NewFile("test.md", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	pis := findPINodes(f.AST)
	if len(pis) != 1 {
		t.Fatalf("expected 1 PI, got %d", len(pis))
	}
	if !pis[0].HasClosure() {
		t.Error("expected HasClosure() == true for single-line PI")
	}
}

func TestPI_SingleLineWithTrailingContent(t *testing.T) {
	src := "<?foo?> trailing\n"
	f, err := NewFile("test.md", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	pis := findPINodes(f.AST)
	if len(pis) != 1 {
		t.Fatalf("expected 1 PI, got %d", len(pis))
	}
	if !pis[0].HasClosure() {
		t.Error("expected HasClosure() == true for PI with trailing content")
	}
	if pis[0].Name != "foo" {
		t.Errorf("expected name %q, got %q", "foo", pis[0].Name)
	}
}
