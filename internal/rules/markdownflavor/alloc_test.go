package markdownflavor

import (
	"fmt"
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// manyAlertsSource builds a document with n GitHub-alert blockquotes,
// each with a continuation line, so fixGitHubAlerts has real marker
// and lazy-continuation lines to strip/re-prefix on every call.
func manyAlertsSource(n int) []byte {
	var src []byte
	for i := 0; i < n; i++ {
		src = append(src, []byte(fmt.Sprintf(
			"> [!NOTE]\n> Alert number %d with some body text.\n> Second continuation line here.\n\n", i,
		))...)
	}
	return src
}

// TestFixGitHubAlerts_PresizedAllocs pins
// docs/development/high-performance-go.md's "pre-size slices" pattern:
// fixGitHubAlerts knows its output has at most len(f.Lines) entries
// (it only ever skips lines, never adds any), so out should be
// allocated once via make([]string, 0, len(f.Lines)) instead of
// growing from a nil slice across repeated append calls, mirroring
// codeblockstyle.Fix's out slice in the same rule family. Combined
// with TestBuildAlertSkipMaps_StaysInBytes below (fixGitHubAlerts
// calls buildAlertSkipMaps first), measured baseline is 113 allocs/op
// on this fixture, down from 169 before either fix.
func TestFixGitHubAlerts_PresizedAllocs(t *testing.T) {
	src := manyAlertsSource(50)
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}

	allocs := testing.AllocsPerRun(50, func() {
		_ = r.fixGitHubAlerts(f)
	})
	assert.LessOrEqualf(t, allocs, 125.0,
		"fixGitHubAlerts allocs regressed: got %v, want <= 125", allocs)
}

// TestBuildAlertSkipMaps_StaysInBytes pins
// docs/development/high-performance-go.md's "stay in []byte" pattern:
// buildAlertSkipMaps used to convert every lazy-continuation line to a
// string (string(f.Lines[contLine-1])) just to strip whitespace and
// check a '>' prefix — both doable directly on the []byte line. The
// string conversion escapes (its result crosses the strings.HasPrefix
// call), so each continuation line paid a full copy-and-allocate.
// Measured baseline after the []byte rewrite: 10 allocs/op on this
// fixture (150 continuation lines), down from 60 before.
func TestBuildAlertSkipMaps_StaysInBytes(t *testing.T) {
	src := manyAlertsSource(50)
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	allocs := testing.AllocsPerRun(50, func() {
		_, _ = buildAlertSkipMaps(f)
	})
	assert.LessOrEqualf(t, allocs, 20.0,
		"buildAlertSkipMaps allocs regressed: got %v, want <= 20", allocs)
}
