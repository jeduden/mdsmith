package linkgraph

import (
	"bytes"
	"cmp"
	"io/fs"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"unicode"

	"github.com/jeduden/mdsmith/pkg/goldmark/ast"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/mdpath"
	"github.com/jeduden/mdsmith/internal/pathutil"
	"github.com/jeduden/mdsmith/internal/runcache"
)

// WikiLink is one parsed Obsidian-style wikilink occurrence.
//
// Target is the destination filename or stem (without alias or
// anchor). Anchor and Alias are the optional fragment and display
// label. Embed reports whether the source used `![[...]]` rather
// than `[[...]]`.
//
// Line and Column are body-relative — same convention as Link.
type WikiLink struct {
	Target string
	Anchor string
	Alias  string
	Embed  bool
	Line   int
	Column int
}

// wikilinkRE matches Obsidian-style wikilinks.
// Group 1: leading "!" (embed marker)
// Group 2: target stem or filename (no anchor or alias)
// Group 3: optional anchor (text after "#")
// Group 4: optional alias (text after "|")
var wikilinkRE = regexp.MustCompile(wikilinkPattern)

const wikilinkPattern = `(!?)\[\[([^\[\]\n|#]+)(?:#([^\[\]\n|]+))?(?:\|([^\[\]\n]+))?\]\]`

// wikilinkAtRE is wikilinkRE anchored at the start of its input, with
// the same groups. A caller reading the link at a known column matches
// there alone rather than scanning the rest of the row.
var wikilinkAtRE = regexp.MustCompile(`^` + wikilinkPattern)

// ExtractWikiLinks scans f.Source for Obsidian-style wikilinks
// (`[[Page]]`, `[[Page#anchor]]`, `[[Page|alias]]`, `![[file.png]]`).
// Matches inside fenced/indented code blocks, code spans, and
// `<?...?>` processing-instruction blocks are skipped — the same
// guards MDS054 applies to its bracket scanner.
//
// Lines are body-relative (post front-matter strip). Returns nil
// for files without a parsed AST (struct-literal *lint.File
// instances): the code-block / code-span guards below walk the
// tree, so a missing AST would otherwise panic.
func ExtractWikiLinks(f *lint.File) []WikiLink {
	if f == nil || len(f.Source) == 0 || f.AST == nil {
		return nil
	}
	// wikilinkRE can only match where the source contains a literal
	// "[[", so most files (which have none) can skip the code-block,
	// PI-block, and code-span AST walks entirely (high-performance-go.md,
	// "gate expensive analyzers behind a cheap pre-check").
	if !bytes.Contains(f.Source, []byte("[[")) {
		return nil
	}
	codeLines := lint.CollectCodeBlockLines(f)
	piLines := lint.CollectPIBlockLines(f)
	codeSpans := collectCodeSpanRanges(f)

	source := f.Source
	var out []WikiLink
	for _, m := range wikilinkRE.FindAllSubmatchIndex(source, -1) {
		start := m[0]
		line := f.LineOfOffset(start)
		if lint.InCodeOrPI(codeLines, piLines, line) {
			continue
		}
		if inCodeSpan(codeSpans, start) {
			continue
		}
		embed := m[3] > m[2]
		bracketStart := m[0]
		if embed {
			bracketStart++
		}
		col := f.ColumnOfOffset(bracketStart)
		wl := WikiLink{
			Target: strings.TrimSpace(string(source[m[4]:m[5]])),
			Embed:  embed,
			Line:   line,
			Column: col,
		}
		if m[6] >= 0 {
			wl.Anchor = strings.TrimSpace(string(source[m[6]:m[7]]))
		}
		if m[8] >= 0 {
			wl.Alias = strings.TrimSpace(string(source[m[8]:m[9]]))
		}
		out = append(out, wl)
	}
	return out
}

// byteRange is a half-open [start, end) byte range.
type byteRange struct{ start, end int }

