package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestKindAcceptsInlineSchema covers the inline `schema:` block on a
// kind body — plan 146's primary new source.
func TestKindAcceptsInlineSchema(t *testing.T) {
	yml := `
kinds:
  rfc:
    schema:
      frontmatter:
        id: '=~"^RFC-[0-9]{4}$"'
      sections:
        - heading: "Overview"
          required: true
`
	cfg := loadFromString(t, yml)
	require.Contains(t, cfg.Kinds, "rfc")
	body := cfg.Kinds["rfc"]
	require.NotNil(t, body.Schema)
	require.Contains(t, body.Schema, "frontmatter")
	require.Contains(t, body.Schema, "sections")
}

// TestKindRejectsInlineMapInRules rejects a kind whose
// `rules.required-structure.inline-schema` is set together with the
// top-level `schema:` block — both declare an inline source.
func TestKindRejectsInlineMapInRules(t *testing.T) {
	yml := `
kinds:
  rfc:
    schema:
      sections:
        - heading: "Overview"
    rules:
      required-structure:
        inline-schema:
          sections:
            - heading: "Other"
`
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".mdsmith.yml")
	require.NoError(t, os.WriteFile(cfgPath, []byte(yml), 0o644))
	_, err := Load(cfgPath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rfc")
	assert.Contains(t, err.Error(), "inline-schema")
}

// TestKindRejectsBothSchemaAndInlineUnderRules covers the case
// where a single kind sets both `schema` and `inline-schema` under
// rules.required-structure (no top-level `schema:` block).
func TestKindRejectsBothSchemaAndInlineUnderRules(t *testing.T) {
	yml := `
kinds:
  rfc:
    rules:
      required-structure:
        schema: schemas/rfc.md
        inline-schema:
          sections:
            - heading: "Overview"
`
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".mdsmith.yml")
	require.NoError(t, os.WriteFile(cfgPath, []byte(yml), 0o644))
	_, err := Load(cfgPath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rfc")
	assert.Contains(t, err.Error(), "schema:")
	assert.Contains(t, err.Error(), "inline-schema:")
}

// TestKindRejectsDualSchemaSources covers acceptance criterion #8:
// declaring both an inline schema and the legacy file-schema path on
// the same kind must error at load time.
func TestKindRejectsDualSchemaSources(t *testing.T) {
	yml := `
kinds:
  rfc:
    schema:
      sections:
        - heading: "Overview"
    rules:
      required-structure:
        schema: schemas/rfc.md
`
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".mdsmith.yml")
	require.NoError(t, os.WriteFile(cfgPath, []byte(yml), 0o644))
	_, err := Load(cfgPath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rfc")
	assert.Contains(t, err.Error(), "inline")
	assert.Contains(t, err.Error(), "schemas/rfc.md")
}

// TestEffectiveInjectsInlineSchema verifies that a kind's inline
// schema reaches required-structure via the effective rule config:
// the merge layer translates KindBody.Schema into a `schema-sources`
// list entry so the rule receives it through ApplySettings without
// any rule-specific wiring at the call site.
func TestEffectiveInjectsInlineSchema(t *testing.T) {
	inline := map[string]any{
		"sections": []any{
			map[string]any{"heading": "Overview"},
		},
	}
	cfg := &Config{
		Rules: map[string]RuleCfg{
			"required-structure": {Enabled: true},
		},
		Kinds: map[string]KindBody{
			"rfc": {Schema: inline},
		},
		KindAssignment: []KindAssignmentEntry{
			{Glob: []string{"docs/rfcs/*.md"}, Kinds: []string{"rfc"}},
		},
	}
	effective := Effective(cfg, "docs/rfcs/foo.md", nil, nil)
	rs, ok := effective["required-structure"]
	require.True(t, ok, "required-structure should be in effective config")
	require.NotNil(t, rs.Settings)
	sources, ok := rs.Settings["schema-sources"].([]any)
	require.True(t, ok, "schema-sources must be a list")
	require.Len(t, sources, 1)
	entry, ok := sources[0].(map[string]any)
	require.True(t, ok)
	got, ok := entry["inline"].(map[string]any)
	require.True(t, ok, "inline-schema entry must wrap the kind's inline map")
	require.Contains(t, got, "sections")
}

