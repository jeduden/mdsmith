package engine

import (
	"fmt"
	"strings"
	"testing"

	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/stretchr/testify/require"
)

// TestRunSourceParseCache_ContractEqualsColdPath pins the invariant
// the LSP relies on: installing a ParseCache must not change the
// diagnostics RunSourceWithVersion produces. The same corpus is run
// through three variants and the diagnostics from the cached path
// must equal the cold path tuple-for-tuple, in order.
//
//   - cold: RunSource (legacy, no cache, no version)
//   - cached-miss: RunSourceWithVersion with a fresh ParseCache (each
//     call parses, then stores)
//   - cached-hit: RunSourceWithVersion called twice on the same
//     (path, version); the second call must serve from the cache and
//     still produce identical diagnostics
//
// A regression here means a cache hit served a *File that diverged
// from what NewFileFromSource would have produced — exactly the
// staleness the warm-cache LSP path must never observe.
func TestRunSourceParseCache_ContractEqualsColdPath(t *testing.T) {
	cfg := config.Defaults()

	// Three rule-exercising shapes so the contract covers heading,
	// list, code-fence, link, and paragraph-structure paths. Each
	// entry's path is workspace-relative because that is the key
	// the LSP hands to RunSourceWithVersion.
	corpus := []struct {
		path string
		body []byte
	}{
		{"docs/headings.md", []byte(buildContractDoc(0, 40, 3))},
		{"docs/sections.md", []byte(buildContractDoc(1, 80, 3))},
		{"docs/long.md", []byte(buildContractDoc(2, 200, 3))},
	}

	newRunner := func(cache *lint.ParseCache) *Runner {
		return &Runner{
			Config:           cfg,
			Rules:            rule.All(),
			StripFrontMatter: true,
			ParseCache:       cache,
		}
	}

	for _, doc := range corpus {
		doc := doc
		t.Run(doc.path, func(t *testing.T) {
			// Cold: no cache, legacy entry.
			cold := newRunner(nil).RunSource(doc.path, doc.body)
			require.Empty(t, cold.Errors, "cold path errors: %v", cold.Errors)

			// Cached miss: fresh cache, version 1, first call must parse.
			cache := lint.NewParseCache()
			miss := newRunner(cache).RunSourceWithVersion(doc.path, doc.body, 1)
			require.Empty(t, miss.Errors)
			requireSameDiagnostics(t, cold.Diagnostics, miss.Diagnostics, "cold vs cached-miss")

			// Cached hit: same (path, version) — second call serves
			// the *File from cache and must produce identical
			// diagnostics. Any divergence means the cache returned
			// a *File that does not match a fresh parse.
			hit := newRunner(cache).RunSourceWithVersion(doc.path, doc.body, 1)
			require.Empty(t, hit.Errors)
			requireSameDiagnostics(t, cold.Diagnostics, hit.Diagnostics, "cold vs cached-hit")
		})
	}
}

// requireSameDiagnostics fails the test when two diagnostic slices
// disagree on length, ordering, or any of the fields the LSP / CLI
// formatters render. Using a field-by-field compare (not reflect.
// DeepEqual on the whole slice) keeps the failure message readable
// when one entry drifts.
func requireSameDiagnostics(t *testing.T, want, got []lint.Diagnostic, label string) {
	t.Helper()
	require.Equalf(t, len(want), len(got),
		"%s: diagnostic count differs (want %d, got %d):\n%s\nvs\n%s",
		label, len(want), len(got), formatDiagnostics(want), formatDiagnostics(got))
	for i := range want {
		w, g := want[i], got[i]
		require.Equalf(t, w.File, g.File, "%s[%d]: File", label, i)
		require.Equalf(t, w.Line, g.Line, "%s[%d]: Line", label, i)
		require.Equalf(t, w.Column, g.Column, "%s[%d]: Column", label, i)
		require.Equalf(t, w.RuleID, g.RuleID, "%s[%d]: RuleID", label, i)
		require.Equalf(t, w.Message, g.Message, "%s[%d]: Message", label, i)
		require.Equalf(t, w.Severity, g.Severity, "%s[%d]: Severity", label, i)
	}
}

// formatDiagnostics renders a slice as a newline-joined string for
// the contract-failure message. Pulled out so the failure path
// allocates only when the assertion actually fires.
func formatDiagnostics(diags []lint.Diagnostic) string {
	var b strings.Builder
	for i, d := range diags {
		fmt.Fprintf(&b, "  [%d] %s:%d:%d %s %s\n", i, d.File, d.Line, d.Column, d.RuleID, d.Message)
	}
	return b.String()
}

// buildContractDoc emits a representative-but-deterministic Markdown
// document for the contract test. It mirrors the shape of the
// benchmark corpus (headings, prose, fenced code, link, table) but
// lives in-package so the contract test does not depend on the
// engine_test bench file.
func buildContractDoc(idx, lines, total int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Document %d\n\n", idx)
	for i := 0; i < lines; i++ {
		switch {
		case i%25 == 0:
			fmt.Fprintf(&b, "## Section %d\n\n", i/25)
		case i%17 == 0:
			b.WriteString("```go\nfunc f() int { return 0 }\n```\n\n")
		case i%11 == 0:
			fmt.Fprintf(&b, "See [the next doc](doc%03d.md) for details.\n\n", (idx+1)%total)
		default:
			b.WriteString("This is a synthetic sentence used to exercise " +
				"the prose and structure rules under benchmark.\n\n")
		}
	}
	return b.String()
}
