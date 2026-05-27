package release

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sampleMessaging returns a Messaging value populated with
// distinct non-empty fields for every key — every fixture in
// this file uses it so an unintended cross-wire in the
// registry (e.g. VS Code description fed Tagline) fails a
// test instead of silently shipping.
func sampleMessaging() *Messaging {
	return &Messaging{
		Title:                       "title",
		Summary:                     "summary",
		Eyebrow:                     "eyebrow text",
		HeadlinePre:                 "Pre",
		HeadlineEm:                  "Em",
		HeadlinePost:                "Post",
		Lead:                        "lead paragraph",
		Tagline:                     "the tagline",
		VSCodeDescription:           "vscode desc",
		VSCodeOverview:              "vscode overview text",
		ClaudeCodeLSPDescription:    "claude lsp desc",
		ClaudeCodeSkillsDescription: "claude skills desc",
		ClaudeCodeAuditDescription:  "claude audit desc",
	}
}

func TestMessagingTargets_StableOrder(t *testing.T) {
	a := MessagingTargets("/r")
	b := MessagingTargets("/r")
	require.Len(t, a, len(b))
	for i := range a {
		assert.Equal(t, a[i].Label, b[i].Label,
			"target order changed between calls at index %d", i)
	}
}

func TestMessagingTargets_FragmentsFirst(t *testing.T) {
	targets := MessagingTargets("/r")
	require.GreaterOrEqual(t, len(targets), 2)
	assert.Equal(t, "tagline fragment", targets[0].Label)
	assert.Equal(t, "lead fragment", targets[1].Label)
}

func TestMessagingTargets_AllValueOfReturnsNonEmpty(t *testing.T) {
	m := sampleMessaging()
	for _, tg := range MessagingTargets("/r") {
		got := tg.ValueOf(m)
		assert.NotEmpty(t, got, "target %q yields empty value", tg.Label)
	}
}

func TestMessagingTargets_VSCodeUsesVSCodeDescription(t *testing.T) {
	m := sampleMessaging()
	for _, tg := range MessagingTargets("/r") {
		if tg.Label == "vscode/package.json description" {
			assert.Equal(t, "vscode desc", tg.ValueOf(m))
			return
		}
	}
	t.Fatal("vscode target not found")
}

func TestMessagingTargets_ClaudeCodeLSPUsesLSPDescription(t *testing.T) {
	m := sampleMessaging()
	for _, tg := range MessagingTargets("/r") {
		if tg.Label == "claude-code plugin.json description" {
			assert.Equal(t, "claude lsp desc", tg.ValueOf(m))
			return
		}
	}
	t.Fatal("claude-code LSP target not found")
}

// ----- ApplyMessaging / CheckMessaging ------------------------------------

// applyTestRoot stages a minimal repo with every tracked
// surface present in a starting state. The fixtures here are
// trimmed copies of the real files — enough for the apply
// path to find each field, not the full content.
func applyTestRoot(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for rel, body := range applyTestFixtures() {
		full := filepath.Join(dir, rel)
		require.NoError(t, mkdirAllForFile(full))
		require.NoError(t, writeFileAt(full, []byte(body)))
	}
	return dir
}

func applyTestFixtures() map[string]string {
	return map[string]string{
		"website/hugo.toml": `baseURL = "https://example/"

[params]
  description = "starting hugo description"
  version = "0.0.0-dev"
`,
		"website/content/_index.md": `---
title: "mdsmith"
summary: "starting summary"
hero:
  eyebrow: "starting eyebrow"
  headline_pre: "Pre0"
  headline_em: "Em0"
  headline_post: "Post0"
  lead: >-
    starting lead.
---
Body content.
`,
		"npm/mdsmith/package.json": `{
  "name": "@mdsmith/cli",
  "version": "0.0.0-dev",
  "description": "starting npm description"
}
`,
		"python/pyproject.toml": `[project]
name = "mdsmith"
version = "0.0.0-dev"
description = "starting pypi description"
`,
		"editors/vscode/package.json": `{
  "name": "mdsmith",
  "description": "starting vscode description",
  "version": "0.0.0-dev"
}
`,
		"editors/claude-code/.claude-plugin/plugin.json": `{
  "name": "mdsmith-lsp",
  "description": "starting lsp description"
}
`,
		"editors/claude-code-skills/.claude-plugin/plugin.json": `{
  "name": "mdsmith-skills",
  "description": "starting skills description"
}
`,
		"editors/claude-code-audit/.claude-plugin/plugin.json": `{
  "name": "mdsmith-audit",
  "description": "starting audit description"
}
`,
	}
}

func TestApplyMessaging_WritesEveryTrackedSurface(t *testing.T) {
	root := applyTestRoot(t)
	m := sampleMessaging()
	results, err := ApplyMessaging(root, m)
	require.NoError(t, err)
	require.Len(t, results, len(MessagingTargets(root)))
	// First apply: every target either creates a fragment or
	// rewrites an existing surface; nothing should be left as
	// "unchanged" because the starting values differ.
	for _, r := range results {
		assert.True(t, r.Changed, "%s did not change on first apply", r.Target.Label)
	}
}

