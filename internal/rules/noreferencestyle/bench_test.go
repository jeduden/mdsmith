package noreferencestyle

import (
	"strconv"
	"strings"
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
)

// buildLargeRefDoc builds a representative file with n paragraphs, each
// referencing and defining its own link — the shape
// collectReferenceDefinitions' line scan walks byte by byte looking for
// the trailing newline. See docs/development/high-performance-go.md:
// bytes.IndexByte over a hand-rolled byte loop for a whole-Source scan.
func buildLargeRefDoc(n int) []byte {
	var b strings.Builder
	for i := 0; i < n; i++ {
		id := strconv.Itoa(i)
		b.WriteString("See [link " + id + "][ref" + id + "] for details on this paragraph of prose\n")
		b.WriteString("text that pads the line out to a realistic width.\n\n")
		b.WriteString("[ref" + id + "]: https://example.com/" + id + "\n\n")
	}
	return []byte(b.String())
}

// BenchmarkCheck_LargeDocument measures collectReferenceDefinitions'
// whole-source line scan on a document with many reference definitions.
func BenchmarkCheck_LargeDocument(b *testing.B) {
	src := buildLargeRefDoc(500)
	f, err := lint.NewFile("f.md", src)
	if err != nil {
		b.Fatal(err)
	}
	r := &Rule{}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = r.Check(f)
	}
}
