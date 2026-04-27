package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLeaf_SourceEmptyChain covers the empty-chain branch of Leaf.Source.
func TestLeaf_SourceEmptyChain(t *testing.T) {
	leaf := Leaf{Path: "x", Value: nil, Chain: nil}
	assert.Equal(t, "", leaf.Source())
}

// TestRuleResolution_LeafByPathMissing covers the not-found branch of
// LeafByPath.
func TestRuleResolution_LeafByPathMissing(t *testing.T) {
	rr := &RuleResolution{
		Leaves: []Leaf{{Path: "enabled"}},
	}
	assert.Nil(t, rr.LeafByPath("settings.nope"))
	require.NotNil(t, rr.LeafByPath("enabled"))
}

// TestResolveFile_SkipsUndeclaredKindFromFrontMatter covers the
// `cfg.Kinds[name]` lookup branch in buildLayers when a front-matter
// kind has no declared body.
func TestResolveFile_SkipsUndeclaredKindFromFrontMatter(t *testing.T) {
	cfg := &Config{
		Rules: map[string]RuleCfg{
			"line-length": {Enabled: true, Settings: map[string]any{"max": 80}},
		},
		// no Kinds map: front-matter "ghost" cannot resolve to a body.
	}
	res := ResolveFile(cfg, "x.md", []string{"ghost"})
	require.NotNil(t, res)
	// ghost still appears in the kind list (resolveKindsWithSources adds it),
	// but no kind layer is appended to the merge chain.
	require.Len(t, res.Kinds, 1)
	assert.Equal(t, "ghost", res.Kinds[0].Name)
	rr := res.Rules["line-length"]
	require.Len(t, rr.Layers, 1, "only the default layer applies")
	assert.Equal(t, "default", rr.Layers[0].Source)
}

// TestLeafValue_UnknownPath covers the fall-through branch of leafValue
// when the path is neither "enabled" nor a "settings." prefix.
func TestLeafValue_UnknownPath(t *testing.T) {
	rc := RuleCfg{Enabled: true, Settings: map[string]any{"a": 1}}
	v, ok := leafValue(rc, "garbage.path")
	assert.False(t, ok)
	assert.Nil(t, v)
}

// TestLeafValue_SettingsNil covers the rc.Settings == nil branch.
func TestLeafValue_SettingsNil(t *testing.T) {
	rc := RuleCfg{Enabled: true}
	v, ok := leafValue(rc, "settings.max")
	assert.False(t, ok)
	assert.Nil(t, v)
}

// TestBuildRuleResolution_RuleNeverInLayers covers the defensive !seen
// branch of buildRuleResolution: when no applicable layer sets the rule,
// the function returns an empty RuleResolution rather than panicking.
func TestBuildRuleResolution_RuleNeverInLayers(t *testing.T) {
	layers := []layerInfo{
		{Source: "default", Rules: map[string]RuleCfg{"other": {Enabled: true}}},
	}
	rr := buildRuleResolution("missing", layers)
	assert.Equal(t, "missing", rr.Rule)
	assert.Empty(t, rr.Layers)
	assert.Nil(t, rr.Leaves)
}

// TestBuildLeaves_ChainSkipsLayersMissingPath covers the buildLeaves
// branch where one layer in the chain does not set a particular leaf
// path: it must not appear in that leaf's Chain, but other layers that
// do set the path still contribute. Because effectiveRules replaces
// (rather than deep-merges) RuleCfg, the only way to leave the final
// Settings keyed by `max` while a previous layer has no Settings is to
// declare the rule with no Settings in the default layer and add the
// keyed value via a kind layer.
func TestBuildLeaves_ChainSkipsLayersMissingPath(t *testing.T) {
	cfg := &Config{
		Rules: map[string]RuleCfg{
			// Default declares the rule but does not set max.
			"line-length": {Enabled: true},
		},
		Kinds: map[string]KindBody{
			// Kind sets max=30 — this becomes the final RuleCfg after merge.
			"size": {Rules: map[string]RuleCfg{
				"line-length": {Enabled: true, Settings: map[string]any{"max": 30}},
			}},
		},
		KindAssignment: []KindAssignmentEntry{
			{Files: []string{"x.md"}, Kinds: []string{"size"}},
		},
	}
	res := ResolveFile(cfg, "x.md", nil)
	rr := res.Rules["line-length"]
	maxLeaf := rr.LeafByPath("settings.max")
	require.NotNil(t, maxLeaf)
	// The default layer's RuleCfg has Settings == nil so leafValue
	// returns ok=false and the layer must be omitted from the chain.
	// Only the kind layer contributes.
	require.Len(t, maxLeaf.Chain, 1)
	assert.Equal(t, "kinds.size", maxLeaf.Chain[0].Source)
}
