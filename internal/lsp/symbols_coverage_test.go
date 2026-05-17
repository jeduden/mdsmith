package lsp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jeduden/mdsmith/internal/index"
)

// These tests fill out coverage on the LSP navigation handlers'
// edge cases — empty results, malformed params, missing buffers,
// and the helper functions that don't get exercised by the happy-
// path tests in symbols_test.go.

func TestDocumentSymbolEmptyOnUnknownDocument(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	_, errResp := h.request("initialize", initializeParams{})
	require.Nil(t, errResp)
	raw, errResp := h.request("textDocument/documentSymbol", documentSymbolParams{
		TextDocument: textDocumentIdentifier{URI: "file:///does/not/exist.md"},
	})
	require.Nil(t, errResp)
	var syms []documentSymbol
	require.NoError(t, json.Unmarshal(raw, &syms))
	assert.Empty(t, syms)
}

func TestDocumentSymbolMalformedParams(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	_, errResp := h.request("initialize", initializeParams{})
	require.Nil(t, errResp)
	_, errResp = h.request("textDocument/documentSymbol", []int{1, 2, 3})
	require.NotNil(t, errResp)
	assert.Equal(t, codeInvalidParams, errResp.Code)
}

func TestDocumentSymbolAttachesDirectivesUnderHeading(t *testing.T) {
	t.Parallel()
	src := "# Top\n\n## Sub\n\n<?include\nfile: \"x.md\"\n?>\n<?/include?>\n"
	h, _, rootURI := rootedHarness(t, map[string]string{"a.md": src})
	uri := rootURI + "/a.md"
	h.srv.settingsMu.Lock()
	h.srv.settings.Run = runOff
	h.srv.settingsMu.Unlock()
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uri, LanguageID: "markdown", Version: 1, Text: src},
	})
	raw, errResp := h.request("textDocument/documentSymbol", documentSymbolParams{
		TextDocument: textDocumentIdentifier{URI: uri},
	})
	require.Nil(t, errResp)
	var syms []documentSymbol
	require.NoError(t, json.Unmarshal(raw, &syms))
	require.NotEmpty(t, syms)
	require.Equal(t, "Top", syms[0].Name)
	// "Sub" is the only child; the directive lives under "Sub".
	require.Len(t, syms[0].Children, 1)
	sub := syms[0].Children[0]
	assert.Equal(t, "Sub", sub.Name)
	var sawDirective bool
	for _, c := range sub.Children {
		if c.Kind == symbolKindEvent && c.Name == "include" {
			sawDirective = true
		}
	}
	assert.True(t, sawDirective, "expected include directive nested under Sub: %+v", sub)
}

func TestDocumentSymbolHoistsUnattachedDirectives(t *testing.T) {
	t.Parallel()
	// Directive precedes any heading — should hoist to file root,
	// not vanish.
	src := "<?include\nfile: \"x.md\"\n?>\n<?/include?>\n\n# Top\n"
	h, _, rootURI := rootedHarness(t, map[string]string{"a.md": src})
	uri := rootURI + "/a.md"
	h.srv.settingsMu.Lock()
	h.srv.settings.Run = runOff
	h.srv.settingsMu.Unlock()
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uri, LanguageID: "markdown", Version: 1, Text: src},
	})
	raw, errResp := h.request("textDocument/documentSymbol", documentSymbolParams{
		TextDocument: textDocumentIdentifier{URI: uri},
	})
	require.Nil(t, errResp)
	var syms []documentSymbol
	require.NoError(t, json.Unmarshal(raw, &syms))
	var sawHoisted bool
	for _, s := range syms {
		if s.Kind == symbolKindEvent && s.Name == "include" {
			sawHoisted = true
		}
	}
	assert.True(t, sawHoisted, "expected hoisted include at file root: %+v", syms)
}

