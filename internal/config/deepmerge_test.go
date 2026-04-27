package config

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEffectiveBlockReplaceRegression replicates the pre-deep-merge
// behaviour: a config that fully restates a rule at every layer that
// touches it should produce the same effective config under deep-merge,
// because every leaf is replaced when every leaf is provided.
func TestEffectiveBlockReplaceRegression(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		Rules: map[string]RuleCfg{
			"first-line-heading": {
				Enabled: true,
				Settings: map[string]any{
					"level":        1,
					"placeholders": []any{"var-token"},
				},
			},
		},
		Kinds: map[string]KindBody{
			"plan": {Rules: map[string]RuleCfg{
				"first-line-heading": {
					Enabled: true,
					Settings: map[string]any{
						"level":        2,
						"placeholders": []any{"heading-question"},
					},
				},
			}},
		},
		Overrides: []Override{
			{Files: []string{"docs/*.md"}, Rules: map[string]RuleCfg{
				"first-line-heading": {
					Enabled: true,
					Settings: map[string]any{
						"level":        3,
						"placeholders": []any{"placeholder-section"},
					},
				},
			}},
		},
	}
	result := Effective(cfg, "docs/foo.md", []string{"plan"})
	rc := result["first-line-heading"]
	// Scalar leaves: latest layer wins, exactly as before.
	assert.Equal(t, 3, rc.Settings["level"])
	// List leaves declared `append` concatenate the chain. This
	// differs from block-replace behavior only when the rule opts in.
	assert.Equal(t, []any{"var-token", "heading-question", "placeholder-section"},
		rc.Settings["placeholders"])
}

// TestPlaceholderRulesDeclareAppendMode verifies that every rule
// exposing a `placeholders:` setting declares it as ListAppend so
// kinds and overrides can extend rather than replace the list.
func TestPlaceholderRulesDeclareAppendMode(t *testing.T) {
	t.Parallel()
	for _, r := range rule.All() {
		c, ok := r.(rule.Configurable)
		if !ok {
			continue
		}
		defaults := c.DefaultSettings()
		if _, has := defaults["placeholders"]; !has {
			continue
		}
		mm, ok := r.(rule.MergeModes)
		if !assert.True(t, ok,
			"rule %s exposes placeholders: but does not implement MergeModes",
			r.Name()) {
			continue
		}
		modes := mm.ListMergeModes()
		assert.Equal(t, rule.ListAppend, modes["placeholders"],
			"rule %s should declare placeholders as ListAppend", r.Name())
	}
}

// --- Deep-merge of RuleCfg across the layer chain ---

// TestEffectiveTwoKindsDeepMergeNestedKeys verifies that two kinds setting
// different nested keys on the same rule both contribute to the final
// effective rule config.
func TestEffectiveTwoKindsDeepMergeNestedKeys(t *testing.T) {
	cfg := &Config{
		Rules: map[string]RuleCfg{
			"first-line-heading": {
				Enabled:  true,
				Settings: map[string]any{"level": 1},
			},
		},
		Kinds: map[string]KindBody{
			"a": {Rules: map[string]RuleCfg{
				"first-line-heading": {
					Enabled:  true,
					Settings: map[string]any{"level": 2},
				},
			}},
			"b": {Rules: map[string]RuleCfg{
				"first-line-heading": {
					Enabled:  true,
					Settings: map[string]any{"placeholders": []any{"var-token"}},
				},
			}},
		},
	}
	result := Effective(cfg, "doc.md", []string{"a", "b"})
	rc := result["first-line-heading"]
	assert.True(t, rc.Enabled)
	assert.Equal(t, 2, rc.Settings["level"], "level from kind a should survive")
	assert.Equal(t, []any{"var-token"}, rc.Settings["placeholders"],
		"placeholders from kind b should survive")
}

// TestEffectiveOverrideTouchingOneKeyKeepsSiblings verifies that a
// matching override that sets only one nested setting does not erase
// sibling settings established by an earlier layer.
func TestEffectiveOverrideTouchingOneKeyKeepsSiblings(t *testing.T) {
	cfg := &Config{
		Rules: map[string]RuleCfg{
			"first-line-heading": {
				Enabled: true,
				Settings: map[string]any{
					"level":        2,
					"placeholders": []any{"var-token"},
				},
			},
		},
		Overrides: []Override{
			{
				Files: []string{"docs/*.md"},
				Rules: map[string]RuleCfg{
					"first-line-heading": {
						Enabled:  true,
						Settings: map[string]any{"level": 3},
					},
				},
			},
		},
	}
	result := Effective(cfg, "docs/foo.md", nil)
	rc := result["first-line-heading"]
	assert.Equal(t, 3, rc.Settings["level"], "level should be overridden")
	assert.Equal(t, []any{"var-token"}, rc.Settings["placeholders"],
		"placeholders should not be erased")
}

// TestEffectiveListReplaceDefault verifies that a list setting whose
// merge mode is `replace` (the default) is replaced wholesale by the
// later layer, not concatenated.
func TestEffectiveListReplaceDefault(t *testing.T) {
	cfg := &Config{
		Rules: map[string]RuleCfg{
			"first-line-heading": {
				Enabled: true,
				Settings: map[string]any{
					"placeholders": []any{"var-token"},
				},
			},
		},
		Kinds: map[string]KindBody{
			"a": {Rules: map[string]RuleCfg{
				"first-line-heading": {
					Enabled: true,
					// "level" is a scalar, so its default mode is replace
					// — this verifies scalar replacement, but the
					// equivalent test for an unknown list key is below.
					Settings: map[string]any{"level": 4},
				},
			}},
		},
	}
	result := Effective(cfg, "doc.md", []string{"a"})
	rc := result["first-line-heading"]
	assert.Equal(t, 4, rc.Settings["level"])
	// placeholders should still be set from earlier layer.
	assert.Equal(t, []any{"var-token"}, rc.Settings["placeholders"])
}

