package lsp

import (
	"context"
	"encoding/json"
	"strings"
	"time"
)

// textDocument/* document-sync handlers — didOpen, didChange, didSave,
// didClose — plus the workspace/didChangeWatchedFiles and
// workspace/didChangeConfiguration events and the session-overlay
// helpers (syncBuffer, dropPath, openDocPaths) they share. Split out of
// server.go so the document-sync dispatch group owns its own file.

func (s *Server) handleDidOpen(ctx context.Context, raw json.RawMessage) {
	var p didOpenTextDocumentParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return
	}
	path := uriToPath(p.TextDocument.URI)
	if path == "" {
		return
	}
	s.docs.set(p.TextDocument.URI, &document{
		uri:     p.TextDocument.URI,
		path:    path,
		text:    []byte(p.TextDocument.Text),
		version: p.TextDocument.Version,
	})
	s.indexUpdate(path, []byte(p.TextDocument.Text))
	// Seed the session overlay with the opened buffer so a cross-file
	// rule in another open document reads its unsaved bytes.
	s.syncBuffer(path, []byte(p.TextDocument.Text))
	// didOpen lints unless run=off — the user wants an initial
	// snapshot when linting is on at all. scheduleLint applies the
	// same off-skip as every other trigger.
	s.scheduleLint(p.TextDocument.URI, lintTriggerOpen)
}

func (s *Server) handleDidChange(ctx context.Context, raw json.RawMessage) {
	var p didChangeTextDocumentParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return
	}
	if len(p.ContentChanges) == 0 {
		return
	}
	doc, ok := s.docs.get(p.TextDocument.URI)
	if !ok {
		return
	}
	last := p.ContentChanges[len(p.ContentChanges)-1]
	doc.text = []byte(last.Text)
	doc.version = p.TextDocument.Version
	s.docs.set(p.TextDocument.URI, doc)
	s.indexUpdate(doc.path, doc.text)
	// Push the edited bytes into the overlay and drop this path's stale
	// read- and parse-cache entries: cross-file rules now see the new
	// buffer, and the edited document re-parses (the version bumped).
	s.syncBuffer(doc.path, doc.text)
	s.scheduleLint(p.TextDocument.URI, lintTriggerChange)
}

