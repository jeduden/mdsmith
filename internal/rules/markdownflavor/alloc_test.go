package markdownflavor

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// manyAlertLinesSource builds a document with n GitHub-alert blockquotes
// so fixGitHubAlerts rewrites n lines per call.
func manyAlertLinesSource(n int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		b.WriteString("> [!NOTE]\n> Body line for alert number.\n\n")
	}
	return b.String()
}

// TestFixGitHubAlerts_LowAllocs pins docs/development/high-performance-go.md's
// "pre-size slices" and "stay in []byte" patterns: fixGitHubAlerts rebuilds
// the whole file line by line, so every unmodified line should pass through
// without a string(line) copy, and the accumulator slice should be
// presized from len(f.Lines) instead of growing via repeated append.
func TestFixGitHubAlerts_LowAllocs(t *testing.T) {
	r := &Rule{}
	f := mkFile(t, manyAlertLinesSource(50))

	allocs := testing.AllocsPerRun(50, func() {
		r.fixGitHubAlerts(f)
	})
	// Baseline before this fix: 68 allocs (one string(line) copy per
	// source line plus an unsized, growing []string). After: 12 (the
	// remaining allocs are buildAlertSkipMaps's own map/AST-walk
	// bookkeeping, unrelated to the line-rebuild loop this test
	// targets). Budget with headroom over the measured post-fix count.
	assert.LessOrEqualf(t, allocs, 16.0,
		"fixGitHubAlerts allocs regressed: got %v, want <= 16", allocs)
}
