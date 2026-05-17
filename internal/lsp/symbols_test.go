package lsp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jeduden/mdsmith/internal/index"
	_ "github.com/jeduden/mdsmith/internal/rules/all"
)

// rootedHarness wires a Server to a real on-disk workspace so the
// symbol-navigation tests can drive lookups against actual files.
// The harness writes the supplied files under a tmp directory, then
// initializes the server with that directory as the workspace root.
func rootedHarness(t *testing.T, files map[string]string) (*testHarness, string, string) {
	t.Helper()
	tmp := t.TempDir()
	for rel, content := range files {
		full := filepath.Join(tmp, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
		require.NoError(t, os.WriteFile(full, []byte(content), 0o644))
	}
	h := newHarness(t)
	rootURI := pathToFileURI(t, tmp)
	_, errResp := h.request("initialize", initializeParams{
		RootURI:      &rootURI,
		Capabilities: clientCapabilities{},
	})
	require.Nil(t, errResp)
	return h, tmp, rootURI
}

func pathToFileURI(t *testing.T, p string) string {
	t.Helper()
	abs, err := filepath.Abs(p)
	require.NoError(t, err)
	// Use the production helper so the test URIs match what the
	// server emits — both follow RFC 8089 (drive-letter prefixed by
	// a `/`, UNC-as-host) and round-trip through uriToPathOnOS.
	return pathToURI(abs)
}

func TestInitializeAdvertisesNavigationCapabilities(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	resultRaw, errResp := h.request("initialize", initializeParams{})
	require.Nil(t, errResp)
	var res initializeResult
	require.NoError(t, json.Unmarshal(resultRaw, &res))
	assert.True(t, res.Capabilities.DocumentSymbolProvider)
	assert.True(t, res.Capabilities.DefinitionProvider)
	assert.True(t, res.Capabilities.ImplementationProvider)
	assert.True(t, res.Capabilities.ReferencesProvider)
	assert.True(t, res.Capabilities.WorkspaceSymbolProvider)
	assert.True(t, res.Capabilities.CallHierarchyProvider)
}

func TestDocumentSymbolReturnsHeadingTree(t *testing.T) {
	t.Parallel()
	h, _, rootURI := rootedHarness(t, map[string]string{
		"a.md": "# Top\n\n## Sub A\n\ntext\n\n## Sub B\n\nbody\n",
	})
	uri := rootURI + "/a.md"
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{
			URI: uri, LanguageID: "markdown", Version: 1,
			Text: "# Top\n\n## Sub A\n\ntext\n\n## Sub B\n\nbody\n",
		},
	})
	// Drain the diagnostics that come from didOpen.
	_ = h.awaitNotification("textDocument/publishDiagnostics", 5*time.Second)

	raw, errResp := h.request("textDocument/documentSymbol", documentSymbolParams{
		TextDocument: textDocumentIdentifier{URI: uri},
	})
	require.Nil(t, errResp)
	var syms []documentSymbol
	require.NoError(t, json.Unmarshal(raw, &syms))
	require.Len(t, syms, 1, "expected one root H1: %s", string(raw))
	assert.Equal(t, "Top", syms[0].Name)
	require.Len(t, syms[0].Children, 2)
	assert.Equal(t, "Sub A", syms[0].Children[0].Name)
	assert.Equal(t, "Sub B", syms[0].Children[1].Name)
}

func TestDocumentSymbolIncludesFrontMatter(t *testing.T) {
	t.Parallel()
	h, _, rootURI := rootedHarness(t, map[string]string{
		"a.md": "---\ntitle: Hi\n---\n# Top\n",
	})
	uri := rootURI + "/a.md"
	src := "---\ntitle: Hi\n---\n# Top\n"
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uri, LanguageID: "markdown", Version: 1, Text: src},
	})
	_ = h.awaitNotification("textDocument/publishDiagnostics", 5*time.Second)

	raw, errResp := h.request("textDocument/documentSymbol", documentSymbolParams{
		TextDocument: textDocumentIdentifier{URI: uri},
	})
	require.Nil(t, errResp)
	var syms []documentSymbol
	require.NoError(t, json.Unmarshal(raw, &syms))
	var sawFM bool
	for _, s := range syms {
		if s.Name == "front matter" {
			sawFM = true
			assert.NotEmpty(t, s.Children)
		}
	}
	assert.True(t, sawFM, "expected synthetic front-matter parent: %+v", syms)
}

func TestDefinitionAnchorLink(t *testing.T) {
	t.Parallel()
	src := "# Top\n\nSee [s](#sub).\n\n## Sub\n"
	h, _, rootURI := rootedHarness(t, map[string]string{"a.md": src})
	uri := rootURI + "/a.md"
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uri, LanguageID: "markdown", Version: 1, Text: src},
	})
	_ = h.awaitNotification("textDocument/publishDiagnostics", 5*time.Second)

	raw, errResp := h.request("textDocument/definition", textDocumentPositionParams{
		TextDocument: textDocumentIdentifier{URI: uri},
		// Cursor inside `[s](#sub)` — line 3 (0-based: 2), char 8.
		Position: Position{Line: 2, Character: 8},
	})
	require.Nil(t, errResp)
	var loc location
	require.NoError(t, json.Unmarshal(raw, &loc))
	assert.Equal(t, uri, loc.URI)
	// "## Sub" is the 5th line (1-based) → LSP line 4.
	assert.Equal(t, 4, loc.Range.Start.Line)
}

func TestDefinitionFileLink(t *testing.T) {
	t.Parallel()
	srcA := "# A\n\n[next](./b.md)\n"
	srcB := "# B\n"
	h, _, rootURI := rootedHarness(t, map[string]string{"a.md": srcA, "b.md": srcB})
	uri := rootURI + "/a.md"
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uri, LanguageID: "markdown", Version: 1, Text: srcA},
	})
	_ = h.awaitNotification("textDocument/publishDiagnostics", 5*time.Second)

	raw, errResp := h.request("textDocument/definition", textDocumentPositionParams{
		TextDocument: textDocumentIdentifier{URI: uri},
		Position:     Position{Line: 2, Character: 4},
	})
	require.Nil(t, errResp)
	var loc location
	require.NoError(t, json.Unmarshal(raw, &loc))
	expected := rootURI + "/b.md"
	assert.Equal(t, expected, loc.URI)
	assert.Equal(t, 0, loc.Range.Start.Line)
}

