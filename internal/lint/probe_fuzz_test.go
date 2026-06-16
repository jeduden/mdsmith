package lint

import (
	"math/rand"
	"testing"

	"github.com/jeduden/mdsmith/pkg/goldmark/ast"
	"github.com/stretchr/testify/require"
)

// astLoneEmphLines walks the real document AST for paragraphs whose sole
// inline child is an Emphasis node, returning their 1-based first lines.
func astLoneEmphLines(f *File) []int {
	var out []int
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
		ln := 1
		if lines := para.Lines(); lines != nil && lines.Len() > 0 {
			ln = f.LineOfOffset(lines.At(0).Start)
		}
		out = append(out, ln)
		return ast.WalkContinue, nil
	})
	return out
}

// TestProbeFuzz_CodeSpan generates random markdown-ish documents from a
// token alphabet biased toward code spans, block boundaries, HTML, PI,
// lists, blockquotes, and reference defs, then asserts the nil-AST code-span
// projection matches the AST walk. Files with a fenced/indented code block
// or a `<?` directive are skipped (the production gate forces a parse for
// those), matching the equivalence contract.
func TestProbeFuzz_CodeSpan(t *testing.T) {
	tokens := []string{
		"`code`", "``a`b``", "text", "`open\n", "close`", "# h", "===",
		"---", "> q", "- li", "1. li", "<div>", "</div>", "<!-- c -->",
		"[r]: /x", "[r]", "*em*", "  cont", "***", "\n", "x `y", "z` w",
		"![](i.png)", "http://e.com", "`", "``", "\t",
	}
	rng := rand.New(rand.NewSource(20260616))
	const iters = 4000
	for it := 0; it < iters; it++ {
		nTok := 1 + rng.Intn(8)
		var b []byte
		for k := 0; k < nTok; k++ {
			b = append(b, tokens[rng.Intn(len(tokens))]...)
			if rng.Intn(3) == 0 {
				b = append(b, '\n')
			} else {
				b = append(b, ' ')
			}
		}
		b = append(b, '\n')

		// Mirror the production parse-skip gate.
		if SourceMayHaveCodeBlock(b) {
			continue
		}
		hasDirective := false
		for i := 0; i+1 < len(b); i++ {
			if b[i] == '<' && b[i+1] == '?' {
				hasDirective = true
				break
			}
		}
		if hasDirective {
			continue
		}

		astFile, err := NewFile("d.md", b)
		require.NoError(t, err)
		lineFile := NewFileLines("d.md", b)
		ac := astFile.CodeSpanContentRanges()
		lc := lineFile.CodeSpanContentRanges()
		al := astFile.CodeSpanLiteralRanges()
		ll := lineFile.CodeSpanLiteralRanges()
		if !rangesEqual(ac, lc) {
			t.Fatalf("CONTENT diverge iter=%d\n src=%q\n ast=%v\n l1 =%v", it, b, ac, lc)
		}
		if !rangesEqual(al, ll) {
			t.Fatalf("LITERAL diverge iter=%d\n src=%q\n ast=%v\n l1 =%v", it, b, al, ll)
		}
	}
}

// TestProbeFuzz_Emphasis fuzzes the lone-emphasis projection vs a full AST
// parse, including list/looseness/front-matter-free shapes.
func TestProbeFuzz_Emphasis(t *testing.T) {
	tokens := []string{
		"*em*", "**str*", "_em_", "text", "# h", "- *x*", "1. *y*",
		"> *q*", "*a\n", "b*", "\n", "***", "* li", "  *cont*", "`c`",
	}
	rng := rand.New(rand.NewSource(99))
	for it := 0; it < 4000; it++ {
		nTok := 1 + rng.Intn(7)
		var b []byte
		for k := 0; k < nTok; k++ {
			b = append(b, tokens[rng.Intn(len(tokens))]...)
			if rng.Intn(3) == 0 {
				b = append(b, '\n')
			} else {
				b = append(b, ' ')
			}
		}
		b = append(b, '\n')
		if SourceMayHaveCodeBlock(b) {
			continue
		}
		astFile, err := NewFile("d.md", b)
		require.NoError(t, err)
		lineFile := NewFileLines("d.md", b)

		astLines := astLoneEmphLines(astFile)
		l1Paras := WholeParagraphEmphasis(lineFile)
		if len(astLines) != len(l1Paras) {
			l1Lines := make([]int, len(l1Paras))
			for i, p := range l1Paras {
				l1Lines[i] = p.Line
			}
			t.Fatalf("emph count diverge iter=%d\n src=%q\n ast=%v\n l1 =%v",
				it, b, astLines, l1Lines)
		}
		for i := range astLines {
			if astLines[i] != l1Paras[i].Line {
				t.Fatalf("emph line diverge iter=%d i=%d\n src=%q\n ast=%v l1=%v",
					it, i, b, astLines[i], l1Paras[i].Line)
			}
		}
	}
}
