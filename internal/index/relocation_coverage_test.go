package index

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

// These tests cover the locate.go / index.go branches that the
// pure git-mv into internal/index surfaced as "new" patch lines.
// Each drives a real input through the function under test (no
// mocks), per the test pyramid.

func TestAbsToWorkspace_RelComputationError(t *testing.T) {
	t.Parallel()
	// A relative root with an absolute target makes filepath.Rel
	// fail ("knowing the working directory would be necessary"),
	// so the helper falls back to normalising the absolute path.
	got := absToWorkspace("relative/base", "/abs/doc.md")
	assert.Equal(t, NormalizePath("/abs/doc.md"), got)
}

func TestLinkContainsOffset_NoOpeningBracket(t *testing.T) {
	t.Parallel()
	// Text segment starts at offset 0, so source[:startOff] is
	// empty and LastIndexByte returns -1 → open falls back to
	// startOff.
	src := []byte("hello world]\n")
	l := ast.NewLink()
	l.AppendChild(l, ast.NewTextSegment(text.NewSegment(0, 5)))
	assert.True(t, linkContainsOffset(src, l, 2))
}

func TestLinkContainsOffset_CloseFallbackToTextEnd(t *testing.T) {
	t.Parallel()
	// Shortcut reference whose `]` sits past a newline: linkClose
	// Offset returns -1, so close falls back to the text end.
	src := []byte("hello\nworld]")
	l := ast.NewLink()
	l.Reference = &ast.ReferenceLink{Type: ast.ReferenceLinkShortcut, Value: []byte("x")}
	l.AppendChild(l, ast.NewTextSegment(text.NewSegment(0, 5)))
	assert.True(t, linkContainsOffset(src, l, 3))
}

func TestLinkCloseOffset_FullRefNoTextCloser(t *testing.T) {
	t.Parallel()
	// Full reference but the text-closing `]` is past a newline
	// from `after`: the first scanForByte returns -1.
	src := []byte("text\n][label]")
	l := ast.NewLink()
	l.Reference = &ast.ReferenceLink{Type: ast.ReferenceLinkFull, Value: []byte("label")}
	assert.Equal(t, -1, linkCloseOffset(src, l, 0))
}

func TestLinkCloseOffset_InlineEOFWithoutClose(t *testing.T) {
	t.Parallel()
	// Inline link whose `(dest` runs to EOF with no `)` and no
	// newline: the paren loop exits and returns -1.
	src := []byte("[t](abc")
	l := ast.NewLink()
	assert.Equal(t, -1, linkCloseOffset(src, l, 2))
}

func TestScanForByte_NoTargetNoNewlineEOF(t *testing.T) {
	t.Parallel()
	// Neither the target byte nor a newline appears before EOF.
	assert.Equal(t, -1, scanForByte([]byte("abcd"), 0, ']'))
}

func TestLinkToLocate_UnparsableDestination(t *testing.T) {
	t.Parallel()
	// A schemed URL is not a workspace link target, so ParseTarget
	// rejects it and linkToLocate yields TokenNone.
	l := ast.NewLink()
	l.Destination = []byte("http://example.com")
	res := linkToLocate("a.md", l, []byte("x"))
	assert.Equal(t, TokenNone, res.Tag)
}

func TestPiToLocate_LineBeyondLines(t *testing.T) {
	t.Parallel()
	src := []byte("<?include\nfile: a.md\n?>\n")
	root := lint.NewParser().Parse(text.NewReader(src), parser.WithContext(parser.NewContext()))
	var pi *lint.ProcessingInstruction
	_ = ast.Walk(root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if p, ok := n.(*lint.ProcessingInstruction); ok {
			pi = p
		}
		return ast.WalkContinue, nil
	})
	require.NotNil(t, pi, "expected an include PI in the parse")
	lines := bytes.Split(src, []byte("\n"))
	res := piToLocate(pi, src, lines, len(lines)+10, 1)
	assert.Equal(t, TokenDirectiveArg, res.Tag)
	assert.Equal(t, "include", res.DirectiveName)
	assert.Empty(t, res.DirectiveArg)
}

func TestLocateInFrontMatter_LineBelowOne(t *testing.T) {
	t.Parallel()
	assert.Equal(t, TokenNone, locateInFrontMatter([]byte("a: b\n"), 0, 1).Tag)
}

func TestLocateInFrontMatter_LineBeyondLines(t *testing.T) {
	t.Parallel()
	assert.Equal(t, TokenNone, locateInFrontMatter([]byte("a: b\n"), 99, 1).Tag)
}

func TestFrontMatterParentKey_SkipsBlankLine(t *testing.T) {
	t.Parallel()
	lines := [][]byte{[]byte("kinds:"), []byte("   "), []byte("  - plan")}
	assert.Equal(t, "kinds", frontMatterParentKey(lines, 2))
}
