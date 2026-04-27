package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- mergeRuleCfg unit tests ---

func TestMergeRuleCfgScalarLeafReplaced(t *testing.T) {
	earlier := RuleCfg{
		Enabled:  true,
		Settings: map[string]any{"max": 80, "tabs": 4},
	}
	later := RuleCfg{
		Enabled:  true,
		Settings: map[string]any{"max": 120},
	}
	got := mergeRuleCfg("line-length", earlier, later)
	assert.Equal(t, 120, got.Settings["max"], "later scalar wins")
	assert.Equal(t, 4, got.Settings["tabs"], "earlier sibling preserved")
}

func TestMergeRuleCfgNestedMapKeyByKey(t *testing.T) {
	earlier := RuleCfg{
		Enabled: true,
		Settings: map[string]any{
			"limits": map[string]any{"max": 80, "min": 10},
		},
	}
	later := RuleCfg{
		Enabled: true,
		Settings: map[string]any{
			"limits": map[string]any{"max": 120},
		},
	}
	got := mergeRuleCfg("line-length", earlier, later)
	limits, ok := got.Settings["limits"].(map[string]any)
	require.True(t, ok, "limits must be a map")
	assert.Equal(t, 120, limits["max"], "later wins inside nested map")
	assert.Equal(t, 10, limits["min"], "earlier sibling inside nested map preserved")
}

func TestMergeRuleCfgListReplacedByDefault(t *testing.T) {
	earlier := RuleCfg{
		Enabled:  true,
		Settings: map[string]any{"exclude": []any{"a", "b"}},
	}
	later := RuleCfg{
		Enabled:  true,
		Settings: map[string]any{"exclude": []any{"c"}},
	}
	got := mergeRuleCfg("line-length", earlier, later)
	assert.Equal(t, []any{"c"}, got.Settings["exclude"], "list defaults to replace")
}

func TestMergeRuleCfgListAppendedWhenRuleOptsIn(t *testing.T) {
	earlier := RuleCfg{
		Enabled:  true,
		Settings: map[string]any{"placeholders": []any{"heading-question"}},
	}
	later := RuleCfg{
		Enabled:  true,
		Settings: map[string]any{"placeholders": []any{"var-token"}},
	}
	got := mergeRuleCfg("first-line-heading", earlier, later)
	assert.Equal(t, []any{"heading-question", "var-token"}, got.Settings["placeholders"],
		"placeholders should concatenate across layers")
}

func TestMergeRuleCfgBoolOnlyLayerPreservesEarlierSettings(t *testing.T) {
	earlier := RuleCfg{
		Enabled:  true,
		Settings: map[string]any{"max": 80},
	}
	later := RuleCfg{Enabled: false} // bool-only disable
	got := mergeRuleCfg("line-length", earlier, later)
	assert.False(t, got.Enabled, "later toggles enabled off")
	assert.Equal(t, 80, got.Settings["max"], "earlier settings preserved when later is bool-only")
}

func TestMergeRuleCfgEarlierEmptyTakesLater(t *testing.T) {
	earlier := RuleCfg{Enabled: true}
	later := RuleCfg{
		Enabled:  true,
		Settings: map[string]any{"max": 80},
	}
	got := mergeRuleCfg("line-length", earlier, later)
	assert.Equal(t, 80, got.Settings["max"])
}

func TestMergeRuleCfgListMixedTypeNormalisedToAny(t *testing.T) {
	// earlier uses []string (from DefaultSettings), later uses []any
	// (from a YAML-decoded layer). Both append cleanly.
	earlier := RuleCfg{
		Enabled:  true,
		Settings: map[string]any{"placeholders": []string{"heading-question"}},
	}
	later := RuleCfg{
		Enabled:  true,
		Settings: map[string]any{"placeholders": []any{"var-token"}},
	}
	got := mergeRuleCfg("first-line-heading", earlier, later)
	assert.Equal(t, []any{"heading-question", "var-token"}, got.Settings["placeholders"])
}

