package noemptyalttext

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"

	"github.com/jeduden/mdsmith/pkg/goldmark/ast"
	"github.com/jeduden/mdsmith/pkg/goldmark/text"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheck_EmptyAlt_Violation(t *testing.T) {
	src := []byte("# Title\n\n![](image.png)\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 1, "expected 1 diagnostic, got %d: %+v", len(diags), diags)
	if diags[0].RuleID != "MDS032" {
		t.Errorf("expected rule ID MDS032, got %s", diags[0].RuleID)
	}
	if diags[0].Severity != lint.Warning {
		t.Errorf("expected warning severity, got %s", diags[0].Severity)
	}
	if diags[0].Line != 3 {
		t.Errorf("expected line 3, got %d", diags[0].Line)
	}
}

func TestCheck_WhitespaceAlt_Violation(t *testing.T) {
	src := []byte("# Title\n\n![  ](image.png)\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 1, "expected 1 diagnostic, got %d: %+v", len(diags), diags)
}

func TestCheck_WithAlt_NoViolation(t *testing.T) {
	src := []byte("# Title\n\n![A sunset over the ocean](image.png)\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 0, "expected 0 diagnostics, got %d: %+v", len(diags), diags)
}

func TestCheck_MultipleImages_MixedViolations(t *testing.T) {
	src := []byte("# Title\n\n![](a.png)\n\n![Good alt](b.png)\n\n![](c.png)\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 2, "expected 2 diagnostics, got %d: %+v", len(diags), diags)
}

func TestCheck_ImageInListItem_Violation(t *testing.T) {
	src := []byte("# Title\n\n- ![](image.png)\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 1, "expected 1 diagnostic, got %d: %+v", len(diags), diags)
	if diags[0].Line != 3 {
		t.Errorf("expected line 3, got %d", diags[0].Line)
	}
}

func TestCheck_ImageInListItem_WithAlt_NoViolation(t *testing.T) {
	src := []byte("# Title\n\n- ![Screenshot](image.png)\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 0, "expected 0 diagnostics, got %d: %+v", len(diags), diags)
}

func TestCheck_ImageInsideEmphasis_Violation(t *testing.T) {
	src := []byte("# Title\n\n*![](image.png)*\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 1, "expected 1 diagnostic, got %d: %+v", len(diags), diags)
	if diags[0].Line != 3 {
		t.Errorf("expected line 3, got %d", diags[0].Line)
	}
}

func TestCheck_ImageWithMarkupAlt_NoViolation(t *testing.T) {
	src := []byte("# Title\n\n![**bold description**](image.png)\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 0, "expected 0 diagnostics, got %d: %+v", len(diags), diags)
}

func TestCheck_NoImages_NoViolation(t *testing.T) {
	src := []byte("# Title\n\nJust text.\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 0, "expected 0 diagnostics, got %d", len(diags))
}

func TestCheck_EmptyFile(t *testing.T) {
	src := []byte("")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 0, "expected 0 diagnostics, got %d", len(diags))
}

func TestID(t *testing.T) {
	r := &Rule{}
	if r.ID() != "MDS032" {
		t.Errorf("expected MDS032, got %s", r.ID())
	}
}

func TestName(t *testing.T) {
	r := &Rule{}
	if r.Name() != "no-empty-alt-text" {
		t.Errorf("expected no-empty-alt-text, got %s", r.Name())
	}
}

func TestCategory(t *testing.T) {
	r := &Rule{}
	if r.Category() == "" {
		t.Error("expected non-empty category")
	}
}

// --- firstTextLine ---

// TestFirstTextLine pins the recursion + direct-text branches: a
// node whose first child is a Text node returns immediately; a
// node whose Text is nested under a non-Text child (Link / Image
// alt) recurses to find it; an empty subtree returns 0. The Check
// loop drives only the recursion branch via real Markdown.
func TestFirstTextLine(t *testing.T) {
	f, err := lint.NewFile("t.md", []byte("# X\n\n  Y\n"))
	require.NoError(t, err)

	t.Run("direct child Text", func(t *testing.T) {
		p := ast.NewParagraph()
		txt := ast.NewText()
		txt.Segment = text.NewSegment(0, 1)
		p.AppendChild(p, txt)
		assert.Equal(t, 1, firstTextLine(p, f, 0))
	})
	t.Run("nested Text via Link", func(t *testing.T) {
		p := ast.NewParagraph()
		link := ast.NewLink()
		txt := ast.NewText()
		txt.Segment = text.NewSegment(7, 8) // points into line 3
		link.AppendChild(link, txt)
		p.AppendChild(p, link)
		assert.Equal(t, 3, firstTextLine(p, f, 0))
	})
	t.Run("no descendant text returns zero", func(t *testing.T) {
		p := ast.NewParagraph()
		assert.Equal(t, 0, firstTextLine(p, f, 0))
	})
}

// TestInlineCapable_NoEmptyAltText covers the InlineCapable method.
func TestInlineCapable_NoEmptyAltText(t *testing.T) {
	r := &Rule{}
	assert.True(t, r.InlineCapable())
}

// TestImageLine_OrphanNoText covers the `return 1` fallback (line 134) in
// imageLine: an Image with no text children (firstTextLine returns 0) and no
// ancestors with line information falls back to line 1.
func TestImageLine_OrphanNoText(t *testing.T) {
	// Build a minimal File so LineOfOffset is callable.
	f, err := lint.NewFile("t.md", []byte("# X\n"))
	require.NoError(t, err)

	// NewImage needs a Link; build an empty Link and an empty Image from it.
	link := ast.NewLink()
	img := ast.NewImage(link)
	// img has no children and no parent — both firstTextLine and the ancestor
	// walk return nothing, so imageLine falls through to return 1.
	got := imageLine(img, f, 0)
	assert.Equal(t, 1, got, "orphan image with no text children must fall back to line 1")
}

// TestCheck_NilASTEquivalence pins the parse-skipped path (f.AST nil,
// served from the Layer 1 per-block inline parse) byte-identical to the
// AST path across image shapes and block contexts.
func TestCheck_NilASTEquivalence(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"empty-alt", "![](img.png)\n"},
		{"good-alt", "![a cat](img.png)\n"},
		{"whitespace-alt", "![   ](img.png)\n"},
		{"empty-in-prose", "see ![](img.png) here\n"},
		{"empty-in-list", "- item ![](img.png)\n- ok ![alt](x.png)\n"},
		{"empty-in-heading", "# title ![](img.png)\n"},
		{"empty-in-quote", "> quote ![](img.png)\n"},
		{"two-empty", "![](a.png) and ![](b.png)\n"},
		{"empty-multiline-para", "text\n![](img.png)\n"},
		{"linked-image-empty", "[![](img.png)](dest)\n"},
		{"ref-image-empty", "![][ref]\n\n[ref]: http://x/y.png\n"},
		{"wrapped-image-list", "- ![\n  ](img.png)\n"},
		{"no-image", "just text\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			astFile, err := lint.NewFile("test.md", []byte(tc.src))
			require.NoError(t, err)
			lineFile := lint.NewFileLines("test.md", []byte(tc.src))
			r := &Rule{}
			assert.Equal(t, r.Check(astFile), r.Check(lineFile),
				"nil-AST diagnostics diverge from AST path")
		})
	}
}
