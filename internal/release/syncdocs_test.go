package release

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSyncDocs_CopiesMarkdownPreservingTree(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	writeFile(t, filepath.Join(src, "top.md"), "top body\n")
	writeFile(t, filepath.Join(src, "guides", "install.md"), "install body\n")

	require.NoError(t, SyncDocs(src, filepath.Join(dst, "out")))

	assertFile(t, filepath.Join(dst, "out", "top.md"), "top body\n")
	assertFile(t, filepath.Join(dst, "out", "guides", "install.md"), "install body\n")
}

func TestSyncDocs_DropsProtoMd(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	writeFile(t, filepath.Join(src, "proto.md"), "schema\n")
	writeFile(t, filepath.Join(src, "real.md"), "real body\n")

	require.NoError(t, SyncDocs(src, dst))

	_, err := os.Stat(filepath.Join(dst, "proto.md"))
	assert.True(t, os.IsNotExist(err), "proto.md should not be copied")
	assertFile(t, filepath.Join(dst, "real.md"), "real body\n")
}

func TestSyncDocs_RenamesIndexMdToUnderscoreIndex(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	writeFile(t, filepath.Join(src, "index.md"), "root body\n")
	writeFile(t, filepath.Join(src, "guides", "index.md"), "guides body\n")

	require.NoError(t, SyncDocs(src, dst))

	assertFile(t, filepath.Join(dst, "_index.md"), "root body\n")
	assertFile(t, filepath.Join(dst, "guides", "_index.md"), "guides body\n")
	_, err := os.Stat(filepath.Join(dst, "index.md"))
	assert.True(t, os.IsNotExist(err))
}

// TestSyncDocs_SynthesizesSectionIndex pins the fix for the
// GitHub Pages 404: a docs subdirectory with content pages but
// no index.md (e.g. docs/reference/, docs/background/) produced
// no _index.md, so Hugo rendered no section landing page and
// /docs/reference/ 404'd. SyncDocs now writes a minimal
// _index.md (front matter only, title humanized from the
// directory name) for any synced subdirectory that has content
// but neither an index.md of its own nor a sibling `<name>.md`
// overview page in the parent (the docs/-tree convention where
// `reference/cli.md` is the overview for `reference/cli/`).
func TestSyncDocs_SynthesizesSectionIndex(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	// reference/ has pages but no index.md and no parent
	// reference.md — must get a synthesized _index.md.
	writeFile(t, filepath.Join(src, "reference", "conventions.md"), "# Conventions\n\nbody\n")
	// reference/cli.md is the overview for reference/cli/, so the
	// cli/ directory must NOT get a synthesized _index.md (it
	// would collide with cli.md's URL).
	writeFile(t, filepath.Join(src, "reference", "cli.md"), "# CLI\n\nbody\n")
	writeFile(t, filepath.Join(src, "reference", "cli", "check.md"), "# check\n\nbody\n")
	// release-channels/ exercises the humanizer.
	writeFile(t, filepath.Join(src, "development", "release-channels", "npm.md"), "# npm\n\nbody\n")
	// guides/index.md must survive untouched (no stub overwrite).
	writeFile(t, filepath.Join(src, "guides", "index.md"), "---\ntitle: \"Guides\"\n---\nguides body\n")

	require.NoError(t, SyncDocs(src, dst))

	got, err := os.ReadFile(filepath.Join(dst, "reference", "_index.md"))
	require.NoError(t, err, "reference/_index.md must be synthesized")
	assert.Contains(t, string(got), `title: "Reference"`)

	_, err = os.Stat(filepath.Join(dst, "reference", "cli", "_index.md"))
	assert.True(t, os.IsNotExist(err),
		"reference/cli/ has a sibling cli.md overview — no stub")

	chans, err := os.ReadFile(filepath.Join(dst, "development", "release-channels", "_index.md"))
	require.NoError(t, err)
	assert.Contains(t, string(chans), `title: "Release Channels"`)

	assertFile(t, filepath.Join(dst, "guides", "_index.md"),
		"---\ntitle: \"Guides\"\n---\nguides body\n")
}

func TestSyncDocs_PrunesNonMarkdownNonImage(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	writeFile(t, filepath.Join(src, "doc.md"), "x\n")
	writeFile(t, filepath.Join(src, "embed.go"), "package x\n")
	writeFile(t, filepath.Join(src, "diagram.svg"), "<svg/>\n")
	writeFile(t, filepath.Join(src, "notes.txt"), "ignore me\n")

	require.NoError(t, SyncDocs(src, dst))

	assertFile(t, filepath.Join(dst, "doc.md"), "x\n")
	assertFile(t, filepath.Join(dst, "diagram.svg"), "<svg/>\n")
	for _, dropped := range []string{"embed.go", "notes.txt"} {
		_, err := os.Stat(filepath.Join(dst, dropped))
		assert.Truef(t, os.IsNotExist(err), "%s should not be copied", dropped)
	}
}

