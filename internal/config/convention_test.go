package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	// Register markdown-flavor so rule.ByName lookups in deep-merge
	// resolve while the convention mechanism is exercised.
	_ "github.com/jeduden/mdsmith/internal/rules/markdownflavor"
)

func TestApplyConvention_NoConventionSet_NoOp(t *testing.T) {
	cfg := &Config{
		Rules: map[string]RuleCfg{
			"markdown-flavor": {Enabled: true, Settings: map[string]any{"flavor": "gfm"}},
		},
	}
	require.NoError(t, applyConvention(cfg))
	assert.Empty(t, cfg.Convention, "Convention stays empty when none is set")
	assert.Nil(t, cfg.ConventionPreset, "ConventionPreset stays nil when none is set")
}

func TestApplyConvention_PortableSetsPreset(t *testing.T) {
	cfg := &Config{Convention: "portable"}
	require.NoError(t, applyConvention(cfg))
	require.NotNil(t, cfg.ConventionPreset)

	mf, ok := cfg.ConventionPreset["markdown-flavor"]
	require.True(t, ok, "preset must contain markdown-flavor")
	assert.True(t, mf.Enabled)
	assert.Equal(t, "commonmark", mf.Settings["flavor"])

	// Spot-check a couple of MDS04x preset entries to confirm the
	// table is wired up. These rules may not be registered yet; the
	// preset table still stores their settings so they activate when
	// the rules ship.
	assert.Contains(t, cfg.ConventionPreset, "no-inline-html")
	assert.Contains(t, cfg.ConventionPreset, "horizontal-rule-style")
}

func TestApplyConvention_UnknownConventionErrors(t *testing.T) {
	cfg := &Config{Convention: "bogus"}
	err := applyConvention(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "convention")
	assert.Contains(t, err.Error(), "bogus")
}

func TestApplyConvention_FlavorMismatchErrors(t *testing.T) {
	cfg := &Config{
		Convention: "portable",
		Rules: map[string]RuleCfg{
			"markdown-flavor": {Enabled: true, Settings: map[string]any{"flavor": "gfm"}},
		},
	}
	err := applyConvention(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "portable")
	assert.Contains(t, err.Error(), "commonmark")
	assert.Contains(t, err.Error(), "gfm")
}

func TestApplyConvention_FlavorAgreeAccepted(t *testing.T) {
	cfg := &Config{
		Convention: "github",
		Rules: map[string]RuleCfg{
			"markdown-flavor": {Enabled: true, Settings: map[string]any{"flavor": "gfm"}},
		},
	}
	require.NoError(t, applyConvention(cfg))
	require.NotNil(t, cfg.ConventionPreset)
}

func TestEffectiveRules_ConventionIsBaseLayerUnderUserRules(t *testing.T) {
	// User extends the no-inline-html allowlist; the github
	// convention presets details/summary; the user's allowlist should
	// replace the preset's (lists default to MergeReplace).
	cfg := &Config{
		Convention: "github",
		Rules: map[string]RuleCfg{
			"no-inline-html": {
				Enabled:  true,
				Settings: map[string]any{"allow": []any{"sub", "sup"}},
			},
		},
		ExplicitRules: map[string]bool{"no-inline-html": true},
	}
	require.NoError(t, applyConvention(cfg))

	got := Effective(cfg, "doc.md", nil)
	rc, ok := got["no-inline-html"]
	require.True(t, ok, "no-inline-html must be present")
	assert.True(t, rc.Enabled)
	assert.Equal(t, []any{"sub", "sup"}, rc.Settings["allow"], "user list replaces preset list")
}

func TestEffectiveRules_ConventionSurvivesWhenUserDoesNotMention(t *testing.T) {
	// User does not touch horizontal-rule-style; the portable preset
	// should appear in the effective config.
	cfg := &Config{Convention: "portable"}
	require.NoError(t, applyConvention(cfg))

	got := Effective(cfg, "doc.md", nil)
	rc, ok := got["horizontal-rule-style"]
	require.True(t, ok, "horizontal-rule-style must be inherited from convention")
	assert.True(t, rc.Enabled)
	assert.Equal(t, "dash", rc.Settings["style"])
	assert.Equal(t, true, rc.Settings["require-blank-lines"])
}

