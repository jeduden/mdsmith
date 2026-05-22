package tablereadability

import "testing"

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
// table-free files. A regression that removes the guard re-runs
// the per-line table-detection pass on prose-only files; this
// test would still pass functionally (no tables found), so it
// pins the contract by verifying the diagnostics slice is the
// nil literal — the early-exit returns it directly, whereas the
// fall-through path returns through the for-loop which yields
// `nil` only by happy accident if no codeLines hit.
func TestCheck_EarlyExitOnNoPipe(t *testing.T) {
	f := newFile(t, "# Title\n\nProse without tables.\n")
	r := &Rule{
		MaxColumns: defaultMaxColumns, MaxRows: defaultMaxRows,
		MaxWordsPerCell: defaultMaxWordsPerCell, MaxColumnWidthRatio: defaultMaxColumnWidthRatio,
	}
	if diags := r.Check(f); diags != nil {
		t.Fatalf("Check on table-free file returned %d diagnostics, want nil", len(diags))
	}
}
