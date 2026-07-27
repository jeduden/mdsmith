package extension

// Internal unit tests for unexported helpers in the extension
// package: isTableDelim, applyFootnoteTemplate, and related
// internals.

import (
	"strings"
	"testing"

	"github.com/jeduden/mdsmith/pkg/goldmark/parser"
	"github.com/jeduden/mdsmith/pkg/goldmark/text"
)

func TestApplyFootnoteTemplate_AllBranches(t *testing.T) {
	// Drive all branches:
	//   - fast path (no placeholders) -> return template as-is.
	//   - ^^ found -> substitute index.
	//   - %% found -> substitute refCount.
	cases := []struct {
		name     string
		tmpl     string
		index    int
		refCount int
		want     string
	}{
		{"fast-path", "no placeholders here", 5, 3, "no placeholders here"},
		{"only-index", "idx=^^ end", 7, 0, "idx=7 end"},
		{"only-refs", "refs=%% end", 0, 4, "refs=4 end"},
		{"both", "i=^^ r=%%", 10, 2, "i=10 r=2"},
		{"empty", "", 0, 0, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := string(applyFootnoteTemplate([]byte(c.tmpl), c.index, c.refCount))
			if got != c.want {
				t.Errorf("applyFootnoteTemplate(%q) = %q, want %q", c.tmpl, got, c.want)
			}
		})
	}
}

func TestStrikethroughParser_CloseBlock_Direct(t *testing.T) {
	// strikethroughParser.CloseBlock has a 2-arg signature that
	// doesn't match goldmark's CloseBlocker interface, so the
	// dispatcher never calls it.  Drive it directly.
	p := defaultStrikethroughParser
	p.CloseBlock(nil, nil)
}

func TestTaskCheckBoxParser_CloseBlock_Direct(t *testing.T) {
	p := defaultTaskCheckBoxParser
	p.CloseBlock(nil, nil)
}

func TestDefinitionListParser_Close_Direct(t *testing.T) {
	p := &definitionListParser{}
	p.Close(nil, nil, nil)
}

func TestFootnoteBlockParser_Open_NoBracketAtStart(t *testing.T) {
	// footnoteBlockParser.Open returns nil when pos < 0 (no block
	// offset) or the line doesn't start with '['.  Trigger is '[',
	// so the dispatcher only calls Open when '[' is the trigger,
	// but the function defensively checks.
	bp := &footnoteBlockParser{}
	// Construct a Context with BlockOffset == -1.
	pc := parser.NewContext()
	pc.SetBlockOffset(-1)

	r := newTextReader("not a footnote\n")
	node, state := bp.Open(nil, r, pc)
	if node != nil {
		t.Errorf("Open with no block offset should return nil, got %v", node)
	}
	_ = state
}

func newTextReader(s string) text.Reader {
	return text.NewReader([]byte(s))
}

func TestIsTableDelim_AllBranches(t *testing.T) {
	// Drive each branch:
	//   - IndentWidth > 3 -> false
	//   - allSep (only dashes) -> false
	//   - invalid char -> false
	//   - valid -> true
	cases := []struct {
		name string
		in   string
		want bool
	}{
		{"valid-simple", "---|---", true},
		{"valid-with-colons", ":---|---:|:---:", true},
		{"valid-with-spaces", " --- | --- ", true},
		{"only-dashes-no-pipe", "------", false}, // allSep -> false
		{"invalid-char", "---|--x", false},       // x is not allowed
		{"too-indented", "    ---|---", false},   // IndentWidth > 3
		{"empty", "", false},                     // allSep stays true on empty -> false
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := isTableDelim([]byte(c.in)); got != c.want {
				t.Errorf("isTableDelim(%q) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}

// TestParseDelimiter_AllocBudget pins parseDelimiter's allocation
// cost on a wide table row. cols is already known before the
// alignment loop (bytes.Split has already produced it), so growing
// alignments via a bare append() forces repeated reallocation-and-copy
// as the slice doubles past its initial nil capacity. Pre-sizing with
// make([]ast.Alignment, 0, len(cols)) allocates once. See
// docs/development/high-performance-go.md "Pre-size slices".
func TestParseDelimiter_AllocBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("alloc budget skipped in -short mode")
	}
	const cols = 40
	row := strings.Repeat("---|", cols)
	reader := newTextReader(row)
	seg := text.NewSegment(0, len(row))

	if got := defaultTableParagraphTransformer.parseDelimiter(seg, reader); len(got) != cols {
		t.Fatalf("parseDelimiter returned %d alignments, want %d", len(got), cols)
	}
	allocs := testing.AllocsPerRun(20, func() {
		defaultTableParagraphTransformer.parseDelimiter(seg, reader)
	})
	// One alloc for the pre-sized alignments slice, plus bytes.Split's
	// own backing array; a doubling append would add several more
	// reallocations on a 40-column row.
	if allocs > 3 {
		t.Errorf("parseDelimiter allocates %.1f/op on a %d-column row (bound 3); "+
			"want alignments pre-sized to len(cols) instead of grown via append", allocs, cols)
	}
}

// TestParseDelimiter_EmptyColsReturnsNil pins parseDelimiter's nil
// contract for a delimiter line with no columns (e.g. a bare "|").
// Transform (table.go) uses `alignments == nil` as its "not a
// delimiter row" sentinel; a pre-sized-but-empty non-nil slice would
// satisfy isTableDelim and bytes.Split down to zero columns, pass
// that sentinel check, then fail the header child-count check and
// return — aborting the whole paragraph's table scan instead of just
// skipping this line.
func TestParseDelimiter_EmptyColsReturnsNil(t *testing.T) {
	row := "|"
	reader := newTextReader(row)
	seg := text.NewSegment(0, len(row))
	got := defaultTableParagraphTransformer.parseDelimiter(seg, reader)
	if got != nil {
		t.Errorf("parseDelimiter(%q) = %#v, want nil", row, got)
	}
}
