package toc

import "testing"

// BenchmarkEscapeLinkText pins escapeLinkText's per-TOC-item cost.
// escapeLinkText runs once per heading captured by Generate, which
// gensection.Engine.Check invokes on every Check() call for a file
// containing a <?toc?> directive (to detect staleness), not only on
// Fix(). The common case is a heading with no backslash/bracket to
// escape at all — the three chained strings.ReplaceAll calls each scan
// the whole string looking for nothing, where one pass would do.
func BenchmarkEscapeLinkText_NoEscaping(b *testing.B) {
	s := "Getting Started with the mdsmith CLI"
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		escapeLinkText(s)
	}
}

func BenchmarkEscapeLinkText_WithEscaping(b *testing.B) {
	s := "Config [overrides] and \\special\\ cases"
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		escapeLinkText(s)
	}
}
