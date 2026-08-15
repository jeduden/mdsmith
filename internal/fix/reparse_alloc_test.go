package fix

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/rule"
	_ "github.com/jeduden/mdsmith/internal/rules/all" // registers the production rule set for rule.All()
)

// cleanFixDoc is valid Markdown that trips no fixable rule in the
// default rule set (no trailing spaces, no long lines, no bare URLs,
// …), so applyFixPasses' single pass makes zero Fix calls: every
// fixable rule's Check runs against the same unchanged content.
const cleanFixDoc = "# Doc\n\nSome clean content here with no issues at all.\n"

// TestFix_CleanFileDoesNotParsePerFixableRule pins the fix for
// applyFixPasses' redundant re-parse: before the fix, every fixable
// rule in the loop got its own fresh lint.NewFile(path, current) call
// regardless of whether the previous rule actually changed current.
// On an already-clean file — the common case, since most files in a
// workspace need no fixes — that was one full parse per registered
// fixable rule (dozens in the production set) for content that never
// changed. After the fix, a pass that makes zero Fix calls parses
// once.
//
// The ceiling is set well below "one parse per fixable rule" (which
// the production rule set, dozens of FixableRule implementations,
// would blow through by a wide margin) but with headroom over a
// single parse plus every rule's own Check allocations (each rule's
// Check is separately budgeted at <= 10 allocs, CLAUDE.md's
// allocation budget) so per-rule Check cost alone cannot trip it.
func TestFix_CleanFileDoesNotParsePerFixableRule(t *testing.T) {
	if testing.Short() {
		t.Skip("alloc gate skipped in -short mode")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	if err := os.WriteFile(path, []byte(cleanFixDoc), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	cfg := config.Defaults()
	newFixer := func() *Fixer {
		return &Fixer{Config: cfg, Rules: rule.All(), RootDir: dir}
	}

	// Warm the filesystem cache before measuring.
	res := newFixer().Fix([]string{path})
	if len(res.Errors) != 0 {
		t.Fatalf("warm-up Fix returned errors: %v", res.Errors)
	}
	if len(res.Modified) != 0 {
		t.Fatalf("cleanFixDoc must trip no fixable rule, got modified=%v", res.Modified)
	}

	fixableCount := 0
	for _, r := range rule.All() {
		if _, ok := r.(rule.FixableRule); ok {
			fixableCount++
		}
	}
	if fixableCount < 20 {
		t.Fatalf("expected the production rule set (rules/all) with dozens of "+
			"FixableRule implementations, got only %d — blank import missing?", fixableCount)
	}

	allocs := testing.AllocsPerRun(5, func() {
		_ = newFixer().Fix([]string{path})
	})
	t.Logf("Fix on a single clean file (%d fixable rules registered): allocs/op = %.0f",
		fixableCount, allocs)

	// A single lint.NewFile parse plus hydration plus every fixable
	// rule's <=10-alloc Check call comfortably fits under 800 allocs
	// for one small file; a per-rule reparse would scale with
	// fixableCount instead (each parse alone costs well over 20
	// allocs on this codebase's goldmark parser), blowing past it.
	const ceiling = 800
	if allocs > ceiling {
		t.Fatalf("Fix allocs/op = %.0f, want <= %d (applyFixPasses reparsing per "+
			"fixable rule instead of only after a Fix call?)", allocs, ceiling)
	}
}
