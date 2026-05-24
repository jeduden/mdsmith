package lsp

import (
	"bytes"
	"encoding/json"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/discovery"
	"github.com/jeduden/mdsmith/internal/index"
	"github.com/jeduden/mdsmith/internal/linkgraph"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/mdtext"
	"github.com/jeduden/mdsmith/internal/yamlutil"
)

// ensureIndex returns the workspace symbol index, building it on
// first call. Build walks the workspace using the same discovery
// patterns the CLI uses; missing roots fall back to an empty index
// so symbol requests are always answerable (just empty).
func (s *Server) ensureIndex() *index.Index {
	s.idxMu.Lock()
	defer s.idxMu.Unlock()
	if s.idx != nil {
		return s.idx
	}
	cfg, _, root := s.snapshotConfig()
	idx := index.New(root)
	if root != "" {
		files, err := discovery.Discover(discovery.Options{
			Patterns:       indexPatterns(),
			BaseDir:        root,
			UseGitignore:   false,
			FollowSymlinks: cfg != nil && cfg.FollowSymlinks,
		})
		if err == nil {
			s.buildIndexFromDisk(idx, cfg, root, filterIgnored(cfg, files))
		}
	}
	// Layer in any open buffers so unsaved edits are visible to
	// symbol queries. The TOCTOU window between openURIs and get
	// is tolerable: a missing doc just means another goroutine
	// closed it, and the index already reflects that via the
	// didClose handler's reload path. Open buffers ignore the
	// ignore list — the user clearly wants this file in scope
	// because they're editing it; matching the lint code-action
	// path (which also runs on ignored buffers when explicitly
	// invoked) keeps the navigation surface consistent.
	for _, uri := range s.docs.openURIs() {
		doc, ok := s.docs.get(uri)
		if !ok {
			continue
		}
		rel := workspaceRelative(root, doc.path)
		idx.UpdateWithKinds(rel, doc.text, effectiveKindsFor(cfg, rel, doc.text))
	}
	s.idx = idx
	return idx
}

// buildIndexFromDisk walks the discovered files and feeds each into
// the index using the resolved effective-kinds list (front matter ∪
// config kind-assignment). Each file is parsed exactly once: we
// UpdateWithKinds directly off the on-disk read instead of calling
// idx.Build (which would re-parse each file when we then layer in
// the config-resolved kinds).
//
// discovery.Discover returns workspace-relative paths, so we join
// root before reading from disk. The relative form is also what
// the index keys on, so we pass it straight to UpdateWithKinds.
func (s *Server) buildIndexFromDisk(idx *index.Index, cfg *config.Config, root string, files []string) {
	for _, rel := range files {
		abs := filepath.Join(root, filepath.FromSlash(rel))
		data, err := os.ReadFile(abs) //nolint:gosec // workspace-rooted, glob-validated
		if err != nil {
			continue
		}
		idx.UpdateWithKinds(rel, data, effectiveKindsFor(cfg, rel, data))
	}
}

// effectiveKindsFor resolves the effective kind list for a file
// given the config and the live source bytes.
//
// Both the scalar `kind: <name>` and the list `kinds: [a, b]`
// front-matter forms are recognized — the scalar form is treated
// as a single-element kinds list. lint.ParseFrontMatterKinds only
// reads the list form; mdsmith's other tooling accepts both, so
// the index has to too or `implementation`/`references` on a
// `kind:` value would silently miss files using the scalar form.
//
// When cfg is nil there are no kind-assignment globs to apply,
// but the file's front-matter kinds are still returned (deduped
// via config.EffectiveKinds) so config-less workspaces still
// pick up scalar / list declarations on the file itself.
func effectiveKindsFor(cfg *config.Config, rel string, source []byte) []string {
	fmBytes, _ := lint.StripFrontMatter(source)
	fmKinds, err := lint.ParseFrontMatterKinds(fmBytes)
	if err != nil {
		fmKinds = nil
	}
	if scalar, ok := frontMatterScalarKind(fmBytes); ok {
		fmKinds = append([]string{scalar}, fmKinds...)
	}
	var fmFields map[string]any
	if config.NeedsFieldsForFile(cfg, rel) {
		fmFields, err = lint.ParseFrontMatterFields(fmBytes)
		if err != nil {
			fmFields = nil
		}
	}
	if len(fmKinds) == 0 && cfg == nil {
		return nil
	}
	return config.EffectiveKinds(cfg, rel, fmKinds, fmFields)
}

// frontMatterScalarKind extracts a scalar `kind: <name>` value
// from front matter, if present. Returns ("", false) when the
// key is absent or the value isn't a scalar.
func frontMatterScalarKind(fm []byte) (string, bool) {
	if len(fm) == 0 {
		return "", false
	}
	var m map[string]any
	if err := yamlutil.UnmarshalSafe(stripFrontMatterDelimiters(fm), &m); err != nil {
		return "", false
	}
	v, ok := m["kind"]
	if !ok {
		return "", false
	}
	if s, ok := v.(string); ok && s != "" {
		return s, true
	}
	return "", false
}

// stripFrontMatterDelimiters removes the leading `---\n` and
// trailing `---\n` (or `---`) from a front-matter prefix as
// returned by lint.StripFrontMatter. Mirrors the helper inside
// internal/index, kept private to avoid leaking the index's
// internal naming.
func stripFrontMatterDelimiters(fm []byte) []byte {
	body := fm
	body = bytes.TrimPrefix(body, []byte("---\n"))
	if t := bytes.TrimSuffix(body, []byte("---\n")); len(t) != len(body) {
		return t
	}
	return bytes.TrimSuffix(body, []byte("---"))
}

// invalidateIndex drops the cached index. The next symbol request
// rebuilds it.
func (s *Server) invalidateIndex() {
	s.idxMu.Lock()
	s.idx = nil
	s.idxMu.Unlock()
}

// indexUpdate refreshes one file in the index. Path is an absolute
// filesystem path or a workspace-relative path; the helper translates.
func (s *Server) indexUpdate(absOrRel string, source []byte) {
	s.idxMu.Lock()
	idx := s.idx
	s.idxMu.Unlock()
	if idx == nil {
		// Index hasn't been built yet — defer until the first
		// symbol request, which will build from disk and pick up
		// open buffers (this one included).
		return
	}
	cfg, _, root := s.snapshotConfig()
	rel := workspaceRelative(root, absOrRel)
	idx.UpdateWithKinds(index.NormalizePath(rel), source, effectiveKindsFor(cfg, rel, source))
}

