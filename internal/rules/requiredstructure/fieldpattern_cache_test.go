package requiredstructure

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestBuildFieldPattern_CachesCompiledRegex guards against buildFieldPattern
// rebuilding the NFA for a {field} body-sync line that recurs within one
// schema parse — e.g. a repeated template row in a proto.md schema.
// appendBodySyncFields calls buildFieldPattern once per matching body line
// (docs/development/high-performance-go.md#allocations, "Compile regexes at
// package scope"); two calls with identical body text and the same cache
// must return the same *regexp.Regexp instance instead of compiling twice.
// The cache is caller-supplied (see buildFieldPattern's doc comment for why
// it isn't a package-level var): a nil cache must still return a correct,
// freshly compiled pattern every call.
func TestBuildFieldPattern_CachesCompiledRegex(t *testing.T) {
	const body = "Cache test body line with a {field} placeholder for FPCacheTest."
	cache := make(map[string]*regexp.Regexp)
	re1 := buildFieldPattern(body, cache)
	re2 := buildFieldPattern(body, cache)
	assert.Same(t, re1, re2, "expected second call to reuse the cached *regexp.Regexp")
}

func TestBuildFieldPattern_NilCacheStillCorrect(t *testing.T) {
	const body = "Nil-cache body line with a {field} placeholder."
	re1 := buildFieldPattern(body, nil)
	re2 := buildFieldPattern(body, nil)
	assert.NotSame(t, re1, re2, "a nil cache must not be written to")
	assert.True(t, re1.MatchString("Nil-cache body line with a replaced-value placeholder."))
}

// TestBuildFieldPattern_CacheHitZeroAllocs proves the cache actually
// avoids the recompile, the way checkextension_alloc_test.go and
// ismarkdownpath_alloc_test.go prove their EqualFold fixes avoid a
// ToLower allocation: a cache hit is a plain map[string]*regexp.Regexp
// read, so it must cost 0 allocs once the entry is warm.
func TestBuildFieldPattern_CacheHitZeroAllocs(t *testing.T) {
	if testing.Short() {
		t.Skip("alloc gate skipped in -short mode")
	}
	if raceEnabled {
		t.Skip("alloc gate skipped under -race")
	}
	const body = "Alloc test body line with a {field} placeholder for FPAllocTest."
	cache := make(map[string]*regexp.Regexp)
	buildFieldPattern(body, cache) // warm the cache
	allocs := testing.AllocsPerRun(100, func() {
		_ = buildFieldPattern(body, cache)
	})
	if allocs > 0 {
		t.Fatalf("buildFieldPattern: expected 0 allocs/op on a cache hit, got %.0f", allocs)
	}
}
