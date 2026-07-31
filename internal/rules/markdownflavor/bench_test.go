package markdownflavor

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/require"
)

// BenchmarkBuildAlertSkipMaps is a manual regression-detection tool for
// buildAlertSkipMaps' continuation-line scan, not a CI-enforced gate:
// MDS034 is opt-in, so no CI benchmark job runs this package, and the
// metric here (ns/op) is too environment-sensitive for a hard b.Fatalf
// budget the way the allocs-based gates in alloc_test.go are. Run it
// manually with `-bench` and compare via benchstat before/after a
// change to this function — f.LineOfOffset is a binary search over f's
// cached newline index; flavor.LineCol, used here before, rescans
// source[:offset] with bytes.Count/LastIndexByte on every call, which
// made this loop O(n) per continuation line (O(n^2) over a long alert
// paragraph). See the commit that introduced this benchmark for a
// recorded benchstat comparison.
func BenchmarkBuildAlertSkipMaps(b *testing.B) {
	src := []byte(fixGitHubAlertsFixture(500))
	f, err := lint.NewFile("bench.md", src)
	require.NoError(b, err)

	b.ReportAllocs()
	for b.Loop() {
		_, _ = buildAlertSkipMaps(f)
	}
}
