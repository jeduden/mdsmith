package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	// Register markdown-flavor so rule.ByName lookups in deep-merge
	// resolve while the profile mechanism is exercised.
	_ "github.com/jeduden/mdsmith/internal/rules/markdownflavor"
)

func TestApplyProfile_NoProfileSetting_NoOp(t *testing.T) {
	cfg := &Config{
		Rules: map[string]RuleCfg{
			"markdown-flavor": {Enabled: true, Settings: map[string]any{"flavor": "gfm"}},
		},
	}
	require.NoError(t, applyProfile(cfg))
	assert.Empty(t, cfg.Profile, "Profile stays empty when no profile is set")
	assert.Nil(t, cfg.ProfilePreset, "ProfilePreset stays nil when no profile is set")
}

func TestApplyProfile_PortableSetsPresetAndProfile(t *testing.T) {
	cfg := &Config{
		Rules: map[string]RuleCfg{
			"markdown-flavor": {Enabled: true, Settings: map[string]any{"profile": "portable"}},
		},
	}
	require.NoError(t, applyProfile(cfg))
	assert.Equal(t, "portable", cfg.Profile)
	require.NotNil(t, cfg.ProfilePreset)
	mf, ok := cfg.ProfilePreset["markdown-flavor"]
	require.True(t, ok, "preset must contain markdown-flavor")
	assert.True(t, mf.Enabled)
	assert.Equal(t, "commonmark", mf.Settings["flavor"])

	// Spot-check a couple of MDS04x preset entries to confirm the
	// table is wired up. These rules may not be registered yet; the
	// preset table still stores their settings so they activate when
	// the rules ship.
	assert.Contains(t, cfg.ProfilePreset, "no-inline-html")
	assert.Contains(t, cfg.ProfilePreset, "horizontal-rule-style")
}

