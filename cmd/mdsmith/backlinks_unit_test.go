package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestNormalizeWorkspacePath(t *testing.T) {
	assert.Equal(t, "docs/api.md", normalizeWorkspacePath("docs/api.md"))
	assert.Equal(t, "docs/api.md", normalizeWorkspacePath("./docs/api.md"))
}

func TestSourceMatches(t *testing.T) {
	assert.True(t, sourceMatches("docs/api.md", nil))
	assert.True(t, sourceMatches("docs/api.md", []string{"docs/**"}))
	assert.False(t, sourceMatches("plan/045.md", []string{"docs/**"}))
	assert.True(t, sourceMatches("plan/045.md", []string{"docs/**", "plan/**"}))
}

func TestEmitBacklinks_Text(t *testing.T) {
	var buf bytes.Buffer
	records := []backlinkRecord{
		{Source: "a.md", Line: 1, Text: "ref", Target: "x.md"},
		{Source: "b.md", Line: 2, Text: "ref2", Target: "./x.md"},
	}
	code := emitBacklinks(&buf, records, "text", 0)
	assert.Equal(t, 0, code)
	out := buf.String()
	assert.Contains(t, out, "a.md:1: [ref](x.md)\n")
	assert.Contains(t, out, "b.md:2: [ref2](./x.md)\n")
}

func TestEmitBacklinks_JSON(t *testing.T) {
	var buf bytes.Buffer
	records := []backlinkRecord{
		{Source: "a.md", Line: 1, Text: "ref", Target: "x.md"},
	}
	code := emitBacklinks(&buf, records, "json", 0)
	assert.Equal(t, 0, code)
	var decoded []backlinkRecord
	require.NoError(t, json.Unmarshal(buf.Bytes(), &decoded))
	assert.Equal(t, records, decoded)
}

func TestEmitBacklinks_JSONEmpty(t *testing.T) {
	var buf bytes.Buffer
	code := emitBacklinks(&buf, nil, "json", 0)
	assert.Equal(t, 1, code, "no records → exit 1")
	// `[]` is the documented stable shape; never `null`.
	assert.Contains(t, buf.String(), "[]")
	assert.NotContains(t, buf.String(), "null")
}

func TestEmitBacklinks_Limit(t *testing.T) {
	records := []backlinkRecord{
		{Source: "a.md", Line: 1},
		{Source: "b.md", Line: 1},
		{Source: "c.md", Line: 1},
	}
	var buf bytes.Buffer
	code := emitBacklinks(&buf, records, "text", 2)
	assert.Equal(t, 0, code)
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	assert.Len(t, lines, 2)
}

func TestEmitBacklinks_UnknownFormat(t *testing.T) {
	var buf bytes.Buffer
	code := emitBacklinks(&buf, []backlinkRecord{{Source: "a.md"}}, "yaml", 0)
	assert.Equal(t, 2, code)
}

