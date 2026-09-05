package backlinks

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jeduden/mdsmith/internal/linkgraph"
	"github.com/jeduden/mdsmith/internal/lint"
)

func TestResolveLinkTarget(t *testing.T) {
	cases := []struct {
		name     string
		src      string
		linkPath string
		want     string
	}{
		{"sibling", "docs/index.md", "api.md", "docs/api.md"},
		{"dot-prefix", "docs/index.md", "./api.md", "docs/api.md"},
		{"parent dir", "docs/sub/index.md", "../api.md", "docs/api.md"},
		{"two levels up", "plan/045.md", "../docs/api.md", "docs/api.md"},
		{"escapes root", "docs/api.md", "../../etc/passwd", ""},
		{"absolute link", "docs/api.md", "/etc/passwd", ""},
		{"absolute source", "/abs/docs/api.md", "guide.md", ""},
		// Windows-style absolutes — path.IsAbs alone misses these.
		{"drive letter link", "docs/api.md", "C:/Windows/system.md", ""},
		{"drive letter source", "C:/docs/api.md", "guide.md", ""},
		{"UNC link", "docs/api.md", "//server/share/file.md", ""},
		{"UNC source", "//server/share/api.md", "guide.md", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveLinkTarget(tc.src, tc.linkPath)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestIsAbsOrDriveOrUNC(t *testing.T) {
	assert.True(t, isAbsOrDriveOrUNC("/etc/passwd"))
	assert.True(t, isAbsOrDriveOrUNC("C:/Windows"))
	assert.True(t, isAbsOrDriveOrUNC("//server/share"))
	assert.False(t, isAbsOrDriveOrUNC("docs/api.md"))
	assert.False(t, isAbsOrDriveOrUNC(""))
}

func TestRelPath_EmptyRootDir(t *testing.T) {
	// When rootDir is empty, the helper just strips a leading "./"
	// and forwards the path through.
	assert.Equal(t, "docs/api.md", relPath("./docs/api.md", ""))
	assert.Equal(t, "docs/api.md", relPath("docs/api.md", ""))
}

// chdirToRemoved changes into a fresh temp dir and then deletes it, so
// the process has no valid working directory and os.Getwd fails.
// t.Chdir restores the original directory at cleanup. This drives the
// filepath.Abs-error fallbacks that a normal filesystem never reaches
// — the same pattern cmd/mdsmith's coverage100_test.go uses for the
// sibling workspaceRelativePath this function duplicates.
func chdirToRemoved(t *testing.T) {
	t.Helper()
	dir, err := os.MkdirTemp("", "mdsmith-backlinks-nowd-*")
	require.NoError(t, err)
	t.Chdir(dir)
	require.NoError(t, os.Remove(dir))
}

func TestRelPath_AbsPathError(t *testing.T) {
	// With no valid cwd, filepath.Abs(p) fails on a relative p before
	// rootDir is even consulted; relPath falls back to the trimmed
	// input.
	chdirToRemoved(t)
	assert.Equal(t, "docs/api.md", relPath("docs/api.md", "root"))
}

func TestRelPath_RelError(t *testing.T) {
	// filepath.Rel can only fail here on a Windows volume mismatch,
	// unreachable given two paths built from filepath.Abs on the
	// Unix runner this project's coverage gate uses. Swap relFn to
	// drive the fallback branch directly.
	orig := relFn
	relFn = func(string, string) (string, error) {
		return "", fmt.Errorf("simulated: can't relate paths")
	}
	t.Cleanup(func() { relFn = orig })

	root := t.TempDir()
	p := filepath.Join(root, "docs", "api.md")
	got := relPath(p, root)
	assert.Equal(t, strings.TrimPrefix(filepath.ToSlash(p), "./"), got)
}

func TestRelPath_AbsRootDirError(t *testing.T) {
	// p is already absolute, so filepath.Abs(p) succeeds without a
	// cwd; rootDir is relative, so filepath.Abs(rootDir) is the one
	// that fails with no valid cwd. The fallback mirrors the
	// original (absolute) p unchanged, since it never went through
	// the rootDir-relative branch.
	chdirToRemoved(t)
	assert.Equal(t, "/abs/docs/api.md", relPath("/abs/docs/api.md", "root"))
}

func TestSourceMatches(t *testing.T) {
	assert.True(t, sourceMatches("docs/api.md", nil))
	assert.True(t, sourceMatches("docs/api.md", []string{"docs/**"}))
	assert.False(t, sourceMatches("plan/045.md", []string{"docs/**"}))
	assert.True(t, sourceMatches("plan/045.md", []string{"docs/**", "plan/**"}))
}

func TestWikilinkTargetString(t *testing.T) {
	assert.Equal(t, "page", wikilinkTargetString(linkgraph.WikiLink{Target: "page"}))
	assert.Equal(t, "page#Section", wikilinkTargetString(linkgraph.WikiLink{Target: "page", Anchor: "Section"}))
}

// setupCollectFixture creates a small workspace with three distinct
// sources linking to docs/api.md so the end-to-end tests can share
// one filesystem layout.
func setupCollectFixture(t *testing.T) (root string, files []string) {
	t.Helper()
	root = t.TempDir()
	mkdir := func(rel string) {
		require.NoError(t, os.MkdirAll(filepath.Join(root, rel), 0o755))
	}
	write := func(rel, body string) {
		require.NoError(t, os.WriteFile(filepath.Join(root, rel), []byte(body), 0o644))
	}
	mkdir("docs")
	mkdir("plan")
	mkdir("docs/sub")
	write("docs/api.md", "# API\n\n## Authentication\n\n## Endpoints\n")
	write("docs/index.md", "# Index\n\nSee [API reference](api.md).\n")
	write("docs/sub/guide.md", "# Guide\n\nUse [api docs](../api.md#authentication).\n")
	write("plan/045_api-overhaul.md", "# Plan\n\n[api](../docs/api.md)\n")
	// File that does NOT link to api.md.
	write("docs/changelog.md", "# Changelog\n\n[plan](../plan/045_api-overhaul.md)\n")

	files = []string{
		filepath.Join(root, "docs/api.md"),
		filepath.Join(root, "docs/changelog.md"),
		filepath.Join(root, "docs/index.md"),
		filepath.Join(root, "docs/sub/guide.md"),
		filepath.Join(root, "plan/045_api-overhaul.md"),
	}
	return root, files
}

// TestCollect_End2End covers the path/anchor combinations the
// backlinks command's acceptance criteria call out: three sources,
// anchor scoping, include filter, limit.
func TestCollect_End2End(t *testing.T) {
	root, files := setupCollectFixture(t)

	t.Run("three sources, no anchor", func(t *testing.T) {
		got, errs := Collect(files, root, "docs/api.md", "", nil, nil, 0, true)
		require.Empty(t, errs)
		require.Len(t, got, 3)
		assert.Equal(t, "docs/index.md", got[0].Source)
		assert.Equal(t, "docs/sub/guide.md", got[1].Source)
		assert.Equal(t, "plan/045_api-overhaul.md", got[2].Source)
	})

	t.Run("anchor scopes to one source", func(t *testing.T) {
		got, errs := Collect(files, root, "docs/api.md", "authentication", nil, nil, 0, true)
		require.Empty(t, errs)
		require.Len(t, got, 1)
		assert.Equal(t, "docs/sub/guide.md", got[0].Source)
	})

	t.Run("anchor with no hits returns empty", func(t *testing.T) {
		got, errs := Collect(files, root, "docs/api.md", "no-such-section", nil, nil, 0, true)
		assert.Empty(t, errs)
		assert.Empty(t, got)
	})

	t.Run("include filter excludes plan/", func(t *testing.T) {
		got, errs := Collect(files, root, "docs/api.md", "", []string{"docs/**"}, nil, 0, true)
		require.Empty(t, errs)
		require.Len(t, got, 2)
		assert.Equal(t, "docs/index.md", got[0].Source)
		assert.Equal(t, "docs/sub/guide.md", got[1].Source)
	})

	t.Run("unreadable source surfaces as error", func(t *testing.T) {
		// A path that does not exist on disk: ReadFileLimited fails;
		// Collect captures the error rather than swallowing.
		bad := filepath.Join(root, "does-not-exist.md")
		filesWithBad := append([]string{bad}, files...)
		got, errs := Collect(filesWithBad, root, "docs/api.md", "", nil, nil, 0, true)
		// The other files still contribute results.
		assert.NotEmpty(t, got)
		require.Len(t, errs, 1)
		assert.Contains(t, errs[0].Error(), "does-not-exist.md")
	})
}

func TestCollect_LocalAnchorSkipped(t *testing.T) {
	// Source contains only a same-file anchor link, no cross-file
	// reference to the target. linkgraph yields a LocalAnchor=true
	// link; Collect must skip it without trying to resolve a path
	// target.
	root, files := setupCollectFixture(t)
	anchorOnly := filepath.Join(root, "anchor-only.md")
	require.NoError(t, os.WriteFile(anchorOnly,
		[]byte("# Intro\n\nJump to [section](#section).\n\n## Section\n"), 0o644))
	filesWithAnchor := append([]string{anchorOnly}, files...)
	got, errs := Collect(filesWithAnchor, root, "docs/api.md", "", nil, nil, 0, true)
	assert.Empty(t, errs)
	// Same three matches as before; anchor-only.md contributes nothing.
	assert.Len(t, got, 3)
}

func TestCollect_IgnorePatternsExclude(t *testing.T) {
	// `plan/**` ignores the plan/* source that links to api.md;
	// the other two sources still produce records.
	root, files := setupCollectFixture(t)
	got, errs := Collect(
		files, root, "docs/api.md", "",
		nil, []string{"plan/**"}, 0, true,
	)
	require.Empty(t, errs)
	require.Len(t, got, 2)
	for _, r := range got {
		assert.NotContains(t, r.Source, "plan/")
	}
}

// TestCollect_SelfLink confirms self-references count as incoming
// edges. The contract is "every workspace file that links to
// <target>" — a file that links to itself satisfies that as
// literally as any other source.
func TestCollect_SelfLink(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "docs"), 0o755))
	// docs/api.md links to itself via `api.md`.
	require.NoError(t, os.WriteFile(filepath.Join(root, "docs", "api.md"),
		[]byte("# API\n\nSee [back to top](api.md).\n"), 0o644))
	files := []string{filepath.Join(root, "docs", "api.md")}

	got, errs := Collect(files, root, "docs/api.md", "", nil, nil, 0, true)
	require.Empty(t, errs)
	require.Len(t, got, 1)
	assert.Equal(t, "docs/api.md", got[0].Source)
	assert.Equal(t, "api.md", got[0].Target)
}