func TestMergeRuleCfgIsolatesNestedMapMutation(t *testing.T) {
	earlier := RuleCfg{
		Enabled: true,
		Settings: map[string]any{
			"limits": map[string]any{"max": 80},
		},
	}
	later := RuleCfg{
		Enabled: true,
		Settings: map[string]any{
			"limits": map[string]any{"min": 10},
		},
	}
	got := mergeRuleCfg("line-length", earlier, later)
	limits := got.Settings["limits"].(map[string]any)
	limits["max"] = 999

	original := earlier.Settings["limits"].(map[string]any)
	assert.Equal(t, 80, original["max"], "mutating result must not mutate earlier source")
}

// --- settingMergeMode fallback paths ---

func TestSettingMergeMode_UnknownRule(t *testing.T) {
	// An unknown rule name → ByName returns nil → default MergeReplace.
	earlier := RuleCfg{
		Enabled:  true,
		Settings: map[string]any{"items": []any{"a", "b"}},
	}
	later := RuleCfg{
		Enabled:  true,
		Settings: map[string]any{"items": []any{"c"}},
	}
	got := mergeRuleCfg("no-such-rule", earlier, later)
	// List should replace (not append) when rule is unknown.
	assert.Equal(t, []any{"c"}, got.Settings["items"])
}

// --- cloneAny branch coverage ---

func TestCloneAny_StringSlice(t *testing.T) {
	original := map[string]any{"tags": []string{"a", "b"}}
	cloned := cloneSettings(original)
	cloned["tags"].([]string)[0] = "z"
	assert.Equal(t, "a", original["tags"].([]string)[0], "cloneAny must deep-copy []string")
}

func TestCloneAny_IntSlice(t *testing.T) {
	original := map[string]any{"counts": []int{1, 2, 3}}
	cloned := cloneSettings(original)
	cloned["counts"].([]int)[0] = 99
	assert.Equal(t, 1, original["counts"].([]int)[0], "cloneAny must deep-copy []int")
}

func TestCloneSettingsNil(t *testing.T) {
	assert.Nil(t, cloneSettings(nil))
}

func TestToAnySlice_IntSlice(t *testing.T) {
	// Exercise the []int case in toAnySlice via mergeAny.
	earlier := RuleCfg{
		Enabled:  true,
		Settings: map[string]any{"nums": []int{1, 2}},
	}
	later := RuleCfg{
		Enabled:  true,
		Settings: map[string]any{"nums": []int{3}},
	}
	got := mergeRuleCfg("some-rule", earlier, later)
	assert.Equal(t, []any{3}, got.Settings["nums"])
}

// --- effectiveRules deep-merge through the layer chain ---

func TestEffectiveTwoKindsContributeDifferentKeys(t *testing.T) {
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
	got := Effective(cfg, "doc.md", []string{"a", "b"})
	rule := got["first-line-heading"]
	assert.Equal(t, 2, rule.Settings["level"],
		"level from kind a should survive deep-merge with kind b")
	assert.Equal(t, []any{"var-token"}, rule.Settings["placeholders"],
		"placeholders from kind b should be present")
}

func TestEffectiveOverridePreservesEarlierSiblings(t *testing.T) {
	cfg := &Config{
		Rules: map[string]RuleCfg{
			"line-length": {Enabled: true, Settings: map[string]any{"max": 80, "tabs": 4}},
		},
		Overrides: []Override{
			{
				Files: []string{"docs/*.md"},
				Rules: map[string]RuleCfg{
					"line-length": {Enabled: true, Settings: map[string]any{"max": 120}},
				},
			},
		},
	}
	got := Effective(cfg, "docs/foo.md", nil)
	rule := got["line-length"]
	assert.Equal(t, 120, rule.Settings["max"], "override updates max")
	assert.Equal(t, 4, rule.Settings["tabs"], "override does not erase sibling tabs")
}

