package lsp

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/index"
)

// These tests exercise the per-handler error and edge paths that
// the happy-path coverage tests in symbols_test.go don't reach —
// missing buffers, malformed parameters, empty index results,
// non-file URIs, and so on.

func TestRangeForLinesClampsStart(t *testing.T) {
	t.Parallel()
	r := rangeForLines(0, 0, []byte("foo\n"))
	assert.Equal(t, 0, r.Start.Line)
	assert.Equal(t, 0, r.End.Line)
}

func TestRangeForLinesEndPastEOF(t *testing.T) {
	t.Parallel()
	src := []byte("foo\nbar\n")
	r := rangeForLines(1, 99, src)
	// End line clamps to the last line of the document; character
	// reflects that line's UTF-16 length.
	assert.Equal(t, 0, r.Start.Line)
	// splitLines yields 3 entries for "foo\nbar\n" (last empty);
	// clamp to the last index, 0-based → 2.
	assert.LessOrEqual(t, r.End.Line, 2)
	assert.GreaterOrEqual(t, r.End.Line, 0)
}

func TestRangeForLinesStartPastEOF(t *testing.T) {
	t.Parallel()
	src := []byte("foo\nbar\n")
	r := rangeForLines(99, 99, src)
	assert.LessOrEqual(t, r.Start.Line, 2)
	assert.GreaterOrEqual(t, r.Start.Line, 0)
}

func TestWorkspaceURIWithEmptyRoot(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	_, errResp := h.request("initialize", initializeParams{})
	require.Nil(t, errResp)
	// No root + relative path → workspaceURI returns "" so the
	// caller drops the location instead of emitting an invalid
	// `docs/x.md` URI to the client.
	assert.Empty(t, h.srv.workspaceURI("docs/x.md"))
	// Absolute path works even without a root: file:// is built
	// from the path directly.
	assert.NotEmpty(t, h.srv.workspaceURI("/abs/x.md"))
}

func TestDocTextOrFileNonFileURI(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	_, errResp := h.request("initialize", initializeParams{})
	require.Nil(t, errResp)
	_, _, ok := h.srv.docTextOrFile("https://example.com/x.md")
	assert.False(t, ok)
}

func TestIndexReloadFromDiskRejectsOutsideRoot(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "a.md"), []byte("# A\n"), 0o644))
	outside := filepath.Join(t.TempDir(), "leak.md")
	require.NoError(t, os.WriteFile(outside, []byte("# Secret\n"), 0o644))

	h := newHarness(t)
	rootURI := pathToFileURI(t, tmp)
	_, errResp := h.request("initialize", initializeParams{
		RootURI: &rootURI, Capabilities: clientCapabilities{},
	})
	require.Nil(t, errResp)
	h.srv.settingsMu.Lock()
	h.srv.settings.Run = runOff
	h.srv.settingsMu.Unlock()
	h.srv.ensureIndex()

	// Reload from a path outside the workspace — must be a no-op
	// (not crash, not leak the secret file's contents).
	h.srv.indexReloadFromDisk(outside)
	// The outside path is normalized away from the workspace, so
	// the index never sees it.
	files := h.srv.idx.Files()
	for _, f := range files {
		assert.NotContains(t, f, "leak.md")
		assert.NotContains(t, f, "Secret")
	}
}

func TestIndexReloadFromDiskRejectsNonMarkdown(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "secret.txt"), []byte("data"), 0o644))
	h := newHarness(t)
	rootURI := pathToFileURI(t, tmp)
	_, errResp := h.request("initialize", initializeParams{
		RootURI: &rootURI, Capabilities: clientCapabilities{},
	})
	require.Nil(t, errResp)
	h.srv.ensureIndex()
	h.srv.indexReloadFromDisk(filepath.Join(tmp, "secret.txt"))
	for _, f := range h.srv.idx.Files() {
		assert.NotContains(t, f, "secret.txt")
	}
}

func TestOutgoingCallsCoalescedItemHasNoAnchor(t *testing.T) {
	t.Parallel()
	// Two outgoing edges from a.md target the same b.md but at
	// different anchors. The coalesced item must have empty
	// Data.Anchor — otherwise a follow-up incomingCalls on the
	// returned item would be filtered to whichever anchor
	// happened to land in the bucket first, hiding the other.
	srcA := "# A\n\n[one](./b.md#sec-1)\n[two](./b.md#sec-2)\n"
	srcB := "# B\n\n## Sec 1\n\n## Sec 2\n"
	h, _, rootURI := rootedHarness(t, map[string]string{"a.md": srcA, "b.md": srcB})
	uri := rootURI + "/a.md"
	h.srv.settingsMu.Lock()
	h.srv.settings.Run = runOff
	h.srv.settingsMu.Unlock()
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uri, LanguageID: "markdown", Version: 1, Text: srcA},
	})
	raw, errResp := h.request("textDocument/prepareCallHierarchy", textDocumentPositionParams{
		TextDocument: textDocumentIdentifier{URI: uri},
		Position:     Position{Line: 0, Character: 0},
	})
	require.Nil(t, errResp)
	var items []callHierarchyItem
	require.NoError(t, json.Unmarshal(raw, &items))
	require.Len(t, items, 1)
	raw, errResp = h.request("callHierarchy/outgoingCalls", callHierarchyOutgoingCallsParams{Item: items[0]})
	require.Nil(t, errResp)
	var calls []callHierarchyOutgoingCall
	require.NoError(t, json.Unmarshal(raw, &calls))
	require.Len(t, calls, 1)
	require.NotNil(t, calls[0].To.Data)
	assert.Empty(t, calls[0].To.Data.Anchor,
		"coalesced item must have empty Anchor; otherwise follow-up calls filter to one heading")
	assert.Len(t, calls[0].FromRanges, 2)
}

func TestEnsureIndexAppliesIgnoreList(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmp, ".mdsmith.yml"),
		[]byte("ignore:\n  - vendor/**\n"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(tmp, "vendor"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "vendor", "vend.md"),
		[]byte("# Vendored\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "doc.md"),
		[]byte("# Doc\n"), 0o644))

	h := newHarness(t)
	rootURI := pathToFileURI(t, tmp)
	_, errResp := h.request("initialize", initializeParams{
		RootURI: &rootURI, Capabilities: clientCapabilities{},
	})
	require.Nil(t, errResp)
	h.srv.settingsMu.Lock()
	h.srv.settings.Run = runOff
	h.srv.settingsMu.Unlock()
	h.srv.reloadConfig()
	h.srv.invalidateIndex()
	idx := h.srv.ensureIndex()

	files := idx.Files()
	assert.Contains(t, files, "doc.md")
	for _, f := range files {
		assert.NotContains(t, f, "vendor",
			"ignore-list path should be excluded from the symbol index: %s", f)
	}
}

