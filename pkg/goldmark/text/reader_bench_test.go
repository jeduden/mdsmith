package text

import "testing"

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
func BenchmarkBlockReaderValue_MultiLine(b *testing.B) {
	r, seg := multiLineValueFixture()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = r.Value(seg)
	}
}
