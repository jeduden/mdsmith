package lint

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func rangesEqual(a, b []Range) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestProbe_CodeSpanDivergence(t *testing.T) {
	cases := map[string]string{
		"nested-list-deep":         "- a\n  - b\n    - `code` here\n",
		"list-in-blockquote":       "> - item `code` here\n> - two `more`\n",
		"codespan-near-refdef":     "use `[ref]` here\n\n[ref]: /x\n",
		"codespan-adj-html":        "<div>x</div>\n`real`\n",
		"codespan-after-pi":        "<?foo\nbar\n?>\n\n`real` span\n",
		"many-spans-many-runs":     "`a` x\n\n`b` y\n\n`c` z\n\n`d` w\n",
		"refdef-in-same-run":       "`code` and [ref]: /x\n",
		"span-in-deeply-nested-bq": "> > > `deep`\n",
		"span-then-loose-list":     "text `a`\n\n- one\n\n- two `b`\n",
		"two-runs-noncontiguous":   "first `a`\n\n```\nblock\n```\n\nsecond `b`\n",
		"crlf-file":                "a `code` b\r\nmore `x` y\r\n",
		"no-trailing-newline":      "a `code` b",
		"span-spanning-runs-blank": "`open\n\nclose`\n",
		"setext-with-span":         "`heading code`\n===\n",
		"span-in-ordered-cont":     "1. `a\n   b`\n",
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			astFile, err := NewFile("doc.md", []byte(src))
			require.NoError(t, err)
			lineFile := NewFileLines("doc.md", []byte(src))
			ac := astFile.CodeSpanContentRanges()
			lc := lineFile.CodeSpanContentRanges()
			al := astFile.CodeSpanLiteralRanges()
			ll := lineFile.CodeSpanLiteralRanges()
			if !rangesEqual(ac, lc) {
				t.Errorf("CONTENT diverge\n ast=%v\n l1 =%v", ac, lc)
			}
			if !rangesEqual(al, ll) {
				t.Errorf("LITERAL diverge\n ast=%v\n l1 =%v", al, ll)
			}
		})
	}
}

func TestProbe_CodeSpanFrontMatter(t *testing.T) {
	cases := map[string]string{
		"fm-then-span":      "---\ntitle: x\n---\n\nuse `code` here\n",
		"only-front-matter": "---\ntitle: x\n---\n",
		"fm-crlf":           "---\r\ntitle: x\r\n---\r\n\r\nuse `code` here\r\n",
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			astFile, err := NewFileFromSource("doc.md", []byte(src), true)
			require.NoError(t, err)
			lineFile := NewFileLinesFromSource("doc.md", []byte(src), true)
			ac := astFile.CodeSpanContentRanges()
			lc := lineFile.CodeSpanContentRanges()
			al := astFile.CodeSpanLiteralRanges()
			ll := lineFile.CodeSpanLiteralRanges()
			if !rangesEqual(ac, lc) {
				t.Errorf("CONTENT diverge\n ast=%v\n l1 =%v", ac, lc)
			}
			if !rangesEqual(al, ll) {
				t.Errorf("LITERAL diverge\n ast=%v\n l1 =%v", al, ll)
			}
		})
	}
}

func TestProbe_EmphasisFrontMatterAndList(t *testing.T) {
	cases := map[string]string{
		"fm-and-list-lone-emph": "---\ntitle: x\n---\n\n- *a*\n\n- *b*\n",
		"fm-only-lone-emph":     "---\ntitle: x\n---\n\n*lone*\n",
		"fm-list-mixed":         "---\nt: x\n---\n\n*top*\n\n- *x*\n\n- *y*\n",
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			astFile, err := NewFileFromSource("doc.md", []byte(src), true)
			require.NoError(t, err)
			lineFile := NewFileLinesFromSource("doc.md", []byte(src), true)

			// Reconstruct AST lone-emphasis lines.
			astParas := wholeParagraphEmphasisFullParse(astFile)
			l1Paras := WholeParagraphEmphasis(lineFile)
			require.Equal(t, len(astParas), len(l1Paras), "count diverges")
			for i := range astParas {
				assert.Equal(t, astParas[i].Line, l1Paras[i].Line, "line %d diverges", i)
				assert.Equal(t, astParas[i].TextSegments, l1Paras[i].TextSegments, "segs %d diverge", i)
			}
		})
	}
}

func TestProbe_MemoSameSlice(t *testing.T) {
	f := NewFileLines("doc.md", []byte("use `a` and `b`\n"))
	a := f.CodeSpanContentRanges()
	b := f.CodeSpanContentRanges()
	require.NotEmpty(t, a)
	assert.Equal(t, &a[0], &b[0], "codespan memo returns same backing array")

	g := NewFileLines("doc.md", []byte("*lone*\n"))
	p1 := WholeParagraphEmphasis(g)
	p2 := WholeParagraphEmphasis(g)
	require.NotEmpty(t, p1)
	assert.Equal(t, &p1[0], &p2[0], "emphasis memo returns same backing array")
}
