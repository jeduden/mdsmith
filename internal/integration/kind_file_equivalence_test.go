package integration

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/engine"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/stretchr/testify/require"
)

// TestKindFile_DiagnosticsMatchInline pins plan 208's
// acceptance criterion #1: a file-defined kind produces
// byte-equal diagnostics to the equivalent inline kind.
// Two parallel workspaces share the same Markdown body. One
// declares the kind inline; the other declares it via
// `.mdsmith/kinds/short.yaml`. The two diagnostic sets must
// match line-by-line so a user migrating from inline to file
// sees no behavior shift (LSP: substitutable).
func TestKindFile_DiagnosticsMatchInline(t *testing.T) {
	body := "# Heading\n\nThis line exceeds the configured maximum length for the short kind on purpose.\n"

	inlineDir := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(inlineDir, ".mdsmith.yml"),
		[]byte(`
kinds:
  short:
    rules:
      line-length:
        max: 30
kind-assignment:
  - glob: ["doc.md"]
    kinds: [short]
`), 0o644))
	require.NoError(t, os.WriteFile(
		filepath.Join(inlineDir, "doc.md"), []byte(body), 0o644))

	fileDir := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(fileDir, ".mdsmith.yml"),
		[]byte(`
kind-assignment:
  - glob: ["doc.md"]
    kinds: [short]
`), 0o644))
	require.NoError(t, os.MkdirAll(
		filepath.Join(fileDir, ".mdsmith", "kinds"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(fileDir, ".mdsmith", "kinds", "short.yaml"),
		[]byte("rules:\n  line-length:\n    max: 30\n"), 0o644))
	require.NoError(t, os.WriteFile(
		filepath.Join(fileDir, "doc.md"), []byte(body), 0o644))

	inlineDiags := runCheckOnDoc(t, inlineDir)
	fileDiags := runCheckOnDoc(t, fileDir)

	require.Equal(t, len(inlineDiags), len(fileDiags),
		"file-defined kind must emit the same number of diagnostics as inline")
	for i := range inlineDiags {
		require.Equal(t, inlineDiags[i], fileDiags[i],
			"diagnostic %d must match between sources", i)
	}
}

// diagKey captures the comparable bits of a diagnostic so two
// runs can be compared without the engine pulling in file paths
// or unstable ordering. Rule + line + message is enough to pin
// the substitutability contract.
type diagKey struct {
	rule    string
	line    int
	message string
}

func runCheckOnDoc(t *testing.T, workspaceDir string) []diagKey {
	t.Helper()
	defaults := config.Defaults()
	loaded, err := config.Load(filepath.Join(workspaceDir, ".mdsmith.yml"))
	require.NoError(t, err)
	cfg := config.Merge(defaults, loaded)

	runner := &engine.Runner{
		Config:           cfg,
		Rules:            rule.All(),
		StripFrontMatter: true,
		RootDir:          workspaceDir,
	}
	result := runner.Run([]string{filepath.Join(workspaceDir, "doc.md")})

	out := make([]diagKey, 0, len(result.Diagnostics))
	for _, d := range result.Diagnostics {
		out = append(out, diagKey{
			rule:    d.RuleID,
			line:    d.Line,
			message: d.Message,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].rule != out[j].rule {
			return out[i].rule < out[j].rule
		}
		if out[i].line != out[j].line {
			return out[i].line < out[j].line
		}
		return out[i].message < out[j].message
	})
	return out
}

// Ensure the lint package import stays in use even if the
// engine path changes — keeps the test file robust to incidental
// import-pruning refactors.
var _ = lint.NewFile