func TestSyncDocs_RemovesEmptyDirsLeftAfterPruning(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	writeFile(t, filepath.Join(src, "kept.md"), "x\n")
	// `internal-helpers` contains only non-content; the synced
	// tree should not expose a hollow directory.
	writeFile(t, filepath.Join(src, "internal-helpers", "embed.go"), "package x\n")

	require.NoError(t, SyncDocs(src, dst))

	assertFile(t, filepath.Join(dst, "kept.md"), "x\n")
	_, err := os.Stat(filepath.Join(dst, "internal-helpers"))
	assert.True(t, os.IsNotExist(err), "empty subdir should be pruned")
}

func TestSyncDocs_EscapesHugoShortcodePatterns(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	body := "Hugo uses `{{< readfile file=\"x.md\" >}}` and `{{% note %}}`.\n"
	writeFile(t, filepath.Join(src, "x.md"), body)

	require.NoError(t, SyncDocs(src, dst))

	got, err := os.ReadFile(filepath.Join(dst, "x.md"))
	require.NoError(t, err)
	assert.Contains(t, string(got), `{{</* readfile file="x.md" */>}}`)
	assert.Contains(t, string(got), `{{%/* note */%}}`)
}

// TestSyncDocs_DoesNotDoubleEscape pins a regression: source docs
// that already contain Hugo's escape syntax (because the author
// is documenting how to escape shortcodes verbatim) must not be
// re-escaped on the second pass. Without this guard, the percent
// form's [^}]* group would re-match the already-escaped body and
// produce broken nested markers like `{{%/*/* … *//*%}}`.
func TestSyncDocs_DoesNotDoubleEscape(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	body := "Escape angle: `{{</* readfile */>}}` and percent: `{{%/* note */%}}`.\n"
	writeFile(t, filepath.Join(src, "x.md"), body)

	require.NoError(t, SyncDocs(src, dst))

	got, err := os.ReadFile(filepath.Join(dst, "x.md"))
	require.NoError(t, err)
	assert.Equal(t, body, string(got),
		"already-escaped patterns must pass through untouched")
}

func TestSyncDocs_OverwritesExistingDestination(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	writeFile(t, filepath.Join(src, "fresh.md"), "fresh\n")
	writeFile(t, filepath.Join(dst, "stale.md"), "stale\n")

	require.NoError(t, SyncDocs(src, dst))

	assertFile(t, filepath.Join(dst, "fresh.md"), "fresh\n")
	_, err := os.Stat(filepath.Join(dst, "stale.md"))
	assert.True(t, os.IsNotExist(err), "stale destination files should be cleared")
}

func TestSyncDocs_MissingSourceFails(t *testing.T) {
	err := SyncDocs(filepath.Join(t.TempDir(), "does-not-exist"), t.TempDir())
	assert.Error(t, err)
}

// TestSyncDocs_RefusesSameSrcAndDst pins the safety check that
// blocks SyncDocs from running when caller passes overlapping
// paths. Without the guard, the leading RemoveAll(dstDir) would
// wipe the very tree we're about to read from.
func TestSyncDocs_RefusesSameSrcAndDst(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "x.md"), "x\n")
	err := SyncDocs(dir, dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "same path")

	// Source survived: the guard fired before the RemoveAll.
	body, readErr := os.ReadFile(filepath.Join(dir, "x.md"))
	require.NoError(t, readErr)
	assert.Equal(t, "x\n", string(body))
}

func TestSyncDocs_RefusesDstInsideSrc(t *testing.T) {
	src := t.TempDir()
	writeFile(t, filepath.Join(src, "x.md"), "x\n")
	err := SyncDocs(src, filepath.Join(src, "out"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "inside")
	_, readErr := os.Stat(filepath.Join(src, "x.md"))
	require.NoError(t, readErr, "source must survive the rejected call")
}

func TestSyncDocs_RefusesSrcInsideDst(t *testing.T) {
	dst := t.TempDir()
	src := filepath.Join(dst, "inner")
	require.NoError(t, os.MkdirAll(src, 0o755))
	writeFile(t, filepath.Join(src, "x.md"), "x\n")
	err := SyncDocs(src, dst)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "inside")
	_, readErr := os.Stat(filepath.Join(src, "x.md"))
	require.NoError(t, readErr, "source must survive the rejected call")
}

// TestSyncDocs_StatNonNotExistWrapsError covers the
// non-ErrNotExist branch of the Stat error handler. The
// fakeFS-injected error is not fs.ErrNotExist, so SyncDocs must
// surface it through %w rather than collapsing it to the
// "source not found" message.
func TestSyncDocs_StatNonNotExistWrapsError(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	writeFile(t, filepath.Join(src, "x.md"), "x\n")
	ff := newFakeFS()
	ff.failOnStatCall = 1

	err := NewWithFS(ff).SyncDocs(src, dst)
	require.Error(t, err)
	assert.ErrorIs(t, err, errInjected)
	assert.Contains(t, err.Error(), "stat ")
	assert.NotContains(t, err.Error(), "source not found")
}

