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
// the merge layer translates KindBody.Schema into
// rules.required-structure.Settings["inline-schema"] so the rule
// receives it through ApplySettings without any rule-specific wiring
// at the call site.
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
	effective := Effective(cfg, "docs/rfcs/foo.md", nil)
	rs, ok := effective["required-structure"]
	require.True(t, ok, "required-structure should be in effective config")
	require.NotNil(t, rs.Settings)
	got, ok := rs.Settings["inline-schema"].(map[string]any)
	require.True(t, ok, "inline-schema must be injected as a map")
	require.Contains(t, got, "sections")
}

// TestEffectiveClearsPriorSchemaWhenNewSourceArrives covers the
// "last source wins" rule the merge layer enforces. When kind A
// supplies a file path and kind B supplies an inline schema, the
// resulting required-structure has only the inline source — the
// file path is cleared.
func TestEffectiveClearsPriorSchemaWhenNewSourceArrives(t *testing.T) {
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
	effective := Effective(cfg, "foo.md", nil)
	rs := effective["required-structure"]
	assert.NotContains(t, rs.Settings, "schema",
		"prior file-source must be cleared when a later kind installs inline")
	assert.Contains(t, rs.Settings, "inline-schema",
		"later inline source should be present")
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

// TestEmptyInlineSchemaDoesNotClearPriorState ensures the merge
// layer doesn't wipe a file-schema path when a later kind has
// `schema: {}`. An empty map is "no source", so prior state
// survives unchanged.
func TestEmptyInlineSchemaDoesNotClearPriorState(t *testing.T) {
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
	effective := Effective(cfg, "foo.md", nil)
	rs := effective["required-structure"]
	assert.Equal(t, "schemas/a.md", rs.Settings["schema"],
		"empty schema map must not clear prior file-source")
	assert.NotContains(t, rs.Settings, "inline-schema",
		"empty schema map must not install an inline-schema entry")
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
	effective := Effective(cfg, "foo.md", nil)
	assert.NotNil(t, effective["required-structure"],
		"bool-only kind entry must still resolve")
}

func TestClearSchemaState_NoRequiredStructureEntry(t *testing.T) {
	// clearSchemaState should be a no-op when result has no
	// required-structure entry. Cover the early-return branch.
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
	effective := Effective(cfg, "foo.md", nil)
	rs, ok := effective["required-structure"]
	require.True(t, ok)
	assert.Contains(t, rs.Settings, "inline-schema")
}

func TestClearSchemaState_NilSettings(t *testing.T) {
	// clearSchemaState with required-structure present but Settings
	// nil — early return on settings-nil branch.
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
	effective := Effective(cfg, "foo.md", nil)
	rs := effective["required-structure"]
	assert.Contains(t, rs.Settings, "inline-schema")
}

func TestKindDeclaresSchemaRecognisesInlineSchemaSetting(t *testing.T) {
	// A kind that supplies `inline-schema` via the rules map (not
	// via KindBody.Schema) still counts as a schema source so the
	// merge clears prior state.
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
	effective := Effective(cfg, "foo.md", nil)
	rs := effective["required-structure"]
	assert.NotContains(t, rs.Settings, "schema",
		"a kind installing inline-schema via rules should clear the file path")
	assert.Contains(t, rs.Settings, "inline-schema")
}

// TestEffectiveClearsPriorSchemaForOverrideInlineSchema covers the
// path where an override (not a kind) installs an inline-schema:
// the helper rulesDeclareSchema must recognise inline-schema as a
// source so the prior file-schema path gets cleared.
func TestEffectiveClearsPriorSchemaForOverrideInlineSchema(t *testing.T) {
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
	effective := Effective(cfg, "foo.md", nil)
	rs := effective["required-structure"]
	assert.NotContains(t, rs.Settings, "schema",
		"override installing inline-schema should clear prior file source")
	assert.Contains(t, rs.Settings, "inline-schema")
}

// TestEffectiveClearsInlineWhenFileSourceArrives is the symmetric
// case: inline first, file second. The later file source wins.
func TestEffectiveClearsInlineWhenFileSourceArrives(t *testing.T) {
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
	effective := Effective(cfg, "foo.md", nil)
	rs := effective["required-structure"]
	assert.NotContains(t, rs.Settings, "inline-schema",
		"prior inline source must be cleared when a later kind installs file path")
	assert.Equal(t, "schemas/b.md", rs.Settings["schema"])
}
