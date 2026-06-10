// Package index builds and maintains the symbol graph that powers
// mdsmith's LSP navigation methods (documentSymbol, definition,
// references, workspace/symbol, callHierarchy).
//
// The graph stores four kinds of symbols — headings, link-reference
// definitions, top-level front-matter keys, and directives — together
// with the inbound/outbound reference edges that connect them across
// files: anchor links, file links, reference-style links, and the
// include / catalog / build directive targets.
//
// Build is workspace-wide; updates are per-file. Callers re-parse one
// buffer with Update on document events and rebuild the whole index
// when the project's `.mdsmith.yml` changes (kind / ignore globs may
// shift scope).
package index

import (
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
)

// SymbolKind enumerates the four symbol shapes the index recognizes.
// Each maps to a specific LSP SymbolKind in the LSP layer; this
// package keeps the spec-level numbers out of its core types.
type SymbolKind int

const (
	// SymbolHeading is a Markdown heading at any level (H1–H6). The
	// Anchor field carries the slug; the Level field carries the
	// heading level.
	SymbolHeading SymbolKind = iota
	// SymbolLinkRef is a `[label]: url` link-reference definition.
	// The Anchor field carries the normalized label.
	SymbolLinkRef
	// SymbolFrontMatter is a top-level YAML front-matter key. The
	// Name field carries the key.
	SymbolFrontMatter
	// SymbolDirective is a processing-instruction block (<?name … ?>).
	// The Name field carries the directive name.
	SymbolDirective
)

// Symbol is one entry in a file's outline.
type Symbol struct {
	// File is the workspace-relative path of the containing file
	// (forward slashes, no leading `./`). Index lookups key on this.
	File string
	// Kind is the symbol category.
	Kind SymbolKind
	// Name is the human-readable label (heading text, key, label,
	// directive name).
	Name string
	// Anchor is the normalized identifier used for cross-document
	// lookups: heading slug, link-ref label, or "" for other kinds.
	Anchor string
	// Level is the heading level (1–6) for SymbolHeading; 0 otherwise.
	Level int
	// StartLine, EndLine are 1-based line numbers covering the
	// symbol's full range. For headings the range extends to the
	// next sibling heading; for other kinds it's the source line.
	StartLine int
	EndLine   int
	// SelectionLine, SelectionCol point to the symbol's name/label
	// (1-based) — what an editor highlights when "go to definition"
	// jumps to it.
	SelectionLine int
	SelectionCol  int
}

// EdgeKind enumerates the kinds of references the index tracks.
type EdgeKind int

const (
	// EdgeAnchorLink is `[text](#anchor)` — same-file heading reference.
	EdgeAnchorLink EdgeKind = iota
	// EdgeFileLink is `[text](./other.md)` (with optional anchor).
	EdgeFileLink
	// EdgeRefLink is `[text][label]` — reference-style link use.
	EdgeRefLink
	// EdgeInclude is a `<?include file: …?>` directive.
	EdgeInclude
	// EdgeCatalog is a `<?catalog?>` directive.
	EdgeCatalog
	// EdgeBuild is one `inputs:` entry of a `<?build?>` directive. A
	// literal entry is a resolved edge to the input file; a glob entry
	// is emitted Unresolved, like a catalog edge.
	EdgeBuild
)

// Edge records one reference from a source position to a target.
//
// Empty TargetFile means "same file as Source" (used for anchor and
// reference-style links). Empty TargetAnchor means the reference
// targets the file as a whole (e.g. `[text](./other.md)`).
//
// Unresolved is set on edges whose target shape is a glob pattern
// (catalog directives) rather than a single file. Reverse-edge
// queries (IncomingEdges / BacklinksFor) skip unresolved edges so
// catalog directives don't surface as phantom self-backlinks the way
// empty-TargetFile placeholders did before plan 153.
type Edge struct {
	SourceFile   string
	SourceLine   int // 1-based
	SourceCol    int // 1-based
	TargetFile   string
	TargetAnchor string
	TargetLabel  string
	Kind         EdgeKind
	Unresolved   bool
}

