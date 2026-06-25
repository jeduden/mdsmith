package lsp

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jeduden/mdsmith/internal/index"
)

// TestServer_PrepareRenameAt drives prepareRenameAt directly with an
// in-memory server and synthetic source. It covers each dispatch arm:
// heading, ref-def, ref-use, and prose (no renameable symbol).
func TestServer_PrepareRenameAt(t *testing.T) {
	t.Parallel()

	t.Run("heading returns range and placeholder", func(t *testing.T) {
		t.Parallel()
		var buf safeBuffer
		s := New(Options{Reader: nil, Writer: &buf})
		src := []byte("# Hello\n")
		res, ok := s.prepareRenameAt(src, "a.md", Position{Line: 0, Character: 3})
		require.True(t, ok)
		assert.Equal(t, "Hello", res.Placeholder)
	})

	t.Run("refDef returns label range", func(t *testing.T) {
		t.Parallel()
		var buf safeBuffer
		s := New(Options{Reader: nil, Writer: &buf})
		// Line 3 (0-indexed line 2) holds the ref def.
		src := []byte("# T\n\n[docs]: https://example.com\n")
		res, ok := s.prepareRenameAt(src, "a.md", Position{Line: 2, Character: 2})
		require.True(t, ok)
		assert.Equal(t, "docs", res.Placeholder)
	})

	t.Run("refUse returns label range", func(t *testing.T) {
		t.Parallel()
		var buf safeBuffer
		s := New(Options{Reader: nil, Writer: &buf})
		// Character 12 lands inside the trailing [docs] bracket pair.
		src := []byte("See [text][docs] here.\n\n[docs]: x\n")
		res, ok := s.prepareRenameAt(src, "a.md", Position{Line: 0, Character: 12})
		require.True(t, ok)
		assert.Equal(t, "docs", res.Placeholder)
	})

	t.Run("prose returns false", func(t *testing.T) {
		t.Parallel()
		var buf safeBuffer
		s := New(Options{Reader: nil, Writer: &buf})
		src := []byte("plain text here\n")
		_, ok := s.prepareRenameAt(src, "a.md", Position{Line: 0, Character: 3})
		assert.False(t, ok)
	})
}

// awaitRenameResponse drains h.responses until it finds a response
// with the given string id or the timeout elapses.
func awaitRenameResponse(t *testing.T, h *testHarness, id string) parsedResponse {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case resp := <-h.responses:
			if resp.ID == id {
				return resp
			}
		case <-deadline:
			t.Fatalf("timeout waiting for response id=%s", id)
			return parsedResponse{}
		}
	}
}

// TestServer_RenameHeading drives renameHeading directly: happy path
// with a cross-file anchor edge, a collision that must be rejected,
// and a new name that produces an empty slug.
func TestServer_RenameHeading(t *testing.T) {
	t.Parallel()
	t.Run("happy path rewrites cross-file anchor", testRenameHeading_HappyPath)
	t.Run("collision returns InvalidParams", testRenameHeading_Collision)
	t.Run("invalid slug returns InvalidParams", testRenameHeading_InvalidSlug)
}

