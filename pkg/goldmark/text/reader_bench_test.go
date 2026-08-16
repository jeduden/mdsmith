package text

import (
	"math/rand"
	"testing"
)

// multiLineValueFixture builds a *blockReader and a Segment spanning
// all three of its lines, mirroring what FindClosure/parser code sees
// for a multi-line link label or title.
func multiLineValueFixture() (*blockReader, Segment) {
	lines := [][]byte{
		[]byte("first line of a fairly long label text here\n"),
		[]byte("second line continues the label text here too\n"),
		[]byte("third line finishes it off nicely here\n"),
	}
	var source []byte
	segs := NewSegments()
	off := 0
	for _, l := range lines {
		source = append(source, l...)
		segs.Append(NewSegment(off, off+len(l)))
		off += len(l)
	}
	r := &blockReader{source: source}
	r.Reset(segs)
	return r, NewSegment(0, len(source))
}

// TestBlockReader_Value_MultiLine pins the byte-for-byte contract
// Value must keep regardless of how it copies the underlying bytes: a
// segment spanning every line returns exactly the concatenation of
// each line's bytes. This is the correctness net for
// BenchmarkBlockReaderValue_MultiLine's bulk-copy rewrite.
func TestBlockReader_Value_MultiLine(t *testing.T) {
	r, seg := multiLineValueFixture()
	got := r.Value(seg)
	want := string(r.source)
	if string(got) != want {
		t.Fatalf("Value() = %q, want %q", got, want)
	}
}

// TestBlockReaderValue_AllocsPerCall pins Value's allocation count for
// the multi-line fixture at exactly one — ret is pre-sized to
// seg.Stop-seg.Start+1 up front, so neither the bulk-copy rewrite nor
// ConcatPadding's own append (Padding is 0 here) should ever grow it.
// Unlike ns/op (see BenchmarkBlockReaderValue_MultiLine's comment),
// allocation count is immune to scheduling noise, so this is a hard
// regression gate: a future edit that drops the pre-sizing or
// re-slices inside the loop would push this from 1 to 3+ and fail
// here instead of silently costing every multi-line parse.
func TestBlockReaderValue_AllocsPerCall(t *testing.T) {
	r, seg := multiLineValueFixture()
	sink := make([][]byte, 0, 64)
	allocs := testing.AllocsPerRun(64, func() {
		sink = append(sink[:0], r.Value(seg))
	})
	if allocs != 1 {
		t.Fatalf("blockReader.Value allocated %v times per call, want exactly 1", allocs)
	}
}

// referenceBlockReaderValue is Value's pre-bulk-copy implementation,
// kept only in this test as the equivalence oracle
// TestBlockReaderValue_MatchesByteByByteReference checks the current
// implementation against — the byte-by-byte inner loop
// BenchmarkBlockReaderValue_MultiLine's fix replaces.
func referenceBlockReaderValue(r *blockReader, seg Segment) []byte {
	line := r.segmentsLength - 1
	ret := make([]byte, 0, seg.Stop-seg.Start+1)
	for ; line >= 0; line-- {
		if seg.Start >= r.segments.At(line).Start {
			break
		}
	}
	i := seg.Start
	for ; line < r.segmentsLength; line++ {
		s := r.segments.At(line)
		if i < 0 {
			i = s.Start
		}
		ret = s.ConcatPadding(ret)
		for ; i < seg.Stop && i < s.Stop; i++ {
			ret = append(ret, r.source[i])
		}
		i = -1
		if s.Stop > seg.Stop {
			break
		}
	}
	return ret
}

// TestBlockReaderValue_MatchesByteByByteReference fuzzes Value against
// referenceBlockReaderValue over random multi-line sources, random
// per-line padding, and random query segments that start and end
// mid-line (not just line-aligned, and not just the whole source) —
// the shapes the bulk-copy rewrite's boundary math (`end := min(seg
// .Stop, s.Stop)`) most needed to get right and that
// TestBlockReader_Value_MultiLine alone does not exercise.
func TestBlockReaderValue_MatchesByteByByteReference(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	for trial := 0; trial < 500; trial++ {
		numLines := 1 + rng.Intn(5)
		var source []byte
		segs := NewSegments()
		off := 0
		for i := 0; i < numLines; i++ {
			lineLen := 1 + rng.Intn(12)
			line := make([]byte, lineLen)
			for j := range line {
				line[j] = byte('a' + rng.Intn(26))
			}
			line[lineLen-1] = '\n'
			source = append(source, line...)
			padding := rng.Intn(3)
			segs.Append(NewSegmentPadding(off, off+lineLen, padding))
			off += lineLen
		}
		r := &blockReader{source: source}
		r.Reset(segs)

		start := rng.Intn(len(source))
		stop := start + rng.Intn(len(source)-start+1)
		seg := NewSegment(start, stop)

		want := referenceBlockReaderValue(r, seg)
		got := r.Value(seg)
		if string(got) != string(want) {
			t.Fatalf("trial %d: Value(%+v) over %q = %q, want %q (reference)",
				trial, seg, source, got, want)
		}
	}
}

// BenchmarkBlockReaderValue_MultiLine tracks Value's per-call cost for
// a segment spanning multiple lines — the multi-line link-label/title
// path every parse hits. Value's inner copy loop appended one byte at
// a time via `append(ret, r.source[i])`; the compiler does not
// vectorize that (docs/development/high-performance-go.md's
// "bytes.IndexByte over a hand-rolled byte loop" applies just as much
// to a copy loop as a search loop). A single bulk
// `append(ret, r.source[i:end]...)` per line compiles to a memmove.
//
// Measured on the dev box: ~205 ns/op (byte-by-byte) vs ~100 ns/op
// (bulk copy) for this fixture, both allocating once (ret is already
// pre-sized) — roughly 2x. Left as a plain benchmark rather than a
// t.Fatalf-gated test: at this ~100-200 ns scale, OS scheduling noise
// from concurrently-running package test binaries swamps the signal
// (observed 160-250 ns/op for either version under load in this
// environment), the same reason none of the project's other pinned
// budgets operate below millisecond scale. Run manually with
// `-bench=BenchmarkBlockReaderValue_MultiLine` to reproduce the ~2x.
// TestBlockReaderValue_AllocsPerCall pins the allocation axis, which
// is noise-free, as a hard gate.
func BenchmarkBlockReaderValue_MultiLine(b *testing.B) {
	r, seg := multiLineValueFixture()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = r.Value(seg)
	}
}