func collectCodeSpanRanges(f *lint.File) []byteRange {
	var out []byteRange
	source := f.Source
	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if _, ok := n.(*ast.CodeSpan); !ok {
			return ast.WalkContinue, nil
		}
		first, last := codeSpanTextBounds(n)
		if first < 0 {
			return ast.WalkContinue, nil
		}
		start := first
		for start > 0 && source[start-1] == '`' {
			start--
		}
		end := last
		for end < len(source) && source[end] == '`' {
			end++
		}
		out = append(out, byteRange{start, end})
		return ast.WalkContinue, nil
	})
	return out
}

func codeSpanTextBounds(n ast.Node) (first, last int) {
	first = -1
	last = -1
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		t, ok := c.(*ast.Text)
		if !ok {
			continue
		}
		if first < 0 {
			first = t.Segment.Start
		}
		last = t.Segment.Stop
	}
	return first, last
}

// inCodeSpan reports whether offset falls inside one of spans. spans
// must be sorted by start and disjoint, which collectCodeSpanRanges
// guarantees (AST walk order; the canonical parser installs no
// node-relocating extension, and code spans cannot nest). Binary search
// keeps a wikilink-heavy file from going matches × spans.
func inCodeSpan(spans []byteRange, offset int) bool {
	i, _ := slices.BinarySearchFunc(spans, offset, func(r byteRange, off int) int {
		return cmp.Compare(r.start, off)
	})
	// i is the first span with start >= offset; the candidate that
	// could contain offset is that span (start == offset) or the one
	// before it.
	if i < len(spans) && spans[i].start == offset && offset < spans[i].end {
		return true
	}
	return i > 0 && offset < spans[i-1].end
}

// WikilinkIndexFor returns a *WikilinkIndex for root, memoized on
// cache under rootKey when cache is non-nil. With cache=nil the
// helper falls through to a direct NewWikilinkIndex call —
// callers without a long-lived cache (one-shot CLI commands)
// still share the same API.
//
// This is the canonical entry point both MDS027 and
// `mdsmith list backlinks` route through; rewriting it once
// keeps the workspace walk semantics in one place.
func WikilinkIndexFor(cache *runcache.Cache, rootKey string, root fs.FS) *WikilinkIndex {
	if cache == nil || rootKey == "" {
		return NewWikilinkIndex(root)
	}
	v := cache.Wikilinks(rootKey, func() any {
		return NewWikilinkIndex(root)
	})
	idx, _ := v.(*WikilinkIndex)
	return idx
}

// WikilinkIndexAtDir walks the directory dir on disk for the wikilink
// index, through the lint.OpenRootFS view the MDS027 resolver walks.
// It is the entry point for a caller holding a root path and no run
// cache (the CLI, the LSP move guard, `mdsmith list backlinks`), so the
// way that root is opened stays in one place. An empty or unreadable
// dir builds no index (nil). The root's handle is closed before it
// returns.
func WikilinkIndexAtDir(dir string) *WikilinkIndex {
	root := lint.OpenRootFS(dir)
	// The index holds paths only, so the root closes once it is built.
	// A close error on a read-only directory handle loses nothing.
	defer func() { _ = root.Close() }()
	return WikilinkIndexFor(nil, "", root)
}

// CachedWikilinkIndexAtDir returns the index memoized on cache for dir,
// walking dir on disk (as WikilinkIndexAtDir does) only on a miss. The
// key is dir's absolute form, the key MDS027 stores its index under, so
// a caller holding a session's run cache reads the index a lint already
// built instead of walking the tree again. A nil cache walks dir, as
// WikilinkIndexFor does.
//
// filepath.Abs only errors when os.Getwd fails, an OS-level failure
// MDS027's wikilinkCacheKey swallows the same way.
func CachedWikilinkIndexAtDir(cache *runcache.Cache, dir string) *WikilinkIndex {
	if cache == nil {
		return WikilinkIndexAtDir(dir)
	}
	key, _ := filepath.Abs(dir) //nolint:errcheck
	v := cache.Wikilinks(key, func() any { return WikilinkIndexAtDir(dir) })
	idx, _ := v.(*WikilinkIndex)
	return idx
}

