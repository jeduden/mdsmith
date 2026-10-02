package metrics

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// tokenPattern is the regexp scanTokens replaced; it stays here as the
// oracle for the byte scanner.
var tokenPattern = regexp.MustCompile(`[a-z0-9']+`)

func TestScanTokens_MatchesTokenPattern(t *testing.T) {
	cases := []string{
		"", "!!!", "hello world", "it's a don't-do 42nd test",
		"mixed_CASE and ünïcode wörds 日本語 ok", "'' ' a'b", "x\ty\nz",
	}
	for _, c := range cases {
		lower := strings.ToLower(c)
		want := tokenPattern.FindAllString(lower, -1)
		var got []string
		scanTokens(lower, func(tok string) { got = append(got, tok) })
		assert.Equal(t, want, got, "input %q", c)
	}
}

// concisenessScore must not allocate per token; see
// docs/development/high-performance-go.md, "Allocations".
func TestConcisenessScore_AllocsIndependentOfTokenCount(t *testing.T) {
	skipAllocGate(t)
	text := strings.Repeat("the quick brown fox jumps over it. ", 500)
	allocs := testing.AllocsPerRun(10, func() {
		concisenessScore(text, 500)
	})
	assert.LessOrEqual(t, allocs, 5.0)
}
