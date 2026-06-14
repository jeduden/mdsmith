package release

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const benchSrcDir = "docs/research/benchmarks"

// TestGithubURLForRelativeTarget pins the resolve-and-route helper:
// repo-relative targets become absolute GitHub URLs on main (file →
// /blob/, directory → /tree/, fragment preserved); anchor-only,
// site-absolute, and already-absolute targets are declined.
func TestGithubURLForRelativeTarget(t *testing.T) {
	const blob = "https://github.com/jeduden/mdsmith/blob/main/"
	const tree = "https://github.com/jeduden/mdsmith/tree/main/"
	cases := []struct {
		name   string
		target string
		want   string
		ok     bool
	}{
		{"sibling file", "run.sh", blob + benchSrcDir + "/run.sh", true},
		{"sibling yml", "bench-parity.mdsmith.yml",
			blob + benchSrcDir + "/bench-parity.mdsmith.yml", true},
		{"one up", "../markdownlint-coverage/README.md",
			blob + "docs/research/markdownlint-coverage/README.md", true},
		{"two up", "../../reference/conventions.md",
			blob + "docs/reference/conventions.md", true},
		{"three up into internal", "../../../internal/rules/MDS068-link-style/README.md",
			blob + "internal/rules/MDS068-link-style/README.md", true},
		{"fragment preserved", "results.fragment.md#x",
			blob + benchSrcDir + "/results.fragment.md#x", true},
		{"directory routes to tree", "../markdownlint-coverage/",
			tree + "docs/research/markdownlint-coverage/", true},
		{"anchor only declined", "#reading-the-result", "", false},
		{"site absolute declined", "/reference/cli/", "", false},
		{"https declined", "https://example.com/x", "", false},
		{"mailto declined", "mailto:a@b.c", "", false},
		{"empty declined", "", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := githubURLForRelativeTarget(tc.target, benchSrcDir)
			assert.Equal(t, tc.ok, ok)
			assert.Equal(t, tc.want, got)
		})
	}
}

// TestRewriteRelativeLinksToGitHub covers the full pass over a body
// mixing inline links, a titled link, a reference definition, an
// absolute reference def, an anchor link, and a code span / fenced
// block whose example links must survive verbatim.
func TestRewriteRelativeLinksToGitHub(t *testing.T) {
	const blob = "https://github.com/jeduden/mdsmith/blob/main/"
	in := "See [`run.sh`](run.sh) and the\n" +
		"[matrix](../markdownlint-coverage/README.md) plus\n" +
		"[the PGO page](../../development/pgo-profile.md \"PGO\").\n" +
		"Jump to [results](#results).\n" +
		"[mdcov]: ../markdownlint-coverage/README.md\n" +
		"[pn]: https://github.com/jolars/panache\n" +
		"\n" +
		"Inline `[x](../keep.md)` stays. Fenced too:\n" +
		"```md\n" +
		"[y](../also-keep.md)\n" +
		"```\n"
	got := string(rewriteRelativeLinksToGitHub([]byte(in), benchSrcDir))

	// Relative links rewritten to GitHub main.
	assert.Contains(t, got, "[`run.sh`]("+blob+benchSrcDir+"/run.sh)")
	assert.Contains(t, got,
		"[matrix]("+blob+"docs/research/markdownlint-coverage/README.md)")
	// Title is preserved after the rewritten URL.
	assert.Contains(t, got,
		"[the PGO page]("+blob+"docs/development/pgo-profile.md \"PGO\")")
	// Reference definition rewritten.
	assert.Contains(t, got,
		"[mdcov]: "+blob+"docs/research/markdownlint-coverage/README.md")

	// Left untouched: anchor, absolute ref def, code span, fenced block.
	assert.Contains(t, got, "[results](#results)")
	assert.Contains(t, got, "[pn]: https://github.com/jolars/panache")
	assert.Contains(t, got, "`[x](../keep.md)`")
	assert.Contains(t, got, "[y](../also-keep.md)")
}

// TestRenderBenchPageHappyPath stages a benchmark README under the
// repo-relative path, renders it, and asserts the output file lands
// (creating its parent) with links rewritten to GitHub main.
func TestRenderBenchPageHappyPath(t *testing.T) {
	root := t.TempDir()
	readme := filepath.Join(root, filepath.FromSlash(benchReadmeRel))
	require.NoError(t, os.MkdirAll(filepath.Dir(readme), 0o755))
	require.NoError(t, os.WriteFile(readme,
		[]byte("# Bench\n\nReproduce with [`run.sh`](run.sh).\n"), 0o644))

	// Exercise the package-level delegator (the public API) so its
	// success path is covered within this package's profile.
	out := filepath.Join(root, "pages", "benchmark.md")
	require.NoError(t, RenderBenchPage(root, out))

	got, err := os.ReadFile(out)
	require.NoError(t, err)
	assert.Contains(t, string(got),
		"https://github.com/jeduden/mdsmith/blob/main/"+benchSrcDir+"/run.sh")
}