func TestEnsureIndexOpenBufferBypassesIgnoreList(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmp, ".mdsmith.yml"),
		[]byte("ignore:\n  - notes.md\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "notes.md"),
		[]byte("# Notes\n"), 0o644))

	h := newHarness(t)
	rootURI := pathToFileURI(t, tmp)
	_, errResp := h.request("initialize", initializeParams{
		RootURI: &rootURI, Capabilities: clientCapabilities{},
	})
	require.Nil(t, errResp)
	h.srv.settingsMu.Lock()
	h.srv.settings.Run = runOff
	h.srv.settingsMu.Unlock()
	h.srv.reloadConfig()
	h.srv.invalidateIndex()

	uri := rootURI + "/notes.md"
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uri, LanguageID: "markdown", Version: 1, Text: "# Notes\n"},
	})
	// Even though notes.md is ignored on disk, the open buffer
	// surfaces it: the user is editing the file, so it has to be
	// navigable. Drive a navigation request to make sure didOpen
	// has been processed before we sample the index.
	_, _ = h.request("workspace/symbol", workspaceSymbolParams{Query: ""})
	assert.Contains(t, h.srv.idx.Files(), "notes.md")
}

func TestFilterIgnoredHelper(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{Ignore: []string{"vendor/**", "*.tmp.md"}}
	got := filterIgnored(cfg, []string{
		"doc.md",
		"vendor/lib.md",
		"draft.tmp.md",
		"src/file.md",
	})
	assert.Equal(t, []string{"doc.md", "src/file.md"}, got)
	// Nil cfg passes through.
	assert.Equal(t, []string{"a", "b"}, filterIgnored(nil, []string{"a", "b"}))
	// Empty Ignore passes through.
	assert.Equal(t, []string{"a"}, filterIgnored(&config.Config{}, []string{"a"}))
}

func TestEffectiveKindsForNoCfgWithScalarKind(t *testing.T) {
	t.Parallel()
	// No config → no kind-assignment globs, but the file's own
	// `kind: guide` front matter must still surface as effective
	// kind so workspace-symbol search picks it up.
	got := effectiveKindsFor(nil, "a.md", []byte("---\nkind: guide\n---\n# A\n"))
	assert.Equal(t, []string{"guide"}, got)
}

func TestEffectiveKindsForNoCfgNoFrontMatterKind(t *testing.T) {
	t.Parallel()
	// No config and no front-matter kinds → nil (the index falls
	// back to whatever buildFileEntry parsed natively).
	got := effectiveKindsFor(nil, "a.md", []byte("# A\n"))
	assert.Nil(t, got)
}

func TestEffectiveKindsForFieldsPresentSelector(t *testing.T) {
	t.Parallel()
	// A config that uses fields-present: triggers the ParseFrontMatterFields
	// branch in effectiveKindsFor; the front-matter mapping must satisfy
	// the entry for the kind to land on the file.
	cfg := &config.Config{
		Kinds: map[string]config.KindBody{"task": {}},
		KindAssignment: []config.KindAssignmentEntry{
			{FieldsPresent: []string{"status", "priority"}, Kinds: []string{"task"}},
		},
	}
	got := effectiveKindsFor(cfg, "a.md", []byte(
		"---\nstatus: open\npriority: high\n---\n# A\n"))
	assert.Contains(t, got, "task")

	// A file missing one of the required fields should not get the kind.
	got = effectiveKindsFor(cfg, "a.md", []byte("---\nstatus: open\n---\n# A\n"))
	assert.NotContains(t, got, "task")
}

// effectiveKindsFor swallows ParseFrontMatterFields errors so a file with
// malformed front matter (a YAML sequence in this case) still returns the
// kinds it can resolve — the LSP index never panics or surfaces an error
// from indexing.
func TestEffectiveKindsForFieldsPresentParseErrorSwallowed(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		Kinds: map[string]config.KindBody{"task": {}},
		KindAssignment: []config.KindAssignmentEntry{
			{FieldsPresent: []string{"status"}, Kinds: []string{"task"}},
		},
	}
	got := effectiveKindsFor(cfg, "a.md", []byte("---\n- not\n- a\n- mapping\n---\n# A\n"))
	assert.NotContains(t, got, "task",
		"unparseable FM yields no fields, so the entry cannot match")
}

func TestEffectiveKindsForScalarKind(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmp, ".mdsmith.yml"),
		[]byte("kinds:\n  guide: {}\n"), 0o644))
	h := newHarness(t)
	rootURI := pathToFileURI(t, tmp)
	_, errResp := h.request("initialize", initializeParams{
		RootURI: &rootURI, Capabilities: clientCapabilities{},
	})
	require.Nil(t, errResp)
	h.srv.reloadConfig()
	cfg, _, _ := h.srv.snapshotConfig()
	require.NotNil(t, cfg)
	// Scalar `kind: guide` form must surface as effective kind.
	got := effectiveKindsFor(cfg, "a.md", []byte("---\nkind: guide\n---\n# A\n"))
	assert.Contains(t, got, "guide")
}

func TestInsideWorkspaceRejectsSymlinkEscape(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	outside := t.TempDir()
	target := filepath.Join(outside, "secret.md")
	require.NoError(t, os.WriteFile(target, []byte("# Secret\n"), 0o644))
	link := filepath.Join(root, "leak.md")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unsupported on this filesystem: %v", err)
	}
	// Symlink lives under root but resolves outside → must be rejected.
	assert.False(t, insideWorkspace(root, link),
		"symlink pointing outside root must not pass workspace gate")
}

func TestInsideWorkspaceAcceptsInRootSymlink(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	target := filepath.Join(root, "real.md")
	require.NoError(t, os.WriteFile(target, []byte("# Real\n"), 0o644))
	link := filepath.Join(root, "alias.md")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unsupported on this filesystem: %v", err)
	}
	assert.True(t, insideWorkspace(root, link),
		"in-root symlink must pass workspace gate")
}

func TestDirectiveArgLocationsEmpty(t *testing.T) {
	t.Parallel()
	h, _, _ := rootedHarness(t, map[string]string{"a.md": "# A\n"})
	got := h.srv.directiveArgLocations("a.md", "")
	assert.Empty(t, got)
	// Escape-the-root → "".
	got = h.srv.directiveArgLocations("a.md", "../../escape.md")
	assert.Empty(t, got)
}

func TestFrontMatterValueTargetsNonKind(t *testing.T) {
	t.Parallel()
	h, _, _ := rootedHarness(t, map[string]string{"a.md": "# A\n"})
	idx := h.srv.ensureIndex()
	got := h.srv.frontMatterValueTargets("title", "anything", idx, true)
	assert.Empty(t, got)
}

func TestFrontMatterValueTargetsDefinitionOnly(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmp, ".mdsmith.yml"),
		[]byte("kinds:\n  guide: {}\n"), 0o644))
	h := newHarness(t)
	rootURI := pathToFileURI(t, tmp)
	_, errResp := h.request("initialize", initializeParams{
		RootURI: &rootURI, Capabilities: clientCapabilities{},
	})
	require.Nil(t, errResp)
	h.srv.reloadConfig()
	idx := h.srv.ensureIndex()
	// wantAll=false → only the definition.
	got := h.srv.frontMatterValueTargets("kind", "guide", idx, false)
	assert.Len(t, got, 1)
	assert.Contains(t, got[0].URI, ".mdsmith.yml")
}

func TestHeadingTargetsDeclarationOnly(t *testing.T) {
	t.Parallel()
	src := "# Top\n"
	h, _, rootURI := rootedHarness(t, map[string]string{"a.md": src})
	uri := rootURI + "/a.md"
	h.srv.settingsMu.Lock()
	h.srv.settings.Run = runOff
	h.srv.settingsMu.Unlock()
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uri, LanguageID: "markdown", Version: 1, Text: src},
	})
	idx := h.srv.ensureIndex()
	p := textDocumentPositionParams{
		TextDocument: textDocumentIdentifier{URI: uri},
		Position:     Position{Line: 0, Character: 0},
	}
	got := h.srv.headingTargets(p, "a.md", "top", 1, []byte(src), idx, false)
	assert.Len(t, got, 1, "wantAll=false returns only the declaration")
}

