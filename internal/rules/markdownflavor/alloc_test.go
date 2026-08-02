package markdownflavor

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// manyAlertLinesSource builds a document with n GitHub-alert blockquotes,
// each with an indented lazy-continuation line (no "> " prefix in the raw
// source), so fixGitHubAlerts exercises both its skip (marker-line
// removal) and addPrefix (continuation-line rewrite) branches n times
// per call.
func manyAlertLinesSource(n int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		b.WriteString("> [!NOTE]\n   indented lazy continuation body.\n\n")
	}
	return b.String()
}

// TestFixGitHubAlerts_LowAllocs pins docs/development/high-performance-go.md's
// "pre-size slices" and "stay in []byte" patterns: fixGitHubAlerts rebuilds
// the whole file line by line, so every unmodified line should pass through
// without a string(line) copy, and the accumulator slice should be
// presized from len(f.Lines) instead of growing via repeated append.
func TestFixGitHubAlerts_LowAllocs(t *testing.T) {
	if testing.Short() {
		t.Skip("alloc gate skipped in -short mode")
	}
	if raceEnabled {
		t.Skip("alloc gate skipped under -race; the race detector's " +
			"instrumentation overhead perturbs the allocation count")
	}
	r := &Rule{}
	f := mkFile(t, manyAlertLinesSource(50))

	allocs := testing.AllocsPerRun(50, func() {
		r.fixGitHubAlerts(f)
	})
	// Measured on this addPrefix-heavy 50-alert fixture: 176 allocs on
	// the pre-fix code (one string(line) copy per source line plus an
	// unsized, growing []string), 120 after (the remaining allocs are
	// buildAlertSkipMaps's own map/AST-walk bookkeeping plus one
	// presized rewritten-line buffer per addPrefix line — the blank
	// and skip lines that pass through unchanged no longer pay a
	// string(line) copy). Budget with headroom over the measured
	// post-fix count.
	assert.LessOrEqualf(t, allocs, 150.0,
		"fixGitHubAlerts allocs regressed: got %v, want <= 150", allocs)
}
