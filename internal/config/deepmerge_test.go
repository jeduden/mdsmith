package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// --- deepMergeRuleCfg ---

func TestDeepMergeRuleCfg_DisabledLayerReplacesAll(t *testing.T) {
	base := RuleCfg{Enabled: true, Settings: map[string]any{"max": 80}}
	layer := RuleCfg{Enabled: false}
	got := deepMergeRuleCfg(base, layer, nil)
	assert.False(t, got.Enabled)
	assert.Nil(t, got.Settings, "disabling a rule should clear settings")
}

func TestDeepMergeRuleCfg_EnabledNoSettingsKeepsBaseSettings(t *testing.T) {
	base := RuleCfg{Enabled: false, Settings: map[string]any{"max": 80}}
	layer := RuleCfg{Enabled: true}
	got := deepMergeRuleCfg(base, layer, nil)
	assert.True(t, got.Enabled)
	// A bare `true` does not erase prior settings; it only flips Enabled.
	assert.Equal(t, 80, got.Settings["max"])
}

func TestDeepMergeRuleCfg_MapMergeMergesSiblingKeys(t *testing.T) {
	base := RuleCfg{Enabled: true, Settings: map[string]any{
		"max":   80,
		"stern": true,
	}}
	layer := RuleCfg{Enabled: true, Settings: map[string]any{
		"max": 100,
	}}
	got := deepMergeRuleCfg(base, layer, nil)
	assert.Equal(t, 100, got.Settings["max"], "later layer's value wins")
	assert.Equal(t, true, got.Settings["stern"], "sibling key from earlier layer is preserved")
}

func TestDeepMergeRuleCfg_NestedMapMerge(t *testing.T) {
	base := RuleCfg{Enabled: true, Settings: map[string]any{
		"thresholds": map[string]any{
			"max": 80,
			"min": 10,
		},
	}}
	layer := RuleCfg{Enabled: true, Settings: map[string]any{
		"thresholds": map[string]any{
			"max": 100,
		},
	}}
	got := deepMergeRuleCfg(base, layer, nil)
	thresholds, ok := got.Settings["thresholds"].(map[string]any)
	assert.True(t, ok)
	assert.Equal(t, 100, thresholds["max"])
	assert.Equal(t, 10, thresholds["min"], "sibling key in nested map preserved")
}

func TestDeepMergeRuleCfg_ListReplaceByDefault(t *testing.T) {
	base := RuleCfg{Enabled: true, Settings: map[string]any{
		"exclude": []any{"code-blocks", "tables"},
	}}
	layer := RuleCfg{Enabled: true, Settings: map[string]any{
		"exclude": []any{"urls"},
	}}
	got := deepMergeRuleCfg(base, layer, nil)
	assert.Equal(t, []any{"urls"}, got.Settings["exclude"],
		"lists default to replace")
}

func TestDeepMergeRuleCfg_ListAppendWhenModeAppend(t *testing.T) {
	base := RuleCfg{Enabled: true, Settings: map[string]any{
		"placeholders": []any{"var-token"},
	}}
	layer := RuleCfg{Enabled: true, Settings: map[string]any{
		"placeholders": []any{"heading-question"},
	}}
	modes := map[string]MergeMode{"placeholders": MergeAppend}
	got := deepMergeRuleCfg(base, layer, modes)
	assert.Equal(t, []any{"var-token", "heading-question"},
		got.Settings["placeholders"])
}

func TestDeepMergeRuleCfg_ListAppendStringSlices(t *testing.T) {
	// Settings parsed via DefaultSettings may yield []string, not []any.
	base := RuleCfg{Enabled: true, Settings: map[string]any{
		"placeholders": []string{"var-token"},
	}}
	layer := RuleCfg{Enabled: true, Settings: map[string]any{
		"placeholders": []any{"heading-question"},
	}}
	modes := map[string]MergeMode{"placeholders": MergeAppend}
	got := deepMergeRuleCfg(base, layer, modes)
	got2, ok := got.Settings["placeholders"].([]any)
	assert.True(t, ok)
	assert.Equal(t, []any{"var-token", "heading-question"}, got2)
}

func TestDeepMergeRuleCfg_ScalarReplace(t *testing.T) {
	base := RuleCfg{Enabled: true, Settings: map[string]any{"style": "atx"}}
	layer := RuleCfg{Enabled: true, Settings: map[string]any{"style": "setext"}}
	got := deepMergeRuleCfg(base, layer, nil)
	assert.Equal(t, "setext", got.Settings["style"])
}

func TestDeepMergeRuleCfg_LayerKeyMissingPreservesBaseKey(t *testing.T) {
	base := RuleCfg{Enabled: true, Settings: map[string]any{
		"level":        2,
		"placeholders": []any{"var-token"},
	}}
	layer := RuleCfg{Enabled: true, Settings: map[string]any{
		"level": 3,
	}}
	got := deepMergeRuleCfg(base, layer, nil)
	assert.Equal(t, 3, got.Settings["level"])
	assert.Equal(t, []any{"var-token"}, got.Settings["placeholders"])
}

