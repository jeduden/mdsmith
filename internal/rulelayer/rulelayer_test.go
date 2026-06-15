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
	// MDS013 (blank-line-around-headings) and MDS044 (horizontal-rule-
	// style) were migrated to rule.BlockChecker (plan 2606141903): their
	// nil-AST paths serve from the Layer 0 block scan, so the audit
	// reclassified them A-no-skipping and the gate now admits them.
	for _, id := range []string{"MDS006", "MDS007", "MDS013", "MDS022", "MDS044", "MDS064"} {
		assert.True(t, IsLayer0(id), "%s should be Layer 0", id)
		assert.Equal(t, Layer0, Of(id))
	}
	// MDS047 (ambiguous-emphasis) and MDS054 (no-undefined-reference-labels)
	// are "A-no-skipping" and formerly forced to AST because they read the
	// inline code-span ranges. Layer 1 (internal/lint/inline_index.go) now
	// backs those ranges on the nil-AST path, so the override is gone and both
	// resolve to Layer 0.
	for _, id := range []string{"MDS047", "MDS054"} {
		assert.True(t, IsLayer0(id), "%s should be Layer 0 once code spans are backed", id)
		assert.Equal(t, Layer0, Of(id))
	}
	// MDS012 (no-bare-urls) and MDS018 (no-emphasis-as-heading) were
	// "hybrid": they walked the inline tree (Text nodes for bare URLs, a
	// lone Emphasis child for emphasis-as-heading) and produced no
	// diagnostics on a nil AST. Layer 1 re-backs both via the per-block
	// inline parse (internal/lint/inline_blocks.go + inline_emphasis.go):
	// each implements rule.BlockChecker so the engine's block-span dispatch
	// drives them on the parse-skipped File. The audit now classes them
	// "A-no-skipping" and the gate admits both.
	// MDS012 (no-bare-urls), MDS018 (no-emphasis-as-heading), MDS032
	// (no-empty-alt-text), and MDS062 (link-validity) were "hybrid": each
	// walked the inline tree and produced no (or different) diagnostics on
	// a nil AST. Layer 1 re-backs all four via the per-block inline parse
	// (internal/lint/inline_blocks.go, inline_emphasis.go): the NodeChecker
	// rules implement rule.BlockChecker so the engine's block-span dispatch
	// drives them, and MDS062 (a plain Check rule) parses each span in its
	// own Check. The audit now classes all four "A-no-skipping".
	for _, id := range []string{"MDS012", "MDS018", "MDS032", "MDS062"} {
		assert.True(t, IsLayer0(id), "%s should be Layer 0 once re-backed on Layer 1", id)
		assert.Equal(t, Layer0, Of(id))
	}
	// AST-requiring rules are not Layer 0.
	for _, id := range []string{"MDS001", "MDS019", "MDS023"} {
		assert.False(t, IsLayer0(id), "%s should require the AST", id)
		assert.Equal(t, LayerAST, Of(id))
	}
}

// TestAstProjectionConsumersAreNotLayer0 guards the parse-skip gate's
// soundness invariant: any rule the manifest marks "A-no-skipping" yet
// listed in astProjectionConsumers (because it reads an AST-only projection
// Layer N does not yet back) must be forced to AST, so the gate never
// admits it. The map is empty today — Layer 1 backs the code-span ranges
// that were its only entries — so this test is vacuous but pins the
// invariant for any future entry.
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

// TestBuildLayerMapFromClassifies confirms both arms of the category
// switch: an "A-no-skipping" rule maps to Layer0 and any other category
// maps to LayerAST. The astProjectionConsumers override is empty today, so
// the only way to reach LayerAST is a non-"A-no-skipping" category.
func TestBuildLayerMapFromClassifies(t *testing.T) {
	m := buildLayerMapFrom([]byte(`[
		{"id":"MDS900","category":"A-no-skipping"},
		{"id":"MDS901","category":"ast-required"}
	]`))
	assert.Equal(t, Layer0, m["MDS900"])
	assert.Equal(t, LayerAST, m["MDS901"])
}

// TestAstProjectionConsumerOverrideForcesAST pins the override arm of the
// category switch independently of the (currently empty) production map: a
// rule whose category is "A-no-skipping" but which is listed in
// astProjectionConsumers must still resolve to LayerAST. It mutates the
// package map for the duration of one buildLayerMapFrom call so the test
// does not depend on a live override entry.
func TestAstProjectionConsumerOverrideForcesAST(t *testing.T) {
	astProjectionConsumers["MDS902"] = true
	t.Cleanup(func() { delete(astProjectionConsumers, "MDS902") })
	m := buildLayerMapFrom([]byte(`[
		{"id":"MDS902","category":"A-no-skipping"}
	]`))
	assert.Equal(t, LayerAST, m["MDS902"], "override forces an A-no-skipping rule to AST")
}
