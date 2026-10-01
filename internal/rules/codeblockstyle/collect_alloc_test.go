package codeblockstyle

import (
	"strings"
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/require"
)

// TestCollectBlocks_PreSized pins collectBlocks to a pre-sized result
// (docs/development/high-performance-go.md, "Pre-size slices"): eight
// blocks used to regrow the slice at 1, 2, 4 and 8.
func TestCollectBlocks_PreSized(t *testing.T) {
	if testing.Short() {
		t.Skip("alloc gate skipped in -short mode")
	}
	src := strings.Repeat("```go\nx\n```\n\n", 8)
	f, err := lint.NewFile("many.md", []byte(src))
	require.NoError(t, err)
	require.Len(t, collectBlocks(f), 8)

	allocs := testing.AllocsPerRun(200, func() { collectBlocks(f) })
	require.LessOrEqual(t, allocs, 1.0, "one pre-sized slice; no regrow")
}

func TestCollectBlocks_NoneIsNil(t *testing.T) {
	f, err := lint.NewFile("none.md", []byte("just text\n"))
	require.NoError(t, err)
	require.Nil(t, collectBlocks(f))
}
