package markdownflavor

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLookup_Portable(t *testing.T) {
	c, err := Lookup("portable", nil)
	require.NoError(t, err)
	assert.Equal(t, "portable", c.Name)
	assert.Equal(t, FlavorCommonMark, c.Flavor)

	mf, ok := c.Rules["markdown-flavor"]
	require.True(t, ok)
	assert.True(t, mf.Enabled)
	assert.Equal(t, "commonmark", mf.Settings["flavor"])

	hr, ok := c.Rules["horizontal-rule-style"]
	require.True(t, ok)
	assert.Equal(t, "dash", hr.Settings["style"])
	assert.Equal(t, 3, hr.Settings["length"])
	assert.Equal(t, true, hr.Settings["require-blank-lines"])
}

func TestLookup_Github(t *testing.T) {
	c, err := Lookup("github", nil)
	require.NoError(t, err)
	assert.Equal(t, FlavorGFM, c.Flavor)

	html, ok := c.Rules["no-inline-html"]
	require.True(t, ok)
	assert.Equal(t, []any{"details", "summary"}, html.Settings["allow"])

	// github convention leaves the strict rules off; horizontal-rule-style
	// should not be in the github preset.
	_, hasHR := c.Rules["horizontal-rule-style"]
	assert.False(t, hasHR, "github convention does not enable horizontal-rule-style")
}

func TestLookup_Plain(t *testing.T) {
	c, err := Lookup("plain", nil)
	require.NoError(t, err)
	assert.Equal(t, FlavorCommonMark, c.Flavor)

	html, ok := c.Rules["no-inline-html"]
	require.True(t, ok)
	assert.Equal(t, false, html.Settings["allow-comments"],
		"plain convention forbids HTML comments")
}

