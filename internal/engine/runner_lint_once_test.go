package engine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	// Import rules so init() registers them.
	_ "github.com/jeduden/mdsmith/internal/rules/notrailingspaces"
)

// trailingSpacesRuleName is the registered name of the no-trailing-spaces rule.
const trailingSpacesRuleName = "no-trailing-spaces"

// makeTrailingSpacesConfig returns a config that enables only no-trailing-spaces.
func makeTrailingSpacesConfig() *config.Config {
	return &config.Config{
		Rules: map[string]config.RuleCfg{
			trailingSpacesRuleName: {Enabled: true},
		},
	}
}

// TestLintOnce_IncludeHost verifies that diagnostics originating in an
// <?include?> generated section are suppressed when linting the host, and
// surface normally when the fragment is linted on its own.
func TestLintOnce_IncludeHost(t *testing.T) {
	dir := t.TempDir()

	// fragment.md has a trailing-spaces violation on line 3.
	fragment := "# Fragment\n\nTrailing spaces here.   \n"
	fragmentPath := filepath.Join(dir, "fragment.md")
	require.NoError(t, os.WriteFile(fragmentPath, []byte(fragment), 0o644))

	// host.md has an <?include?> section that already contains the fragment's
	// content (lines 6-8 are the generated body — ContentFrom=6, ContentTo=8).
	// The trailing-spaces violation on line 8 lives inside the generated range.
	host := "# Host File\n\n" +
		"<?include\nfile: fragment.md\n?>\n" +
		"# Fragment\n\nTrailing spaces here.   \n" +
		"<?/include?>\n\n" +
		"# After Include\n"
	hostPath := filepath.Join(dir, "host.md")
	require.NoError(t, os.WriteFile(hostPath, []byte(host), 0o644))

	rules := rule.All()
	require.NotEmpty(t, rules, "rules must be registered via imports")

	runner := &Runner{
		Config:  makeTrailingSpacesConfig(),
		Rules:   rules,
		RootDir: dir,
	}

	// Linting the host must not surface trailing-spaces from the embedded body.
	hostResult := runner.Run([]string{hostPath})
	require.Empty(t, hostResult.Errors, "unexpected errors linting host: %v", hostResult.Errors)
	for _, d := range hostResult.Diagnostics {
		if d.RuleName == trailingSpacesRuleName {
			t.Errorf("host-lint produced trailing-spaces diagnostic from embedded fragment: line %d: %s",
				d.Line, d.Message)
		}
	}

	// Linting the fragment directly must surface the trailing-spaces diagnostic.
	fragmentResult := runner.Run([]string{fragmentPath})
	require.Empty(t, fragmentResult.Errors, "unexpected errors linting fragment: %v", fragmentResult.Errors)
	found := false
	for _, d := range fragmentResult.Diagnostics {
		if d.RuleName == trailingSpacesRuleName {
			found = true
			break
		}
	}
	assert.True(t, found, "fragment linted directly must produce trailing-spaces diagnostic")
}

// TestLintOnce_CatalogHost verifies that diagnostics in a <?catalog?>
// generated section are suppressed when linting the catalog host.
func TestLintOnce_CatalogHost(t *testing.T) {
	dir := t.TempDir()

	// catalogHost.md has a <?catalog?> section whose body contains a line
	// with trailing spaces (lines 7 is ContentFrom=7, ContentTo=7).
	catalogHost := "# Catalog Host\n\n" +
		"<?catalog\nglob: \"*.md\"\nrow: \"- {filename}\"\n?>\n" +
		"- source.md   \n" +
		"<?/catalog?>\n"
	hostPath := filepath.Join(dir, "catalogHost.md")
	require.NoError(t, os.WriteFile(hostPath, []byte(catalogHost), 0o644))

	rules := rule.All()
	runner := &Runner{
		Config:  makeTrailingSpacesConfig(),
		Rules:   rules,
		RootDir: dir,
	}

	result := runner.Run([]string{hostPath})
	require.Empty(t, result.Errors, "unexpected errors: %v", result.Errors)
	for _, d := range result.Diagnostics {
		if d.RuleName == trailingSpacesRuleName {
			t.Errorf("catalog host must not surface trailing-spaces from generated body: line %d: %s",
				d.Line, d.Message)
		}
	}
}

// TestLintOnce_HostOwnedDiagnosticsPreserved verifies that diagnostics in
// host-authored content (outside generated sections) are not suppressed.
func TestLintOnce_HostOwnedDiagnosticsPreserved(t *testing.T) {
	dir := t.TempDir()

	// host.md has a trailing-spaces violation at line 1 (author-owned) AND
	// the generated section body at lines 6-8 also has trailing spaces.
	// Only the author-owned violation (line 1) must be reported.
	host := "# Host trailing spaces.   \n\n" +
		"<?include\nfile: frag.md\n?>\n" +
		"# Fragment\n\nEmbedded trailing spaces.   \n" +
		"<?/include?>\n"
	hostPath := filepath.Join(dir, "host.md")
	require.NoError(t, os.WriteFile(hostPath, []byte(host), 0o644))

	rules := rule.All()
	runner := &Runner{
		Config:  makeTrailingSpacesConfig(),
		Rules:   rules,
		RootDir: dir,
	}

	result := runner.Run([]string{hostPath})
	require.Empty(t, result.Errors, "unexpected errors: %v", result.Errors)

	var tsLines []int
	for _, d := range result.Diagnostics {
		if d.RuleName == trailingSpacesRuleName {
			tsLines = append(tsLines, d.Line)
		}
	}
	require.Len(t, tsLines, 1, "expected exactly one trailing-spaces diagnostic (host-owned only), got %v", tsLines)
	assert.Equal(t, 1, tsLines[0], "trailing-spaces must be on line 1 (host-owned), got line %d", tsLines[0])
}

// TestLintOnce_NoGeneratedSections verifies that files without generated
// sections are unaffected: all diagnostics are preserved.
func TestLintOnce_NoGeneratedSections(t *testing.T) {
	f, err := lint.NewFile("plain.md", []byte("# Plain\n\nTrailing.   \n"))
	require.NoError(t, err)
	// No GeneratedRanges set.

	rules := rule.All()
	rulesCfg := make(map[string]config.RuleCfg, len(rules))
	for _, r := range rules {
		rulesCfg[r.Name()] = config.RuleCfg{Enabled: true}
	}
	diags, _ := CheckRules(f, rules, rulesCfg)

	found := false
	for _, d := range diags {
		if d.RuleName == trailingSpacesRuleName {
			found = true
			break
		}
	}
	assert.True(t, found, "files without generated sections must preserve all diagnostics")
}
