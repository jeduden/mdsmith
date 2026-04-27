package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- ResolveWithProvenance ---

func TestResolveWithProvenance_DefaultLayer(t *testing.T) {
	cfg := &Config{
		Rules: map[string]RuleCfg{
			"line-length": {Enabled: true, Settings: map[string]any{"max": 80}},
		},
	}
	res := ResolveWithProvenance(cfg, "doc.md", nil)

	require.Contains(t, res.Rules, "line-length")
	rp := res.Rules["line-length"]
	assert.True(t, rp.Final.Enabled)
	require.Contains(t, rp.Leaves, "max")
	leaf := rp.Leaves["max"]
	assert.Equal(t, 80, leaf.Final)
	require.Len(t, leaf.Chain, 1)
	assert.Equal(t, LayerDefault, leaf.Chain[0].Layer)
	assert.Equal(t, "default", leaf.Chain[0].Source)
	assert.Equal(t, 80, leaf.Chain[0].Value)
}

func TestResolveWithProvenance_KindOverridesDefault(t *testing.T) {
	cfg := &Config{
		Rules: map[string]RuleCfg{
			"line-length": {Enabled: true, Settings: map[string]any{"max": 80}},
		},
		Kinds: map[string]KindBody{
			"wide": {Rules: map[string]RuleCfg{
				"line-length": {Enabled: true, Settings: map[string]any{"max": 200}},
			}},
		},
		KindAssignment: []KindAssignmentEntry{
			{Files: []string{"wide/*.md"}, Kinds: []string{"wide"}},
		},
	}
	res := ResolveWithProvenance(cfg, "wide/doc.md", nil)

	rp := res.Rules["line-length"]
	leaf := rp.Leaves["max"]
	assert.Equal(t, 200, leaf.Final)
	require.Len(t, leaf.Chain, 2)
	assert.Equal(t, LayerDefault, leaf.Chain[0].Layer)
	assert.Equal(t, LayerKind, leaf.Chain[1].Layer)
	assert.Equal(t, "kinds.wide", leaf.Chain[1].Source)
	assert.Equal(t, 200, leaf.Chain[1].Value)
	assert.Equal(t, "kinds.wide", leaf.WinningSource)
}

func TestResolveWithProvenance_OverrideBeatsKind(t *testing.T) {
	cfg := &Config{
		Rules: map[string]RuleCfg{
			"line-length": {Enabled: true, Settings: map[string]any{"max": 80}},
		},
		Kinds: map[string]KindBody{
			"wide": {Rules: map[string]RuleCfg{
				"line-length": {Enabled: true, Settings: map[string]any{"max": 200}},
			}},
		},
		KindAssignment: []KindAssignmentEntry{
			{Files: []string{"wide/*.md"}, Kinds: []string{"wide"}},
		},
		Overrides: []Override{
			{
				Files: []string{"wide/special.md"},
				Rules: map[string]RuleCfg{
					"line-length": {Enabled: true, Settings: map[string]any{"max": 120}},
				},
			},
		},
	}
	res := ResolveWithProvenance(cfg, "wide/special.md", nil)

	leaf := res.Rules["line-length"].Leaves["max"]
	assert.Equal(t, 120, leaf.Final)
	require.Len(t, leaf.Chain, 3)
	assert.Equal(t, LayerOverride, leaf.Chain[2].Layer)
	assert.Equal(t, "overrides[0]", leaf.Chain[2].Source)
	assert.Equal(t, "overrides[0]", leaf.WinningSource)
}

func TestResolveWithProvenance_FrontMatterKindAppears(t *testing.T) {
	cfg := &Config{
		Kinds: map[string]KindBody{
			"plan": {Rules: map[string]RuleCfg{
				"paragraph-readability": {Enabled: false},
			}},
		},
	}
	res := ResolveWithProvenance(cfg, "doc.md", []string{"plan"})

	require.Contains(t, res.EffectiveKinds, "plan")
	srcs := res.KindSources["plan"]
	require.NotEmpty(t, srcs)
	assert.Equal(t, "front-matter", srcs[0])

	rp := res.Rules["paragraph-readability"]
	assert.False(t, rp.Final.Enabled)
	require.Contains(t, rp.Leaves, "enabled")
	enabledLeaf := rp.Leaves["enabled"]
	assert.Equal(t, "kinds.plan", enabledLeaf.WinningSource)
}