// TestEffectiveInjectsInlineSchemaSourcePath covers plan 230: the
// kind's defining file (SourcePath) rides on the schema-sources entry
// as `source`, so an inline-kind violation names a navigable file
// instead of the bare "inline kind schema" label.
func TestEffectiveInjectsInlineSchemaSourcePath(t *testing.T) {
	cfg := &Config{
		Rules: map[string]RuleCfg{"required-structure": {Enabled: true}},
		Kinds: map[string]KindBody{
			"rfc": {
				Schema: map[string]any{
					"sections": []any{map[string]any{"heading": "Overview"}},
				},
				SourcePath: "/work/.mdsmith.yml",
			},
		},
		KindAssignment: []KindAssignmentEntry{
			{Glob: []string{"docs/rfcs/*.md"}, Kinds: []string{"rfc"}},
		},
	}
	effective := Effective(cfg, "docs/rfcs/foo.md", nil, nil)
	sources, ok := effective["required-structure"].Settings["schema-sources"].([]any)
	require.True(t, ok)
	require.Len(t, sources, 1)
	entry := sources[0].(map[string]any)
	require.Equal(t, "/work/.mdsmith.yml", entry["source"])
}

// TestEffectiveComposesSchemaSourcesAcrossKinds covers plan 156: when
// kind A supplies a file-path schema and kind B supplies an inline
// schema, both end up in the composed schema-sources list rather
// than the last one winning.
func TestEffectiveComposesSchemaSourcesAcrossKinds(t *testing.T) {
	cfg := &Config{
		Rules: map[string]RuleCfg{
			"required-structure": {Enabled: true},
		},
		Kinds: map[string]KindBody{
			"a": {Rules: map[string]RuleCfg{
				"required-structure": {Enabled: true, Settings: map[string]any{
					"schema": "schemas/a.md",
				}},
			}},
			"b": {Schema: map[string]any{
				"sections": []any{map[string]any{"heading": "Overview"}},
			}},
		},
		KindAssignment: []KindAssignmentEntry{
			{Glob: []string{"*.md"}, Kinds: []string{"a", "b"}},
		},
	}
	effective := Effective(cfg, "foo.md", nil, nil)
	rs := effective["required-structure"]
	sources, ok := rs.Settings["schema-sources"].([]any)
	require.True(t, ok, "schema-sources must accumulate across kinds")
	require.Len(t, sources, 2, "both kinds should contribute a source")
	first := sources[0].(map[string]any)
	assert.Equal(t, "schemas/a.md", first["file"])
	second := sources[1].(map[string]any)
	assert.Contains(t, second, "inline")
	assert.NotContains(t, rs.Settings, "schema",
		"legacy schema key must be stripped after translation")
	assert.NotContains(t, rs.Settings, "inline-schema",
		"legacy inline-schema key must be stripped after translation")
}

// TestValidateKindAllowsInlineWithoutFileSchemaSetting covers
// validateKindSchemaSources' early-return branches: inline schema
// alone, inline schema plus other rule settings, and inline schema
// plus a required-structure entry that has no `schema:` key.
func TestValidateKindAllowsInlineWithoutFileSchemaSetting(t *testing.T) {
	cases := []struct {
		name string
		body KindBody
	}{
		{
			name: "inline only",
			body: KindBody{Schema: map[string]any{"sections": []any{}}},
		},
		{
			name: "inline plus unrelated rule",
			body: KindBody{
				Schema: map[string]any{"sections": []any{}},
				Rules: map[string]RuleCfg{
					"line-length": {Enabled: true},
				},
			},
		},
		{
			name: "inline plus required-structure without schema key",
			body: KindBody{
				Schema: map[string]any{"sections": []any{}},
				Rules: map[string]RuleCfg{
					"required-structure": {Enabled: true, Settings: map[string]any{
						"placeholders": []any{"foo"},
					}},
				},
			},
		},
		{
			name: "inline plus required-structure with empty schema",
			body: KindBody{
				Schema: map[string]any{"sections": []any{}},
				Rules: map[string]RuleCfg{
					"required-structure": {Enabled: true, Settings: map[string]any{
						"schema": "",
					}},
				},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &Config{Kinds: map[string]KindBody{"k": tc.body}}
			assert.NoError(t, ValidateKinds(cfg))
		})
	}
}