// TestSyncDocs_AbsResolveErrorBubblesUp covers the absPath()
// error branches in checkSyncDocsPaths. filepath.Abs only fails
// when os.Getwd does, which is unreachable from a test process —
// the package-level absPath seam (shared with BuildWheels) lets
// us drive both branches deterministically.
func TestSyncDocs_AbsResolveErrorBubblesUp(t *testing.T) {
	orig := absPath
	t.Cleanup(func() { absPath = orig })

	// Branch 1: src resolve fails.
	absPath = func(string) (string, error) { return "", errInjected }
	err := SyncDocs("any-src", "any-dst")
	require.Error(t, err)
	assert.ErrorIs(t, err, errInjected)
	assert.Contains(t, err.Error(), "resolve src")

	// Branch 2: src succeeds, dst fails.
	calls := 0
	absPath = func(p string) (string, error) {
		calls++
		if calls == 2 {
			return "", errInjected
		}
		return orig(p)
	}
	err = SyncDocs(t.TempDir(), "any-dst")
	require.Error(t, err)
	assert.ErrorIs(t, err, errInjected)
	assert.Contains(t, err.Error(), "resolve dst")
}

func TestSyncDocs_DestRemoveAllErrorPropagates(t *testing.T) {
	src := t.TempDir()
	writeFile(t, filepath.Join(src, "x.md"), "x\n")
	ff := newFakeFS()
	ff.failOnRemoveAllCall = 1
	err := NewWithFS(ff).SyncDocs(src, t.TempDir())
	require.Error(t, err)
	assert.ErrorIs(t, err, errInjected)
}

func TestSyncDocs_DestMkdirAllErrorPropagates(t *testing.T) {
	src := t.TempDir()
	writeFile(t, filepath.Join(src, "x.md"), "x\n")
	ff := newFakeFS()
	ff.failOnMkdirAllCall = 1
	err := NewWithFS(ff).SyncDocs(src, t.TempDir())
	require.Error(t, err)
	assert.ErrorIs(t, err, errInjected)
}

func TestSyncDocs_ReadDirErrorPropagates(t *testing.T) {
	src := t.TempDir()
	writeFile(t, filepath.Join(src, "x.md"), "x\n")
	ff := newFakeFS()
	ff.failOnReadDirCall = 1
	err := NewWithFS(ff).SyncDocs(src, t.TempDir())
	require.Error(t, err)
	assert.ErrorIs(t, err, errInjected)
}

func TestSyncDocs_ReadFileErrorPropagates(t *testing.T) {
	src := t.TempDir()
	writeFile(t, filepath.Join(src, "x.md"), "x\n")
	ff := newFakeFS()
	ff.failOnReadFileCall = 1
	err := NewWithFS(ff).SyncDocs(src, t.TempDir())
	require.Error(t, err)
	assert.ErrorIs(t, err, errInjected)
}

func TestSyncDocs_WriteFileErrorPropagates(t *testing.T) {
	src := t.TempDir()
	writeFile(t, filepath.Join(src, "x.md"), "x\n")
	ff := newFakeFS()
	ff.failOnWriteFileCall = 1
	err := NewWithFS(ff).SyncDocs(src, t.TempDir())
	require.Error(t, err)
	assert.ErrorIs(t, err, errInjected)
}

func TestSyncDocs_ChildMkdirErrorPropagates(t *testing.T) {
	src := t.TempDir()
	writeFile(t, filepath.Join(src, "sub", "x.md"), "x\n")
	ff := newFakeFS()
	// MkdirAll #1 = SyncDocs's own dest mkdir. #2 = child subdir.
	ff.failOnMkdirAllCall = 2
	err := NewWithFS(ff).SyncDocs(src, t.TempDir())
	require.Error(t, err)
	assert.ErrorIs(t, err, errInjected)
}

func TestSyncDocs_RecursiveFailurePropagates(t *testing.T) {
	src := t.TempDir()
	writeFile(t, filepath.Join(src, "a.md"), "x\n")
	writeFile(t, filepath.Join(src, "sub", "b.md"), "y\n")
	ff := newFakeFS()
	// ReadFile #1 = a.md (succeeds). The recursive syncDocsDir
	// call hits ReadFile #2 on sub/b.md, which fails. Covers the
	// parent loop's "if err != nil { return ... }" arm after the
	// recursive call.
	ff.failOnReadFileCall = 2
	err := NewWithFS(ff).SyncDocs(src, t.TempDir())
	require.Error(t, err)
	assert.ErrorIs(t, err, errInjected)
}

