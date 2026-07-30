package rename

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// noRefDefBody is a multi-KB body with several headings and prose
// paragraphs but no `]:` substring anywhere, so it can never contain a
// reference definition. It exercises the cheap pre-check in
// validRefDefMatches: without it, every call does a full CommonMark
// parse plus a contentBlockLines AST walk to answer "no matches".
func noRefDefBody() []byte {
	var b strings.Builder
	for i := 0; i < 40; i++ {
		b.WriteString("## Heading number ")
		b.WriteString(strings.Repeat("x", 1))
		b.WriteString("\n\n")
		b.WriteString(strings.Repeat(
			"This is an ordinary paragraph of prose with no reference "+
				"definitions in it at all, just plain sentences. ", 6,
		))
		b.WriteString("\n\n")
	}
	return []byte(b.String())
}

// TestValidRefDefMatchesNoRefDefsCheapNoParse pins the "gate expensive
// analyzers behind a cheap pre-check" pattern (see
// docs/development/high-performance-go.md): when body contains no
// `]:` substring at all, validRefDefMatches must return nil without
// running a full CommonMark parse, so its allocation count on such
// input stays tiny regardless of body size.
func TestValidRefDefMatchesNoRefDefsCheapNoParse(t *testing.T) {
	body := noRefDefBody()
	require.NotContains(t, string(body), "]:")

	var got []validRefDefMatch
	allocs := testing.AllocsPerRun(100, func() {
		got = validRefDefMatches(body)
	})
	require.Nil(t, got)
	require.Lessf(t, allocs, 3.0,
		"validRefDefMatches allocated %v times per call; expected a cheap "+
			"pre-check to skip the parse when body has no ']:' substring", allocs)
}
