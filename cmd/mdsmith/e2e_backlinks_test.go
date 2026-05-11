package main_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// backlinkRecordE2E mirrors cmd/mdsmith.backlinkRecord with exported
// JSON keys. Kept local to avoid coupling the test package to the
// internal struct definition.
type backlinkRecordE2E struct {
	Source string `json:"source"`
	Line   int    `json:"line"`
	Text   string `json:"text"`
	Target string `json:"target"`
}

// setupBacklinksWorkspace creates a minimal workspace with three
// distinct sources linking to docs/api.md, plus an anchor case.
func setupBacklinksWorkspace(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	mk := func(rel string) {
		require.NoError(t, os.MkdirAll(filepath.Join(dir, rel), 0o755))
	}
	wf := func(rel, body string) {
		require.NoError(t, os.WriteFile(filepath.Join(dir, rel), []byte(body), 0o644))
	}
	mk("docs/sub")
	mk("plan")
	// Mark the temp dir as a project root so config discovery does
	// not climb out into the repo's own .mdsmith.yml.
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".git"), 0o755))
	wf(".mdsmith.yml", "files:\n  - \"**/*.md\"\nrules:\n  cross-file-reference-integrity: false\n")
	wf("docs/api.md", "# API\n\n## Authentication\n\n## Endpoints\n")
	wf("docs/index.md", "# Index\n\nSee [API reference](api.md).\n")
	wf("docs/sub/guide.md", "# Guide\n\nUse [api docs](../api.md#authentication).\n")
	wf("plan/045_api-overhaul.md", "# Plan\n\n[api](../docs/api.md)\n")
	wf("docs/changelog.md", "# Changelog\n\n[plan](../plan/045_api-overhaul.md)\n")
	return dir
}

func TestE2E_Backlinks_ThreeSources_Text(t *testing.T) {
	dir := setupBacklinksWorkspace(t)
	stdout, _, exitCode := runBinaryInDir(t, dir, "", "list", "backlinks", "docs/api.md")
	require.Equal(t, 0, exitCode, "expected exit 0 when matches found")
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	require.Len(t, lines, 3, "expected 3 backlink rows, got: %q", stdout)
	// Sorted by source path.
	assert.True(t, strings.HasPrefix(lines[0], "docs/index.md:"))
	assert.True(t, strings.HasPrefix(lines[1], "docs/sub/guide.md:"))
	assert.True(t, strings.HasPrefix(lines[2], "plan/045_api-overhaul.md:"))
	assert.Contains(t, lines[0], "[API reference](api.md)")
}

func TestE2E_Backlinks_Anchor_Filter(t *testing.T) {
	dir := setupBacklinksWorkspace(t)
	stdout, _, exitCode := runBinaryInDir(t, dir, "", "list", "backlinks", "docs/api.md#authentication")
	require.Equal(t, 0, exitCode)
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	require.Len(t, lines, 1, "anchor-scoped query expected to return one row, got: %q", stdout)
	assert.Contains(t, lines[0], "docs/sub/guide.md")
}

func TestE2E_Backlinks_Anchor_NoHits_ExitsOne(t *testing.T) {
	dir := setupBacklinksWorkspace(t)
	stdout, _, exitCode := runBinaryInDir(t, dir, "", "list", "backlinks", "docs/api.md#no-such-section")
	require.Equal(t, 1, exitCode, "no matches → exit 1")
	assert.Empty(t, strings.TrimSpace(stdout))
}

func TestE2E_Backlinks_JSON_Shape(t *testing.T) {
	dir := setupBacklinksWorkspace(t)
	stdout, _, exitCode := runBinaryInDir(t, dir, "", "list", "backlinks", "--format", "json", "docs/api.md")
	require.Equal(t, 0, exitCode)
	var got []backlinkRecordE2E
	require.NoError(t, json.Unmarshal([]byte(stdout), &got))
	require.Len(t, got, 3)
	// Stable shape per docs/reference/cli/backlinks.md.
	assert.Equal(t, "docs/index.md", got[0].Source)
	assert.Equal(t, "API reference", got[0].Text)
	assert.Equal(t, "api.md", got[0].Target)
	assert.Greater(t, got[0].Line, 0)
}

func TestE2E_Backlinks_IncludeAndLimit(t *testing.T) {
	dir := setupBacklinksWorkspace(t)
	// --include docs/** filters out the plan/ source; --limit 1
	// then caps the remaining two rows.
	stdout, _, exitCode := runBinaryInDir(
		t, dir, "", "list", "backlinks", "--include", "docs/**", "--limit", "1", "docs/api.md")
	require.Equal(t, 0, exitCode)
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	require.Len(t, lines, 1, "include + limit: expected 1 row, got: %q", stdout)
	assert.Contains(t, lines[0], "docs/index.md")
	assert.NotContains(t, stdout, "plan/")
}

func TestE2E_Backlinks_NoTarget_ExitsTwo(t *testing.T) {
	_, stderr, exitCode := runBinary(t, "", "list", "backlinks")
	assert.Equal(t, 2, exitCode)
	assert.Contains(t, stderr, "requires a target file argument")
}