// FileEntry is one file's contribution to the index.
type FileEntry struct {
	// Path is the workspace-relative path with forward slashes.
	Path string
	// Symbols are this file's symbols, in document order.
	Symbols []Symbol
	// Outgoing are the references this file emits.
	Outgoing []Edge
	// Title is the front-matter `title:` value if set, "" otherwise.
	Title string
	// Kinds are the front-matter `kinds:` values if set.
	Kinds []string
	// LineCount is the number of source lines (1-based-inclusive
	// upper bound for symbol ranges). Used to bound heading ranges.
	LineCount int
}

// Index is the workspace-wide symbol graph. Methods are safe to call
// concurrently with each other; concurrent Update/Remove on the same
// path is serialized internally.
type Index struct {
	mu    sync.RWMutex
	root  string
	files map[string]*FileEntry
}

// New returns an empty Index rooted at root. Build populates it.
func New(root string) *Index {
	return &Index{
		root:  root,
		files: make(map[string]*FileEntry),
	}
}

// Root returns the workspace root the index was built against.
func (i *Index) Root() string {
	if i == nil {
		return ""
	}
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.root
}

// Files returns a snapshot of the indexed file paths in arbitrary
// order. Callers must not retain the slice across mutations of the
// index.
func (i *Index) Files() []string {
	if i == nil {
		return nil
	}
	i.mu.RLock()
	defer i.mu.RUnlock()
	out := make([]string, 0, len(i.files))
	for path := range i.files {
		out = append(out, path)
	}
	return out
}

// File returns a snapshot of the FileEntry for the given workspace-
// relative path. The pointer is to a copy so callers may read the
// slices without holding the index lock; the slices themselves are
// shared, so callers must not mutate them.
func (i *Index) File(path string) (*FileEntry, bool) {
	if i == nil {
		return nil, false
	}
	path = NormalizePath(path)
	i.mu.RLock()
	defer i.mu.RUnlock()
	fe, ok := i.files[path]
	if !ok {
		return nil, false
	}
	cp := *fe
	return &cp, true
}

// upsert installs or replaces a FileEntry under fe.Path.
func (i *Index) upsert(fe *FileEntry) {
	i.mu.Lock()
	i.files[fe.Path] = fe
	i.mu.Unlock()
}

// Remove drops the entry for path. No-op when absent.
func (i *Index) Remove(path string) {
	if i == nil {
		return
	}
	path = NormalizePath(path)
	i.mu.Lock()
	delete(i.files, path)
	i.mu.Unlock()
}

// Update re-parses source under path and replaces the FileEntry.
// When source is empty the file is removed entirely (matches the
// case where the file was deleted from disk).
//
// path must be workspace-relative. AbsPathToWorkspace is provided as
// a helper for callers that hold an absolute filesystem path.
func (i *Index) Update(path string, source []byte) {
	if i == nil {
		return
	}
	path = NormalizePath(path)
	if path == "" {
		return
	}
	if len(source) == 0 {
		i.Remove(path)
		return
	}
	fe := buildFileEntry(path, source)
	i.upsert(fe)
}

// UpdateWithKinds is Update plus an override for the file's effective
// kinds list. Callers pass the resolved (front-matter ∪ kind-
// assignment) list so workspace-symbol search and `kind:` navigation
// see config-driven assignments, not just front-matter declarations.
// When kinds is nil the result is identical to Update.
func (i *Index) UpdateWithKinds(path string, source []byte, kinds []string) {
	if i == nil {
		return
	}
	path = NormalizePath(path)
	if path == "" {
		return
	}
	if len(source) == 0 {
		i.Remove(path)
		return
	}
	fe := buildFileEntry(path, source)
	if kinds != nil {
		fe.Kinds = append([]string(nil), kinds...)
	}
	i.upsert(fe)
}