// TestEmptyInlineSchemaDoesNotTriggerMutex covers the edge case
// where `kinds.<n>.schema:` is set to an empty map (e.g. via
// templating). It should be treated as "no inline schema" rather
// than triggering the mutual-exclusion error against a file-based
// schema setting under rules.required-structure.
func TestEmptyInlineSchemaDoesNotTriggerMutex(t *testing.T) {
	cfg := &Config{
		Kinds: map[string]KindBody{
			"k": {
				Schema: map[string]any{}, // empty inline source
				Rules: map[string]RuleCfg{
					"required-structure": {Enabled: true, Settings: map[string]any{
						"schema": "schemas/k.md",
					}},
				},
			},
		},
	}
	assert.NoError(t, ValidateKinds(cfg),
		"empty inline schema map must not count as a declared source")
}

// TestEffectiveKinds_NilCfg covers the cfg == nil fast-path:
// without a Config the function returns the deduplicated fmKinds
// list straight through.
func TestEffectiveKinds_NilCfg(t *testing.T) {
	got := EffectiveKinds(nil, "doc.md", []string{"a", "b", "a"}, nil)
	assert.Equal(t, []string{"a", "b"}, got,
		"nil cfg path must dedupe fmKinds in input order")
}

// TestEffectiveKinds_CfgDelegatesToResolver verifies the non-nil
// cfg branch forwards to resolveEffectiveKinds.
func TestEffectiveKinds_CfgDelegatesToResolver(t *testing.T) {
	cfg := &Config{
		Kinds: map[string]KindBody{"plan": {}},
		KindAssignment: []KindAssignmentEntry{
			{Glob: []string{"plan/*.md"}, Kinds: []string{"plan"}},
		},
	}
	got := EffectiveKinds(cfg, "plan/foo.md", nil, nil)
	assert.Equal(t, []string{"plan"}, got)
}

// TestEffectiveKinds_AllBranches pins every branch of the public
// helper with direct cases so its coverage no longer depends on
// indirect call sites (the recurring codecov/changes signal).
func TestEffectiveKinds_AllBranches(t *testing.T) {
	t.Run("nil cfg, empty fmKinds -> empty", func(t *testing.T) {
		got := EffectiveKinds(nil, "doc.md", nil, nil)
		assert.Empty(t, got)
	})
	t.Run("nil cfg, no duplicates -> order preserved", func(t *testing.T) {
		got := EffectiveKinds(nil, "doc.md", []string{"a", "b", "c"}, nil)
		assert.Equal(t, []string{"a", "b", "c"}, got)
	})
	t.Run("nil cfg, duplicates -> deduped first-wins", func(t *testing.T) {
		got := EffectiveKinds(nil, "doc.md",
			[]string{"a", "b", "a", "c", "b"}, nil)
		assert.Equal(t, []string{"a", "b", "c"}, got,
			"seen-true branch must skip later duplicates")
	})
	t.Run("non-nil cfg delegates and merges fm + assignment", func(t *testing.T) {
		cfg := &Config{
			Kinds: map[string]KindBody{"plan": {}, "proto": {}},
			KindAssignment: []KindAssignmentEntry{
				{Glob: []string{"plan/*.md"}, Kinds: []string{"plan"}},
			},
		}
		got := EffectiveKinds(cfg, "plan/foo.md",
			[]string{"proto"}, nil)
		// front-matter kinds first, then kind-assignment matches.
		assert.Equal(t, []string{"proto", "plan"}, got)
	})
	t.Run("non-nil cfg, fields-present selector via fmFields", func(t *testing.T) {
		cfg := &Config{
			Kinds: map[string]KindBody{"rfc": {}},
			KindAssignment: []KindAssignmentEntry{
				{Glob: []string{"**/*.md"}, FieldsPresent: []string{"rfc-id"},
					Kinds: []string{"rfc"}},
			},
		}
		with := EffectiveKinds(cfg, "x.md", nil,
			map[string]any{"rfc-id": "RFC-1"})
		assert.Equal(t, []string{"rfc"}, with,
			"fmFields must feed the fields-present selector")
		without := EffectiveKinds(cfg, "x.md", nil, nil)
		assert.Empty(t, without,
			"no fmFields -> fields-present entry must not match")
	})
}

// TestTranslateLayerSettings_NonStringSchemaKey covers a layer
// whose `schema:` is a non-string value: the rule's translator
// still strips the legacy key but adds no schema-sources entry.
func TestTranslateLayerSettings_NonStringSchemaKey(t *testing.T) {
	out := translateLayerSettings("required-structure", RuleCfg{
		Enabled:  true,
		Settings: map[string]any{"schema": 42},
	})
	assert.NotContains(t, out.Settings, "schema")
	assert.NotContains(t, out.Settings, "schema-sources")
}