// WikilinkIndex is a pre-built directory of every file under one
// workspace root, keyed for the two lookup shapes ResolveWikiLink
// uses: stem (.md/.markdown filename minus extension) and exact
// filename. Each key holds the matching paths in shortest-then-
// alphabetical order, the same order ResolveWikiLink would
// otherwise sort on every call.
//
// Build the index once per (run, root) — e.g. via
// `runcache.Cache.Wikilinks` — and call Resolve for each wikilink
// target. Lookups are then O(stems + matches) instead of O(files
// in workspace) per target.
type WikilinkIndex struct {
	stems map[string][]string // lowercased stem → sorted .md paths
	names map[string][]string // lowercased filename → sorted any-ext paths
	base  *WikilinkIndex      // Moved's receiver: holds every key stems and names lack
}

// NewWikilinkIndex walks root once and returns a lookup table that
// future Resolve calls can serve from memory. Returns nil when root
// is nil or the workspace walk itself fails (e.g. Open(".") on root
// returns an error). A nil return causes Resolve to return ("", false)
// for every target; this is preferable to returning an empty index that
// would silently report every target as not found.
func NewWikilinkIndex(root fs.FS) *WikilinkIndex {
	if root == nil {
		return nil
	}
	idx := newEmptyWikilinkIndex()
	if err := fs.WalkDir(root, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			// Root-level read failures (e.g. ReadDir(".") returns an
			// error) mean the index would otherwise be silently empty.
			// Propagate so NewWikilinkIndex can return nil and the
			// resolver falls back to per-call walks. Sub-tree failures
			// stay local — one rejected directory should not poison
			// resolution against unrelated sibling subtrees.
			if p == "." {
				return err
			}
			return nil
		}
		if d.IsDir() {
			return skipHeavyDirs(p)
		}
		idx.add(p)
		return nil
	}); err != nil {
		return nil
	}
	idx.sort()
	return idx
}

// NewWikilinkIndexFromPaths builds the index over a list of
// workspace-relative file paths instead of a walk, keying and ordering
// them as NewWikilinkIndex does and dropping any path under `.git` or
// `node_modules`. A caller with no readable root but a known file list
// uses it so a lookup still sees those files.
func NewWikilinkIndexFromPaths(paths []string) *WikilinkIndex {
	idx := newEmptyWikilinkIndex()
	for _, p := range paths {
		if WikilinkIndexed(p) {
			idx.add(p)
		}
	}
	idx.sort()
	return idx
}

// Moved returns a new index that reads as idx will once every move in
// moves (workspace-relative source → destination) has run: each source
// leaves its keys, and each destination joins its own, keyed and
// ordered as NewWikilinkIndexFromPaths keys them. An empty destination
// only removes its source (the file leaves the workspace), and a
// destination already indexed is held once. A source idx lacks has
// nothing to remove, but its destination still joins: the file exists
// once the move has run. idx itself is not changed. A nil index
// returns nil: it stands for a root that could not be walked, which no
// move changes. A batch of renames planned together reads it to learn
// which file a `[[stem]]` reaches after the batch.
//
// The result is an overlay on idx: it holds only the keys the moves
// touch and reads every other key from idx, so its cost follows the
// moves, not the workspace. idx must not change while it is in use.
func (idx *WikilinkIndex) Moved(moves map[string]string) *WikilinkIndex {
	if idx == nil {
		return nil
	}
	// Every touched key starts with no joining destination; each
	// indexed destination is then filed under its own keys once, so the
	// build stays linear in the moves.
	out := &WikilinkIndex{stems: map[string][]string{}, names: map[string][]string{}, base: idx}
	touch := func(p string, join bool) {
		base := path.Base(p)
		name := FileNameKey(base)
		out.names[name] = appendIf(out.names[name], p, join)
		if stem, ok := FileStemKey(base); ok {
			out.stems[stem] = appendIf(out.stems[stem], p, join)
		}
	}
	for src, dst := range moves {
		touch(src, false)
		if dst != "" {
			touch(dst, WikilinkIndexed(dst))
		}
	}
	for key, joining := range out.names {
		out.names[key] = movedPaths(idx.NamePaths(key), moves, joining)
	}
	for key, joining := range out.stems {
		out.stems[key] = movedPaths(idx.StemPaths(key), moves, joining)
	}
	return out
}

// appendIf returns paths with p appended when join is set, and paths
// unchanged otherwise.
func appendIf(paths []string, p string, join bool) []string {
	if join {
		return append(paths, p)
	}
	return paths
}

