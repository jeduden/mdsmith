package tablefmt

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTryParseTable_FirstLineHasPipeButNotTableRow covers the
// `!isTableRow(content)` arm on the first line. Plan 195's
// new pre-check in findTables (`bytes.IndexByte(lines[i], '|') < 0`)
// removed the previous coverage path: lines without `|` no longer
// reach tryParseTable, so the `!isTableRow` branch only fires
// today when a line has `|` but is not a complete table row
// (e.g. starts with `|` but doesn't end with one). This test
// pins that path so the branch stays covered.
func TestTryParseTable_FirstLineHasPipeButNotTableRow(t *testing.T) {
	lines := [][]byte{
		[]byte("| only-open-pipe"), // has `|` but doesn't end with `|`
		[]byte("|---|---|"),
	}
	tbl, end := tryParseTable(lines, 0, nil)
	require.Nil(t, tbl, "expected nil when the first line is not a valid table row")
	assert.Equal(t, 0, end)
}

// TestFindTables_SkipsNonPipeLines covers the plan-195 fast-path
// in findTables that skips tryParseTable on lines without `|`.
// Pinning the behaviour anchors the optimisation against an
// accidental rollback that would re-trigger the per-line
// detectPrefix allocation.
func TestFindTables_SkipsNonPipeLines(t *testing.T) {
	lines := [][]byte{
		[]byte("# Title"),
		[]byte(""),
		[]byte("Some prose with no pipe."),
		[]byte(""),
		[]byte("| Col | Col2 |"),
		[]byte("|-----|------|"),
		[]byte("| a   | b    |"),
	}
	got := findTables(lines, map[int]struct{}{})
	require.Len(t, got, 1)
	assert.Equal(t, 5, got[0].startLine)
}