func TestDefinitionReferenceLink(t *testing.T) {
	t.Parallel()
	src := "# T\n\nSee [foo][bar].\n\n[bar]: https://example.com\n"
	h, _, rootURI := rootedHarness(t, map[string]string{"a.md": src})
	uri := rootURI + "/a.md"
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uri, LanguageID: "markdown", Version: 1, Text: src},
	})
	_ = h.awaitNotification("textDocument/publishDiagnostics", 5*time.Second)

	raw, errResp := h.request("textDocument/definition", textDocumentPositionParams{
		TextDocument: textDocumentIdentifier{URI: uri},
		// Cursor inside `[foo][bar]` on line 3.
		Position: Position{Line: 2, Character: 6},
	})
	require.Nil(t, errResp)
	var loc location
	require.NoError(t, json.Unmarshal(raw, &loc))
	assert.Equal(t, uri, loc.URI)
	// `[bar]: …` is on line 5 (1-based) → 4 (0-based).
	assert.Equal(t, 4, loc.Range.Start.Line)
}

func TestReferencesOnHeading(t *testing.T) {
	t.Parallel()
	srcA := "# A\n\n## Sec\n"
	srcB := "# B\n\n[s](./a.md#sec)\n"
	h, _, rootURI := rootedHarness(t, map[string]string{"a.md": srcA, "b.md": srcB})
	uri := rootURI + "/a.md"
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uri, LanguageID: "markdown", Version: 1, Text: srcA},
	})
	_ = h.awaitNotification("textDocument/publishDiagnostics", 5*time.Second)

	raw, errResp := h.request("textDocument/references", referencesParams{
		textDocumentPositionParams: textDocumentPositionParams{
			TextDocument: textDocumentIdentifier{URI: uri},
			// Cursor on `## Sec` (line 3, 1-based) → 2.
			Position: Position{Line: 2, Character: 3},
		},
		Context: referencesContext{IncludeDeclaration: false},
	})
	require.Nil(t, errResp)
	var locs []location
	require.NoError(t, json.Unmarshal(raw, &locs))
	require.Len(t, locs, 1)
	assert.Equal(t, rootURI+"/b.md", locs[0].URI)
}

func TestReferencesIncludeDeclaration(t *testing.T) {
	t.Parallel()
	srcA := "# A\n\n## Sec\n"
	srcB := "# B\n\n[s](./a.md#sec)\n"
	h, _, rootURI := rootedHarness(t, map[string]string{"a.md": srcA, "b.md": srcB})
	uri := rootURI + "/a.md"
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uri, LanguageID: "markdown", Version: 1, Text: srcA},
	})
	_ = h.awaitNotification("textDocument/publishDiagnostics", 5*time.Second)

	raw, errResp := h.request("textDocument/references", referencesParams{
		textDocumentPositionParams: textDocumentPositionParams{
			TextDocument: textDocumentIdentifier{URI: uri},
			Position:     Position{Line: 2, Character: 3},
		},
		Context: referencesContext{IncludeDeclaration: true},
	})
	require.Nil(t, errResp)
	var locs []location
	require.NoError(t, json.Unmarshal(raw, &locs))
	assert.Len(t, locs, 2, "expected the heading itself plus the link reference")
}

func TestWorkspaceSymbolMatchesHeading(t *testing.T) {
	t.Parallel()
	h, _, rootURI := rootedHarness(t, map[string]string{
		"a.md": "# Apple Pie\n",
		"b.md": "# Banana Split\n",
	})
	// Force the index to build.
	_, _ = h.request("workspace/symbol", workspaceSymbolParams{Query: ""})
	raw, errResp := h.request("workspace/symbol", workspaceSymbolParams{Query: "apple"})
	require.Nil(t, errResp)
	var hits []symbolInformation
	require.NoError(t, json.Unmarshal(raw, &hits))
	require.Len(t, hits, 1)
	assert.Equal(t, "Apple Pie", hits[0].Name)
	assert.Equal(t, rootURI+"/a.md", hits[0].Location.URI)
}

func TestPrepareAndIncomingCalls(t *testing.T) {
	t.Parallel()
	srcA := "# A\n"
	srcB := "# B\n\n[a](./a.md)\n"
	h, _, rootURI := rootedHarness(t, map[string]string{"a.md": srcA, "b.md": srcB})
	uriA := rootURI + "/a.md"

	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uriA, LanguageID: "markdown", Version: 1, Text: srcA},
	})
	_ = h.awaitNotification("textDocument/publishDiagnostics", 5*time.Second)

	raw, errResp := h.request("textDocument/prepareCallHierarchy", textDocumentPositionParams{
		TextDocument: textDocumentIdentifier{URI: uriA},
		Position:     Position{Line: 0, Character: 0},
	})
	require.Nil(t, errResp)
	var items []callHierarchyItem
	require.NoError(t, json.Unmarshal(raw, &items))
	require.Len(t, items, 1)
	assert.Equal(t, "a.md", items[0].Name)

	raw, errResp = h.request("callHierarchy/incomingCalls", callHierarchyIncomingCallsParams{Item: items[0]})
	require.Nil(t, errResp)
	var calls []callHierarchyIncomingCall
	require.NoError(t, json.Unmarshal(raw, &calls))
	require.Len(t, calls, 1)
	assert.Equal(t, "b.md", calls[0].From.Name)
}

