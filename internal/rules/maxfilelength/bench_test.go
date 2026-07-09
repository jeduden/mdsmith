package maxfilelength

import (
	"strings"
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
)

// BenchmarkCheck_OverLimit measures the violating-file cost, dominated
// by the diagnostic message build. docs/development/high-performance-go.md
// recommends strconv over fmt.Sprintf ("~3x faster... skips reflection");
// this benchmark is the evidence for that swap (checked with benchstat
// old.txt new.txt per the doc's Process).
func BenchmarkCheck_OverLimit(b *testing.B) {
	var sb strings.Builder
	for i := 0; i < 301; i++ {
		sb.WriteString("line\n")
	}
	src := []byte(sb.String())
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		b.Fatal(err)
	}
	r := &Rule{Max: 300}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = r.Check(f)
	}
}