// TestTranslateLayerSettings_NonMapInlineKey covers a non-map
// `inline-schema` value: treated as empty, key stripped, no
// source added.
func TestTranslateLayerSettings_NonMapInlineKey(t *testing.T) {
	out := translateLayerSettings("required-structure", RuleCfg{
		Enabled:  true,
		Settings: map[string]any{"inline-schema": "not-a-map"},
	})
	assert.NotContains(t, out.Settings, "inline-schema")
	assert.NotContains(t, out.Settings, "schema-sources")
}

// TestTranslateLayerSettings_UnknownRulePassthrough covers the
// rule-not-registered / no-translator branches: a rule with no
// SettingsTranslator (or an unknown name) returns its settings
// untouched.
func TestTranslateLayerSettings_UnknownRulePassthrough(t *testing.T) {
	in := RuleCfg{Enabled: true, Settings: map[string]any{"schema": "x.md"}}
	out := translateLayerSettings("definitely-not-a-rule", in)
	assert.Equal(t, "x.md", out.Settings["schema"],
		"unknown rule must not have its settings rewritten")
}

// TestTranslateLayerSettings_NilSettingsPassthrough covers the
// rc.Settings == nil early return.
func TestTranslateLayerSettings_NilSettingsPassthrough(t *testing.T) {
	out := translateLayerSettings("required-structure",
		RuleCfg{Enabled: true})
	assert.Nil(t, out.Settings)
}

// TestKindLayerRules_InlineSchemaWithoutPathPattern covers the
// `body.PathPattern != ""` false branch in kindLayerRules: when
// a kind body has an inline Schema map but no path-pattern, the
// path-patterns inject is skipped while the schema source still
// lands in schema-sources.
func TestKindLayerRules_InlineSchemaWithoutPathPattern(t *testing.T) {
	body := KindBody{
		Schema: map[string]any{
			"sections": []any{map[string]any{"heading": "X"}},
		},
		// PathPattern intentionally empty.
	}
	out := kindLayerRules("k", body, nil)
	rs := out["required-structure"]
	assert.True(t, rs.Enabled)
	assert.NotContains(t, rs.Settings, "path-patterns",
		"empty PathPattern must not inject a path-patterns entry")
	sources, ok := rs.Settings["schema-sources"].([]any)
	require.True(t, ok)
	require.Len(t, sources, 1)
	assert.Contains(t, sources[0].(map[string]any), "inline")
}

// TestTranslateLayerSettings_StripsLegacyKeyWithoutAddingSource
// covers the empty-value path: a layer with `schema: ""` (the
// rule's DefaultSettings placeholder) is stripped of the legacy
// key without contributing a schema-sources entry, and unrelated
// settings survive.
func TestTranslateLayerSettings_StripsLegacyKeyWithoutAddingSource(t *testing.T) {
	out := translateLayerSettings("required-structure", RuleCfg{
		Enabled: true,
		Settings: map[string]any{
			"schema":       "",
			"placeholders": []string{"cue-frontmatter"},
		},
	})
	assert.NotContains(t, out.Settings, "schema",
		"legacy schema key must be stripped even when value is empty")
	assert.NotContains(t, out.Settings, "schema-sources",
		"empty schema value must NOT add a schema-sources entry")
	assert.Contains(t, out.Settings, "placeholders",
		"unrelated settings must survive translation")
}

// TestTranslateLayerSettings_BothEmptyValuesStripped covers a
// layer carrying both legacy keys with empty values: both keys
// are stripped, no source is added.
func TestTranslateLayerSettings_BothEmptyValuesStripped(t *testing.T) {
	out := translateLayerSettings("required-structure", RuleCfg{
		Enabled: true,
		Settings: map[string]any{
			"schema":        "",
			"inline-schema": map[string]any{},
		},
	})
	assert.NotContains(t, out.Settings, "schema")
	assert.NotContains(t, out.Settings, "inline-schema")
	assert.NotContains(t, out.Settings, "schema-sources")
}