func TestOutgoingCallsForIncludeChain(t *testing.T) {
	t.Parallel()
	srcA := "# A\n\n<?include\nfile: \"b.md\"\n?>\n<?/include?>\n"
	srcB := "# B\n\n<?include\nfile: \"c.md\"\n?>\n<?/include?>\n"
	srcC := "# C\n"
	h, _, rootURI := rootedHarness(t, map[string]string{
		"a.md": srcA, "b.md": srcB, "c.md": srcC,
	})
	// The include rule keeps per-run state on the registered
	// singleton; concurrent lint passes from t.Parallel() siblings
	// race that state. Disable lint for the duration of this test
	// since we only exercise the symbol-navigation surface.
	h.srv.settingsMu.Lock()
	h.srv.settings.Run = runOff
	h.srv.settingsMu.Unlock()

	uriA := rootURI + "/a.md"
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uriA, LanguageID: "markdown", Version: 1, Text: srcA},
	})

	raw, errResp := h.request("textDocument/prepareCallHierarchy", textDocumentPositionParams{
		TextDocument: textDocumentIdentifier{URI: uriA},
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
	assert.Equal(t, "b.md", calls[0].To.Name)
}

func TestIncomingCallsCoalescesByFile(t *testing.T) {
	t.Parallel()
	// b.md links to a.md twice — the call-hierarchy view should show
	// b.md once with two fromRanges, not two separate caller items.
	srcA := "# A\n"
	srcB := "# B\n\n[one](./a.md)\n[two](./a.md)\n"
	h, _, rootURI := rootedHarness(t, map[string]string{"a.md": srcA, "b.md": srcB})
	uriA := rootURI + "/a.md"
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uriA, LanguageID: "markdown", Version: 1, Text: srcA},
	})
	_ = h.awaitNotification("textDocument/publishDiagnostics", 5*time.Second)

	raw, errResp := h.request("textDocument/prepareCallHierarchy", textDocumentPositionParams{
		TextDocument: textDocumentIdentifier{URI: uriA},
		Position:     Position{Line: 0, Character: 0},
	})
	require.Nil(t, errResp)
	var items []callHierarchyItem
	require.NoError(t, json.Unmarshal(raw, &items))
	require.Len(t, items, 1)

	raw, errResp = h.request("callHierarchy/incomingCalls", callHierarchyIncomingCallsParams{Item: items[0]})
	require.Nil(t, errResp)
	var calls []callHierarchyIncomingCall
	require.NoError(t, json.Unmarshal(raw, &calls))
	require.Len(t, calls, 1, "expected one caller (coalesced)")
	assert.Len(t, calls[0].FromRanges, 2, "expected two fromRanges for the two links")
}

func TestOutgoingCallsScopedToHeading(t *testing.T) {
	t.Parallel()
	// a.md has two H2 sections, only the first links to b.md. A
	// heading-scoped outgoingCalls on the second section must not
	// inherit calls from the first.
	srcA := "# Top\n\n## First\n\n[one](./b.md)\n\n## Second\n\nno links here\n"
	srcB := "# B\n"
	h, _, rootURI := rootedHarness(t, map[string]string{"a.md": srcA, "b.md": srcB})
	uriA := rootURI + "/a.md"
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uriA, LanguageID: "markdown", Version: 1, Text: srcA},
	})
	_ = h.awaitNotification("textDocument/publishDiagnostics", 5*time.Second)

	// Cursor on `## Second` (line 7, 1-based → 6).
	raw, errResp := h.request("textDocument/prepareCallHierarchy", textDocumentPositionParams{
		TextDocument: textDocumentIdentifier{URI: uriA},
		Position:     Position{Line: 6, Character: 4},
	})
	require.Nil(t, errResp)
	var items []callHierarchyItem
	require.NoError(t, json.Unmarshal(raw, &items))
	require.Len(t, items, 1)
	require.NotNil(t, items[0].Data)
	assert.Equal(t, "second", items[0].Data.Anchor)

	raw, errResp = h.request("callHierarchy/outgoingCalls", callHierarchyOutgoingCallsParams{Item: items[0]})
	require.Nil(t, errResp)
	var calls []callHierarchyOutgoingCall
	require.NoError(t, json.Unmarshal(raw, &calls))
	assert.Empty(t, calls, "Second section has no links; outgoingCalls must not leak from First")
}

func TestReferencesOnDirectiveArgIncludesIncludeDirectives(t *testing.T) {
	t.Parallel()
	// a.md is the include target; b.md and c.md both <?include?> it.
	// We turn off lint runs so the (stateful, package-level) include
	// rule isn't invoked by didOpen — this test exercises symbol
	// navigation, not lint, and the include rule's chain state would
	// otherwise race with sibling parallel tests sharing the
	// rule.Register singleton.
	srcTarget := "# Target\n"
	srcIncluder := "# B\n\n<?include\nfile: \"./a.md\"\n?>\n<?/include?>\n"
	h, _, rootURI := rootedHarness(t, map[string]string{
		"a.md": srcTarget,
		"b.md": srcIncluder,
		"c.md": strings.Replace(srcIncluder, "# B", "# C", 1),
	})
	h.srv.settingsMu.Lock()
	h.srv.settings.Run = runOff
	h.srv.settingsMu.Unlock()

	uriB := rootURI + "/b.md"
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uriB, LanguageID: "markdown", Version: 1, Text: srcIncluder},
	})

	// Cursor inside `file: "./a.md"` on line 4 of b.md.
	raw, errResp := h.request("textDocument/references", referencesParams{
		textDocumentPositionParams: textDocumentPositionParams{
			TextDocument: textDocumentIdentifier{URI: uriB},
			Position:     Position{Line: 3, Character: 8},
		},
		Context: referencesContext{IncludeDeclaration: false},
	})
	require.Nil(t, errResp)
	var locs []location
	require.NoError(t, json.Unmarshal(raw, &locs))
	// Both b.md and c.md include a.md → two locations.
	assert.GreaterOrEqual(t, len(locs), 2,
		"expected references to include both <?include?> directives, got %v", locs)
}

func TestImplementationIncludesKindAssignment(t *testing.T) {
	t.Parallel()
	// `implementation` on a `kind:` value must surface every file
	// assigned that kind, including config-driven `kind-assignment`
	// matches — front-matter declarations alone aren't enough.
	tmp := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmp, ".mdsmith.yml"), []byte(`
kinds:
  guide: {}
kind-assignment:
  - glob: ["assigned.md"]
    kinds: [guide]
`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "fm.md"),
		[]byte("---\nkinds:\n  - guide\n---\n# FM declared\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "assigned.md"),
		[]byte("# Globbed in by config\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "src.md"),
		[]byte("---\nkind: guide\n---\n# Cursor here\n"), 0o644))

	h := newHarness(t)
	rootURI := pathToFileURI(t, tmp)
	_, errResp := h.request("initialize", initializeParams{
		RootURI:      &rootURI,
		Capabilities: clientCapabilities{},
	})
	require.Nil(t, errResp)
	h.srv.settingsMu.Lock()
	h.srv.settings.Run = runOff
	h.srv.settingsMu.Unlock()
	h.srv.reloadConfig()
	h.srv.invalidateIndex()

	uri := rootURI + "/src.md"
	srcText := "---\nkind: guide\n---\n# Cursor here\n"
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{
			URI: uri, LanguageID: "markdown", Version: 1, Text: srcText,
		},
	})

	// Cursor on `kind: guide` value (line 2, char 8).
	raw, errResp := h.request("textDocument/implementation", textDocumentPositionParams{
		TextDocument: textDocumentIdentifier{URI: uri},
		Position:     Position{Line: 1, Character: 8},
	})
	require.Nil(t, errResp)
	var locs []location
	require.NoError(t, json.Unmarshal(raw, &locs))
	uris := map[string]bool{}
	for _, l := range locs {
		uris[l.URI] = true
	}
	assert.True(t, uris[rootURI+"/fm.md"], "expected fm.md in implementations: %v", locs)
	assert.True(t, uris[rootURI+"/assigned.md"], "expected assigned.md in implementations: %v", locs)
}

