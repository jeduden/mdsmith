package lint

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
)

// diffSets returns the keys present in got but not want (extra) and in
// want but not got (missing), each sorted ascending.
func diffSets(got, want map[int]struct{}) (extra, missing []int) {
	for k := range got {
		if _, ok := want[k]; !ok {
			extra = append(extra, k)
		}
	}
	for k := range want {
		if _, ok := got[k]; !ok {
			missing = append(missing, k)
		}
	}
	sort.Ints(extra)
	sort.Ints(missing)
	return extra, missing
}

// TestFlatClassifierEquivalence_Corpus is the equivalence gate (plan
// 2606142147, task 4): the flat classifier's code-block line set must be
// byte-identical to the AST-derived lint.collectCodeBlockLines across the
// pinned neutral corpus. It is inert unless MDSMITH_FLATL0_CORPUS points
// at the corpus directory, so CI skips it; the bundled fixture gate below
// runs everywhere.
func TestFlatClassifierEquivalence_Corpus(t *testing.T) {
	corpus := os.Getenv("MDSMITH_FLATL0_CORPUS")
	if corpus == "" {
		t.Skip("set MDSMITH_FLATL0_CORPUS to the neutral corpus dir")
	}
	var files, diverged, extraLines, missingLines int
	_ = filepath.WalkDir(corpus, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(p) != ".md" {
			return nil
		}
		files++
		src, _ := os.ReadFile(p)
		// Mirror production: the engine strips front matter before both
		// the AST parse and the flat classifier, so the equivalence gate
		// must compare on the same stripped line basis.
		f, _ := NewFileFromSource(p, src, true)
		astSet := collectCodeBlockLines(f)
		flatSet := ClassifyLines(f.Lines).CodeBlockLines()
		extra, missing := diffSets(flatSet, astSet)
		if len(extra)+len(missing) > 0 {
			diverged++
			extraLines += len(extra)
			missingLines += len(missing)
			if diverged <= 20 {
				t.Errorf("%s: flat extra=%v missing=%v", filepath.Base(p), extra, missing)
			}
		}
		return nil
	})
	t.Logf("corpus equivalence: files=%d diverged=%d extraLines=%d missingLines=%d",
		files, diverged, extraLines, missingLines)
	assert.Zero(t, diverged, "flat classifier must be byte-identical to the AST code-block set")
}
