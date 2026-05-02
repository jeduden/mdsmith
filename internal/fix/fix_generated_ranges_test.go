package fix

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// alwaysFiringRule emits one diagnostic per line in the file. It is
// non-fixable, so any diagnostic the engine doesn't filter survives
// into the post-fix result.
type alwaysFiringRule struct {
	id   string
	name string
}

func (r *alwaysFiringRule) ID() string       { return r.id }
func (r *alwaysFiringRule) Name() string     { return r.name }
func (r *alwaysFiringRule) Category() string { return "test" }

func (r *alwaysFiringRule) Check(f *lint.File) []lint.Diagnostic {
	diags := make([]lint.Diagnostic, 0, len(f.Lines))
	for i := range f.Lines {
		diags = append(diags, lint.Diagnostic{
			File:     f.Path,
			Line:     i + 1,
			Column:   1,
			RuleID:   r.id,
			RuleName: r.name,
			Severity: lint.Warning,
			Message:  "always",
		})
	}
	return diags
}

// TestFix_SuppressesDiagnosticsInsideCatalogBody mirrors
// engine.TestLintOnce_CatalogHost on the fix path: diagnostics whose
// lines fall inside a <?catalog?> generated body must not surface
// from the host's perspective. Without this, `mdsmith fix` and
// `mdsmith check` disagree on the same source bytes — check filters
// the body, fix doesn't — which is what surfaced as a CI-only failure
// in the merge queue when the pre-merge-commit hook ran fix on a
// merge containing a long catalog summary.
func TestFix_SuppressesDiagnosticsInsideCatalogBody(t *testing.T) {
	dir := t.TempDir()

	// Lines:
	// 1: # Catalog Host
	// 2: (empty)
	// 3: <?catalog
	// 4: glob: "*.md"
	// 5: row: "- {filename}"
	// 6: ?>
	// 7: - body-line.md         <-- generated-body content (line 7)
	// 8: <?/catalog?>
	host := "# Catalog Host\n\n" +
		"<?catalog\nglob: \"*.md\"\nrow: \"- {filename}\"\n?>\n" +
		"- body-line.md\n" +
		"<?/catalog?>\n"
	hostPath := filepath.Join(dir, "host.md")
	require.NoError(t, os.WriteFile(hostPath, []byte(host), 0o644))

	const ruleName = "always-fires"
	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			ruleName: {Enabled: true},
		},
	}

	fixer := &Fixer{
		Config: cfg,
		Rules:  []rule.Rule{&alwaysFiringRule{id: "MDS999", name: ruleName}},
	}

	result := fixer.Fix([]string{hostPath})
	require.Empty(t, result.Errors, "unexpected errors: %v", result.Errors)

	// Line 7 is inside the generated catalog body. A diagnostic on
	// that line must be filtered out before reaching the result.
	for _, d := range result.Diagnostics {
		if d.RuleName == ruleName && d.Line == 7 {
			t.Errorf("fix surfaced %s diagnostic from inside <?catalog?> body (line 7): %s",
				ruleName, d.Message)
		}
	}

	// Pre-fix Failures count must also exclude the generated-body
	// line. The host has 9 line entries (8 source lines plus the
	// trailing-newline split) but line 7 is inside the generated
	// body, so fewer than 9 should remain after generated-body
	// filtering. The strict-less assertion stays robust to unrelated
	// trailing-line counting tweaks.
	assert.Less(t, result.Failures, 9,
		"pre-fix Failures should exclude the generated-body line, got %d (no filtering applied)",
		result.Failures)
}