func testRenameHeading_HappyPath(t *testing.T) {
	t.Parallel()
	srcA := "# Alpha\n\n## Setup\n\nbody\n"
	srcB := "# Beta\n\n[s](./a.md#setup)\n"
	h, _, rootURI := rootedHarness(t, map[string]string{"a.md": srcA, "b.md": srcB})
	uriA := rootURI + "/a.md"
	uriB := rootURI + "/b.md"
	for _, d := range []struct{ uri, src string }{{uriA, srcA}, {uriB, srcB}} {
		h.notify("textDocument/didOpen", didOpenTextDocumentParams{
			TextDocument: textDocumentItem{URI: d.uri, LanguageID: "markdown", Version: 1, Text: d.src},
		})
		_ = h.awaitNotification("textDocument/publishDiagnostics", 5*time.Second)
	}

	srcABytes := []byte(srcA)
	line := 3 // 1-based: "## Setup" is line 3
	res := index.Locator{Path: "a.md"}.Locate(srcABytes, line, 4)
	require.Equal(t, index.TokenHeading, res.Tag, "locator must tag line 3 as heading")

	msg := &requestMessage{ID: json.RawMessage(`77`)}
	p := renameParams{
		TextDocument: textDocumentIdentifier{URI: uriA},
		Position:     Position{Line: 2, Character: 4},
		NewName:      "Configuration",
	}
	h.srv.renameHeading(msg, p, srcABytes, "a.md", line, res, "Configuration")
	resp := awaitRenameResponse(t, h, "77")

	require.Nil(t, resp.Resp.Error)
	var edit workspaceEdit
	require.NoError(t, json.Unmarshal(resp.Resp.Result, &edit))
	require.Contains(t, edit.Changes, uriA, "heading edit must appear in a.md")
	require.Contains(t, edit.Changes, uriB, "anchor edit must appear in b.md")
	assert.Equal(t, "Configuration", edit.Changes[uriA][0].NewText)
}

func testRenameHeading_Collision(t *testing.T) {
	t.Parallel()
	src := "# Top\n\n## Foo\n\n## Bar\n"
	h, _, rootURI := rootedHarness(t, map[string]string{"a.md": src})
	uri := rootURI + "/a.md"
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uri, LanguageID: "markdown", Version: 1, Text: src},
	})
	_ = h.awaitNotification("textDocument/publishDiagnostics", 5*time.Second)

	srcBytes := []byte(src)
	line := 5 // 1-based: "## Bar" is line 5
	res := index.Locator{Path: "a.md"}.Locate(srcBytes, line, 4)
	require.Equal(t, index.TokenHeading, res.Tag)

	msg := &requestMessage{ID: json.RawMessage(`78`)}
	p := renameParams{
		TextDocument: textDocumentIdentifier{URI: uri},
		Position:     Position{Line: 4, Character: 4},
		NewName:      "Foo",
	}
	h.srv.renameHeading(msg, p, srcBytes, "a.md", line, res, "Foo")
	resp := awaitRenameResponse(t, h, "78")

	require.NotNil(t, resp.Resp.Error)
	assert.Equal(t, codeInvalidParams, resp.Resp.Error.Code)
}

func testRenameHeading_InvalidSlug(t *testing.T) {
	t.Parallel()
	src := "# Top\n\n## Setup\n"
	h, _, rootURI := rootedHarness(t, map[string]string{"a.md": src})
	uri := rootURI + "/a.md"
	h.notify("textDocument/didOpen", didOpenTextDocumentParams{
		TextDocument: textDocumentItem{URI: uri, LanguageID: "markdown", Version: 1, Text: src},
	})
	_ = h.awaitNotification("textDocument/publishDiagnostics", 5*time.Second)

	srcBytes := []byte(src)
	line := 3 // 1-based: "## Setup" is line 3
	res := index.Locator{Path: "a.md"}.Locate(srcBytes, line, 4)
	require.Equal(t, index.TokenHeading, res.Tag)

	msg := &requestMessage{ID: json.RawMessage(`79`)}
	p := renameParams{
		TextDocument: textDocumentIdentifier{URI: uri},
		Position:     Position{Line: 2, Character: 4},
		NewName:      "!!!",
	}
	h.srv.renameHeading(msg, p, srcBytes, "a.md", line, res, "!!!")
	resp := awaitRenameResponse(t, h, "79")

	require.NotNil(t, resp.Resp.Error)
	assert.Equal(t, codeInvalidParams, resp.Resp.Error.Code)
}

