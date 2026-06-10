package config

import (
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jeduden/mdsmith/internal/convention"

	// Register rules so rule.ByName lookups resolve while the
	// convention mechanism is exercised.
	_ "github.com/jeduden/mdsmith/internal/rules/emphasisstyle"
	_ "github.com/jeduden/mdsmith/internal/rules/markdownflavor"
	_ "github.com/jeduden/mdsmith/internal/rules/nohardtabs"
)

// TestConfigDoesNotImportRules guards the dependency direction
// recorded in docs/development/architecture/index.md: rules sit at
// the lowest layer, config sits above them, so config must not
// import any rule package. The convention and flavor data types
// live in internal/convention so this constraint can be enforced.
// Test files are exempt because they need to register rules to
// exercise rule.ByName lookups.
func TestConfigDoesNotImportRules(t *testing.T) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(fi fs.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, parser.ImportsOnly)
	require.NoError(t, err)
	for _, pkg := range pkgs {
		for fname, file := range pkg.Files {
			for _, imp := range file.Imports {
				path := strings.Trim(imp.Path.Value, `"`)
				assert.NotContains(t, path, "internal/rules/",
					"%s imports a rule package: %s", fname, path)
			}
		}
	}
}

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

func TestApplyConvention_FlavorUnsetConventionAllowsAnyUserFlavor(t *testing.T) {
	// no-llm-tells leaves its flavor unset (FlavorAny). A project that
	// also pins markdown-flavor to gfm must not be rejected: the
	// convention is renderer-agnostic and does not enable markdown-flavor.
	cfg := &Config{
		Convention: "no-llm-tells",
		Rules: map[string]RuleCfg{
			"markdown-flavor": {Enabled: true, Settings: map[string]any{"flavor": "gfm"}},
		},
	}
	require.NoError(t, applyConvention(cfg))
	require.NotNil(t, cfg.ConventionPreset)
}

func TestApplyConvention_NoLLMTells_EnablesRulesWithSettings(t *testing.T) {
	cfg := &Config{Convention: "no-llm-tells"}
	require.NoError(t, applyConvention(cfg))

	ft, ok := cfg.ConventionPreset["forbidden-text"]
	require.True(t, ok, "preset must contain forbidden-text")
	assert.True(t, ft.Enabled)
	contains, ok := ft.Settings["contains"].([]any)
	require.True(t, ok, "contains must be []any")
	assert.Contains(t, contains, "delve")
	assert.Contains(t, contains, "it's important to note that")

	fps, ok := cfg.ConventionPreset["forbidden-paragraph-starts"]
	require.True(t, ok, "preset must contain forbidden-paragraph-starts")
	starts, ok := fps.Settings["starts"].([]any)
	require.True(t, ok)
	assert.Contains(t, starts, "Moreover,")

	ps, ok := cfg.ConventionPreset["paragraph-structure"]
	require.True(t, ok)
	assert.Equal(t, 25, ps.Settings["max-words-per-sentence"])

	pr, ok := cfg.ConventionPreset["paragraph-readability"]
	require.True(t, ok)
	assert.Equal(t, 12.0, pr.Settings["max-index"])

	dlt, ok := cfg.ConventionPreset["descriptive-link-text"]
	require.True(t, ok)
	assert.True(t, dlt.Enabled)
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

	got := Effective(cfg, "doc.md", nil, nil)
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

	got := Effective(cfg, "doc.md", nil, nil)
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

	got := Effective(cfg, "doc.md", nil, nil)
	rc := got["horizontal-rule-style"]
	assert.Equal(t, 5, rc.Settings["length"], "user scalar wins")
	assert.Equal(t, "dash", rc.Settings["style"], "preset sibling preserved")
	assert.Equal(t, true, rc.Settings["require-blank-lines"], "preset sibling preserved")
}

func TestEffectiveRules_NoLLMTells_UserForbiddenTextUnionsWithConvention(t *testing.T) {
	// A project pins no-llm-tells and adds its own forbidden phrase.
	// MDS056 opts contains: into MergeAppend, so the user's list unions
	// with the convention's instead of replacing it.
	cfg := &Config{
		Convention: "no-llm-tells",
		Rules: map[string]RuleCfg{
			"forbidden-text": {
				Enabled:  true,
				Settings: map[string]any{"contains": []any{"synergy"}},
			},
		},
		ExplicitRules: map[string]bool{"forbidden-text": true},
	}
	require.NoError(t, applyConvention(cfg))

	got := Effective(cfg, "doc.md", nil, nil)
	contains, ok := got["forbidden-text"].Settings["contains"].([]any)
	require.True(t, ok)
	assert.Contains(t, contains, "delve", "convention entry survives")
	assert.Contains(t, contains, "synergy", "user entry is added")
}

