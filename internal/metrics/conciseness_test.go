package metrics

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

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
	text := strings.Repeat("the quick brown fox jumps over it. ", 500)
	allocs := testing.AllocsPerRun(10, func() {
		concisenessScore(text, 500)
	})
	assert.LessOrEqual(t, allocs, 5.0)
}
