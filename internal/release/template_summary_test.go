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
// or renders through `.RenderString`. The regression this guards
// against is `{{ with .Params.summary }}<p>{{ . }}</p>{{ end }}`:
// `with` rebinds `.` to the summary string, and `{{ . }}` then
// emits the value raw — so a summary like "Use `<?catalog?>`..."
// ships with literal backticks instead of <code> tags. Forbidding
// the rebinding (`with`) and bare output forms makes the bug
// unauthorable.
//
// The scan operates on the whole file rather than line by line,
// so a multi-line template action like `{{ with\n  .Params.summary
// }}` is still caught — `[^{}]` in templateExpr matches newlines.
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
		content := string(data)
		rel, _ := filepath.Rel(layoutsDir, path)
		for _, loc := range templateExpr.FindAllStringIndex(content, -1) {
			expr := content[loc[0]:loc[1]]
			if !summaryRef.MatchString(expr) {
				continue
			}
			if ifPredicate.MatchString(expr) {
				continue
			}
			if renderString.MatchString(expr) {
				continue
			}
			line := 1 + strings.Count(content[:loc[0]], "\n")
			violations = append(violations,
				fmt.Sprintf("%s:%d: %s", rel, line, expr))
		}
		return nil
	}))

	assert.Empty(t, violations,
		"every .Params.summary reference outside baseof.html must be "+
			"an `if .Params.summary` predicate or use .RenderString; "+
			"a bare `with .Params.summary` rebinds `.` and lets the "+
			"value ship as raw text instead of rendered Markdown")
}

// TestSummaryFrontMatterCheck_DetectsMultiLineWith pins the
// whole-file scan: a `with .Params.summary` that wraps across
// newlines must still be flagged. The earlier per-line scan
// would have missed this and let the bug regress.
func TestSummaryFrontMatterCheck_DetectsMultiLineWith(t *testing.T) {
	tmp := t.TempDir()
	bad := `<p>
{{ with
  .Params.summary }}
  <span>{{ . }}</span>
{{ end }}
</p>`
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "page.html"), []byte(bad), 0o644))

	templateExpr := regexp.MustCompile(`\{\{-?[^{}]*-?\}\}`)
	summaryRef := regexp.MustCompile(`\.Params\.summary\b`)
	ifPredicate := regexp.MustCompile(`^\{\{-?\s*if\s+(?:not\s+)?\.Params\.summary\s*-?\}\}$`)
	renderString := regexp.MustCompile(`\.RenderString\b`)

	data, err := os.ReadFile(filepath.Join(tmp, "page.html"))
	require.NoError(t, err)
	content := string(data)

	var hits []string
	for _, loc := range templateExpr.FindAllStringIndex(content, -1) {
		expr := content[loc[0]:loc[1]]
		if !summaryRef.MatchString(expr) {
			continue
		}
		if ifPredicate.MatchString(expr) {
			continue
		}
		if renderString.MatchString(expr) {
			continue
		}
		hits = append(hits, expr)
	}
	assert.NotEmpty(t, hits, "multi-line `with .Params.summary` must be detected")
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
