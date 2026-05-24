package release

import (
	"os"
	"path/filepath"
	"regexp"
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
	// deploys get "/mdsmith/rules/…" not "/mdsmith//rules/…".
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
	assert.Contains(t, body, "](/background/concepts/placeholder-grammar/)",
		"docs link must become site-absolute path (no .md extension)")
	assert.Contains(t, body, "](/guides/schemas/#section-content)",
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
	assert.Contains(t, body, "(/rules/mds020-required-structure/#index-side-output)",
		"deep rule link must rewrite to lowercased site URL with anchor preserved")
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

// --- G6: titled-link rewrite (plan/173) -------------------------
//
// Every inline rewrite regex used to stop the path capture at the
// first whitespace and then require a literal `)`, so a Markdown
// link title (`[x](../y.md "t")`) made the close fail and the
// titled link shipped verbatim as a dead repo-relative path. The
// `$`-anchored rule reference-def had the same gap. These tests
// pin the fix: the path is rewritten and the title re-emitted.

func TestRewriteRuleLinks_TitledNonPublishedInline(t *testing.T) {
	got := string(rewriteRuleLinks(
		[]byte(`See [x](../../plan/107_no-reference-style.md "Plan 107").` + "\n")))
	assert.Contains(t, got,
		`[x](https://github.com/jeduden/mdsmith/blob/main/plan/107_no-reference-style.md "Plan 107")`,
		"titled non-published inline link must be rewritten with its title preserved")
	assert.NotContains(t, got, "../../plan/",
		"no repo-relative path may survive the rewrite")
}

func TestRewriteRuleLinks_TitledNonPublishedRefDef(t *testing.T) {
	got := string(rewriteRuleLinks(
		[]byte(`[p]: ../../plan/107_no-reference-style.md "Plan 107"` + "\n")))
	assert.Contains(t, got,
		`[p]: https://github.com/jeduden/mdsmith/blob/main/plan/107_no-reference-style.md "Plan 107"`,
		"titled non-published reference definition must be rewritten with its title preserved")
}

func TestRewriteRuleLinks_TitledRuleInlineAndRefDef(t *testing.T) {
	in := "Deep: [MDS020](../../../internal/rules/" +
		"MDS020-required-structure/README.md \"Req\").\n\n" +
		"[r]: ../../internal/rules/MDS020-required-structure/README.md \"Req\"\n"
	got := string(rewriteRuleLinks([]byte(in)))
	assert.Contains(t, got, `[MDS020](/rules/mds020-required-structure/ "Req")`,
		"titled deep rule link must rewrite to the site URL with its title kept")
	assert.Contains(t, got, `[r]: /rules/mds020-required-structure/ "Req"`,
		"titled rule reference-def (the $-anchored regex) must keep its title")
	assert.NotContains(t, got, "internal/rules/MDS020",
		"no repo-relative rule path may survive the rewrite")
}

func TestRewriteRuleLinks_TitledIndexMdLink(t *testing.T) {
	got := string(rewriteRuleLinks(
		[]byte(`See [d](development/index.md "Dev").` + "\n")))
	assert.Contains(t, got, `[d](development/ "Dev")`,
		"titled index.md link must drop the filename and keep its title")
}

func TestTransformRulePage_TitledReadmeFixtureAndSibling(t *testing.T) {
	in := "Sibling: [MDS021](../MDS021-include/README.md \"Inc\").\n" +
		"Fixture: [good](good/default.md \"Good\").\n" +
		"Pkg: [pkg](../linelength/rule.go \"Pkg\").\n"
	got := string(transformRulePage([]byte(in), "MDS001-line-length"))
	assert.Contains(t, got, `[MDS021](../MDS021-include/ "Inc")`,
		"titled sibling-rule README link must drop README.md and keep its title")
	assert.Contains(t, got,
		`[good](https://github.com/jeduden/mdsmith/blob/main/internal/rules/`+
			`MDS001-line-length/good/default.md "Good")`,
		"titled fixture link must become the GitHub blob URL with its title kept")
	assert.Contains(t, got,
		`[pkg](https://github.com/jeduden/mdsmith/blob/main/internal/rules/`+
			`linelength/rule.go "Pkg")`,
		"titled sibling non-MDS link must become the GitHub blob URL with its title kept")
	assert.NotContains(t, got, "README.md",
		"no unpublished README.md target may survive")
}

func TestRewriteRuleLinks_TitledLinkInCodeUntouched(t *testing.T) {
	in := "Inline `[x](../../plan/a.md \"t\")` span.\n\n" +
		"```md\n[y](../../plan/b.md \"t\")\n```\n"
	got := string(rewriteRuleLinks([]byte(in)))
	assert.Contains(t, got, "`[x](../../plan/a.md \"t\")`",
		"a titled link inside an inline code span must stay verbatim")
	assert.Contains(t, got, "[y](../../plan/b.md \"t\")",
		"a titled link inside a fenced block must stay verbatim")
}

func TestRewriteRuleLinks_UntitledStillRewrites(t *testing.T) {
	got := string(rewriteRuleLinks(
		[]byte(`See [x](../../plan/107_no-reference-style.md).` + "\n")))
	assert.Contains(t, got,
		`[x](https://github.com/jeduden/mdsmith/blob/main/plan/107_no-reference-style.md)`,
		"an untitled link must still rewrite exactly as before")
}

// A link that carries BOTH a `#fragment` and a title is the case
// the anchor-capture tightening (`#[^)\s]*`, not `#[^)]*`) exists
// for: the anchor must stop at the space so linkTitleTail keeps
// the title in its own group and the rewritten destination has no
// embedded space. These pin that anchor and title stay cleanly
// separated across every regex that has both.

func TestRewriteRuleLinks_AnchoredTitledRuleInlineAndRefDef(t *testing.T) {
	in := "Deep: [MDS020](../../../internal/rules/" +
		"MDS020-required-structure/README.md#sec \"Req\").\n\n" +
		"[r]: ../../internal/rules/MDS020-required-structure/README.md#sec \"Req\"\n"
	got := string(rewriteRuleLinks([]byte(in)))
	assert.Contains(t, got, `[MDS020](/rules/mds020-required-structure/#sec "Req")`,
		"anchored+titled rule link must keep both #fragment and title, no embedded space")
	assert.Contains(t, got, `[r]: /rules/mds020-required-structure/#sec "Req"`,
		"anchored+titled rule reference-def must keep both #fragment and title")
}

func TestRewriteRuleLinks_AnchoredTitledIndexMdLink(t *testing.T) {
	got := string(rewriteRuleLinks(
		[]byte(`See [d](development/index.md#x "Dev").` + "\n")))
	assert.Contains(t, got, `[d](development/#x "Dev")`,
		"anchored+titled index.md link must keep both #fragment and title")
}

// A bare `index.md` link with no parent directory is the in-tree
// sibling form: `docs/development/high-performance-go.md` linking
// back to `docs/development/index.md`. The source path is
// filesystem-valid (so the source-tree MDS027 check passes) but
// post-sync `index.md` no longer exists in the directory — Hugo
// renamed it to `_index.md` — so the synced-tree lint would fail
// on the original `(index.md…)` form. Rewriting bare `index.md`
// to `./` mirrors Hugo's own render-link hook (which collapses a
// bare `index.md` href to `./`) and lets MDS027 short-circuit
// the dot-only path as valid.
func TestRewriteRuleLinks_BareIndexMdLink(t *testing.T) {
	got := string(rewriteRuleLinks(
		[]byte(`See [a](index.md#allocation-budget) — top.` + "\n")))
	assert.Contains(t, got, `[a](./#allocation-budget)`,
		"bare index.md#anchor must drop the filename and resolve to the current directory")
	assert.NotContains(t, got, "index.md",
		"no bare index.md target may survive the rewrite")
}

func TestRewriteRuleLinks_BareIndexMdNoAnchor(t *testing.T) {
	got := string(rewriteRuleLinks([]byte(`Up: [home](index.md).` + "\n")))
	assert.Contains(t, got, `[home](./)`,
		"bare index.md with no anchor must rewrite to ./")
}

func TestRewriteRuleLinks_BareIndexMdTitled(t *testing.T) {
	got := string(rewriteRuleLinks(
		[]byte(`See [a](index.md#anchor "Title").` + "\n")))
	assert.Contains(t, got, `[a](./#anchor "Title")`,
		"bare index.md with anchor and title must rewrite to ./ while keeping both")
}

func TestTransformRulePage_AnchoredTitledReadmeLink(t *testing.T) {
	in := "Anchor: [MDS021 a](../MDS021-include/README.md#syntax \"Inc\").\n"
	got := string(transformRulePage([]byte(in), "MDS001-line-length"))
	assert.Contains(t, got, `[MDS021 a](../MDS021-include/#syntax "Inc")`,
		"anchored+titled sibling README link must keep both #fragment and title")
	assert.NotContains(t, got, "README.md",
		"no unpublished README.md target may survive")
}

// --- non-MDS relative-link rewrite ------------------------------
//
// MDS024's README references its rule_test.go sibling as a
// reference-style def (`[guard-test]: ../paragraphstructure/rule_test.go`)
// and its mdtext cousin as a 2-up def
// (`[harness]: ../../mdtext/sentence_equivalence_test.go`). Both
// shipped verbatim into the synced rule page and were flagged
// MDS027 by the deploy's mdsmith check pass. The four tests
// below pin the rewrite for the four shapes — inline vs ref-def,
// 1-up sibling vs 2-up cousin — that a rule README can author
// when pointing at unpublished Go sources or test files.

func TestTransformRulePage_SiblingNonMDSRefDef(t *testing.T) {
	in := "[guard-test]: ../paragraphstructure/rule_test.go\n"
	got := string(transformRulePage([]byte(in), "MDS024-paragraph-structure"))
	assert.Contains(t, got,
		"[guard-test]: https://github.com/jeduden/mdsmith/blob/main/"+
			"internal/rules/paragraphstructure/rule_test.go",
		"reference-style 1-up sibling non-MDS link must become a GitHub blob URL")
	assert.NotContains(t, got, "../paragraphstructure/",
		"no repo-relative sibling path may survive")
}

func TestTransformRulePage_CousinNonMDSInline(t *testing.T) {
	in := "See [harness](../../mdtext/sentence_equivalence_test.go).\n"
	got := string(transformRulePage([]byte(in), "MDS024-paragraph-structure"))
	assert.Contains(t, got,
		"[harness](https://github.com/jeduden/mdsmith/blob/main/"+
			"internal/mdtext/sentence_equivalence_test.go)",
		"inline 2-up cousin non-MDS link must become a GitHub blob URL")
	assert.NotContains(t, got, "../../mdtext/",
		"no repo-relative cousin path may survive")
}

func TestTransformRulePage_CousinNonMDSRefDef(t *testing.T) {
	in := "[harness]: ../../mdtext/sentence_equivalence_test.go\n"
	got := string(transformRulePage([]byte(in), "MDS024-paragraph-structure"))
	assert.Contains(t, got,
		"[harness]: https://github.com/jeduden/mdsmith/blob/main/"+
			"internal/mdtext/sentence_equivalence_test.go",
		"reference-style 2-up cousin non-MDS link must become a GitHub blob URL")
	assert.NotContains(t, got, "../../mdtext/",
		"no repo-relative cousin path may survive")
}

func TestTransformRulePage_TitledRelativeNonMDSKeepsTitle(t *testing.T) {
	in := "Inline: [g](../../mdtext/x.go \"H\").\n" +
		"[r]: ../../mdtext/x.go \"H\"\n" +
		"[s]: ../paragraphstructure/rule_test.go \"G\"\n"
	got := string(transformRulePage([]byte(in), "MDS024-paragraph-structure"))
	assert.Contains(t, got,
		`[g](https://github.com/jeduden/mdsmith/blob/main/internal/mdtext/x.go "H")`,
		"titled 2-up cousin inline must keep its title")
	assert.Contains(t, got,
		`[r]: https://github.com/jeduden/mdsmith/blob/main/internal/mdtext/x.go "H"`,
		"titled 2-up cousin ref-def must keep its title")
	assert.Contains(t, got,
		`[s]: https://github.com/jeduden/mdsmith/blob/main/internal/rules/`+
			`paragraphstructure/rule_test.go "G"`,
		"titled 1-up sibling ref-def must keep its title")
}

// TestRewriteRuleRelative_Depth3PlusDeclined pins that
// depth-3+ relative non-MDS links are NOT rewritten by the
// rule-page rewriter. The earlier rewriteRuleLinks pass owns
// every repo-rooted path it can name (plan/, cmd/, internal/,
// docs/, plan/, root files, …); a path outside that
// alternative list (e.g. `../../../demo/foo.go`) reaches
// rewriteRuleRelativeInline at depth 3 and must stay as
// authored, because depth 3 from internal/rules/<rule>/ is the
// repo root and the per-rule rewriter cannot guess a prefix
// that would produce a correct GitHub URL. A visible relative
// link the synced-tree lint will flag is the safe outcome —
// silently mapping three-up to a fabricated internal/<x> URL
// would be worse.
func TestRewriteRuleRelative_Depth3PlusDeclined(t *testing.T) {
	in := "Inline: [d](../../../demo/foo.go).\n[r]: ../../../demo/foo.go\n"
	got := string(transformRulePage([]byte(in), "MDS001-line-length"))
	assert.Contains(t, got, "[d](../../../demo/foo.go)",
		"depth-3+ inline link must remain unchanged")
	assert.Contains(t, got, "[r]: ../../../demo/foo.go",
		"depth-3+ ref-def must remain unchanged")
}

// TestRulePageTransforms_NoLeftoverRelativeNonMDSLinks is the
// repo-wide regression for the deploy failure: every rule
// README that links at a `(?:\.\./)+<non-MDS>` repo path must
// have that path rewritten before it lands on the published
// site, since the synced rule pages live at
// website/content/docs/rules/MDS…/ and unrewritten relative
// targets resolve to nothing under that tree.
//
// The check walks the actual internal/rules/MDS…/README.md
// files, runs transformMarkdown then transformRulePage (the
// same composition syncRulePages uses), and scans each rule's
// transformed body — outside code regions — for any leftover
// inline `\]\((?:\.\./)+[a-z._]` or reference-style
// `^\[…\]:\s+(?:\.\./)+[a-z._]` pattern. The `(?:\.\./)+`
// matches any depth (1 sibling, 2 cousin, 3+ repo-rooted)
// and the lead-in–only match catches a titled link too — the
// production regex's `\S*` would stop at the space before
// a `"title"` and the closing `\)` would never reach, leaving
// a titled leftover invisible to a path-anchored scan. The
// lowercase first-char excludes legitimate `../MDSyyy/`
// cross-rule site references that survive untouched.
func TestRulePageTransforms_NoLeftoverRelativeNonMDSLinks(t *testing.T) {
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	require.NoError(t, err)
	rulesDir := filepath.Join(repoRoot, "internal", "rules")
	entries, err := os.ReadDir(rulesDir)
	require.NoError(t, err)

	leftoverInline := regexp.MustCompile(`\]\((?:\.\./)+[a-z._][^)\n]*\)?`)
	leftoverRefDef := regexp.MustCompile(`(?m)^\[[^\]]+\]:\s+(?:\.\./)+[a-z._][^\n]*`)

	for _, e := range entries {
		if !e.IsDir() || !ruleDirName.MatchString(e.Name()) {
			continue
		}
		ruleName := e.Name()
		t.Run(ruleName, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join(rulesDir, ruleName, "README.md"))
			if os.IsNotExist(err) {
				t.Skip("rule has no README.md")
			}
			require.NoError(t, err)
			transformed := transformRulePage(transformMarkdown(data), ruleName)
			var inlineHit, refDefHit string
			applyOutsideCode(transformed, func(seg []byte) []byte {
				if loc := leftoverInline.FindIndex(seg); loc != nil && inlineHit == "" {
					inlineHit = string(seg[loc[0]:loc[1]])
				}
				if loc := leftoverRefDef.FindIndex(seg); loc != nil && refDefHit == "" {
					refDefHit = string(seg[loc[0]:loc[1]])
				}
				return seg
			})
			assert.Empty(t, inlineHit,
				"inline non-MDS relative link survived rule-page transforms")
			assert.Empty(t, refDefHit,
				"ref-def non-MDS relative link survived rule-page transforms")
		})
	}
}