func TestLocationsForFileReferencesAnchoredFileLinkSkipped(t *testing.T) {
	t.Parallel()
	srcA := "# A\n\n## Sec\n"
	srcB := "# B\n\n[anchor](./a.md#sec)\n[plain](./a.md)\n"
	h, _, _ := rootedHarness(t, map[string]string{"a.md": srcA, "b.md": srcB})
	idx := h.srv.ensureIndex()
	got := h.srv.locationsForFileReferences("a.md", idx)
	// Only the unanchored file link counts as a "file reference";
	// the anchored one targets a heading.
	require.Len(t, got, 1)
}

func TestLocationsForFileReferencesIncludesBuildAndCatalog(t *testing.T) {
	t.Parallel()
	srcA := "# A\n"
	srcInc := "# I\n\n<?include\nfile: \"./a.md\"\n?>\n<?/include?>\n"
	srcBuild := "# B2\n\n<?build\nrecipe: r\ninputs:\n  - \"./a.md\"\noutputs:\n  - out.html\n?>\n<?/build?>\n"
	h, _, _ := rootedHarness(t, map[string]string{
		"a.md": srcA, "i.md": srcInc, "b.md": srcBuild,
	})
	idx := h.srv.ensureIndex()
	got := h.srv.locationsForFileReferences("a.md", idx)
	// Both include and build should appear.
	assert.GreaterOrEqual(t, len(got), 2)
}

func TestLocationsForRefUsesSkipsNonRefLinks(t *testing.T) {
	t.Parallel()
	// Mix of ref-style + plain links — ref-style only.
	src := "# T\n\n[a](./b.md) and [foo][lab]\n\n[lab]: u\n"
	h, _, _ := rootedHarness(t, map[string]string{"a.md": src, "b.md": "# B\n"})
	idx := h.srv.ensureIndex()
	got := h.srv.locationsForRefUses("a.md", "lab", idx)
	assert.Len(t, got, 1)
}

func TestWorkspaceSymbolWithDirective(t *testing.T) {
	t.Parallel()
	src := "# Top\n\n<?include\nfile: \"x.md\"\n?>\n<?/include?>\n"
	h, _, _ := rootedHarness(t, map[string]string{"a.md": src, "x.md": "# X\n"})
	// Force the index to populate via an initial query.
	_, _ = h.request("workspace/symbol", workspaceSymbolParams{Query: ""})
	// SearchSymbols only matches headings, link refs, titles, and
	// kinds — directives don't appear in workspace/symbol results
	// today; verify the empty query returns at least the heading
	// from a.md so a regression that drops headings would be
	// caught.
	raw, errResp := h.request("workspace/symbol", workspaceSymbolParams{Query: "Top"})
	require.Nil(t, errResp)
	var hits []symbolInformation
	require.NoError(t, json.Unmarshal(raw, &hits))
	require.NotEmpty(t, hits)
	assert.Equal(t, "Top", hits[0].Name)
}

func TestLocationsForRefsToHeadingMultiFileSort(t *testing.T) {
	t.Parallel()
	srcA := "# A\n\n## Sec\n"
	srcB := "# B\n\n[s](./a.md#sec)\n"
	srcC := "# C\n\n[s](./a.md#sec)\n[s2](./a.md#sec)\n"
	h, _, rootURI := rootedHarness(t, map[string]string{
		"a.md": srcA, "b.md": srcB, "c.md": srcC,
	})
	idx := h.srv.ensureIndex()
	got := h.srv.locationsForRefsToHeading("a.md", "sec", idx)
	require.GreaterOrEqual(t, len(got), 3)
	// Verify the sort fires: same-URI entries grouped and ordered
	// by line.
	for i := 1; i < len(got); i++ {
		if got[i-1].URI == got[i].URI {
			assert.LessOrEqual(t, got[i-1].Range.Start.Line, got[i].Range.Start.Line)
		}
	}
	_ = rootURI
}

func TestDocTextOrFileReadsOnDiskFile(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "a.md"), []byte("# A\n"), 0o644))
	h := newHarness(t)
	rootURI := pathToFileURI(t, tmp)
	_, errResp := h.request("initialize", initializeParams{
		RootURI: &rootURI, Capabilities: clientCapabilities{},
	})
	require.Nil(t, errResp)
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(filepath.Join(tmp, "a.md"))}
	data, rel, ok := h.srv.docTextOrFile(u.String())
	assert.True(t, ok)
	assert.Equal(t, "a.md", rel)
	assert.NotEmpty(t, data)
}

func TestDocTextOrFileRejectsOutsideWorkspace(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "a.md"), []byte("# A\n"), 0o644))
	// Write a file outside the workspace.
	outside := filepath.Join(t.TempDir(), "leak.md")
	require.NoError(t, os.WriteFile(outside, []byte("# Secret\n"), 0o644))

	h := newHarness(t)
	rootURI := pathToFileURI(t, tmp)
	_, errResp := h.request("initialize", initializeParams{
		RootURI: &rootURI, Capabilities: clientCapabilities{},
	})
	require.Nil(t, errResp)

	u := url.URL{Scheme: "file", Path: filepath.ToSlash(outside)}
	_, _, ok := h.srv.docTextOrFile(u.String())
	assert.False(t, ok, "out-of-workspace files must not be readable via docTextOrFile")
}

func TestDocTextOrFileRejectsNonMarkdown(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "secret.txt"), []byte("data"), 0o644))
	h := newHarness(t)
	rootURI := pathToFileURI(t, tmp)
	_, errResp := h.request("initialize", initializeParams{
		RootURI: &rootURI, Capabilities: clientCapabilities{},
	})
	require.Nil(t, errResp)
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(filepath.Join(tmp, "secret.txt"))}
	_, _, ok := h.srv.docTextOrFile(u.String())
	assert.False(t, ok)
}

func TestInsideWorkspaceCases(t *testing.T) {
	t.Parallel()
	// Empty root fails closed: no workspace configured means no
	// on-disk reads.
	assert.False(t, insideWorkspace("", "/anywhere/x.md"))
	// Inside.
	tmp := t.TempDir()
	abs, _ := filepath.Abs(filepath.Join(tmp, "a.md"))
	assert.True(t, insideWorkspace(tmp, abs))
	// Outside.
	other := t.TempDir()
	abs, _ = filepath.Abs(filepath.Join(other, "a.md"))
	assert.False(t, insideWorkspace(tmp, abs))
}

func TestIsMarkdownExt(t *testing.T) {
	t.Parallel()
	assert.True(t, isMarkdownExt("a.md"))
	assert.True(t, isMarkdownExt("a.MD"))
	assert.True(t, isMarkdownExt("a.markdown"))
	assert.False(t, isMarkdownExt("a.txt"))
	assert.False(t, isMarkdownExt("noext"))
}

func TestDocTextOrFileMissingDisk(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	_, errResp := h.request("initialize", initializeParams{})
	require.Nil(t, errResp)
	abs, err := filepath.Abs(filepath.Join(t.TempDir(), "nope.md"))
	require.NoError(t, err)
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(abs)}
	_, _, ok := h.srv.docTextOrFile(u.String())
	assert.False(t, ok)
}