// TestCollect_FrontMatterStrippingDisabled verifies the
// stripFrontMatter parameter is honored. When set to false (matching
// `frontMatter: false` in config), Collect parses the entire file
// including its front matter — line numbers stay in raw file
// coordinates rather than body-relative.
func TestCollect_FrontMatterStrippingDisabled(t *testing.T) {
	root, files := setupCollectFixture(t)
	fmSrc := filepath.Join(root, "fm-src.md")
	require.NoError(t, os.WriteFile(fmSrc,
		[]byte("---\ntitle: x\n---\n# H\n\nSee [api](docs/api.md).\n"), 0o644))
	filesWithFM := append([]string{fmSrc}, files...)
	got, errs := Collect(filesWithFM, root, "docs/api.md", "", nil, nil, 0, false)
	require.Empty(t, errs)
	var fmRec *Record
	for i := range got {
		if got[i].Source == "fm-src.md" {
			fmRec = &got[i]
			break
		}
	}
	require.NotNil(t, fmRec)
	// Front matter spans 3 lines; the link sits on the 6th line.
	// stripFrontMatter=false → no LineOffset adjustment → 6.
	assert.Equal(t, 6, fmRec.Line)
}

// TestCollect_SortStable verifies that two records from the same
// source file are returned in line order (the secondary key in the
// SliceStable comparator).
func TestCollect_SortStable(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "target.md"), []byte("# T\n"), 0o644))
	// Two links to target.md from the SAME source: line 3 and line 5.
	body := "# Src\n\nFirst [a](target.md), and\n\nsecond [b](target.md).\n"
	require.NoError(t, os.WriteFile(filepath.Join(root, "src.md"), []byte(body), 0o644))

	files := []string{filepath.Join(root, "src.md"), filepath.Join(root, "target.md")}
	got, errs := Collect(files, root, "target.md", "", nil, nil, 0, true)
	require.Empty(t, errs)
	require.Len(t, got, 2)
	assert.Equal(t, "src.md", got[0].Source)
	assert.Equal(t, "src.md", got[1].Source)
	assert.Less(t, got[0].Line, got[1].Line, "same-source records sort by line")
}

