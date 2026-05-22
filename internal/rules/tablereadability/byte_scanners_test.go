package tablereadability

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/require"
)

// TestDetectPrefix pins the byte-scan replacement that plan 195
// landed: each case exercises a path the previous string-based
// implementation (`s := string(line); strings.TrimLeft; HasPrefix;
// Index "|"`) decided, so a future regression that drifts from
// the upstream GFM prefix grammar fails here.
func TestDetectPrefix(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"non_blockquote_plain_table", "| a | b |", ""},
		{"non_blockquote_indented_table", "  | a | b |", "  "},
		{"non_pipe_line", "regular prose", ""},
		{"text_before_pipe", "text | mid", ""},
		{"blockquote_space_table", "> | a | b |", "> "},
		{"nested_blockquote_table", "> > | a | b |", "> > "},
		// ">>" is parsed as `>` (the bare-then-> case) plus a
		// trailing `> ` consumption. The walker writes `>` and then
		// `> ` to the prefix buffer, no separator, so the byte
		// stream is `>>` followed by the trailing space — matching
		// the original strings.Builder shape.
		{"bare_blockquote_then_blockquote", ">> | a | b |", ">> "},
		{"indented_blockquote", "  > | a | b |", "  > "},
		// detectPrefix returns the blockquote prefix even when the
		// inner content is not a table — the caller's isTableRow
		// check then bails. Pinning the result documents that the
		// helper does not duplicate that downstream check.
		{"blockquote_only_no_table", "> just prose", "> "},
		{"empty_line", "", ""},
		{"only_spaces", "    ", ""},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			got := detectPrefix([]byte(c.in))
			if got != c.want {
				t.Fatalf("detectPrefix(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

// TestFindTables_SkipsNonPipeLines covers the plan-195 inner
// guard that skips tryParseTable on lines without `|`. The
// fixture's prose lines must not trigger the table parser; a
// regression that removes the guard still parses every line.
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
	got := findTables(lines, map[int]bool{})
	if len(got) != 1 {
		t.Fatalf("findTables returned %d tables, want 1", len(got))
	}
	if got[0].startLine != 5 {
		t.Fatalf("table startLine = %d, want 5", got[0].startLine)
	}
}

// TestCheck_EarlyExitOnNoPipe covers the plan-195 file-level
// early-exit that skips the AST walk for code-block lines on
// table-free files. The previous `diags == nil` assertion did
// not pin the optimisation: both the early-exit and the
// fall-through path return nil when no tables are found, so a
// rollback that removes the guard would have passed silently.
// Measuring on a cold File (fresh per iteration, parse baseline
// subtracted) reveals the AST walk's allocation footprint:
// with the guard the delta is 0; without it the delta is
// several allocs from the code-block walk and findTables.
func TestCheck_EarlyExitOnNoPipe_AllocsZero(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	src := []byte("# Title\n\nProse without tables.\n")
	r := &Rule{
		MaxColumns: defaultMaxColumns, MaxRows: defaultMaxRows,
		MaxWordsPerCell: defaultMaxWordsPerCell, MaxColumnWidthRatio: defaultMaxColumnWidthRatio,
	}
	const runs = 100
	parse := testing.AllocsPerRun(runs, func() {
		_, err := lint.NewFile("p.md", src)
		require.NoError(t, err)
	})
	full := testing.AllocsPerRun(runs, func() {
		f, err := lint.NewFile("c.md", src)
		require.NoError(t, err)
		_ = r.Check(f)
	})
	delta := full - parse
	require.Zero(t, delta,
		"Check on a no-pipe file should hit the bytes.IndexByte "+
			"early-exit and add 0 allocs over parse; "+
			"got %.1f/op (full=%.1f, parse=%.1f)", delta, full, parse)
}
