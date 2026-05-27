package release

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSummaryFrontMatterRenderedThroughRenderString pins the
// invariant that every reference to `.Params.summary` in Hugo
// templates either checks presence (`{{ if .Params.summary }}`)
// or renders through `.RenderString`. The bug fixed in
// claude/dazzling-archimedes-8sUqZ was a `{{ with .Params.summary
// }}<p>{{ . }}</p>{{ end }}` pattern that rebound `.` to the
// summary string and emitted it verbatim, so a value with
// backticks shipped with literal backticks instead of <code>
// tags. Forbidding the rebinding (`with`) and bare output forms
// makes the bug unauthorable.
//
// baseof.html is exempt: its meta-description fallback intentionally
// uses the raw text (meta tags don't accept HTML).
func TestSummaryFrontMatterRenderedThroughRenderString(t *testing.T) {
	layoutsDir := layoutsPath(t)

	templateExpr := regexp.MustCompile(`\{\{-?[^{}]*-?\}\}`)
	summaryRef := regexp.MustCompile(`\.Params\.summary\b`)
	ifPredicate := regexp.MustCompile(`^\{\{-?\s*if\s+(?:not\s+)?\.Params\.summary\s*-?\}\}$`)
	renderString := regexp.MustCompile(`\.RenderString\b`)

	var violations []string
	require.NoError(t, filepath.Walk(layoutsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || filepath.Ext(path) != ".html" {
			return nil
		}
		if filepath.Base(path) == "baseof.html" {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		rel, _ := filepath.Rel(layoutsDir, path)
		for i, line := range strings.Split(string(data), "\n") {
			for _, expr := range templateExpr.FindAllString(line, -1) {
				if !summaryRef.MatchString(expr) {
					continue
				}
				if ifPredicate.MatchString(expr) {
					continue
				}
				if renderString.MatchString(expr) {
					continue
				}
				violations = append(violations,
					fmt.Sprintf("%s:%d: %s", rel, i+1, expr))
			}
		}
		return nil
	}))

	assert.Empty(t, violations,
		"every .Params.summary reference outside baseof.html must be "+
			"an `if .Params.summary` predicate or use .RenderString; "+
			"a bare `with .Params.summary` rebinds `.` and lets the "+
			"value ship as raw text (the bug fixed in "+
			"PR claude/dazzling-archimedes-8sUqZ)")
}

// layoutsPath returns the absolute path to website/layouts/,
// resolved from this test file's location so the test works
// regardless of the working directory at `go test` time.
func layoutsPath(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok, "runtime.Caller failed")
	// internal/release/template_summary_test.go -> repo root -> website/layouts
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	return filepath.Join(repoRoot, "website", "layouts")
}