func TestCollect_Wikilinks(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "page.md"), []byte("# Page\n\n## Section\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "other.md"),
		[]byte("# Other\n\nSee [[page]] and [[page|alias]].\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "anchored.md"),
		[]byte("# Anchored\n\n[[page#Section]]\n"), 0o644))
	files := []string{
		filepath.Join(root, "anchored.md"),
		filepath.Join(root, "other.md"),
		filepath.Join(root, "page.md"),
	}

	t.Run("matches both wikilink shapes", func(t *testing.T) {
		got, errs := Collect(files, root, "page.md", "", nil, nil, 0, true)
		require.Empty(t, errs)
		// Expect three wikilink hits — two on other.md, one on anchored.md.
		require.Len(t, got, 3)
		for _, r := range got {
			assert.Equal(t, "wikilink", r.Kind, "all wikilink hits must carry kind=wikilink")
		}
	})

	t.Run("anchor scoping", func(t *testing.T) {
		got, errs := Collect(files, root, "page.md", "section", nil, nil, 0, true)
		require.Empty(t, errs)
		require.Len(t, got, 1)
		assert.Equal(t, "anchored.md", got[0].Source)
		assert.Equal(t, "wikilink", got[0].Kind)
		assert.Equal(t, "page#Section", got[0].Target)
		// `text` is the visible link text — for anchor-only wikilinks
		// the full target-as-written, not the bare stem. Setting
		// Text=wl.Target alone would drop the `#Section` half and
		// break JSON round-tripping.
		assert.Equal(t, "page#Section", got[0].Text)
	})
}