func TestDefinitionMalformedParams(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	_, errResp := h.request("initialize", initializeParams{})
	require.Nil(t, errResp)
	_, errResp = h.request("textDocument/definition", "garbage")
	require.NotNil(t, errResp)
	assert.Equal(t, codeInvalidParams, errResp.Code)
}

func TestDefinitionMissesOnPlainProse(t *testing.T) {
	t.Parallel()
	src := "# Top\n\nplain prose with no link\n"
	h, _, rootURI := rootedHarness(t, map[string]string{"a.md": src})
	uri := rootURI + "/a.md"
	h.srv.settingsMu.Lock()
	h.srv.settings.Run = runOff
	h.srv.settingsMu.Unlock()
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uri, LanguageID: "markdown", Version: 1, Text: src},
	})
	raw, errResp := h.request("textDocument/definition", textDocumentPositionParams{
		TextDocument: textDocumentIdentifier{URI: uri},
		Position:     Position{Line: 2, Character: 5},
	})
	require.Nil(t, errResp)
	assert.Equal(t, "null", string(raw))
}

func TestDefinitionFromDirectiveArg(t *testing.T) {
	t.Parallel()
	srcA := "# A\n"
	srcB := "# B\n\n<?include\nfile: \"./a.md\"\n?>\n<?/include?>\n"
	h, _, rootURI := rootedHarness(t, map[string]string{"a.md": srcA, "b.md": srcB})
	uriB := rootURI + "/b.md"
	h.srv.settingsMu.Lock()
	h.srv.settings.Run = runOff
	h.srv.settingsMu.Unlock()
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uriB, LanguageID: "markdown", Version: 1, Text: srcB},
	})
	// Cursor inside `file: "./a.md"` on line 4.
	raw, errResp := h.request("textDocument/definition", textDocumentPositionParams{
		TextDocument: textDocumentIdentifier{URI: uriB},
		Position:     Position{Line: 3, Character: 8},
	})
	require.Nil(t, errResp)
	var loc location
	require.NoError(t, json.Unmarshal(raw, &loc))
	assert.Equal(t, rootURI+"/a.md", loc.URI)
}

func TestDefinitionOnFileTopReturnsFile(t *testing.T) {
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
	raw, errResp := h.request("textDocument/definition", textDocumentPositionParams{
		TextDocument: textDocumentIdentifier{URI: uri},
		Position:     Position{Line: 0, Character: 0},
	})
	require.Nil(t, errResp)
	var loc location
	require.NoError(t, json.Unmarshal(raw, &loc))
	assert.Equal(t, uri, loc.URI)
}

func TestDefinitionFileLinkNoAnchor(t *testing.T) {
	t.Parallel()
	srcA := "# A\n"
	srcB := "# B\n\n[a](./a.md)\n"
	h, _, rootURI := rootedHarness(t, map[string]string{"a.md": srcA, "b.md": srcB})
	uri := rootURI + "/b.md"
	h.srv.settingsMu.Lock()
	h.srv.settings.Run = runOff
	h.srv.settingsMu.Unlock()
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uri, LanguageID: "markdown", Version: 1, Text: srcB},
	})
	raw, errResp := h.request("textDocument/definition", textDocumentPositionParams{
		TextDocument: textDocumentIdentifier{URI: uri},
		Position:     Position{Line: 2, Character: 4},
	})
	require.Nil(t, errResp)
	var loc location
	require.NoError(t, json.Unmarshal(raw, &loc))
	assert.Equal(t, rootURI+"/a.md", loc.URI)
}

