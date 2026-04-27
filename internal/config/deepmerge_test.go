package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Deep-merge primitive: deepMergeRule ---

func TestDeepMergeRule_ScalarReplaced(t *testing.T) {
	base := RuleCfg{Enabled: true, Settings: map[string]any{"max": 80}}
	over := RuleCfg{Enabled: true, Settings: map[string]any{"max": 200}}
	got := deepMergeRule(base, over, nil)
	assert.Equal(t, 200, got.Settings["max"])
}

func TestDeepMergeRule_PreservesUntouchedSiblings(t *testing.T) {
	base := RuleCfg{Enabled: true, Settings: map[string]any{
		"level":        1,
		"placeholders": []any{"heading-question"},
	}}
	// Later layer only touches "level"; "placeholders" must survive.
	over := RuleCfg{Enabled: true, Settings: map[string]any{
		"level": 2,
	}}
	got := deepMergeRule(base, over, nil)
	assert.Equal(t, 2, got.Settings["level"])
	assert.Equal(t, []any{"heading-question"}, got.Settings["placeholders"],
		"sibling key must survive a partial override")
}

func TestDeepMergeRule_NestedMapsRecursed(t *testing.T) {
	base := RuleCfg{Enabled: true, Settings: map[string]any{
		"nested": map[string]any{
			"a": 1,
			"b": 2,
		},
	}}
	over := RuleCfg{Enabled: true, Settings: map[string]any{
		"nested": map[string]any{
			"b": 20,
			"c": 30,
		},
	}}
	got := deepMergeRule(base, over, nil)
	nested, ok := got.Settings["nested"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, 1, nested["a"], "untouched nested key survives")
	assert.Equal(t, 20, nested["b"], "later nested key wins")
	assert.Equal(t, 30, nested["c"], "new nested key added")
}

func TestDeepMergeRule_ListReplaceByDefault(t *testing.T) {
	base := RuleCfg{Enabled: true, Settings: map[string]any{
		"exclude": []any{"a", "b"},
	}}
	over := RuleCfg{Enabled: true, Settings: map[string]any{
		"exclude": []any{"c"},
	}}
	got := deepMergeRule(base, over, nil)
	assert.Equal(t, []any{"c"}, got.Settings["exclude"],
		"lists default to replace mode")
}

func TestDeepMergeRule_ListAppendOptIn(t *testing.T) {
	base := RuleCfg{Enabled: true, Settings: map[string]any{
		"placeholders": []any{"heading-question"},
	}}
	over := RuleCfg{Enabled: true, Settings: map[string]any{
		"placeholders": []any{"var-token"},
	}}
	modes := map[string]MergeMode{"placeholders": MergeAppend}
	got := deepMergeRule(base, over, modes)
	assert.Equal(t, []any{"heading-question", "var-token"}, got.Settings["placeholders"],
		"lists declared append concatenate across layers")
}

func TestDeepMergeRule_DisabledLayerWinsAndKeepsSettings(t *testing.T) {
	// When a later layer disables the rule (Enabled=false, no Settings),
	// it should win on Enabled but earlier Settings can still merge.
	// Today block-replacement makes Settings nil. Under deep-merge a layer
	// that does not redeclare a key leaves it alone. We treat
	// Enabled=false with nil Settings as: replace Enabled, leave Settings.
	base := RuleCfg{Enabled: true, Settings: map[string]any{"max": 80}}
	over := RuleCfg{Enabled: false}
	got := deepMergeRule(base, over, nil)
	assert.False(t, got.Enabled, "later layer disables the rule")
	assert.Equal(t, 80, got.Settings["max"],
		"earlier layer's settings survive when later layer omits Settings")
}

// --- Effective(): kinds and overrides deep-merge ---

func TestEffective_TwoKindsContributeDifferentNestedKeys(t *testing.T) {
	cfg := &Config{
		Rules: map[string]RuleCfg{
			"first-line-heading": {Enabled: true, Settings: map[string]any{
				"level":        1,
				"placeholders": []any{},
			}},
		},
		Kinds: map[string]KindBody{
			"a": {Rules: map[string]RuleCfg{
				"first-line-heading": {Enabled: true, Settings: map[string]any{
					"level": 2,
				}},
			}},
			"b": {Rules: map[string]RuleCfg{
				"first-line-heading": {Enabled: true, Settings: map[string]any{
					"placeholders": []any{"heading-question"},
				}},
			}},
		},
	}
	got := Effective(cfg, "doc.md", []string{"a", "b"})
	rc := got["first-line-heading"]
	assert.Equal(t, 2, rc.Settings["level"], "kind a contributed level")
	// Without merge mode info Effective should still preserve the
	// placeholders contribution from kind b.
	assert.NotNil(t, rc.Settings["placeholders"],
		"kind b contributed placeholders without erasing level")
}

func TestEffective_OverrideOnlyOneKey(t *testing.T) {
	cfg := &Config{
		Rules: map[string]RuleCfg{
			"first-line-heading": {Enabled: true, Settings: map[string]any{
				"level":        1,
				"placeholders": []any{"heading-question"},
			}},
		},
		Overrides: []Override{
			{
				Files: []string{"docs/*.md"},
				Rules: map[string]RuleCfg{
					"first-line-heading": {Enabled: true, Settings: map[string]any{
						"level": 3,
					}},
				},
			},
		},
	}
	got := Effective(cfg, "docs/foo.md", nil)
	rc := got["first-line-heading"]
	assert.Equal(t, 3, rc.Settings["level"], "override changed level")
	assert.Equal(t, []any{"heading-question"}, rc.Settings["placeholders"],
		"sibling 'placeholders' must not be erased")
}

func TestEffective_KindOverrideContributesAppendList(t *testing.T) {
	// Use a real rule name to exercise the merge-mode lookup.
	// "first-line-heading" declares placeholders as append.
	cfg := &Config{
		Rules: map[string]RuleCfg{
			"first-line-heading": {Enabled: true, Settings: map[string]any{
				"level":        1,
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
	}
	got := Effective(cfg, "doc.md", []string{"plan"})
	rc := got["first-line-heading"]
	ph, ok := rc.Settings["placeholders"].([]any)
	require.True(t, ok)
	assert.Equal(t, []any{"heading-question", "var-token"}, ph,
		"placeholders list must concatenate across layers")
}

func TestEffective_BlockReplacementStillWorks(t *testing.T) {
	// A layer that restates the whole rule body should still override
	// every leaf, the same as the pre-deep-merge behavior.
	cfg := &Config{
		Rules: map[string]RuleCfg{
			"line-length": {Enabled: true, Settings: map[string]any{"max": 80}},
		},
		Overrides: []Override{
			{
				Files: []string{"wide/*.md"},
				Rules: map[string]RuleCfg{
					"line-length": {Enabled: true, Settings: map[string]any{
						"max": 200,
					}},
				},
			},
		},
	}
	got := Effective(cfg, "wide/foo.md", nil)
	assert.Equal(t, 200, got["line-length"].Settings["max"])
}
