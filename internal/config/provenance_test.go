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

	res := ResolveFile(cfg, "plan/big.md", nil)
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
	res := ResolveFile(cfg, "doc.md", []string{"proto"})

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
	res := ResolveFile(cfg, "doc.md", nil)

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
	res := ResolveFile(cfg, "doc.md", nil)

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
	res := ResolveFile(cfg, "doc.md", []string{"plan"})

	require.Len(t, res.Kinds, 2)
	assert.Equal(t, "plan", res.Kinds[0].Name)
	assert.Equal(t, KindAssignmentSource("front-matter"), res.Kinds[0].Source)
	assert.Equal(t, "proto", res.Kinds[1].Name)
	assert.Equal(t, KindAssignmentSource("kind-assignment[0]"), res.Kinds[1].Source)
}
