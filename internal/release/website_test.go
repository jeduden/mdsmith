package release

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRenderLinkHook_SubpathBaseURL is a regression test for the
// site-absolute href prefix fix from PR #309. A deploy with a
// non-root baseURL (e.g. https://example.com/mdsmith/) must
// prefix every site-absolute href with the configured path
// prefix. This test asserts the render-link hook derives that
// prefix from site.Home.RelPermalink — which Hugo sets to
// "/mdsmith/" for a sub-path deploy — rather than hardcoding
// "/". Without this, site-absolute hrefs (those starting with
// "/") resolve against the server root, not the sub-path.
func TestRenderLinkHook_SubpathBaseURL(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	require.NoError(t, err)
	hookPath := filepath.Join(
		repoRoot,
		"website", "layouts", "_default", "_markup", "render-link.html",
	)
	data, err := os.ReadFile(hookPath)
	require.NoError(t, err, "render-link.html must exist at %s", hookPath)

	hook := string(data)
	// The hook must use site.Home.RelPermalink to build the prefix
	// for the $absolute branch — hardcoding "/" would break a
	// sub-path deploy.
	assert.Contains(t, hook, "site.Home.RelPermalink",
		"site-absolute branch must derive prefix from site.Home.RelPermalink")
	// The trailing slash of RelPermalink must be stripped before
	// concatenation so root deploys get "/" not "//" and sub-path
	// deploys get "/mdsmith/docs/…" not "/mdsmith//docs/…".
	assert.Contains(t, hook, `strings.TrimSuffix "/" site.Home.RelPermalink`,
		"trailing slash of site.Home.RelPermalink must be stripped before concatenation")
}

const ruleIndexFixture = `---
title: Rule Directory
summary: >-
  Complete list of all mdsmith rules.
---
# Rule Directory

All mdsmith rules.

<?catalog
glob: "MDS*/README.md"
?>
- [MDS001](MDS001-line-length/README.md)
<?/catalog?>
`

const ruleReadmeFixture = `---
id: MDS001
name: line-length
status: ready
description: Line exceeds maximum length.
category: line
nature: content
maintainability: null
---
# MDS001: line-length

Line exceeds maximum length.

## Settings

| Setting | Type | Default | Description |
|---------|------|---------|-------------|
| ` + "`max`" + ` | int | 80 | Maximum line length |

See also [MDS021](../MDS021-include/README.md) and [MDS027][mds027].
Sibling rule with anchor: [MDS021 anchor](../MDS021-include/README.md#syntax).
Anchor link: [MDS020 anchor](../../../internal/rules/MDS020-required-structure/README.md#index-side-output).

See the [placeholder grammar](../../../docs/background/concepts/placeholder-grammar.md)
and the [schemas guide](../../../docs/guides/schemas.md#section-content).

See [Plan 107](../../../plan/107_no-reference-style.md) for background.

Fixture examples: [good/default.md](good/default.md) and [bad/x.md](bad/x.md).
Pattern directory: [pattern/good/](pattern/good/).
Sibling Go package: [linelength rule](../linelength/rule.go).
Schema: [proto.md](../proto.md).

## Meta-Information

- **Implementation**:
  [source](./)

## See also

- [MDS027 cross-file-reference-integrity][mds027]
- [Plan 107][plan107]

[mds027]: ../MDS027-cross-file-reference-integrity/README.md
[plan107]: ../../../plan/107_no-reference-style.md
`

// ruleIndexAt writes the rule-directory fixture to
// <parent>/internal/rules/index.md so BuildWebsite's sibling
// lookup (filepath.Dir(srcDir)/internal/rules) finds it when
// srcDir is <parent>/docs.
func ruleIndexAt(t *testing.T, parent string) {
	t.Helper()
	writeFile(t, filepath.Join(parent, "internal", "rules", "index.md"), ruleIndexFixture)
}

// ruleReadmeAt writes a minimal rule README fixture to
// <parent>/internal/rules/<dir>/README.md.
func ruleReadmeAt(t *testing.T, parent, dir string) {
	t.Helper()
	writeFile(t, filepath.Join(parent, "internal", "rules", dir, "README.md"), ruleReadmeFixture)
}