// TestEmptyInlineSchemaIsNoOp ensures the merge layer doesn't add a
// source entry when a kind has `schema: {}`. An empty map is "no
// source", so the composed schema-sources list contains only the
// prior kind's file source.
func TestEmptyInlineSchemaIsNoOp(t *testing.T) {
	cfg := &Config{
		Rules: map[string]RuleCfg{
			"required-structure": {Enabled: true},
		},
		ExplicitRules: map[string]bool{"required-structure": true},
		Kinds: map[string]KindBody{
			"a": {Rules: map[string]RuleCfg{
				"required-structure": {Enabled: true, Settings: map[string]any{
					"schema": "schemas/a.md",
				}},
			}},
			"b": {Schema: map[string]any{}}, // empty — should be ignored
		},
		KindAssignment: []KindAssignmentEntry{
			{Glob: []string{"*.md"}, Kinds: []string{"a", "b"}},
		},
	}
	effective := Effective(cfg, "foo.md", nil, nil)
	rs := effective["required-structure"]
	sources, ok := rs.Settings["schema-sources"].([]any)
	require.True(t, ok)
	require.Len(t, sources, 1, "empty schema map must not add a source entry")
	first := sources[0].(map[string]any)
	assert.Equal(t, "schemas/a.md", first["file"])
}

// TestInlineSchemaMarksRequiredStructureExplicit regresses a
// bug where an inline `schema:` on a kind would resolve to an
// enabled required-structure rule, but EffectiveExplicitRules
// would not flag it as explicit (it only walked body.Rules). A
// disabled `meta` category then silently wiped the inline
// schema's effect. The explicit map must now include
// required-structure when KindBody.Schema is non-empty.
func TestInlineSchemaMarksRequiredStructureExplicit(t *testing.T) {
	cfg := &Config{
		Kinds: map[string]KindBody{
			"k": {Schema: map[string]any{
				"sections": []any{map[string]any{"heading": "X"}},
			}},
		},
		KindAssignment: []KindAssignmentEntry{
			{Glob: []string{"*.md"}, Kinds: []string{"k"}},
		},
	}
	explicit := EffectiveExplicitRules(cfg, "foo.md", nil, nil)
	assert.True(t, explicit["required-structure"],
		"an inline kind schema must mark required-structure as explicit")
}

// TestBoolOnlyRequiredStructureRuleCfg covers the case where a
// kind or override sets `required-structure: true/false` — the
// RuleCfg has Settings=nil, and ValidateKinds / Effective must not
// panic when probing schema-source state.
func TestBoolOnlyRequiredStructureRuleCfg(t *testing.T) {
	cfg := &Config{
		Rules: map[string]RuleCfg{
			"required-structure": {Enabled: true},
		},
		ExplicitRules: map[string]bool{"required-structure": true},
		Kinds: map[string]KindBody{
			"k": {Rules: map[string]RuleCfg{
				"required-structure": {Enabled: false}, // bool-only
			}},
		},
		KindAssignment: []KindAssignmentEntry{
			{Glob: []string{"*.md"}, Kinds: []string{"k"}},
		},
		Overrides: []Override{
			{Glob: []string{"x.md"}, Rules: map[string]RuleCfg{
				"required-structure": {Enabled: true}, // bool-only override
			}},
		},
	}
	require.NoError(t, ValidateKinds(cfg),
		"bool-only RuleCfg must not crash schema-source validation")
	effective := Effective(cfg, "foo.md", nil, nil)
	assert.NotNil(t, effective["required-structure"],
		"bool-only kind entry must still resolve")
}

func TestEffectiveInlineSchemaInjectsSourceEntryWithoutPriorRule(t *testing.T) {
	// A kind with an inline schema reaches the effective config even
	// when no required-structure rule entry exists yet — the merge
	// layer creates the rule entry and installs the source.
	cfg := &Config{
		Rules: map[string]RuleCfg{"line-length": {Enabled: true}},
		Kinds: map[string]KindBody{
			"k": {Schema: map[string]any{
				"sections": []any{map[string]any{"heading": "X"}},
			}},
		},
		KindAssignment: []KindAssignmentEntry{
			{Glob: []string{"*.md"}, Kinds: []string{"k"}},
		},
	}
	effective := Effective(cfg, "foo.md", nil, nil)
	rs, ok := effective["required-structure"]
	require.True(t, ok)
	sources, ok := rs.Settings["schema-sources"].([]any)
	require.True(t, ok)
	require.Len(t, sources, 1)
	entry := sources[0].(map[string]any)
	assert.Contains(t, entry, "inline")
}

