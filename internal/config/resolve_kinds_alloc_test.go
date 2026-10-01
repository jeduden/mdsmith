package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolveEffectiveKinds_Dedup(t *testing.T) {
	cfg := &Config{KindAssignment: []KindAssignmentEntry{
		{Glob: []string{"**/*.md"}, Kinds: []string{"b", "c", "a"}},
	}}
	got := resolveEffectiveKinds(cfg, "x.md", []string{"a", "b", "a"}, nil)
	assert.Equal(t, []string{"a", "b", "c"}, got)
}

func TestResolveEffectiveKinds_NoKindsIsNilNoAlloc(t *testing.T) {
	cfg := &Config{}
	assert.Nil(t, resolveEffectiveKinds(cfg, "x.md", nil, nil))
	allocs := testing.AllocsPerRun(100, func() {
		resolveEffectiveKinds(cfg, "x.md", nil, nil)
	})
	assert.Zero(t, allocs, "no kinds must not allocate a dedup map")
}

func TestResolveEffectiveKinds_OneAlloc(t *testing.T) {
	cfg := &Config{}
	fm := []string{"a", "b", "c"}
	allocs := testing.AllocsPerRun(100, func() {
		resolveEffectiveKinds(cfg, "x.md", fm, nil)
	})
	assert.LessOrEqual(t, allocs, 1.0, "result slice only, pre-sized")
}

func TestEffectiveSignature_OneAllocNoKinds(t *testing.T) {
	cfg := &Config{}
	allocs := testing.AllocsPerRun(100, func() {
		EffectiveSignature(cfg, "x.md", nil, nil)
	})
	assert.LessOrEqual(t, allocs, 1.0)
}
