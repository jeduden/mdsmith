package markdownflavor

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLookup_Portable(t *testing.T) {
	c, err := Lookup("portable")
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
	c, err := Lookup("github")
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
	c, err := Lookup("plain")
	require.NoError(t, err)
	assert.Equal(t, FlavorCommonMark, c.Flavor)

	html, ok := c.Rules["no-inline-html"]
	require.True(t, ok)
	assert.Equal(t, false, html.Settings["allow-comments"],
		"plain convention forbids HTML comments")
}

func TestLookup_Unknown(t *testing.T) {
	_, err := Lookup("bogus")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown convention")
	assert.Contains(t, err.Error(), "bogus")
	assert.Contains(t, err.Error(), "github")
	assert.Contains(t, err.Error(), "plain")
	assert.Contains(t, err.Error(), "portable")
}

func TestConventionNamesSorted(t *testing.T) {
	names := ConventionNames()
	assert.True(t, sort.StringsAreSorted(names),
		"ConventionNames should return a sorted slice; got %v", names)
	assert.ElementsMatch(t, []string{"github", "plain", "portable"}, names)
}