// Build walks the workspace and indexes every Markdown file the
// supplied loader yields. The loader is called once per discovered
// path; returning an error skips that file. files is the list of
// workspace-relative paths to index, typically produced by
// discovery.Discover and then made workspace-relative.
//
// Build replaces the entire current index, including evicting any
// entries whose path no longer appears in files.
//
// Build fans out the per-file extractor across runtime.GOMAXPROCS(0)
// worker goroutines so a multi-thousand-file workspace lands in the
// graph in roughly wall-clock / cpu-cores time. The extractor itself
// is pure given (path, bytes); the only shared state is the result
// map, which a single collector goroutine drains. The supplied loader
// is called concurrently — callers whose loader is not safe for
// concurrent calls must serialise inside it or fall back to
// BuildSerial.
func (i *Index) Build(files []string, load func(path string) ([]byte, error)) {
	if i == nil {
		return
	}
	next := buildEntriesParallel(files, load, runtime.GOMAXPROCS(0))
	i.mu.Lock()
	i.files = next
	i.mu.Unlock()
}

// BuildSerial is the single-threaded variant of Build. Use this when
// the loader is not safe for concurrent calls.
func (i *Index) BuildSerial(files []string, load func(path string) ([]byte, error)) {
	if i == nil {
		return
	}
	next := make(map[string]*FileEntry, len(files))
	for _, p := range files {
		path := NormalizePath(p)
		if path == "" {
			continue
		}
		data, err := load(path)
		if err != nil || len(data) == 0 {
			continue
		}
		next[path] = buildFileEntry(path, data)
	}
	i.mu.Lock()
	i.files = next
	i.mu.Unlock()
}

// buildEntriesParallel runs the per-file extractor across workers
// goroutines and returns the assembled map. workers <= 1 falls back
// to a sequential build so callers can dial the parallelism via the
// caller (Build uses GOMAXPROCS).
//
// Each worker handles a contiguous slice of files and writes into
// its own local map; the final merge happens after wg.Wait so no
// channel or mutex is in the hot path. The per-file extractor is
// pure given (path, bytes), so workers share no mutable state.
func buildEntriesParallel(files []string, load func(path string) ([]byte, error), workers int) map[string]*FileEntry {
	if workers < 1 {
		workers = 1
	}
	if workers == 1 || len(files) <= 1 {
		return buildEntriesChunk(files, load)
	}
	chunkSize := (len(files) + workers - 1) / workers
	localMaps := make([]map[string]*FileEntry, workers)
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		start := w * chunkSize
		if start >= len(files) {
			break
		}
		end := start + chunkSize
		if end > len(files) {
			end = len(files)
		}
		wg.Add(1)
		go func(idx int, chunk []string) {
			defer wg.Done()
			localMaps[idx] = buildEntriesChunk(chunk, load)
		}(w, files[start:end])
	}
	wg.Wait()
	return mergeFileEntryMaps(localMaps)
}

// buildEntriesChunk builds FileEntries for a contiguous slice of
// files into a fresh map. Returns the map even when chunk is empty
// so callers can append it unconditionally during the merge.
func buildEntriesChunk(chunk []string, load func(path string) ([]byte, error)) map[string]*FileEntry {
	out := make(map[string]*FileEntry, len(chunk))
	for _, p := range chunk {
		path := NormalizePath(p)
		if path == "" {
			continue
		}
		data, err := load(path)
		if err != nil || len(data) == 0 {
			continue
		}
		out[path] = buildFileEntry(path, data)
	}
	return out
}

// mergeFileEntryMaps concatenates per-worker maps into one. Workers
// in buildEntriesParallel touch disjoint file paths so a key
// collision indicates a bug — the loop overwrites silently to
// preserve the "last worker wins" semantic that matched the previous
// sequential path.
func mergeFileEntryMaps(maps []map[string]*FileEntry) map[string]*FileEntry {
	total := 0
	for _, m := range maps {
		total += len(m)
	}
	next := make(map[string]*FileEntry, total)
	for _, m := range maps {
		for k, v := range m {
			next[k] = v
		}
	}
	return next
}