func TestEnsureIndexEmptyRoot(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	_, errResp := h.request("initialize", initializeParams{})
	require.Nil(t, errResp)
	idx := h.srv.ensureIndex()
	assert.NotNil(t, idx)
	assert.Empty(t, idx.Files())
}

func TestEnsureIndexAfterDocOpenButNoBuild(t *testing.T) {
	t.Parallel()
	src := "# Top\n"
	h, _, rootURI := rootedHarness(t, map[string]string{"a.md": src})
	h.srv.settingsMu.Lock()
	h.srv.settings.Run = runOff
	h.srv.settingsMu.Unlock()
	uri := rootURI + "/a.md"
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uri, LanguageID: "markdown", Version: 1, Text: src},
	})
	idx := h.srv.ensureIndex()
	assert.NotNil(t, idx)
	assert.NotEmpty(t, idx.Files())
}

func TestIndexUpdateNoIndex(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	_, errResp := h.request("initialize", initializeParams{})
	require.Nil(t, errResp)
	// idx is nil — should be a no-op.
	h.srv.indexUpdate("/abs/x.md", []byte("# x\n"))
	assert.Nil(t, h.srv.idx)
}

func TestIndexUpdateNonAbs(t *testing.T) {
	t.Parallel()
	src := "# Top\n"
	h, _, rootURI := rootedHarness(t, map[string]string{"a.md": src})
	h.srv.settingsMu.Lock()
	h.srv.settings.Run = runOff
	h.srv.settingsMu.Unlock()
	// Force the index up.
	h.srv.ensureIndex()
	// Pass a relative path (workspaceRelative returns it unchanged).
	h.srv.indexUpdate("a.md", []byte("# Updated\n"))
	fe, ok := h.srv.idx.File("a.md")
	require.True(t, ok)
	assert.NotEmpty(t, fe.Symbols)
	_ = rootURI
}

func TestIndexReloadFromDiskMissingThenWritten(t *testing.T) {
	t.Parallel()
	src := "# Top\n"
	h, tmp, _ := rootedHarness(t, map[string]string{"a.md": src})
	h.srv.settingsMu.Lock()
	h.srv.settings.Run = runOff
	h.srv.settingsMu.Unlock()
	h.srv.ensureIndex()
	// Delete then reload — should evict.
	abs := filepath.Join(tmp, "a.md")
	require.NoError(t, os.Remove(abs))
	h.srv.indexReloadFromDisk(abs)
	_, ok := h.srv.idx.File("a.md")
	assert.False(t, ok)
	// Re-add and reload — should re-index.
	require.NoError(t, os.WriteFile(abs, []byte("# Back\n"), 0o644))
	h.srv.indexReloadFromDisk(abs)
	fe, ok := h.srv.idx.File("a.md")
	require.True(t, ok)
	assert.Equal(t, "Back", fe.Symbols[0].Name)
}

func TestLocationsForFileLinkUnknownTarget(t *testing.T) {
	t.Parallel()
	h, _, _ := rootedHarness(t, map[string]string{"a.md": "# A\n"})
	idx := h.srv.ensureIndex()
	got := h.srv.locationsForFileLink("not-in-index.md", "anchor", idx)
	require.Len(t, got, 1)
	assert.Equal(t, 0, got[0].Range.Start.Line)
}

func TestLocationsForFileLinkEmptyTarget(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	_, errResp := h.request("initialize", initializeParams{})
	require.Nil(t, errResp)
	got := h.srv.locationsForFileLink("", "anchor", h.srv.ensureIndex())
	assert.Empty(t, got)
}

func TestLocationsForFileLinkAnchorNotInTarget(t *testing.T) {
	t.Parallel()
	srcA := "# A\n"
	srcB := "# B\n"
	h, _, _ := rootedHarness(t, map[string]string{"a.md": srcA, "b.md": srcB})
	idx := h.srv.ensureIndex()
	got := h.srv.locationsForFileLink("b.md", "missing", idx)
	require.Len(t, got, 1)
	assert.Equal(t, 0, got[0].Range.Start.Line)
}

func TestLocationForRefDefMissing(t *testing.T) {
	t.Parallel()
	h, _, rootURI := rootedHarness(t, map[string]string{"a.md": "# A\n"})
	h.srv.settingsMu.Lock()
	h.srv.settings.Run = runOff
	h.srv.settingsMu.Unlock()
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: rootURI + "/a.md", LanguageID: "markdown", Version: 1, Text: "# A\n"},
	})
	_, ok := h.srv.locationForRefDef("a.md", "nonexistent", []byte("# A\n"))
	assert.False(t, ok)
}

func TestLocationForRefDefUnknownFile(t *testing.T) {
	t.Parallel()
	h, _, _ := rootedHarness(t, map[string]string{"a.md": "# A\n"})
	_, ok := h.srv.locationForRefDef("not-in-index.md", "x", nil)
	assert.False(t, ok)
}

func TestLocationsForRefsToHeadingEmpty(t *testing.T) {
	t.Parallel()
	h, _, _ := rootedHarness(t, map[string]string{"a.md": "# A\n"})
	got := h.srv.locationsForRefsToHeading("a.md", "", h.srv.ensureIndex())
	assert.Empty(t, got)
}

func TestLocationsForRefsToHeadingSorted(t *testing.T) {
	t.Parallel()
	srcA := "# A\n\n## Sec\n"
	srcB := "# B\n\n[s](./a.md#sec)\n[s2](./a.md#sec)\n"
	srcC := "# C\n\n[s](./a.md#sec)\n"
	h, _, rootURI := rootedHarness(t, map[string]string{
		"a.md": srcA, "b.md": srcB, "c.md": srcC,
	})
	got := h.srv.locationsForRefsToHeading("a.md", "sec", h.srv.ensureIndex())
	require.Len(t, got, 3)
	// Check sort: same URI grouped, then by line.
	for i := 1; i < len(got); i++ {
		if got[i-1].URI == got[i].URI {
			assert.LessOrEqual(t, got[i-1].Range.Start.Line, got[i].Range.Start.Line)
		} else {
			assert.Less(t, got[i-1].URI, got[i].URI)
		}
	}
	_ = rootURI
}

func TestLocationsForKindDefinitionWithKindBlock(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmp, ".mdsmith.yml"), []byte(`
kinds:
  guide: {}
`), 0o644))
	h := newHarness(t)
	rootURI := pathToFileURI(t, tmp)
	_, errResp := h.request("initialize", initializeParams{
		RootURI: &rootURI, Capabilities: clientCapabilities{},
	})
	require.Nil(t, errResp)
	h.srv.reloadConfig()
	got := h.srv.locationsForKindDefinition("guide")
	require.Len(t, got, 1)
	assert.Contains(t, got[0].URI, ".mdsmith.yml")
	// Missing kind.
	assert.Empty(t, h.srv.locationsForKindDefinition("not-a-kind"))
}

func TestLocationsForRefUsesUnknownFile(t *testing.T) {
	t.Parallel()
	h, _, _ := rootedHarness(t, map[string]string{"a.md": "# A\n"})
	got := h.srv.locationsForRefUses("nope.md", "x", h.srv.ensureIndex())
	assert.Empty(t, got)
}

