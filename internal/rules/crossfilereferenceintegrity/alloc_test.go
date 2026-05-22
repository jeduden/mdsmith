package crossfilereferenceintegrity

import (
	"testing"
	"testing/fstest"
	"time"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/require"
)

// allocBudgetMDS027 mirrors the per-Check ceiling. Plan 195 task 5
// brings MDS027 well under it by lazy-building selfAnchors and the
// per-Check anchorCache, splitting "does target exist" from "build
// the read closure" so the closure only escapes when an anchor
// check actually fires, and caching filepath.Abs at package scope.
const allocBudgetMDS027 = 10

const allocBudgetFixture = "# Document title\n" +
	"\n" +
	"A short prose paragraph for the readability and structural\n" +
	"rules to scan. It stays one paragraph long.\n" +
	"\n" +
	"## Section\n" +
	"\n" +
	"See [other](other.md) and [label][ref] for examples.\n" +
	"\n" +
	"```go\nfunc f() int { return 0 }\n```\n" +
	"\n" +
	"- one item\n" +
	"- two items\n" +
	"\n" +
	"| Col | Other |\n" +
	"|-----|-------|\n" +
	"| a   | b     |\n" +
	"\n" +
	"[ref]: https://example.com/\n"

func mapFSFixture() fstest.MapFS {
	return fstest.MapFS{
		"doc.md":   &fstest.MapFile{Data: []byte(allocBudgetFixture), ModTime: time.Time{}},
		"other.md": &fstest.MapFile{Data: []byte("# Other\n\nBody.\n"), ModTime: time.Time{}},
	}
}

// checkAllocsPerOp returns parse-subtracted allocs/op for r.Check on
// the fixture. Mirrors the integration matrix's shape so a unit-level
// regression catches the same delta the gate would.
func checkAllocsPerOp(tb testing.TB, r *Rule) float64 {
	tb.Helper()
	src := []byte(allocBudgetFixture)
	mfs := mapFSFixture()
	makeFile := func(name string) *lint.File {
		f, err := lint.NewFile(name, src)
		require.NoError(tb, err)
		f.FS = mfs
		f.RootDir = "."
		f.RunCache = lint.NewRunCache()
		return f
	}
	_ = r.Check(makeFile("warm.md"))
	const runs = 100
	parse := testing.AllocsPerRun(runs, func() {
		_ = makeFile("parse.md")
	})
	full := testing.AllocsPerRun(runs, func() {
		_ = r.Check(makeFile("check.md"))
	})
	delta := full - parse
	if delta < 0 {
		delta = 0
	}
	return delta
}

func TestCheckAllocBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("alloc gate skipped in -short mode")
	}
	if raceEnabled {
		t.Skip("alloc gate skipped under -race")
	}
	r := &Rule{}
	allocs := checkAllocsPerOp(t, r)
	t.Logf("MDS027 Check allocs/op = %.0f (budget = %d)",
		allocs, allocBudgetMDS027)
	require.LessOrEqualf(t, allocs, float64(allocBudgetMDS027),
		"MDS027 Check allocs/op = %.0f, budget = %d (plan 195)",
		allocs, allocBudgetMDS027)
}