func TestLookup_Unknown(t *testing.T) {
	_, err := Lookup("bogus", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown convention")
	assert.Contains(t, err.Error(), "bogus")
	assert.Contains(t, err.Error(), "github")
	assert.Contains(t, err.Error(), "plain")
	assert.Contains(t, err.Error(), "portable")
}

func TestCloneValue_TypedSlices(t *testing.T) {
	// cloneValue must handle slices typed concretely as []string,
	// []int, and []bool (a contributor adding a preset directly in
	// Go code might use any of these). The bug we are guarding
	// against: the default branch returned the original slice, so a
	// caller mutating the clone could rewrite the package-level
	// convention table.
	src := map[string]any{
		"strs":  []string{"a", "b"},
		"ints":  []int{1, 2},
		"bools": []bool{true, false},
	}
	got := cloneAny(src)

	got["strs"].([]string)[0] = "tampered"
	got["ints"].([]int)[0] = 99
	got["bools"].([]bool)[0] = false

	assert.Equal(t, "a", src["strs"].([]string)[0],
		"[]string must be deep-copied")
	assert.Equal(t, 1, src["ints"].([]int)[0],
		"[]int must be deep-copied")
	assert.Equal(t, true, src["bools"].([]bool)[0],
		"[]bool must be deep-copied")
}

func TestCloneValue_NestedMapsAndSlices(t *testing.T) {
	// cloneValue handles three shapes: nested maps, slices, and
	// scalars. The built-in convention table happens not to contain
	// nested maps, so exercise that branch directly.
	src := map[string]any{
		"nested": map[string]any{
			"deep":  "v",
			"inner": []any{"a", "b"},
		},
		"list":   []any{1, 2, 3},
		"scalar": "ok",
	}
	got := cloneAny(src)

	// Mutating the clone must not bleed back into the source.
	got["nested"].(map[string]any)["deep"] = "tampered"
	got["list"].([]any)[0] = 99

	assert.Equal(t, "v", src["nested"].(map[string]any)["deep"],
		"nested map must be deep-copied")
	assert.Equal(t, 1, src["list"].([]any)[0],
		"slice must be deep-copied")
}

func TestCloneAny_NilReturnsNil(t *testing.T) {
	assert.Nil(t, cloneAny(nil))
}

func TestLookup_ReturnsDeepCopy(t *testing.T) {
	// Mutating the returned Convention must not corrupt the
	// package-level table. Lookup is exported, so callers could
	// otherwise rewrite the built-ins by accident.
	first, err := Lookup("portable", nil)
	require.NoError(t, err)
	first.Rules["markdown-flavor"].Settings["flavor"] = "tampered"
	first.Rules["new-rule"] = RulePreset{Enabled: true}

	if allow, ok := first.Rules["no-inline-html"]; ok && allow.Settings != nil {
		allow.Settings["allow"] = []any{"tampered"}
	}

	second, err := Lookup("portable", nil)
	require.NoError(t, err)
	assert.Equal(t, "commonmark",
		second.Rules["markdown-flavor"].Settings["flavor"],
		"second Lookup must return the original flavor")
	_, hasNewRule := second.Rules["new-rule"]
	assert.False(t, hasNewRule,
		"new entries on the first copy must not leak into the table")
}

func TestConventionNamesSorted(t *testing.T) {
	names := ConventionNames()
	assert.True(t, sort.StringsAreSorted(names),
		"ConventionNames should return a sorted slice; got %v", names)
	assert.ElementsMatch(t, []string{"github", "plain", "portable"}, names)
}

func TestLookup_UserConventionFoundFirst(t *testing.T) {
	// A user convention map is consulted before the built-in table.
	userMap := map[string]Convention{
		"our-team": {
			Name:   "our-team",
			Flavor: FlavorGFM,
			Rules: map[string]RulePreset{
				"list-marker-style": {Enabled: true, Settings: map[string]any{"style": "dash"}},
			},
		},
	}
	c, err := Lookup("our-team", userMap)
	require.NoError(t, err)
	assert.Equal(t, "our-team", c.Name)
	assert.Equal(t, FlavorGFM, c.Flavor)
	lm, ok := c.Rules["list-marker-style"]
	require.True(t, ok)
	assert.Equal(t, "dash", lm.Settings["style"])
}

func TestLookup_UserConventionDeepCopy(t *testing.T) {
	// Mutating the returned convention from Lookup must not affect the
	// source map entry.
	userMap := map[string]Convention{
		"our-team": {
			Name:   "our-team",
			Flavor: FlavorGFM,
			Rules: map[string]RulePreset{
				"x": {Enabled: true, Settings: map[string]any{"v": "original"}},
			},
		},
	}
	c, err := Lookup("our-team", userMap)
	require.NoError(t, err)
	c.Rules["x"].Settings["v"] = "tampered"
	c.Rules["new"] = RulePreset{Enabled: true}

	assert.Equal(t, "original", userMap["our-team"].Rules["x"].Settings["v"],
		"user map entry must not be mutated through the returned copy")
	_, hasNew := userMap["our-team"].Rules["new"]
	assert.False(t, hasNew, "new rule must not leak into the user map")
}

func TestLookup_UnknownListsUserAndBuiltIns(t *testing.T) {
	// When lookup fails and a user map is present, the error must list
	// both built-in names and user-defined names.
	userMap := map[string]Convention{
		"our-team": {Name: "our-team", Flavor: FlavorGFM},
	}
	_, err := Lookup("does-not-exist", userMap)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does-not-exist")
	// Built-ins
	assert.Contains(t, err.Error(), "github")
	assert.Contains(t, err.Error(), "plain")
	assert.Contains(t, err.Error(), "portable")
	// User-defined
	assert.Contains(t, err.Error(), "our-team")
}

func TestLookup_UserMapNilFallsBackToBuiltin(t *testing.T) {
	// nil userConventions must not crash and must still find built-ins.
	c, err := Lookup("github", nil)
	require.NoError(t, err)
	assert.Equal(t, "github", c.Name)
}

func TestLookup_BuiltInNotShadowedByUserMap(t *testing.T) {
	// Built-in names cannot appear in the user map (they are rejected at
	// config load time), so Lookup should always find them in the
	// built-in table. A user map that does NOT contain "portable" still
	// lets Lookup find it in the built-in table.
	userMap := map[string]Convention{
		"our-team": {Name: "our-team", Flavor: FlavorGFM},
	}
	c, err := Lookup("portable", userMap)
	require.NoError(t, err)
	assert.Equal(t, "portable", c.Name)
	assert.Equal(t, FlavorCommonMark, c.Flavor)
}