func TestEffectivePlaceholdersAppendAcrossLayers(t *testing.T) {
	cfg := &Config{
		Rules: map[string]RuleCfg{
			"first-line-heading": {Enabled: true, Settings: map[string]any{
				"placeholders": []any{"heading-question"},
			}},
		},
		Kinds: map[string]KindBody{
			"plan": {Rules: map[string]RuleCfg{
				"first-line-heading": {Enabled: true, Settings: map[string]any{
					"placeholders": []any{"var-token"},
				}},
			}},
		},
		Overrides: []Override{
			{
				Files: []string{"plan/*.md"},
				Rules: map[string]RuleCfg{
					"first-line-heading": {Enabled: true, Settings: map[string]any{
						"placeholders": []any{"cue-frontmatter"},
					}},
				},
			},
		},
	}
	got := Effective(cfg, "plan/97.md", []string{"plan"})
	rule := got["first-line-heading"]
	assert.Equal(t,
		[]any{"heading-question", "var-token", "cue-frontmatter"},
		rule.Settings["placeholders"],
		"append-mode list should concatenate defaults, kind, override")
}

func TestEffectiveListReplaceRemainsTheDefault(t *testing.T) {
	// "include" on cross-file-reference-integrity is not opted into
	// MergeAppend, so the override layer should fully replace the
	// inherited list.
	cfg := &Config{
		Rules: map[string]RuleCfg{
			"cross-file-reference-integrity": {
				Enabled:  true,
				Settings: map[string]any{"include": []any{"docs/**"}},
			},
		},
		Overrides: []Override{
			{
				Files: []string{"plan/*.md"},
				Rules: map[string]RuleCfg{
					"cross-file-reference-integrity": {
						Enabled:  true,
						Settings: map[string]any{"include": []any{"plan/**"}},
					},
				},
			},
		},
	}
	got := Effective(cfg, "plan/97.md", nil)
	rule := got["cross-file-reference-integrity"]
	assert.Equal(t, []any{"plan/**"}, rule.Settings["include"],
		"replace-mode list (default) should not concatenate")
}

func TestEffectiveBoolOnlyLayerPreservesInheritedSettings(t *testing.T) {
	// A kind that disables a rule via the bool form should preserve
	// the inherited settings so a later layer can re-enable it without
	// restating the body.
	cfg := &Config{
		Rules: map[string]RuleCfg{
			"line-length": {Enabled: true, Settings: map[string]any{"max": 80}},
		},
		Kinds: map[string]KindBody{
			"off":  {Rules: map[string]RuleCfg{"line-length": {Enabled: false}}},
			"back": {Rules: map[string]RuleCfg{"line-length": {Enabled: true}}},
		},
	}
	got := Effective(cfg, "doc.md", []string{"off", "back"})
	rule := got["line-length"]
	assert.True(t, rule.Enabled, "later kind re-enables")
	assert.Equal(t, 80, rule.Settings["max"], "inherited settings survive bool-only layers")
}

// --- Regression: a layer that fully restates the rule body wins on
// every key, matching the pre-deep-merge behavior for that layer.

func TestEffectiveKindAddsRuleNotInDefaults(t *testing.T) {
	// A kind introduces a rule not present in the top-level defaults at
	// all. The else branch in effectiveRules (copyRuleCfg) must fire.
	cfg := &Config{
		Rules: map[string]RuleCfg{},
		Kinds: map[string]KindBody{
			"plan": {Rules: map[string]RuleCfg{
				"line-length": {Enabled: true, Settings: map[string]any{"max": 80}},
			}},
		},
	}
	got := Effective(cfg, "doc.md", []string{"plan"})
	assert.True(t, got["line-length"].Enabled)
	assert.Equal(t, 80, got["line-length"].Settings["max"])
}

func TestEffectiveFullyRestatedLayerStillWins(t *testing.T) {
	cfg := &Config{
		Rules: map[string]RuleCfg{
			"line-length": {Enabled: true, Settings: map[string]any{"max": 80, "tabs": 4}},
		},
		Overrides: []Override{
			{
				Files: []string{"docs/*.md"},
				Rules: map[string]RuleCfg{
					"line-length": {Enabled: true, Settings: map[string]any{
						"max": 120, "tabs": 2,
					}},
				},
			},
		},
	}
	got := Effective(cfg, "docs/foo.md", nil)
	rule := got["line-length"]
	assert.Equal(t, 120, rule.Settings["max"])
	assert.Equal(t, 2, rule.Settings["tabs"])
}