// IncomingEdges returns every workspace edge whose target is the
// given (file, anchor). When anchor is "" matches edges to the file
// at large (no anchor specified by the caller).
//
// Unresolved edges (catalog directives whose glob hasn't been
// expanded) are skipped — they don't yet point at a specific file,
// so they can't satisfy a (file, anchor) match. Treating their
// empty TargetFile as "same file" the way concrete same-file edges
// are treated would misattribute them as phantom self-backlinks
// (see plan 153 for the unification that introduced the flag).
//
// The returned slice is a fresh copy.
func (i *Index) IncomingEdges(file, anchor string) []Edge {
	if i == nil {
		return nil
	}
	file = NormalizePath(file)
	i.mu.RLock()
	defer i.mu.RUnlock()
	var out []Edge
	for _, fe := range i.files {
		for _, e := range fe.Outgoing {
			if e.Unresolved {
				continue
			}
			tFile := e.TargetFile
			if tFile == "" {
				tFile = fe.Path
			}
			tFile = NormalizePath(tFile)
			if tFile != file {
				continue
			}
			if anchor != "" && e.TargetAnchor != anchor {
				continue
			}
			out = append(out, e)
		}
	}
	return out
}

// BacklinksFor returns every workspace edge whose target is file,
// regardless of anchor. Use this for the "what cites this file?"
// question — IncomingEdges(file, anchor) answers the narrower
// "what targets this specific heading".
//
// IncomingEdges already drops Unresolved edges (catalog directives
// whose glob pattern hasn't been expanded) so they don't surface
// here as phantom self-backlinks on every catalog host file.
//
// Same-file citations (EdgeAnchorLink, EdgeRefLink) stay in the
// result so callers can filter on SourceFile when they want only
// external citations. The returned slice is freshly allocated and
// sorted by (SourceFile, SourceLine, SourceCol) so callers
// presenting the result to a user — or asserting on it in a
// test — see a stable order regardless of the underlying map
// iteration.
func (i *Index) BacklinksFor(file string) []Edge {
	if i == nil {
		return nil
	}
	edges := i.IncomingEdges(file, "")
	sort.Slice(edges, func(a, b int) bool {
		if edges[a].SourceFile != edges[b].SourceFile {
			return edges[a].SourceFile < edges[b].SourceFile
		}
		if edges[a].SourceLine != edges[b].SourceLine {
			return edges[a].SourceLine < edges[b].SourceLine
		}
		return edges[a].SourceCol < edges[b].SourceCol
	})
	return edges
}

// OutgoingEdges returns the edges originating in file.
func (i *Index) OutgoingEdges(file string) []Edge {
	if i == nil {
		return nil
	}
	fe, ok := i.File(file)
	if !ok {
		return nil
	}
	out := make([]Edge, len(fe.Outgoing))
	copy(out, fe.Outgoing)
	return out
}

// DependencyOrder returns paths reordered so that a file's
// generated-section dependencies come before the file itself: a file
// that <?include?>s or <?build?>s another is placed after its targets
// (leaves first). This lets a single fix sweep regenerate an upstream
// file before the downstream file that embeds it, so an
// include/catalog cascade converges in one productive pass instead of
// one pass per dependency level.
//
// Only resolved include and build edges constrain the order. Catalog
// edges are glob-based (Unresolved) and impose no constraint — the
// fix workspace fixpoint loop settles catalog sources that are
// themselves fixed. Link edges (anchor/file/ref) do not embed content
// and are ignored. Targets outside paths impose no constraint (they
// are read as-is and never fixed). Files in a dependency cycle, and
// files with no constraint, keep their original relative order.
//
// When two or more paths are given and the receiver is non-nil, they
// are normalized with NormalizePath before matching, so callers may
// pass workspace-relative paths in any equivalent spelling (a leading
// "./", OS-specific separators) without silently missing a constraint,
// and the returned slice is in that same normalized form. The input
// slice is never mutated. A nil receiver or fewer than two paths is a
// no-op: the input slice is returned unchanged (not re-normalized).
func (i *Index) DependencyOrder(paths []string) []string {
	if i == nil || len(paths) < 2 {
		return paths
	}

	// Match the index's edge targets (which are NormalizePath-normalized)
	// regardless of how the caller spelled each path. A fresh slice keeps
	// the caller's input intact.
	norm := make([]string, len(paths))
	for idx, p := range paths {
		norm[idx] = NormalizePath(p)
	}
	paths = norm

	indegree, dependents := i.dependencyEdges(paths)

	// Kahn's algorithm, seeded in input order so independent files keep
	// their original relative order.
	queue := make([]string, 0, len(paths))
	for _, p := range paths {
		if indegree[p] == 0 {
			queue = append(queue, p)
		}
	}
	out := make([]string, 0, len(paths))
	seen := make(map[string]struct{}, len(paths))
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		out = append(out, n)
		seen[n] = struct{}{}
		for _, d := range dependents[n] {
			indegree[d]--
			if indegree[d] == 0 {
				queue = append(queue, d)
			}
		}
	}

	// Any file still unseen is part of a dependency cycle; append it in
	// input order. The fix workspace fixpoint loop converges these.
	if len(out) < len(paths) {
		for _, p := range paths {
			if _, ok := seen[p]; !ok {
				out = append(out, p)
			}
		}
	}
	return out
}