func TestSyncDocs_EmptySubdirRemoveAllErrorPropagates(t *testing.T) {
	src := t.TempDir()
	writeFile(t, filepath.Join(src, "kept.md"), "x\n")
	// `pruned/` contains only an excluded extension, so the
	// recursive call ends with !wrote and tries RemoveAll on the
	// freshly-created empty child. RemoveAll #1 = SyncDocs's
	// initial dest wipe (passes); #2 = the empty-child cleanup
	// (fails).
	writeFile(t, filepath.Join(src, "pruned", "embed.go"), "package x\n")
	ff := newFakeFS()
	ff.failOnRemoveAllCall = 2
	err := NewWithFS(ff).SyncDocs(src, t.TempDir())
	require.Error(t, err)
	assert.ErrorIs(t, err, errInjected)
}

// TestSyncDocs_SynthesizeStatErrorPropagates covers the
// non-ErrNotExist branch of synthesizeSectionIndex's _index.md
// existence probe. Stat call #1 is SyncDocs's own srcDir stat;
// the sub/ directory's content writes nothing else through
// Stat until the synthesize probe, so failing Stat #2 is the
// _index.md check, which must surface (not be collapsed to a
// "skip — already exists" no-op).
func TestSyncDocs_SynthesizeStatErrorPropagates(t *testing.T) {
	src := t.TempDir()
	writeFile(t, filepath.Join(src, "sub", "x.md"), "x\n")
	ff := newFakeFS()
	ff.failOnStatCall = 2
	err := NewWithFS(ff).SyncDocs(src, t.TempDir())
	require.Error(t, err)
	assert.ErrorIs(t, err, errInjected)
	assert.Contains(t, err.Error(), "stat ")
}

// TestSyncDocs_SynthesizeWriteErrorPropagates covers both the
// stub-write failure inside synthesizeSectionIndex and the
// `return true, err` arm in syncDocsSubdir that surfaces it.
// WriteFile #1 is sub/x.md (succeeds); #2 is the synthesized
// sub/_index.md, which fails.
func TestSyncDocs_SynthesizeWriteErrorPropagates(t *testing.T) {
	src := t.TempDir()
	writeFile(t, filepath.Join(src, "sub", "x.md"), "x\n")
	ff := newFakeFS()
	ff.failOnWriteFileCall = 2
	err := NewWithFS(ff).SyncDocs(src, t.TempDir())
	require.Error(t, err)
	assert.ErrorIs(t, err, errInjected)
}

// TestHumanizeDirName pins rune-aware capitalization: a
// multibyte leading rune must be upper-cased whole, not have
// its first byte sliced (which would emit invalid UTF-8 into
// the synthesized front-matter title).
func TestHumanizeDirName(t *testing.T) {
	assert.Equal(t, "Release Channels", humanizeDirName("release-channels"))
	assert.Equal(t, "Reference", humanizeDirName("reference"))
	assert.Equal(t, "Über Docs", humanizeDirName("über_docs"))
	assert.Equal(t, "Éclair", humanizeDirName("éclair"))
}

// TestSyncDocs_SynthesizedTitleEscaping pins that a directory
// name containing YAML metacharacters (`"` and `\`) is escaped
// in the synthesized _index.md front matter — backslash first,
// matching mergeFMTitle — so the output is valid YAML.
func TestSyncDocs_SynthesizedTitleEscaping(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	dirName := `we"ird\name`
	writeFile(t, filepath.Join(src, dirName, "page.md"), "# Page\n\nbody\n")

	require.NoError(t, SyncDocs(src, dst))

	got, err := os.ReadFile(filepath.Join(dst, dirName, "_index.md"))
	require.NoError(t, err)
	assert.Contains(t, string(got), `title: "We\"ird\\name"`)
}

// TestIsUnder_HandlesFilesystemRoot is the regression for the
// RemoveAll("/") hazard: the old HasPrefix(child, parent+sep)
// test built "//" for a root parent, so isUnder("/a/b", "/")
// wrongly returned false and checkSyncDocsPaths would let
// SyncDocs wipe a root destination.
func TestIsUnder_HandlesFilesystemRoot(t *testing.T) {
	sep := string(filepath.Separator)
	root := sep
	assert.True(t, isUnder(sep+"repo"+sep+"docs", root),
		"a path must be detected as under the filesystem root")
	assert.True(t, isUnder(sep+"a"+sep+"b"+sep+"c", sep+"a"+sep+"b"))
	assert.False(t, isUnder(sep+"a", sep+"a"), "isUnder(p, p) is false")
	assert.False(t, isUnder(sep+"a", sep+"a"+sep+"b"),
		"parent is not under its own child")
	assert.False(t, isUnder(sep+"tmp"+sep+"foobar", sep+"tmp"+sep+"foo"),
		"sibling sharing a name prefix is not nested")
	// filepath.Rel errors when one side is absolute and the
	// other relative; isUnder must treat that as "not under".
	assert.False(t, isUnder("relative/path", sep+"abs"),
		"a Rel() error means not-under, never a false positive")
}