// setupCollectBacklinksFixture creates a small workspace with three
// distinct sources linking to docs/api.md so the end-to-end tests can
// share one filesystem layout.
func setupCollectBacklinksFixture(t *testing.T) (root string, files []string) {
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

// TestCollectBacklinks_End2End covers the path/anchor combinations the
// plan's acceptance criteria call out: three sources, anchor scoping,
// include filter, limit.
func TestCollectBacklinks_End2End(t *testing.T) {
	root, files := setupCollectBacklinksFixture(t)

	t.Run("three sources, no anchor", func(t *testing.T) {
		got, errs := collectBacklinks(files, root, "docs/api.md", "", nil, nil, 0, true)
		require.Empty(t, errs)
		require.Len(t, got, 3)
		assert.Equal(t, "docs/index.md", got[0].Source)
		assert.Equal(t, "docs/sub/guide.md", got[1].Source)
		assert.Equal(t, "plan/045_api-overhaul.md", got[2].Source)
	})

	t.Run("anchor scopes to one source", func(t *testing.T) {
		got, errs := collectBacklinks(files, root, "docs/api.md", "authentication", nil, nil, 0, true)
		require.Empty(t, errs)
		require.Len(t, got, 1)
		assert.Equal(t, "docs/sub/guide.md", got[0].Source)
	})

	t.Run("anchor with no hits returns empty", func(t *testing.T) {
		got, errs := collectBacklinks(files, root, "docs/api.md", "no-such-section", nil, nil, 0, true)
		assert.Empty(t, errs)
		assert.Empty(t, got)
	})

	t.Run("include filter excludes plan/", func(t *testing.T) {
		got, errs := collectBacklinks(files, root, "docs/api.md", "", []string{"docs/**"}, nil, 0, true)
		require.Empty(t, errs)
		require.Len(t, got, 2)
		assert.Equal(t, "docs/index.md", got[0].Source)
		assert.Equal(t, "docs/sub/guide.md", got[1].Source)
	})

	t.Run("unreadable source surfaces as error", func(t *testing.T) {
		// A path that does not exist on disk: ReadFileLimited fails;
		// collectBacklinks captures the error rather than swallowing.
		bad := filepath.Join(root, "does-not-exist.md")
		filesWithBad := append([]string{bad}, files...)
		got, errs := collectBacklinks(filesWithBad, root, "docs/api.md", "", nil, nil, 0, true)
		// The other files still contribute results.
		assert.NotEmpty(t, got)
		require.Len(t, errs, 1)
		assert.Contains(t, errs[0].Error(), "does-not-exist.md")
	})

}

func TestCollectBacklinks_LocalAnchorSkipped(t *testing.T) {
	// Source contains only a same-file anchor link, no cross-file
	// reference to the target. linkgraph yields a LocalAnchor=true
	// link; collectBacklinks must skip it without trying to resolve a
	// path target.
	root, files := setupCollectBacklinksFixture(t)
	anchorOnly := filepath.Join(root, "anchor-only.md")
	require.NoError(t, os.WriteFile(anchorOnly,
		[]byte("# Intro\n\nJump to [section](#section).\n\n## Section\n"), 0o644))
	filesWithAnchor := append([]string{anchorOnly}, files...)
	got, errs := collectBacklinks(filesWithAnchor, root, "docs/api.md", "", nil, nil, 0, true)
	assert.Empty(t, errs)
	// Same three matches as before; anchor-only.md contributes nothing.
	assert.Len(t, got, 3)
}

func TestCollectBacklinks_IgnorePatternsExclude(t *testing.T) {
	// `plan/**` ignores the plan/* source that links to api.md;
	// the other two sources still produce records.
	root, files := setupCollectBacklinksFixture(t)
	got, errs := collectBacklinks(
		files, root, "docs/api.md", "",
		nil, []string{"plan/**"}, 0, true,
	)
	require.Empty(t, errs)
	require.Len(t, got, 2)
	for _, r := range got {
		assert.NotContains(t, r.Source, "plan/")
	}
}

// TestCollectBacklinks_SelfLink confirms self-references count as
// incoming edges. The contract is "every workspace file that links
// to <target>" — a file that links to itself satisfies that as
// literally as any other source.
func TestCollectBacklinks_SelfLink(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "docs"), 0o755))
	// docs/api.md links to itself via `api.md`.
	require.NoError(t, os.WriteFile(filepath.Join(root, "docs", "api.md"),
		[]byte("# API\n\nSee [back to top](api.md).\n"), 0o644))
	files := []string{filepath.Join(root, "docs", "api.md")}

	got, errs := collectBacklinks(files, root, "docs/api.md", "", nil, nil, 0, true)
	require.Empty(t, errs)
	require.Len(t, got, 1)
	assert.Equal(t, "docs/api.md", got[0].Source)
	assert.Equal(t, "api.md", got[0].Target)
}

