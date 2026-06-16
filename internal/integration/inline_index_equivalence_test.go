package integration

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jeduden/mdsmith/internal/lint"
)

// TestInlineIndexEquivalence_CodeSpans is the Layer 1 counterpart to
// TestLayer0Equivalence_Fixtures: for every parse-skip-eligible Markdown
// file in the repository corpus, the code-span content and literal ranges
// served on the nil-AST path (from the shared run-grouped inline parse,
// InlineBlocks) must be byte-identical to the ones the goldmark AST walk
// produces.
//
// It restricts the comparison to the files the production parse-skip gate
// would actually skip — those with no fenced/indented code block
// (lint.SourceMayHaveCodeBlock) and no `<?` directive marker — so the test
// scope matches the inputs the gate admits. That is the soundness contract
// that lets MDS047 and MDS054 resolve to Layer 0.
func TestInlineIndexEquivalence_CodeSpans(t *testing.T) {
	root := repoRoot(t)
	files := collectMarkdownCorpus(t, root)
	require.NotEmpty(t, files)

	var checked int
	for _, path := range files {
		source, err := os.ReadFile(path)
		require.NoError(t, err)
		_, body := lint.StripFrontMatter(source)

		// Mirror the engine's parse-skip eligibility (runner.layer0SkipEligible).
		if lint.SourceMayHaveCodeBlock(body) || bytes.Contains(body, []byte("<?")) {
			continue
		}
		checked++

		rel, _ := filepath.Rel(root, path)
		t.Run(rel, func(t *testing.T) {
			astFile, err := lint.NewFile(path, body)
			require.NoError(t, err)
			l0File := lint.NewFileLines(path, body)

			assert.Equal(t, astFile.CodeSpanContentRanges(), l0File.CodeSpanContentRanges(),
				"code-span content ranges differ between AST and inline index")
			assert.Equal(t, astFile.CodeSpanLiteralRanges(), l0File.CodeSpanLiteralRanges(),
				"code-span literal ranges differ between AST and inline index")
		})
	}
	require.NotZero(t, checked, "expected at least one parse-skip-eligible corpus file")
}