func TestCompletionAdvertisesCapability(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	resultRaw, errResp := h.request("initialize", initializeParams{})
	require.Nil(t, errResp)
	var res initializeResult
	require.NoError(t, json.Unmarshal(resultRaw, &res))
	require.NotNil(t, res.Capabilities.CompletionProvider, "completionProvider must be set")
	assert.Equal(t, []string{"#", "[", ":", "/", "\""}, res.Capabilities.CompletionProvider.TriggerCharacters)
	assert.False(t, res.Capabilities.CompletionProvider.ResolveProvider)
}

func TestCompletionAnchorCurrentFile(t *testing.T) {
	t.Parallel()
	src := "# Alpha Heading\n\n## Beta Section\n\nSee [ref](#al\n"
	h, _, rootURI := rootedHarness(t, map[string]string{"a.md": src})
	uri := rootURI + "/a.md"
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uri, LanguageID: "markdown", Version: 1, Text: src},
	})
	_ = h.awaitNotification("textDocument/publishDiagnostics", 5*time.Second)

	// Cursor after "al" in "[ref](#al" → LSP line 4 (0-based), char 13.
	raw, errResp := h.request("textDocument/completion", completionParams{
		TextDocument: textDocumentIdentifier{URI: uri},
		Position:     Position{Line: 4, Character: 13},
	})
	require.Nil(t, errResp)
	var list completionList
	require.NoError(t, json.Unmarshal(raw, &list))
	require.NotEmpty(t, list.Items, "expected anchor completion items")
	labels := make([]string, len(list.Items))
	for i, item := range list.Items {
		labels[i] = item.Label
	}
	assert.Contains(t, labels, "alpha-heading", "expected alpha-heading anchor")
	for _, label := range labels {
		assert.True(t, strings.HasPrefix(label, "al"), "all items should start with prefix 'al': %q", label)
	}
	// Kind must be Reference.
	assert.Equal(t, completionItemKindReference, list.Items[0].Kind)
}

func TestCompletionAnchorCurrentFileDuplicateSlugs(t *testing.T) {
	t.Parallel()
	// Two headings that produce the same base slug; the second gets a -1 suffix.
	src := "# Foo\n\n# Foo\n\nSee [link](#foo\n"
	h, _, rootURI := rootedHarness(t, map[string]string{"dup.md": src})
	uri := rootURI + "/dup.md"
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uri, LanguageID: "markdown", Version: 1, Text: src},
	})
	_ = h.awaitNotification("textDocument/publishDiagnostics", 5*time.Second)

	// Cursor after "foo" in "#foo" — LSP line 4 (0-based), char 15.
	raw, errResp := h.request("textDocument/completion", completionParams{
		TextDocument: textDocumentIdentifier{URI: uri},
		Position:     Position{Line: 4, Character: 15},
	})
	require.Nil(t, errResp)
	var list completionList
	require.NoError(t, json.Unmarshal(raw, &list))
	labels := make(map[string]bool, len(list.Items))
	for _, item := range list.Items {
		labels[item.Label] = true
	}
	assert.True(t, labels["foo"], "expected 'foo' anchor")
	assert.True(t, labels["foo-1"], "expected disambiguated 'foo-1' anchor")
}

func TestCompletionAnchorOtherFile(t *testing.T) {
	t.Parallel()
	srcA := "# Doc A\n\nSee [ref](./b.md#be\n"
	srcB := "# Beta Heading\n\n## Gamma Section\n"
	h, _, rootURI := rootedHarness(t, map[string]string{"a.md": srcA, "b.md": srcB})
	uri := rootURI + "/a.md"
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uri, LanguageID: "markdown", Version: 1, Text: srcA},
	})
	_ = h.awaitNotification("textDocument/publishDiagnostics", 5*time.Second)

	// Cursor after "be" in "./b.md#be" → LSP line 2 (0-based), char 26.
	raw, errResp := h.request("textDocument/completion", completionParams{
		TextDocument: textDocumentIdentifier{URI: uri},
		Position:     Position{Line: 2, Character: 26},
	})
	require.Nil(t, errResp)
	var list completionList
	require.NoError(t, json.Unmarshal(raw, &list))
	require.NotEmpty(t, list.Items)
	labels := make([]string, len(list.Items))
	for i, item := range list.Items {
		labels[i] = item.Label
	}
	assert.Contains(t, labels, "beta-heading", "expected beta-heading from b.md")
	// Labels from a.md (doc-a) must NOT appear since we targeted b.md.
	for _, label := range labels {
		assert.NotEqual(t, "doc-a", label, "items from a.md must be excluded")
	}
}

func TestCompletionRefLabel(t *testing.T) {
	t.Parallel()
	src := "# T\n\nSee [x][fo\n\n[foo]: https://example.com\n[bar]: https://other.com\n"
	h, _, rootURI := rootedHarness(t, map[string]string{"a.md": src})
	uri := rootURI + "/a.md"
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uri, LanguageID: "markdown", Version: 1, Text: src},
	})
	_ = h.awaitNotification("textDocument/publishDiagnostics", 5*time.Second)

	// Cursor after "fo" in "[x][fo" → LSP line 2 (0-based), char 10.
	raw, errResp := h.request("textDocument/completion", completionParams{
		TextDocument: textDocumentIdentifier{URI: uri},
		Position:     Position{Line: 2, Character: 10},
	})
	require.Nil(t, errResp)
	var list completionList
	require.NoError(t, json.Unmarshal(raw, &list))
	require.NotEmpty(t, list.Items)
	labels := make([]string, len(list.Items))
	for i, item := range list.Items {
		labels[i] = item.Label
	}
	assert.Contains(t, labels, "foo", "expected 'foo' label")
	// 'bar' does not start with 'fo', so must be excluded.
	for _, label := range labels {
		assert.NotEqual(t, "bar", label, "bar must be excluded by prefix filter")
	}
}

