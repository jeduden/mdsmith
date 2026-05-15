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
