package linkgraph

import (
	"bytes"
	"cmp"
	"io/fs"
	"path"
	"regexp"
	"slices"
	"strings"

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
var wikilinkRE = regexp.MustCompile(
	`(!?)\[\[([^\[\]\n|#]+)(?:#([^\[\]\n|]+))?(?:\|([^\[\]\n]+))?\]\]`,
)

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
	idx := &WikilinkIndex{
		stems: map[string][]string{},
		names: map[string][]string{},
	}
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
		base := path.Base(p)
		lcName := FileNameKey(base)
		idx.names[lcName] = append(idx.names[lcName], p)
		if lcStem, ok := FileStemKey(base); ok {
			idx.stems[lcStem] = append(idx.stems[lcStem], p)
		}
		return nil
	}); err != nil {
		return nil
	}
	for k, v := range idx.stems {
		sortByDepthThenName(v)
		idx.stems[k] = v
	}
	for k, v := range idx.names {
		sortByDepthThenName(v)
		idx.names[k] = v
	}
	return idx
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
		if matches, ok := idx.stems[FileNameKey(wantStem)]; ok && len(matches) > 0 {
			return matches[0], true
		}
		return "", false
	}
	if matches, ok := idx.names[FileNameKey(wantName)]; ok && len(matches) > 0 {
		return matches[0], true
	}
	return "", false
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
	slices.SortFunc(paths, func(a, b string) int {
		return cmp.Or(
			cmp.Compare(strings.Count(a, "/"), strings.Count(b, "/")),
			cmp.Compare(a, b),
		)
	})
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
	target, ok := normalizeTarget(target)
	if !ok {
		return "", false
	}
	_, stem, stemMode := wikilinkSearchKey(target)
	if !stemMode || stem == "" {
		return "", false
	}
	return FileNameKey(stem), true
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
// the link would be split across lines. A stem-mode target must equal
// base's FileStemKey; a typed one must equal base by name, ignoring case.
func WikilinkReaches(spelling, base string) bool {
	if strings.TrimSpace(spelling) != spelling || strings.ContainsRune(spelling, '\r') {
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