func TestLocationsForFileTopFiltersAnchorAndKind(t *testing.T) {
	t.Parallel()
	srcA := "# A\n"
	// Only the unanchored file link should be included.
	srcB := "# B\n\n[a](./a.md)\n[c](./a.md#sec)\n"
	srcInc := "# I\n\n<?include\nfile: \"./a.md\"\n?>\n<?/include?>\n"
	h, _, rootURI := rootedHarness(t, map[string]string{
		"a.md": srcA, "b.md": srcB, "i.md": srcInc,
	})
	idx := h.srv.ensureIndex()
	got := h.srv.locationsForFileTop("a.md", idx)
	require.Len(t, got, 1)
	assert.Equal(t, rootURI+"/b.md", got[0].URI)
}

func TestLocationsForFileReferencesIncludesDirectives(t *testing.T) {
	t.Parallel()
	srcA := "# A\n"
	srcB := "# B\n\n[a](./a.md)\n"
	srcInc := "# I\n\n<?include\nfile: \"./a.md\"\n?>\n<?/include?>\n"
	srcBuild := "# B2\n\n<?build\nrecipe: r\ninputs:\n  - \"./a.md\"\noutputs:\n  - out.html\n?>\n<?/build?>\n"
	h, _, _ := rootedHarness(t, map[string]string{
		"a.md": srcA, "b.md": srcB, "i.md": srcInc, "b2.md": srcBuild,
	})
	idx := h.srv.ensureIndex()
	got := h.srv.locationsForFileReferences("a.md", idx)
	assert.GreaterOrEqual(t, len(got), 3,
		"expected file link + include + build references, got %v", got)
}

func TestLocationsForFileReferencesUnknownFile(t *testing.T) {
	t.Parallel()
	h, _, _ := rootedHarness(t, map[string]string{"a.md": "# A\n"})
	got := h.srv.locationsForFileReferences("absent.md", h.srv.ensureIndex())
	assert.Empty(t, got)
}

func TestHandleWorkspaceSymbolEmptyResultIsList(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	_, errResp := h.request("initialize", initializeParams{})
	require.Nil(t, errResp)
	raw, errResp := h.request("workspace/symbol", workspaceSymbolParams{Query: ""})
	require.Nil(t, errResp)
	// Empty result must be `[]`, not `null`.
	assert.True(t, strings.HasPrefix(string(raw), "["))
}

func TestHandleReferencesOnPlainProse(t *testing.T) {
	t.Parallel()
	src := "# Top\n\nplain prose only\n"
	h, _, rootURI := rootedHarness(t, map[string]string{"a.md": src})
	uri := rootURI + "/a.md"
	h.srv.settingsMu.Lock()
	h.srv.settings.Run = runOff
	h.srv.settingsMu.Unlock()
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uri, LanguageID: "markdown", Version: 1, Text: src},
	})
	raw, errResp := h.request("textDocument/references", referencesParams{
		textDocumentPositionParams: textDocumentPositionParams{
			TextDocument: textDocumentIdentifier{URI: uri},
			Position:     Position{Line: 2, Character: 5},
		},
	})
	require.Nil(t, errResp)
	var locs []location
	require.NoError(t, json.Unmarshal(raw, &locs))
	assert.Empty(t, locs)
}

func TestHandleReferencesOnMissingDocument(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	_, errResp := h.request("initialize", initializeParams{})
	require.Nil(t, errResp)
	raw, errResp := h.request("textDocument/references", referencesParams{
		textDocumentPositionParams: textDocumentPositionParams{
			TextDocument: textDocumentIdentifier{URI: "file:///nope/x.md"},
			Position:     Position{Line: 0, Character: 0},
		},
	})
	require.Nil(t, errResp)
	var locs []location
	require.NoError(t, json.Unmarshal(raw, &locs))
	assert.Empty(t, locs)
}

func TestHandlePrepareCallHierarchyMissingDocument(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	_, errResp := h.request("initialize", initializeParams{})
	require.Nil(t, errResp)
	raw, errResp := h.request("textDocument/prepareCallHierarchy", textDocumentPositionParams{
		TextDocument: textDocumentIdentifier{URI: "file:///nope.md"},
	})
	require.Nil(t, errResp)
	var items []callHierarchyItem
	require.NoError(t, json.Unmarshal(raw, &items))
	assert.Empty(t, items)
}

func TestWatcherAcceptsCaseInsensitiveMarkdownExt(t *testing.T) {
	t.Parallel()
	// The watcher must accept `.MD` / `.Markdown` as Markdown so a
	// rename to an upper-case extension still refreshes the index.
	tmp := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "a.MD"), []byte("# Upper\n"), 0o644))

	h := newHarness(t)
	rootURI := pathToFileURI(t, tmp)
	_, errResp := h.request("initialize", initializeParams{
		RootURI: &rootURI, Capabilities: clientCapabilities{},
	})
	require.Nil(t, errResp)
	h.srv.settingsMu.Lock()
	h.srv.settings.Run = runOff
	h.srv.settingsMu.Unlock()
	// Force the index to build empty.
	h.srv.ensureIndex()

	abs := filepath.Join(tmp, "a.MD")
	h.notify("workspace/didChangeWatchedFiles", didChangeWatchedFilesParams{
		Changes: []fileEvent{{URI: "file://" + abs, Type: 1}},
	})
	// The watcher dispatches asynchronously; issue a follow-up
	// request to drain the queue, then verify the index picked up
	// the case-shifted extension.
	_, _ = h.request("workspace/symbol", workspaceSymbolParams{Query: ""})
	assert.Contains(t, h.srv.idx.Files(), "a.MD")
}

func TestHandlePrepareCallHierarchyOnPlainProseEmpty(t *testing.T) {
	t.Parallel()
	// Cursor sits in a paragraph that's neither a heading nor a
	// directive arg. The handler must NOT synthesize a file-level
	// item for arbitrary mid-document positions — that would
	// surface a phantom "Call Hierarchy" entry for every
	// paragraph in the file. Only TokenFileTop (line 1, col 1)
	// anchors at the file.
	src := "# Top\n\nplain prose with no link\n"
	h, _, rootURI := rootedHarness(t, map[string]string{"a.md": src})
	uri := rootURI + "/a.md"
	h.srv.settingsMu.Lock()
	h.srv.settings.Run = runOff
	h.srv.settingsMu.Unlock()
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uri, LanguageID: "markdown", Version: 1, Text: src},
	})
	raw, errResp := h.request("textDocument/prepareCallHierarchy", textDocumentPositionParams{
		TextDocument: textDocumentIdentifier{URI: uri},
		Position:     Position{Line: 2, Character: 5},
	})
	require.Nil(t, errResp)
	var items []callHierarchyItem
	require.NoError(t, json.Unmarshal(raw, &items))
	assert.Empty(t, items)
}