// TestBuildWebsitePackageDelegator covers the package-level
// BuildWebsite wrapper (the New()-backed entry the CLI calls),
// distinct from the Toolkit method the dependency-injected
// tests exercise.
func TestBuildWebsitePackageDelegator(t *testing.T) {
	src := t.TempDir()
	writeFile(t, filepath.Join(src, "x.md"), "plain body, no heading\n")
	dst := filepath.Join(t.TempDir(), "out")

	require.NoError(t, BuildWebsite(src, dst, false))
	assertFile(t, filepath.Join(dst, "x.md"), "plain body, no heading\n")
}

func TestSyncDocs_SkipsSymlinks(t *testing.T) {
	src := t.TempDir()
	outside := filepath.Join(t.TempDir(), "secret.md")
	writeFile(t, outside, "leaked body\n")
	writeFile(t, filepath.Join(src, "real.md"), "real body\n")
	require.NoError(t, os.Symlink(outside, filepath.Join(src, "link.md")))
	require.NoError(t, os.Symlink(t.TempDir(), filepath.Join(src, "linkdir")))
	dst := filepath.Join(t.TempDir(), "out")

	require.NoError(t, SyncDocs(src, dst))

	assertFile(t, filepath.Join(dst, "real.md"), "real body\n")
	_, err := os.Lstat(filepath.Join(dst, "link.md"))
	assert.True(t, os.IsNotExist(err), "symlinked file must not be copied")
	_, err = os.Lstat(filepath.Join(dst, "linkdir"))
	assert.True(t, os.IsNotExist(err), "symlinked dir must not be copied")
}

// TestReconcileDocForHugo_TitleLift exercises the title-lift
// half of reconcileDocForHugo: a first-block H1 is promoted
// to front-matter title: and spliced out of the body.
func TestReconcileDocForHugo_TitleLift(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{
			"summary-only front matter, body H1 promoted and stripped",
			"---\nsummary: CLI commands.\n---\n# CLI Reference\n\nbody\n",
			"---\nsummary: CLI commands.\ntitle: \"CLI Reference\"\n---\nbody\n",
		},
		{
			"existing title kept, duplicate body H1 still stripped",
			"---\ntitle: Architecture principles\nsummary: s\n---\n# Architecture principles\n\nbody\n",
			"---\ntitle: Architecture principles\nsummary: s\n---\nbody\n",
		},
		{
			"backticks stripped from promoted title",
			"---\nsummary: s\n---\n# The `mdsmith` CLI\n\nbody\n",
			"---\nsummary: s\ntitle: \"The mdsmith CLI\"\n---\nbody\n",
		},
		{
			"double quotes in title are escaped",
			"---\nsummary: s\n---\n# The \"smith\" tool\n\nbody\n",
			"---\nsummary: s\ntitle: \"The \\\"smith\\\" tool\"\n---\nbody\n",
		},
		{
			"no front matter + body H1 synthesizes front matter (research/ scratch notes)",
			"# Collection Policy\n\n## Licensing\nrules\n",
			"---\ntitle: \"Collection Policy\"\n---\n## Licensing\nrules\n",
		},
		{
			"setext H1: heading text + === underline both removed",
			"---\nsummary: s\n---\nCLI Reference\n=============\n\nbody\n",
			"---\nsummary: s\ntitle: \"CLI Reference\"\n---\nbody\n",
		},
		{
			"inline markup in heading flattened to plain title",
			"---\nsummary: s\n---\n# The [mdsmith](/x) *fast* linter\n\nbody\n",
			"---\nsummary: s\ntitle: \"The mdsmith fast linter\"\n---\nbody\n",
		},
		{
			"setext underline at EOF (no trailing newline)",
			"---\nsummary: s\n---\nCLI Reference\n===",
			"---\nsummary: s\ntitle: \"CLI Reference\"\n---\n",
		},
		{
			// Regression: CommonMark allows up to 3 leading
			// spaces before an ATX '#'. The indented heading
			// must not be misread as setext (which would
			// delete the following content line).
			"indented ATX H1 keeps the next content line",
			"---\nsummary: s\n---\n  # Title\nfirst paragraph\n",
			"---\nsummary: s\ntitle: \"Title\"\n---\nfirst paragraph\n",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, string(reconcileDocForHugo([]byte(c.in))))
		})
	}
}

// TestReconcileDocForHugo_TitleNoOp pins the cases goldmark
// classifies as "first block is not a liftable H1" (and that
// carry no directive markers, so reconcileDocForHugo returns
// the input byte-for-byte): prose-first pages, level-2-first
// files, a '#' that is really fenced-code content (the old
// line-prefix regex wrongly lifted this), an empty ATX
// heading, a heading whose inline content flattens to
// nothing, and a front-matter fence that never closes.
func TestReconcileDocForHugo_TitleNoOp(t *testing.T) {
	for _, in := range []string{
		"---\ntitle: Development\nsummary: s\n---\nBuild reference, no body H1.\n",
		"---\nsummary: s\n---\njust prose, no leading heading\n",
		"---\nsummary: s\n---\n## only a level-2 heading\n",
		"plain notes, no front matter, no heading\n",
		"---\nsummary: s\n---\n```\n# not a heading, this is code\n```\n",
		"---\nsummary: s\n---\n#\n\nempty heading\n",
		"---\nsummary: s\n---\n# <!-- only an html comment, no text -->\n\nbody\n",
		"---\nsummary: s\nno closing fence so leave the file alone\n",
	} {
		assert.Equal(t, in, string(reconcileDocForHugo([]byte(in))), "must be unchanged: %q", in)
	}
}

