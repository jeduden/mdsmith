package convention

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
	assert.Contains(t, err.Error(), "obsidian")
}

func TestLookup_Obsidian(t *testing.T) {
	c, err := Lookup("obsidian", nil)
	require.NoError(t, err)
	assert.Equal(t, "obsidian", c.Name)
	assert.Equal(t, FlavorGFM, c.Flavor)

	xref, ok := c.Rules["cross-file-reference-integrity"]
	require.True(t, ok)
	assert.True(t, xref.Enabled)
	assert.Equal(t, true, xref.Settings["wikilinks"])
	assert.Equal(t, "obsidian", xref.Settings["wikilink-style"])

	co, ok := c.Rules["callout-type"]
	require.True(t, ok)
	assert.True(t, co.Enabled)
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

func TestLookup_UserConvention(t *testing.T) {
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

func TestLookup_UserConventionFallsBackToBuiltin(t *testing.T) {
	userMap := map[string]Convention{
		"our-team": {Name: "our-team", Flavor: FlavorGFM},
	}
	c, err := Lookup("portable", userMap)
	require.NoError(t, err)
	assert.Equal(t, "portable", c.Name)
}

func TestLookup_UnknownListsUserAndBuiltin(t *testing.T) {
	userMap := map[string]Convention{
		"our-team": {Name: "our-team", Flavor: FlavorGFM},
	}
	_, err := Lookup("bogus", userMap)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "our-team", "error must list user convention name")
	assert.Contains(t, err.Error(), "portable", "error must list built-in name")
}

func TestNamesSorted(t *testing.T) {
	names := Names()
	assert.True(t, sort.StringsAreSorted(names),
		"Names should return a sorted slice; got %v", names)
	assert.ElementsMatch(t, []string{"github", "obsidian", "plain", "portable"}, names)
}