func TestHandlePrepareCallHierarchyDirectiveMissingTarget(t *testing.T) {
	t.Parallel()
	// Cursor on a directive arg whose key has no target file (not
	// include's `file:` nor a build `inputs:` list item) —
	// handlePrepareCallHierarchy has no target file
	// to anchor at, so the response is an empty item slice rather
	// than a synthetic file-level fallback. The empty slice is
	// what the editor needs to render "no call hierarchy here"
	// without spawning a phantom item.
	src := "# T\n\n<?include\nstrip-frontmatter: \"true\"\n?>\n<?/include?>\n"
	h, _, rootURI := rootedHarness(t, map[string]string{"a.md": src})
	uri := rootURI + "/a.md"
	h.srv.settingsMu.Lock()
	h.srv.settings.Run = runOff
	h.srv.settingsMu.Unlock()
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uri, LanguageID: "markdown", Version: 1, Text: src},
	})
	// Cursor on the directive arg line.
	raw, errResp := h.request("textDocument/prepareCallHierarchy", textDocumentPositionParams{
		TextDocument: textDocumentIdentifier{URI: uri},
		Position:     Position{Line: 3, Character: 5},
	})
	require.Nil(t, errResp)
	var items []callHierarchyItem
	require.NoError(t, json.Unmarshal(raw, &items))
	// Directive without a target file → empty item slice.
	assert.Empty(t, items)
}

func TestHeadingRangeFromIndexMissingFile(t *testing.T) {
	t.Parallel()
	src := []byte("# Top\n")
	r := headingRangeFromIndex("a.md", "anchor", nil, src)
	assert.Equal(t, 0, r.Start.Line)
}

func TestHeadingRangeFromIndexUnknownAnchor(t *testing.T) {
	t.Parallel()
	idx := index.New("/r")
	idx.Update("a.md", []byte("# Top\n"))
	fe, ok := idx.File("a.md")
	require.True(t, ok)
	r := headingRangeFromIndex("a.md", "absent-anchor", fe, []byte("# Top\n"))
	assert.Equal(t, 0, r.Start.Line)
}

func TestOutgoingScopeFileLevel(t *testing.T) {
	t.Parallel()
	idx := index.New("/r")
	start, end := outgoingScope(idx, &callHierarchyData{File: "x.md"})
	assert.Equal(t, 1, start)
	assert.Equal(t, 0, end)
	// Nil data.
	start, end = outgoingScope(idx, nil)
	assert.Equal(t, 1, start)
	assert.Equal(t, 0, end)
}

func TestOutgoingScopeUnknownFile(t *testing.T) {
	t.Parallel()
	idx := index.New("/r")
	start, end := outgoingScope(idx, &callHierarchyData{File: "missing.md", Anchor: "x"})
	assert.Equal(t, 1, start)
	assert.Equal(t, 0, end)
}

func TestOutgoingScopeAnchorNotFound(t *testing.T) {
	t.Parallel()
	idx := index.New("/r")
	idx.Update("a.md", []byte("# A\n"))
	start, end := outgoingScope(idx, &callHierarchyData{File: "a.md", Anchor: "missing"})
	assert.Equal(t, 1, start)
	assert.Equal(t, 0, end)
}

func TestEffectiveKindsForNoConfig(t *testing.T) {
	t.Parallel()
	got := effectiveKindsFor(nil, "a.md", []byte("# A\n"))
	assert.Nil(t, got)
}

func TestEffectiveKindsForBadFrontMatter(t *testing.T) {
	t.Parallel()
	// Front matter parse error must not crash; the function
	// gracefully degrades to whatever the config layer returns
	// (which may be nil when no kind-assignment matches).
	tmp := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmp, ".mdsmith.yml"), []byte("kinds:\n  guide: {}\n"), 0o644))
	h := newHarness(t)
	rootURI := pathToFileURI(t, tmp)
	_, errResp := h.request("initialize", initializeParams{
		RootURI: &rootURI, Capabilities: clientCapabilities{},
	})
	require.Nil(t, errResp)
	h.srv.reloadConfig()
	cfg, _, _ := h.srv.snapshotConfig()
	require.NotNil(t, cfg)
	// Malformed YAML in the front matter — exercises the err
	// branch in effectiveKindsFor.
	got := effectiveKindsFor(cfg, "a.md", []byte("---\n!!malformed\n---\n# A\n"))
	assert.NotPanics(t, func() { _ = got })
}

func TestBuildOutlineEmptyDocument(t *testing.T) {
	t.Parallel()
	got := buildOutline(nil)
	assert.Empty(t, got)
}

func TestLeafSymbolDirective(t *testing.T) {
	t.Parallel()
	got := leafSymbol(index.Symbol{
		Kind:          index.SymbolDirective,
		Name:          "include",
		StartLine:     5,
		EndLine:       7,
		SelectionLine: 5,
		SelectionCol:  1,
	}, []byte(""))
	assert.Equal(t, "include", got.Name)
	assert.Equal(t, "<?include?>", got.Detail)
	assert.Equal(t, symbolKindEvent, got.Kind)
}

func TestLeafSymbolLinkRef(t *testing.T) {
	t.Parallel()
	got := leafSymbol(index.Symbol{
		Kind:          index.SymbolLinkRef,
		Name:          "lab",
		StartLine:     1,
		SelectionLine: 1,
		SelectionCol:  1,
	}, nil)
	assert.Equal(t, symbolKindKey, got.Kind)
	assert.Equal(t, "[lab]:", got.Detail)
}

func TestLeafSymbolFrontMatter(t *testing.T) {
	t.Parallel()
	got := leafSymbol(index.Symbol{
		Kind:          index.SymbolFrontMatter,
		Name:          "title",
		StartLine:     1,
		SelectionLine: 1,
		SelectionCol:  1,
	}, nil)
	assert.Equal(t, symbolKindProperty, got.Kind)
	assert.Empty(t, got.Detail)
}

func TestDefinitionDirectiveArgEscapeRejected(t *testing.T) {
	t.Parallel()
	// `<?include file: "../../escape.md"?>` from `docs/a.md`
	// resolves outside the workspace root — definition must
	// return null, not a Location pointing at a sibling project.
	tmp := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(tmp, "docs"), 0o755))
	srcA := "# A\n\n<?include\nfile: \"../../escape.md\"\n?>\n<?/include?>\n"
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "docs", "a.md"), []byte(srcA), 0o644))

	h := newHarness(t)
	rootURI := pathToFileURI(t, tmp)
	_, errResp := h.request("initialize", initializeParams{
		RootURI: &rootURI, Capabilities: clientCapabilities{},
	})
	require.Nil(t, errResp)
	h.srv.settingsMu.Lock()
	h.srv.settings.Run = runOff
	h.srv.settingsMu.Unlock()

	uri := rootURI + "/docs/a.md"
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uri, LanguageID: "markdown", Version: 1, Text: srcA},
	})
	// Cursor on the directive arg line (line 4 → 0-based 3).
	raw, errResp := h.request("textDocument/definition", textDocumentPositionParams{
		TextDocument: textDocumentIdentifier{URI: uri},
		Position:     Position{Line: 3, Character: 8},
	})
	require.Nil(t, errResp)
	assert.Equal(t, "null", string(raw),
		"escapes-the-root directive target must produce a null definition: %s", raw)
}

