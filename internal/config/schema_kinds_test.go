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