// handleDidSave re-lints when the user saves. The onSave run mode
// triggers a lint pass on save, on document open, and on
// config-change events; the only event it skips is didChange. See
// scheduleLint for the full per-trigger / per-mode table.
func (s *Server) handleDidSave(ctx context.Context, raw json.RawMessage) {
	var p struct {
		TextDocument textDocumentIdentifier `json:"textDocument"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return
	}
	if doc, ok := s.docs.get(p.TextDocument.URI); ok {
		// On save the on-disk file matches the buffer, so drop the
		// overlay and caches; cross-file reads fall through to the saved
		// file. The buffer is re-overlaid on the next edit.
		s.dropPath(doc.path)
	}
	s.scheduleLint(p.TextDocument.URI, lintTriggerSave)
}

func (s *Server) handleDidClose(raw json.RawMessage) {
	var p didCloseTextDocumentParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return
	}
	uri := p.TextDocument.URI
	doc, _ := s.docs.get(uri)
	s.docs.delete(uri)
	// Cancel any armed debounce timer so a pending runLint cannot fire
	// and re-publish diagnostics after we clear them below. Collect the
	// timer under the lock, delete the map entry, then call Stop OUTSIDE
	// pendingMu — Stop hits the runtime timer heap and can block under
	// load, and holding pendingMu across it would serialize concurrent
	// scheduleLint callers. The local is named `pending` (not `p`) to
	// avoid shadowing the function parameter holding the LSP params.
	s.pendingMu.Lock()
	var pendingTimer *time.Timer
	if pending, ok := s.pending[uri]; ok {
		pendingTimer = pending.timer
		delete(s.pending, uri)
	}
	s.pendingMu.Unlock()
	if pendingTimer != nil {
		pendingTimer.Stop()
	}
	// Refresh the index from on-disk content so the closed buffer's
	// last-saved state replaces the editor-only edits we accumulated.
	// When the file no longer exists on disk we silently skip — the
	// watcher path will catch the deletion if it lands separately.
	if doc != nil {
		s.indexReloadFromDisk(doc.path)
		// Drop the overlay and per-document parse cache entry: the
		// buffer is gone, so cross-file reads must fall through to the
		// saved file, and a reopen lands at version 1 again.
		s.dropPath(doc.path)
	}
	// Clear cached diagnostics and squiggles on close.
	s.diagsMu.Lock()
	delete(s.diags, uri)
	s.diagsMu.Unlock()
	_ = s.t.writeNotification("textDocument/publishDiagnostics",
		publishDiagnosticsParams{URI: uri, Diagnostics: []Diagnostic{}})
}

func (s *Server) handleDidChangeWatchedFiles(ctx context.Context, raw json.RawMessage) {
	var p didChangeWatchedFilesParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return
	}
	configChanged := false
	mdChanges := make([]string, 0, len(p.Changes))
	for _, c := range p.Changes {
		path := uriToPath(c.URI)
		if strings.HasSuffix(path, ".mdsmith.yml") {
			configChanged = true
			continue
		}
		// Use isMarkdownExt for case-insensitive extension match
		// — the rest of the navigation surface (docTextOrFile,
		// indexReloadFromDisk) treats `.MD` / `.Markdown` as
		// Markdown, and the watcher must agree or a rename to a
		// case-shifted extension would silently stop refreshing
		// the index.
		if isMarkdownExt(path) {
			mdChanges = append(mdChanges, path)
		}
	}
	treeChanged := watchedFilesTreeChanged(p.Changes)
	if configChanged {
		s.reloadConfig()
		// kind / ignore globs may have shifted — drop the index so
		// the next symbol request rebuilds it from scratch.
		s.invalidateIndex()
		for _, uri := range s.docs.openURIs() {
			s.scheduleLint(uri, lintTriggerConfig)
		}
		return
	}
	if treeChanged {
		// File create / delete / rename changes the candidate set
		// the WikilinkIndex keys off, so the next Check must rebuild
		// the index from scratch — otherwise MDS027 would resolve
		// `[[NewPage]]` against the pre-create set and report it
		// missing (or keep resolving `[[OldName]]` after a delete).
		if sess, _ := s.currentSession(); sess != nil {
			sess.InvalidateWikilinks()
		}
	}
	openPaths := s.openDocPaths()
	for _, path := range mdChanges {
		// Skip files the editor currently has open as a buffer: their
		// authoritative content is the overlay buffer (kept current by
		// didOpen/didChange), so an external on-disk edit does not change
		// what we lint, and dropPath would wrongly delete the buffer
		// overlay and make cross-file reads fall through to the stale
		// disk content. Symbol navigation likewise stays on the live
		// buffer.
		if openPaths[path] {
			continue
		}
		// External edit to a file we do not have open: drop its cache
		// entries (it has no overlay) so the next cross-file Check
		// re-reads the changed bytes from disk.
		s.dropPath(path)
		s.indexReloadFromDisk(path)
	}
}

// watchedFilesTreeChanged reports whether a watched-file batch creates
// or deletes any non-config file, which changes the candidate set the
// wikilink index keys off — so the session's wikilink index must rebuild
// on the next Check (`[[NewPage]]` / `![[image.png]]` resolve against any
// extension, so a binary asset add counts too). A pure-change batch (no
// create/delete) leaves the candidate set intact. Per LSP spec:
// 1=Created, 2=Changed, 3=Deleted; a rename arrives as a Deleted+Created
// pair. Pulled out of handleDidChangeWatchedFiles so the decision is
// unit-testable without a live session and its caches.
func watchedFilesTreeChanged(changes []fileEvent) bool {
	for _, c := range changes {
		if strings.HasSuffix(uriToPath(c.URI), ".mdsmith.yml") {
			continue
		}
		if c.Type == fileChangeCreated || c.Type == fileChangeDeleted {
			return true
		}
	}
	return false
}

// syncBuffer pushes an open buffer's current bytes into the session's
// overlay and drops the path's stale cache entries, so the next
// cross-file Check reads the unsaved content and the edited document
// itself re-parses. Called from didOpen (seed the overlay) and
// didChange (refresh it). absPath is the document's absolute filesystem
// path; the session keys the overlay and parse cache by the
// workspace-relative form and the read cache by the absolute form, all
// derived from the relative uri passed here.
func (s *Server) syncBuffer(absPath string, content []byte) {
	sess, _ := s.currentSession()
	if sess == nil || absPath == "" {
		return
	}
	_, _, root := s.snapshotConfig()
	sess.Invalidate(workspaceRelative(root, absPath), content)
}

// dropPath drops the session caches for absPath and removes any overlay
// entry, so the next read falls through to disk. Used by didSave and
// didClose (the buffer's bytes are now the saved file) and by
// didChangeWatchedFiles (an external edit landed on disk underneath us).
// A no-content Invalidate deletes the overlay entry; for a file with no
// overlay (a watched neighbour the editor never opened) it just drops
// the caches.
func (s *Server) dropPath(absPath string) {
	sess, _ := s.currentSession()
	if sess == nil || absPath == "" {
		return
	}
	_, _, root := s.snapshotConfig()
	sess.Invalidate(workspaceRelative(root, absPath))
}

// openDocPaths returns the set of filesystem paths currently held as
// open buffers. The map is keyed by the same absolute path the
// watcher emits so callers can do a direct lookup.
func (s *Server) openDocPaths() map[string]bool {
	out := make(map[string]bool)
	for _, uri := range s.docs.openURIs() {
		if doc, ok := s.docs.get(uri); ok {
			out[doc.path] = true
		}
	}
	return out
}

func (s *Server) handleDidChangeConfiguration(ctx context.Context) {
	// fetchClientSettings reschedules the per-document lint passes
	// after the new settings (and re-discovered config) land, so the
	// republished diagnostics reflect the updated state instead of
	// the stale settings the dispatch goroutine still has cached.
	go s.fetchClientSettings(ctx)
}
