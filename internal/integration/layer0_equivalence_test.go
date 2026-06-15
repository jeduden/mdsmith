package integration

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jeduden/mdsmith/internal/lint"
)

// TestLayer0Equivalence_Fixtures is the non-negotiable Layer 0 gate: for
// every Markdown file in the repository corpus (rule fixtures, docs, plans,
// shared testdata) the block-level projections served from the Layer 0
// scan must be byte-identical to the ones the goldmark AST walk produces.
//
// Both paths consume the same post-front-matter body so line numbers share
// one coordinate system: the AST File parses it the normal way, the Layer 0
// File is built with a nil AST (lint.NewFileLines) so CollectCodeBlockLines
// and CollectPIBlockLines route through the scan. Any divergence here means
// the parse-skip gate would change observable output, so the test is always
// on in CI and runs by default — it walks the in-repo corpus, no network or
// external fixture needed.
func TestLayer0Equivalence_Fixtures(t *testing.T) {
	root := repoRoot(t)
	files := collectMarkdownCorpus(t, root)
	require.NotEmpty(t, files, "expected to discover Markdown corpus files")

	for _, path := range files {
		path := path
		rel, _ := filepath.Rel(root, path)
		t.Run(rel, func(t *testing.T) {
			source, err := os.ReadFile(path)
			require.NoError(t, err)

			// Mirror the production pooled path: strip front matter so the
			// AST and Layer 0 files share the same body coordinate system.
			_, body := lint.StripFrontMatter(source)

			astFile, err := lint.NewFile(path, body)
			require.NoError(t, err)
			astCode := lint.CollectCodeBlockLines(astFile)
			astPI := lint.CollectPIBlockLines(astFile)

			l0File := lint.NewFileLines(path, body)
			l0Code := lint.CollectCodeBlockLines(l0File)
			l0PI := lint.CollectPIBlockLines(l0File)

			assert.Equal(t, sortedKeys(astCode), sortedKeys(l0Code),
				"code-block line set differs between AST and Layer 0")
			assert.Equal(t, sortedKeys(astPI), sortedKeys(l0PI),
				"PI-block line set differs between AST and Layer 0")
		})
	}
}

// sortedKeys returns the keys of a 1-based line set in ascending order so
// two sets compare by value regardless of map iteration order.
func sortedKeys(set map[int]struct{}) []int {
	out := make([]int, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Ints(out)
	return out
}

// repoRoot walks up from the test's working directory to the module root
// (the directory holding go.mod).
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	require.NoError(t, err)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		require.NotEqual(t, parent, dir, "go.mod not found walking up from test dir")
		dir = parent
	}
}

// collectMarkdownCorpus gathers every .md file under root, skipping the
// VCS, vendored-goldmark, and build-output trees that are not part of the
// linted corpus.
func collectMarkdownCorpus(t *testing.T, root string) []string {
	t.Helper()
	var files []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "node_modules", "dist", "build":
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, ".md") {
			files = append(files, path)
		}
		return nil
	})
	require.NoError(t, err)
	sort.Strings(files)
	return files
}