// dependencyEdges builds the in-set generated-section dependency graph
// among paths. indegree[p] is the number of distinct in-set files p
// must be processed after (its resolved include/build targets), and
// dependents[d] lists the files that depend on d. indegree has an entry
// for every path; dependents is keyed only by files that are depended
// upon, so a path nothing depends on has no key — a missing key reads
// as a nil slice, which ranges as empty.
func (i *Index) dependencyEdges(paths []string) (map[string]int, map[string][]string) {
	inSet := make(map[string]struct{}, len(paths))
	for _, p := range paths {
		inSet[p] = struct{}{}
	}
	deps := make(map[string]map[string]struct{}, len(paths))
	dependents := make(map[string][]string, len(paths))
	indegree := make(map[string]int, len(paths))
	for _, p := range paths {
		indegree[p] = 0
	}
	for _, p := range paths {
		for _, e := range i.OutgoingEdges(p) {
			t, ok := orderingTarget(e, p, inSet)
			if !ok {
				continue
			}
			if _, dup := deps[p][t]; dup {
				continue
			}
			if deps[p] == nil {
				deps[p] = make(map[string]struct{})
			}
			deps[p][t] = struct{}{}
			dependents[t] = append(dependents[t], p)
			indegree[p]++
		}
	}
	return indegree, dependents
}

// orderingTarget returns e's in-set dependency target when e is a
// resolved include or build edge that constrains fix order. ok is
// false for catalog (glob) and link edges, unresolved or empty
// targets, self-edges, and targets outside the fix set.
func orderingTarget(e Edge, source string, inSet map[string]struct{}) (string, bool) {
	if e.Unresolved || (e.Kind != EdgeInclude && e.Kind != EdgeBuild) {
		return "", false
	}
	t := e.TargetFile
	// A self-edge (a file that includes itself) imposes no ordering
	// constraint. collectDirectiveEdges never emits an empty target for
	// include/build edges, and an empty target could not be in inSet
	// anyway, so the membership check below covers that case.
	if t == source {
		return "", false
	}
	if _, ok := inSet[t]; !ok {
		return "", false
	}
	return t, true
}

// FilesByKind returns workspace files whose front-matter `kinds:`
// list contains kind. Order is undefined.
func (i *Index) FilesByKind(kind string) []string {
	if i == nil || kind == "" {
		return nil
	}
	i.mu.RLock()
	defer i.mu.RUnlock()
	var out []string
	for path, fe := range i.files {
		for _, k := range fe.Kinds {
			if k == kind {
				out = append(out, path)
				break
			}
		}
	}
	return out
}