// TestReconcileDocForHugo_StripMarkers proves the AST-based
// strip half: real top-level `<?…?>` markers (CommonMark
// type-3 HTML blocks) are removed while the same syntax
// inside a fenced code block or inline code is structurally
// distinct and must survive verbatim — so directive
// *documentation* still renders. ~~~ fences avoid backticks
// in Go strings.
func TestReconcileDocForHugo_StripMarkers(t *testing.T) {
	const fence = "~~~text\n<?catalog\nsort: path\n?>\n<?/catalog?>\n~~~\n"
	cases := []struct{ name, in, want string }{
		{
			"opener+closer removed, generated body and fenced/inline examples kept",
			"---\ntitle: \"X\"\n---\n" +
				"Intro.\n\n" +
				"<?catalog\nglob:\n  - \"docs/**/*.md\"\n?>\n" +
				"- [A](a.md)\n- [B](b.md)\n<?/catalog?>\n\n" +
				"More prose.\n\n" + fence + "\nInline `<?include?>` stays.\n",
			"---\ntitle: \"X\"\n---\n" +
				"Intro.\n\n- [A](a.md)\n- [B](b.md)\n\n" +
				"More prose.\n\n" + fence + "\nInline `<?include?>` stays.\n",
		},
		{
			"single-line <?toc?> with no front matter",
			"intro\n\n<?toc?>\n\nend\n",
			"intro\n\n\nend\n",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, string(reconcileDocForHugo([]byte(c.in))))
		})
	}
}

// TestReconcileDocForHugo_StripNoOp: with no liftable H1,
// no real markers (none, fence-only, or inline-only) and
// malformed front matter all return the input byte-for-byte.
func TestReconcileDocForHugo_StripNoOp(t *testing.T) {
	for _, in := range []string{
		"---\ntitle: x\n---\njust prose, no markers\n",
		"---\ns: 1\n---\n~~~\n<?toc?>\n<?/toc?>\n~~~\n",
		"prose with inline `<?catalog?>` only, no block marker\n",
		"---\ns: 1\nno closing fence so leave the file alone\n",
	} {
		assert.Equal(t, in, string(reconcileDocForHugo([]byte(in))),
			"must be unchanged: %q", in)
	}
}

// ghBlob / ghTree / rulesBase short-name the URL prefixes that
// would otherwise blow past the golangci-lint `lll` (line-length)
// cap in every expected-output literal below. Tests append the
// rest of the path inline.
const (
	ghBlob    = "https://github.com/jeduden/mdsmith/blob/main/"
	ghTree    = "https://github.com/jeduden/mdsmith/tree/main/"
	rulesBase = "/docs/rules/"
)

// transformCase pairs an input markdown body with the expected
// transformMarkdown output so every test below can share the
// same runTransformCases driver.
type transformCase struct {
	name string
	in   string
	want string
}

func runTransformCases(t *testing.T, cases []transformCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, string(transformMarkdown([]byte(tc.in))))
		})
	}
}

// TestTransformMarkdown_RewritesRuleLinks: docs that link into
// internal/rules/ (any `../` depth, with or without README.md,
// inline or reference-style) must come out pointing at the
// published /docs/rules/<dir>/ URL. The same link works on
// GitHub because the source tree still has internal/rules/, but
// that directory is not published on the site — without the
// rewrite the link 404s.
func TestTransformMarkdown_RewritesRuleLinks(t *testing.T) {
	runTransformCases(t, []transformCase{
		{
			name: "deep inline no readme",
			in:   "See [x](../../../../internal/rules/MDS019-catalog/).\n",
			want: "See [x](" + rulesBase + "MDS019-catalog/).\n",
		},
		{
			name: "shallow inline with readme",
			in:   "See [x](../../internal/rules/MDS020-required-structure/README.md).\n",
			want: "See [x](" + rulesBase + "MDS020-required-structure/).\n",
		},
		{
			name: "reference-style def with readme",
			in:   "[mds]: ../../../internal/rules/MDS027-cross-file-reference-integrity/README.md\n",
			want: "[mds]: " + rulesBase + "MDS027-cross-file-reference-integrity/\n",
		},
		{
			name: "reference-style def no readme",
			in:   "[mds]: ../../../../internal/rules/MDS019-catalog/\n",
			want: "[mds]: " + rulesBase + "MDS019-catalog/\n",
		},
		{
			name: "already-rewritten path is unchanged",
			in:   "See [x](" + rulesBase + "MDS019-catalog/).\n",
			want: "See [x](" + rulesBase + "MDS019-catalog/).\n",
		},
		{
			name: "deep rule link preserves anchor",
			in:   "See [x](../../../internal/rules/MDS020-required-structure/README.md#out).\n",
			want: "See [x](" + rulesBase + "MDS020-required-structure/#out).\n",
		},
	})
}