func TestE2E_Backlinks_HelpFlag(t *testing.T) {
	_, stderr, exitCode := runBinary(t, "", "list", "backlinks", "--help")
	assert.Equal(t, 0, exitCode)
	assert.Contains(t, stderr, "Usage: mdsmith list backlinks")
}

func TestE2E_Backlinks_InvalidIncludeGlob_ExitsTwo(t *testing.T) {
	dir := setupBacklinksWorkspace(t)
	_, stderr, exitCode := runBinaryInDir(
		t, dir, "", "list", "backlinks", "--include", "[", "docs/api.md")
	require.Equal(t, 2, exitCode, "invalid glob must exit 2, not silently match nothing")
	assert.Contains(t, stderr, "invalid --include glob")
}

func TestE2E_Backlinks_TooManyArgs_ExitsTwo(t *testing.T) {
	_, stderr, exitCode := runBinary(t, "", "list", "backlinks", "a.md", "b.md")
	require.Equal(t, 2, exitCode)
	assert.Contains(t, stderr, "takes one target argument")
}

func TestE2E_Backlinks_AnchorOnlyTarget_ExitsTwo(t *testing.T) {
	// "#anchor" alone has no file path; backlinks always require one.
	_, stderr, exitCode := runBinary(t, "", "list", "backlinks", "#anchor-only")
	require.Equal(t, 2, exitCode)
	assert.Contains(t, stderr, "target must include a file path")
}

func TestE2E_Backlinks_NegativeLimit_ExitsTwo(t *testing.T) {
	// --limit is documented as `0 = no cap`; negative values would
	// otherwise be silently treated as "no cap" because emitBacklinks
	// only caps when limit > 0. Reject upfront.
	dir := setupBacklinksWorkspace(t)
	_, stderr, exitCode := runBinaryInDir(
		t, dir, "", "list", "backlinks", "--limit", "-1", "docs/api.md")
	require.Equal(t, 2, exitCode)
	assert.Contains(t, stderr, "--limit must be >= 0")
}

func TestE2E_Backlinks_InvalidMaxInputSize_ExitsTwo(t *testing.T) {
	dir := setupBacklinksWorkspace(t)
	_, stderr, exitCode := runBinaryInDir(
		t, dir, "", "list", "backlinks", "--max-input-size", "garbage", "docs/api.md")
	require.Equal(t, 2, exitCode)
	assert.Contains(t, stderr, "max-input-size")
}

func TestE2E_Backlinks_JSON_EmptyResult(t *testing.T) {
	dir := setupBacklinksWorkspace(t)
	// A target that exists but has no incoming links.
	stdout, _, exitCode := runBinaryInDir(
		t, dir, "", "list", "backlinks", "--format", "json", "docs/changelog.md")
	require.Equal(t, 1, exitCode, "no matches → exit 1")
	// Stable shape: `[]`, never `null`.
	assert.Contains(t, stdout, "[]")
	assert.NotContains(t, stdout, "null")
}

func TestE2E_Backlinks_UnreadableSource_ExitsTwo(t *testing.T) {
	// When every source file is too large to read, collectBacklinks
	// reports an error per file and emits no records. Per the doc'd
	// precedence (records → 0, runtime err → 2, clean empty → 1) the
	// exit code must be 2, not 1.
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".git"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".mdsmith.yml"),
		[]byte("files:\n  - \"**/*.md\"\nrules:\n  cross-file-reference-integrity: false\n"), 0o644))
	// 200 bytes of body — comfortably larger than the 10-byte cap.
	src := "# Src\n\n[ref](target.md) " + strings.Repeat("x", 200) + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src.md"), []byte(src), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "target.md"), []byte("# Target\n"), 0o644))

	_, stderr, exitCode := runBinaryInDir(
		t, dir, "", "list", "backlinks", "--max-input-size", "10", "target.md")
	require.Equal(t, 2, exitCode, "unreadable sources → runtime error → exit 2")
	assert.Contains(t, stderr, "reading src.md")
}

func TestE2E_Backlinks_PercentEncodedTarget(t *testing.T) {
	// A workspace file with a space in its name, linked to via
	// `%20` from another source. linkgraph.ParseTarget decodes the
	// link destination to `my file.md`, so the CLI target must also
	// decode to match — otherwise `backlinks docs/my%20file.md`
	// silently returns nothing.
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".git"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".mdsmith.yml"),
		[]byte("files:\n  - \"**/*.md\"\nrules:\n  cross-file-reference-integrity: false\n"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "docs"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "docs", "my file.md"),
		[]byte("# Target\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src.md"),
		[]byte("# Src\n\nSee [it](docs/my%20file.md).\n"), 0o644))

	// Encoded query form.
	stdout, _, exitCode := runBinaryInDir(t, dir, "", "list", "backlinks", "docs/my%20file.md")
	require.Equal(t, 0, exitCode)
	assert.Contains(t, stdout, "src.md:")
	// Decoded query form must also match.
	stdout, _, exitCode = runBinaryInDir(t, dir, "", "list", "backlinks", "docs/my file.md")
	require.Equal(t, 0, exitCode)
	assert.Contains(t, stdout, "src.md:")
}

