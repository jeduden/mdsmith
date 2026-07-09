package orderedlistnumbering

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
)

// BenchmarkCheckItem measures the per-violating-item cost of building a
// diagnostic message. docs/development/high-performance-go.md recommends
// strconv over fmt.Sprintf ("~3x faster... skips reflection"); this
// benchmark is the evidence for that swap on this rule's hot per-item
// loop (checked with benchstat old.txt new.txt per the doc's Process).
func BenchmarkCheckItem(b *testing.B) {
	src := []byte("300. a\n")
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		b.Fatal(err)
	}
	r := &Rule{Style: StyleSequential, Start: 1}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, _ = r.checkItem(f, 1, 0)
	}
}