func TestApplyMessaging_IsIdempotent(t *testing.T) {
	root := applyTestRoot(t)
	m := sampleMessaging()
	_, err := ApplyMessaging(root, m)
	require.NoError(t, err)
	results, err := ApplyMessaging(root, m)
	require.NoError(t, err)
	for _, r := range results {
		assert.False(t, r.Changed, "%s reported a change on second apply",
			r.Target.Label)
	}
}

func TestApplyMessaging_CreatesFragmentDirectory(t *testing.T) {
	root := applyTestRoot(t)
	m := sampleMessaging()
	_, err := ApplyMessaging(root, m)
	require.NoError(t, err)
	for _, tg := range MessagingTargets(root) {
		if _, ok := tg.Patcher.(MarkdownFragment); ok {
			body, err := readFileAt(tg.Path)
			require.NoError(t, err, "fragment %s missing", tg.Label)
			assert.Contains(t, string(body),
				"Generated by `mdsmith-release sync-messaging`")
		}
	}
}

func TestCheckMessaging_CleanTreeNoDrift(t *testing.T) {
	root := applyTestRoot(t)
	m := sampleMessaging()
	_, err := ApplyMessaging(root, m)
	require.NoError(t, err)
	drifts, err := CheckMessaging(root, m)
	require.NoError(t, err)
	assert.Empty(t, drifts, "clean tree should report no drift")
}

func TestCheckMessaging_ReportsDriftWithDiff(t *testing.T) {
	root := applyTestRoot(t)
	m := sampleMessaging()
	// Apply once so every target is in sync.
	_, err := ApplyMessaging(root, m)
	require.NoError(t, err)
	// Hand-edit one surface back to a drifted value.
	pkgPath := filepath.Join(root, "editors/vscode/package.json")
	require.NoError(t, writeFileAt(pkgPath, []byte(`{
  "name": "mdsmith",
  "description": "drifted!"
}
`)))
	drifts, err := CheckMessaging(root, m)
	require.NoError(t, err)
	require.Len(t, drifts, 1)
	assert.Equal(t, "vscode/package.json description", drifts[0].Target.Label)
	assert.Equal(t, "drifted!", drifts[0].Have)
	assert.Equal(t, m.VSCodeDescription, drifts[0].Want)
}

func TestFormatDrift_RendersReadableReport(t *testing.T) {
	drifts := []MessagingDrift{
		{
			Target: MessagingTarget{
				Label: "test target",
				Path:  "/tmp/x",
			},
			Have: "current",
			Want: "new",
		},
	}
	out := FormatDrift(drifts)
	assert.Contains(t, out, "messaging drift detected:")
	assert.Contains(t, out, "test target")
	assert.Contains(t, out, "have: current")
	assert.Contains(t, out, "want: new")
	assert.True(t, strings.HasSuffix(out, "sync-messaging` to update.\n"))
}

func TestApplyMessaging_FailsWhenRequiredFileMissing(t *testing.T) {
	// Delete one non-fragment target (a JSON manifest); apply
	// must return "required file missing" because only fragment
	// targets are created on demand.
	root := applyTestRoot(t)
	require.NoError(t,
		os.Remove(filepath.Join(root, "npm/mdsmith/package.json")))
	_, err := ApplyMessaging(root, sampleMessaging())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "required file missing")
}

func TestApplyMessaging_FailsOnCorruptTargetFile(t *testing.T) {
	// Replace a JSON manifest with non-JSON bytes; the patcher
	// returns an error which applyTarget must surface.
	root := applyTestRoot(t)
	require.NoError(t,
		os.WriteFile(filepath.Join(root, "npm/mdsmith/package.json"),
			[]byte("not json at all"), 0o644))
	_, err := ApplyMessaging(root, sampleMessaging())
	require.Error(t, err)
}

func TestCheckMessaging_ReportsMissingFileAsDrift(t *testing.T) {
	// CheckMessaging treats a missing target as drift (have:
	// "<missing>") rather than an error, so the report tells the
	// caller exactly what is gone.
	root := applyTestRoot(t)
	require.NoError(t,
		os.Remove(filepath.Join(root, "npm/mdsmith/package.json")))
	drifts, err := CheckMessaging(root, sampleMessaging())
	require.NoError(t, err)
	require.NotEmpty(t, drifts)
	found := false
	for _, d := range drifts {
		if d.Have == "<missing>" {
			found = true
			break
		}
	}
	assert.True(t, found, "missing file did not appear as <missing> drift")
}

