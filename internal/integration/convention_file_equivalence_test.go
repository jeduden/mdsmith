package integration

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestConventionFile_DiagnosticsMatchInline pins plan 209's acceptance
// criterion #1: a file-defined convention produces byte-equal
// diagnostics to the equivalent inline convention. Two parallel
// workspaces share the same Markdown body and select a convention
// named "strict"; one declares it inline under `conventions:`, the
// other via `.mdsmith/conventions/strict.yaml`. The two diagnostic
// sets must match line-by-line so a user migrating from inline to file
// sees no behavior shift (LSP: substitutable). Reuses runCheckOnDoc
// from the kind-file equivalence test.
func TestConventionFile_DiagnosticsMatchInline(t *testing.T) {
	body := "# Heading\n\nThis line is definitely longer than the thirty " +
		"character maximum the strict convention sets.\n"

	inlineDir := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(inlineDir, ".mdsmith.yml"),
		[]byte(`
convention: strict
conventions:
  strict:
    flavor: commonmark
    rules:
      line-length:
        max: 30
`), 0o644))
	require.NoError(t, os.WriteFile(
		filepath.Join(inlineDir, "doc.md"), []byte(body), 0o644))

	fileDir := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(fileDir, ".mdsmith.yml"),
		[]byte("convention: strict\n"), 0o644))
	require.NoError(t, os.MkdirAll(
		filepath.Join(fileDir, ".mdsmith", "conventions"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(fileDir, ".mdsmith", "conventions", "strict.yaml"),
		[]byte("flavor: commonmark\nrules:\n  line-length:\n    max: 30\n"), 0o644))
	require.NoError(t, os.WriteFile(
		filepath.Join(fileDir, "doc.md"), []byte(body), 0o644))

	inlineDiags := runCheckOnDoc(t, inlineDir)
	fileDiags := runCheckOnDoc(t, fileDir)

	require.NotEmpty(t, inlineDiags,
		"the convention's line-length:30 must fire on the long line "+
			"(guards against a trivial empty-vs-empty match)")
	require.Equal(t, len(inlineDiags), len(fileDiags),
		"file-defined convention must emit the same number of diagnostics as inline")
	for i := range inlineDiags {
		require.Equal(t, inlineDiags[i], fileDiags[i],
			"diagnostic %d must match between sources", i)
	}
}