// TestCollectBacklinks_FrontMatterStrippingDisabled verifies the
// stripFrontMatter parameter is honored. When set to false (matching
// `frontMatter: false` in config), collectBacklinks parses the entire
// file including its front matter — line numbers stay in raw file
// coordinates rather than body-relative.
func TestCollectBacklinks_FrontMatterStrippingDisabled(t *testing.T) {
	root, files := setupCollectBacklinksFixture(t)
	fmSrc := filepath.Join(root, "fm-src.md")
	require.NoError(t, os.WriteFile(fmSrc,
		[]byte("---\ntitle: x\n---\n# H\n\nSee [api](docs/api.md).\n"), 0o644))
	filesWithFM := append([]string{fmSrc}, files...)
	got, errs := collectBacklinks(filesWithFM, root, "docs/api.md", "", nil, nil, 0, false)
	require.Empty(t, errs)
	var fmRec *backlinkRecord
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

func TestValidateIncludePatterns(t *testing.T) {
	assert.NoError(t, validateIncludePatterns(nil))
	assert.NoError(t, validateIncludePatterns([]string{"docs/**", "plan/*.md"}))
	// `[` opens a character class that's never closed; doublestar
	// would silently mismatch every path, so we reject it upfront.
	err := validateIncludePatterns([]string{"["})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid --include glob")
}

func TestWorkspaceRelativePath_EmptyRootDir(t *testing.T) {
	// When rootDir is empty, the helper just strips a leading "./"
	// and forwards the path through.
	assert.Equal(t, "docs/api.md", workspaceRelativePath("./docs/api.md", ""))
	assert.Equal(t, "docs/api.md", workspaceRelativePath("docs/api.md", ""))
}

// TestCollectBacklinks_SortStable verifies that two records from the
// same source file are returned in line order (the secondary key in
// the SliceStable comparator).
func TestCollectBacklinks_SortStable(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "target.md"), []byte("# T\n"), 0o644))
	// Two links to target.md from the SAME source: line 3 and line 5.
	body := "# Src\n\nFirst [a](target.md), and\n\nsecond [b](target.md).\n"
	require.NoError(t, os.WriteFile(filepath.Join(root, "src.md"), []byte(body), 0o644))

	files := []string{filepath.Join(root, "src.md"), filepath.Join(root, "target.md")}
	got, errs := collectBacklinks(files, root, "target.md", "", nil, nil, 0, true)
	require.Empty(t, errs)
	require.Len(t, got, 2)
	assert.Equal(t, "src.md", got[0].Source)
	assert.Equal(t, "src.md", got[1].Source)
	assert.Less(t, got[0].Line, got[1].Line, "same-source records sort by line")
}

func TestEmitBacklinks_LimitZeroNoCap(t *testing.T) {
	// limit=0 means "no cap" — every record is emitted.
	records := make([]backlinkRecord, 5)
	for i := range records {
		records[i] = backlinkRecord{Source: "a.md", Line: i + 1}
	}
	var buf bytes.Buffer
	code := emitBacklinks(&buf, records, "text", 0)
	assert.Equal(t, 0, code)
	assert.Equal(t, 5, strings.Count(buf.String(), "\n"))
}

// failingWriter is an io.Writer that returns an error on every Write
// so tests can exercise emitBacklinks' write-error branches.
type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) {
	return 0, fmt.Errorf("simulated write failure")
}

func TestEmitBacklinks_TextWriteError(t *testing.T) {
	records := []backlinkRecord{{Source: "a.md", Line: 1, Text: "t", Target: "x.md"}}
	code := emitBacklinks(failingWriter{}, records, "text", 0)
	assert.Equal(t, 2, code)
}

func TestEmitBacklinks_JSONWriteError(t *testing.T) {
	code := emitBacklinks(failingWriter{}, []backlinkRecord{{Source: "a.md", Line: 1}}, "json", 0)
	assert.Equal(t, 2, code)
}

func TestCollectBacklinks_Wikilinks(t *testing.T) {
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
		got, errs := collectBacklinks(files, root, "page.md", "", nil, nil, 0, true)
		require.Empty(t, errs)
		// Expect three wikilink hits — two on other.md, one on anchored.md.
		require.Len(t, got, 3)
		for _, r := range got {
			assert.Equal(t, "wikilink", r.Kind, "all wikilink hits must carry kind=wikilink")
		}
	})

	t.Run("anchor scoping", func(t *testing.T) {
		got, errs := collectBacklinks(files, root, "page.md", "section", nil, nil, 0, true)
		require.Empty(t, errs)
		require.Len(t, got, 1)
		assert.Equal(t, "anchored.md", got[0].Source)
		assert.Equal(t, "wikilink", got[0].Kind)
		assert.Equal(t, "page#Section", got[0].Target)
	})
}

func TestFormatBacklinkTextLine_Wikilink(t *testing.T) {
	bare := backlinkRecord{
		Source: "from.md", Line: 4, Text: "page", Target: "page", Kind: "wikilink",
	}
	assert.Equal(t, "from.md:4: [[page]]", formatBacklinkTextLine(bare))

	alias := backlinkRecord{
		Source: "from.md", Line: 4, Text: "Display", Target: "page", Kind: "wikilink",
	}
	assert.Equal(t, "from.md:4: [[page|Display]]", formatBacklinkTextLine(alias))

	std := backlinkRecord{
		Source: "from.md", Line: 1, Text: "ref", Target: "x.md", Kind: "link",
	}
	assert.Equal(t, "from.md:1: [ref](x.md)", formatBacklinkTextLine(std))
}
