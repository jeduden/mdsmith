package flavor

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestLineIndex_MatchesLineCol pins lineIndex.lineCol's O(log n)
// binary-search result against LineCol's O(offset) rescan — the
// trusted oracle this package has always exposed — across every byte
// boundary (plus out-of-range on both ends) for a range of source
// shapes, mirroring internal/lint's TestLineOfOffset_MatchesOracle.
func TestLineIndex_MatchesLineCol(t *testing.T) {
	sources := [][]byte{
		nil,
		[]byte(""),
		[]byte("\n"),
		[]byte("no newline"),
		[]byte("a\nb\nc"),
		[]byte("line1\nline2\nline3\n"),
		[]byte("\n\n\nx\n\n"),
		[]byte("héllo\nwörld\n€\n"), // multibyte: offsets are byte-based
	}
	for _, src := range sources {
		idx := newLineIndex(src)
		for off := -3; off <= len(src)+3; off++ {
			wantLine, wantCol := LineCol(src, off)
			gotLine, gotCol := idx.lineCol(off)
			assert.Equalf(t, wantLine, gotLine, "src=%q offset=%d line", src, off)
			assert.Equalf(t, wantCol, gotCol, "src=%q offset=%d col", src, off)
		}
	}
}

// TestNewLazyLineCol_MatchesLineCol exercises the lazy wrapper end to
// end: the first call builds the index, subsequent calls reuse it,
// and every call must still agree with the LineCol oracle.
func TestNewLazyLineCol_MatchesLineCol(t *testing.T) {
	src := []byte("line1\nline2\nline3\nline4\n")
	lineCol := newLazyLineCol(src)
	for _, off := range []int{0, 3, 6, 12, 18, len(src)} {
		wantLine, wantCol := LineCol(src, off)
		gotLine, gotCol := lineCol(off)
		assert.Equalf(t, wantLine, gotLine, "offset=%d line", off)
		assert.Equalf(t, wantCol, gotCol, "offset=%d col", off)
	}
}

// TestNewLazyLineCol_NeverBuildsIndexWithoutACall confirms the lazy
// getter itself imposes no cost until invoked — newLazyLineCol must
// not eagerly scan source. Regression guard for the "common case: zero
// findings" allocation-budget claim in BenchmarkBareURLNoMatches and
// BenchmarkDetectManyFindings's doc comments.
func TestNewLazyLineCol_NeverBuildsIndexWithoutACall(t *testing.T) {
	src := []byte("line1\nline2\nline3\n")
	allocs := testing.AllocsPerRun(200, func() {
		_ = newLazyLineCol(src)
	})
	if allocs > 1 {
		t.Fatalf("newLazyLineCol allocs/op = %.1f, want <= 1 (must not build the index eagerly)", allocs)
	}
}

// --- newlineSearch ---

// TestNewlineSearch pins newlineSearch's binary-search semantics: it returns
// the count of entries in nl (sorted newline byte offsets) that are strictly
// less than offset — the number of newlines preceding offset.
func TestNewlineSearch(t *testing.T) {
	// nl holds the newline positions of "a\nb\nc\n" (0-indexed: 1, 3, 5).
	nl := []int{1, 3, 5}

	tests := []struct {
		offset int
		want   int // number of newlines strictly before offset
	}{
		{0, 0}, // before first newline
		{1, 0}, // exactly at first newline: newline is not "before" offset
		{2, 1}, // after first newline
		{3, 1}, // exactly at second newline
		{4, 2}, // between second and third newline
		{5, 2}, // exactly at third newline
		{6, 3}, // past all newlines
		{7, 3}, // well past end
	}
	for _, tc := range tests {
		got := newlineSearch(nl, tc.offset)
		assert.Equalf(t, tc.want, got, "newlineSearch(nl, %d)", tc.offset)
	}

	// Empty newline list: always returns 0.
	assert.Equal(t, 0, newlineSearch(nil, 0))
	assert.Equal(t, 0, newlineSearch([]int{}, 5))
}