func TestE2E_Backlinks_DiscoveryEmpty_ExitsOne(t *testing.T) {
	// A workspace whose `files:` glob matches nothing should exit 1
	// (no backlinks found) rather than crash.
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".git"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".mdsmith.yml"),
		[]byte("files:\n  - \"no-such-pattern/**/*.md\"\nrules: {}\n"), 0o644))

	_, _, exitCode := runBinaryInDir(t, dir, "", "list", "backlinks", "target.md")
	require.Equal(t, 1, exitCode)
}

func TestE2E_Backlinks_AbsoluteTarget_ExitsTwo(t *testing.T) {
	// `<target>` is documented as workspace-relative. An absolute path
	// would otherwise normalize to a path outside the workspace and
	// silently match nothing, indistinguishable from a real empty
	// result. Reject upfront.
	dir := setupBacklinksWorkspace(t)
	_, stderr, exitCode := runBinaryInDir(t, dir, "", "list", "backlinks", "/etc/passwd")
	require.Equal(t, 2, exitCode)
	assert.Contains(t, stderr, "must be workspace-relative")
}

func TestE2E_Backlinks_ParentTraversalTarget_ExitsTwo(t *testing.T) {
	// Same justification: `../foo.md` resolves outside the workspace
	// and would silently match nothing.
	dir := setupBacklinksWorkspace(t)
	_, stderr, exitCode := runBinaryInDir(t, dir, "", "list", "backlinks", "../foo.md")
	require.Equal(t, 2, exitCode)
	assert.Contains(t, stderr, "must be workspace-relative")
}

// TestE2E_Backlinks_EncodedEscapeRejected guards against a bypass of
// the workspace-relative target contract: `%2Fetc%2Fpasswd` decodes
// to `/etc/passwd` and `%2e%2e/foo.md` to `../foo.md`, both of which
// must be rejected with exit 2.
func TestE2E_Backlinks_EncodedEscapeRejected(t *testing.T) {
	dir := setupBacklinksWorkspace(t)
	cases := []string{
		"%2Fetc%2Fpasswd",
		"%2e%2e/foo.md",
	}
	for _, target := range cases {
		t.Run(target, func(t *testing.T) {
			_, stderr, exitCode := runBinaryInDir(t, dir, "", "list", "backlinks", target)
			require.Equal(t, 2, exitCode)
			assert.Contains(t, stderr, "must be workspace-relative")
		})
	}
}

// TestE2E_Backlinks_MalformedTargetRejected ensures inputs the link
// parser cannot decode (malformed percent escapes, query-only URLs,
// schemed URLs) fail loudly with exit 2 instead of silently producing
// an empty result.
func TestE2E_Backlinks_MalformedTargetRejected(t *testing.T) {
	dir := setupBacklinksWorkspace(t)
	cases := []string{
		"docs/my%ZZfile.md", // %ZZ is not a valid percent-escape
		"?q=1",              // query-only, no path
		"https://example.com/x.md",
	}
	for _, target := range cases {
		t.Run(target, func(t *testing.T) {
			_, stderr, exitCode := runBinaryInDir(t, dir, "", "list", "backlinks", target)
			require.Equal(t, 2, exitCode)
			assert.Contains(t, stderr, "invalid target")
		})
	}
}

func TestE2E_Backlinks_RespectsIgnore(t *testing.T) {
	// Sources matched by `ignore:` patterns must not contribute
	// backlinks, mirroring `check` / `fix` behavior.
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".git"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "vendor"), 0o755))
	cfg := "files:\n  - \"**/*.md\"\nignore:\n  - \"vendor/**\"\n" +
		"rules:\n  cross-file-reference-integrity: false\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".mdsmith.yml"), []byte(cfg), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "target.md"), []byte("# Target\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src.md"),
		[]byte("# Src\n\n[ok](target.md)\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "vendor", "lib.md"),
		[]byte("# Vendor\n\n[skip](../target.md)\n"), 0o644))

	stdout, _, exitCode := runBinaryInDir(t, dir, "", "list", "backlinks", "target.md")
	require.Equal(t, 0, exitCode)
	assert.Contains(t, stdout, "src.md:")
	assert.NotContains(t, stdout, "vendor/", "ignored sources must not appear in backlinks output")
}

func TestE2E_Backlinks_FrontMatterLines_FileRelative(t *testing.T) {
	// Regression for the body-vs-file-relative line bug: when the
	// source file has front matter, the backlinks CLI must show the
	// file-relative line number, not the body-relative one.
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".git"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".mdsmith.yml"),
		[]byte("files:\n  - \"**/*.md\"\nrules:\n  cross-file-reference-integrity: false\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "target.md"),
		[]byte("# Target\n"), 0o644))
	// 3 lines of front matter, blank, heading, blank, link → line 6.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src.md"),
		[]byte("---\ntitle: src\n---\n# Src\n\nSee [it](target.md).\n"), 0o644))

	stdout, _, exitCode := runBinaryInDir(t, dir, "", "list", "backlinks", "target.md")
	require.Equal(t, 0, exitCode)
	assert.Contains(t, stdout, "src.md:6:")
}
