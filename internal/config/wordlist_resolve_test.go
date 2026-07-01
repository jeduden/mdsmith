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

func TestEffective_ListsResolvesUserList(t *testing.T) {
	cfg := &Config{
		Rules: map[string]RuleCfg{
			"forbidden-text": {Enabled: true, Settings: map[string]any{"lists": []any{"house"}}},
		},
		ExplicitRules: map[string]bool{"forbidden-text": true},
		Wordlists: map[string]UserWordlist{
			"house": {Entries: []string{"delve", "it's important to note that"}},
		},
	}
	got := containsOf(t, Effective(cfg, "doc.md", nil, nil), "forbidden-text")
	assert.Contains(t, got, "delve")
	assert.Contains(t, got, "it's important to note that")
}

func TestEffective_ListsUnionWithInline(t *testing.T) {
	cfg := &Config{
		Rules: map[string]RuleCfg{
			"forbidden-text": {Enabled: true, Settings: map[string]any{
				"lists":    []any{"house"},
				"contains": []any{"myword"},
			}},
		},
		ExplicitRules: map[string]bool{"forbidden-text": true},
		Wordlists: map[string]UserWordlist{
			"house": {Entries: []string{"delve"}},
		},
	}
	got := containsOf(t, Effective(cfg, "doc.md", nil, nil), "forbidden-text")
	assert.Contains(t, got, "delve", "from the named list")
	assert.Contains(t, got, "myword", "from the inline contains")
}

func TestEffective_UserListExtendsUserList(t *testing.T) {
	cfg := &Config{
		Rules: map[string]RuleCfg{
			"forbidden-text": {Enabled: true, Settings: map[string]any{"lists": []any{"team"}}},
		},
		ExplicitRules: map[string]bool{"forbidden-text": true},
		Wordlists: map[string]UserWordlist{
			"base": {Entries: []string{"delve"}},
			"team": {Extends: "base", Entries: []string{"synergy"}},
		},
	}
	got := containsOf(t, Effective(cfg, "doc.md", nil, nil), "forbidden-text")
	assert.Contains(t, got, "delve", "inherited from base")
	assert.Contains(t, got, "synergy", "own entry")
}

func TestEffective_ConventionListsAndUserListsAppend(t *testing.T) {
	// Convention preset names one list; the user's explicit rule adds
	// another. Both should resolve into contains.
	cfg := &Config{
		ConventionPreset: map[string]RuleCfg{
			"forbidden-text": {Enabled: true, Settings: map[string]any{"lists": []any{"house"}}},
		},
		Rules: map[string]RuleCfg{
			"forbidden-text": {Enabled: true, Settings: map[string]any{"lists": []any{"team"}}},
		},
		ExplicitRules: map[string]bool{"forbidden-text": true},
		Wordlists: map[string]UserWordlist{
			"house": {Entries: []string{"delve"}},
			"team":  {Entries: []string{"synergy"}},
		},
	}
	got := containsOf(t, Effective(cfg, "doc.md", nil, nil), "forbidden-text")
	assert.Contains(t, got, "delve", "from the convention's list")
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
		"line-length": {Enabled: true, Settings: map[string]any{"lists": []any{"team"}}},
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
			"base": {Entries: []string{"delve"}},
			"team": {Extends: "base", Entries: []string{"synergy"}},
		},
	}
	require.NoError(t, validateWordlists(cfg))
}