func TestEffectiveRules_UserSettingDeepMergesOverConvention(t *testing.T) {
	// User overrides one scalar setting on horizontal-rule-style;
	// preset's other settings should survive.
	cfg := &Config{
		Convention: "portable",
		Rules: map[string]RuleCfg{
			"horizontal-rule-style": {
				Enabled:  true,
				Settings: map[string]any{"length": 5},
			},
		},
		ExplicitRules: map[string]bool{"horizontal-rule-style": true},
	}
	require.NoError(t, applyConvention(cfg))

	got := Effective(cfg, "doc.md", nil)
	rc := got["horizontal-rule-style"]
	assert.Equal(t, 5, rc.Settings["length"], "user scalar wins")
	assert.Equal(t, "dash", rc.Settings["style"], "preset sibling preserved")
	assert.Equal(t, true, rc.Settings["require-blank-lines"], "preset sibling preserved")
}

func TestProvenance_ConventionLayerVisible(t *testing.T) {
	cfg := &Config{Convention: "portable"}
	require.NoError(t, applyConvention(cfg))

	res := ResolveFile(cfg, "doc.md", nil)
	rr, ok := res.Rules["horizontal-rule-style"]
	require.True(t, ok, "rule must appear in resolution")
	require.NotEmpty(t, rr.Layers)

	var sources []string
	for _, l := range rr.Layers {
		sources = append(sources, l.Source)
	}
	require.Contains(t, sources, "convention.portable",
		"convention layer must appear in chain")

	// The "enabled" leaf should attribute its winning value to the
	// convention layer when no other layer touches it.
	leaf := rr.LeafByPath("enabled")
	require.NotNil(t, leaf)
	assert.Equal(t, "convention.portable", leaf.Source(),
		"convention is the winning source when no later layer touches the rule")
}

func TestProvenance_UserLayerWinsOverConvention(t *testing.T) {
	// User explicitly sets horizontal-rule-style.length to 5; the
	// convention layer's value (3) should appear earlier in the
	// chain and the user layer should be the winning source for
	// that leaf.
	cfg := &Config{
		Convention: "portable",
		Rules: map[string]RuleCfg{
			"horizontal-rule-style": {
				Enabled:  true,
				Settings: map[string]any{"length": 5},
			},
		},
		ExplicitRules: map[string]bool{"horizontal-rule-style": true},
	}
	require.NoError(t, applyConvention(cfg))

	res := ResolveFile(cfg, "doc.md", nil)
	rr := res.Rules["horizontal-rule-style"]
	leaf := rr.LeafByPath("settings.length")
	require.NotNil(t, leaf)
	assert.Equal(t, 5, leaf.Value)
	assert.Equal(t, "user", leaf.Source(),
		"user's explicit setting should win over convention")
	require.Len(t, leaf.Chain, 2,
		"chain should record convention then user")
	assert.Equal(t, "convention.portable", leaf.Chain[0].Source)
	assert.Equal(t, 3, leaf.Chain[0].Value, "convention contributed 3 first")
	assert.Equal(t, "user", leaf.Chain[1].Source)
}

func TestApplyConvention_DisablingMarkdownFlavorPreservesPresetForOtherRules(t *testing.T) {
	// The acceptance criterion: disabling MDS034 itself does not
	// disable rules a convention turned on. The preset has already
	// been applied at config load, so cfg.ConventionPreset still
	// contains the other rules' presets.
	cfg := &Config{Convention: "portable"}
	require.NoError(t, applyConvention(cfg))

	// Now simulate the user disabling markdown-flavor afterwards.
	if cfg.Rules == nil {
		cfg.Rules = map[string]RuleCfg{}
	}
	cfg.Rules["markdown-flavor"] = RuleCfg{Enabled: false}

	got := Effective(cfg, "doc.md", nil)
	rc := got["horizontal-rule-style"]
	assert.True(t, rc.Enabled,
		"convention-enabled rule survives MDS034 being disabled")
	assert.Equal(t, "dash", rc.Settings["style"])
}

func TestApplyConvention_ListsValidConventionNamesInError(t *testing.T) {
	cfg := &Config{Convention: "wat"}
	err := applyConvention(cfg)
	require.Error(t, err)
	for _, name := range []string{"github", "plain", "portable"} {
		assert.True(t, strings.Contains(err.Error(), name),
			"error must list valid convention %q; got %q", name, err.Error())
	}
}

func TestApplyConvention_NilCfg(t *testing.T) {
	assert.NoError(t, applyConvention(nil))
}

func TestApplyConvention_MarkdownFlavorWithoutFlavorKey(t *testing.T) {
	// Cover the stringSetting "key not in map" branch: the user
	// sets the markdown-flavor rule but does not provide a flavor.
	// applyConvention must read no flavor (no error) and apply the
	// preset normally.
	cfg := &Config{
		Convention: "portable",
		Rules: map[string]RuleCfg{
			"markdown-flavor": {Enabled: true},
		},
	}
	require.NoError(t, applyConvention(cfg))
	require.NotNil(t, cfg.ConventionPreset)
}

