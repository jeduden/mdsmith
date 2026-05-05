package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	// Register rules so ByName and ApplySettings work.
	_ "github.com/jeduden/mdsmith/internal/rules/markdownflavor"
)

// TestLoad_UserConventionDefined verifies that a conventions: block is
// parsed and a user-defined convention can be selected.
func TestLoad_UserConventionDefined(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".mdsmith.yml")
	yaml := `conventions:
  our-team:
    flavor: gfm
    rules:
      list-marker-style:
        style: dash
convention: our-team
`
	require.NoError(t, os.WriteFile(path, []byte(yaml), 0o600))

	cfg, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, "our-team", cfg.Convention)
	require.NotNil(t, cfg.ConventionPreset)
	lms, ok := cfg.ConventionPreset["list-marker-style"]
	require.True(t, ok, "preset must contain list-marker-style")
	assert.True(t, lms.Enabled)
	assert.Equal(t, "dash", lms.Settings["style"])
}

// TestLoad_UserConventionReservedName verifies that defining a
// convention with a reserved built-in name produces a config error.
func TestLoad_UserConventionReservedName(t *testing.T) {
	for _, reserved := range []string{"portable", "github", "plain"} {
		t.Run(reserved, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, ".mdsmith.yml")
			yaml := "conventions:\n  " + reserved + ":\n    flavor: gfm\n"
			require.NoError(t, os.WriteFile(path, []byte(yaml), 0o600))

			_, err := Load(path)
			require.Error(t, err)
			assert.Contains(t, err.Error(), reserved)
		})
	}
}

// TestLoad_UserConventionInvalidFlavor verifies that a bad flavor
// value inside a user-defined convention produces a config error.
func TestLoad_UserConventionInvalidFlavor(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".mdsmith.yml")
	yaml := `conventions:
  our-team:
    flavor: bogus-flavor
`
	require.NoError(t, os.WriteFile(path, []byte(yaml), 0o600))

	_, err := Load(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "our-team")
	assert.Contains(t, err.Error(), "flavor")
}

// TestLoad_UserConventionUnknownRule verifies that referencing an
// unknown rule inside a user convention produces a config error.
func TestLoad_UserConventionUnknownRule(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".mdsmith.yml")
	yaml := `conventions:
  our-team:
    flavor: gfm
    rules:
      no-such-rule:
        foo: bar
`
	require.NoError(t, os.WriteFile(path, []byte(yaml), 0o600))

	_, err := Load(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "our-team")
	assert.Contains(t, err.Error(), "no-such-rule")
}

// TestLoad_UserConventionInvalidRuleSetting verifies that an invalid
// rule setting inside a user convention produces a config error naming
// the convention and the rule.
func TestLoad_UserConventionInvalidRuleSetting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".mdsmith.yml")
	// list-marker-style does not have a setting called "allowed"
	yaml := `conventions:
  our-team:
    flavor: gfm
    rules:
      list-marker-style:
        unknown-setting: bad
`
	require.NoError(t, os.WriteFile(path, []byte(yaml), 0o600))

	_, err := Load(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "our-team")
	assert.Contains(t, err.Error(), "list-marker-style")
}

// TestLoad_UserConventionUnknownListsBothSets verifies that an unknown
// convention name lists both built-in and user-defined names in the error.
func TestLoad_UserConventionUnknownListsBothSets(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".mdsmith.yml")
	yaml := `conventions:
  our-team:
    flavor: gfm
convention: bogus
`
	require.NoError(t, os.WriteFile(path, []byte(yaml), 0o600))

	_, err := Load(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bogus")
	// must list user-defined names
	assert.Contains(t, err.Error(), "our-team")
	// must list built-in names
	assert.True(t,
		strings.Contains(err.Error(), "github") ||
			strings.Contains(err.Error(), "portable") ||
			strings.Contains(err.Error(), "plain"),
		"error must list built-in names; got: %s", err.Error())
}

// TestLoad_UserConventionTopLevelRulesOverride verifies that top-level
// rules: overrides win over user convention presets via deep-merge.
func TestLoad_UserConventionTopLevelRulesOverride(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".mdsmith.yml")
	yaml := `conventions:
  our-team:
    flavor: gfm
    rules:
      list-marker-style:
        style: dash
convention: our-team
rules:
  list-marker-style:
    style: asterisk
`
	require.NoError(t, os.WriteFile(path, []byte(yaml), 0o600))

	cfg, err := Load(path)
	require.NoError(t, err)

	merged := Merge(Defaults(), cfg)
	got := Effective(merged, "doc.md", nil)
	lms, ok := got["list-marker-style"]
	require.True(t, ok)
	assert.Equal(t, "asterisk", lms.Settings["style"],
		"top-level rules: override must win over user convention preset")
}

// TestApplyConvention_UserConventionPreset verifies that applyConvention
// uses the user conventions map for resolution.
func TestApplyConvention_UserConventionPreset(t *testing.T) {
	userConventions := map[string]UserConventionBody{
		"our-team": {
			Flavor: "gfm",
			Rules: map[string]RuleCfg{
				"list-marker-style": {
					Enabled:  true,
					Settings: map[string]any{"style": "dash"},
				},
			},
		},
	}
	cfg := &Config{
		Convention:      "our-team",
		UserConventions: userConventions,
	}
	require.NoError(t, applyConvention(cfg))
	require.NotNil(t, cfg.ConventionPreset)
	lms := cfg.ConventionPreset["list-marker-style"]
	assert.Equal(t, "dash", lms.Settings["style"])
}