func TestImplementationOnHeadingIncludesDeclarationAndRefs(t *testing.T) {
	t.Parallel()
	srcA := "# A\n\n## Target\n"
	srcB := "# B\n\n[r](./a.md#target)\n"
	h, _, rootURI := rootedHarness(t, map[string]string{"a.md": srcA, "b.md": srcB})
	uri := rootURI + "/a.md"
	h.srv.settingsMu.Lock()
	h.srv.settings.Run = runOff
	h.srv.settingsMu.Unlock()
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uri, LanguageID: "markdown", Version: 1, Text: srcA},
	})
	// Cursor on `## Target` (line 3).
	raw, errResp := h.request("textDocument/implementation", textDocumentPositionParams{
		TextDocument: textDocumentIdentifier{URI: uri},
		Position:     Position{Line: 2, Character: 4},
	})
	require.Nil(t, errResp)
	var locs []location
	require.NoError(t, json.Unmarshal(raw, &locs))
	// Declaration + the link in b.md.
	assert.GreaterOrEqual(t, len(locs), 2)
}

func TestReferencesOnRefDefIncludesUses(t *testing.T) {
	t.Parallel()
	src := "# T\n\n[See][lab] and [also][lab].\n\n[lab]: https://example.com\n"
	h, _, rootURI := rootedHarness(t, map[string]string{"a.md": src})
	uri := rootURI + "/a.md"
	h.srv.settingsMu.Lock()
	h.srv.settings.Run = runOff
	h.srv.settingsMu.Unlock()
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uri, LanguageID: "markdown", Version: 1, Text: src},
	})
	// Cursor on `[lab]:` on line 5.
	raw, errResp := h.request("textDocument/references", referencesParams{
		textDocumentPositionParams: textDocumentPositionParams{
			TextDocument: textDocumentIdentifier{URI: uri},
			Position:     Position{Line: 4, Character: 2},
		},
		Context: referencesContext{IncludeDeclaration: true},
	})
	require.Nil(t, errResp)
	var locs []location
	require.NoError(t, json.Unmarshal(raw, &locs))
	// Two uses + the declaration.
	assert.GreaterOrEqual(t, len(locs), 3, "got %v", locs)
}

func TestReferencesOnFileTopReturnsLinks(t *testing.T) {
	t.Parallel()
	srcA := "# A\n"
	srcB := "# B\n\n[ref](./a.md)\n"
	h, _, rootURI := rootedHarness(t, map[string]string{"a.md": srcA, "b.md": srcB})
	uri := rootURI + "/a.md"
	h.srv.settingsMu.Lock()
	h.srv.settings.Run = runOff
	h.srv.settingsMu.Unlock()
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uri, LanguageID: "markdown", Version: 1, Text: srcA},
	})
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
	require.Len(t, locs, 1)
	assert.Equal(t, rootURI+"/b.md", locs[0].URI)
}

func TestReferencesMalformedParams(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	_, errResp := h.request("initialize", initializeParams{})
	require.Nil(t, errResp)
	_, errResp = h.request("textDocument/references", "not-an-object")
	require.NotNil(t, errResp)
	assert.Equal(t, codeInvalidParams, errResp.Code)
}

func TestWorkspaceSymbolEmptyForUnknownQuery(t *testing.T) {
	t.Parallel()
	h, _, _ := rootedHarness(t, map[string]string{"a.md": "# A\n"})
	raw, errResp := h.request("workspace/symbol", workspaceSymbolParams{Query: "this-name-does-not-exist"})
	require.Nil(t, errResp)
	var hits []symbolInformation
	require.NoError(t, json.Unmarshal(raw, &hits))
	assert.Empty(t, hits)
}

func TestWorkspaceSymbolMalformedParams(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	_, errResp := h.request("initialize", initializeParams{})
	require.Nil(t, errResp)
	_, errResp = h.request("workspace/symbol", []int{1})
	require.NotNil(t, errResp)
}

func TestPrepareCallHierarchyOnHeading(t *testing.T) {
	t.Parallel()
	src := "# Top\n\n## Sub\n\nbody\n"
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
		Position:     Position{Line: 2, Character: 3},
	})
	require.Nil(t, errResp)
	var items []callHierarchyItem
	require.NoError(t, json.Unmarshal(raw, &items))
	require.Len(t, items, 1)
	assert.Equal(t, "Sub", items[0].Name)
	require.NotNil(t, items[0].Data)
	assert.Equal(t, "sub", items[0].Data.Anchor)
}