func TestBuildWebsite_PublishesRuleDirectory(t *testing.T) {
	parent := t.TempDir()
	src := filepath.Join(parent, "docs")
	dst := filepath.Join(t.TempDir(), "out")
	writeFile(t, filepath.Join(src, "top.md"), "top body\n")
	ruleIndexAt(t, parent)

	require.NoError(t, NewWithDeps(osFS{}, &recordingRunner{}).BuildWebsite(src, dst, false))

	assertFile(t, filepath.Join(dst, "top.md"), "top body\n")
	got, err := os.ReadFile(filepath.Join(dst, "rules", "_index.md"))
	require.NoError(t, err, "Rules section page must be written")
	body := string(got)
	assert.Contains(t, body, "MDS001-line-length/",
		"rule-README link must be rewritten to a local site URL")
	assert.NotContains(t, body, "github.com",
		"local rule pages replace GitHub links in the index")
	assert.NotContains(t, body, "README.md",
		"README.md suffix must be stripped from rewritten links")
	assert.NotContains(t, body, "<?catalog", "directive markers must be stripped")
	assert.NotContains(t, body, "# Rule Directory", "the body H1 must be lifted to front matter")
	assert.Contains(t, body, "title: Rule Directory", "front-matter title is preserved")
	assert.Contains(t, body, "cascade:", "cascade block must be injected for rule layout type")
	assert.Contains(t, body, "type: rule", "cascade must set layout type to rule")
}

// buildRulePageBody runs BuildWebsite over a single-rule
// fixture and returns the synced rule page body so each of the
// per-rewrite tests below can assert against it without
// duplicating the setup.
func buildRulePageBody(t *testing.T) string {
	t.Helper()
	parent := t.TempDir()
	src := filepath.Join(parent, "docs")
	dst := filepath.Join(t.TempDir(), "out")
	writeFile(t, filepath.Join(src, "top.md"), "top body\n")
	ruleIndexAt(t, parent)
	ruleReadmeAt(t, parent, "MDS001-line-length")
	require.NoError(t, NewWithDeps(osFS{}, &recordingRunner{}).BuildWebsite(src, dst, false))
	got, err := os.ReadFile(filepath.Join(dst, "rules", "MDS001-line-length", "index.md"))
	require.NoError(t, err, "per-rule page must be written at rules/<dir>/index.md")
	return string(got)
}

func TestBuildWebsite_PublishesRulePages_FrontMatter(t *testing.T) {
	body := buildRulePageBody(t)
	assert.Contains(t, body, `title: "MDS001: line-length"`,
		"rule H1 must be lifted to front-matter title")
	assert.Contains(t, body, "github_source: internal/rules/MDS001-line-length/",
		"github_source field must be injected for source link")
	assert.NotContains(t, body, "# MDS001: line-length",
		"body H1 must be stripped after promotion")
}

func TestBuildWebsite_PublishesRulePages_SiblingLinks(t *testing.T) {
	body := buildRulePageBody(t)
	assert.Contains(t, body, "[MDS021](../MDS021-include/)",
		"sibling rule links must drop the README.md target")
	assert.Contains(t, body, "[MDS021 anchor](../MDS021-include/#syntax)",
		"sibling rule links with anchors must drop README.md and keep the anchor")
	assert.NotContains(t, body, "../MDS021-include/README.md",
		"no unpublished README.md link target may remain")
	assert.Contains(t, body,
		"[source](https://github.com/jeduden/mdsmith/tree/main/internal/rules/MDS001-line-length/)",
		"[source](./) self-link must be repointed at the GitHub tree URL")
	assert.NotContains(t, body, "[source](./)",
		"the on-site self-link must not survive")
	assert.Contains(t, body, "[mds027]: ../MDS027-cross-file-reference-integrity/",
		"reference-style rule link definitions must drop README.md")
	assert.NotContains(t, body, "[mds027]: ../MDS027-cross-file-reference-integrity/README.md",
		"raw reference def README.md target must not survive")
}

