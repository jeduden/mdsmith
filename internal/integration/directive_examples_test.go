package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jeduden/mdsmith/internal/archetype/gensection"
	"github.com/jeduden/mdsmith/internal/rule"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDirectiveRulesHaveExamples enforces that every rule providing a
// generated-section directive (gensection.Directive) ships canonical
// before/after example fixtures: a bad/ folder demonstrating the
// violating pattern and a good/ folder demonstrating the corrected
// pattern. When the rule is also fixable, a fixed/ folder must hold
// the post-`mdsmith fix` body for every bad/ example.
//
// The markdown-audit skill and the rule README use these folders as
// the single source of truth for "what does this directive look like
// in practice"; missing examples leave both surfaces out of date.
func TestDirectiveRulesHaveExamples(t *testing.T) {
	for _, r := range rule.All() {
		d, ok := r.(gensection.Directive)
		if !ok {
			continue
		}
		t.Run(r.ID(), func(t *testing.T) {
			dir := ruleFixtureDir(t, r.ID())

			requireExampleDir(t, dir, "bad")
			requireExampleDir(t, dir, "good")

			if _, fixable := r.(rule.FixableRule); fixable {
				requireFixedMatchesBad(t, dir)
			}

			// Cross-check the directive name resolves to a real rule.
			assert.Equal(t, r.ID(), d.RuleID(),
				"directive name %q reports rule ID %q but is registered as %q",
				d.Name(), d.RuleID(), r.ID())
		})
	}
}

// ruleFixtureDir resolves the on-disk fixture directory for a rule ID by
// globbing `internal/rules/<id>-*`. Fails the test when no directory or
// more than one matches.
func ruleFixtureDir(t *testing.T, ruleID string) string {
	t.Helper()
	matches, err := filepath.Glob(
		filepath.Join("..", "..", "internal", "rules", ruleID+"-*"),
	)
	require.NoError(t, err)
	require.Lenf(t, matches, 1,
		"expected exactly one fixture dir for %s, got %v", ruleID, matches)
	return matches[0]
}

func requireExampleDir(t *testing.T, ruleDir, name string) {
	t.Helper()
	sub := filepath.Join(ruleDir, name)
	require.Truef(t, isDir(sub),
		"directive rule %s is missing %s/ examples (expected at %s)",
		filepath.Base(ruleDir), name, sub)
	files, err := filepath.Glob(filepath.Join(sub, "*.md"))
	require.NoError(t, err)
	require.NotEmptyf(t, files,
		"directive rule %s has %s/ with no .md examples",
		filepath.Base(ruleDir), name)
}

// requireFixedMatchesBad enforces a 1:1 mapping between bad/*.md and
// fixed/*.md for fixable directive rules: every bad example must have
// a sibling fixed example with the same filename. The fix-loop test in
// runFixFolderFile already verifies the body, but it silently skips
// bad/ files that have no fixed/ counterpart. This makes the gap loud.
func requireFixedMatchesBad(t *testing.T, ruleDir string) {
	t.Helper()
	fixedDir := filepath.Join(ruleDir, "fixed")
	require.Truef(t, isDir(fixedDir),
		"fixable directive rule %s is missing fixed/ examples",
		filepath.Base(ruleDir))

	badFiles, err := filepath.Glob(filepath.Join(ruleDir, "bad", "*.md"))
	require.NoError(t, err)
	fixedFiles, err := filepath.Glob(filepath.Join(fixedDir, "*.md"))
	require.NoError(t, err)
	require.NotEmptyf(t, fixedFiles,
		"fixable directive rule %s has fixed/ with no .md examples",
		filepath.Base(ruleDir))

	fixedSet := map[string]struct{}{}
	for _, f := range fixedFiles {
		fixedSet[filepath.Base(f)] = struct{}{}
	}
	var missing []string
	for _, b := range badFiles {
		name := filepath.Base(b)
		// Skip pure-validation bad fixtures that cannot be auto-fixed
		// (e.g. cycle detection, missing required params). They are
		// opted out by listing the basename in a sentinel marker file
		// `bad/.nofix` to keep the convention explicit.
		if isNoFixBad(t, ruleDir, name) {
			continue
		}
		if _, ok := fixedSet[name]; !ok {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		assert.Failf(t, "missing fixed/ examples",
			"rule %s: bad/%s have no matching fixed/ entry "+
				"(list them in bad/.nofix if they cannot be auto-fixed)",
			filepath.Base(ruleDir), strings.Join(missing, ", "))
	}
}

// isNoFixBad reports whether the named bad fixture is listed in
// `bad/.nofix` as an intentionally non-fixable case. Lines starting
// with `#` are comments; blank lines are ignored.
func isNoFixBad(t *testing.T, ruleDir, name string) bool {
	t.Helper()
	path := filepath.Join(ruleDir, "bad", ".nofix")
	if !fileExists(path) {
		return false
	}
	data := readFixture(t, path)
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if line == name {
			return true
		}
	}
	return false
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