func TestEffectiveRules_NoLLMTells_UserOpenersUnionWithConvention(t *testing.T) {
	cfg := &Config{
		Convention: "no-llm-tells",
		Rules: map[string]RuleCfg{
			"forbidden-paragraph-starts": {
				Enabled:  true,
				Settings: map[string]any{"starts": []any{"We "}},
			},
		},
		ExplicitRules: map[string]bool{"forbidden-paragraph-starts": true},
	}
	require.NoError(t, applyConvention(cfg))

	got := Effective(cfg, "doc.md", nil, nil)
	starts, ok := got["forbidden-paragraph-starts"].Settings["starts"].([]any)
	require.True(t, ok)
	assert.Contains(t, starts, "Moreover,", "convention entry survives")
	assert.Contains(t, starts, "We ", "user entry is added")
}

func TestProvenance_ConventionLayerVisible(t *testing.T) {
	cfg := &Config{Convention: "portable"}
	require.NoError(t, applyConvention(cfg))

	res := ResolveFile(cfg, "doc.md", nil, nil)
	rr, ok := res.Rules["horizontal-rule-style"]
	require.True(t, ok, "rule must appear in resolution")
	require.NotEmpty(t, rr.Layers)

	sources := make([]string, 0, len(rr.Layers))
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

	res := ResolveFile(cfg, "doc.md", nil, nil)
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

	got := Effective(cfg, "doc.md", nil, nil)
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

// TestEffectiveRules_ParityConventionDisablesExtras proves the
// `parity` convention drives the effective config down to the
// markdownlint-compatible rule class: every rule it names is off,
// including rules enabled by default, while markdownlint-class rules
// it does not name stay on. This is the "a convention disables a
// default-on rule" path, which no built-in convention exercised
// before parity.
func TestEffectiveRules_ParityConventionDisablesExtras(t *testing.T) {
	loaded := &Config{Convention: "parity"}
	require.NoError(t, applyConvention(loaded))
	cfg := Merge(Defaults(), loaded)
	got := Effective(cfg, "doc.md", nil, nil)

	conv, err := convention.Lookup("parity", nil)
	require.NoError(t, err)
	// Every parity rule registered in this build is disabled in the
	// effective config (opt-in rules absent from this test binary's
	// registry are simply skipped).
	for name := range conv.Rules {
		if rc, ok := got[name]; ok {
			assert.False(t, rc.Enabled,
				"parity must disable %q in effective config", name)
		}
	}

	// Spot-check the default-on extras so the disable-a-default-on-rule
	// path is genuinely exercised, not just opt-in no-ops.
	for _, name := range []string{
		"catalog", "cross-file-reference-integrity", "token-budget",
		"paragraph-readability", "max-file-length", "required-structure",
	} {
		rc, ok := got[name]
		require.True(t, ok, "default-on rule %q must be present", name)
		assert.False(t, rc.Enabled, "parity must disable default-on rule %q", name)
	}

	// Markdownlint-class rules parity does not name stay enabled.
	for _, name := range []string{"line-length", "heading-style", "no-bare-urls"} {
		rc, ok := got[name]
		require.True(t, ok, "rule %q must be present", name)
		assert.True(t, rc.Enabled, "parity must leave %q enabled", name)
	}
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

// TestValidateConventionScalar_RejectsYAMLAnchors pins the
// document-wide anchor/alias rejection added for audit finding
// S003: an alias-valued convention and an anchor on an
// unrelated key are both rejected — the latter is the case a
// per-value check would miss.
func TestValidateConventionScalar_RejectsYAMLAnchors(t *testing.T) {
	for _, data := range []string{
		"base: &anchor portable\nconvention: *anchor\n",
		"other: &a x\nconvention: portable\n",
	} {
		err := validateConventionScalar([]byte(data))
		require.Error(t, err, data)
		assert.Contains(t, err.Error(), "anchors/aliases", data)
	}
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

	got := Effective(merged, "doc.md", nil, nil)
	mf, ok := got["markdown-flavor"]
	require.True(t, ok, "markdown-flavor must be present after merge")
	assert.True(t, mf.Enabled,
		"convention: portable must enable markdown-flavor (opt-in by default)")
	assert.Equal(t, "commonmark", mf.Settings["flavor"],
		"convention preset must populate flavor on MDS034")
}

// ---- User-defined convention tests ----

func TestApplyConvention_UserConvention_Valid(t *testing.T) {
	cfg := &Config{
		Conventions: map[string]UserConvention{
			"our-team": {
				Flavor: "gfm",
				Rules: map[string]RuleCfg{
					"markdown-flavor": {
						Enabled:  true,
						Settings: map[string]any{"flavor": "gfm"},
					},
				},
			},
		},
		Convention: "our-team",
	}
	require.NoError(t, applyConvention(cfg))
	require.NotNil(t, cfg.ConventionPreset)
	mf, ok := cfg.ConventionPreset["markdown-flavor"]
	require.True(t, ok, "preset must contain markdown-flavor")
	assert.True(t, mf.Enabled)
	assert.Equal(t, "gfm", mf.Settings["flavor"])
}

func TestApplyConvention_UserConvention_ReservedName(t *testing.T) {
	for _, reserved := range []string{"portable", "github", "plain", "parity"} {
		t.Run(reserved, func(t *testing.T) {
			cfg := &Config{
				Conventions: map[string]UserConvention{
					reserved: {Flavor: "gfm"},
				},
			}
			err := applyConvention(cfg)
			require.Error(t, err)
			assert.Contains(t, err.Error(), reserved)
			assert.Contains(t, err.Error(), "reserved")
		})
	}
}

func TestApplyConvention_UserConvention_InvalidFlavor(t *testing.T) {
	cfg := &Config{
		Conventions: map[string]UserConvention{
			"our-team": {Flavor: "notaflavor"},
		},
	}
	err := applyConvention(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "our-team")
	assert.Contains(t, err.Error(), "notaflavor")
}

func TestApplyConvention_UserConvention_UnknownRule(t *testing.T) {
	cfg := &Config{
		Conventions: map[string]UserConvention{
			"our-team": {
				Flavor: "gfm",
				Rules: map[string]RuleCfg{
					"no-such-rule": {Enabled: true},
				},
			},
		},
	}
	err := applyConvention(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "our-team")
	assert.Contains(t, err.Error(), "no-such-rule")
}

func TestApplyConvention_UserConvention_InvalidSetting(t *testing.T) {
	cfg := &Config{
		Conventions: map[string]UserConvention{
			"our-team": {
				Flavor: "gfm",
				Rules: map[string]RuleCfg{
					"markdown-flavor": {
						Enabled:  true,
						Settings: map[string]any{"no-such-setting": "val"},
					},
				},
			},
		},
	}
	err := applyConvention(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "our-team")
	assert.Contains(t, err.Error(), "markdown-flavor")
	assert.Contains(t, err.Error(), "no-such-setting")
}

func TestApplyConvention_UserConvention_TopLevelRulesOverride(t *testing.T) {
	// Convention presets emphasis-style with bold=asterisk; user's top-level
	// rules override bold to underscore. The user layer wins.
	cfg := &Config{
		Conventions: map[string]UserConvention{
			"our-team": {
				Flavor: "gfm",
				Rules: map[string]RuleCfg{
					"emphasis-style": {
						Enabled:  true,
						Settings: map[string]any{"bold": "asterisk"},
					},
				},
			},
		},
		Convention: "our-team",
		Rules: map[string]RuleCfg{
			"emphasis-style": {
				Enabled:  true,
				Settings: map[string]any{"bold": "underscore"},
			},
		},
		ExplicitRules: map[string]bool{"emphasis-style": true},
	}
	require.NoError(t, applyConvention(cfg))

	got := Effective(cfg, "doc.md", nil, nil)
	es, ok := got["emphasis-style"]
	require.True(t, ok)
	assert.True(t, es.Enabled)
	// User's explicit setting overrides the convention preset.
	assert.Equal(t, "underscore", es.Settings["bold"],
		"top-level rules must win over user convention preset")
}

func TestApplyConvention_UserConvention_ErrorListsBothSets(t *testing.T) {
	// When convention: references an unknown name, the error must list
	// both built-in and user-defined convention names.
	cfg := &Config{
		Conventions: map[string]UserConvention{
			"our-team": {Flavor: "gfm"},
		},
		Convention: "bogus",
	}
	err := applyConvention(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bogus")
	assert.Contains(t, err.Error(), "our-team",
		"error must include user-defined convention names")
	assert.Contains(t, err.Error(), "portable",
		"error must include built-in convention names")
}

func TestApplyConvention_UserConvention_NotSelectedStillValidated(t *testing.T) {
	// A user convention declared but not selected must still be validated.
	cfg := &Config{
		Conventions: map[string]UserConvention{
			"bad-convention": {
				Flavor: "gfm",
				Rules: map[string]RuleCfg{
					"no-such-rule": {Enabled: true},
				},
			},
		},
		// Convention is NOT set — we don't select bad-convention.
	}
	err := applyConvention(cfg)
	require.Error(t, err, "unselected convention with bad rule must still fail")
	assert.Contains(t, err.Error(), "no-such-rule")
}

func TestProvenance_UserConventionLayerHasUserSuffix(t *testing.T) {
	cfg := &Config{
		Conventions: map[string]UserConvention{
			"our-team": {
				Flavor: "gfm",
				Rules: map[string]RuleCfg{
					"markdown-flavor": {
						Enabled:  true,
						Settings: map[string]any{"flavor": "gfm"},
					},
				},
			},
		},
		Convention: "our-team",
	}
	require.NoError(t, applyConvention(cfg))

	res := ResolveFile(cfg, "doc.md", nil, nil)
	rr, ok := res.Rules["markdown-flavor"]
	require.True(t, ok)

	sources := make([]string, 0, len(rr.Layers))
	for _, l := range rr.Layers {
		sources = append(sources, l.Source)
	}
	require.Contains(t, sources, "convention.our-team (user)",
		"user convention layer must carry the (user) suffix")
}

func TestLoad_UserConvention_Valid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".mdsmith.yml")
	yaml := `conventions:
  our-team:
    flavor: gfm
    rules:
      markdown-flavor:
        flavor: gfm
convention: our-team
`
	require.NoError(t, os.WriteFile(path, []byte(yaml), 0o600))

	cfg, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, "our-team", cfg.Convention)
	assert.NotNil(t, cfg.ConventionPreset)
	mf, ok := cfg.ConventionPreset["markdown-flavor"]
	require.True(t, ok)
	assert.Equal(t, "gfm", mf.Settings["flavor"])
}