// movedPaths returns paths, one key's holders, once moves has run:
// without every source, and with every one of joining (the indexed
// destinations whose base name holds the key), each held once, in
// resolver order. paths is not changed.
func movedPaths(paths []string, moves map[string]string, joining []string) []string {
	out := make([]string, 0, len(paths)+len(joining))
	for _, p := range paths {
		if _, gone := moves[p]; !gone {
			out = append(out, p)
		}
	}
	for _, dst := range joining {
		if !slices.Contains(out, dst) {
			out = append(out, dst)
		}
	}
	sortByDepthThenName(out)
	return out
}

// newEmptyWikilinkIndex returns an index with no files, ready for add.
func newEmptyWikilinkIndex() *WikilinkIndex {
	return &WikilinkIndex{
		stems: map[string][]string{},
		names: map[string][]string{},
	}
}

// add files p under its exact-name key and, for a Markdown file, its
// stem key.
func (idx *WikilinkIndex) add(p string) {
	base := path.Base(p)
	lcName := FileNameKey(base)
	idx.names[lcName] = append(idx.names[lcName], p)
	if lcStem, ok := FileStemKey(base); ok {
		idx.stems[lcStem] = append(idx.stems[lcStem], p)
	}
}

// sort orders every key's paths shallowest first, then by name.
func (idx *WikilinkIndex) sort() {
	for _, v := range idx.stems {
		sortByDepthThenName(v)
	}
	for _, v := range idx.names {
		sortByDepthThenName(v)
	}
}

// Resolve answers the same question as ResolveWikiLink but serves
// it from the prebuilt index — no filesystem walk per call.
func (idx *WikilinkIndex) Resolve(target string) (string, bool) {
	if idx == nil || target == "" {
		return "", false
	}
	target, ok := normalizeTarget(target)
	if !ok {
		return "", false
	}
	wantName, wantStem, stemMode := wikilinkSearchKey(target)
	if stemMode {
		if matches := idx.StemPaths(FileNameKey(wantStem)); len(matches) > 0 {
			return matches[0], true
		}
		return "", false
	}
	if matches := idx.NamePaths(FileNameKey(wantName)); len(matches) > 0 {
		return matches[0], true
	}
	return "", false
}

// StemPaths returns the Markdown files the resolver reaches by the
// stem key (as FileStemKey returns it), shallowest first. The slice is
// the index's own: callers must not modify it. A nil index returns nil.
func (idx *WikilinkIndex) StemPaths(key string) []string {
	for ; idx != nil; idx = idx.base {
		if paths, ok := idx.stems[key]; ok || idx.base == nil {
			return paths
		}
	}
	return nil
}

// StemResolvesTo reports whether a `[[stem]]` link keyed by key (as
// FileStemKey returns it) resolves to the workspace-relative path p,
// counting p as a holder of key even when the index lacks it: the
// shallowest holder wins, then the first by full path in byte order
// (so `Docs/` sorts before `archive/`). A nil index holds no other
// file, so p wins. An indexed path that differs from p in letter case
// alone keeps p from winning unless it is p exactly (see resolvesTo).
func (idx *WikilinkIndex) StemResolvesTo(key, p string) bool {
	return resolvesTo(idx.StemPaths(key), p)
}

// NameResolvesTo is StemResolvesTo for a typed `[[name.ext]]` link
// keyed by key (as FileNameKey returns it): the files holding that
// exact name pick the link's file in the same order.
func (idx *WikilinkIndex) NameResolvesTo(key, p string) bool {
	return resolvesTo(idx.NamePaths(key), p)
}

// resolvesTo reports whether p is the first of paths, sorted by
// compareDepthThenName, once p is counted among them. A path that
// differs from p in letter case alone may be p itself, as a
// case-insensitive file system spells it on disk, and then p's own
// spelling does not say where it sorts: p is not known to win.
func resolvesTo(paths []string, p string) bool {
	if len(paths) == 0 || paths[0] == p {
		return true
	}
	if slices.ContainsFunc(paths, func(q string) bool { return strings.EqualFold(q, p) }) {
		return false
	}
	return compareDepthThenName(p, paths[0]) < 0
}