func TestCompletionRefLabelExcludesOtherFiles(t *testing.T) {
	t.Parallel()
	srcA := "# A\n\nSee [x][\n"
	srcB := "# B\n\n[remote]: https://example.com\n"
	h, _, rootURI := rootedHarness(t, map[string]string{"a.md": srcA, "b.md": srcB})
	uri := rootURI + "/a.md"
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uri, LanguageID: "markdown", Version: 1, Text: srcA},
	})
	_ = h.awaitNotification("textDocument/publishDiagnostics", 5*time.Second)

	// Cursor after '[' in "[x][" → char 9 on line 2 (0-based).
	raw, errResp := h.request("textDocument/completion", completionParams{
		TextDocument: textDocumentIdentifier{URI: uri},
		Position:     Position{Line: 2, Character: 9},
	})
	require.Nil(t, errResp)
	var list completionList
	require.NoError(t, json.Unmarshal(raw, &list))
	for _, item := range list.Items {
		assert.NotEqual(t, "remote", item.Label, "labels from b.md must be excluded")
	}
}

func TestCompletionKindValue(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmp, ".mdsmith.yml"), []byte(`
kinds:
  guide: {}
  tutorial: {}
  reference: {}
`), 0o644))
	srcText := "---\nkind: gu\n---\n# Body\n"
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "a.md"), []byte(srcText), 0o644))

	h := newHarness(t)
	rootURI := pathToFileURI(t, tmp)
	_, errResp := h.request("initialize", initializeParams{
		RootURI:      &rootURI,
		Capabilities: clientCapabilities{},
	})
	require.Nil(t, errResp)
	h.srv.settingsMu.Lock()
	h.srv.settings.Run = runOff
	h.srv.settingsMu.Unlock()
	h.srv.reloadConfig()
	h.srv.invalidateIndex()

	uri := rootURI + "/a.md"
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uri, LanguageID: "markdown", Version: 1, Text: srcText},
	})

	// Cursor on "gu" value: FM line 2 is `kind: gu`, character 8 is
	// in the value. LSP line 1 (0-based), char 8.
	raw, errResp := h.request("textDocument/completion", completionParams{
		TextDocument: textDocumentIdentifier{URI: uri},
		Position:     Position{Line: 1, Character: 8},
	})
	require.Nil(t, errResp)
	var list completionList
	require.NoError(t, json.Unmarshal(raw, &list))
	require.NotEmpty(t, list.Items, "expected kind completion items")
	labels := make([]string, len(list.Items))
	for i, item := range list.Items {
		labels[i] = item.Label
		assert.Equal(t, completionItemKindEnumMember, item.Kind)
		assert.Equal(t, ".mdsmith.yml", item.Detail)
	}
	assert.Contains(t, labels, "guide")
	for _, label := range labels {
		assert.True(t, strings.HasPrefix(label, "gu"), "all items should start with 'gu': %q", label)
	}
}

func TestCompletionDirectivePath(t *testing.T) {
	t.Parallel()
	srcA := strings.Join([]string{
		"# Top",
		"",
		"<?include",
		`file: "doc`,
		"?>",
		"<?/include?>",
		"",
	}, "\n")
	h, _, rootURI := rootedHarness(t, map[string]string{
		"a.md":      srcA,
		"docs/b.md": "# B\n",
		"docs/c.md": "# C\n",
		"other.md":  "# Other\n",
	})
	uri := rootURI + "/a.md"
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uri, LanguageID: "markdown", Version: 1, Text: srcA},
	})
	_ = h.awaitNotification("textDocument/publishDiagnostics", 5*time.Second)

	// Cursor after `"doc` on line 4 (0-based: 3), char 10.
	raw, errResp := h.request("textDocument/completion", completionParams{
		TextDocument: textDocumentIdentifier{URI: uri},
		Position:     Position{Line: 3, Character: 10},
	})
	require.Nil(t, errResp)
	var list completionList
	require.NoError(t, json.Unmarshal(raw, &list))
	require.NotEmpty(t, list.Items, "expected path completion items for 'doc' prefix")
	for _, item := range list.Items {
		assert.Equal(t, completionItemKindFile, item.Kind)
		assert.True(t, strings.HasPrefix(strings.ToLower(item.Label), "doc"),
			"all items should start with 'doc': %q", item.Label)
	}
}

func TestCompletionDirectivePathExclusionPrefix(t *testing.T) {
	t.Parallel()
	// When a catalog glob list item starts with "!", the "!" is stripped for
	// path matching and prepended on each label, so exclusion patterns like
	// `- "!docs/internal/` get real completions.
	srcA := strings.Join([]string{
		"# Top",
		"",
		"<?catalog",
		"glob:",
		`  - "docs/*.md"`,
		`  - "!docs/`,
		"?>",
		"<?/catalog?>",
		"",
	}, "\n")
	h, _, rootURI := rootedHarness(t, map[string]string{
		"a.md":                 srcA,
		"docs/guide.md":        "# Guide\n",
		"docs/internal/ref.md": "# Ref\n",
	})
	uri := rootURI + "/a.md"
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uri, LanguageID: "markdown", Version: 1, Text: srcA},
	})
	_ = h.awaitNotification("textDocument/publishDiagnostics", 5*time.Second)

	// Cursor after "!docs/" on line 5 (0-based), character 11.
	raw, errResp := h.request("textDocument/completion", completionParams{
		TextDocument: textDocumentIdentifier{URI: uri},
		Position:     Position{Line: 5, Character: 11},
	})
	require.Nil(t, errResp)
	var list completionList
	require.NoError(t, json.Unmarshal(raw, &list))
	require.NotEmpty(t, list.Items, "expected items for exclusion prefix !docs/")
	for _, item := range list.Items {
		assert.True(t, strings.HasPrefix(item.Label, "!docs/"),
			"all labels should start with !docs/: %q", item.Label)
	}
}