func TestBuildWebsite_PublishesRulePages_DocsAndPlanLinks(t *testing.T) {
	body := buildRulePageBody(t)
	assert.Contains(t, body, "](/docs/background/concepts/placeholder-grammar/)",
		"docs link must become site-absolute path (no .md extension)")
	assert.Contains(t, body, "](/docs/guides/schemas/#section-content)",
		"docs link with anchor must preserve the anchor after the trailing slash")
	assert.NotContains(t, body, "../../../docs/",
		"no repo-relative docs/ link may remain on the published page")
	assert.Contains(t, body,
		"](https://github.com/jeduden/mdsmith/blob/main/plan/107_no-reference-style.md)",
		"plan inline link must become a GitHub blob URL")
	assert.Contains(t, body,
		"[plan107]: https://github.com/jeduden/mdsmith/blob/main/plan/107_no-reference-style.md",
		"plan reference-style definition must become a GitHub blob URL")
	assert.NotContains(t, body, "../../../plan/",
		"no repo-relative plan/ link may remain on the published page")
}

func TestBuildWebsite_PublishesRulePages_FixtureAndSibling(t *testing.T) {
	body := buildRulePageBody(t)
	// Deep MDS rule link with anchor → site URL with anchor preserved.
	assert.Contains(t, body, "(/docs/rules/MDS020-required-structure/#index-side-output)",
		"deep rule link must rewrite to site URL with anchor preserved")
	// Fixture file links (good/, bad/) → rule's GitHub /blob/ URL.
	assert.Contains(t, body,
		"](https://github.com/jeduden/mdsmith/blob/main/internal/rules/MDS001-line-length/good/default.md)",
		"good/ fixture file link must become rule's GitHub blob URL")
	assert.Contains(t, body,
		"](https://github.com/jeduden/mdsmith/blob/main/internal/rules/MDS001-line-length/bad/x.md)",
		"bad/ fixture file link must become rule's GitHub blob URL")
	// Fixture directory link (pattern/good/) → rule's GitHub /tree/ URL.
	assert.Contains(t, body,
		"](https://github.com/jeduden/mdsmith/tree/main/internal/rules/MDS001-line-length/pattern/good/)",
		"pattern/ fixture directory link must become rule's GitHub tree URL")
	// Sibling non-MDS references (Go package, proto.md) → GitHub
	// /blob/ URL; cross-rule (../MDS021-include/) must NOT match.
	assert.Contains(t, body,
		"](https://github.com/jeduden/mdsmith/blob/main/internal/rules/linelength/rule.go)",
		"sibling Go package link must become a GitHub blob URL")
	assert.Contains(t, body,
		"](https://github.com/jeduden/mdsmith/blob/main/internal/rules/proto.md)",
		"sibling proto.md link must become a GitHub blob URL")
}

func TestBuildWebsite_NoRuleDirectoryIsNotAnError(t *testing.T) {
	parent := t.TempDir()
	src := filepath.Join(parent, "docs")
	dst := filepath.Join(t.TempDir(), "out")
	writeFile(t, filepath.Join(src, "top.md"), "top body\n")

	require.NoError(t, NewWithDeps(osFS{}, &recordingRunner{}).BuildWebsite(src, dst, false))

	_, err := os.Stat(filepath.Join(dst, "rules"))
	assert.True(t, os.IsNotExist(err), "no rule index -> no Rules section, no error")
}

// TestBuildWebsite_RuleIndexErrorPropagates covers the
// `if err := t.syncRuleIndex(...); err != nil { return err }`
// branch in BuildWebsite: SyncDocs succeeds (the first WriteFile,
// for top.md) and syncRuleIndex fails on its _index.md write (the
// second WriteFile), so BuildWebsite must surface that error.
func TestBuildWebsite_RuleIndexErrorPropagates(t *testing.T) {
	parent := t.TempDir()
	src := filepath.Join(parent, "docs")
	dst := filepath.Join(t.TempDir(), "out")
	writeFile(t, filepath.Join(src, "top.md"), "top body\n")
	ruleIndexAt(t, parent)
	ff := newFakeFS()
	ff.failOnWriteFileCall = 2 // 1 = docs top.md, 2 = rules/_index.md

	err := NewWithDeps(ff, &recordingRunner{}).BuildWebsite(src, dst, false)

	require.Error(t, err)
	assert.ErrorIs(t, err, errInjected)
	assert.Contains(t, err.Error(), "write rule index")
}