func TestValidateConventionScalar_NonMappingDocument(t *testing.T) {
	// Defensive branch: non-mapping documents (e.g. an empty file
	// or a top-level scalar) cannot carry a "convention:" key, so
	// validateConventionScalar is a no-op.
	assert.NoError(t, validateConventionScalar([]byte("")))
	assert.NoError(t, validateConventionScalar([]byte("just-a-string\n")))
}

func TestLoad_TopLevelConventionLoaded(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".mdsmith.yml")
	yaml := "convention: portable\n"
	require.NoError(t, os.WriteFile(path, []byte(yaml), 0o600))

	cfg, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, "portable", cfg.Convention)
	assert.NotNil(t, cfg.ConventionPreset)
}

func TestLoad_NonStringConventionScalarRejected(t *testing.T) {
	// yaml.v3 will silently coerce bare ints and bools into a
	// string field, which would surface as "unknown convention
	// 123". Catch the type mismatch before that coercion happens
	// and report a clean error naming the field.
	cases := []struct {
		name string
		yaml string
		want string
	}{
		{"int", "convention: 123\n", "got int"},
		{"bool", "convention: true\n", "got bool"},
		{"float", "convention: 1.5\n", "got float"},
		{"sequence", "convention: [a, b]\n", "must be a string scalar"},
		{"mapping", "convention:\n  a: 1\n", "must be a string scalar"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, ".mdsmith.yml")
			require.NoError(t, os.WriteFile(path, []byte(tc.yaml), 0o600))

			_, err := Load(path)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "convention")
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}

func TestLoad_InvalidConventionSurfacesError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".mdsmith.yml")
	yaml := "convention: bogus\n"
	require.NoError(t, os.WriteFile(path, []byte(yaml), 0o600))

	_, err := Load(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "applying convention")
	assert.Contains(t, err.Error(), "bogus")
}

func TestCopyConventionPreset_NilReturnsNil(t *testing.T) {
	assert.Nil(t, copyConventionPreset(nil))
}

func TestApplyConvention_NonStringFlavorErrors(t *testing.T) {
	cfg := &Config{
		Convention: "portable",
		Rules: map[string]RuleCfg{
			"markdown-flavor": {Enabled: true, Settings: map[string]any{
				"flavor": 42,
			}},
		},
	}
	err := applyConvention(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rules.markdown-flavor.flavor")
	assert.Contains(t, err.Error(), "must be a string")
}

// TestConvention_EnablesOptInRuleEndToEnd is a regression test for
// the "convention preset can override built-in defaults" contract.
// Goes through the full Load + Merge(defaults, loaded) pipeline so
// it exercises the same path the CLI uses.
//
// MDS034 (markdown-flavor) is opt-in (EnabledByDefault returns
// false). Setting `convention: portable` in YAML must enable it
// because the convention preset includes
// `markdown-flavor: { Enabled: true }`. An earlier implementation
// applied cfg.ConventionPreset *under* cfg.Rules, which after
// Merge contained the default's `Enabled: false` for every
// registered rule — so the convention's `Enabled: true` got
// silently overwritten by the default's `Enabled: false`.
func TestConvention_EnablesOptInRuleEndToEnd(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".mdsmith.yml")
	require.NoError(t, os.WriteFile(path, []byte("convention: portable\n"), 0o600))

	loaded, err := Load(path)
	require.NoError(t, err)
	merged := Merge(Defaults(), loaded)

	got := Effective(merged, "doc.md", nil)
	mf, ok := got["markdown-flavor"]
	require.True(t, ok, "markdown-flavor must be present after merge")
	assert.True(t, mf.Enabled,
		"convention: portable must enable markdown-flavor (opt-in by default)")
	assert.Equal(t, "commonmark", mf.Settings["flavor"],
		"convention preset must populate flavor on MDS034")
}

func TestMerge_PreservesConvention(t *testing.T) {
	loaded := &Config{
		Convention: "portable",
		ConventionPreset: map[string]RuleCfg{
			"line-length": {Enabled: true, Settings: map[string]any{"max": 80}},
		},
	}
	merged := Merge(&Config{Rules: map[string]RuleCfg{}}, loaded)
	assert.Equal(t, "portable", merged.Convention)
	require.Contains(t, merged.ConventionPreset, "line-length")
	assert.Equal(t, 80, merged.ConventionPreset["line-length"].Settings["max"])

	// Mutating the merged copy must not bleed back into the source.
	merged.ConventionPreset["line-length"].Settings["max"] = 999
	assert.Equal(t, 80, loaded.ConventionPreset["line-length"].Settings["max"])
}