// indexReloadFromDisk re-reads path from disk and replaces its
// FileEntry. When path no longer exists the entry is removed.
//
// The on-disk read is gated by the same workspace + extension
// rules docTextOrFile applies: the path must resolve inside the
// workspace root (with symlinks resolved) and must be a Markdown
// file. handleDidClose and handleDidChangeWatchedFiles pass
// client-derived paths to this helper, and a malicious client
// could otherwise send events for out-of-workspace files and
// drive arbitrary local reads. Fail closed if either invariant
// is violated.
func (s *Server) indexReloadFromDisk(absOrRel string) {
	s.idxMu.Lock()
	idx := s.idx
	s.idxMu.Unlock()
	if idx == nil {
		return
	}
	cfg, _, root := s.snapshotConfig()
	rel := workspaceRelative(root, absOrRel)
	abs := absOrRel
	if !filepath.IsAbs(abs) && root != "" {
		abs = filepath.Join(root, filepath.FromSlash(rel))
	}
	if !insideWorkspace(root, abs) || !isMarkdownExt(abs) {
		// Drop any stale entry under the workspace-relative form
		// but never read the file from disk.
		idx.Remove(index.NormalizePath(rel))
		return
	}
	data, err := os.ReadFile(abs) //nolint:gosec // workspace-root + extension guarded above
	if err != nil {
		idx.Remove(index.NormalizePath(rel))
		return
	}
	idx.UpdateWithKinds(index.NormalizePath(rel), data, effectiveKindsFor(cfg, rel, data))
}

// indexPatterns returns the glob patterns the workspace index walks.
// The index intentionally uses the built-in defaults rather than the
// project's `files:` configuration: the symbol graph wants every
// Markdown file even if a project narrows its lint scope, so
// cross-file references resolve into linked-but-not-linted files.
// The user's `ignore:` list is still applied via filterIgnored so
// vendored content, fixtures, and generated trees stay out of the
// outline / symbol picker.
func indexPatterns() []string {
	return []string{"**/*.md", "**/*.markdown"}
}

// filterIgnored drops paths matching cfg.Ignore from files. The
// ignore list expresses the user's curated project scope —
// putting `testdata/**` or `vendor/**` there should keep those
// trees out of `documentSymbol` outlines and `workspace/symbol`
// hits. Open buffers bypass this filter (the user editing a file
// always wants it visible).
func filterIgnored(cfg *config.Config, files []string) []string {
	if cfg == nil || len(cfg.Ignore) == 0 {
		return files
	}
	out := files[:0]
	for _, rel := range files {
		if config.IsIgnored(cfg.Ignore, rel) {
			continue
		}
		out = append(out, rel)
	}
	return out
}