func TestPrepareCallHierarchyOnDirectiveArg(t *testing.T) {
	t.Parallel()
	srcA := "# A\n"
	srcB := "# B\n\n<?include\nfile: \"./a.md\"\n?>\n<?/include?>\n"
	h, _, rootURI := rootedHarness(t, map[string]string{"a.md": srcA, "b.md": srcB})
	uri := rootURI + "/b.md"
	h.srv.settingsMu.Lock()
	h.srv.settings.Run = runOff
	h.srv.settingsMu.Unlock()
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uri, LanguageID: "markdown", Version: 1, Text: srcB},
	})
	raw, errResp := h.request("textDocument/prepareCallHierarchy", textDocumentPositionParams{
		TextDocument: textDocumentIdentifier{URI: uri},
		Position:     Position{Line: 3, Character: 8},
	})
	require.Nil(t, errResp)
	var items []callHierarchyItem
	require.NoError(t, json.Unmarshal(raw, &items))
	require.Len(t, items, 1)
	assert.Equal(t, "a.md", items[0].Name)
	require.NotNil(t, items[0].Data)
	assert.Equal(t, "a.md", items[0].Data.File)
}

func TestIncomingCallsEmptyDataReturnsEmpty(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	_, errResp := h.request("initialize", initializeParams{})
	require.Nil(t, errResp)
	raw, errResp := h.request("callHierarchy/incomingCalls", callHierarchyIncomingCallsParams{
		Item: callHierarchyItem{Name: "x"},
	})
	require.Nil(t, errResp)
	var calls []callHierarchyIncomingCall
	require.NoError(t, json.Unmarshal(raw, &calls))
	assert.Empty(t, calls)
}

func TestOutgoingCallsCoalescesByTarget(t *testing.T) {
	t.Parallel()
	srcA := "# A\n\n[one](./b.md)\n[two](./b.md)\n"
	srcB := "# B\n"
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
	require.Len(t, calls, 1, "expected one coalesced target")
	assert.Len(t, calls[0].FromRanges, 2)
}

func TestOutgoingCallsMalformedParams(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	_, errResp := h.request("initialize", initializeParams{})
	require.Nil(t, errResp)
	_, errResp = h.request("callHierarchy/outgoingCalls", "garbage")
	require.NotNil(t, errResp)
}

func TestPathToURIRejectsRelative(t *testing.T) {
	t.Parallel()
	assert.Empty(t, pathToURI(""))
	assert.Empty(t, pathToURI("relative/path"))
	abs, err := filepath.Abs("/tmp/x.md")
	require.NoError(t, err)
	got := pathToURI(abs)
	assert.Contains(t, got, "file://")
}

func TestRangeAtAndForLinesEdgeCases(t *testing.T) {
	t.Parallel()
	src := []byte("foo\nbar\n")
	r := rangeAt(0, 0, src)
	assert.Equal(t, 0, r.Start.Line)
	assert.Equal(t, 0, r.Start.Character)
	// rangeForLines clamps both bounds to the document's line count.
	r = rangeForLines(5, 1, src)
	assert.GreaterOrEqual(t, r.End.Line, r.Start.Line)
}

func TestLineCountVariants(t *testing.T) {
	t.Parallel()
	assert.Equal(t, 1, lineCount(nil))
	assert.Equal(t, 1, lineCount([]byte("hi")))
	assert.Equal(t, 2, lineCount([]byte("hi\nthere")))
	assert.Equal(t, 2, lineCount([]byte("hi\nthere\n")))
}

