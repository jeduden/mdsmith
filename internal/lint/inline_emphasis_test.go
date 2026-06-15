package lint

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jeduden/mdsmith/pkg/goldmark/ast"
)

// astLoneEmphasisLines parses src normally and returns the 1-based source
// lines of every paragraph whose sole inline child is an Emphasis node —
// the exact condition MDS018 checks on the AST path. It is the byte-
// identical target the bounded inline detector must reproduce.
func astLoneEmphasisLines(t *testing.T, src string) []int {
	t.Helper()
	f, err := NewFile("doc.md", []byte(src))
	require.NoError(t, err)
	var lines []int
	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		para, ok := n.(*ast.Paragraph)
		if !ok {
			return ast.WalkContinue, nil
		}
		first := para.FirstChild()
		if first == nil || first.NextSibling() != nil {
			return ast.WalkContinue, nil
		}
		if _, isEmph := first.(*ast.Emphasis); !isEmph {
			return ast.WalkContinue, nil
		}
		lines = append(lines, paragraphFirstLine(para, f))
		return ast.WalkContinue, nil
	})
	return lines
}

// paragraphFirstLine returns the 1-based source line of a paragraph's
// first text segment, mirroring astutil.ParagraphLine closely enough for
// the equivalence assertion (the detector emits the same line).
func paragraphFirstLine(para ast.Node, f *File) int {
	lines := para.Lines()
	if lines != nil && lines.Len() > 0 {
		return f.LineOfOffset(lines.At(0).Start)
	}
	return 1
}

func indexLoneEmphasisLines(src string) []int {
	f := NewFileLines("doc.md", []byte(src))
	paras := WholeParagraphEmphasis(f)
	if len(paras) == 0 {
		return nil
	}
	out := make([]int, len(paras))
	for i, p := range paras {
		out[i] = p.Line
	}
	return out
}

// TestWholeParagraphEmphasis_Equivalence pins the bounded detector byte-
// identical to goldmark's lone-emphasis-child result across the shapes the
// corpus exercises: simple and strong emphasis, underscore variants,
// emphasis with trailing/leading spaces, multi-child paragraphs (not
// flagged), partial emphasis (not flagged), and non-emphasis paragraphs.
func TestWholeParagraphEmphasis_Equivalence(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"star-emphasis", "*just emphasis*\n"},
		{"strong-emphasis", "**just strong**\n"},
		{"underscore-emphasis", "_just emphasis_\n"},
		{"underscore-strong", "__just strong__\n"},
		{"emphasis-with-trailing-space", "*emphasis*  \n"},
		{"emphasis-then-text", "*emphasis* and text\n"},
		{"text-then-emphasis", "text and *emphasis*\n"},
		{"two-emphasis", "*one* *two*\n"},
		{"plain-paragraph", "just plain text\n"},
		{"heading-not-paragraph", "# heading\n"},
		{"partial-emphasis", "*not closed\n"},
		{"nested-strong-in-emphasis", "*outer **inner** outer*\n"},
		{"emphasis-with-code", "*a `code` b*\n"},
		{"empty-emphasis-markers", "**\n"},
		{"multi-paragraph", "*first*\n\nnormal\n\n*second*\n"},
		{"leading-spaces", "   *emphasis*\n"},
		{"intraword-underscore", "snake_case_word\n"},
		{"emphasis-over-two-lines", "*emphasis\nspanning*\n"},
		{"thematic-break-not-emphasis", "***\n"},
		{"star-list-not-paragraph", "* list item\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			want := astLoneEmphasisLines(t, tc.src)
			got := indexLoneEmphasisLines(tc.src)
			assert.Equal(t, want, got, "lone-emphasis lines diverge from AST")
		})
	}
}

// TestWholeParagraphEmphasis_NilSource keeps a struct-literal File (no AST,
// no source) returning nil, matching the empty-document case.
func TestWholeParagraphEmphasis_NilSource(t *testing.T) {
	f := &File{}
	assert.Nil(t, WholeParagraphEmphasis(f))
}
