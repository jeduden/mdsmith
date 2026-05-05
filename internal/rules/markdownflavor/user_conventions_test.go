package markdownflavor

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLookupWithUserConventions_UserDefinedFound verifies that a
// user-defined convention is returned when present in the user map.
func TestLookupWithUserConventions_UserDefinedFound(t *testing.T) {
	userConventions := map[string]Convention{
		"our-team": {
			Name:   "our-team",
			Flavor: FlavorGFM,
			Rules: map[string]RulePreset{
				"list-marker-style": {
					Enabled:  true,
					Settings: map[string]any{"style": "dash"},
				},
			},
		},
	}
	c, err := Lookup("our-team", userConventions)
	require.NoError(t, err)
	assert.Equal(t, "our-team", c.Name)
	assert.Equal(t, FlavorGFM, c.Flavor)
	lms, ok := c.Rules["list-marker-style"]
	require.True(t, ok)
	assert.Equal(t, "dash", lms.Settings["style"])
}

// TestLookupWithUserConventions_BuiltInStillFound verifies that
// built-in conventions are still returned when the user map does not
// shadow them.
func TestLookupWithUserConventions_BuiltInStillFound(t *testing.T) {
	c, err := Lookup("portable", nil)
	require.NoError(t, err)
	assert.Equal(t, "portable", c.Name)
}

// TestLookupWithUserConventions_UnknownListsBoth verifies that an
// unknown convention name lists both built-in and user-defined names
// in the error.
func TestLookupWithUserConventions_UnknownListsBoth(t *testing.T) {
	userConventions := map[string]Convention{
		"our-team": {Name: "our-team", Flavor: FlavorGFM},
	}
	_, err := Lookup("bogus", userConventions)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bogus")
	assert.Contains(t, err.Error(), "our-team")
	assert.Contains(t, err.Error(), "github")
	assert.Contains(t, err.Error(), "plain")
	assert.Contains(t, err.Error(), "portable")
}

// TestLookupWithUserConventions_NilUserMap verifies that a nil user
// map falls back to built-ins without error.
func TestLookupWithUserConventions_NilUserMap(t *testing.T) {
	c, err := Lookup("github", nil)
	require.NoError(t, err)
	assert.Equal(t, "github", c.Name)
}

// TestLookupWithUserConventions_UserConventionDeepCopied verifies that
// mutating a returned user convention does not corrupt the user map.
func TestLookupWithUserConventions_UserConventionDeepCopied(t *testing.T) {
	userConventions := map[string]Convention{
		"our-team": {
			Name:   "our-team",
			Flavor: FlavorGFM,
			Rules: map[string]RulePreset{
				"list-marker-style": {
					Enabled:  true,
					Settings: map[string]any{"style": "dash"},
				},
			},
		},
	}
	first, err := Lookup("our-team", userConventions)
	require.NoError(t, err)
	first.Rules["list-marker-style"].Settings["style"] = "tampered"

	second, err := Lookup("our-team", userConventions)
	require.NoError(t, err)
	assert.Equal(t, "dash", second.Rules["list-marker-style"].Settings["style"],
		"mutating returned copy must not corrupt the user map")
}