func TestDefinitionFileLinkWithAnchor(t *testing.T) {
	t.Parallel()
	srcA := "# A\n\n## Sub\n"
	srcB := "# B\n\n[s](./a.md#sub)\n"
	h, _, rootURI := rootedHarness(t, map[string]string{"a.md": srcA, "b.md": srcB})
	uri := rootURI + "/b.md"
	h.srv.settingsMu.Lock()
	h.srv.settings.Run = runOff
	h.srv.settingsMu.Unlock()
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uri, LanguageID: "markdown", Version: 1, Text: srcB},
	})
	raw, errResp := h.request("textDocument/definition", textDocumentPositionParams{
		TextDocument: textDocumentIdentifier{URI: uri},
		Position:     Position{Line: 2, Character: 4},
	})
	require.Nil(t, errResp)
	var loc location
	require.NoError(t, json.Unmarshal(raw, &loc))
	assert.Equal(t, rootURI+"/a.md", loc.URI)
	// "## Sub" is line 3 → LSP line 2.
	assert.Equal(t, 2, loc.Range.Start.Line)
}

func TestDefinitionAnchorLinkUnknownAnchor(t *testing.T) {
	t.Parallel()
	src := "# T\n\n[broken](#missing)\n"
	h, _, rootURI := rootedHarness(t, map[string]string{"a.md": src})
	uri := rootURI + "/a.md"
	h.srv.settingsMu.Lock()
	h.srv.settings.Run = runOff
	h.srv.settingsMu.Unlock()
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uri, LanguageID: "markdown", Version: 1, Text: src},
	})
	raw, errResp := h.request("textDocument/definition", textDocumentPositionParams{
		TextDocument: textDocumentIdentifier{URI: uri},
		Position:     Position{Line: 2, Character: 4},
	})
	require.Nil(t, errResp)
	assert.Equal(t, "null", string(raw))
}

func TestImplementationMalformedParams(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	_, errResp := h.request("initialize", initializeParams{})
	require.Nil(t, errResp)
	_, errResp = h.request("textDocument/implementation", "garbage")
	require.NotNil(t, errResp)
}

func TestPrepareCallHierarchyMalformedParams(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	_, errResp := h.request("initialize", initializeParams{})
	require.Nil(t, errResp)
	_, errResp = h.request("textDocument/prepareCallHierarchy", "garbage")
	require.NotNil(t, errResp)
}

func TestIncomingCallsMalformedParams(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	_, errResp := h.request("initialize", initializeParams{})
	require.Nil(t, errResp)
	_, errResp = h.request("callHierarchy/incomingCalls", "garbage")
	require.NotNil(t, errResp)
}

func TestOutgoingCallsEmptyData(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	_, errResp := h.request("initialize", initializeParams{})
	require.Nil(t, errResp)
	raw, errResp := h.request("callHierarchy/outgoingCalls", callHierarchyOutgoingCallsParams{
		Item: callHierarchyItem{Name: "x"},
	})
	require.Nil(t, errResp)
	var calls []callHierarchyOutgoingCall
	require.NoError(t, json.Unmarshal(raw, &calls))
	assert.Empty(t, calls)
}

func TestCodeActionStillWorksAfterSymbolRequest(t *testing.T) {
	t.Parallel()
	// Sanity check that the symbol-navigation surface and the
	// pre-existing code-action surface coexist.
	h, _, rootURI := rootedHarness(t, map[string]string{"a.md": "# A\n"})
	uri := rootURI + "/a.md"
	h.srv.settingsMu.Lock()
	h.srv.settings.Run = runOff
	h.srv.settingsMu.Unlock()
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uri, LanguageID: "markdown", Version: 1, Text: "# A\n"},
	})
	_, _ = h.request("textDocument/documentSymbol", documentSymbolParams{
		TextDocument: textDocumentIdentifier{URI: uri},
	})
	raw, errResp := h.request("textDocument/codeAction", codeActionParams{
		TextDocument: textDocumentIdentifier{URI: uri},
		Context:      codeActionContext{Diagnostics: []Diagnostic{}},
	})
	require.Nil(t, errResp)
	var actions []codeAction
	require.NoError(t, json.Unmarshal(raw, &actions))
	// No diagnostics → no quickfixes; optional source.fixAll.
	assert.NotNil(t, actions)
}

