package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// loadAndMergeFromString parses a user config from yml and merges it on
// top of an empty defaults config so the result has the same shape as
// configs produced by the CLI's loadConfig path.
func resolveFromYAML(t *testing.T, yml string) *Config {
	t.Helper()
	loaded := loadFromString(t, yml)
	defaults := &Config{Rules: map[string]RuleCfg{}}
	return Merge(defaults, loaded)
}

func TestResolveFile_KindsAndOverridesProvenance(t *testing.T) {
	yml := `
rules:
  max-file-length:
    max: 300
kinds:
  plan:
    rules:
      max-file-length:
        max: 500
kind-assignment:
  - files: ["plan/*.md"]
    kinds: [plan]
overrides:
  - files: ["plan/big.md"]
    rules:
      max-file-length:
        max: 900
`
	cfg := resolveFromYAML(t, yml)

	res := ResolveFile(cfg, "plan/big.md", nil, nil)
	require.NotNil(t, res)

	require.Len(t, res.Kinds, 1)
	assert.Equal(t, "plan", res.Kinds[0].Name)
	assert.Equal(t, KindAssignmentSource("kind-assignment[0]"), res.Kinds[0].Source)

	rr, ok := res.Rules["max-file-length"]
	require.True(t, ok, "max-file-length must appear in rules")

	// Three applicable layers: user, kinds.plan, overrides[0]. The
	// "default" layer is empty because the test's defaults Config
	// has no built-in rules and resolveFromYAML marks every rule
	// the test set as user-explicit.
	require.Len(t, rr.Layers, 3)
	assert.Equal(t, "user", rr.Layers[0].Source)
	assert.True(t, rr.Layers[0].Set)
	assert.Equal(t, "kinds.plan", rr.Layers[1].Source)
	assert.True(t, rr.Layers[1].Set)
	assert.Equal(t, "overrides[0]", rr.Layers[2].Source)
	assert.True(t, rr.Layers[2].Set)

	// Final value is from overrides[0].
	assert.Equal(t, 900, rr.Final.Settings["max"])

	// Per-leaf provenance: settings.max chain has three entries.
	leaf := rr.LeafByPath("settings.max")
	require.NotNil(t, leaf)
	require.Len(t, leaf.Chain, 3)
	assert.Equal(t, "user", leaf.Chain[0].Source)
	assert.Equal(t, 300, leaf.Chain[0].Value)
	assert.Equal(t, "kinds.plan", leaf.Chain[1].Source)
	assert.Equal(t, 500, leaf.Chain[1].Value)
	assert.Equal(t, "overrides[0]", leaf.Chain[2].Source)
	assert.Equal(t, 900, leaf.Chain[2].Value)
	assert.Equal(t, "overrides[0]", leaf.Source())
}

func TestResolveFile_KindAppliedFromFrontMatter(t *testing.T) {
	yml := `
rules:
  line-length:
    max: 80
kinds:
  proto:
    rules:
      line-length:
        max: 120
`
	cfg := resolveFromYAML(t, yml)
	res := ResolveFile(cfg, "doc.md", []string{"proto"}, nil)

	require.Len(t, res.Kinds, 1)
	assert.Equal(t, "proto", res.Kinds[0].Name)
	assert.Equal(t, KindAssignmentSource("front-matter"), res.Kinds[0].Source)

	rr := res.Rules["line-length"]
	leaf := rr.LeafByPath("settings.max")
	require.NotNil(t, leaf)
	assert.Equal(t, "kinds.proto", leaf.Source())
	assert.Equal(t, 120, leaf.Value)
}

