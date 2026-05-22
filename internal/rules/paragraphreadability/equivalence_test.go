package paragraphreadability

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/mdtext"
	"github.com/stretchr/testify/require"
	"github.com/yuin/goldmark/ast"
)

// TestCountWordsInNode_EquivalentToCountWordsExtractPlainText pins
// the contract behind plan 196: paragraph-readability's minWords gate
// reads its word count through CountWordsInNode instead of
// materialising the paragraph text via ExtractPlainText. The two
// chains must agree on every fixture paragraph; drift would shift
// which paragraphs pass the gate and emit a downstream diagnostic
// change.
//
// The harness walks every paragraph in good.md, bad.md, and the
// in-package "easy" / "hard" prose helpers — the same prose the
// rule's TestCheck_* set exercises — so the gate covers every text
// shape the MDS023 suite asserts on.
func TestCountWordsInNode_EquivalentToCountWordsExtractPlainText(t *testing.T) {
	fixtureDir := "../MDS023-paragraph-readability"
	sources := [][]byte{
		readFixtureFile(t, filepath.Join(fixtureDir, "good.md")),
		readFixtureFile(t, filepath.Join(fixtureDir, "bad.md")),
		// The in-package prose helpers are independently asserted by
		// TestCheck_*; widening the harness to them catches an
		// equivalence break the fixture pair would miss because the
		// fixture corpus is intentionally tiny.
		[]byte(easyText() + "\n"),
		[]byte(hardText() + "\n"),
	}

	for _, src := range sources {
		f, err := lint.NewFile("equivalence.md", src)
		require.NoError(t, err)

		paragraphs := collectParagraphNodes(f.AST)
		require.NotEmpty(t, paragraphs,
			"expected at least one paragraph in fixture, source: %q",
			previewSource(src))

		for _, p := range paragraphs {
			text := mdtext.ExtractPlainText(p, f.Source)
			wantWords := mdtext.CountWords(text)
			gotWords := mdtext.CountWordsInNode(p, f.Source)
			require.Equalf(t, wantWords, gotWords,
				"CountWordsInNode disagrees with CountWords(ExtractPlainText) "+
					"on paragraph %q (file: %q)", text, previewSource(src))
		}
	}
}

// readFixtureFile reads the fixture verbatim, including any YAML
// front matter (bad.md fixtures encode expected diagnostics in front
// matter). The equivalence harness walks the parsed AST and compares
// per-paragraph counts; front matter parses into the same AST shape
// for both chains, so leaving it in does not skew the comparison.
func readFixtureFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err, "reading fixture %s", path)
	return data
}

// collectParagraphNodes returns every paragraph node in the document
// in source order. Unlike astutil.CollectSectionParagraphs, this walk
// does NOT apply the table-paragraph filter — the harness measures
// CountWordsInNode equivalence per paragraph, so widening the input
// to include the (rare) table-shaped paragraphs strengthens the
// gate without changing what is being tested.
func collectParagraphNodes(root ast.Node) []*ast.Paragraph {
	var out []*ast.Paragraph
	_ = ast.Walk(root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if p, ok := n.(*ast.Paragraph); ok {
			out = append(out, p)
		}
		return ast.WalkContinue, nil
	})
	return out
}

// previewSource truncates source to the first 80 bytes for assertion
// messages; the full fixture is read from disk so the test path
// already names it.
func previewSource(src []byte) string {
	const limit = 80
	if len(src) <= limit {
		return string(src)
	}
	return string(src[:limit]) + "..."
}