// SearchSymbols returns symbols whose name (case-insensitive)
// contains query. Match scope:
//
//   - heading text
//   - link-ref labels
//   - front-matter title (matched against the file's Title)
//   - kind names from kinds:
//
// Returns at most max entries (0 = unlimited).
func (i *Index) SearchSymbols(query string, max int) []SymbolMatch {
	if i == nil {
		return nil
	}
	q := strings.ToLower(strings.TrimSpace(query))
	i.mu.RLock()
	defer i.mu.RUnlock()
	var out []SymbolMatch
	full := func() bool { return max > 0 && len(out) >= max }
	for path, fe := range i.files {
		out = matchFileSymbols(out, path, fe, q)
		if full() {
			return out[:max]
		}
		out = matchFileTitle(out, path, fe, q)
		if full() {
			return out[:max]
		}
		out = matchFileKinds(out, path, fe, q)
		if full() {
			return out[:max]
		}
	}
	return out
}

// matchFileSymbols appends matches for headings and link refs.
func matchFileSymbols(out []SymbolMatch, path string, fe *FileEntry, q string) []SymbolMatch {
	for _, s := range fe.Symbols {
		if s.Kind != SymbolHeading && s.Kind != SymbolLinkRef {
			continue
		}
		if !nameMatches(s.Name, q) {
			continue
		}
		out = append(out, SymbolMatch{File: path, Symbol: s})
	}
	return out
}

// matchFileTitle appends a synthetic Title symbol when the file's
// front-matter title matches.
func matchFileTitle(out []SymbolMatch, path string, fe *FileEntry, q string) []SymbolMatch {
	if fe.Title == "" || !nameMatches(fe.Title, q) {
		return out
	}
	return append(out, SymbolMatch{
		File: path,
		Symbol: Symbol{
			File:          path,
			Kind:          SymbolFrontMatter,
			Name:          fe.Title,
			StartLine:     1,
			EndLine:       1,
			SelectionLine: 1,
			SelectionCol:  1,
		},
	})
}

// matchFileKinds appends one synthetic symbol per matching kind.
func matchFileKinds(out []SymbolMatch, path string, fe *FileEntry, q string) []SymbolMatch {
	for _, k := range fe.Kinds {
		if !nameMatches(k, q) {
			continue
		}
		out = append(out, SymbolMatch{
			File: path,
			Symbol: Symbol{
				File:          path,
				Kind:          SymbolFrontMatter,
				Name:          "kind:" + k,
				StartLine:     1,
				EndLine:       1,
				SelectionLine: 1,
				SelectionCol:  1,
			},
		})
	}
	return out
}

// nameMatches returns true when q is empty or a case-insensitive
// substring of name.
func nameMatches(name, q string) bool {
	if q == "" {
		return true
	}
	return strings.Contains(strings.ToLower(name), q)
}

// SymbolMatch pairs a Symbol with the file that contains it. Returned
// from workspace-wide queries so callers can build LSP locations.
type SymbolMatch struct {
	File   string
	Symbol Symbol
}

// NormalizePath returns path with forward slashes and no leading
// `./`. Empty input passes through. Backslashes are translated even
// on platforms where filepath.ToSlash is a no-op so a Windows-style
// path landing in the index from a cross-platform test still keys
// against the same slot as the slashed form.
func NormalizePath(path string) string {
	if path == "" {
		return ""
	}
	p := strings.ReplaceAll(filepath.ToSlash(path), `\`, "/")
	p = strings.TrimPrefix(p, "./")
	return p
}

// AbsPathToWorkspace returns the workspace-relative form of abs given
// the index's root directory. When abs is already relative, or when
// root is empty, the input is returned unchanged.
func (i *Index) AbsPathToWorkspace(abs string) string {
	if i == nil {
		return abs
	}
	i.mu.RLock()
	root := i.root
	i.mu.RUnlock()
	return absToWorkspace(root, abs)
}

func absToWorkspace(root, abs string) string {
	if root == "" || !filepath.IsAbs(abs) {
		return NormalizePath(abs)
	}
	rel, err := filepath.Rel(root, abs)
	if err != nil {
		return NormalizePath(abs)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return NormalizePath(abs)
	}
	return NormalizePath(rel)
}