func TestResolveFile_NoOpKindLayerStillInChain(t *testing.T) {
	yml := `
rules:
  line-length:
    max: 80
  paragraph-readability: false
kinds:
  proto:
    rules:
      paragraph-readability: false
kind-assignment:
  - files: ["doc.md"]
    kinds: [proto]
`
	cfg := resolveFromYAML(t, yml)
	res := ResolveFile(cfg, "doc.md", nil, nil)

	rr := res.Rules["line-length"]
	require.Len(t, rr.Layers, 2, "user + kinds.proto")
	assert.Equal(t, "user", rr.Layers[0].Source)
	assert.True(t, rr.Layers[0].Set, "user sets line-length")
	assert.Equal(t, "kinds.proto", rr.Layers[1].Source)
	assert.False(t, rr.Layers[1].Set, "kinds.proto does not set line-length")
}

func TestResolveFile_OverridesExclusiveToMatchingFiles(t *testing.T) {
	yml := `
rules:
  line-length:
    max: 80
overrides:
  - files: ["other.md"]
    rules:
      line-length:
        max: 200
`
	cfg := resolveFromYAML(t, yml)
	res := ResolveFile(cfg, "doc.md", nil, nil)

	rr := res.Rules["line-length"]
	// Override does not match doc.md, so only the user layer is in
	// the chain (the rule is user-explicit; defaults map is empty).
	require.Len(t, rr.Layers, 1)
	assert.Equal(t, "user", rr.Layers[0].Source)
}

func TestResolveFile_KindsListPreservesOrderAndDedup(t *testing.T) {
	yml := `
kinds:
  plan: {}
  proto: {}
kind-assignment:
  - files: ["doc.md"]
    kinds: [proto, plan]
  - files: ["doc.md"]
    kinds: [plan]
`
	cfg := resolveFromYAML(t, yml)
	// front-matter declares plan first, then kind-assignment adds proto and (dup) plan.
	res := ResolveFile(cfg, "doc.md", []string{"plan"}, nil)

	require.Len(t, res.Kinds, 2)
	assert.Equal(t, "plan", res.Kinds[0].Name)
	assert.Equal(t, KindAssignmentSource("front-matter"), res.Kinds[0].Source)
	assert.Equal(t, "proto", res.Kinds[1].Name)
	assert.Equal(t, KindAssignmentSource("kind-assignment[0]"), res.Kinds[1].Source)
}

// TestResolveFile_PathPatternAppearsInProvenance verifies that a
// kind's top-level `path-pattern:` field — which the engine merges
// into required-structure via a synthetic `path-patterns` setting —
// shows up in `kinds resolve` / `--explain` provenance under its
// kind layer, instead of being dropped by buildLayers.
func TestResolveFile_PathPatternAppearsInProvenance(t *testing.T) {
	yml := `
kinds:
  plan:
    path-pattern: "plan/[0-9][0-9]*_*.md"
kind-assignment:
  - files: ["plan/*.md"]
    kinds: [plan]
`
	cfg := resolveFromYAML(t, yml)
	res := ResolveFile(cfg, "plan/140_x.md", nil, nil)

	rr, ok := res.Rules["required-structure"]
	require.True(t, ok,
		"required-structure must appear in rules when path-pattern is set")
	leaf := rr.LeafByPath("settings.path-patterns")
	require.NotNil(t, leaf,
		"path-patterns leaf must be present in provenance")
	assert.Equal(t, "kinds.plan", leaf.Source())
	list, ok := leaf.Value.([]any)
	require.True(t, ok)
	require.Len(t, list, 1)
	entry := list[0].(map[string]any)
	assert.Equal(t, "plan", entry["kind"])
	assert.Equal(t, "plan/[0-9][0-9]*_*.md", entry["pattern"])
}