// TestServer_RenameLinkRef drives renameLinkRef directly: happy path
// (def and use are both rewritten), whitespace-only new label, and a
// new label that collides with an existing def.
func TestServer_RenameLinkRef(t *testing.T) {
	t.Parallel()

	t.Run("happy path rewrites def and use", func(t *testing.T) {
		t.Parallel()
		var buf safeBuffer
		s := New(Options{Reader: nil, Writer: &buf})
		src := []byte("# T\n\nSee [text][docs].\n\n[docs]: https://x\n")
		msg := &requestMessage{ID: json.RawMessage(`1`)}
		p := renameParams{TextDocument: textDocumentIdentifier{URI: "file:///a.md"}}
		s.renameLinkRef(msg, p, src, "docs", "manual")
		out := buf.String()
		assert.Contains(t, out, `"manual"`)
		assert.NotContains(t, out, `"code":-32602`)
	})

	t.Run("empty label returns InvalidParams", func(t *testing.T) {
		t.Parallel()
		var buf safeBuffer
		s := New(Options{Reader: nil, Writer: &buf})
		src := []byte("# T\n\n[docs]: https://x\n")
		msg := &requestMessage{ID: json.RawMessage(`1`)}
		p := renameParams{TextDocument: textDocumentIdentifier{URI: "file:///a.md"}}
		s.renameLinkRef(msg, p, src, "docs", "   ")
		out := buf.String()
		assert.Contains(t, out, `"code":-32602`)
	})

	t.Run("collision returns InvalidParams with conflict name", func(t *testing.T) {
		t.Parallel()
		var buf safeBuffer
		s := New(Options{Reader: nil, Writer: &buf})
		src := []byte("# T\n\n[docs]: https://x\n[manual]: https://y\n")
		msg := &requestMessage{ID: json.RawMessage(`1`)}
		p := renameParams{TextDocument: textDocumentIdentifier{URI: "file:///a.md"}}
		s.renameLinkRef(msg, p, src, "docs", "manual")
		out := buf.String()
		assert.Contains(t, out, `"code":-32602`)
		assert.Contains(t, out, `"conflict":"manual"`)
	})
}

// TestLspRenameWorkspace_Resolve drives lspRenameWorkspace.Resolve
// directly: the open-document buffer path (returns the client URI)
// and the disk-fallback path (returns the canonical workspace URI).
func TestLspRenameWorkspace_Resolve(t *testing.T) {
	t.Parallel()

	t.Run("returns client URI for open buffer", func(t *testing.T) {
		t.Parallel()
		const src = "# open\n"
		h, _, rootURI := rootedHarness(t, map[string]string{"open.md": src})
		uri := rootURI + "/open.md"
		h.notify("textDocument/didOpen", didOpenTextDocumentParams{
			TextDocument: textDocumentItem{URI: uri, LanguageID: "markdown", Version: 1, Text: src},
		})
		_ = h.awaitNotification("textDocument/publishDiagnostics", 5*time.Second)

		ws := lspRenameWorkspace{s: h.srv, idx: h.srv.ensureIndex()}
		gotURI, gotSrc, ok := ws.Resolve("open.md")
		require.True(t, ok)
		assert.Equal(t, uri, gotURI, "expected client URI from open buffer")
		assert.Equal(t, src, string(gotSrc))
	})

	t.Run("falls back to disk for closed file", func(t *testing.T) {
		t.Parallel()
		h, _, rootURI := rootedHarness(t, map[string]string{
			"open.md":   "# open\n",
			"closed.md": "# closed\n",
		})
		openURI := rootURI + "/open.md"
		h.notify("textDocument/didOpen", didOpenTextDocumentParams{
			TextDocument: textDocumentItem{URI: openURI, LanguageID: "markdown", Version: 1, Text: "# open\n"},
		})
		_ = h.awaitNotification("textDocument/publishDiagnostics", 5*time.Second)

		ws := lspRenameWorkspace{s: h.srv, idx: h.srv.ensureIndex()}
		gotURI, gotSrc, ok := ws.Resolve("closed.md")
		require.True(t, ok)
		assert.Equal(t, rootURI+"/closed.md", gotURI, "expected canonical workspace URI for disk fallback")
		assert.Equal(t, "# closed\n", string(gotSrc))
	})
}