func TestSyncRulePages_ReadDirErrorWraps(t *testing.T) {
	rulesDir := t.TempDir()
	ff := newFakeFS()
	ff.failOnReadDirCall = 1 // errInjected, not fs.ErrNotExist

	err := NewWithFS(ff).syncRulePages(rulesDir, t.TempDir())

	require.Error(t, err)
	assert.ErrorIs(t, err, errInjected)
	assert.Contains(t, err.Error(), "read rule dir")
}

func TestSyncRulePages_SkipsNonMDSDirs(t *testing.T) {
	rulesDir := t.TempDir()
	dst := filepath.Join(t.TempDir(), "out")
	// A helper directory that must not be copied as a rule page.
	writeFile(t, filepath.Join(rulesDir, "testdata", "README.md"), "private\n")
	writeFile(t, filepath.Join(rulesDir, "MDS001-line-length", "README.md"), ruleReadmeFixture)

	require.NoError(t, NewWithFS(osFS{}).syncRulePages(rulesDir, dst))

	_, err := os.Stat(filepath.Join(dst, "rules", "testdata"))
	assert.True(t, os.IsNotExist(err), "non-MDS directory must not be published")
	_, err = os.Stat(filepath.Join(dst, "rules", "MDS001-line-length", "index.md"))
	assert.NoError(t, err, "MDS rule page must be published")
}

func TestSyncRulePages_MissingReadmeIsSkipped(t *testing.T) {
	rulesDir := t.TempDir()
	dst := filepath.Join(t.TempDir(), "out")
	// Rule dir with no README.md (partially authored rule).
	require.NoError(t, os.MkdirAll(filepath.Join(rulesDir, "MDS099-wip"), 0o755))

	require.NoError(t, NewWithFS(osFS{}).syncRulePages(rulesDir, dst))

	_, err := os.Stat(filepath.Join(dst, "rules", "MDS099-wip"))
	assert.True(t, os.IsNotExist(err), "rule dir with no README must produce no page")
}

func TestSyncRulePages_NoRulesDirIsNoop(t *testing.T) {
	dst := filepath.Join(t.TempDir(), "out")

	require.NoError(t, NewWithFS(osFS{}).syncRulePages("/does/not/exist", dst))
}

func TestSyncRulePages_ReadmeReadErrorWraps(t *testing.T) {
	rulesDir := t.TempDir()
	writeFile(t, filepath.Join(rulesDir, "MDS001-line-length", "README.md"), ruleReadmeFixture)
	ff := newFakeFS()
	// ReadDir succeeds; first ReadFile (the README) fails.
	ff.failOnReadFileCall = 1

	err := NewWithFS(ff).syncRulePages(rulesDir, t.TempDir())

	require.Error(t, err)
	assert.ErrorIs(t, err, errInjected)
	assert.Contains(t, err.Error(), "read rule README")
}

func TestSyncRulePages_MkdirErrorWraps(t *testing.T) {
	rulesDir := t.TempDir()
	writeFile(t, filepath.Join(rulesDir, "MDS001-line-length", "README.md"), ruleReadmeFixture)
	ff := newFakeFS()
	// ReadDir succeeds; ReadFile succeeds; MkdirAll for the rule dst fails.
	ff.failOnMkdirAllCall = 1

	err := NewWithFS(ff).syncRulePages(rulesDir, t.TempDir())

	require.Error(t, err)
	assert.ErrorIs(t, err, errInjected)
	assert.Contains(t, err.Error(), "mkdir")
}

