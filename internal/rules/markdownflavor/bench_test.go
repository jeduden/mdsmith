package markdownflavor

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/require"
)

// BenchmarkBuildAlertSkipMaps guards buildAlertSkipMaps' continuation-
// line scan against reverting to a per-line O(offset) line lookup.
// f.LineOfOffset is a binary search over f's cached newline index;
// flavor.LineCol, used here before, rescans source[:offset] with
// bytes.Count/LastIndexByte on every call, which made this loop
// O(n) per continuation line (O(n^2) over a long alert paragraph).
// Compare before/after with benchstat on a change to this function.
func BenchmarkBuildAlertSkipMaps(b *testing.B) {
	src := []byte(fixGitHubAlertsFixture(500))
	f, err := lint.NewFile("bench.md", src)
	require.NoError(b, err)

	b.ReportAllocs()
	for b.Loop() {
		_, _ = buildAlertSkipMaps(f)
	}
}
