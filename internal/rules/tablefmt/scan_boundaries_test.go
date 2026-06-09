package tablefmt

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScanTableBoundaries(t *testing.T) {
	cases := []struct {
		name      string
		lines     [][]byte
		codeLines map[int]struct{}
		want      [][2]int
	}{
		{name: "no tables", lines: blines("# Title", "", "Prose paragraph.")},
		{
			name:  "single table",
			lines: blines("# Title", "", "| A | B |", "|---|---|", "| 1 | 2 |", "", "After."),
			want:  [][2]int{{2, 4}},
		},
		{
			name:  "two tables",
			lines: blines("| A | B |", "|---|---|", "| 1 | 2 |", "", "| X | Y |", "|---|---|", "| a | b |"),
			want:  [][2]int{{0, 2}, {4, 6}},
		},
		{
			name:      "skips code-block lines",
			lines:     blines("| A | B |", "|---|---|", "| 1 | 2 |"),
			codeLines: map[int]struct{}{1: {}, 2: {}, 3: {}},
		},
		{name: "header only no separator", lines: blines("| A | B |", "not a separator")},
		{
			name:  "inclusive end index",
			lines: blines("| Col1 | Col2 |", "|------|------|", "| v1   | v2   |", "| v3   | v4   |"),
			want:  [][2]int{{0, 3}},
		},
	}
	runScanCases(t, cases)
}

func TestScanTableBoundariesEdgeCases(t *testing.T) {
	cases := []struct {
		name      string
		lines     [][]byte
		codeLines map[int]struct{}
		want      [][2]int
	}{
		// header on the very last line — no room for a separator row
		{name: "pipe line at end of file", lines: blines("| A | B |")},
		// separator line is a code-block line
		{
			name:      "separator line is code-block line",
			lines:     blines("| A | B |", "|---|---|", "| 1 | 2 |"),
			codeLines: map[int]struct{}{2: {}},
		},
		// a data row is in a code block — table stops before it
		{
			name:      "data row in code block truncates table",
			lines:     blines("| A | B |", "|---|---|", "| 1 | 2 |", "| 3 | 4 |"),
			codeLines: map[int]struct{}{3: {}},
			want:      [][2]int{{0, 1}},
		},
		// line has | and - but cell content fails :?-+:? (e.g. "foo-bar")
		{name: "dash in cell content not a separator", lines: blines("| A | B |", "| foo-bar | baz |")},
	}
	runScanCases(t, cases)
}

func runScanCases(t *testing.T, cases []struct {
	name      string
	lines     [][]byte
	codeLines map[int]struct{}
	want      [][2]int
}) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ScanTableBoundaries(tc.lines, tc.codeLines)
			if tc.want == nil {
				assert.Nil(t, got)
				return
			}
			require.Len(t, got, len(tc.want))
			for i, w := range tc.want {
				assert.Equal(t, w, got[i])
			}
		})
	}
}

func blines(ss ...string) [][]byte {
	out := make([][]byte, len(ss))
	for i, s := range ss {
		out[i] = []byte(s)
	}
	return out
}