// TestTransformMarkdown_RewritesNonPublishedLinks: every
// repo-relative link to a path outside the published trees
// (plan/, cmd/, internal/ non-rules, website/, .claude/, root
// files) must rewrite to a GitHub URL. The /blob/ vs /tree/ route
// is decided by trailing slash: files go to /blob/, directories
// to /tree/. The reference-style sibling shares the rewrite logic.
func TestTransformMarkdown_RewritesNonPublishedLinks(t *testing.T) {
	runTransformCases(t, []transformCase{
		{
			name: "plan link → GitHub blob",
			in:   "See [x](../../plan/154_arch.md).\n",
			want: "See [x](" + ghBlob + "plan/154_arch.md).\n",
		},
		{
			name: "internal Go source → GitHub blob",
			in:   "See [x](../../internal/config/convention.go).\n",
			want: "See [x](" + ghBlob + "internal/config/convention.go).\n",
		},
		{
			name: "claude skill → GitHub blob",
			in:   "See [x](../../../.claude/skills/foo/SKILL.md).\n",
			want: "See [x](" + ghBlob + ".claude/skills/foo/SKILL.md).\n",
		},
		{
			name: "repo root PLAN.md → GitHub blob",
			in:   "See [x](../../PLAN.md).\n",
			want: "See [x](" + ghBlob + "PLAN.md).\n",
		},
		{
			name: "internal directory → GitHub tree",
			in:   "See [x](../../internal/lint/).\n",
			want: "See [x](" + ghTree + "internal/lint/).\n",
		},
		{
			name: "reference-style def → GitHub blob",
			in:   "[plan]: ../../plan/154_arch.md\n",
			want: "[plan]: " + ghBlob + "plan/154_arch.md\n",
		},
		{
			name: "reference-style def directory → GitHub tree",
			in:   "[lint]: ../../internal/lint/\n",
			want: "[lint]: " + ghTree + "internal/lint/\n",
		},
		{
			name: "website layout file → GitHub blob",
			in:   "See [x](../../../website/layouts/_default/_markup/render-link.html).\n",
			want: "See [x](" + ghBlob + "website/layouts/_default/_markup/render-link.html).\n",
		},
		{
			name: "website reference-style def → GitHub blob",
			in:   "[hook]: ../../../website/layouts/_default/_markup/render-link.html\n",
			want: "[hook]: " + ghBlob + "website/layouts/_default/_markup/render-link.html\n",
		},
	})
}

// TestTransformMarkdown_DropsIndexMdFilename: Hugo serves a
// section's _index.md at the directory URL (no filename), so a
// relative `<path>/index.md` link in a docs body must be
// rewritten to `<path>/` to render correctly on the site. The
// anchor passes through; a bare `index.md` with no parent
// directory is left alone because the resulting empty target
// would be ambiguous.
func TestTransformMarkdown_DropsIndexMdFilename(t *testing.T) {
	runTransformCases(t, []transformCase{
		{
			name: "sibling index.md becomes section URL",
			in:   "See [x](architecture/index.md).\n",
			want: "See [x](architecture/).\n",
		},
		{
			name: "sibling index.md with anchor",
			in:   "See [x](architecture/index.md#section).\n",
			want: "See [x](architecture/#section).\n",
		},
		{
			name: "bare index.md with no parent dir is unchanged",
			in:   "See [x](index.md).\n",
			want: "See [x](index.md).\n",
		},
	})
}

