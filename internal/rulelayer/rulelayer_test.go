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
	for _, id := range []string{"MDS006", "MDS007", "MDS022", "MDS064"} {
		assert.True(t, IsLayer0(id), "%s should be Layer 0", id)
		assert.Equal(t, Layer0, Of(id))
	}
	// AST-requiring rules are not Layer 0.
	for _, id := range []string{"MDS001", "MDS019", "MDS023"} {
		assert.False(t, IsLayer0(id), "%s should require the AST", id)
		assert.Equal(t, LayerAST, Of(id))
	}
}

// TestAuditLayer0RulesAreCodeSpanFree guards the parse-skip gate's
// soundness invariant: every rule the manifest marks "A-no-skipping" and
// rulelayer admits as Layer 0 must not consume an AST-only inline
// projection. MDS047 and MDS054 are "A-no-skipping" (nil-AST safe) yet read
// code-span ranges that go empty without a parse, so they are excluded.
// This test fails if a future rule lands in "A-no-skipping" while reading
// code spans and is not added to astProjectionConsumers.
func TestAstProjectionConsumersAreNotLayer0(t *testing.T) {
	for id := range astProjectionConsumers {
		assert.False(t, IsLayer0(id),
			"%s consumes an AST-only projection; gate must not admit it", id)
		assert.Equal(t, LayerAST, Of(id))
	}
}

func TestUnknownRuleIsNotLayer0(t *testing.T) {
	assert.False(t, IsLayer0("MDS999"))
	assert.Equal(t, LayerUnknown, Of("MDS999"))
}

// TestBuildLayerMapFromPanicsOnMalformedManifest drives the decode-failure
// branch: malformed manifest JSON is a build-time contract violation, so the
// builder panics rather than returning a degraded table.
func TestBuildLayerMapFromPanicsOnMalformedManifest(t *testing.T) {
	assert.Panics(t, func() {
		buildLayerMapFrom([]byte("{not json"))
	})
}

// TestBuildLayerMapFromClassifies confirms both arms of the category switch:
// an "A-no-skipping" rule maps to Layer0, an astProjectionConsumer and any
// other category map to LayerAST.
func TestBuildLayerMapFromClassifies(t *testing.T) {
	m := buildLayerMapFrom([]byte(`[
		{"id":"MDS900","category":"A-no-skipping"},
		{"id":"MDS047","category":"A-no-skipping"},
		{"id":"MDS901","category":"ast-required"}
	]`))
	assert.Equal(t, Layer0, m["MDS900"])
	assert.Equal(t, LayerAST, m["MDS047"], "astProjectionConsumer stays AST")
	assert.Equal(t, LayerAST, m["MDS901"])
}
