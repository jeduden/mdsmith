package occurrence

import (
	"regexp"
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
)

// checkPatternOnlyAllocBudget pins docs/development/high-performance-go.md's
// "skip work you don't need" pattern: checkFile/checkSections/checkParagraphs
// each ran r.searchText (a strings.ToLower allocation) once per paragraph in
// "each" mode even when the rule is configured with Pattern instead of
// Tokens, since the inner `for ti := range r.Tokens` loop is a no-op when
// r.Tokens is empty. Measured baseline on a 5-paragraph fixture: 15
// allocs/op (ExtractText + the wasted ToLower + countPattern's
// FindAllStringIndex, once each per paragraph) before gating the
// tokens-only work on len(r.Tokens) > 0; 10 after (the wasted ToLower is
// gone, ExtractText and countPattern still allocate per paragraph as
// expected).
const checkPatternOnlyAllocBudget = 10

var checkPatternOnlyFixture = mustFileForAlloc(`# Title

First paragraph with some prose to scan for a pattern.

Second paragraph continues the document with more words.

Third paragraph adds even more text for the scan to walk.

Fourth paragraph rounds out the section under test here.

Fifth paragraph closes the fixture with a final sentence.
`)

func mustFileForAlloc(src string) *lint.File {
	f, err := lint.NewFile("test.md", []byte(src))
	if err != nil {
		panic(err)
	}
	return f
}

func newPatternOnlyRule() *Rule {
	return &Rule{
		Pattern:       regexp.MustCompile(`paragraph`),
		patternSource: "paragraph",
		Max:           -1,
		Count:         "each",
	}
}

// TestCheck_PatternOnly_AllocBudget_File pins the file-scope wasted-alloc
// fix under a normal `go test` run.
func TestCheck_PatternOnly_AllocBudget_File(t *testing.T) {
	if testing.Short() {
		t.Skip("alloc gate skipped in -short mode")
	}
	if raceEnabled {
		t.Skip("alloc gate skipped under -race; the race detector " +
			"adds allocation bookkeeping that perturbs the count")
	}
	r := newPatternOnlyRule()
	r.Scope = "file"

	allocs := testing.AllocsPerRun(200, func() {
		_ = r.checkFile(checkPatternOnlyFixture)
	})
	t.Logf("checkFile (pattern-only) allocs/op = %.1f (budget = %d)", allocs, checkPatternOnlyAllocBudget)
	if allocs > float64(checkPatternOnlyAllocBudget) {
		t.Fatalf("checkFile (pattern-only) allocs/op = %.1f, budget = %d; "+
			"the empty-Tokens loop may be doing wasted searchText work again",
			allocs, checkPatternOnlyAllocBudget)
	}
}

// TestCheck_PatternOnly_AllocBudget_Section mirrors the file-scope gate for
// section scope.
func TestCheck_PatternOnly_AllocBudget_Section(t *testing.T) {
	if testing.Short() {
		t.Skip("alloc gate skipped in -short mode")
	}
	if raceEnabled {
		t.Skip("alloc gate skipped under -race; the race detector " +
			"adds allocation bookkeeping that perturbs the count")
	}
	r := newPatternOnlyRule()
	r.Scope = "section"

	allocs := testing.AllocsPerRun(200, func() {
		_ = r.checkSections(checkPatternOnlyFixture)
	})
	t.Logf("checkSections (pattern-only) allocs/op = %.1f (budget = %d)", allocs, checkPatternOnlyAllocBudget)
	if allocs > float64(checkPatternOnlyAllocBudget) {
		t.Fatalf("checkSections (pattern-only) allocs/op = %.1f, budget = %d; "+
			"the empty-Tokens loop may be doing wasted searchText work again",
			allocs, checkPatternOnlyAllocBudget)
	}
}

// TestCheck_PatternOnly_AllocBudget_Paragraph mirrors the file-scope gate
// for paragraph scope.
func TestCheck_PatternOnly_AllocBudget_Paragraph(t *testing.T) {
	if testing.Short() {
		t.Skip("alloc gate skipped in -short mode")
	}
	if raceEnabled {
		t.Skip("alloc gate skipped under -race; the race detector " +
			"adds allocation bookkeeping that perturbs the count")
	}
	r := newPatternOnlyRule()
	r.Scope = "paragraph"

	allocs := testing.AllocsPerRun(200, func() {
		_ = r.checkParagraphs(checkPatternOnlyFixture)
	})
	t.Logf("checkParagraphs (pattern-only) allocs/op = %.1f (budget = %d)", allocs, checkPatternOnlyAllocBudget)
	if allocs > float64(checkPatternOnlyAllocBudget) {
		t.Fatalf("checkParagraphs (pattern-only) allocs/op = %.1f, budget = %d; "+
			"the empty-Tokens loop may be doing wasted searchText work again",
			allocs, checkPatternOnlyAllocBudget)
	}
}