// NamePaths returns the files, of any extension, the resolver reaches
// by the exact-name key (as FileNameKey returns it), shallowest first.
// The slice is the index's own: callers must not modify it. A nil
// index returns nil.
func (idx *WikilinkIndex) NamePaths(key string) []string {
	for ; idx != nil; idx = idx.base {
		if paths, ok := idx.names[key]; ok || idx.base == nil {
			return paths
		}
	}
	return nil
}

// WikilinkIndexed reports whether NewWikilinkIndex's walk prunes no
// directory on the workspace-relative path p: none is one
// skipHeavyDirs prunes (`.git`, `node_modules`). No wikilink reaches a
// file under a pruned directory, whatever its name. It reads p alone,
// so it cannot see that the walk also stays out of a symlinked or
// unreadable directory.
func WikilinkIndexed(p string) bool {
	for d := path.Dir(p); d != "." && d != "/"; d = path.Dir(d) {
		if skipHeavyDirs(d) != nil {
			return false
		}
	}
	return true
}

// skipHeavyDirs returns fs.SkipDir for known-heavy subtrees that
// never carry wikilink targets users want resolved (`.git`,
// `node_modules`). Used as a fs.WalkDirFunc verdict for directory
// entries — file entries pass through unchanged. Mirrors the
// pattern walkDirDecision uses in duplicatedcontent so the same
// names stay pruned across every rule that walks the workspace.
func skipHeavyDirs(p string) error {
	if p == "." {
		return nil
	}
	switch path.Base(p) {
	case ".git", "node_modules":
		return fs.SkipDir
	}
	return nil
}

// sortByDepthThenName orders paths by (path-separator count, name), so
// Resolve's first match is the shallowest path. slices.SortFunc
// compares the concrete string values directly, unlike sort.Slice,
// which drives reflect.Swapper internally (see
// docs/development/high-performance-go.md, "reflect in hot paths").
// This runs once per basename bucket on every WikilinkIndex (re)build,
// so real workspaces with colliding basenames (README.md, index.md)
// pay it N times per rebuild, not once.
//
// The comparator counts separators on each call rather than caching
// the depths in a side slice. strings.Count does not allocate, and
// buckets are small: a cached-depth copy cost one allocation per
// bucket and measured slower below about 50 paths, breaking even there.
func sortByDepthThenName(paths []string) {
	slices.SortFunc(paths, compareDepthThenName)
}

// compareDepthThenName orders a before b when it is shallower, or at
// the same depth sorts first by name: the order a `[[stem]]` picks its
// file in.
func compareDepthThenName(a, b string) int {
	return cmp.Or(
		cmp.Compare(strings.Count(a, "/"), strings.Count(b, "/")),
		cmp.Compare(a, b),
	)
}

// ResolveWikiLink resolves an Obsidian-style wikilink target against
// root, returning the workspace-relative path of the resolved file.
// Delegates to NewWikilinkIndex so the walk/match algorithm lives in
// one place. For repeated resolutions against the same root, build
// a WikilinkIndex once (via WikilinkIndexFor) and call Resolve
// directly instead.
//
// from is reserved for future per-directory resolution preference
// and is currently unused.
func ResolveWikiLink(root fs.FS, _ string, target string) (string, bool) {
	return NewWikilinkIndex(root).Resolve(target)
}

// WikilinkStem returns the lowercased basename stem that a bare-page
// or Markdown-extension wikilink target resolves by, with ok=true. It
// mirrors WikilinkIndex.Resolve's stem-mode matching (lowercased stem
// lookup), so a caller keying edges by stem matches the same files the
// resolver would. A typed non-Markdown target (e.g. `diagram.png`)
// returns ok=false: those resolve by exact filename, never by stem, and
// never point at the Markdown files a move relocates. A traversal or
// absolute target also returns ok=false.
func WikilinkStem(target string) (string, bool) {
	key, stem, ok := WikilinkKey(target)
	if !ok || !stem {
		return "", false
	}
	return key, true
}