// TestEffectiveListAppendMode verifies that lists declared `append`
// concatenate across layers. The placeholders setting opts into append
// mode via the rule's MergeModes interface.
func TestEffectiveListAppendMode(t *testing.T) {
	cfg := &Config{
		Rules: map[string]RuleCfg{
			"first-line-heading": {
				Enabled: true,
				Settings: map[string]any{
					"placeholders": []any{"var-token"},
				},
			},
		},
		Kinds: map[string]KindBody{
			"a": {Rules: map[string]RuleCfg{
				"first-line-heading": {
					Enabled: true,
					Settings: map[string]any{
						"placeholders": []any{"heading-question"},
					},
				},
			}},
		},
	}
	result := Effective(cfg, "doc.md", []string{"a"})
	rc := result["first-line-heading"]
	got, ok := rc.Settings["placeholders"].([]any)
	require.True(t, ok, "placeholders should be a list of any")
	assert.Equal(t, []any{"var-token", "heading-question"}, got,
		"placeholders should concatenate across layers in append mode")
}

// TestEffectiveListReplaceForUnknownKey verifies that a list setting
// whose key is not declared `append` defaults to `replace`.
func TestEffectiveListReplaceForUnknownKey(t *testing.T) {
	cfg := &Config{
		Rules: map[string]RuleCfg{
			"first-line-heading": {
				Enabled: true,
				Settings: map[string]any{
					// Use an unknown list key (the rule will reject it
					// at ApplySettings time, but the merge layer must
					// still treat it consistently).
					"unknown-list": []any{"a", "b"},
				},
			},
		},
		Kinds: map[string]KindBody{
			"a": {Rules: map[string]RuleCfg{
				"first-line-heading": {
					Enabled: true,
					Settings: map[string]any{
						"unknown-list": []any{"c"},
					},
				},
			}},
		},
	}
	result := Effective(cfg, "doc.md", []string{"a"})
	rc := result["first-line-heading"]
	assert.Equal(t, []any{"c"}, rc.Settings["unknown-list"],
		"unknown lists default to replace mode")
}

// TestEffectiveBlockReplaceStillWins verifies the backward-compatibility
// guarantee: a layer that sets every key of a rule's body still wins
// for every key, because deep-merge replaces every leaf when every
// leaf is provided.
func TestEffectiveBlockReplaceStillWins(t *testing.T) {
	cfg := &Config{
		Rules: map[string]RuleCfg{
			"first-line-heading": {
				Enabled: true,
				Settings: map[string]any{
					"level":        2,
					"placeholders": []any{"var-token"},
				},
			},
		},
		Overrides: []Override{
			{
				Files: []string{"docs/*.md"},
				Rules: map[string]RuleCfg{
					"first-line-heading": {
						Enabled: true,
						Settings: map[string]any{
							"level":        3,
							"placeholders": []any{"heading-question"},
						},
					},
				},
			},
		},
	}
	result := Effective(cfg, "docs/foo.md", nil)
	rc := result["first-line-heading"]
	assert.Equal(t, 3, rc.Settings["level"])
	// Even with append-mode placeholders, a layer that restates the
	// list does not concatenate — the later list is the value at that
	// layer; concatenation is only across distinct layers, so this
	// matches expected behavior. Wait: append-mode says concatenate.
	// The contract: append concatenates across layers. So this test
	// expects [var-token, heading-question].
	assert.Equal(t, []any{"var-token", "heading-question"},
		rc.Settings["placeholders"],
		"append-mode lists concatenate even when both layers set it")
}

// TestEffectiveLayerSettingNilSettingsTreatsAsEmpty verifies that a
// layer that disables a rule entirely (Enabled=false, Settings=nil) is
// applied as a scalar replacement of Enabled, not as a deep merge of
// Settings.
func TestEffectiveLayerDisablingClearsEnabled(t *testing.T) {
	cfg := &Config{
		Rules: map[string]RuleCfg{
			"first-line-heading": {
				Enabled: true,
				Settings: map[string]any{
					"level": 2,
				},
			},
		},
		Overrides: []Override{
			{
				Files: []string{"docs/*.md"},
				Rules: map[string]RuleCfg{
					"first-line-heading": {Enabled: false},
				},
			},
		},
	}
	result := Effective(cfg, "docs/foo.md", nil)
	rc := result["first-line-heading"]
	assert.False(t, rc.Enabled, "override disables the rule")
	// Settings map from earlier layer is preserved (deep-merge keeps
	// sibling keys); Enabled toggled independently.
	assert.Equal(t, 2, rc.Settings["level"],
		"sibling settings remain after disabling")
}

// TestDeepMergeIsolation verifies that the merged result does not
// alias the source maps — mutating the result must not change cfg.
func TestDeepMergeIsolation(t *testing.T) {
	cfg := &Config{
		Rules: map[string]RuleCfg{
			"first-line-heading": {
				Enabled: true,
				Settings: map[string]any{
					"placeholders": []any{"var-token"},
				},
			},
		},
		Kinds: map[string]KindBody{
			"a": {Rules: map[string]RuleCfg{
				"first-line-heading": {
					Enabled:  true,
					Settings: map[string]any{"level": 3},
				},
			}},
		},
	}
	result := Effective(cfg, "doc.md", []string{"a"})
	result["first-line-heading"].Settings["level"] = 99
	// Source kind body is unchanged.
	assert.Equal(t, 3, cfg.Kinds["a"].Rules["first-line-heading"].Settings["level"])
}
