package tablereadability

import (
	"testing"
	"unsafe"

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
	got := findTables(lines, map[int]struct{}{})
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
	if raceEnabled {
		// The race detector's allocation bookkeeping perturbs
		// AllocsPerRun counts; a strict delta == 0 assertion
		// flakes under `go test -race ./...`. The optimisation
		// is for production builds, so skipping race builds
		// keeps the gate stable. Same pattern as the
		// alloc-budget tests in this package.
		t.Skip("alloc gate skipped under -race")
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
	if delta < 0 {
		// Clamp small negative deltas — AllocsPerRun averages
		// across iterations and can report negative when the
		// parse and check passes hit different GC states.
		delta = 0
	}
	require.Zero(t, delta,
		"Check on a no-pipe file should hit the bytes.IndexByte "+
			"early-exit and add 0 allocs over parse; "+
			"got %.1f/op (full=%.1f, parse=%.1f)", delta, full, parse)
}

// TestCountWords pins the byte-scan word-count replacement for
// `len(strings.Fields(string(b)))`. The cases exercise the
// ASCII fast path, multi-byte Unicode (CJK + non-breaking space
// + en-quad), leading and trailing whitespace, and mixed runs
// — every branch of countWords + isUnicodeSpace + asciiSpace.
// Plan 195 task 2.
func TestCountWords(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want int
	}{
		{"empty", "", 0},
		{"single", "hello", 1},
		{"two", "hello world", 2},
		{"ascii_tabs", "a\tb\tc", 3},
		{"ascii_newline", "a\nb", 2},
		{"ascii_carriage", "a\rb", 2},
		{"ascii_vtab", "a\vb", 2},
		{"ascii_formfeed", "a\fb", 2},
		{"leading_space", "  hello", 1},
		{"trailing_space", "hello  ", 1},
		{"only_spaces", "   ", 0},
		// Unicode whitespace: NEL (0x85), NBSP (0xA0), Ogham (0x1680),
		// en-quad (0x2000), line separator (0x2028), paragraph (0x2029),
		// narrow NBSP (0x202F), medium math (0x205F), ideographic (0x3000)
		{"nbsp", "a b", 2},
		{"nel", "a\u0085b", 2},
		{"ogham", "a b", 2},
		{"en_quad", "a b", 2},
		{"line_sep", "a b", 2},
		{"para_sep", "a b", 2},
		{"narrow_nbsp", "a b", 2},
		{"med_math_space", "a b", 2},
		{"ideographic_space", "a　b", 2},
		// CJK chars are word-characters, not whitespace.
		{"cjk_run", "日本語", 1},
		{"cjk_space_cjk", "日本　語", 2},
		// Unicode runes that are NOT whitespace fall through.
		{"non_space_high_codepoint", "aÿb", 1},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			if got := countWords([]byte(c.in)); got != c.want {
				t.Fatalf("countWords(%q) = %d, want %d", c.in, got, c.want)
			}
		})
	}
}

// TestSplitRow_ByteSlices pins that splitRow returns sub-slices
// aliased into the input row (not freshly allocated strings). The
// alloc-budget gate depends on the no-per-cell-string promise; a
// regression that copies cells out into fresh storage would still
// pass functional tests but blow the budget.
func TestSplitRow_ByteSlices(t *testing.T) {
	row := []byte("| a | b | c |")
	cells := splitRow(row)
	if len(cells) != 3 {
		t.Fatalf("splitRow returned %d cells, want 3", len(cells))
	}
	for i, cell := range cells {
		if len(cell) == 0 {
			continue
		}
		// Every non-empty cell must lie inside the row buffer. The
		// address compare uses unsafe.Pointer because Go's `==`
		// on `*byte` does not return an ordering and the slice
		// headers themselves have no public accessor for the
		// underlying data pointer.
		rowStart := &row[0]
		cellStart := &cell[0]
		if uintptr(unsafe.Pointer(cellStart)) < uintptr(unsafe.Pointer(rowStart)) ||
			uintptr(unsafe.Pointer(cellStart)) >= uintptr(unsafe.Pointer(rowStart))+uintptr(len(row)) {
			t.Fatalf("cell %d backing array is not inside row buffer", i)
		}
	}
}

// TestIsUnicodeSpace exercises every arm of the isUnicodeSpace
// switch so a regression that drops a Unicode whitespace rune
// trips a coverage gate. Callers in production only reach this
// helper from countWords when the byte is ≥ utf8.RuneSelf, so
// the ASCII arms cannot fire from the production path — but a
// direct call exercises them and pins the table.
func TestIsUnicodeSpace(t *testing.T) {
	wanted := []rune{
		' ', '\t', '\n', '\v', '\f', '\r',
		0x85, 0xA0, 0x1680,
		0x2000, 0x2001, 0x2002, 0x2003, 0x2004, 0x2005,
		0x2006, 0x2007, 0x2008, 0x2009, 0x200A,
		0x2028, 0x2029, 0x202F, 0x205F, 0x3000,
	}
	for _, r := range wanted {
		if !isUnicodeSpace(r) {
			t.Errorf("isUnicodeSpace(%U) = false, want true", r)
		}
	}
	notWanted := []rune{'a', 'Z', '0', '日', 0x1F00, 0x4000}
	for _, r := range notWanted {
		if isUnicodeSpace(r) {
			t.Errorf("isUnicodeSpace(%U) = true, want false", r)
		}
	}
}

// TestStripPrefix_LineShorterThanPrefix covers the
// `len(line) < len(prefix)` guard added by the byte-scanner
// refactor. A blockquote-prefixed table's prefix is longer
// than a blank line that follows, so the guard fires in real
// fixtures; this test pins it explicitly.
func TestStripPrefix_LineShorterThanPrefix(t *testing.T) {
	out := stripPrefix([]byte("ab"), "abcd")
	if string(out) != "ab" {
		t.Fatalf("stripPrefix returned %q, want %q", out, "ab")
	}
}

// TestStripPrefix_PrefixMismatch covers the byte-by-byte
// comparison loop returning early when a non-matching byte
// is found.
func TestStripPrefix_PrefixMismatch(t *testing.T) {
	out := stripPrefix([]byte("xyabc"), "abc")
	if string(out) != "xyabc" {
		t.Fatalf("stripPrefix returned %q, want %q", out, "xyabc")
	}
}
