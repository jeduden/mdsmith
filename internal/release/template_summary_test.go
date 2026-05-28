package release

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/jeduden/mdsmith/internal/templatecheck"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSummaryFrontMatterRenderedThroughRenderString walks every
// `.html` file under `website/layouts/` and asserts none uses
// `.Params.summary` in a forbidden context. The classifier lives
// in `internal/templatecheck`; this is the integration test that
// applies it to the real layouts. No template is exempt —
// baseof.html's meta-description fallback uses
// `{{ if .Params.summary }}{{ $.RenderString ... .Params.summary | plainify }}`
// (no `with`-rebinding) so the scanner verifies it natively.
//
// See docs/development/website-config.md for the safe/forbidden
// shape enumeration and `internal/templatecheck/templatecheck.go`
// for the scanner implementation.
func TestSummaryFrontMatterRenderedThroughRenderString(t *testing.T) {
	layoutsDir := filepath.Join(repoRoot(t), "website", "layouts")

	var violations []templatecheck.Violation
	var ioErrors []string
	scanned := 0
	require.NoError(t, filepath.Walk(layoutsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			ioErrors = append(ioErrors, fmt.Sprintf("walk %s: %v", path, err))
			return nil
		}
		if info.IsDir() || filepath.Ext(path) != ".html" {
			return nil
		}
		rel, relErr := filepath.Rel(layoutsDir, path)
		if relErr != nil {
			ioErrors = append(ioErrors, fmt.Sprintf("rel %s: %v", path, relErr))
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			ioErrors = append(ioErrors, fmt.Sprintf("read %s: %v", path, readErr))
			return nil
		}
		got, scanErr := templatecheck.Scan(rel, string(data))
		if scanErr != nil {
			ioErrors = append(ioErrors, fmt.Sprintf("scan %s: %v", path, scanErr))
			return nil
		}
		violations = append(violations, got...)
		scanned++
		return nil
	}))

	formatted := make([]string, 0, len(violations))
	for _, v := range violations {
		formatted = append(formatted, fmt.Sprintf("%s:%d: %s", v.Path, v.Line, v.Why))
	}
	assert.Empty(t, formatted)
	assert.Empty(t, ioErrors, "filesystem errors during scan")
	// Guard against the test passing vacuously if website/layouts/
	// ever disappears or the walker is misconfigured: at least the
	// `_default/baseof.html` + four page-rendering layouts must
	// have been scanned.
	assert.GreaterOrEqual(t, scanned, 5, "expected to scan at least 5 .html files; got %d", scanned)
}