func TestCheckMessaging_FailsOnCorruptTargetFile(t *testing.T) {
	// ReadValue on a malformed file surfaces as an error;
	// CheckMessaging propagates it because drift can't be
	// computed against unparseable bytes.
	root := applyTestRoot(t)
	require.NoError(t,
		os.WriteFile(filepath.Join(root, "npm/mdsmith/package.json"),
			[]byte("not json at all"), 0o644))
	_, err := CheckMessaging(root, sampleMessaging())
	require.Error(t, err)
}

func TestFormatDrift_EmptyOnCleanTree(t *testing.T) {
	assert.Empty(t, FormatDrift(nil))
}

func TestOneLineForDrift_TruncatesLongValues(t *testing.T) {
	// A value longer than the 117-rune budget gets cut and the
	// "..." suffix appended.
	long := strings.Repeat("x", 200)
	got := oneLineForDrift(long)
	assert.Equal(t, 120, len(got))
	assert.True(t, strings.HasSuffix(got, "..."))
}

func TestOneLineForDrift_TruncatesByRunesNotBytes(t *testing.T) {
	// 200 em-dashes (3 bytes each in UTF-8) must truncate by
	// rune so the output stays valid UTF-8.
	long := strings.Repeat("—", 200)
	got := oneLineForDrift(long)
	assert.True(t, strings.HasSuffix(got, "..."))
	for _, r := range got {
		assert.NotEqual(t, '�', r, "truncation split a rune")
	}
}

// faultyFS wraps a real os-backed FS, overriding one method to
// inject a failure. Used to cover the mkdir/write error branches
// in applyTarget that no on-disk setup can reliably trigger
// (chmod tricks are skipped under root, which CI sometimes is).
type faultyFS struct {
	FS
	failMkdir bool
	failWrite bool
}

func (f *faultyFS) MkdirAll(path string, perm fs.FileMode) error {
	if f.failMkdir {
		return errors.New("synthetic mkdir failure")
	}
	return f.FS.MkdirAll(path, perm)
}

func (f *faultyFS) WriteFile(name string, data []byte, perm fs.FileMode) error {
	if f.failWrite {
		return errors.New("synthetic write failure")
	}
	return f.FS.WriteFile(name, data, perm)
}

func TestApplyMessaging_FailsOnMkdirError(t *testing.T) {
	root := applyTestRoot(t)
	tk := NewWithFS(&faultyFS{FS: osFS{}, failMkdir: true})
	_, err := tk.ApplyMessaging(root, sampleMessaging())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mkdir")
}

func TestApplyMessaging_FailsOnWriteError(t *testing.T) {
	root := applyTestRoot(t)
	tk := NewWithFS(&faultyFS{FS: osFS{}, failWrite: true})
	_, err := tk.ApplyMessaging(root, sampleMessaging())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "write")
}

func TestApplyMessaging_FailsOnWriteError_directory(t *testing.T) {
	// All target files exist (so ReadFile succeeds and the
	// missing-file branch doesn't fire), but the fragment target
	// already exists as a directory of the expected name, so
	// WriteFile fails with "is a directory". The write-error
	// branch surfaces.
	if os.Geteuid() == 0 {
		t.Skip("running as root: directory-as-file write succeeds")
	}
	root := applyTestRoot(t)
	frag := filepath.Join(root, "docs/brand/fragments/tagline.fragment.md")
	require.NoError(t, os.MkdirAll(frag, 0o755))
	_, err := ApplyMessaging(root, sampleMessaging())
	require.Error(t, err)
}

func TestApplyMessaging_FailsOnReadError(t *testing.T) {
	// Replace a target file with a directory of the same name;
	// ReadFile then returns an error that is not fs.ErrNotExist
	// (it is "is a directory"). applyTarget must surface the
	// error rather than the missing-file branch.
	root := applyTestRoot(t)
	pkg := filepath.Join(root, "npm/mdsmith/package.json")
	require.NoError(t, os.Remove(pkg))
	require.NoError(t, os.Mkdir(pkg, 0o755))
	_, err := ApplyMessaging(root, sampleMessaging())
	require.Error(t, err)
}

func TestCheckMessaging_FailsOnReadError(t *testing.T) {
	// Same directory-instead-of-file trick on the Check side.
	root := applyTestRoot(t)
	pkg := filepath.Join(root, "npm/mdsmith/package.json")
	require.NoError(t, os.Remove(pkg))
	require.NoError(t, os.Mkdir(pkg, 0o755))
	_, err := CheckMessaging(root, sampleMessaging())
	require.Error(t, err)
}

// Local IO helpers — kept tiny here to avoid a dependency on
// the unexported `release` Toolkit FS surface in tests that
// only need the on-disk default.
func mkdirAllForFile(path string) error {
	return mkdirAll(filepath.Dir(path))
}

func mkdirAll(dir string) error {
	return New().fs.MkdirAll(dir, 0o755)
}

func writeFileAt(path string, body []byte) error {
	return New().fs.WriteFile(path, body, 0o644)
}

func readFileAt(path string) ([]byte, error) {
	return New().fs.ReadFile(path)
}