// TestTransformMarkdown_SkipsFencedExamples: fenced code blocks
// contain Markdown examples (rule READMEs demoing what a
// directive rewrites, catalog-output snippets in the directive
// guide) whose link patterns are documentation, not real
// targets. transformMarkdown's rewrites must leave those lines
// verbatim or the published docs misrepresent the tool.
func TestTransformMarkdown_SkipsFencedExamples(t *testing.T) {
	runTransformCases(t, []transformCase{
		{
			name: "backtick fence preserves rule-link example",
			in:   "```markdown\nSee [r](../../internal/rules/MDS019-catalog/).\n```\n",
			want: "```markdown\nSee [r](../../internal/rules/MDS019-catalog/).\n```\n",
		},
		{
			name: "backtick fence preserves index.md example",
			in:   "```markdown\n- [i](development/index.md)\n```\n",
			want: "```markdown\n- [i](development/index.md)\n```\n",
		},
		{
			name: "tilde fence preserves plan example",
			in:   "~~~markdown\nSee [p](../../plan/x.md).\n~~~\n",
			want: "~~~markdown\nSee [p](../../plan/x.md).\n~~~\n",
		},
		{
			name: "fence followed by real link still rewrites the real link",
			in:   "```markdown\n[r](../../plan/x.md)\n```\n\nSee [p](../../plan/y.md).\n",
			want: "```markdown\n[r](../../plan/x.md)\n```\n\nSee [p](" + ghBlob + "plan/y.md).\n",
		},
		{
			name: "inline code span preserves rule-link example",
			in:   "Sample: `[r](../../internal/rules/MDS019-catalog/)` is doc.\n",
			want: "Sample: `[r](../../internal/rules/MDS019-catalog/)` is doc.\n",
		},
		{
			name: "inline code span and real link on same line",
			in:   "Doc `[p](../../plan/x.md)` real [p](../../plan/y.md).\n",
			want: "Doc `[p](../../plan/x.md)` real [p](" + ghBlob + "plan/y.md).\n",
		},
		{
			name: "single backtick run is not a fence — rewrite continues",
			in:   "Prose with one `tick` and [p](../../plan/y.md).\n",
			want: "Prose with one `tick` and [p](" + ghBlob + "plan/y.md).\n",
		},
		{
			name: "line starting with one backtick does not open a fence",
			in:   "`code line`\n\n[p](../../plan/y.md)\n",
			want: "`code line`\n\n[p](" + ghBlob + "plan/y.md)\n",
		},
		{
			name: "closing fence with leading spaces still ends fence",
			in:   "```markdown\n[r](../../plan/a.md)\n   ```\n\nReal [p](../../plan/b.md).\n",
			want: "```markdown\n[r](../../plan/a.md)\n   ```\n\nReal [p](" + ghBlob + "plan/b.md).\n",
		},
	})
}

// TestRewriteSiblingNonPublished: a relative link to a
// same-directory or subdirectory file whose extension is not
// synced into the Hugo tree (a `.sh` script, a `.yml` config,
// `.json` data) must rewrite to a GitHub blob URL anchored at
// the doc's repo-relative directory. Markdown and image links
// (synced), site-absolute targets, external URLs, pure
// anchors, and `../`-prefixed paths (handled by the
// non-published rewrite) are left untouched.
func TestRewriteSiblingNonPublished(t *testing.T) {
	const rd = "docs/research/benchmarks"
	blob := ghBlob + rd + "/"
	cases := []struct{ name, in, want string }{
		{"same-dir script", "[`r`](run.sh)\n", "[`r`](" + blob + "run.sh)\n"},
		{"yml config", "[c](bench.mdsmith.yml)\n", "[c](" + blob + "bench.mdsmith.yml)\n"},
		{"subdir data", "[j](data/r.json)\n", "[j](" + blob + "data/r.json)\n"},
		{"anchor kept", "[x](run.sh#L3)\n", "[x](" + blob + "run.sh#L3)\n"},
		{"markdown untouched", "[f](res.md)\n", "[f](res.md)\n"},
		{"image untouched", "![d](d.svg)\n", "![d](d.svg)\n"},
		{"pure anchor untouched", "[r](#sec)\n", "[r](#sec)\n"},
		{"external url untouched", "[c](https://x.io/run.sh)\n", "[c](https://x.io/run.sh)\n"},
		{"site-absolute untouched", "[a](/docs/rules/MDS027/)\n", "[a](/docs/rules/MDS027/)\n"},
		{"parent-relative untouched", "[p](../b/run.sh)\n", "[p](../b/run.sh)\n"},
		{"code span untouched", "use `[x](run.sh)` lit\n", "use `[x](run.sh)` lit\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want,
				string(rewriteSiblingNonPublished([]byte(tc.in), rd)))
		})
	}
}

// TestSyncDocs_RewritesSiblingNonPublishedLinks pins the
// benchmarks-README fix end to end: research docs link to
// sibling `run.sh` / `bench-parity.mdsmith.yml` plumbing that
// SyncDocs prunes, so the synced page must point those links at
// GitHub instead of a dead relative path that MDS027 flags.
func TestSyncDocs_RewritesSiblingNonPublishedLinks(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "docs")
	dst := t.TempDir()
	writeFile(t, filepath.Join(src, "research", "benchmarks", "README.md"),
		"# Bench\n\nRun [`run.sh`](run.sh) with "+
			"[cfg](bench-parity.mdsmith.yml).\n")

	require.NoError(t, SyncDocs(src, dst))

	got, err := os.ReadFile(
		filepath.Join(dst, "research", "benchmarks", "README.md"))
	require.NoError(t, err)
	assert.Contains(t, string(got),
		"["+"`run.sh`"+"]("+ghBlob+"docs/research/benchmarks/run.sh)")
	assert.Contains(t, string(got),
		"[cfg]("+ghBlob+"docs/research/benchmarks/bench-parity.mdsmith.yml)")
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func assertFile(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	require.NoError(t, err, "read %s", path)
	assert.Equal(t, want, string(got))
}