func TestHeadingDisplayAndDetailEdgeCases(t *testing.T) {
	t.Parallel()
	// Empty-name heading falls back to the level marker.
	got := headingDisplay(index.Symbol{Level: 2})
	assert.Equal(t, "##", got)
	assert.Empty(t, headingDetail(index.Symbol{}))
	assert.Equal(t, "#anchor", headingDetail(index.Symbol{Anchor: "anchor"}))
	// leafDetail dispatches by kind.
	assert.Equal(t, "<?include?>", leafDetail(index.Symbol{Kind: index.SymbolDirective, Name: "include"}))
	assert.Equal(t, "[label]:", leafDetail(index.Symbol{Kind: index.SymbolLinkRef, Name: "label"}))
	assert.Empty(t, leafDetail(index.Symbol{Kind: index.SymbolFrontMatter}))
}

func TestLocationsForKindDefinitionAbsentKind(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	_, errResp := h.request("initialize", initializeParams{})
	require.Nil(t, errResp)
	// No config → no kind block → empty.
	assert.Empty(t, h.srv.locationsForKindDefinition("nope"))
}

func TestIndexUpdateRunsAfterEnsureIndex(t *testing.T) {
	t.Parallel()
	// Force the index to build, then drive a didChange so
	// indexUpdate's non-nil-index branch runs.
	src := "# Top\n"
	h, _, rootURI := rootedHarness(t, map[string]string{"a.md": src})
	h.srv.settingsMu.Lock()
	h.srv.settings.Run = runOff
	h.srv.settingsMu.Unlock()
	uri := rootURI + "/a.md"
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uri, LanguageID: "markdown", Version: 1, Text: src},
	})
	// Build the index.
	_, _ = h.request("textDocument/documentSymbol", documentSymbolParams{
		TextDocument: textDocumentIdentifier{URI: uri},
	})
	// Now trigger didChange so indexUpdate sees a non-nil idx.
	h.notify("textDocument/didChange", didChangeTextDocumentParams{
		TextDocument: versionedTextDocumentIdentifier{URI: uri, Version: 2},
		ContentChanges: []textDocumentContentChangeEvent{
			{Text: "# Updated\n"},
		},
	})
	// Re-query — outline should reflect the new heading.
	raw, errResp := h.request("textDocument/documentSymbol", documentSymbolParams{
		TextDocument: textDocumentIdentifier{URI: uri},
	})
	require.Nil(t, errResp)
	var syms []documentSymbol
	require.NoError(t, json.Unmarshal(raw, &syms))
	require.NotEmpty(t, syms)
	assert.Equal(t, "Updated", syms[0].Name)
}

func TestIndexReloadFromDiskHandlesMissingFile(t *testing.T) {
	t.Parallel()
	src := "# Top\n"
	h, tmp, _ := rootedHarness(t, map[string]string{"a.md": src})
	h.srv.settingsMu.Lock()
	h.srv.settings.Run = runOff
	h.srv.settingsMu.Unlock()
	// Build the index.
	_, _ = h.request("workspace/symbol", workspaceSymbolParams{Query: "Top"})
	// Delete the file and trigger a watcher reload.
	abs := filepath.Join(tmp, "a.md")
	require.NoError(t, removeFileForTest(abs))
	h.notify("workspace/didChangeWatchedFiles", didChangeWatchedFilesParams{
		Changes: []fileEvent{{URI: "file://" + abs, Type: 3}}, // Deleted
	})
	// Subsequent search no longer finds the file.
	raw, errResp := h.request("workspace/symbol", workspaceSymbolParams{Query: "Top"})
	require.Nil(t, errResp)
	var hits []symbolInformation
	require.NoError(t, json.Unmarshal(raw, &hits))
	assert.Empty(t, hits)
}

