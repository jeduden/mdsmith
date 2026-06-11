package lsp

import (
	"bytes"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/discovery"
	"github.com/jeduden/mdsmith/internal/index"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/mdtext"
	"github.com/jeduden/mdsmith/internal/yamlutil"
	mdsmith "github.com/jeduden/mdsmith/pkg/mdsmith"
)

// symbolWorkspace is the pkg/mdsmith.Workspace the symbol index reads
// through. Plan 215 routes every filesystem read in internal/lsp via
// this seam rather than a direct disk read, so the LSP shares the
// engine's filesystem abstraction. A root-less OSWorkspace reads the
// absolute, workspace-guarded paths the callers pass exactly as a
// direct host read did.
var symbolWorkspace mdsmith.Workspace = mdsmith.OSWorkspace{}

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
		data, err := symbolWorkspace.ReadFile(abs) // workspace-rooted, glob-validated
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
	data, err := symbolWorkspace.ReadFile(abs) // workspace-root + extension guarded above
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
// the workspace read runs only after both checks. Without those
// gates, a client could request `documentSymbol` / `definition` for
// arbitrary local files and exfiltrate their outlines through the
// response.
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
	data, err := symbolWorkspace.ReadFile(p) // workspace-root guarded; .md/.markdown only
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
	if real, err := evalSymlinks(abs); err == nil {
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
