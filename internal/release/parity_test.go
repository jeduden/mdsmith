package release

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jeduden/mdsmith/internal/convention"

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