func TestSyncRulePages_WriteErrorWraps(t *testing.T) {
	rulesDir := t.TempDir()
	writeFile(t, filepath.Join(rulesDir, "MDS001-line-length", "README.md"), ruleReadmeFixture)
	ff := newFakeFS()
	// ReadDir + ReadFile + MkdirAll succeed; WriteFile fails.
	ff.failOnWriteFileCall = 1

	err := NewWithFS(ff).syncRulePages(rulesDir, t.TempDir())

	require.Error(t, err)
	assert.ErrorIs(t, err, errInjected)
	assert.Contains(t, err.Error(), "write rule page")
}

func TestInjectFMField_AddsFieldToExistingFrontMatter(t *testing.T) {
	input := []byte("---\ntitle: Foo\n---\nbody\n")
	got := injectFMField(input, "cascade:\n  type: rule")
	assert.Equal(t, "---\ntitle: Foo\ncascade:\n  type: rule\n---\nbody\n", string(got))
}

func TestInjectFMField_CreatesFrontMatterWhenAbsent(t *testing.T) {
	input := []byte("body only\n")
	got := injectFMField(input, "type: rule")
	assert.Equal(t, "---\ntype: rule\n---\nbody only\n", string(got))
}

func TestInjectFMField_MalformedFrontMatterReturnedUnchanged(t *testing.T) {
	input := []byte("---\nno close\nbody\n")
	got := injectFMField(input, "type: rule")
	assert.Equal(t, string(input), string(got))
}

func TestSyncRuleIndex_ReadErrorWraps(t *testing.T) {
	parent := t.TempDir()
	ruleIndexAt(t, parent) // ReadDir finds a real (non-symlink) index.md
	ff := newFakeFS()
	ff.failOnReadFileCall = 1 // errInjected, not fs.ErrNotExist

	err := NewWithFS(ff).syncRuleIndex(filepath.Join(parent, "internal", "rules"), t.TempDir())

	require.Error(t, err)
	assert.ErrorIs(t, err, errInjected)
	assert.Contains(t, err.Error(), "read rule index")
}

func TestSyncRuleIndex_ReadDirErrorWraps(t *testing.T) {
	parent := t.TempDir()
	ruleIndexAt(t, parent)
	ff := newFakeFS()
	ff.failOnReadDirCall = 1 // not fs.ErrNotExist

	err := NewWithFS(ff).syncRuleIndex(filepath.Join(parent, "internal", "rules"), t.TempDir())

	require.Error(t, err)
	assert.ErrorIs(t, err, errInjected)
	assert.Contains(t, err.Error(), "read rule dir")
}

func TestSyncRuleIndex_SymlinkIndexSkipped(t *testing.T) {
	parent := t.TempDir()
	rulesDir := filepath.Join(parent, "internal", "rules")
	require.NoError(t, os.MkdirAll(rulesDir, 0o755))
	// A symlink planted at index.md must not be followed: the
	// link target's bytes would otherwise be published into the
	// Hugo tree.
	secret := filepath.Join(parent, "secret.md")
	writeFile(t, secret, "PRIVATE RUNNER FILE\n")
	require.NoError(t, os.Symlink(secret, filepath.Join(rulesDir, "index.md")))
	dst := filepath.Join(t.TempDir(), "out")

	require.NoError(t, NewWithFS(osFS{}).syncRuleIndex(rulesDir, dst))

	_, err := os.Stat(filepath.Join(dst, "rules"))
	assert.True(t, os.IsNotExist(err), "symlinked index.md -> no Rules section")
}

func TestSyncRuleIndex_NoIndexFileIsNoop(t *testing.T) {
	rulesDir := t.TempDir() // exists, but holds no index.md
	// A decoy sibling exercises the non-index loop branch.
	writeFile(t, filepath.Join(rulesDir, "proto.md"), "schema\n")
	dst := filepath.Join(t.TempDir(), "out")

	require.NoError(t, NewWithFS(osFS{}).syncRuleIndex(rulesDir, dst))

	_, err := os.Stat(filepath.Join(dst, "rules"))
	assert.True(t, os.IsNotExist(err))
}