// --- Effective with deep-merge ---

func TestEffectiveTwoKindsContributeNonOverlappingNestedKeys(t *testing.T) {
	cfg := &Config{
		Rules: map[string]RuleCfg{
			"first-line-heading": {Enabled: true, Settings: map[string]any{"level": 1}},
		},
		Kinds: map[string]KindBody{
			"a": {Rules: map[string]RuleCfg{
				"first-line-heading": {Enabled: true, Settings: map[string]any{"level": 2}},
			}},
			"b": {Rules: map[string]RuleCfg{
				"first-line-heading": {Enabled: true, Settings: map[string]any{
					"placeholders": []any{"var-token"},
				}},
			}},
		},
	}
	result := Effective(cfg, "doc.md", []string{"a", "b"})
	rc := result["first-line-heading"]
	assert.Equal(t, 2, rc.Settings["level"], "kind a's level survives")
	assert.Equal(t, []any{"var-token"}, rc.Settings["placeholders"],
		"kind b's placeholders contribute")
}

func TestEffectiveOverrideKeepsSiblingFromKind(t *testing.T) {
	cfg := &Config{
		Rules: map[string]RuleCfg{
			"line-length": {Enabled: true, Settings: map[string]any{"max": 80}},
		},
		Kinds: map[string]KindBody{
			"wide": {Rules: map[string]RuleCfg{
				"line-length": {Enabled: true, Settings: map[string]any{
					"max":     200,
					"exclude": []any{"code-blocks", "tables"},
				}},
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
	result := Effective(cfg, "wide/special.md", nil)
	rc := result["line-length"]
	assert.Equal(t, 120, rc.Settings["max"], "override wins on max")
	assert.Equal(t, []any{"code-blocks", "tables"}, rc.Settings["exclude"],
		"override does not erase sibling exclude key set by kind")
}

func TestEffectiveListAppendModeAcrossLayers(t *testing.T) {
	// Use a custom merge-mode lookup via the test helper effectiveRulesWithModes.
	cfg := &Config{
		Rules: map[string]RuleCfg{
			"first-line-heading": {Enabled: true, Settings: map[string]any{
				"placeholders": []any{"var-token"},
			}},
		},
		Kinds: map[string]KindBody{
			"k": {Rules: map[string]RuleCfg{
				"first-line-heading": {Enabled: true, Settings: map[string]any{
					"placeholders": []any{"heading-question"},
				}},
			}},
		},
	}
	modes := func(rule string) map[string]MergeMode {
		if rule == "first-line-heading" {
			return map[string]MergeMode{"placeholders": MergeAppend}
		}
		return nil
	}
	result := effectiveRulesWithModes(cfg, "doc.md",
		resolveEffectiveKinds(cfg, "doc.md", []string{"k"}), modes)
	rc := result["first-line-heading"]
	assert.Equal(t, []any{"var-token", "heading-question"}, rc.Settings["placeholders"])
}

func TestEffectiveListReplaceByDefault(t *testing.T) {
	cfg := &Config{
		Rules: map[string]RuleCfg{
			"line-length": {Enabled: true, Settings: map[string]any{
				"exclude": []any{"code-blocks", "tables"},
			}},
		},
		Kinds: map[string]KindBody{
			"k": {Rules: map[string]RuleCfg{
				"line-length": {Enabled: true, Settings: map[string]any{
					"exclude": []any{"urls"},
				}},
			}},
		},
	}
	result := Effective(cfg, "doc.md", []string{"k"})
	rc := result["line-length"]
	assert.Equal(t, []any{"urls"}, rc.Settings["exclude"],
		"exclude replaces by default (no opt-in mode)")
}

// --- Regression: pre-deep-merge fixture preserves diagnostics ---

// TestDeepMergeRegressionFullBlockReplace verifies that a config layer that
// specifies the entire rule body still wins as before — every leaf is set,
// so deep-merge produces the same result as the old block-replace.
func TestDeepMergeRegressionFullBlockReplace(t *testing.T) {
	cfg := &Config{
		Rules: map[string]RuleCfg{
			"line-length": {Enabled: true, Settings: map[string]any{
				"max":     80,
				"exclude": []any{"code-blocks"},
				"stern":   true,
			}},
		},
		Overrides: []Override{
			{
				Files: []string{"docs/*.md"},
				// Override specifies the full body — same shape as before.
				Rules: map[string]RuleCfg{
					"line-length": {Enabled: true, Settings: map[string]any{
						"max":     200,
						"exclude": []any{"urls", "tables"},
						"stern":   false,
					}},
				},
			},
		},
	}
	result := Effective(cfg, "docs/foo.md", nil)
	rc := result["line-length"]
	assert.Equal(t, 200, rc.Settings["max"])
	assert.Equal(t, []any{"urls", "tables"}, rc.Settings["exclude"])
	assert.Equal(t, false, rc.Settings["stern"])
}