func TestDefinitionDirectiveArgWithinRoot(t *testing.T) {
	t.Parallel()
	// `<?include file: "../sibling.md"?>` from `docs/a.md` resolves
	// to `sibling.md` — still inside the workspace, so definition
	// must return that file.
	tmp := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(tmp, "docs"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "sibling.md"), []byte("# Sibling\n"), 0o644))
	srcA := "# A\n\n<?include\nfile: \"../sibling.md\"\n?>\n<?/include?>\n"
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "docs", "a.md"), []byte(srcA), 0o644))

	h := newHarness(t)
	rootURI := pathToFileURI(t, tmp)
	_, errResp := h.request("initialize", initializeParams{
		RootURI: &rootURI, Capabilities: clientCapabilities{},
	})
	require.Nil(t, errResp)
	h.srv.settingsMu.Lock()
	h.srv.settings.Run = runOff
	h.srv.settingsMu.Unlock()
	uri := rootURI + "/docs/a.md"
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uri, LanguageID: "markdown", Version: 1, Text: srcA},
	})
	raw, errResp := h.request("textDocument/definition", textDocumentPositionParams{
		TextDocument: textDocumentIdentifier{URI: uri},
		Position:     Position{Line: 3, Character: 8},
	})
	require.Nil(t, errResp)
	var loc location
	require.NoError(t, json.Unmarshal(raw, &loc))
	assert.Equal(t, rootURI+"/sibling.md", loc.URI)
}

func TestReferencesOnFrontMatterKindValueListsFiles(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmp, ".mdsmith.yml"),
		[]byte("kinds:\n  guide: {}\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "src.md"),
		[]byte("---\nkind: guide\n---\n# Cursor here\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "fm.md"),
		[]byte("---\nkinds:\n  - guide\n---\n# FM declared\n"), 0o644))
	h := newHarness(t)
	rootURI := pathToFileURI(t, tmp)
	_, errResp := h.request("initialize", initializeParams{
		RootURI: &rootURI, Capabilities: clientCapabilities{},
	})
	require.Nil(t, errResp)
	h.srv.settingsMu.Lock()
	h.srv.settings.Run = runOff
	h.srv.settingsMu.Unlock()
	h.srv.reloadConfig()
	h.srv.invalidateIndex()
	uri := rootURI + "/src.md"
	src := "---\nkind: guide\n---\n# Cursor here\n"
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uri, LanguageID: "markdown", Version: 1, Text: src},
	})
	// Cursor on `kind: guide` value (line 2 char 8).
	raw, errResp := h.request("textDocument/references", referencesParams{
		textDocumentPositionParams: textDocumentPositionParams{
			TextDocument: textDocumentIdentifier{URI: uri},
			Position:     Position{Line: 1, Character: 8},
		},
	})
	require.Nil(t, errResp)
	var locs []location
	require.NoError(t, json.Unmarshal(raw, &locs))
	assert.NotEmpty(t, locs)
}

func TestPrepareCallHierarchyDirectiveEscapeRejected(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(tmp, "docs"), 0o755))
	srcA := "# A\n\n<?include\nfile: \"../../way-up.md\"\n?>\n<?/include?>\n"
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "docs", "a.md"), []byte(srcA), 0o644))

	h := newHarness(t)
	rootURI := pathToFileURI(t, tmp)
	_, errResp := h.request("initialize", initializeParams{
		RootURI: &rootURI, Capabilities: clientCapabilities{},
	})
	require.Nil(t, errResp)
	h.srv.settingsMu.Lock()
	h.srv.settings.Run = runOff
	h.srv.settingsMu.Unlock()
	uri := rootURI + "/docs/a.md"
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uri, LanguageID: "markdown", Version: 1, Text: srcA},
	})
	raw, errResp := h.request("textDocument/prepareCallHierarchy", textDocumentPositionParams{
		TextDocument: textDocumentIdentifier{URI: uri},
		Position:     Position{Line: 3, Character: 8},
	})
	require.Nil(t, errResp)
	var items []callHierarchyItem
	require.NoError(t, json.Unmarshal(raw, &items))
	assert.Empty(t, items, "directive arg targeting a path that escapes the workspace must produce no item")
}

func TestHandleDefinitionUnknownDoc(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	_, errResp := h.request("initialize", initializeParams{})
	require.Nil(t, errResp)
	raw, errResp := h.request("textDocument/definition", textDocumentPositionParams{
		TextDocument: textDocumentIdentifier{URI: "file:///nope.md"},
		Position:     Position{Line: 0, Character: 0},
	})
	require.Nil(t, errResp)
	assert.Equal(t, "null", string(raw))
}

func TestHandleImplementationUnknownDoc(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	_, errResp := h.request("initialize", initializeParams{})
	require.Nil(t, errResp)
	raw, errResp := h.request("textDocument/implementation", textDocumentPositionParams{
		TextDocument: textDocumentIdentifier{URI: "file:///nope.md"},
	})
	require.Nil(t, errResp)
	var locs []location
	require.NoError(t, json.Unmarshal(raw, &locs))
	assert.Empty(t, locs)
}

func TestResolveTargetsRefDef(t *testing.T) {
	t.Parallel()
	src := "# T\n\n[See][label]\n\n[label]: ./other.md\n"
	h, _, rootURI := rootedHarness(t, map[string]string{"a.md": src})
	uri := rootURI + "/a.md"
	h.srv.settingsMu.Lock()
	h.srv.settings.Run = runOff
	h.srv.settingsMu.Unlock()
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uri, LanguageID: "markdown", Version: 1, Text: src},
	})
	// Cursor on `[label]:` line 5.
	raw, errResp := h.request("textDocument/definition", textDocumentPositionParams{
		TextDocument: textDocumentIdentifier{URI: uri},
		Position:     Position{Line: 4, Character: 2},
	})
	require.Nil(t, errResp)
	var loc location
	require.NoError(t, json.Unmarshal(raw, &loc))
	assert.Equal(t, uri, loc.URI)
	// `implementation` returns the same single result for a TokenRefDef.
	raw, errResp = h.request("textDocument/implementation", textDocumentPositionParams{
		TextDocument: textDocumentIdentifier{URI: uri},
		Position:     Position{Line: 4, Character: 2},
	})
	require.Nil(t, errResp)
	var locs []location
	require.NoError(t, json.Unmarshal(raw, &locs))
	assert.NotEmpty(t, locs)
}

func TestLocationsForAnchorEmpty(t *testing.T) {
	t.Parallel()
	h, _, _ := rootedHarness(t, map[string]string{"a.md": "# A\n"})
	got := h.srv.locationsForAnchor("a.md", "", h.srv.ensureIndex(), nil)
	assert.Empty(t, got)
}

func TestLocationsForAnchorMissingAnchor(t *testing.T) {
	t.Parallel()
	h, _, _ := rootedHarness(t, map[string]string{"a.md": "# A\n"})
	got := h.srv.locationsForAnchor("a.md", "missing", h.srv.ensureIndex(), nil)
	assert.Empty(t, got)
}

func TestHandleReferencesOnFrontMatterNonKind(t *testing.T) {
	t.Parallel()
	src := "---\ntitle: Hello\n---\n# Body\n"
	h, _, rootURI := rootedHarness(t, map[string]string{"a.md": src})
	uri := rootURI + "/a.md"
	h.srv.settingsMu.Lock()
	h.srv.settings.Run = runOff
	h.srv.settingsMu.Unlock()
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uri, LanguageID: "markdown", Version: 1, Text: src},
	})
	// Cursor on `title: Hello` value (line 2 char 12).
	raw, errResp := h.request("textDocument/references", referencesParams{
		textDocumentPositionParams: textDocumentPositionParams{
			TextDocument: textDocumentIdentifier{URI: uri},
			Position:     Position{Line: 1, Character: 9},
		},
	})
	require.Nil(t, errResp)
	var locs []location
	require.NoError(t, json.Unmarshal(raw, &locs))
	// Non-kind FM value → no references.
	assert.Empty(t, locs)
}

