package release

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jeduden/mdsmith/internal/convention"
	"github.com/jeduden/mdsmith/internal/rule"

	// Register the production rule set so RenderParityRulesFragment can
	// resolve each parity rule's MDS id and default-enabled state.
	_ "github.com/jeduden/mdsmith/internal/rules/all"
)

func TestRenderParityRulesFragment(t *testing.T) {
	got, err := RenderParityRulesFragment()
	require.NoError(t, err)

	// One labeled table per parity convention.
	for _, name := range parityConventions {
		assert.Contains(t, got, "**`"+name+"`**",
			"fragment must document %q", name)
	}
	// Each table heads its state column "Parity" (capitalized only in
	// the header; convention names use lowercase "-parity").
	assert.Equal(t, len(parityConventions),
		strings.Count(got, "Parity"),
		"one Parity column header per convention table")

	// One data row per rule across every convention's preset.
	totalRules := 0
	for _, name := range parityConventions {
		conv, err := convention.Lookup(name, nil)
		require.NoError(t, err)
		totalRules += len(conv.Rules)
	}
	dataRows := 0
	for _, line := range strings.Split(strings.TrimSpace(got), "\n") {
		if strings.HasPrefix(line, "| MDS") {
			dataRows++
		}
	}
	assert.Equal(t, totalRules, dataRows, "one table row per preset rule")

	// Spot-check that gomarklint-parity enables an opt-in rule it turns
	// on and disables a default it skips, with the default/opt-in
	// labelling read from the rule registry, not hand-written.
	assert.Regexp(t, `MDS042 emphasis-style\s+\| opt-in\s+\| enabled`, got)
	assert.Regexp(t, `MDS001 line-length\s+\| default\s+\| disabled`, got)

	// Conventions render in parityConventions order (gomarklint first).
	assert.Less(t,
		strings.Index(got, "gomarklint-parity"),
		strings.Index(got, "markdownlint-parity"))
}

// TestParityRulesFragmentInSync is the drift gate: the committed
// fragment must equal the freshly rendered one. It runs in the normal
// `go test ./...` pass, so a change to the parity convention that is
// not followed by `mdsmith-release sync-parity-rules` fails CI.
func TestParityRulesFragmentInSync(t *testing.T) {
	msg, err := CheckParityRulesFragment(repoRoot(t))
	require.NoError(t, err)
	assert.Empty(t, msg, "run `mdsmith-release sync-parity-rules` to refresh")
}

func TestRenderConventionRuleTable_UnknownConvention(t *testing.T) {
	// An unknown convention name surfaces the Lookup error rather than
	// rendering an empty table.
	var buf bytes.Buffer
	err := renderConventionRuleTable(&buf, "does-not-exist", rule.ByName)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does-not-exist")
}

func TestRenderConventionRuleTable_UnregisteredRule(t *testing.T) {
	// A byName that resolves nothing models a rule renamed out of the
	// registry; the renderer must error, not drop the row.
	var buf bytes.Buffer
	err := renderConventionRuleTable(
		&buf, "gomarklint-parity", func(string) rule.Rule { return nil })
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not registered")
}

// TestWritePaddedTableFloorsNarrowColumns drives the separator floor:
// a column whose widest cell is narrower than three must still render
// at least "---", matching internal/rules/tablefmt (and the
// cross-flavor minimum markdown-it and pandoc require).
func TestWritePaddedTableFloorsNarrowColumns(t *testing.T) {
	var buf bytes.Buffer
	writePaddedTable(&buf, []string{"A", "B"}, [][]string{{"x", "y"}})

	want := "| A   | B   |\n| --- | --- |\n| x   | y   |\n"
	assert.Equal(t, want, buf.String())
}

func TestApplyParityRulesFragment_WritesWhenMissing(t *testing.T) {
	// Empty root: Apply creates the nested fragment path and reports a
	// change. Covers the write branch the in-sync test does not reach.
	root := t.TempDir()
	changed, err := ApplyParityRulesFragment(root)
	require.NoError(t, err)
	assert.True(t, changed)
	_, statErr := os.Stat(filepath.Join(root, ParityRulesFragmentFile))
	require.NoError(t, statErr)
}

func TestApplyParityRulesFragment_Idempotent(t *testing.T) {
	// A second Apply on an already-written tree reports no change.
	root := t.TempDir()
	_, err := ApplyParityRulesFragment(root)
	require.NoError(t, err)
	changed, err := ApplyParityRulesFragment(root)
	require.NoError(t, err)
	assert.False(t, changed)
}

func TestApplyParityRulesFragment_PropagatesReadError(t *testing.T) {
	// A directory at the target path makes os.ReadFile fail with a
	// non-NotExist error, which Apply surfaces.
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(
		filepath.Join(root, ParityRulesFragmentFile), 0o755))
	_, err := ApplyParityRulesFragment(root)
	assert.Error(t, err)
}

func TestApplyParityRulesFragment_PropagatesRenderError(t *testing.T) {
	// A failing renderer short-circuits Apply before any file I/O.
	_, err := applyParityRulesFragment(t.TempDir(),
		func() (string, error) { return "", errors.New("boom") })
	assert.Error(t, err)
}

func TestCheckParityRulesFragment_PropagatesRenderError(t *testing.T) {
	// A failing renderer short-circuits Check before any file I/O.
	_, err := checkParityRulesFragment(t.TempDir(),
		func() (string, error) { return "", errors.New("boom") })
	assert.Error(t, err)
}

func TestCheckParityRulesFragment_ReadError(t *testing.T) {
	// Empty root: the fragment is missing, so Check surfaces a read
	// error (distinct from the drift branch below).
	root := t.TempDir()
	_, err := CheckParityRulesFragment(root)
	assert.Error(t, err)
}

func TestCheckParityRulesFragment_Drift(t *testing.T) {
	// Apply then tamper: Check returns a non-empty drift message with
	// no error.
	root := t.TempDir()
	_, err := ApplyParityRulesFragment(root)
	require.NoError(t, err)
	path := filepath.Join(root, ParityRulesFragmentFile)
	require.NoError(t, os.WriteFile(path, []byte("hand-edited\n"), 0o644))
	msg, err := CheckParityRulesFragment(root)
	require.NoError(t, err)
	assert.NotEmpty(t, msg)
	assert.Contains(t, msg, ParityRulesFragmentFile)
}
