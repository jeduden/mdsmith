package release

import (
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

	// One row per rule the convention disables, plus the generated-by
	// comment and the two table header lines.
	conv, err := convention.Lookup("parity", nil)
	require.NoError(t, err)
	dataRows := 0
	for _, line := range strings.Split(strings.TrimSpace(got), "\n") {
		if strings.HasPrefix(line, "| MDS") {
			dataRows++
		}
	}
	assert.Equal(t, len(conv.Rules), dataRows,
		"one table row per disabled rule")

	// Spot-check representative rows and the default/opt-in labelling,
	// which is read from the rule registry, not hand-written.
	assert.Contains(t, got, "MDS019 catalog")
	assert.Contains(t, got, "MDS027 cross-file-reference-integrity")
	assert.Regexp(t, `MDS028 token-budget\s+\| default`, got)
	assert.Regexp(t, `MDS029 conciseness-scoring\s+\| opt-in`, got)
	assert.Regexp(t, `MDS033 directory-structure\s+\| opt-in`, got)

	// Rows are sorted by MDS id: MDS019 precedes MDS058.
	assert.Less(t, strings.Index(got, "MDS019"), strings.Index(got, "MDS058"))
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

func TestRenderConventionDisableTable_UnknownConvention(t *testing.T) {
	// An unknown convention name surfaces the Lookup error rather than
	// rendering an empty table.
	_, err := renderConventionDisableTable("does-not-exist", rule.ByName)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does-not-exist")
}

func TestRenderConventionDisableTable_UnregisteredRule(t *testing.T) {
	// A byName that resolves nothing models a rule renamed out of the
	// registry; the renderer must error, not drop the row.
	_, err := renderConventionDisableTable(
		"parity", func(string) rule.Rule { return nil })
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not registered")
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