func TestApplyProfile_UnknownProfileErrors(t *testing.T) {
	cfg := &Config{
		Rules: map[string]RuleCfg{
			"markdown-flavor": {Enabled: true, Settings: map[string]any{"profile": "bogus"}},
		},
	}
	err := applyProfile(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rules.markdown-flavor.profile")
	assert.Contains(t, err.Error(), "bogus")
	assert.Contains(t, err.Error(), "valid:")
}

func TestApplyProfile_FlavorProfileMismatchErrors(t *testing.T) {
	cfg := &Config{
		Rules: map[string]RuleCfg{
			"markdown-flavor": {Enabled: true, Settings: map[string]any{
				"profile": "portable",
				"flavor":  "gfm",
			}},
		},
	}
	err := applyProfile(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "portable")
	assert.Contains(t, err.Error(), "commonmark")
	assert.Contains(t, err.Error(), "gfm")
}

func TestApplyProfile_FlavorProfileAgreeAccepted(t *testing.T) {
	cfg := &Config{
		Rules: map[string]RuleCfg{
			"markdown-flavor": {Enabled: true, Settings: map[string]any{
				"profile": "github",
				"flavor":  "gfm",
			}},
		},
	}
	require.NoError(t, applyProfile(cfg))
	assert.Equal(t, "github", cfg.Profile)
}

func TestEffectiveRules_ProfileIsBaseLayerUnderUserRules(t *testing.T) {
	// User extends the no-inline-html allowlist; the github profile
	// presets details/summary; the user's allowlist should replace
	// the preset's (lists default to MergeReplace).
	cfg := &Config{
		Rules: map[string]RuleCfg{
			"markdown-flavor": {Enabled: true, Settings: map[string]any{"profile": "github"}},
			"no-inline-html": {
				Enabled:  true,
				Settings: map[string]any{"allow": []any{"sub", "sup"}},
			},
		},
	}
	require.NoError(t, applyProfile(cfg))

	got := Effective(cfg, "doc.md", nil)
	rc, ok := got["no-inline-html"]
	require.True(t, ok, "no-inline-html must be present")
	assert.True(t, rc.Enabled)
	assert.Equal(t, []any{"sub", "sup"}, rc.Settings["allow"], "user list replaces preset list")
}

func TestEffectiveRules_ProfileSurvivesWhenUserDoesNotMention(t *testing.T) {
	// User does not touch horizontal-rule-style; the portable preset
	// should appear in the effective config.
	cfg := &Config{
		Rules: map[string]RuleCfg{
			"markdown-flavor": {Enabled: true, Settings: map[string]any{"profile": "portable"}},
		},
	}
	require.NoError(t, applyProfile(cfg))

	got := Effective(cfg, "doc.md", nil)
	rc, ok := got["horizontal-rule-style"]
	require.True(t, ok, "horizontal-rule-style must be inherited from profile")
	assert.True(t, rc.Enabled)
	assert.Equal(t, "dash", rc.Settings["style"])
	assert.Equal(t, true, rc.Settings["require-blank-lines"])
}

func TestEffectiveRules_UserSettingDeepMergesOverProfile(t *testing.T) {
	// User overrides one scalar setting on horizontal-rule-style;
	// preset's other settings should survive.
	cfg := &Config{
		Rules: map[string]RuleCfg{
			"markdown-flavor": {Enabled: true, Settings: map[string]any{"profile": "portable"}},
			"horizontal-rule-style": {
				Enabled:  true,
				Settings: map[string]any{"length": 5},
			},
		},
	}
	require.NoError(t, applyProfile(cfg))

	got := Effective(cfg, "doc.md", nil)
	rc := got["horizontal-rule-style"]
	assert.Equal(t, 5, rc.Settings["length"], "user scalar wins")
	assert.Equal(t, "dash", rc.Settings["style"], "preset sibling preserved")
	assert.Equal(t, true, rc.Settings["require-blank-lines"], "preset sibling preserved")
}

func TestProvenance_ProfileLayerVisible(t *testing.T) {
	cfg := &Config{
		Rules: map[string]RuleCfg{
			"markdown-flavor": {Enabled: true, Settings: map[string]any{"profile": "portable"}},
		},
	}
	require.NoError(t, applyProfile(cfg))

	res := ResolveFile(cfg, "doc.md", nil)
	rr, ok := res.Rules["horizontal-rule-style"]
	require.True(t, ok, "rule must appear in resolution")
	require.NotEmpty(t, rr.Layers)

	var sources []string
	for _, l := range rr.Layers {
		sources = append(sources, l.Source)
	}
	require.Contains(t, sources, "profile.portable", "profile layer must appear in chain")

	// The "enabled" leaf should attribute its winning value to the
	// profile layer when no other layer touches it.
	leaf := rr.LeafByPath("enabled")
	require.NotNil(t, leaf)
	assert.Equal(t, "profile.portable", leaf.Source(),
		"profile is the winning source when no later layer touches the rule")
}

func TestProvenance_DefaultLayerWinsOverProfile(t *testing.T) {
	// User explicitly sets horizontal-rule-style.length to 5; the
	// profile layer's value (3) should appear earlier in the chain
	// and the default layer should be the winning source for that
	// leaf.
	cfg := &Config{
		Rules: map[string]RuleCfg{
			"markdown-flavor": {Enabled: true, Settings: map[string]any{"profile": "portable"}},
			"horizontal-rule-style": {
				Enabled:  true,
				Settings: map[string]any{"length": 5},
			},
		},
	}
	require.NoError(t, applyProfile(cfg))

	res := ResolveFile(cfg, "doc.md", nil)
	rr := res.Rules["horizontal-rule-style"]
	leaf := rr.LeafByPath("settings.length")
	require.NotNil(t, leaf)
	assert.Equal(t, 5, leaf.Value)
	assert.Equal(t, "default", leaf.Source(),
		"user's explicit setting should win over profile")
	require.Len(t, leaf.Chain, 2, "chain should record profile then default")
	assert.Equal(t, "profile.portable", leaf.Chain[0].Source)
	assert.Equal(t, 3, leaf.Chain[0].Value, "profile contributed 3 first")
	assert.Equal(t, "default", leaf.Chain[1].Source)
}

func TestApplyProfile_DisablingMarkdownFlavorPreservesPresetForOtherRules(t *testing.T) {
	// The acceptance criterion: disabling MDS034 itself does not
	// disable rules a profile turned on. The preset has already been
	// applied at config load, so cfg.ProfilePreset still contains
	// the other rules' presets.
	cfg := &Config{
		Rules: map[string]RuleCfg{
			"markdown-flavor": {Enabled: true, Settings: map[string]any{"profile": "portable"}},
		},
	}
	require.NoError(t, applyProfile(cfg))

	// Now simulate the user disabling markdown-flavor afterwards.
	cfg.Rules["markdown-flavor"] = RuleCfg{Enabled: false}

	got := Effective(cfg, "doc.md", nil)
	rc := got["horizontal-rule-style"]
	assert.True(t, rc.Enabled, "profile-enabled rule survives MDS034 being disabled")
	assert.Equal(t, "dash", rc.Settings["style"])
}

func TestApplyProfile_ListsValidProfileNamesInError(t *testing.T) {
	cfg := &Config{
		Rules: map[string]RuleCfg{
			"markdown-flavor": {Enabled: true, Settings: map[string]any{"profile": "wat"}},
		},
	}
	err := applyProfile(cfg)
	require.Error(t, err)
	for _, name := range []string{"github", "plain", "portable"} {
		assert.True(t, strings.Contains(err.Error(), name),
			"error must list valid profile %q; got %q", name, err.Error())
	}
}

func TestApplyProfile_NilCfg(t *testing.T) {
	assert.NoError(t, applyProfile(nil))
}

func TestApplyProfile_NoMarkdownFlavorRule(t *testing.T) {
	cfg := &Config{
		Rules: map[string]RuleCfg{
			"line-length": {Enabled: true},
		},
	}
	require.NoError(t, applyProfile(cfg))
	assert.Empty(t, cfg.Profile)
	assert.Nil(t, cfg.ProfilePreset)
}

func TestLoad_InvalidProfileSurfacesError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".mdsmith.yml")
	yaml := "rules:\n  markdown-flavor:\n    profile: bogus\n"
	require.NoError(t, os.WriteFile(path, []byte(yaml), 0o600))

	_, err := Load(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "applying profile")
	assert.Contains(t, err.Error(), "bogus")
}

func TestCopyProfilePreset_NilReturnsNil(t *testing.T) {
	assert.Nil(t, copyProfilePreset(nil))
}

func TestMerge_PreservesProfilePreset(t *testing.T) {
	loaded := &Config{
		Profile: "portable",
		ProfilePreset: map[string]RuleCfg{
			"line-length": {Enabled: true, Settings: map[string]any{"max": 80}},
		},
	}
	merged := Merge(&Config{Rules: map[string]RuleCfg{}}, loaded)
	assert.Equal(t, "portable", merged.Profile)
	require.Contains(t, merged.ProfilePreset, "line-length")
	assert.Equal(t, 80, merged.ProfilePreset["line-length"].Settings["max"])

	// Mutating the merged copy must not bleed back into the source.
	merged.ProfilePreset["line-length"].Settings["max"] = 999
	assert.Equal(t, 80, loaded.ProfilePreset["line-length"].Settings["max"])
}