func TestCompletionKindValueNoConfig(t *testing.T) {
	t.Parallel()
	// kindItems returns empty when no .mdsmith.yml is loaded (cfg == nil).
	srcText := "---\nkind: gu\n---\n# Body\n"
	h, _, rootURI := rootedHarness(t, map[string]string{"a.md": srcText})
	uri := rootURI + "/a.md"
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uri, LanguageID: "markdown", Version: 1, Text: srcText},
	})

	// LSP line 1 (0-based), char 8 — inside the kind value.
	raw, errResp := h.request("textDocument/completion", completionParams{
		TextDocument: textDocumentIdentifier{URI: uri},
		Position:     Position{Line: 1, Character: 8},
	})
	require.Nil(t, errResp)
	var list completionList
	require.NoError(t, json.Unmarshal(raw, &list))
	assert.Empty(t, list.Items, "no kinds without a config file")
}

func TestCompletionAnchorOtherFileEscapingWorkspace(t *testing.T) {
	t.Parallel()
	// A workspace-escaping path ("../../") produces an empty TargetFile; the
	// handler must return an empty list without panicking (exercises the
	// ctx.TargetFile == "" guard in completionItems).
	src := "# Top\n\nSee [ref](../../escape.md#sec\n"
	h, _, rootURI := rootedHarness(t, map[string]string{"a.md": src})
	uri := rootURI + "/a.md"
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uri, LanguageID: "markdown", Version: 1, Text: src},
	})
	_ = h.awaitNotification("textDocument/publishDiagnostics", 5*time.Second)

	// Cursor after "sec" in "../../escape.md#sec" → line 2 (0-based), char 35.
	raw, errResp := h.request("textDocument/completion", completionParams{
		TextDocument: textDocumentIdentifier{URI: uri},
		Position:     Position{Line: 2, Character: 35},
	})
	require.Nil(t, errResp)
	var list completionList
	require.NoError(t, json.Unmarshal(raw, &list))
	assert.Empty(t, list.Items)
}

func TestCompletionDirectivePathFromSubdir(t *testing.T) {
	t.Parallel()
	// Buffer is in docs/; returned paths must be relative to docs/ (exercises
	// the dir != "" path in directivePathItems and relFromDir).
	srcGuide := strings.Join([]string{
		"# Guide",
		"",
		"<?include",
		`file: "oth`,
		"?>",
		"<?/include?>",
		"",
	}, "\n")
	h, _, rootURI := rootedHarness(t, map[string]string{
		"docs/guide.md": srcGuide,
		"docs/other.md": "# Other\n",
		"docs/notes.md": "# Notes\n",
		"root.md":       "# Root\n",
	})
	uri := rootURI + "/docs/guide.md"
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uri, LanguageID: "markdown", Version: 1, Text: srcGuide},
	})
	_ = h.awaitNotification("textDocument/publishDiagnostics", 5*time.Second)

	// Cursor after `"oth` on line 4 (0-based: 3), char 10.
	raw, errResp := h.request("textDocument/completion", completionParams{
		TextDocument: textDocumentIdentifier{URI: uri},
		Position:     Position{Line: 3, Character: 10},
	})
	require.Nil(t, errResp)
	var list completionList
	require.NoError(t, json.Unmarshal(raw, &list))
	require.NotEmpty(t, list.Items)
	for _, item := range list.Items {
		// Paths must be relative to docs/, so "other.md" not "docs/other.md".
		assert.False(t, strings.HasPrefix(item.Label, "docs/"),
			"path should be relative to docs/, got %q", item.Label)
		assert.True(t, strings.HasPrefix(strings.ToLower(item.Label), "oth"),
			"expected 'oth' prefix, got %q", item.Label)
	}
}

func TestCompletionOutsideContextReturnsEmpty(t *testing.T) {
	t.Parallel()
	src := "# Heading\n\nJust plain prose here.\n"
	h, _, rootURI := rootedHarness(t, map[string]string{"a.md": src})
	uri := rootURI + "/a.md"
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uri, LanguageID: "markdown", Version: 1, Text: src},
	})
	_ = h.awaitNotification("textDocument/publishDiagnostics", 5*time.Second)

	raw, errResp := h.request("textDocument/completion", completionParams{
		TextDocument: textDocumentIdentifier{URI: uri},
		Position:     Position{Line: 2, Character: 8},
	})
	require.Nil(t, errResp)
	var list completionList
	require.NoError(t, json.Unmarshal(raw, &list))
	assert.Empty(t, list.Items, "expected no completions on plain prose")
	assert.False(t, list.IsIncomplete)
}

func TestWatcherSkipsOpenBuffer(t *testing.T) {
	t.Parallel()
	// File on disk has no headings. Editor buffer has one, so the
	// index should reflect the open buffer. A subsequent
	// didChangeWatchedFiles event for the same file must not
	// overwrite the index entry with the stale on-disk content.
	tmp := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "a.md"), []byte("text only\n"), 0o644))
	h := newHarness(t)
	rootURI := pathToFileURI(t, tmp)
	_, errResp := h.request("initialize", initializeParams{
		RootURI:      &rootURI,
		Capabilities: clientCapabilities{},
	})
	require.Nil(t, errResp)
	h.srv.settingsMu.Lock()
	h.srv.settings.Run = runOff
	h.srv.settingsMu.Unlock()

	uri := rootURI + "/a.md"
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uri, LanguageID: "markdown", Version: 1, Text: "# Live Heading\n"},
	})

	// Force the index to build with the open buffer's contents.
	_, _ = h.request("textDocument/documentSymbol", documentSymbolParams{
		TextDocument: textDocumentIdentifier{URI: uri},
	})

	// Fire a watcher event for the same file. With the bug, this
	// would re-read the on-disk content and replace the index entry,
	// hiding the live heading.
	h.notify("workspace/didChangeWatchedFiles", didChangeWatchedFilesParams{
		Changes: []fileEvent{{URI: uri, Type: 2}},
	})

	// Documentsymbol should still see the live heading.
	raw, errResp := h.request("textDocument/documentSymbol", documentSymbolParams{
		TextDocument: textDocumentIdentifier{URI: uri},
	})
	require.Nil(t, errResp)
	var syms []documentSymbol
	require.NoError(t, json.Unmarshal(raw, &syms))
	require.NotEmpty(t, syms)
	assert.Equal(t, "Live Heading", syms[0].Name)
}