func TestEffectiveInlineSchemaSourceSurvivesNilSettings(t *testing.T) {
	// required-structure may be present with nil Settings before the
	// kind runs (bool-only enable). Installing the kind's inline
	// source must allocate Settings rather than panicking.
	cfg := &Config{
		Rules: map[string]RuleCfg{
			"required-structure": {Enabled: true},
		},
		ExplicitRules: map[string]bool{"required-structure": true},
		Kinds: map[string]KindBody{
			"k": {Schema: map[string]any{
				"sections": []any{map[string]any{"heading": "X"}},
			}},
		},
		KindAssignment: []KindAssignmentEntry{
			{Glob: []string{"*.md"}, Kinds: []string{"k"}},
		},
	}
	effective := Effective(cfg, "foo.md", nil, nil)
	rs := effective["required-structure"]
	sources, ok := rs.Settings["schema-sources"].([]any)
	require.True(t, ok)
	require.Len(t, sources, 1)
}

func TestEffectiveComposesInlineSchemaFromRulesWithBaseFile(t *testing.T) {
	// A kind that supplies `inline-schema` via the rules map composes
	// with a top-level `schema:` set on the base required-structure
	// rule. Both sources end up in schema-sources.
	cfg := &Config{
		Rules: map[string]RuleCfg{
			"required-structure": {Enabled: true, Settings: map[string]any{
				"schema": "schemas/base.md",
			}},
		},
		ExplicitRules: map[string]bool{"required-structure": true},
		Kinds: map[string]KindBody{
			"k": {Rules: map[string]RuleCfg{
				"required-structure": {Enabled: true, Settings: map[string]any{
					"inline-schema": map[string]any{
						"sections": []any{
							map[string]any{"heading": "X"},
						},
					},
				}},
			}},
		},
		KindAssignment: []KindAssignmentEntry{
			{Glob: []string{"*.md"}, Kinds: []string{"k"}},
		},
	}
	effective := Effective(cfg, "foo.md", nil, nil)
	rs := effective["required-structure"]
	sources, ok := rs.Settings["schema-sources"].([]any)
	require.True(t, ok, "schema-sources must accumulate base + kind sources")
	require.Len(t, sources, 2)
	assert.Equal(t, "schemas/base.md", sources[0].(map[string]any)["file"])
	assert.Contains(t, sources[1].(map[string]any), "inline")
}

// TestEffectiveComposesOverrideInlineWithBaseFile covers the path
// where an override (not a kind) installs an inline-schema: the
// override and the base file source compose into a two-entry list
// rather than the override clearing the base.
func TestEffectiveComposesOverrideInlineWithBaseFile(t *testing.T) {
	cfg := &Config{
		Rules: map[string]RuleCfg{
			"required-structure": {Enabled: true, Settings: map[string]any{
				"schema": "schemas/base.md",
			}},
		},
		ExplicitRules: map[string]bool{"required-structure": true},
		Overrides: []Override{
			{Glob: []string{"*.md"}, Rules: map[string]RuleCfg{
				"required-structure": {Enabled: true, Settings: map[string]any{
					"inline-schema": map[string]any{
						"sections": []any{map[string]any{"heading": "X"}},
					},
				}},
			}},
		},
	}
	effective := Effective(cfg, "foo.md", nil, nil)
	rs := effective["required-structure"]
	sources, ok := rs.Settings["schema-sources"].([]any)
	require.True(t, ok, "schema-sources must compose base + override")
	require.Len(t, sources, 2)
	assert.Equal(t, "schemas/base.md", sources[0].(map[string]any)["file"])
	assert.Contains(t, sources[1].(map[string]any), "inline")
}

// TestEffectiveComposesInlineThenFileSources is the symmetric case
// to TestEffectiveComposesSchemaSourcesAcrossKinds: kind A has the
// inline schema and kind B the file. Both compose, in declared order.
func TestEffectiveComposesInlineThenFileSources(t *testing.T) {
	cfg := &Config{
		Rules: map[string]RuleCfg{
			"required-structure": {Enabled: true},
		},
		Kinds: map[string]KindBody{
			"a": {Schema: map[string]any{
				"sections": []any{map[string]any{"heading": "Overview"}},
			}},
			"b": {Rules: map[string]RuleCfg{
				"required-structure": {Enabled: true, Settings: map[string]any{
					"schema": "schemas/b.md",
				}},
			}},
		},
		KindAssignment: []KindAssignmentEntry{
			{Glob: []string{"*.md"}, Kinds: []string{"a", "b"}},
		},
	}
	effective := Effective(cfg, "foo.md", nil, nil)
	rs := effective["required-structure"]
	sources, ok := rs.Settings["schema-sources"].([]any)
	require.True(t, ok)
	require.Len(t, sources, 2)
	assert.Contains(t, sources[0].(map[string]any), "inline")
	assert.Equal(t, "schemas/b.md", sources[1].(map[string]any)["file"])
}