func TestAppendWikilinkBacklinks_FallbackToPerCallWalk(t *testing.T) {
	// The Collect public path always pairs index with RootFS; calling
	// appendWikilinkBacklinks directly with a nil index but a
	// populated RootFS exercises the per-call fs.WalkDir fallback,
	// which would otherwise rot if the helper API ever gets reused
	// without the index.
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "page.md"), []byte("# P\n"), 0o644))
	srcPath := filepath.Join(root, "src.md")
	require.NoError(t, os.WriteFile(srcPath, []byte("# S\n\n[[page]]\n"), 0o644))
	data, err := os.ReadFile(srcPath)
	require.NoError(t, err)
	f, err := lint.NewFileFromSource(srcPath, data, true)
	require.NoError(t, err)
	f.RootFS = os.DirFS(root)
	got := appendWikilinkBacklinks(nil, f, "src.md", "page.md", "", nil)
	require.Len(t, got, 1)
	// Target on a wikilink record is the source-form `[[target]]`,
	// not the resolved filename; resolution matches "page" against
	// "page.md" but the record carries the bare stem.
	assert.Equal(t, "page", got[0].Target)
}

func TestAppendWikilinkBacklinks_NoRootFS(t *testing.T) {
	// extractBacklinksFromSource leaves f.RootFS nil whenever
	// rootDir == "" — appendWikilinkBacklinks then has no workspace
	// to walk and must return the input slice untouched.
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "src.md"),
		[]byte("# S\n\n[[page]]\n"), 0o644))
	files := []string{filepath.Join(root, "src.md")}
	got, errs := Collect(files, "", "page.md", "", nil, nil, 0, true)
	require.Empty(t, errs)
	assert.Empty(t, got)
}

func TestAppendWikilinkBacklinks_UnrelatedTargetSkipped(t *testing.T) {
	// A wikilink that resolves to a different file than wantTarget
	// exercises the `r.path != wantTarget` continue branch.
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "other.md"),
		[]byte("# Other\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "src.md"),
		[]byte("# S\n\n[[other]]\n"), 0o644))
	files := []string{filepath.Join(root, "src.md")}
	got, errs := Collect(files, root, "page.md", "", nil, nil, 0, true)
	require.Empty(t, errs)
	assert.Empty(t, got)
}
