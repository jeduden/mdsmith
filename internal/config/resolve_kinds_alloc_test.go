package config

import (
	"strconv"
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
	if testing.Short() {
		t.Skip("alloc gate skipped in -short mode")
	}
	cfg := &Config{}
	assert.Nil(t, resolveEffectiveKinds(cfg, "x.md", nil, nil))
	allocs := testing.AllocsPerRun(100, func() {
		resolveEffectiveKinds(cfg, "x.md", nil, nil)
	})
	assert.Zero(t, allocs, "no kinds allocates nothing")
}

func TestResolveEffectiveKinds_OneAlloc(t *testing.T) {
	if testing.Short() {
		t.Skip("alloc gate skipped in -short mode")
	}
	cfg := &Config{}
	fm := []string{"a", "b", "c"}
	allocs := testing.AllocsPerRun(100, func() {
		resolveEffectiveKinds(cfg, "x.md", fm, nil)
	})
	assert.LessOrEqual(t, allocs, 1.0, "result slice only, pre-sized")
}

func TestEffectiveSignature_OneAlloc(t *testing.T) {
	fm := []string{"a", "b", "c"}
	if testing.Short() {
		t.Skip("alloc gate skipped in -short mode")
	}
	cfg := &Config{}
	allocs := testing.AllocsPerRun(100, func() {
		EffectiveSignature(cfg, "x.md", fm, nil)
	})
	assert.LessOrEqual(t, allocs, 2.0, "kinds slice plus one Builder buffer")
}

func TestResolveEffectiveKinds_ManyKindsDedupOrder(t *testing.T) {
	var fm, want []string
	for i := 0; i < 5000; i++ {
		name := "kind" + strconv.Itoa(i)
		want = append(want, name)
		fm = append(fm, name, name) // each twice
	}
	got := resolveEffectiveKinds(&Config{}, "x.md", fm, nil)
	assert.Equal(t, want, got)
}

func TestEffectiveKinds_NilCfgDedup(t *testing.T) {
	assert.Nil(t, EffectiveKinds(nil, "x.md", nil, nil), "no kinds is nil")
}
