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
		{
			name:  "no tables",
			lines: blines("# Title", "", "Prose paragraph."),
		},
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
		{
			name:  "header only no separator",
			lines: blines("| A | B |", "not a separator"),
		},
		{
			name:  "inclusive end index",
			lines: blines("| Col1 | Col2 |", "|------|------|", "| v1   | v2   |", "| v3   | v4   |"),
			want:  [][2]int{{0, 3}},
		},
	}
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