// TestRenderBenchPageReadError surfaces a missing README as an error
// naming the source path.
func TestRenderBenchPageReadError(t *testing.T) {
	err := New().RenderBenchPage(t.TempDir(), filepath.Join(t.TempDir(), "out.md"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "read benchmark README")
}

// TestRenderBenchPageMkdirError covers the parent-dir MkdirAll fault
// branch via the injecting fake FS.
func TestRenderBenchPageMkdirError(t *testing.T) {
	root := t.TempDir()
	readme := filepath.Join(root, filepath.FromSlash(benchReadmeRel))
	require.NoError(t, os.MkdirAll(filepath.Dir(readme), 0o755))
	require.NoError(t, os.WriteFile(readme, []byte("# Bench\n"), 0o644))

	ff := newFakeFS()
	ff.failOnMkdirAllCall = 1
	err := NewWithFS(ff).RenderBenchPage(root, filepath.Join(root, "pages", "benchmark.md"))
	require.Error(t, err)
	assert.ErrorIs(t, err, errInjected)
	assert.Contains(t, err.Error(), "mkdir")
}

// TestRenderBenchPageWriteError covers the WriteFile fault branch.
func TestRenderBenchPageWriteError(t *testing.T) {
	root := t.TempDir()
	readme := filepath.Join(root, filepath.FromSlash(benchReadmeRel))
	require.NoError(t, os.MkdirAll(filepath.Dir(readme), 0o755))
	require.NoError(t, os.WriteFile(readme, []byte("# Bench\n"), 0o644))

	ff := newFakeFS()
	ff.failOnWriteFileCall = 1
	err := NewWithFS(ff).RenderBenchPage(root, filepath.Join(root, "pages", "benchmark.md"))
	require.Error(t, err)
	assert.ErrorIs(t, err, errInjected)
	assert.Contains(t, err.Error(), "write benchmark page")
}

// TestIsPerformanceFeaturePage pins the matcher that scopes the
// assets-branch repoint to docs/features/performance.md alone.
func TestIsPerformanceFeaturePage(t *testing.T) {
	assert.True(t, isPerformanceFeaturePage("docs/features", "performance.md"))
	// Any docs-root basename works (the recursion builds repoDir
	// with path.Join, and only the last segment is matched).
	assert.True(t, isPerformanceFeaturePage("tmpdocs/features", "performance.md"))
	assert.False(t, isPerformanceFeaturePage("docs/features", "auto-fix.md"))
	assert.False(t, isPerformanceFeaturePage("docs/reference", "performance.md"))
}

// TestRepointBenchDocToAssets confirms the main-source benchmark URL
// (what rewriteRuleLinks emits) is swapped for the assets-branch page,
// and an unrelated GitHub link is left untouched.
func TestRepointBenchDocToAssets(t *testing.T) {
	in := "a [doc](" + benchDocReadmeMainURL + ") and " +
		"[other](" + githubBlobBase + "docs/reference/cli.md)\n"
	got := string(repointBenchDocToAssets([]byte(in)))
	assert.Contains(t, got, "[doc]("+benchDocAssetsPageURL+")")
	assert.NotContains(t, got, benchDocReadmeMainURL)
	assert.Contains(t, got, "[other]("+githubBlobBase+"docs/reference/cli.md)")
}

// TestSyncDocsRepointsBenchLinkOnlyForPerformancePage drives the
// scope end-to-end: a features/performance.md and a sibling feature
// page both carry the relative benchmark-README link, but only the
// performance page's synced output resolves to the assets branch.
func TestSyncDocsRepointsBenchLinkOnlyForPerformancePage(t *testing.T) {
	src := t.TempDir()
	feat := filepath.Join(src, "features")
	require.NoError(t, os.MkdirAll(feat, 0o755))
	body := "# X\n\nSee the [bench](../research/benchmarks/README.md).\n"
	require.NoError(t, os.WriteFile(filepath.Join(feat, "performance.md"), []byte(body), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(feat, "other.md"), []byte(body), 0o644))

	dst := filepath.Join(t.TempDir(), "out")
	require.NoError(t, New().SyncDocs(src, dst))

	perf, err := os.ReadFile(filepath.Join(dst, "features", "performance.md"))
	require.NoError(t, err)
	assert.Contains(t, string(perf), benchDocAssetsPageURL)
	assert.NotContains(t, string(perf), benchDocReadmeMainURL)

	other, err := os.ReadFile(filepath.Join(dst, "features", "other.md"))
	require.NoError(t, err)
	assert.Contains(t, string(other), benchDocReadmeMainURL)
	assert.NotContains(t, string(other), benchDocAssetsPageURL)
}