func TestLoad_UserConvention_ReservedName(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".mdsmith.yml")
	yaml := "conventions:\n  portable:\n    flavor: gfm\n"
	require.NoError(t, os.WriteFile(path, []byte(yaml), 0o600))

	_, err := Load(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "portable")
	assert.Contains(t, err.Error(), "reserved")
}

func TestApplyConvention_UserConvention_NonConfigurableRuleWithSettings(t *testing.T) {
	// no-hard-tabs has no ApplySettings; passing settings for it must
	// return a "rule has no configurable settings" error.
	cfg := &Config{
		Conventions: map[string]UserConvention{
			"our-team": {
				Flavor: "gfm",
				Rules: map[string]RuleCfg{
					"no-hard-tabs": {
						Enabled:  true,
						Settings: map[string]any{"some-setting": "val"},
					},
				},
			},
		},
	}
	err := applyConvention(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "our-team")
	assert.Contains(t, err.Error(), "no-hard-tabs")
	assert.Contains(t, err.Error(), "no configurable settings")
}

func TestMerge_PreservesUserConventions(t *testing.T) {
	loaded := &Config{
		Convention: "our-team",
		Conventions: map[string]UserConvention{
			"our-team": {
				Flavor: "gfm",
				Rules: map[string]RuleCfg{
					"markdown-flavor": {Enabled: true, Settings: map[string]any{"flavor": "gfm"}},
				},
			},
		},
		ConventionPreset: map[string]RuleCfg{
			"markdown-flavor": {Enabled: true, Settings: map[string]any{"flavor": "gfm"}},
		},
	}
	merged := Merge(&Config{Rules: map[string]RuleCfg{}}, loaded)
	assert.Equal(t, "our-team", merged.Convention)
	require.Contains(t, merged.Conventions, "our-team")
	assert.Equal(t, "gfm", merged.Conventions["our-team"].Flavor)

	// Mutating the merged map must not bleed into the source.
	merged.Conventions["our-team"] = UserConvention{Flavor: "tampered"}
	assert.Equal(t, "gfm", loaded.Conventions["our-team"].Flavor)
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
