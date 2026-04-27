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
