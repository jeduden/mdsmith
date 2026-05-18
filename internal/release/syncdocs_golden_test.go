package release

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// reconcileGoldenPath is the single concatenated snapshot of
// reconcileDocForHugo over the whole repository docs/ tree plus a set
// of synthetic edge cases. Regenerate with UPDATE_GOLDEN=1.
const reconcileGoldenPath = "testdata/reconcile_corpus.golden"

// reconcileSyntheticCases exercises branches the real docs/ tree may
// not: setext H1, indented ATX, an unterminated PI, a front-matter-
// only file, a malformed (never-closed) front matter, no front
// matter, and a single-line vs multi-line directive marker.
func reconcileSyntheticCases() []struct{ name, body string } {
	return []struct{ name, body string }{
		{"no-frontmatter-atx-h1", "# Title\n\nBody text.\n"},
		{"frontmatter-and-h1", "---\nsummary: s\n---\n# Title\n\nBody.\n"},
		{"setext-h1", "---\nsummary: s\n---\nTitle\n=====\n\nBody.\n"},
		{"indented-atx-h1", "---\nsummary: s\n---\n   # Title\n\nBody.\n"},
		{"single-line-pi", "---\nsummary: s\n---\n# T\n\n<?toc?>\n<?/toc?>\n\nEnd.\n"},
		{"multi-line-pi", "---\nsummary: s\n---\n# T\n\n<?catalog\nglob: x\n?>\nrow\n<?/catalog?>\n\nEnd.\n"},
		{"unterminated-pi", "---\nsummary: s\n---\n# T\n\n<?include\nfile: x\n"},
		{"frontmatter-only", "---\nsummary: s\n---\n"},
		{"malformed-frontmatter", "---\nsummary: s\n# never closed\n"},
		{"no-liftable-h1", "---\nsummary: s\n---\n## Sub first\n\nBody.\n"},
		{"pi-in-code-fence", "---\nsummary: s\n---\n# T\n\n```\n<?toc?>\n```\n\nEnd.\n"},
		{"empty", ""},
	}
}

// buildReconcileCorpus produces the deterministic concatenation that
// the golden snapshot pins: every docs/**/*.md (sorted) followed by
// the synthetic cases, each prefixed with a stable separator.
func buildReconcileCorpus(t *testing.T) string {
	t.Helper()
	docsRoot := filepath.Join("..", "..", "docs")
	var paths []string
	require.NoError(t, filepath.WalkDir(docsRoot, func(p string, d os.DirEntry, err error) error {
		require.NoError(t, err)
		if !d.IsDir() && strings.HasSuffix(p, ".md") {
			rel, rerr := filepath.Rel(docsRoot, p)
			require.NoError(t, rerr)
			paths = append(paths, filepath.ToSlash(rel))
		}
		return nil
	}))
	sort.Strings(paths)

	var b strings.Builder
	for _, rel := range paths {
		data, err := os.ReadFile(filepath.Join(docsRoot, filepath.FromSlash(rel)))
		require.NoError(t, err)
		b.WriteString("===== docs/" + rel + " =====\n")
		b.Write(reconcileDocForHugo(data))
		b.WriteString("\n")
	}
	for _, c := range reconcileSyntheticCases() {
		b.WriteString("===== synthetic/" + c.name + " =====\n")
		b.Write(reconcileDocForHugo([]byte(c.body)))
		b.WriteString("\n")
	}
	return b.String()
}

// TestReconcileDocForHugo_GoldenCorpus pins reconcileDocForHugo's
// output byte-for-byte across the whole docs/ tree and the synthetic
// edge cases. It is the AC4 guard for plan 163: the snapshot is
// generated from the pre-migration goldmark path, so the test failing
// after the pkg/markdown migration means sync-docs output drifted.
func TestReconcileDocForHugo_GoldenCorpus(t *testing.T) {
	got := buildReconcileCorpus(t)
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		require.NoError(t, os.MkdirAll(filepath.Dir(reconcileGoldenPath), 0o755))
		require.NoError(t, os.WriteFile(reconcileGoldenPath, []byte(got), 0o644))
		t.Skip("golden regenerated")
	}
	want, err := os.ReadFile(reconcileGoldenPath)
	require.NoError(t, err, "golden missing; run with UPDATE_GOLDEN=1")
	require.Equal(t, string(want), got,
		"reconcileDocForHugo output drifted from the pre-migration snapshot")
}
