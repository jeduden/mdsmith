package rulelayer

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEmbeddedManifestMatchesAuditOracle keeps the embedded copy of the
// rule-walk manifest byte-identical to the audit oracle the integration
// gate enforces. If the audit re-classifies a rule, this fails until the
// embedded copy is refreshed, so the engine's parse-skip gate and the
// audit gate can never disagree about a rule's layer.
func TestEmbeddedManifestMatchesAuditOracle(t *testing.T) {
	oracle, err := os.ReadFile(filepath.Join(
		"..", "integration", "testdata", "rule_walk_audit.json"))
	require.NoError(t, err)
	assert.Equal(t, string(oracle), string(auditJSON),
		"embedded manifest drifted from the audit oracle; copy "+
			"internal/integration/testdata/rule_walk_audit.json into "+
			"internal/rulelayer/")
}

func TestIsLayer0(t *testing.T) {
	// A few representative "A-no-skipping" rules resolve to Layer 0.
	for _, id := range []string{"MDS006", "MDS007", "MDS022", "MDS054"} {
		assert.True(t, IsLayer0(id), "%s should be Layer 0", id)
		assert.Equal(t, Layer0, Of(id))
	}
	// AST-requiring rules are not Layer 0.
	for _, id := range []string{"MDS001", "MDS019", "MDS023"} {
		assert.False(t, IsLayer0(id), "%s should require the AST", id)
		assert.Equal(t, LayerAST, Of(id))
	}
}

func TestUnknownRuleIsNotLayer0(t *testing.T) {
	assert.False(t, IsLayer0("MDS999"))
	assert.Equal(t, LayerUnknown, Of("MDS999"))
}
