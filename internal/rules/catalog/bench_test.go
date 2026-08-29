package catalog

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/jeduden/mdsmith/internal/lint"
)

// BenchmarkCheck_NoCatalogDirective pins Check's CPU cost on a
// representative host file with no <?catalog?> directive — the
// common case for most files in a workspace, since MDS019 is
// default-enabled and every host file pays this path whether or not
// it ever uses the directive. Before the fix, Check unconditionally
// ran gensection.FindMarkerPairs' top-level AST walk three times
// (once via getEngine().Check, once each in checkCaseMismatches and
// checkInjection) regardless of whether the source could contain a
// catalog directive at all. See
// docs/development/high-performance-go.md's "gate expensive
// analyzers behind a cheap pre-check" pattern.
func BenchmarkCheck_NoCatalogDirective(b *testing.B) {
	var sb strings.Builder
	sb.WriteString("# Doc\n\n")
	for range 200 {
		sb.WriteString("## Section\n\n" +
			"A representative paragraph of prose with enough words " +
			"to look like real content in a typical Markdown file.\n\n")
	}
	src := []byte(sb.String())
	fsys := fstest.MapFS{}
	r := &Rule{}

	// Parsing 200 sections dwarfs Check's own cost, so the parse step
	// is built outside the timed region: this benchmark isolates the
	// AST-walk work Check performs, not goldmark's parse.
	b.ReportAllocs()
	b.StopTimer()
	for i := 0; i < b.N; i++ {
		f, err := lint.NewFile("doc.md", src)
		if err != nil {
			b.Fatal(err)
		}
		f.FS = fsys
		f.RootFS = fsys
		b.StartTimer()
		_ = r.Check(f)
		b.StopTimer()
	}
}
