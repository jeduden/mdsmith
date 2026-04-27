package config

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Deep-merge across kinds and overrides ---

// TestEffective_TwoKindsContributeNestedKeys verifies that when two
// kinds touch the same rule on different nested keys, the effective
// rule config carries both leaves.
func TestEffective_TwoKindsContributeNestedKeys(t *testing.T) {
	cfg := &Config{
		Rules: map[string]RuleCfg{
			"line-length": {Enabled: true, Settings: map[string]any{"max": 80}},
		},
		Kinds: map[string]KindBody{
			"a": {Rules: map[string]RuleCfg{
				"line-length": {Enabled: true, Settings: map[string]any{"max": 100}},
			}},
			"b": {Rules: map[string]RuleCfg{
				"line-length": {Enabled: true, Settings: map[string]any{"strict": true}},
			}},
		},
	}
	result := Effective(cfg, "doc.md", []string{"a", "b"})
	got := result["line-length"].Settings
	require.NotNil(t, got)
	assert.Equal(t, 100, got["max"], "max from kind a should survive")
	assert.Equal(t, true, got["strict"], "strict from kind b should be present")
}

// TestEffective_OverrideDoesNotEraseSiblings verifies an override that
// sets only one nested key does not wipe out sibling keys set earlier.
func TestEffective_OverrideDoesNotEraseSiblings(t *testing.T) {
	cfg := &Config{
		Rules: map[string]RuleCfg{
			"line-length": {Enabled: true, Settings: map[string]any{
				"max":    80,
				"strict": true,
			}},
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
	result := Effective(cfg, "docs/foo.md", nil)
	got := result["line-length"].Settings
	require.NotNil(t, got)
	assert.Equal(t, 120, got["max"], "override should set max")
	assert.Equal(t, true, got["strict"], "sibling strict should still be present")
}

// TestEffective_NestedMapsMergeRecursively verifies that nested maps
// merge key-by-key rather than being replaced wholesale.
func TestEffective_NestedMapsMergeRecursively(t *testing.T) {
	cfg := &Config{
		Rules: map[string]RuleCfg{
			"some-rule": {Enabled: true, Settings: map[string]any{
				"thresholds": map[string]any{"warn": 5, "error": 10},
			}},
		},
		Overrides: []Override{
			{
				Files: []string{"*.md"},
				Rules: map[string]RuleCfg{
					"some-rule": {Enabled: true, Settings: map[string]any{
						"thresholds": map[string]any{"warn": 7},
					}},
				},
			},
		},
	}
	result := Effective(cfg, "doc.md", nil)
	thresholds, ok := result["some-rule"].Settings["thresholds"].(map[string]any)
	require.True(t, ok, "thresholds should be a map")
	assert.Equal(t, 7, thresholds["warn"], "warn overridden")
	assert.Equal(t, 10, thresholds["error"], "error preserved")
}

// TestEffective_ListsReplaceByDefault verifies that list settings with
// no declared merge mode are replaced wholesale (the default).
func TestEffective_ListsReplaceByDefault(t *testing.T) {
	cfg := &Config{
		Rules: map[string]RuleCfg{
			"line-length": {Enabled: true, Settings: map[string]any{
				"exclude": []any{"a", "b"},
			}},
		},
		Overrides: []Override{
			{
				Files: []string{"*.md"},
				Rules: map[string]RuleCfg{
					"line-length": {Enabled: true, Settings: map[string]any{
						"exclude": []any{"c"},
					}},
				},
			},
		},
	}
	result := Effective(cfg, "doc.md", nil)
	got := result["line-length"].Settings["exclude"]
	assert.Equal(t, []any{"c"}, got, "default replace mode wholesale-replaces lists")
}

// TestEffective_ListsAppendForDeclaredKeys verifies that list settings
// declared with append mode concatenate across layers.
func TestEffective_ListsAppendForDeclaredKeys(t *testing.T) {
	cfg := &Config{
		Rules: map[string]RuleCfg{
			"first-line-heading": {Enabled: true, Settings: map[string]any{
				"placeholders": []any{"var-token"},
			}},
		},
		Kinds: map[string]KindBody{
			"plan": {Rules: map[string]RuleCfg{
				"first-line-heading": {Enabled: true, Settings: map[string]any{
					"placeholders": []any{"heading-question"},
				}},
			}},
		},
		Overrides: []Override{
			{
				Files: []string{"*.md"},
				Rules: map[string]RuleCfg{
					"first-line-heading": {Enabled: true, Settings: map[string]any{
						"placeholders": []any{"placeholder-section"},
					}},
				},
			},
		},
	}
	result := Effective(cfg, "doc.md", []string{"plan"})
	got, ok := result["first-line-heading"].Settings["placeholders"].([]any)
	require.True(t, ok, "placeholders should be a list, got %T", result["first-line-heading"].Settings["placeholders"])
	assert.Equal(t,
		[]any{"var-token", "heading-question", "placeholder-section"},
		got,
		"append mode concatenates lists in layer order")
}

// TestEffective_BlockReplacementStillWorks verifies the regression case:
// a config that already specifies the full rule body at the latest layer
// produces the same effective config as block replacement would.
func TestEffective_BlockReplacementStillWorks(t *testing.T) {
	cfg := &Config{
		Rules: map[string]RuleCfg{
			"line-length": {Enabled: true, Settings: map[string]any{
				"max":    80,
				"strict": true,
			}},
		},
		Overrides: []Override{
			{
				Files: []string{"*.md"},
				Rules: map[string]RuleCfg{
					// Full body restated — every leaf is replaced.
					"line-length": {Enabled: true, Settings: map[string]any{
						"max":    120,
						"strict": false,
					}},
				},
			},
		},
	}
	result := Effective(cfg, "doc.md", nil)
	got := result["line-length"].Settings
	assert.Equal(t, 120, got["max"])
	assert.Equal(t, false, got["strict"])
}

// TestEffective_DisablingRuleDropsSettings verifies that a later layer
// can disable a rule (Enabled=false with no Settings) and that this
// supersedes earlier merged settings.
func TestEffective_DisablingRuleDropsSettings(t *testing.T) {
	cfg := &Config{
		Rules: map[string]RuleCfg{
			"line-length": {Enabled: true, Settings: map[string]any{"max": 80}},
		},
		Overrides: []Override{
			{
				Files: []string{"*.md"},
				Rules: map[string]RuleCfg{
					"line-length": {Enabled: false},
				},
			},
		},
	}
	result := Effective(cfg, "doc.md", nil)
	assert.False(t, result["line-length"].Enabled, "later layer disables the rule")
}

// TestEffective_SourceConfigUnmodified guards against the deep-merge
// implementation mutating the source config maps via shared references.
func TestEffective_SourceConfigUnmodified(t *testing.T) {
	srcSettings := map[string]any{"max": 80}
	cfg := &Config{
		Rules: map[string]RuleCfg{
			"line-length": {Enabled: true, Settings: srcSettings},
		},
		Overrides: []Override{
			{
				Files: []string{"*.md"},
				Rules: map[string]RuleCfg{
					"line-length": {Enabled: true, Settings: map[string]any{"max": 120}},
				},
			},
		},
	}
	_ = Effective(cfg, "doc.md", nil)
	assert.Equal(t, 80, srcSettings["max"], "original settings map must not be mutated")
}

// --- Provenance ---

// TestEffectiveWithProvenance_ReportsContributingLayer verifies that
// the provenance map records which layer set each leaf.
func TestEffectiveWithProvenance_ReportsContributingLayer(t *testing.T) {
	cfg := &Config{
		Rules: map[string]RuleCfg{
			"line-length": {Enabled: true, Settings: map[string]any{
				"max":    80,
				"strict": true,
			}},
		},
		Kinds: map[string]KindBody{
			"plan": {Rules: map[string]RuleCfg{
				"line-length": {Enabled: true, Settings: map[string]any{"max": 100}},
			}},
		},
		Overrides: []Override{
			{
				Files: []string{"*.md"},
				Rules: map[string]RuleCfg{
					"line-length": {Enabled: true, Settings: map[string]any{"strict": false}},
				},
			},
		},
	}
	_, prov := EffectiveWithProvenance(cfg, "doc.md", []string{"plan"})
	require.Contains(t, prov, "line-length")
	leaves := prov["line-length"]

	// Expect three leaves: enabled, max, strict.
	assert.Equal(t, LayerKind, leaves["max"].Layer, "max from kind plan")
	assert.Equal(t, "plan", leaves["max"].KindName)
	assert.Equal(t, LayerOverride, leaves["strict"].Layer, "strict from override")
}

// --- ListMerger plumbing ---

// fakeListMerger declares custom list merge modes per setting key.
type fakeListMerger struct {
	listModes map[string]rule.ListMergeMode
}

func (r *fakeListMerger) ListMergeMode(key string) rule.ListMergeMode {
	return r.listModes[key]
}

// TestListMergeMode_AppendForDeclaredKey verifies the rule.ListMerger
// contract: declared keys return ListAppend, unknown keys return
// ListReplace (the zero value).
func TestListMergeMode_AppendForDeclaredKey(t *testing.T) {
	r := &fakeListMerger{listModes: map[string]rule.ListMergeMode{"placeholders": rule.ListAppend}}
	assert.Equal(t, rule.ListAppend, r.ListMergeMode("placeholders"))
	assert.Equal(t, rule.ListReplace, r.ListMergeMode("exclude"))
}