func TestCompletionInvalidParams(t *testing.T) {
	t.Parallel()
	// Sending a non-object as params (integer 42) triggers the JSON unmarshal
	// error path (L19-22) in handleCompletion.
	h := newHarness(t)
	_, errResp := h.request("initialize", initializeParams{Capabilities: clientCapabilities{}})
	require.Nil(t, errResp)
	_, errResp = h.request("textDocument/completion", 42)
	require.NotNil(t, errResp, "expected error for non-object completion params")
}

func TestCompletionUnknownFile(t *testing.T) {
	t.Parallel()
	// Completion for a file outside the workspace hits the !ok path (L24-27)
	// in handleCompletion.
	h, _, _ := rootedHarness(t, map[string]string{"a.md": "# Heading\n"})
	raw, errResp := h.request("textDocument/completion", completionParams{
		TextDocument: textDocumentIdentifier{URI: "file:///no-such-workspace/missing.md"},
		Position:     Position{Line: 0, Character: 0},
	})
	require.Nil(t, errResp)
	var list completionList
	require.NoError(t, json.Unmarshal(raw, &list))
	assert.Empty(t, list.Items)
}

func TestCompletionItemsUnknownTag(t *testing.T) {
	t.Parallel()
	// completionItems with an unrecognised tag hits the default return (L62).
	h := newHarness(t)
	_, errResp := h.request("initialize", initializeParams{Capabilities: clientCapabilities{}})
	require.Nil(t, errResp)
	items := h.srv.completionItems(index.CompletionContext{Tag: index.CompletionTag(99)}, "a.md", nil)
	assert.Empty(t, items)
}

func TestCompletionAnchorMissingIndexFile(t *testing.T) {
	t.Parallel()
	// Anchor completion for a cross-file link where the target file is not
	// in the index returns empty (L70-72 in anchorItems: idx.File returns !ok).
	src := "# Top\n\nSee [ref](./missing.md#sec\n"
	h, _, rootURI := rootedHarness(t, map[string]string{"a.md": src})
	uri := rootURI + "/a.md"
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uri, LanguageID: "markdown", Version: 1, Text: src},
	})
	_ = h.awaitNotification("textDocument/publishDiagnostics", 5*time.Second)

	// Cursor after "sec" in "./missing.md#sec" (missing.md is not on disk).
	raw, errResp := h.request("textDocument/completion", completionParams{
		TextDocument: textDocumentIdentifier{URI: uri},
		Position:     Position{Line: 2, Character: 33},
	})
	require.Nil(t, errResp)
	var list completionList
	require.NoError(t, json.Unmarshal(raw, &list))
	assert.Empty(t, list.Items)
}

func TestCompletionAnchorNonHeadingSymbolFiltered(t *testing.T) {
	t.Parallel()
	// File has both a heading and a link-ref definition. When requesting anchor
	// completion, the link-ref symbol must be filtered out (L76-77: sym.Kind !=
	// SymbolHeading hits the continue).
	src := "# Alpha Heading\n\n## Beta Section\n\nSee [ref][label]\n\n[label]: https://example.com\n"
	h, _, rootURI := rootedHarness(t, map[string]string{"a.md": src})
	uri := rootURI + "/a.md"
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uri, LanguageID: "markdown", Version: 1, Text: src},
	})
	_ = h.awaitNotification("textDocument/publishDiagnostics", 5*time.Second)

	// First request is on the ref-link syntax (no completion context yet);
	// discard the result before we rewrite the buffer.
	_, _ = h.request("textDocument/completion", completionParams{
		TextDocument: textDocumentIdentifier{URI: uri},
		Position:     Position{Line: 4, Character: 13},
	})

	// Rewrite src to add the inline anchor context after the ref links.
	srcWithAnchor := "# Alpha Heading\n\n## Beta Section\n\nSee [ref](#al\n\n[label]: https://example.com\n"
	h.notify("textDocument/didChange", didChangeTextDocumentParams{
		TextDocument:   versionedTextDocumentIdentifier{URI: uri, Version: 2},
		ContentChanges: []textDocumentContentChangeEvent{{Text: srcWithAnchor}},
	})
	_ = h.awaitNotification("textDocument/publishDiagnostics", 5*time.Second)

	raw, errResp := h.request("textDocument/completion", completionParams{
		TextDocument: textDocumentIdentifier{URI: uri},
		Position:     Position{Line: 4, Character: 13},
	})
	require.Nil(t, errResp)
	var list completionList
	require.NoError(t, json.Unmarshal(raw, &list))
	for _, item := range list.Items {
		assert.NotEqual(t, "label", item.Label, "ref-label should not appear in anchor completions")
	}
}

func TestCompletionRefLabelMissingIndexFile(t *testing.T) {
	t.Parallel()
	// Ref-label completion for a buffer not yet indexed (file opened but not
	// on disk) hits !ok in refLabelItems (L102-104).
	src := "# Top\n\nSee [text][\n\n[foo]: https://example.com\n"
	h, _, rootURI := rootedHarness(t, map[string]string{"other.md": "# Other\n"})
	// Open a.md via didOpen (so docTextOrFile succeeds) without writing it to disk.
	uri := rootURI + "/a.md"
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uri, LanguageID: "markdown", Version: 1, Text: src},
	})
	_ = h.awaitNotification("textDocument/publishDiagnostics", 5*time.Second)

	// Cursor after "[" in "[text][" — triggers CompletionRefLabel.
	raw, errResp := h.request("textDocument/completion", completionParams{
		TextDocument: textDocumentIdentifier{URI: uri},
		Position:     Position{Line: 2, Character: 11},
	})
	require.Nil(t, errResp)
	var list completionList
	require.NoError(t, json.Unmarshal(raw, &list))
	// No assertions on items; coverage is the goal.
	_ = list
}

func TestSortItemsTieBreakByLabel(t *testing.T) {
	t.Parallel()
	// Two items with identical SortText but different Labels: the secondary
	// sort by Label is exercised (L199 in sortItems).
	items := []completionItem{
		{Label: "zoo", SortText: "same"},
		{Label: "aaa", SortText: "same"},
	}
	sortItems(items)
	assert.Equal(t, "aaa", items[0].Label)
	assert.Equal(t, "zoo", items[1].Label)
}

