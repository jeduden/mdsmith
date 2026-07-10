package tokenbudget

import (
	"strings"
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
)

// BenchmarkCheck_OverBudget measures the violating-file cost, dominated
// by the diagnostic message build. docs/development/high-performance-go.md
// recommends strconv over fmt.Sprintf ("~3x faster... skips reflection");
// this benchmark is the evidence for that swap (checked with benchstat
// old.txt new.txt per the doc's Process). count/budget here exceed 99
// (real token budgets run in the thousands), matching production usage.
func BenchmarkCheck_OverBudget(b *testing.B) {
	var sb strings.Builder
	for i := 0; i < 200; i++ {
		sb.WriteString("word ")
	}
	src := []byte(sb.String())
	f, err := lint.NewFile("test.md", src)
	if err != nil {
		b.Fatal(err)
	}
	r := &Rule{Max: 100, Mode: "heuristic", TokensPerWord: 1.0}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = r.Check(f)
	}
}