func TestSyncRuleIndex_MkdirErrorWraps(t *testing.T) {
	parent := t.TempDir()
	ruleIndexAt(t, parent)
	ff := newFakeFS()
	ff.failOnMkdirAllCall = 1 // ReadFile succeeds, the rules-dir mkdir fails

	err := NewWithFS(ff).syncRuleIndex(filepath.Join(parent, "internal", "rules"), t.TempDir())

	require.Error(t, err)
	assert.ErrorIs(t, err, errInjected)
	assert.Contains(t, err.Error(), "mkdir")
}

func TestSyncRuleIndex_WriteErrorWraps(t *testing.T) {
	parent := t.TempDir()
	ruleIndexAt(t, parent)
	ff := newFakeFS()
	ff.failOnWriteFileCall = 1 // ReadFile + MkdirAll succeed, the page write fails

	err := NewWithFS(ff).syncRuleIndex(filepath.Join(parent, "internal", "rules"), t.TempDir())

	require.Error(t, err)
	assert.ErrorIs(t, err, errInjected)
	assert.Contains(t, err.Error(), "write rule index")
}

func TestBuildWebsite_RunsFixThenSync(t *testing.T) {
	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "out")
	writeFile(t, filepath.Join(src, "top.md"), "top body\n")
	rec := &recordingRunner{}

	require.NoError(t, NewWithDeps(osFS{}, rec).BuildWebsite(src, dst, true))

	require.Len(t, rec.calls, 1)
	assert.Equal(t, "go", rec.calls[0].name)
	assert.Equal(t, []string{"run", "./cmd/mdsmith", "fix", src}, rec.calls[0].args)
	assertFile(t, filepath.Join(dst, "top.md"), "top body\n")
}

func TestBuildWebsite_NoFixSkipsRunner(t *testing.T) {
	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "out")
	writeFile(t, filepath.Join(src, "top.md"), "top body\n")
	rec := &recordingRunner{}

	require.NoError(t, NewWithDeps(osFS{}, rec).BuildWebsite(src, dst, false))

	assert.Empty(t, rec.calls, "no-fix must not invoke the runner")
	assertFile(t, filepath.Join(dst, "top.md"), "top body\n")
}

func TestBuildWebsite_FixFailureWraps(t *testing.T) {
	src := t.TempDir()
	writeFile(t, filepath.Join(src, "top.md"), "top body\n")

	err := NewWithDeps(osFS{}, &fakeRunner{failOnCall: 1}).
		BuildWebsite(src, filepath.Join(t.TempDir(), "out"), true)

	require.Error(t, err)
	assert.ErrorIs(t, err, errInjected)
	assert.Contains(t, err.Error(), "mdsmith fix")
}

func TestBuildWebsite_SyncErrorSurfacedNotDoubleWrapped(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "x.md"), "x\n")

	// recordingRunner succeeds on fix; src==dst trips the
	// SyncDocs overlap guard. BuildWebsite must surface that
	// error unwrapped — SyncDocs already contextualizes it,
	// so there must be no duplicated prefix.
	err := NewWithDeps(osFS{}, &recordingRunner{}).BuildWebsite(dir, dir, true)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "same path")
	assert.NotContains(t, err.Error(), "sync ", "no redundant build-website wrap")
}

// TestBuildWebsite_SyncErrorNotDoubleWrapped is the regression
// for the duplicated `sync a -> b: sync a -> b:` prefix:
// SyncDocs already wraps the syncDocsDir failure with the
// `sync <src> -> <dst>:` prefix, so BuildWebsite must not add
// its own — the prefix must appear exactly once.
func TestBuildWebsite_SyncErrorNotDoubleWrapped(t *testing.T) {
	src := t.TempDir()
	writeFile(t, filepath.Join(src, "x.md"), "x\n")
	ff := newFakeFS()
	ff.failOnReadDirCall = 1

	err := NewWithDeps(ff, &recordingRunner{}).
		BuildWebsite(src, filepath.Join(t.TempDir(), "out"), false)

	require.Error(t, err)
	assert.ErrorIs(t, err, errInjected)
	assert.Contains(t, err.Error(), "read dir")
	assert.Equal(t, 1, strings.Count(err.Error(), "sync "),
		"the sync prefix must appear exactly once")
}
