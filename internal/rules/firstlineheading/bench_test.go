package firstlineheading

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
)

// BenchmarkCheck_WrongLevel measures the violating-file cost, dominated
// by the diagnostic message build. See the rationale on
// TestCheck_WrongLevel_MessageAllocs in alloc_test.go.
func BenchmarkCheck_WrongLevel(b *testing.B) {
	src := []byte("## Not H1\n\nText\n")
	f := lint.NewFileLines("f.md", src)
	r := &Rule{Level: 1}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = r.Check(f)
	}
}