// TestResolveFile_SchemaSourcePath_NamedYAML pins that a kind which
// references a `.mdsmith/schemas/` schema by name surfaces the
// schema's own `.yaml` path as ResolvedKind.SchemaSourcePath, distinct
// from the kind's SourcePath (plan 241 task 9).
func TestResolveFile_SchemaSourcePath_NamedYAML(t *testing.T) {
	schemaPath := "/ws/.mdsmith/schemas/rfc-v1.yaml"
	cfg := &Config{
		Rules: map[string]RuleCfg{},
		Kinds: map[string]KindBody{
			"rfc": {
				SourcePath: "/ws/.mdsmith.yml",
				Schema:     resolvedSchemaRef("rfc-v1", map[string]any{"filename": "RFC-*.md"}, schemaPath),
			},
		},
		KindAssignment: []KindAssignmentEntry{
			{Glob: []string{"*.md"}, Kinds: []string{"rfc"}},
		},
	}
	res := ResolveFile(cfg, "doc.md", nil, nil)
	require.Len(t, res.Kinds, 1)
	assert.Equal(t, "/ws/.mdsmith.yml", res.Kinds[0].SourcePath)
	assert.Equal(t, schemaPath, res.Kinds[0].SchemaSourcePath,
		"a named YAML schema's defining path must surface as schema-source")
}

// TestResolveFile_SchemaSourcePath_ProtoMd pins that a kind whose
// schema is a `proto.md` file (rules.required-structure.schema:)
// surfaces that path as SchemaSourcePath.
func TestResolveFile_SchemaSourcePath_ProtoMd(t *testing.T) {
	yml := `
kinds:
  rfc:
    rules:
      required-structure:
        schema: schemas/rfc.md
kind-assignment:
  - files: ["*.md"]
    kinds: [rfc]
`
	cfg := resolveFromYAML(t, yml)
	res := ResolveFile(cfg, "doc.md", nil, nil)
	require.Len(t, res.Kinds, 1)
	assert.Equal(t, "schemas/rfc.md", res.Kinds[0].SchemaSourcePath)
}

// TestResolveFile_SchemaSourcePath_InlineOmitted pins that an inline
// schema on a kind leaves SchemaSourcePath empty — the kind's own
// SourcePath already names the defining file.
func TestResolveFile_SchemaSourcePath_InlineOmitted(t *testing.T) {
	cfg := &Config{
		Rules: map[string]RuleCfg{},
		Kinds: map[string]KindBody{
			"rfc": {
				SourcePath: "/ws/.mdsmith.yml",
				Schema:     inlineSchemaRef(map[string]any{"filename": "RFC-*.md"}),
			},
		},
		KindAssignment: []KindAssignmentEntry{
			{Glob: []string{"*.md"}, Kinds: []string{"rfc"}},
		},
	}
	res := ResolveFile(cfg, "doc.md", nil, nil)
	require.Len(t, res.Kinds, 1)
	assert.Empty(t, res.Kinds[0].SchemaSourcePath,
		"an inline-on-kind schema has no separate source path")
}

// TestResolveConvention covers the three convention-provenance cases
// plan 209 surfaces: a user convention carries its name, the user
// flag, and its defining file; a built-in carries only its name; no
// selection yields the zero value.
func TestResolveConvention(t *testing.T) {
	t.Run("user convention carries source path", func(t *testing.T) {
		cfg := &Config{
			Convention: "portable-strict",
			Conventions: map[string]UserConvention{
				"portable-strict": {
					SourcePath: "/ws/.mdsmith/conventions/portable-strict.yaml",
				},
			},
		}
		rc := resolveConvention(cfg)
		assert.Equal(t, "portable-strict", rc.Name)
		assert.True(t, rc.IsUser)
		assert.Equal(t,
			"/ws/.mdsmith/conventions/portable-strict.yaml", rc.SourcePath)
	})
	t.Run("built-in convention has no source path", func(t *testing.T) {
		rc := resolveConvention(&Config{Convention: "github"})
		assert.Equal(t, "github", rc.Name)
		assert.False(t, rc.IsUser)
		assert.Empty(t, rc.SourcePath)
	})
	t.Run("no convention selected", func(t *testing.T) {
		rc := resolveConvention(&Config{})
		assert.Empty(t, rc.Name)
		assert.False(t, rc.IsUser)
		assert.Empty(t, rc.SourcePath)
	})
}