// pathToURI returns a `file://` URI for an absolute path. The
// emitted form is RFC 8089-compliant on every platform:
//
//   - POSIX absolute path `/x/y` → `file:///x/y`.
//   - Windows drive-letter path `C:\x\y` → `file:///C:/x/y` (note
//     the three-slash form: empty host + leading slash before the
//     drive letter, which is what `uriToPathOnOS` expects to
//     round-trip).
//   - Windows UNC path `\\server\share\x` → `file://server/share/x`.
//
// Without the explicit drive-letter `/` prefix `url.URL` would emit
// `file://C:/x/y`, which clients parse as host=`C:` and break
// initialize / Location round-tripping.
func pathToURI(p string) string {
	if p == "" {
		return ""
	}
	// Drive-letter and UNC checks run before filepath.IsAbs so the
	// helper produces correct output regardless of the host OS:
	// filepath.IsAbs(`C:\x`) returns false on Linux, which would
	// otherwise reject Windows paths under cross-platform tests
	// and from RPC payloads sent by Windows clients.
	// filepath.ToSlash is OS-specific and a no-op on Linux when the
	// input contains `\`, so Windows-style separators have to be
	// translated explicitly here. forwardSlash gives us a portable
	// version regardless of host OS.
	forwardSlash := strings.ReplaceAll(p, `\`, `/`)
	if isWindowsDrivePath(p) {
		// `C:\x\y` → `/C:/x/y` so url.URL's empty Host stays empty
		// and the drive letter lands in the path component.
		u := url.URL{Scheme: "file", Path: "/" + forwardSlash}
		return u.String()
	}
	if strings.HasPrefix(p, `\\`) {
		// UNC path `\\server\share\x`. The first slash-separated
		// component is the host; the rest is the path.
		rest := strings.TrimPrefix(forwardSlash, "//")
		host, tail, _ := strings.Cut(rest, "/")
		u := url.URL{Scheme: "file", Host: host, Path: "/" + tail}
		return u.String()
	}
	if !filepath.IsAbs(p) {
		// Relative path — caller probably wanted a workspace-
		// relative URI. file:// requires absolute, so emit
		// nothing; the caller can handle the empty.
		return ""
	}
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(p)}
	return u.String()
}

// isWindowsDrivePath reports whether p starts with `X:` where X is
// an ASCII letter — the canonical Windows drive-letter path prefix.
func isWindowsDrivePath(p string) bool {
	if len(p) < 2 || p[1] != ':' {
		return false
	}
	c := p[0]
	return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')
}

// workspaceURI returns a file:// URI for rel, joined against the
// workspace root when one was supplied at initialize. When no
// root is configured the helper still emits a real URI for
// rel inputs that are themselves absolute (POSIX `/`, Windows
// drive `C:`, or UNC `\\server`); a non-absolute rel returns ""
// so the caller drops the location instead of sending an
// invalid URI to the client (LSP requires Location.URI to be a
// real URI, not a bare path).
func (s *Server) workspaceURI(rel string) string {
	_, _, root := s.snapshotConfig()
	if root == "" {
		// No workspace configured. If the caller already has an
		// absolute path (Windows drive / UNC / POSIX root) we can
		// still emit a real file:// URI from it; otherwise we
		// have nothing the LSP spec considers valid for
		// Location.URI, so return "" and let the caller fall
		// through to "no location".
		if filepath.IsAbs(rel) || isWindowsDrivePath(rel) || strings.HasPrefix(rel, `\\`) {
			return pathToURI(rel)
		}
		return ""
	}
	abs := filepath.Join(root, filepath.FromSlash(rel))
	return pathToURI(abs)
}

// docTextOrFile returns the live buffer for uri when the document is
// open; otherwise it reads the on-disk file. Returns the bytes plus
// the workspace-relative path for the document. The returned rel
// is normalized to forward slashes, since `path.Dir` / `path.Join`
// callers in the navigation surface expect forward-slash semantics
// regardless of host OS — `workspaceRelative` returns OS-specific
// separators on Windows, which would mis-resolve directive targets.
//
// When the URI is not already an open buffer, the on-disk read is
// guarded against three concerns: the path must resolve inside the
// configured workspace root, it must have a Markdown extension, and
// the read goes through os.ReadFile only after both checks. Without
// those gates, a client could request `documentSymbol` /
// `definition` for arbitrary local files and exfiltrate their
// outlines through the response.
func (s *Server) docTextOrFile(uri string) ([]byte, string, bool) {
	if doc, ok := s.docs.get(uri); ok {
		_, _, root := s.snapshotConfig()
		rel := index.NormalizePath(workspaceRelative(root, doc.path))
		return doc.text, rel, true
	}
	p := uriToPath(uri)
	if p == "" {
		return nil, "", false
	}
	_, _, root := s.snapshotConfig()
	rel := index.NormalizePath(workspaceRelative(root, p))
	if !insideWorkspace(root, p) {
		return nil, rel, false
	}
	if !isMarkdownExt(p) {
		return nil, rel, false
	}
	data, err := os.ReadFile(p) //nolint:gosec // workspace-root guarded; .md/.markdown only
	if err != nil {
		return nil, rel, false
	}
	return data, rel, true
}

// insideWorkspace reports whether p resolves inside root after
// symlink-resolved path normalization. An empty root fails closed:
// when no workspace was supplied at initialize, on-disk reads must
// be rejected so a client can't drive symbol requests against
// arbitrary local files outside any project.
//
// Both root and p are resolved through filepath.EvalSymlinks
// before the containment check so a markdown symlink inside the
// workspace that points outside the root is rejected. Without
// this, an attacker who could plant a symlink in the project
// could read arbitrary files via symbol requests.
func insideWorkspace(root, p string) bool {
	if root == "" {
		return false
	}
	resolvedRoot := resolveAbsAndSymlinks(root)
	resolvedP := resolveAbsAndSymlinks(p)
	if resolvedRoot == "" || resolvedP == "" {
		return false
	}
	rel, err := filepath.Rel(resolvedRoot, resolvedP)
	if err != nil {
		return false
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}
	return true
}

// resolveAbsAndSymlinks returns p as an absolute, symlink-resolved
// path, falling back to a cleaned absolute form when the target
// doesn't exist (e.g. a path the client supplied for a file that
// hasn't been created yet — still subject to the lexical
// containment check).
func resolveAbsAndSymlinks(p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		return ""
	}
	if real, err := filepath.EvalSymlinks(abs); err == nil {
		return real
	}
	return filepath.Clean(abs)
}

// isMarkdownExt reports whether p has a .md or .markdown extension.
// Case-insensitive.
func isMarkdownExt(p string) bool {
	ext := strings.ToLower(filepath.Ext(p))
	return ext == ".md" || ext == ".markdown"
}

// handleDocumentSymbol returns a hierarchical outline of the buffer.
// Front-matter keys hang off a synthetic top-of-file symbol;
// directives become children of their enclosing heading.
func (s *Server) handleDocumentSymbol(msg *requestMessage) {
	var p documentSymbolParams
	if err := json.Unmarshal(msg.Params, &p); err != nil {
		_ = s.t.writeError(msg.ID, codeInvalidParams, "invalid documentSymbol params")
		return
	}
	source, _, ok := s.docTextOrFile(p.TextDocument.URI)
	if !ok {
		_ = s.t.writeResponse(msg.ID, []documentSymbol{})
		return
	}
	// Outline is built from the live source so unsaved edits are
	// reflected even if the index hasn't been refreshed yet.
	out := buildOutline(source)
	_ = s.t.writeResponse(msg.ID, out)
}

// buildOutline turns a freshly parsed FileEntry into an LSP
// hierarchical outline. Headings stack by level; front-matter keys
// gather under a synthetic top-of-file node; directives attach to
// their enclosing heading or to the file root.
func buildOutline(source []byte) []documentSymbol {
	idx := index.New("")
	idx.Update("buffer", source)
	fe, ok := idx.File("buffer")
	if !ok {
		return nil
	}

	var fmKids []documentSymbol
	var dirRoot []documentSymbol
	var headings []index.Symbol
	for _, sym := range fe.Symbols {
		switch sym.Kind {
		case index.SymbolFrontMatter:
			fmKids = append(fmKids, leafSymbol(sym, source))
		case index.SymbolDirective:
			dirRoot = append(dirRoot, leafSymbol(sym, source))
		case index.SymbolHeading:
			headings = append(headings, sym)
		case index.SymbolLinkRef:
			dirRoot = append(dirRoot, leafSymbol(sym, source))
		}
	}

	var roots []documentSymbol
	if len(fmKids) > 0 {
		// Synthetic "front matter" parent at line 1.
		roots = append(roots, documentSymbol{
			Name:           "front matter",
			Kind:           symbolKindProperty,
			Range:          rangeForLines(1, 1, source),
			SelectionRange: rangeForLines(1, 1, source),
			Children:       fmKids,
		})
	}

	hroots := buildHeadingTree(headings, source)
	// Attach directives + link-refs whose line falls under a heading
	// span; everything else hoists to the file root.
	hroots, unattached := attachDirectives(hroots, dirRoot)
	roots = append(roots, hroots...)
	roots = append(roots, unattached...)
	return roots
}

// buildHeadingTree turns a flat heading list into a nested
// documentSymbol tree using a level-aware stack walk.
func buildHeadingTree(headings []index.Symbol, source []byte) []documentSymbol {
	var roots []documentSymbol
	type stackEntry struct {
		level int
		node  *documentSymbol
	}
	var stack []stackEntry
	for _, h := range headings {
		ds := documentSymbol{
			Name:           headingDisplay(h),
			Detail:         headingDetail(h),
			Kind:           symbolKindString,
			Range:          rangeForLines(h.StartLine, h.EndLine, source),
			SelectionRange: rangeForLines(h.SelectionLine, h.SelectionLine, source),
		}
		// Pop until we find a parent with a lower level.
		for len(stack) > 0 && stack[len(stack)-1].level >= h.Level {
			stack = stack[:len(stack)-1]
		}
		if len(stack) == 0 {
			roots = append(roots, ds)
			stack = append(stack, stackEntry{
				level: h.Level,
				node:  &roots[len(roots)-1],
			})
		} else {
			parent := stack[len(stack)-1].node
			parent.Children = append(parent.Children, ds)
			stack = append(stack, stackEntry{
				level: h.Level,
				node:  &parent.Children[len(parent.Children)-1],
			})
		}
	}
	return roots
}

// attachDirectives walks the heading tree and reparents each
// directive/leaf into the deepest heading whose range covers its
// start line. Leaves that don't fall under any heading return as
// the second value so the caller can hoist them to the file root.
func attachDirectives(headings []documentSymbol, leaves []documentSymbol) ([]documentSymbol, []documentSymbol) {
	var unattached []documentSymbol
	for _, leaf := range leaves {
		startLine := leaf.SelectionRange.Start.Line + 1 // back to 1-based
		if !attachInto(headings, leaf, startLine) {
			unattached = append(unattached, leaf)
		}
	}
	return headings, unattached
}

func attachInto(nodes []documentSymbol, leaf documentSymbol, startLine int) bool {
	for i := range nodes {
		// LSP ranges are [start, end) in 0-based form. The leaf's
		// start line lives inside the node when it falls between
		// the node's Range start and end (inclusive).
		nodeStart := nodes[i].Range.Start.Line + 1
		nodeEnd := nodes[i].Range.End.Line + 1
		if startLine >= nodeStart && startLine <= nodeEnd {
			// Try to attach into a deeper child first.
			if attachInto(nodes[i].Children, leaf, startLine) {
				return true
			}
			nodes[i].Children = append(nodes[i].Children, leaf)
			return true
		}
	}
	return false
}

func leafSymbol(sym index.Symbol, source []byte) documentSymbol {
	kind := symbolKindKey
	switch sym.Kind {
	case index.SymbolFrontMatter:
		kind = symbolKindProperty
	case index.SymbolDirective:
		kind = symbolKindEvent
	case index.SymbolLinkRef:
		kind = symbolKindKey
	}
	return documentSymbol{
		Name:           sym.Name,
		Detail:         leafDetail(sym),
		Kind:           kind,
		Range:          rangeForLines(sym.StartLine, sym.EndLine, source),
		SelectionRange: rangeForLines(sym.SelectionLine, sym.SelectionLine, source),
	}
}

func headingDisplay(h index.Symbol) string {
	if h.Name == "" {
		return strings.Repeat("#", h.Level)
	}
	return h.Name
}

func headingDetail(h index.Symbol) string {
	if h.Anchor == "" {
		return ""
	}
	return "#" + h.Anchor
}

func leafDetail(sym index.Symbol) string {
	switch sym.Kind {
	case index.SymbolDirective:
		return "<?" + sym.Name + "?>"
	case index.SymbolLinkRef:
		return "[" + sym.Name + "]:"
	}
	return ""
}

// rangeForLines returns an LSP Range covering 1-based start..end
// lines inclusive. Columns are 0..end-of-line. Both bounds are
// clamped to the document's line count so the emitted Range stays
// inside the document — LSP requires positions to be within
// bounds, and an out-of-range End.Line causes some clients to
// reject or silently ignore the result.
func rangeForLines(start, end int, source []byte) Range {
	lines := splitLines(source)
	// splitLines guarantees at least one entry (empty input yields
	// a single empty line) so maxLine is always >= 1 and the
	// clamp arithmetic below stays well-defined.
	maxLine := len(lines)
	if start < 1 {
		start = 1
	}
	if start > maxLine {
		start = maxLine
	}
	if end < start {
		end = start
	}
	if end > maxLine {
		end = maxLine
	}
	return Range{
		Start: Position{Line: start - 1, Character: 0},
		End:   Position{Line: end - 1, Character: utf16Length(lines[end-1])},
	}
}

// lspPositionToByteColumn converts an LSP Position.Character
// (UTF-16 code units, 0-based) to a 1-based UTF-8 byte column for
// the given 1-based source line. The Locator works in byte columns
// (so it can index into the parsed AST consistently with the rest
// of mdsmith), but LSP clients send UTF-16; without this
// translation, every cursor on a line containing non-ASCII text
// would mis-locate by the count of multi-byte runes preceding it.
func lspPositionToByteColumn(source []byte, line, utf16Char int) int {
	if line < 1 || utf16Char <= 0 {
		return 1
	}
	lines := splitLines(source)
	if line-1 >= len(lines) {
		return 1
	}
	return byteOffsetFromUTF16(lines[line-1], utf16Char) + 1
}

// rangeAt returns an LSP Range that anchors at (line, col) and
// extends to end-of-line. line and col are 1-based; col is a UTF-8
// byte column. Despite the name, the End is the line's UTF-16
// length so editors can highlight the whole containing line — see
// callers like definition / references / implementation that want
// the editor to flash the matched line, not just the cursor.
func rangeAt(line, col int, source []byte) Range {
	if line < 1 {
		line = 1
	}
	if col < 1 {
		col = 1
	}
	lines := splitLines(source)
	startCh := 0
	endCh := 0
	if line-1 < len(lines) {
		startCh = mdtext.UTF16FromByteOffset(lines[line-1], col-1)
		endCh = utf16Length(lines[line-1])
	}
	return Range{
		Start: Position{Line: line - 1, Character: startCh},
		End:   Position{Line: line - 1, Character: endCh},
	}
}

// handleDefinition resolves textDocument/definition.
func (s *Server) handleDefinition(msg *requestMessage) {
	var p textDocumentPositionParams
	if err := json.Unmarshal(msg.Params, &p); err != nil {
		_ = s.t.writeError(msg.ID, codeInvalidParams, "invalid definition params")
		return
	}
	locs := s.resolveTargets(p, false)
	if len(locs) == 0 {
		_ = s.t.writeResponse(msg.ID, nil)
		return
	}
	_ = s.t.writeResponse(msg.ID, locs[0])
}

// handleImplementation returns every match. For most tags this is the
// same answer as Definition; only `kind:` values and headings (with
// references) produce multi-target sets.
func (s *Server) handleImplementation(msg *requestMessage) {
	var p textDocumentPositionParams
	if err := json.Unmarshal(msg.Params, &p); err != nil {
		_ = s.t.writeError(msg.ID, codeInvalidParams, "invalid implementation params")
		return
	}
	locs := s.resolveTargets(p, true)
	_ = s.t.writeResponse(msg.ID, locs)
}

// resolveTargets is the shared core for definition and implementation.
// When wantAll is false the slice is truncated to the first match.
func (s *Server) resolveTargets(p textDocumentPositionParams, wantAll bool) []location {
	source, rel, ok := s.docTextOrFile(p.TextDocument.URI)
	if !ok {
		return nil
	}
	idx := s.ensureIndex()

	line := p.Position.Line + 1
	col := lspPositionToByteColumn(source, line, p.Position.Character)
	res := index.Locator{Path: rel}.Locate(source, line, col)
	return s.resolveByTag(p, res, line, source, rel, idx, wantAll)
}

// resolveByTag dispatches on the locator's TokenTag and returns the
// matching navigation targets. Split out of resolveTargets so the
// switch stays small enough for funlen.
func (s *Server) resolveByTag(
	p textDocumentPositionParams, res index.LocateResult,
	line int, source []byte, rel string, idx *index.Index, wantAll bool,
) []location {
	switch res.Tag {
	case index.TokenAnchorLink:
		return s.locationsForAnchor(rel, res.TargetAnchor, idx, source)
	case index.TokenFileLink:
		return s.locationsForFileLink(res.TargetFile, res.TargetAnchor, idx)
	case index.TokenRefUse, index.TokenRefDef:
		if loc, ok := s.locationForRefDef(rel, res.Label, source); ok {
			return []location{loc}
		}
	case index.TokenDirectiveArg:
		return s.directiveArgLocations(rel, res.DirectiveTargetFile)
	case index.TokenHeading:
		return s.headingTargets(p, rel, res.Anchor, line, source, idx, wantAll)
	case index.TokenFileTop:
		return []location{{
			URI:   p.TextDocument.URI,
			Range: rangeAt(1, 1, source),
		}}
	case index.TokenFrontMatterValue:
		return s.frontMatterValueTargets(res.FrontMatterKey, res.FrontMatterValue, idx, wantAll)
	}
	return nil
}

func (s *Server) directiveArgLocations(rel, target string) []location {
	if target == "" {
		return nil
	}
	tgt := linkgraph.ResolveRelTarget(rel, target)
	if tgt == "" {
		return nil
	}
	return []location{{
		URI:   s.workspaceURI(tgt),
		Range: Range{Start: Position{Line: 0, Character: 0}, End: Position{Line: 0, Character: 0}},
	}}
}

// headingTargets returns the heading itself for definition, plus
// every link to it for implementation.
func (s *Server) headingTargets(
	p textDocumentPositionParams, rel, anchor string,
	line int, source []byte, idx *index.Index, wantAll bool,
) []location {
	decl := []location{{
		URI:   p.TextDocument.URI,
		Range: rangeAt(line, 1, source),
	}}
	if !wantAll {
		return decl
	}
	return append(decl, s.locationsForRefsToHeading(rel, anchor, idx)...)
}

// frontMatterValueTargets handles the `kind:` / `kinds:` value arm:
// definition resolves to the kind block in `.mdsmith.yml`,
// implementation widens to every file with that kind.
func (s *Server) frontMatterValueTargets(key, val string, idx *index.Index, wantAll bool) []location {
	if key != "kind" && key != "kinds" {
		return nil
	}
	defs := s.locationsForKindDefinition(val)
	if !wantAll {
		return defs
	}
	return append(defs, s.locationsForFilesByKind(val, idx)...)
}

// locationsForAnchor returns the in-file heading targeted by an
// anchor reference. It always returns at most one location — the
// matching heading itself; multi-target widening for headings (the
// implementation behavior) lives in resolveTargets' TokenHeading
// arm, where the declaration is paired with all incoming links.
func (s *Server) locationsForAnchor(rel, anchor string, idx *index.Index, source []byte) []location {
	if anchor == "" {
		return nil
	}
	if fe, ok := idx.File(rel); ok {
		for _, sym := range fe.Symbols {
			if sym.Kind == index.SymbolHeading && sym.Anchor == anchor {
				return []location{{
					URI:   s.workspaceURI(rel),
					Range: rangeAt(sym.SelectionLine, sym.SelectionCol, source),
				}}
			}
		}
	}
	return nil
}

// locationsForFileLink resolves `[text](./other.md#anchor)` to either
// a heading in the target file or the file's first line.
func (s *Server) locationsForFileLink(targetFile, anchor string, idx *index.Index) []location {
	tgt := index.NormalizePath(targetFile)
	if tgt == "" {
		return nil
	}
	if anchor == "" {
		return []location{{
			URI:   s.workspaceURI(tgt),
			Range: Range{Start: Position{Line: 0, Character: 0}, End: Position{Line: 0, Character: 0}},
		}}
	}
	fe, ok := idx.File(tgt)
	if !ok {
		// File lives outside the index (or wasn't loaded yet).
		// Return a best-effort target at line 1.
		return []location{{
			URI:   s.workspaceURI(tgt),
			Range: Range{Start: Position{Line: 0, Character: 0}, End: Position{Line: 0, Character: 0}},
		}}
	}
	for _, sym := range fe.Symbols {
		if sym.Kind == index.SymbolHeading && sym.Anchor == anchor {
			return []location{{
				URI:   s.workspaceURI(tgt),
				Range: rangeAt(sym.SelectionLine, sym.SelectionCol, nil),
			}}
		}
	}
	return []location{{
		URI:   s.workspaceURI(tgt),
		Range: Range{Start: Position{Line: 0, Character: 0}, End: Position{Line: 0, Character: 0}},
	}}
}

// locationForRefDef returns the position of `[label]: …` in the
// current file.
func (s *Server) locationForRefDef(rel, label string, source []byte) (location, bool) {
	idx := s.ensureIndex()
	fe, ok := idx.File(rel)
	if !ok {
		return location{}, false
	}
	for _, sym := range fe.Symbols {
		if sym.Kind == index.SymbolLinkRef && sym.Anchor == label {
			return location{
				URI:   s.workspaceURI(rel),
				Range: rangeAt(sym.SelectionLine, sym.SelectionCol, source),
			}, true
		}
	}
	return location{}, false
}

// locationsForRefsToHeading scans every file's outgoing edges for
// references to (rel, anchor) and returns one location per match.
func (s *Server) locationsForRefsToHeading(rel, anchor string, idx *index.Index) []location {
	if anchor == "" {
		return nil
	}
	edges := idx.IncomingEdges(rel, anchor)
	out := make([]location, 0, len(edges))
	for _, e := range edges {
		out = append(out, location{
			URI:   s.workspaceURI(e.SourceFile),
			Range: rangeAt(e.SourceLine, e.SourceCol, nil),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].URI != out[j].URI {
			return out[i].URI < out[j].URI
		}
		return out[i].Range.Start.Line < out[j].Range.Start.Line
	})
	return out
}

// locationsForKindDefinition reports the location of the kind block
// in `.mdsmith.yml`. We surface the config file at line 1 when the
// kind is declared; absent kinds yield nothing.
func (s *Server) locationsForKindDefinition(kind string) []location {
	cfg, configPath, _ := s.snapshotConfig()
	if cfg == nil || configPath == "" {
		return nil
	}
	if _, ok := cfg.Kinds[kind]; !ok {
		return nil
	}
	return []location{{
		URI:   pathToURI(configPath),
		Range: Range{Start: Position{Line: 0, Character: 0}, End: Position{Line: 0, Character: 0}},
	}}
}

// locationsForFilesByKind returns one Location per workspace file
// whose front-matter `kinds:` includes kind.
func (s *Server) locationsForFilesByKind(kind string, idx *index.Index) []location {
	files := idx.FilesByKind(kind)
	out := make([]location, 0, len(files))
	for _, rel := range files {
		out = append(out, location{
			URI:   s.workspaceURI(rel),
			Range: Range{Start: Position{Line: 0, Character: 0}, End: Position{Line: 0, Character: 0}},
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].URI < out[j].URI })
	return out
}

// handleReferences resolves textDocument/references.
func (s *Server) handleReferences(msg *requestMessage) {
	var p referencesParams
	if err := json.Unmarshal(msg.Params, &p); err != nil {
		_ = s.t.writeError(msg.ID, codeInvalidParams, "invalid references params")
		return
	}
	source, rel, ok := s.docTextOrFile(p.TextDocument.URI)
	if !ok {
		_ = s.t.writeResponse(msg.ID, []location{})
		return
	}
	idx := s.ensureIndex()
	line := p.Position.Line + 1
	col := lspPositionToByteColumn(source, line, p.Position.Character)
	res := index.Locator{Path: rel}.Locate(source, line, col)

	var out []location
	switch res.Tag {
	case index.TokenHeading:
		out = s.locationsForRefsToHeading(rel, res.Anchor, idx)
		if p.Context.IncludeDeclaration {
			out = prependLocation(out, location{
				URI:   p.TextDocument.URI,
				Range: rangeAt(p.Position.Line+1, 1, source),
			})
		}
	case index.TokenRefDef:
		// Every reference-style use of `label` in this file.
		out = s.locationsForRefUses(rel, res.Label, idx)
		if p.Context.IncludeDeclaration {
			if loc, ok := s.locationForRefDef(rel, res.Label, source); ok {
				out = prependLocation(out, loc)
			}
		}
	case index.TokenFileTop:
		// Every link target that names this file with no anchor.
		out = s.locationsForFileTop(rel, idx)
	case index.TokenFrontMatterValue:
		if res.FrontMatterKey == "kind" || res.FrontMatterKey == "kinds" {
			out = s.locationsForFilesByKind(res.FrontMatterValue, idx)
		}
	case index.TokenDirectiveArg:
		// References on a directive argument resolve to "every
		// workspace edge that points at this file" — file links
		// (no anchor) plus every <?include?>, <?build?>, and
		// <?catalog?>. Limiting to EdgeFileLink (the previous
		// behavior) hid the directive-to-directive references that
		// users actually need when navigating include / build chains.
		if res.DirectiveTargetFile != "" {
			if tgt := linkgraph.ResolveRelTarget(rel, res.DirectiveTargetFile); tgt != "" {
				out = s.locationsForFileReferences(tgt, idx)
			}
		}
	}
	if out == nil {
		out = []location{}
	}
	_ = s.t.writeResponse(msg.ID, out)
}

func prependLocation(rest []location, loc location) []location {
	out := make([]location, 0, len(rest)+1)
	out = append(out, loc)
	out = append(out, rest...)
	return out
}

// locationsForRefUses returns every `[text][label]` in rel.
func (s *Server) locationsForRefUses(rel, label string, idx *index.Index) []location {
	fe, ok := idx.File(rel)
	if !ok {
		return nil
	}
	var out []location
	for _, e := range fe.Outgoing {
		if e.Kind != index.EdgeRefLink {
			continue
		}
		if !strings.EqualFold(e.TargetLabel, label) {
			continue
		}
		out = append(out, location{
			URI:   s.workspaceURI(rel),
			Range: rangeAt(e.SourceLine, e.SourceCol, nil),
		})
	}
	return out
}

// locationsForFileTop returns every workspace link whose path
// component points at file (with empty anchor).
func (s *Server) locationsForFileTop(file string, idx *index.Index) []location {
	edges := idx.IncomingEdges(file, "")
	var out []location
	for _, e := range edges {
		if e.Kind != index.EdgeFileLink {
			continue
		}
		if e.TargetAnchor != "" {
			continue
		}
		out = append(out, location{
			URI:   s.workspaceURI(e.SourceFile),
			Range: rangeAt(e.SourceLine, e.SourceCol, nil),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].URI != out[j].URI {
			return out[i].URI < out[j].URI
		}
		return out[i].Range.Start.Line < out[j].Range.Start.Line
	})
	return out
}

// locationsForFileReferences returns every workspace edge whose
// target is file: the union of file-top links and the include /
// build / catalog directives that target this file. Reference-style
// link uses are not included because they target a label, not a
// file path.
func (s *Server) locationsForFileReferences(file string, idx *index.Index) []location {
	edges := idx.IncomingEdges(file, "")
	var out []location
	for _, e := range edges {
		switch e.Kind {
		case index.EdgeFileLink:
			if e.TargetAnchor != "" {
				continue
			}
		case index.EdgeInclude, index.EdgeBuild, index.EdgeCatalog:
			// keep
		default:
			continue
		}
		out = append(out, location{
			URI:   s.workspaceURI(e.SourceFile),
			Range: rangeAt(e.SourceLine, e.SourceCol, nil),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].URI != out[j].URI {
			return out[i].URI < out[j].URI
		}
		return out[i].Range.Start.Line < out[j].Range.Start.Line
	})
	return out
}

// handleWorkspaceSymbol returns SymbolInformation entries for every
// substring match in the workspace index.
func (s *Server) handleWorkspaceSymbol(msg *requestMessage) {
	var p workspaceSymbolParams
	if err := json.Unmarshal(msg.Params, &p); err != nil {
		_ = s.t.writeError(msg.ID, codeInvalidParams, "invalid workspace/symbol params")
		return
	}
	idx := s.ensureIndex()
	hits := idx.SearchSymbols(p.Query, 1024)
	out := make([]symbolInformation, 0, len(hits))
	for _, h := range hits {
		kind := symbolKindString
		switch h.Symbol.Kind {
		case index.SymbolFrontMatter:
			kind = symbolKindProperty
		case index.SymbolLinkRef:
			kind = symbolKindKey
		case index.SymbolDirective:
			kind = symbolKindEvent
		}
		out = append(out, symbolInformation{
			Name: h.Symbol.Name,
			Kind: kind,
			Location: location{
				URI:   s.workspaceURI(h.File),
				Range: rangeAt(h.Symbol.SelectionLine, h.Symbol.SelectionCol, nil),
			},
			ContainerName: h.File,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ContainerName != out[j].ContainerName {
			return out[i].ContainerName < out[j].ContainerName
		}
		return out[i].Name < out[j].Name
	})
	_ = s.t.writeResponse(msg.ID, out)
}

// handlePrepareCallHierarchy returns a single call-hierarchy item
// anchored at (file, optional heading). On a directive arg the item
// is the target file; on a heading line, the heading section.
func (s *Server) handlePrepareCallHierarchy(msg *requestMessage) {
	var p textDocumentPositionParams
	if err := json.Unmarshal(msg.Params, &p); err != nil {
		_ = s.t.writeError(msg.ID, codeInvalidParams, "invalid prepareCallHierarchy params")
		return
	}
	source, rel, ok := s.docTextOrFile(p.TextDocument.URI)
	if !ok {
		_ = s.t.writeResponse(msg.ID, []callHierarchyItem{})
		return
	}
	idx := s.ensureIndex()
	line := p.Position.Line + 1
	col := lspPositionToByteColumn(source, line, p.Position.Character)
	res := index.Locator{Path: rel}.Locate(source, line, col)

	var item callHierarchyItem
	switch res.Tag {
	case index.TokenHeading:
		fe, _ := idx.File(rel)
		item = callHierarchyItem{
			Name:           res.Name,
			Kind:           symbolKindString,
			Detail:         "#" + res.Anchor,
			URI:            p.TextDocument.URI,
			Range:          headingRangeFromIndex(rel, res.Anchor, fe, source),
			SelectionRange: rangeAt(p.Position.Line+1, 1, source),
			Data:           &callHierarchyData{File: rel, Anchor: res.Anchor},
		}
	case index.TokenDirectiveArg:
		if res.DirectiveTargetFile != "" {
			tgt := linkgraph.ResolveRelTarget(rel, res.DirectiveTargetFile)
			if tgt == "" {
				_ = s.t.writeResponse(msg.ID, []callHierarchyItem{})
				return
			}
			item = callHierarchyItem{
				Name:           tgt,
				Kind:           symbolKindString,
				URI:            s.workspaceURI(tgt),
				Range:          Range{Start: Position{Line: 0, Character: 0}, End: Position{Line: 0, Character: 0}},
				SelectionRange: Range{Start: Position{Line: 0, Character: 0}, End: Position{Line: 0, Character: 0}},
				Data:           &callHierarchyData{File: tgt},
			}
		}
	case index.TokenFileTop:
		// File-level call hierarchy: only the very top of the
		// document anchors at the file. Plain prose lower in the
		// file does not — see Plan 131's cursor matrix
		// (file root / heading / directive arg). Without this
		// gate the editor would offer a "call hierarchy" entry
		// for arbitrary positions, including paragraphs that
		// have no inbound or outbound references.
		item = callHierarchyItem{
			Name:           rel,
			Kind:           symbolKindString,
			URI:            p.TextDocument.URI,
			Range:          rangeForLines(1, lineCount(source), source),
			SelectionRange: rangeAt(1, 1, source),
			Data:           &callHierarchyData{File: rel},
		}
	}
	if item.URI == "" {
		_ = s.t.writeResponse(msg.ID, []callHierarchyItem{})
		return
	}
	_ = s.t.writeResponse(msg.ID, []callHierarchyItem{item})
}

func headingRangeFromIndex(rel, anchor string, fe *index.FileEntry, source []byte) Range {
	if fe == nil {
		return rangeAt(1, 1, source)
	}
	for _, sym := range fe.Symbols {
		if sym.Kind == index.SymbolHeading && sym.Anchor == anchor {
			return rangeForLines(sym.StartLine, sym.EndLine, source)
		}
	}
	return rangeAt(1, 1, source)
}

func lineCount(source []byte) int {
	if len(source) == 0 {
		return 1
	}
	n := bytes.Count(source, []byte{'\n'})
	if source[len(source)-1] != '\n' {
		n++
	}
	return n
}

// handleIncomingCalls returns every workspace edge into the item.
// Edges from the same source file are coalesced into one entry with
// multiple `fromRanges`; LSP clients render each fromRange as a
// click target under the same caller, so emitting one item per edge
// would show the same caller N times in the call-hierarchy view.
func (s *Server) handleIncomingCalls(msg *requestMessage) {
	var p callHierarchyIncomingCallsParams
	if err := json.Unmarshal(msg.Params, &p); err != nil {
		_ = s.t.writeError(msg.ID, codeInvalidParams, "invalid incomingCalls params")
		return
	}
	if p.Item.Data == nil {
		_ = s.t.writeResponse(msg.ID, []callHierarchyIncomingCall{})
		return
	}
	idx := s.ensureIndex()
	edges := idx.IncomingEdges(p.Item.Data.File, p.Item.Data.Anchor)

	type bucket struct {
		item   callHierarchyItem
		ranges []Range
	}
	order := make([]string, 0, len(edges))
	groups := make(map[string]*bucket, len(edges))
	for _, e := range edges {
		// Call hierarchy is a cross-file dependency view: keep only
		// the edge kinds that represent inter-document flow. Anchor
		// and reference-style links are intra-document, and an
		// edge whose SourceFile equals the item's File is a
		// self-reference (e.g. `[a](#sec)` to a heading whose
		// anchor matches `Anchor`); both would clutter the result.
		if e.Kind == index.EdgeAnchorLink || e.Kind == index.EdgeRefLink {
			continue
		}
		if e.SourceFile == p.Item.Data.File {
			continue
		}
		r := rangeAt(e.SourceLine, e.SourceCol, nil)
		if g, ok := groups[e.SourceFile]; ok {
			g.ranges = append(g.ranges, r)
			continue
		}
		groups[e.SourceFile] = &bucket{
			item: callHierarchyItem{
				Name:           e.SourceFile,
				Kind:           symbolKindString,
				URI:            s.workspaceURI(e.SourceFile),
				Range:          r,
				SelectionRange: r,
				Data:           &callHierarchyData{File: e.SourceFile},
			},
			ranges: []Range{r},
		}
		order = append(order, e.SourceFile)
	}
	out := make([]callHierarchyIncomingCall, 0, len(order))
	for _, k := range order {
		g := groups[k]
		out = append(out, callHierarchyIncomingCall{From: g.item, FromRanges: g.ranges})
	}
	_ = s.t.writeResponse(msg.ID, out)
}

// handleOutgoingCalls returns every edge out of the item, scoped
// to the section when the item carries an anchor (heading-level
// call hierarchy). Edges to the same target file are coalesced into
// one entry with multiple `fromRanges`, matching the LSP grouping
// contract.
func (s *Server) handleOutgoingCalls(msg *requestMessage) {
	var p callHierarchyOutgoingCallsParams
	if err := json.Unmarshal(msg.Params, &p); err != nil {
		_ = s.t.writeError(msg.ID, codeInvalidParams, "invalid outgoingCalls params")
		return
	}
	if p.Item.Data == nil {
		_ = s.t.writeResponse(msg.ID, []callHierarchyOutgoingCall{})
		return
	}
	idx := s.ensureIndex()
	edges := idx.OutgoingEdges(p.Item.Data.File)
	startLine, endLine := outgoingScope(idx, p.Item.Data)

	type bucket struct {
		item   callHierarchyItem
		ranges []Range
	}
	order := make([]string, 0, len(edges))
	groups := make(map[string]*bucket, len(edges))
	for _, e := range edges {
		// Same-file anchor / ref-style links are intra-document and
		// don't fit the cross-file call-graph view.
		if e.Kind == index.EdgeAnchorLink || e.Kind == index.EdgeRefLink {
			continue
		}
		// Heading-scoped item: skip edges outside the section's
		// source range so a heading with no outbound links doesn't
		// inherit calls from sibling sections.
		if endLine > 0 && (e.SourceLine < startLine || e.SourceLine > endLine) {
			continue
		}
		toFile := e.TargetFile
		if toFile == "" {
			// Catalog without expansion: point at the host file's
			// directory as a placeholder. Plan 131 documents this
			// fallback explicitly under "Open Questions".
			toFile = path.Dir(p.Item.Data.File)
		}
		r := rangeAt(e.SourceLine, e.SourceCol, nil)
		if g, ok := groups[toFile]; ok {
			g.ranges = append(g.ranges, r)
			continue
		}
		// Coalesce by target file. The bucket represents the
		// callee file as a whole, so Data.Anchor must stay empty
		// — different edges from the source can target different
		// headings inside the same file, and a follow-up
		// incomingCalls on this item would otherwise be filtered
		// to whichever anchor happened to land in the bucket
		// first. To navigate to a specific heading, the user can
		// open the callee and re-issue prepareCallHierarchy
		// there.
		groups[toFile] = &bucket{
			item: callHierarchyItem{
				Name:           toFile,
				Kind:           symbolKindString,
				URI:            s.workspaceURI(toFile),
				Range:          Range{Start: Position{Line: 0, Character: 0}, End: Position{Line: 0, Character: 0}},
				SelectionRange: Range{Start: Position{Line: 0, Character: 0}, End: Position{Line: 0, Character: 0}},
				Data:           &callHierarchyData{File: toFile},
			},
			ranges: []Range{r},
		}
		order = append(order, toFile)
	}
	out := make([]callHierarchyOutgoingCall, 0, len(order))
	for _, k := range order {
		g := groups[k]
		out = append(out, callHierarchyOutgoingCall{To: g.item, FromRanges: g.ranges})
	}
	_ = s.t.writeResponse(msg.ID, out)
}

// outgoingScope returns the [startLine, endLine] bound for outgoing
// edges when the call-hierarchy item is heading-scoped. Returns
// (1, 0) — i.e. an open-ended range — for file-level items so the
// caller treats every edge as in scope.
func outgoingScope(idx *index.Index, data *callHierarchyData) (int, int) {
	if data == nil || data.Anchor == "" {
		return 1, 0
	}
	fe, ok := idx.File(data.File)
	if !ok {
		return 1, 0
	}
	for _, sym := range fe.Symbols {
		if sym.Kind == index.SymbolHeading && sym.Anchor == data.Anchor {
			return sym.StartLine, sym.EndLine
		}
	}
	return 1, 0
}
