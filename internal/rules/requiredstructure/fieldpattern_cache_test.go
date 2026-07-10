package requiredstructure

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestBuildFieldPattern_CachesCompiledRegex guards against buildFieldPattern
// rebuilding the NFA for a {field} body-sync line that recurs — e.g. a
// repeated template row in a proto.md schema. appendBodySyncFields calls
// buildFieldPattern once per matching body line
// (docs/development/high-performance-go.md#allocations, "Compile regexes at
// package scope"); two calls with identical body text must return the same
// *regexp.Regexp instance instead of compiling twice.
func TestBuildFieldPattern_CachesCompiledRegex(t *testing.T) {
	const body = "Cache test body line with a {field} placeholder for FPCacheTest."
	re1 := buildFieldPattern(body)
	re2 := buildFieldPattern(body)
	assert.Same(t, re1, re2, "expected second call to reuse the cached *regexp.Regexp")
}
