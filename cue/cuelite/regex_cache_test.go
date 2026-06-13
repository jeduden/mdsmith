package cuelite

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCachedRegexp_SamePointer verifies the cache returns the same compiled
// *regexp.Regexp for repeated calls with the same pattern. This test is RED
// until cachedRegexp is added to eval.go.
func TestCachedRegexp_SamePointer(t *testing.T) {
	pat := "^test[0-9]+$"
	re1, err := cachedRegexp(pat)
	require.NoError(t, err)
	re2, err := cachedRegexp(pat)
	require.NoError(t, err)
	assert.Same(t, re1, re2, "cachedRegexp must return the same *regexp.Regexp on cache hit")
}

// TestCachedRegexp_ZeroAllocsOnHit verifies that a warm (cached) call to
// cachedRegexp allocates zero objects — the regex is not recompiled.
func TestCachedRegexp_ZeroAllocsOnHit(t *testing.T) {
	pat := "^cached[a-z]+$"
	_, err := cachedRegexp(pat) // warm: populate cache
	require.NoError(t, err)
	avg := testing.AllocsPerRun(100, func() {
		_, _ = cachedRegexp(pat)
	})
	assert.Equal(t, 0.0, avg, "cached cachedRegexp must allocate 0 per call")
}

// TestCachedRegexp_InvalidPattern verifies that invalid patterns return an error.
func TestCachedRegexp_InvalidPattern(t *testing.T) {
	_, err := cachedRegexp("(unclosed")
	assert.Error(t, err)
}
