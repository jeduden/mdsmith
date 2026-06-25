package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// containsOf reads a rule's resolved "contains" list from effective config.
func containsOf(t *testing.T, eff map[string]RuleCfg, ruleName string) []string {
	t.Helper()
	rc, ok := eff[ruleName]
	require.True(t, ok, "rule %q present in effective config", ruleName)
	_, hasLists := rc.Settings["lists"]
	assert.False(t, hasLists, "lists key must be stripped before rules see settings")
	got, ok := anyToStrings(rc.Settings["contains"])
	require.True(t, ok, "contains must be a string list")
	return got
}

func TestEffective_ListsResolvesBuiltin(t *testing.T) {
	cfg := &Config{
		Rules: map[string]RuleCfg{
			"forbidden-text": {Enabled: true, Settings: map[string]any{"lists": []any{"ai-speak"}}},
		},
		ExplicitRules: map[string]bool{"forbidden-text": true},
	}
	got := containsOf(t, Effective(cfg, "doc.md", nil, nil), "forbidden-text")
	assert.Contains(t, got, "delve")
	assert.Contains(t, got, "honest")
	assert.Contains(t, got, "it's important to note that")
}

func TestEffective_ListsUnionWithInline(t *testing.T) {
	cfg := &Config{
		Rules: map[string]RuleCfg{
			"forbidden-text": {Enabled: true, Settings: map[string]any{
				"lists":    []any{"ai-speak"},
				"contains": []any{"myword"},
			}},
		},
		ExplicitRules: map[string]bool{"forbidden-text": true},
	}
	got := containsOf(t, Effective(cfg, "doc.md", nil, nil), "forbidden-text")
	assert.Contains(t, got, "delve", "from the named list")
	assert.Contains(t, got, "myword", "from the inline contains")
}

func TestEffective_UserListExtendsBuiltin(t *testing.T) {
	cfg := &Config{
		Rules: map[string]RuleCfg{
			"forbidden-text": {Enabled: true, Settings: map[string]any{"lists": []any{"team"}}},
		},
		ExplicitRules: map[string]bool{"forbidden-text": true},
		Wordlists: map[string]UserWordlist{
			"team": {Extends: "ai-speak", Entries: []string{"synergy"}},
		},
	}
	got := containsOf(t, Effective(cfg, "doc.md", nil, nil), "forbidden-text")
	assert.Contains(t, got, "delve", "inherited from ai-speak")
	assert.Contains(t, got, "synergy", "own entry")
}

func TestEffective_ConventionListsAndUserListsAppend(t *testing.T) {
	// Convention preset names ai-speak; the user's explicit rule adds
	// its own list. Both should resolve into contains.
	cfg := &Config{
		ConventionPreset: map[string]RuleCfg{
			"forbidden-text": {Enabled: true, Settings: map[string]any{"lists": []any{"ai-speak"}}},
		},
		Rules: map[string]RuleCfg{
			"forbidden-text": {Enabled: true, Settings: map[string]any{"lists": []any{"team"}}},
		},
		ExplicitRules: map[string]bool{"forbidden-text": true},
		Wordlists: map[string]UserWordlist{
			"team": {Entries: []string{"synergy"}},
		},
	}
	got := containsOf(t, Effective(cfg, "doc.md", nil, nil), "forbidden-text")
	assert.Contains(t, got, "delve", "from the convention's ai-speak")
	assert.Contains(t, got, "synergy", "from the user's team list")
}

func TestValidateWordlists_UnknownList(t *testing.T) {
	cfg := &Config{Rules: map[string]RuleCfg{
		"forbidden-text": {Enabled: true, Settings: map[string]any{"lists": []any{"ghost"}}},
	}}
	err := validateWordlists(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ghost")
}

func TestValidateWordlists_NonConsumerRule(t *testing.T) {
	cfg := &Config{Rules: map[string]RuleCfg{
		"line-length": {Enabled: true, Settings: map[string]any{"lists": []any{"ai-speak"}}},
	}}
	err := validateWordlists(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not accept lists")
}

func TestValidateWordlists_Cycle(t *testing.T) {
	cfg := &Config{
		Rules: map[string]RuleCfg{
			"forbidden-text": {Enabled: true, Settings: map[string]any{"lists": []any{"a"}}},
		},
		Wordlists: map[string]UserWordlist{
			"a": {Extends: "b", Entries: []string{"x"}},
			"b": {Extends: "a", Entries: []string{"y"}},
		},
	}
	err := validateWordlists(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cycle")
}

func TestValidateWordlists_OK(t *testing.T) {
	cfg := &Config{
		Rules: map[string]RuleCfg{
			"forbidden-text": {Enabled: true, Settings: map[string]any{"lists": []any{"team"}}},
		},
		Wordlists: map[string]UserWordlist{
			"team": {Extends: "ai-speak", Entries: []string{"synergy"}},
		},
	}
	require.NoError(t, validateWordlists(cfg))
}