// WikilinkKey reads target once and returns the key it resolves by,
// with stem reporting the key space: the lowercased basename stem
// (stem=true, as WikilinkStem returns it) for a bare or Markdown
// target, or the lowercased exact base name (stem=false, as
// WikilinkName returns it) for a typed one. ok is false for a target
// the resolver refuses and for a stem-mode target with an empty stem
// (`[[.md]]`), which neither key space files a link under. A caller
// that takes either key, such as the index build, reads each target
// through it once rather than through both functions.
func WikilinkKey(target string) (key string, stem, ok bool) {
	target, ok = normalizeTarget(target)
	if !ok {
		return "", false, false
	}
	name, base, stemMode := wikilinkSearchKey(target)
	if !stemMode {
		return FileNameKey(name), false, true
	}
	if base == "" {
		return "", false, false
	}
	return FileNameKey(base), true, true
}

// WikilinkStemAt reads the wikilink whose `[[` starts at bracketStart
// in row. It returns the target's stem key (as WikilinkStem returns it)
// and the byte span, within row, of the target's base segment: the part
// the resolver keys by (path.Base of the trimmed target with `\` read as
// `/`). Any folder prefix, anchor, and alias lie outside the span, as
// does the `\` that escapes a `|` in a table cell. Key and span come
// from one match, the same one ExtractWikiLinks reads, so they cannot
// disagree on where a target ends. ok is false when no wikilink starts
// there or its target has no stem key: a typed non-Markdown name, or a
// target the resolver refuses. A caller holding an edge from an index
// that may be stale checks the key before it edits the span.
func WikilinkStemAt(row []byte, bracketStart int) (stem string, start, end int, ok bool) {
	raw, at, ok := wikilinkTargetAt(row, bracketStart)
	if !ok {
		return "", 0, 0, false
	}
	stem, ok = WikilinkStem(string(raw))
	if !ok {
		return "", 0, 0, false
	}
	lo, hi := wikilinkBaseSpan(raw)
	return stem, at + lo, at + hi, true
}

// WikilinkName returns the exact-name key a typed `[[name.ext]]`
// target resolves by: the lowercased basename (FileNameKey), the key
// WikilinkIndex.NamePaths files every file under. ok is false for a
// stem-mode target (bare or Markdown, see WikilinkStem) and for a
// target the resolver refuses. The two functions split between them
// every target WikilinkKey keys.
func WikilinkName(target string) (string, bool) {
	key, stem, ok := WikilinkKey(target)
	if !ok || stem {
		return "", false
	}
	return key, true
}

// WikilinkNameAt is WikilinkStemAt for a typed link: it returns the
// target's name key (as WikilinkName returns it) and the byte span,
// within row, of the target's base segment. ok is false when no
// wikilink starts at bracketStart or its target has no name key.
func WikilinkNameAt(row []byte, bracketStart int) (name string, start, end int, ok bool) {
	raw, at, ok := wikilinkTargetAt(row, bracketStart)
	if !ok {
		return "", 0, 0, false
	}
	name, ok = WikilinkName(string(raw))
	if !ok {
		return "", 0, 0, false
	}
	lo, hi := wikilinkBaseSpan(raw)
	return name, at + lo, at + hi, true
}

// wikilinkBaseSpan returns the byte span, within raw, of the segment
// the resolver keys a target by: path.Base of the trimmed target with
// `\` read as `/`.
func wikilinkBaseSpan(raw []byte) (lo, hi int) {
	left := bytes.TrimLeftFunc(raw, unicode.IsSpace)
	lo = len(raw) - len(left)
	hi = lo + len(bytes.TrimRightFunc(left, unicode.IsSpace))
	for hi > lo && (raw[hi-1] == '/' || raw[hi-1] == '\\') {
		hi--
	}
	for j := lo; j < hi; j++ {
		if raw[j] == '/' || raw[j] == '\\' {
			lo = j + 1
		}
	}
	return lo, hi
}

// wikilinkTargetAt returns the raw target of the wikilink whose `[[`
// starts at bracketStart, read by the same match ExtractWikiLinks uses,
// and the target's byte offset within row.
func wikilinkTargetAt(row []byte, bracketStart int) (raw []byte, at int, ok bool) {
	if bracketStart < 0 || bracketStart >= len(row) {
		return nil, 0, false
	}
	m := wikilinkAtRE.FindSubmatchIndex(row[bracketStart:])
	if m == nil {
		return nil, 0, false
	}
	at = bracketStart + m[4]
	return row[at : bracketStart+m[5]], at, true
}