// removeFileForTest deletes path; pulled into a helper so the test
// reads top-down without an inline std-lib import.
func removeFileForTest(path string) error {
	return os.Remove(path)
}

func TestLocationsForFileTopFiltersAnchored(t *testing.T) {
	t.Parallel()
	srcA := "# A\n\n## Sec\n"
	// b.md links to a.md#sec (anchored) — locationsForFileTop must
	// skip it because it filters EdgeFileLink with empty anchor only.
	srcB := "# B\n\n[r](./a.md#sec)\n"
	h, _, rootURI := rootedHarness(t, map[string]string{"a.md": srcA, "b.md": srcB})
	uri := rootURI + "/a.md"
	h.srv.settingsMu.Lock()
	h.srv.settings.Run = runOff
	h.srv.settingsMu.Unlock()
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uri, LanguageID: "markdown", Version: 1, Text: srcA},
	})
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
	assert.Empty(t, locs, "anchored links shouldn't match file-top references")
}

func TestLSPPositionToByteColumnAscii(t *testing.T) {
	t.Parallel()
	src := []byte("# Hello\n\nfoo\n")
	// Cursor at UTF-16 char 5 on line 1 → byte column 6 (1-based).
	got := lspPositionToByteColumn(src, 1, 5)
	assert.Equal(t, 6, got)
}

func TestLSPPositionToByteColumnNonASCII(t *testing.T) {
	t.Parallel()
	// "héllo" has `é` taking 2 UTF-8 bytes but 1 UTF-16 unit, so
	// UTF-16 char 3 corresponds to UTF-8 byte 4.
	src := []byte("héllo\n")
	got := lspPositionToByteColumn(src, 1, 3)
	assert.Equal(t, 5, got) // 1-based
}

func TestLSPPositionToByteColumnEdgeCases(t *testing.T) {
	t.Parallel()
	src := []byte("foo\n")
	assert.Equal(t, 1, lspPositionToByteColumn(src, 0, 5))
	assert.Equal(t, 1, lspPositionToByteColumn(src, 1, 0))
	assert.Equal(t, 1, lspPositionToByteColumn(src, 99, 5))
}

func TestByteOffsetFromUTF16(t *testing.T) {
	t.Parallel()
	// Surrogate pair: U+1F600 (emoji 😀) is 4 UTF-8 bytes and 2
	// UTF-16 units. Cursor past the emoji at UTF-16 char 2 maps to
	// byte 4.
	line := []byte("😀x")
	assert.Equal(t, 4, byteOffsetFromUTF16(line, 2))
	// Cursor halfway "into" the surrogate pair clamps to before it.
	assert.Equal(t, 0, byteOffsetFromUTF16(line, 1))
	assert.Equal(t, 0, byteOffsetFromUTF16(line, -1))
	assert.Equal(t, len(line), byteOffsetFromUTF16(line, 1000))
}

func TestPathToURIDriveLetter(t *testing.T) {
	t.Parallel()
	got := pathToURI(`C:\foo\bar.md`)
	assert.Equal(t, "file:///C:/foo/bar.md", got)
}

func TestPathToURIUNC(t *testing.T) {
	t.Parallel()
	got := pathToURI(`\\server\share\x.md`)
	assert.Equal(t, "file://server/share/x.md", got)
}

func TestPathToURIPosix(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "file:///tmp/x.md", pathToURI("/tmp/x.md"))
}

func TestIsWindowsDrivePath(t *testing.T) {
	t.Parallel()
	assert.True(t, isWindowsDrivePath(`C:\foo`))
	assert.True(t, isWindowsDrivePath(`z:relative`))
	assert.False(t, isWindowsDrivePath(""))
	assert.False(t, isWindowsDrivePath("a"))
	assert.False(t, isWindowsDrivePath("/tmp/x"))
	assert.False(t, isWindowsDrivePath("1:foo"))
}
