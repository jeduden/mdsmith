package build

import "testing"

// hasReservedDeviceNameBudget pins docs/development/high-performance-go.md's
// "compile regexes at package scope" sibling pattern for case folding: gate
// the strings.ToUpper allocation behind a cheap length check instead of
// converting every path segment. reservedDeviceNames only holds 3-4 byte
// entries (CON, PRN, COM1..9, LPT1..9), so a segment outside that length
// range can never match and must not pay for the case-fold copy. Measured
// baseline on a 3-path, 7-segment fixture: 10 allocs/op before the length
// gate, 5 after (the two segments that do fall in the 3-4 byte range still
// allocate, which is correct — only the always-allocate-on-every-segment
// behavior regressed).
const hasReservedDeviceNameAllocBudget = 5

// hasReservedDeviceNameFixturePaths mirrors a small set of real repo-style
// paths: a mix of segment lengths, most outside the reserved-name range.
var hasReservedDeviceNameFixturePaths = []string{
	"docs/readme.md",
	"internal/rules/foo.go",
	"scripts/build.sh",
}

// TestHasReservedDeviceName_AllocBudget pins the allocation regression gate
// under a normal `go test` run (not only `-bench`), matching the project's
// paragraphstructure.TestCheckAllocBudget convention.
func TestHasReservedDeviceName_AllocBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("alloc gate skipped in -short mode")
	}

	allocs := testing.AllocsPerRun(200, func() {
		for _, p := range hasReservedDeviceNameFixturePaths {
			_ = hasReservedDeviceName(p)
		}
	})
	t.Logf("hasReservedDeviceName allocs/op over %d paths = %.1f (budget = %d)",
		len(hasReservedDeviceNameFixturePaths), allocs, hasReservedDeviceNameAllocBudget)
	if allocs > float64(hasReservedDeviceNameAllocBudget) {
		t.Fatalf("hasReservedDeviceName allocs/op = %.1f, budget = %d; "+
			"the length gate before strings.ToUpper may have regressed",
			allocs, hasReservedDeviceNameAllocBudget)
	}
}

// TestHasReservedDeviceName_Correctness pins that the length gate does not
// change which paths are flagged: reserved names at every valid length
// (3 and 4 bytes) still match, in any case, and non-reserved segments of
// any length (including 3-4 byte look-alikes) do not.
func TestHasReservedDeviceName_Correctness(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"CON", true},
		{"con", true},
		{"dir/NUL.txt", true},
		{"COM1.log", true},
		{"com9", true},
		{"LPT9", true},
		{"CONSOLE.md", false},
		{"docs/readme.md", false},
		{"foo", false},
		{"bar/baz.md", false},
	}
	for _, c := range cases {
		if got := hasReservedDeviceName(c.path); got != c.want {
			t.Errorf("hasReservedDeviceName(%q) = %v, want %v", c.path, got, c.want)
		}
	}
}

// BenchmarkHasReservedDeviceName reports allocs/op alongside ns/op so a
// regression shows up in `go test -bench` output too.
func BenchmarkHasReservedDeviceName(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		for _, p := range hasReservedDeviceNameFixturePaths {
			_ = hasReservedDeviceName(p)
		}
	}
}
