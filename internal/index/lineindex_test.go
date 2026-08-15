package index

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

// TestLineOfOffsetIndexed_MatchesOracle pins lineOfOffsetIndexed's
// O(log n) binary-search result against lineOfOffset's O(offset)
// rescan — the function it replaces in the hot per-symbol loop —
// across every byte boundary (plus out-of-range on both ends) for a
// range of source shapes.
func TestLineOfOffsetIndexed_MatchesOracle(t *testing.T) {
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
		nl := newlineIndex(src)
		for off := -3; off <= len(src)+3; off++ {
			want := lineOfOffset(src, off)
			got := lineOfOffsetIndexed(nl, off)
			if got != want {
				t.Errorf("src=%q offset=%d: lineOfOffsetIndexed=%d want %d", src, off, got, want)
			}
		}
	}
}

// TestColumnOfLineIndexed_MatchesOracle pins columnOfLineIndexed
// against columnOfLine across every line and a range of offsets per
// source.
func TestColumnOfLineIndexed_MatchesOracle(t *testing.T) {
	sources := [][]byte{
		[]byte("a\nb\nc"),
		[]byte("line1\nline2\nline3\n"),
		[]byte("héllo\nwörld\n€\n"),
	}
	for _, src := range sources {
		lines := bytes.Split(src, []byte("\n"))
		nl := newlineIndex(src)
		for lineIdx := -1; lineIdx <= len(lines); lineIdx++ {
			for off := -3; off <= len(src)+3; off++ {
				want := columnOfLine(lines, lineIdx, off, src)
				got := columnOfLineIndexed(nl, lines, lineIdx, off)
				if got != want {
					t.Errorf("src=%q lineIdx=%d offset=%d: columnOfLineIndexed=%d want %d",
						src, lineIdx, off, got, want)
				}
			}
		}
	}
}

// manyHeadingsDoc builds a document with n headings, each followed by
// a short paragraph, so buildFileEntry's heading-outline pass does
// real work proportional to n.
func manyHeadingsDoc(n int) []byte {
	var b strings.Builder
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "## Heading %d\n\nParagraph body text for heading %d "+
			"with enough words to be realistic prose content here.\n\n", i, i)
	}
	return []byte(b.String())
}

// TestBuildFileEntryHeadingsScaleLinearly guards against the exact
// anti-pattern buildFileEntry's heading/link-ref-def/directive
// collectors once had: lineOfOffset and columnOfLine rescanning from
// byte 0 on every call instead of reusing a memoized newline index,
// which turns a single buildFileEntry call into O(headings x
// file-size) instead of O(file-size) — see
// docs/development/high-performance-go.md "Memoize per-input
// computations".
//
// A document with 4x the headings (and roughly 4x the bytes) must
// cost close to 4x as long, not 4x^2 = 16x, which is what the
// unmemoized rescan produces.
func TestBuildFileEntryHeadingsScaleLinearly(t *testing.T) {
	small := manyHeadingsDoc(300)
	large := manyHeadingsDoc(1200) // 4x headings/bytes

	smallNs := testing.Benchmark(func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			buildFileEntry("doc.md", small)
		}
	}).NsPerOp()
	largeNs := testing.Benchmark(func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			buildFileEntry("doc.md", large)
		}
	}).NsPerOp()

	ratio := float64(largeNs) / float64(smallNs)
	t.Logf("small(300 headings)=%dns large(1200 headings)=%dns ratio=%.2f", smallNs, largeNs, ratio)
	// Linear scaling lands close to 4x; a quadratic rescan lands close
	// to 16x. 8x is a generous midpoint that still catches the
	// regression without being sensitive to constant-factor noise.
	if ratio > 8 {
		t.Fatalf("buildFileEntry heading scan looks quadratic: 4x input -> %.2fx time (want well under 16x)", ratio)
	}
}