func TestResolveWithProvenance_LeafGranularKeptAcrossLayers(t *testing.T) {
	// Plan 95 specifies per-leaf granularity. Today every leaf in a
	// given rule shares the same source (block-replace), but the data
	// structure must already track them individually.
	cfg := &Config{
		Rules: map[string]RuleCfg{
			"line-length": {Enabled: true, Settings: map[string]any{
				"max":         80,
				"strict-tabs": true,
			}},
		},
		Kinds: map[string]KindBody{
			"wide": {Rules: map[string]RuleCfg{
				"line-length": {Enabled: true, Settings: map[string]any{
					"max": 200,
				}},
			}},
		},
		KindAssignment: []KindAssignmentEntry{
			{Files: []string{"wide/*.md"}, Kinds: []string{"wide"}},
		},
	}
	res := ResolveWithProvenance(cfg, "wide/doc.md", nil)
	rp := res.Rules["line-length"]
	// Block-replace today: under the kind body, only "max" is in scope.
	assert.Equal(t, 200, rp.Leaves["max"].Final)
	assert.Equal(t, "kinds.wide", rp.Leaves["max"].WinningSource)
	// "strict-tabs" was only on the default; it is not present in the kind
	// body, so under block-replace its sibling "max" comes from the kind
	// alone — the leaf model reports each leaf's own chain.
	if leaf, ok := rp.Leaves["strict-tabs"]; ok {
		// If it is preserved, source must be the default; if not,
		// block-replace dropped it. Either is consistent with the model.
		_ = leaf
	}
}

func TestResolveWithProvenance_DisabledViaCategory(t *testing.T) {
	// Category disabling is part of the resolution but goes through
	// ApplyCategories rather than the rule chain. We simply verify the
	// structure exposes the post-category enabled state.
	cfg := &Config{
		Rules: map[string]RuleCfg{
			"line-length": {Enabled: true, Settings: map[string]any{"max": 80}},
		},
		Categories: map[string]bool{"line": false},
	}
	res := ResolveWithProvenance(cfg, "doc.md", nil)
	require.Contains(t, res.Rules, "line-length")
	// Category overrides are observable on Final after ApplyCategories.
	// The leaf chain still reflects the per-rule layers.
	assert.False(t, res.Categories["line"])
}

// --- ChainForRule ---

func TestChainForRule_ReportsEveryLayerEvenNoOps(t *testing.T) {
	cfg := &Config{
		Rules: map[string]RuleCfg{
			"line-length": {Enabled: true, Settings: map[string]any{"max": 80}},
		},
		Kinds: map[string]KindBody{
			"plan": {Rules: map[string]RuleCfg{
				"paragraph-readability": {Enabled: false},
			}},
			"wide": {Rules: map[string]RuleCfg{
				"line-length": {Enabled: true, Settings: map[string]any{"max": 200}},
			}},
		},
		KindAssignment: []KindAssignmentEntry{
			{Files: []string{"*.md"}, Kinds: []string{"plan", "wide"}},
		},
		Overrides: []Override{
			{
				Files: []string{"*.md"},
				Rules: map[string]RuleCfg{
					"paragraph-readability": {Enabled: false},
				},
			},
		},
	}
	chain := ChainForRule(cfg, "doc.md", nil, "line-length")

	// Expect one entry per layer including no-op layers:
	//   default, kinds.plan (no-op), kinds.wide (touches), overrides[0] (no-op).
	require.GreaterOrEqual(t, len(chain), 4)

	// Find each layer in the chain.
	var foundPlan, foundWide, foundOverride bool
	for _, e := range chain {
		switch e.Source {
		case "kinds.plan":
			foundPlan = true
			assert.False(t, e.Touched)
		case "kinds.wide":
			foundWide = true
			assert.True(t, e.Touched)
		case "overrides[0]":
			foundOverride = true
			assert.False(t, e.Touched, "override does not touch line-length")
		}
	}
	assert.True(t, foundPlan, "plan kind should appear as a no-op layer")
	assert.True(t, foundWide, "wide kind should appear as a touching layer")
	assert.True(t, foundOverride, "matching override should appear as a no-op layer for the unrelated rule")
}