// wikilinkSearchKey splits target into the lookup parameters
// WikilinkIndex.Resolve uses. target must already have backslashes
// normalised to forward slashes (Resolve does this before calling).
//
// stemMode true means "match by filename stem against .md/.markdown
// files only" — the bare-page case (`[[Notes]]` or
// `[[Notes.md]]`). stemMode false means "match by exact filename"
// — the typed-extension case (`![[diagram.png]]`).
func wikilinkSearchKey(target string) (wantName, wantStem string, stemMode bool) {
	base := path.Base(target)
	ext := path.Ext(base)
	// An empty extension (bare page, e.g. [[Notes]]) or a Markdown
	// extension resolves by stem; a typed non-Markdown extension
	// (e.g. ![[diagram.png]]) resolves by exact filename.
	if ext == "" || mdpath.HasMarkdownExt(ext) {
		return "", strings.TrimSuffix(base, ext), true
	}
	return base, "", false
}

// normalizeTarget trims target, turns backslashes into slashes, and
// reports ok=false for a target the resolver never looks up: an empty,
// absolute, drive-letter, or UNC one, or one that cleans to `.` or
// climbs out with `..`. Resolve, WikilinkStem, and WikilinkReaches all
// read targets through it, so they agree on which ones resolve.
func normalizeTarget(target string) (string, bool) {
	target = strings.TrimSpace(target)
	if target == "" || pathutil.IsAbsOrDriveOrUNC(target) {
		return "", false
	}
	target = strings.ReplaceAll(target, `\`, `/`)
	cleaned := path.Clean(target)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", false
	}
	return target, true
}

// FileNameKey returns the key NewWikilinkIndex files base under by
// exact name: the lowercased basename. A typed wikilink such as
// `[[guide.mdx]]` resolves through this key. strings.ToLower, not
// strings.EqualFold, defines the match, so callers comparing names
// must key both sides with this function.
func FileNameKey(base string) string {
	return strings.ToLower(base)
}

// FileStemKey returns the lowercased stem that NewWikilinkIndex keys a
// file named base under, with ok=true when base is a Markdown file. A
// non-Markdown base has no stem key (ok=false): a bare `[[name]]`
// never reaches it. The stem is not trimmed, so ` guide.md` keys as
// ` guide`, which no trimmed `[[guide]]` target matches. An empty stem
// (`.md`) returns "" with ok=true; no wikilink spells it.
func FileStemKey(base string) (string, bool) {
	if !mdpath.IsMarkdownPath(base) {
		return "", false
	}
	return FileNameKey(strings.TrimSuffix(base, path.Ext(base))), true
}

// WikilinkReaches reports whether writing `[[spelling]]` yields a
// wikilink that resolves by the key of a file named base. The token
// must be read back by ExtractWikiLinks whole, with no anchor or alias
// split off and no whitespace trimmed, and the resolver must accept the
// target. A CR is refused too: CommonMark ends a line at a lone CR, so
// the link would be split across lines. So is a backtick, which can
// pair with a later backtick on the line and open a code span that
// swallows the closing `]]`. A stem-mode target must equal
// base's FileStemKey; a typed one must equal base by name, ignoring case.
func WikilinkReaches(spelling, base string) bool {
	if strings.TrimSpace(spelling) != spelling || strings.ContainsAny(spelling, "\r`") {
		return false
	}
	token := "[[" + spelling + "]]"
	m := wikilinkRE.FindStringSubmatchIndex(token)
	if m == nil || m[0] != 0 || m[1] != len(token) || m[6] >= 0 || m[8] >= 0 {
		return false
	}
	target, ok := normalizeTarget(spelling)
	if !ok {
		return false
	}
	wantName, wantStem, stemMode := wikilinkSearchKey(target)
	if !stemMode {
		return FileNameKey(wantName) == FileNameKey(base)
	}
	stem, ok := FileStemKey(base)
	return ok && stem != "" && FileNameKey(wantStem) == stem
}