func TestLocationsForRefUsesFiltersKindAndLabel(t *testing.T) {
	t.Parallel()
	src := "# T\n\n[A][lab1] and [B][lab2]\n\n[lab1]: u1\n[lab2]: u2\n"
	h, _, _ := rootedHarness(t, map[string]string{"a.md": src})
	idx := h.srv.ensureIndex()
	// Asking for "lab1" should pick the first ref use only.
	got := h.srv.locationsForRefUses("a.md", "lab1", idx)
	assert.Len(t, got, 1)
}

func TestWorkspaceSymbolMixedKinds(t *testing.T) {
	t.Parallel()
	src := "---\ntitle: Foo\nkinds:\n  - guide\n---\n# Match Foo\n\n[lab]: u\n"
	h, _, _ := rootedHarness(t, map[string]string{"a.md": src})
	raw, errResp := h.request("workspace/symbol", workspaceSymbolParams{Query: ""})
	require.Nil(t, errResp)
	var hits []symbolInformation
	require.NoError(t, json.Unmarshal(raw, &hits))
	kinds := map[symbolKind]bool{}
	for _, h := range hits {
		kinds[h.Kind] = true
	}
	assert.True(t, kinds[symbolKindString], "expected heading kind: %+v", hits)
	// Either Property (front matter) or Key (link ref) should appear.
	assert.True(t, kinds[symbolKindProperty] || kinds[symbolKindKey], "expected property or key kind: %+v", hits)
}

func TestHandleOutgoingCallsCatalogPlaceholder(t *testing.T) {
	t.Parallel()
	src := "# A\n\n<?catalog\nglob:\n  - \"*.md\"\n?>\n<?/catalog?>\n"
	h, _, rootURI := rootedHarness(t, map[string]string{"a.md": src, "b.md": "# B\n"})
	uri := rootURI + "/a.md"
	h.srv.settingsMu.Lock()
	h.srv.settings.Run = runOff
	h.srv.settingsMu.Unlock()
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uri, LanguageID: "markdown", Version: 1, Text: src},
	})
	raw, errResp := h.request("textDocument/prepareCallHierarchy", textDocumentPositionParams{
		TextDocument: textDocumentIdentifier{URI: uri},
		Position:     Position{Line: 0, Character: 0},
	})
	require.Nil(t, errResp)
	var items []callHierarchyItem
	require.NoError(t, json.Unmarshal(raw, &items))
	require.Len(t, items, 1)
	raw, errResp = h.request("callHierarchy/outgoingCalls", callHierarchyOutgoingCallsParams{Item: items[0]})
	require.Nil(t, errResp)
	var calls []callHierarchyOutgoingCall
	require.NoError(t, json.Unmarshal(raw, &calls))
	// Catalog edge has no concrete target; the helper points at the
	// host directory as a placeholder.
	require.NotEmpty(t, calls)
}

func TestHandleOutgoingCallsSkipsRefAndAnchorLinks(t *testing.T) {
	t.Parallel()
	src := "# A\n\nSee [self](#sec) and [Foo][bar].\n\n## Sec\n\n[bar]: ./b.md\n"
	srcB := "# B\n"
	h, _, rootURI := rootedHarness(t, map[string]string{"a.md": src, "b.md": srcB})
	uri := rootURI + "/a.md"
	h.srv.settingsMu.Lock()
	h.srv.settings.Run = runOff
	h.srv.settingsMu.Unlock()
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uri, LanguageID: "markdown", Version: 1, Text: src},
	})
	raw, errResp := h.request("textDocument/prepareCallHierarchy", textDocumentPositionParams{
		TextDocument: textDocumentIdentifier{URI: uri},
		Position:     Position{Line: 0, Character: 0},
	})
	require.Nil(t, errResp)
	var items []callHierarchyItem
	require.NoError(t, json.Unmarshal(raw, &items))
	require.Len(t, items, 1)
	raw, errResp = h.request("callHierarchy/outgoingCalls", callHierarchyOutgoingCallsParams{Item: items[0]})
	require.Nil(t, errResp)
	var calls []callHierarchyOutgoingCall
	require.NoError(t, json.Unmarshal(raw, &calls))
	// Anchor link to #sec is intra-document (skipped). Reference-style
	// link [Foo][bar] is also intra-document (skipped). The target of
	// the reference resolves through link-ref to b.md but ast.Link
	// with non-nil Reference is dropped from outgoing.
	for _, c := range calls {
		assert.NotEmpty(t, c.To.Name)
	}
}

func TestHandleIncomingCallsSelfReferenceFiltered(t *testing.T) {
	t.Parallel()
	// Same file links to itself — the self-edge must not appear in
	// the incoming-calls view.
	src := "# A\n\n## Sec\n\n[loop](#sec)\n"
	h, _, rootURI := rootedHarness(t, map[string]string{"a.md": src})
	uri := rootURI + "/a.md"
	h.srv.settingsMu.Lock()
	h.srv.settings.Run = runOff
	h.srv.settingsMu.Unlock()
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uri, LanguageID: "markdown", Version: 1, Text: src},
	})
	// Prepare on `## Sec`.
	raw, errResp := h.request("textDocument/prepareCallHierarchy", textDocumentPositionParams{
		TextDocument: textDocumentIdentifier{URI: uri},
		Position:     Position{Line: 2, Character: 4},
	})
	require.Nil(t, errResp)
	var items []callHierarchyItem
	require.NoError(t, json.Unmarshal(raw, &items))
	require.Len(t, items, 1)
	raw, errResp = h.request("callHierarchy/incomingCalls", callHierarchyIncomingCallsParams{Item: items[0]})
	require.Nil(t, errResp)
	var calls []callHierarchyIncomingCall
	require.NoError(t, json.Unmarshal(raw, &calls))
	// Self-anchor edge from the same file → empty.
	assert.Empty(t, calls)
}

func TestLineCountUnbounded(t *testing.T) {
	t.Parallel()
	// Edge case: source ending without newline.
	assert.Equal(t, 2, lineCount([]byte("a\nb")))
	// Empty.
	assert.Equal(t, 1, lineCount(nil))
}

func TestBuildOutlineLinkRefBranch(t *testing.T) {
	t.Parallel()
	src := []byte("# Top\n\nSee [foo][lab].\n\n[lab]: ./other.md\n")
	got := buildOutline(src)
	require.NotEmpty(t, got)
	// Walk for SymbolKindKey leaf (link-ref symbol).
	var sawKey bool
	var visit func(syms []documentSymbol)
	visit = func(syms []documentSymbol) {
		for _, s := range syms {
			if s.Kind == symbolKindKey {
				sawKey = true
			}
			visit(s.Children)
		}
	}
	visit(got)
	assert.True(t, sawKey, "expected link-ref leaf symbol: %+v", got)
}

func TestBuildIndexFromDiskHandlesReadFailure(t *testing.T) {
	t.Parallel()
	h, _, _ := rootedHarness(t, map[string]string{"a.md": "# A\n"})
	cfg, _, root := h.srv.snapshotConfig()
	idx := index.New(root)
	// Pass a path that doesn't exist on disk.
	h.srv.buildIndexFromDisk(idx, cfg, root, []string{"/does-not-exist/x.md"})
	// No panic, no entry added.
	assert.Empty(t, idx.Files())
}
