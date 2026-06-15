package integration

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/jeduden/mdsmith/internal/rules/nobareurls"
	"github.com/jeduden/mdsmith/internal/rules/noemphasisasheading"
)

// inlineEquivRules are the inline rules re-backed on the Layer 1 inline
// index / per-block parse: each must produce byte-identical diagnostics on
// the parse-skipped path (f.AST nil) and the AST path for every file the
// production parse-skip gate admits.
func inlineEquivRules() []rule.Rule {
	return []rule.Rule{
		&nobareurls.Rule{},
		&noemphasisasheading.Rule{},
	}
}

// TestInlineRuleEquivalence_Corpus runs each re-backed inline rule over the
// repository corpus on both the AST path and the parse-skipped path, for
// exactly the files the production gate would skip (no fenced/indented code
// block and no `<?` directive marker), and asserts the diagnostics are
// byte-identical. This is the soundness contract that lets the rules resolve
// to Layer 0: the gate admits only these files, and on them the two paths
// agree.
func TestInlineRuleEquivalence_Corpus(t *testing.T) {
	root := repoRoot(t)
	files := collectMarkdownCorpus(t, root)
	require.NotEmpty(t, files)

	var checked int
	for _, path := range files {
		source, err := os.ReadFile(path)
		require.NoError(t, err)
		_, body := lint.StripFrontMatter(source)

		// Mirror the engine's parse-skip eligibility.
		if lint.SourceMayHaveCodeBlock(body) || bytes.Contains(body, []byte("<?")) {
			continue
		}
		checked++

		rel, _ := filepath.Rel(root, path)
		t.Run(rel, func(t *testing.T) {
			for _, r := range inlineEquivRules() {
				astFile, err := lint.NewFile(path, body)
				require.NoError(t, err)
				lineFile := lint.NewFileLines(path, body)
				assert.Equal(t, r.Check(astFile), r.Check(lineFile),
					"%s diverges between AST and Layer 1 paths", r.ID())
			}
		})
	}
	require.NotZero(t, checked, "expected at least one parse-skip-eligible corpus file")
}
