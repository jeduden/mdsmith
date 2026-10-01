package tocdirective

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMatchVariant(t *testing.T) {
	tests := []struct {
		line  string
		token string
	}{
		{"[TOC]", "[TOC]"},
		{"[TOC] \t", "[TOC]"},
		{"[[_TOC_]]", "[[_TOC_]]"},
		{"[[toc]]  ", "[[toc]]"},
		{"${toc}", "${toc}"},
		{"[toc]", ""},
		{"[TOC] x", ""},
		{" [TOC]", ""},
		{"[TOC", ""},
		{"", ""},
		{"text", ""},
		{"[[TOC]]", ""},
	}
	for _, tt := range tests {
		v, ok := matchVariant([]byte(tt.line))
		assert.Equal(t, tt.token != "", ok, "line %q", tt.line)
		assert.Equal(t, tt.token, v.token, "line %q", tt.line)
	}
}

func TestMatchVariant_EveryVariantPassesGate(t *testing.T) {
	for _, v := range variants {
		got, ok := matchVariant(v.literal)
		assert.True(t, ok, v.token)
		assert.Equal(t, v.token, got.token)
	}
}

func TestMatchVariant_LinkRefCandidate(t *testing.T) {
	v, ok := matchVariant([]byte("[TOC]"))
	assert.True(t, ok)
	assert.True(t, v.isLinkRefCandidate)
	v, ok = matchVariant([]byte("${toc}"))
	assert.True(t, ok)
	assert.False(t, v.isLinkRefCandidate)
}

// The doc's "bytes over regexp for a literal" rule: a line that cannot
// be a directive must be rejected without running any regexp.
func TestMatchVariant_RejectNoAlloc(t *testing.T) {
	if testing.Short() {
		t.Skip("alloc gate skipped in -short mode")
	}
	line := []byte("An ordinary paragraph line with no directive.")
	allocs := testing.AllocsPerRun(100, func() { matchVariant(line) })
	assert.Zero(t, allocs)
}

func BenchmarkMatchVariant(b *testing.B) {
	line := []byte("An ordinary paragraph line with no directive.")
	b.ReportAllocs()
	for b.Loop() {
		matchVariant(line)
	}
}