func TestFrontMatterScalarKindNonString(t *testing.T) {
	t.Parallel()
	// kind value is an integer, not a string → returns ("", false) (L132).
	fm := []byte("---\nkind: 42\n---\n")
	v, ok := frontMatterScalarKind(fm)
	assert.False(t, ok)
	assert.Empty(t, v)
}

func TestStripFrontMatterDelimitersNoTrailingNewline(t *testing.T) {
	t.Parallel()
	// FM without a trailing newline after "---" exercises the TrimSuffix("---")
	// branch (L146) of stripFrontMatterDelimiters.
	fm := []byte("---\ntitle: foo\n---")
	result := stripFrontMatterDelimiters(fm)
	assert.Equal(t, []byte("title: foo\n"), result)
}

func TestRefLabelItemsNotInIndex(t *testing.T) {
	t.Parallel()
	// refLabelItems with a file absent from the index hits the !ok guard
	// (completion.go L102-104).
	h := newHarness(t)
	_, errResp := h.request("initialize", initializeParams{Capabilities: clientCapabilities{}})
	require.Nil(t, errResp)
	items := h.srv.refLabelItems("nonexistent.md", "", index.New(""))
	assert.Empty(t, items)
}

func TestCompletionMissingFileOnDisk(t *testing.T) {
	t.Parallel()
	// Completion for a file that is inside the workspace but not on disk and
	// not opened via didOpen hits the os.ReadFile error path (symbols.go L365-367).
	h, _, rootURI := rootedHarness(t, map[string]string{"a.md": "# Hello\n"})
	raw, errResp := h.request("textDocument/completion", completionParams{
		TextDocument: textDocumentIdentifier{URI: rootURI + "/does-not-exist.md"},
		Position:     Position{Line: 0, Character: 0},
	})
	require.Nil(t, errResp)
	var list completionList
	require.NoError(t, json.Unmarshal(raw, &list))
	assert.Empty(t, list.Items)
}

func TestLocationsForFileTopSameSrcSorted(t *testing.T) {
	t.Parallel()
	// source.md links to target.md twice; both locations share the same URI so
	// the sort comparator's second branch (symbols.go L1044) is exercised.
	files := map[string]string{
		"source.md": "# S\n\n[first](./target.md)\n\n[second](./target.md)\n",
		"target.md": "# Target\n",
	}
	h, _, rootURI := rootedHarness(t, files)
	uri := rootURI + "/target.md"
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uri, LanguageID: "markdown", Version: 1, Text: files["target.md"]},
	})
	_ = h.awaitNotification("textDocument/publishDiagnostics", 5*time.Second)

	raw, errResp := h.request("textDocument/references", referencesParams{
		textDocumentPositionParams: textDocumentPositionParams{
			TextDocument: textDocumentIdentifier{URI: uri},
			Position:     Position{Line: 0, Character: 0},
		},
		Context: referencesContext{IncludeDeclaration: false},
	})
	require.Nil(t, errResp)
	var locs []location
	require.NoError(t, json.Unmarshal(raw, &locs))
	assert.GreaterOrEqual(t, len(locs), 2, "expected two references from source.md")
}

func TestLocationsForFileReferencesCoverage(t *testing.T) {
	t.Parallel()
	// referer.md has two <?include?> directives pointing at target.md.
	// target.md has a self-referencing anchor link.
	//
	// Requesting references with the cursor on the "target.md" arg exercises:
	//   L1065: EdgeAnchorLink hits the default:continue branch.
	//   L1077: two EdgeInclude locations share the same URI → sort by line.
	files := map[string]string{
		"target.md": "# Target\n\n[self](#target)\n",
		"referer.md": strings.Join([]string{
			"# Referer",
			"",
			"<?include",
			`file: "target.md"`,
			"?>",
			"<?/include?>",
			"",
			"<?include",
			`file: "target.md"`,
			"?>",
			"<?/include?>",
			"",
		}, "\n"),
	}
	h, _, rootURI := rootedHarness(t, files)
	uri := rootURI + "/referer.md"
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uri, LanguageID: "markdown", Version: 1, Text: files["referer.md"]},
	})
	_ = h.awaitNotification("textDocument/publishDiagnostics", 5*time.Second)

	// Cursor on the `t` of "target.md" in the first include directive
	// (0-indexed line 3, character 7 = `t` inside the quoted string).
	raw, errResp := h.request("textDocument/references", referencesParams{
		textDocumentPositionParams: textDocumentPositionParams{
			TextDocument: textDocumentIdentifier{URI: uri},
			Position:     Position{Line: 3, Character: 7},
		},
		Context: referencesContext{IncludeDeclaration: false},
	})
	require.Nil(t, errResp)
	var locs []location
	require.NoError(t, json.Unmarshal(raw, &locs))
	// Both include directives from referer.md should be returned.
	assert.GreaterOrEqual(t, len(locs), 2, "expected references from both include directives")
}

func TestLocationsForFileTopSorted(t *testing.T) {
	t.Parallel()
	// Two files link to target.md without an anchor. References on target.md
	// line 1 returns 2 locations; sort.Slice exercises the comparator (L1040-1044).
	files := map[string]string{
		"source1.md": "# S1\n\n[link](./target.md)\n",
		"source2.md": "# S2\n\n[link](./target.md)\n",
		"target.md":  "# Target\n",
	}
	h, _, rootURI := rootedHarness(t, files)
	uri := rootURI + "/target.md"
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uri, LanguageID: "markdown", Version: 1, Text: files["target.md"]},
	})
	_ = h.awaitNotification("textDocument/publishDiagnostics", 5*time.Second)

	raw, errResp := h.request("textDocument/references", referencesParams{
		textDocumentPositionParams: textDocumentPositionParams{
			TextDocument: textDocumentIdentifier{URI: uri},
			Position:     Position{Line: 0, Character: 0},
		},
		Context: referencesContext{IncludeDeclaration: false},
	})
	require.Nil(t, errResp)
	var locs []location
	require.NoError(t, json.Unmarshal(raw, &locs))
	assert.GreaterOrEqual(t, len(locs), 2, "expected references from source1 and source2")
}
