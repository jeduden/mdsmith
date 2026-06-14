package integration

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/assert"
)

// TestFlatClassifierEquivalence_Fixtures is the always-on (no-corpus) half
// of the flat-Layer-0 equivalence gate (plan 2606142147, acceptance
// criterion 2): for every checked-in fixture — every rule's good/ and bad/
// examples (the line-length fixtures among them) and the shared testdata
// tree — the flat line classifier's code-block line set must be
// byte-identical to the AST-derived lint.CollectCodeBlockLines.
//
// It compares on front-matter-stripped content, the same basis the engine
// uses, so it mirrors what the parse-skip path actually serves. The pinned
// neutral-corpus half runs under MDSMITH_FLATL0_CORPUS in
// internal/lint/lineclass_equiv_test.go.
func TestFlatClassifierEquivalence_Fixtures(t *testing.T) {
	roots := []string{
		filepath.Join("..", "..", "internal", "rules"),
		filepath.Join("..", "..", "testdata"),
	}
	var files, diverged int
	for _, root := range roots {
		err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || filepath.Ext(p) != ".md" {
				return nil
			}
			files++
			src, readErr := os.ReadFile(p)
			if readErr != nil {
				return nil
			}
			f, parseErr := lint.NewFileFromSource(p, src, true)
			if parseErr != nil {
				return nil
			}
			astSet := lint.CollectCodeBlockLines(f)
			flatSet := lint.ClassifyLines(f.Lines).CodeBlockLines()
			if !assert.Equalf(t, sortedSet(astSet), sortedSet(flatSet),
				"flat code-block set diverges from AST for %s", p) {
				diverged++
			}
			return nil
		})
		assert.NoError(t, err)
	}
	t.Logf("flat-classifier fixture equivalence: %d files, %d diverged", files, diverged)
	assert.Positive(t, files, "expected to walk at least one fixture")
}

// sortedSet returns the ascending 1-based keys of a line set.
func sortedSet(m map[int]struct{}) []int {
	out := make([]int, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Ints(out)
	return out
}
