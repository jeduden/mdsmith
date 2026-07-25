package lint

import (
	"bytes"
	"io/fs"
	"sync"
	"sync/atomic"

	"github.com/jeduden/mdsmith/pkg/goldmark/ast"
	"github.com/jeduden/mdsmith/pkg/goldmark/parser"

	"github.com/jeduden/mdsmith/internal/gitignore"
	"github.com/jeduden/mdsmith/pkg/markdown"
)

// File holds a parsed Markdown document and its source. File is
// allocated once per NewFile call — the single most common
// allocation across the engine's per-file Check path — so its field
// order follows docs/development/high-performance-go.md#struct-layout
// large-to-small: every pointer/slice/map/interface field first,
// then the block of 4-byte-aligned lazy-init guards (gitignoreOnce,
// every *Done atomic.Bool, every *Mu sync.Mutex, and the two bools),
// then the two remaining 8-byte scalars last. Each guard field still
// documents which cache slice/map above it protects; only the
// declaration site moved, to let same-size fields pack without the
// per-pair padding that interleaving them costs. See
// file_size_test.go for the pinned budget.
type File struct {
	Path        string
	Source      []byte
	Lines       [][]byte
	AST         ast.Node
	FS          fs.FS
	RootFS      fs.FS
	RootDir     string
	FrontMatter []byte

	// GitignoreFunc is a lazy factory for the gitignore matcher.
	// It is called at most once (on first access via GetGitignore)
	// and the result is cached. Rules that do not call GetGitignore
	// never trigger matcher construction. sync.Once keeps the lazy
	// build race-free if a *File is shared across goroutines.
	GitignoreFunc func() *gitignore.Matcher
	gitignoreVal  *gitignore.Matcher

	// GeneratedRanges records the content line ranges of generated
	// sections (<?include?> / <?catalog?> bodies). Diagnostics whose
	// line falls within these ranges are suppressed when linting the
	// host file — the source file is responsible for those bytes.
	GeneratedRanges []LineRange

	// newlineOffsets caches the byte offset of every '\n' in Source,
	// built once on first LineOfOffset call. Without it LineOfOffset
	// rescans Source from byte 0 on every call, which made it ~24%
	// of total `mdsmith check` CPU (plan 175 profiling). Built
	// lazily because File is also constructed as a struct literal,
	// not only via NewFile. The atomic.Bool + mutex pair (instead of
	// sync.Once) avoids the per-call closure allocation the
	// `once.Do(func(){...})` form forces — that closure box was the
	// single largest non-parse allocator on the plan-195 alloc-budget
	// gate, because every rule that calls f.LineOfOffset on a fresh
	// File pays for it. The semantics still match sync.Once: build
	// runs at most once, concurrent callers serialise on the mutex,
	// a panic inside build leaves `done` set so subsequent calls
	// observe the (zero-valued) cached result rather than retrying.
	// newlineOffsetsDone / newlineOffsetsMu live in the guard block
	// below.
	newlineOffsets []int

	// codeBlockLines / piBlockLines cache the line-set walks behind
	// CollectCodeBlockLines / CollectPIBlockLines. Both are pure
	// functions of the immutable f.AST, yet up to a dozen default
	// rules each called them independently — ~20 redundant full AST
	// walks per file over the 600-file check gate (plan 175
	// profiling). The cached map is shared read-only with every
	// caller; no caller mutates it. atomic.Bool + mutex matches
	// newlineOffsets above for the same closure-box reason, in the
	// guard block below.
	codeBlockLines map[int]struct{}

	// headingTextCache memoizes HeadingTextCache's compute result per
	// (heading, base) key — see headingTextCacheKey; base disambiguates
	// the AST path (always base 0) from the parse-skipped path's
	// run-local offsets so the two never collide on one heading
	// pointer. Unlike the caches above, it is not a single lazily-built
	// value: entries accumulate one per heading as rules visit them,
	// so a plain mutex-guarded map fits better than the atomic.Bool
	// "build once" pattern (there is no single build to gate) — see
	// headingTextCacheMu in the guard block below. A plain map beats
	// routing through the sync.Map-backed scratch/Memo facility here:
	// scratch's per-key Store path is tuned for a read-mostly, stable
	// keyset, but headings are a write-once, read-a-few-times keyset
	// per File, and sync.Map's per-insert entry/dirty-map bookkeeping
	// cost more than it saved when benchmarked against
	// BenchmarkCheckCorpusLarge.
	//
	// These two fields push unsafe.Sizeof(File{}) from 640 to 656
	// bytes, crossing a Go allocator size-class boundary (641-704 all
	// round up to 704, measured) — every *File allocation costs 64
	// bytes more than the raw field growth suggests. An alternative
	// that avoids the class jump by routing the map through a single
	// Memo-cached container (paying one scratch entry per File instead
	// of dedicated fields) was benchmarked directly against this one:
	// it kept File at 640 bytes but cost 2 extra small heap objects
	// (the memo entry and the container) on every file that touches a
	// heading, which measured worse on both allocs/op and B/op than
	// this simpler direct-field version despite avoiding the class
	// jump — the extra per-file object overhead outweighed the 64
	// bytes it saved. Kept as fields for that reason; see PR #770's
	// review discussion for the numbers.
	headingTextCache map[headingTextCacheKey]string

	// lineClass, when non-nil, is the flat Layer-0 line classifier built
	// in place of the goldmark parse on the engine's parse-skip path
	// (plan 2606142147, Runner.FlatLayer0). CollectCodeBlockLines and
	// FlatHeadingLines serve from it instead of walking f.AST, which is
	// nil on that path. Set only by NewFileFlatPooled; nil on every
	// normal (AST) parse, so the AST fallback is the default everywhere.
	lineClass    *LineClassifier
	piBlockLines map[int]struct{}

	// layer0 caches the single-pass block scan (layer0.go) behind
	// Layer0. It is the block-level projection source whenever f.AST is
	// nil (the parse-skipped path) and the cheap re-backing for
	// CollectCodeBlockLines / CollectPIBlockLines once computed.
	// atomic.Bool + mutex matches the caches above for the same
	// closure-box reason, in the guard block below.
	layer0 *Layer0Scan

	// inlineBlocks caches the run-grouped per-block inline parse
	// (inline_blocks.go) behind InlineBlocks. It is the single shared
	// inline-node stream every inline rule consumes on the parse-skipped
	// path (f.AST nil), so each contiguous run of inline-bearing lines is
	// parsed once per file rather than once per rule. atomic.Bool + mutex
	// matches the caches above for the same closure-box reason, in the
	// guard block below.
	inlineBlocks []InlineBlock

	// emphasisParas caches the lone-emphasis-paragraph projection
	// (inline_emphasis.go) behind WholeParagraphEmphasis, MDS018's
	// parse-skipped source. The build is the run-grouped inline walk, or a
	// single full-document parse on the loose-list fallback path; either
	// way it runs once per file so a list-bearing file is not re-parsed on
	// a second call. atomic.Bool + mutex matches the caches above for the
	// same closure-box reason, in the guard block below.
	emphasisParas []EmphasisParagraph

	// proseRanges caches the byte-offset projection behind ProseRanges:
	// the source spans inside prose nodes (paragraph, heading, list-item
	// and blockquote text) with code blocks, code spans, HTML, autolinks
	// and inline raw HTML excluded. It is a pure function of the
	// immutable f.AST. Plan 215 routes every Lines-only prose rule
	// (proper-name casing, forbidden text, …) through it instead of each
	// rule re-walking the tree to rediscover the same code-skipping
	// filter: one walk per file, amortized across all of them. atomic.Bool
	// + mutex matches codeBlockLines above for the same closure-box
	// reason, in the guard block below (sync.Once would heap-allocate
	// the build closure on the alloc gate).
	proseRanges []Range

	// codeSpanContent / codeSpanLiteral cache the projections behind
	// CodeSpanContentRanges / CodeSpanLiteralRanges: each inline code
	// span's text bounds and its backtick-extended literal range.
	// Several rules each re-walked the AST for these; one walk now
	// fills both. atomic.Bool + mutex matches the caches above, in the
	// guard block below.
	codeSpanContent []Range
	codeSpanLiteral []Range

	// lineStrings caches the zero-copy string views of Lines behind
	// LineStrings. atomic.Bool + mutex matches the caches above, in
	// the guard block below.
	lineStrings []string

	// parseCtx is the goldmark parser.Context produced by the one
	// parse NewFile already runs. It is the source for LinkReferences
	// so MDS053/MDS054 no longer each re-parse the whole document
	// just to read its link reference definitions — the single
	// largest hot spot on the 600-file check gate (~10% CPU, plan
	// 175 profiling). nil when the File was built as a struct literal
	// rather than via NewFile; LinkReferences then parses once on
	// demand. Released once linkRefs is materialized.
	parseCtx parser.Context

	// RunCache is the engine-owned read cache shared by every File
	// processed in one engine.Run pass. Catalog and include rules
	// consult it before falling back to per-Check Memo so a target
	// globbed by N host-file catalogs is read once per run, not N
	// times. nil for struct-literal Files in unit tests; the
	// catalog rule then takes the per-Check fallback path.
	//
	// RunCache (runcache.go) and the parse cache (parsecache.go) stay in
	// this package rather than moving to siblings like the gitignore,
	// bytelimit, and piparser splits: File embeds *RunCache here, so a
	// dedicated internal/runcache package would import lint for File
	// while lint imports it for the field — a circular import. They are
	// facets of the parsed-file model, not standalone utilities, so
	// they belong with File anyway. See plan/224.
	RunCache *RunCache

	// scratch backs Memo: per-Check rule memoization. A *File is
	// built fresh for each Check and discarded after, so values
	// cached here never outlive a single Check — no cross-file or
	// cross-run staleness, the same scope as the cross-file rule's
	// per-Check cache. sync.Map keeps it safe for the concurrent
	// readers the LSP may run against one document.
	scratch sync.Map

	// linkRefs is declared last among the pointer-bearing fields on
	// purpose: as a slice, only its 8-byte data-pointer word is
	// GC-scanned when nothing after it holds a pointer, so its
	// trailing len/cap words (16 bytes) fall outside ptrdata for
	// free — unlike ending on a bare pointer field (e.g. RunCache),
	// whose single word carries no such free tail. See parseCtx's
	// comment above for what linkRefs caches.
	linkRefs []Reference

	// --- lazy-init guards -------------------------------------------
	// Every *Done atomic.Bool / *Mu sync.Mutex pair below guards the
	// same-named cache field above; see that field's comment for what
	// it caches and why. Grouping every guard here (instead of beside
	// its cache field) lets the two 4-byte-aligned kinds pack without
	// the padding a pointer-sized field would force between them —
	// see file_size_test.go.
	gitignoreOnce      sync.Once
	newlineOffsetsDone atomic.Bool
	codeBlockLinesDone atomic.Bool
	piBlockLinesDone   atomic.Bool
	layer0Done         atomic.Bool
	inlineBlocksDone   atomic.Bool
	emphasisParasDone  atomic.Bool
	proseRangesDone    atomic.Bool
	codeSpansDone      atomic.Bool
	lineStringsDone    atomic.Bool
	linkRefsDone       atomic.Bool
	newlineOffsetsMu   sync.Mutex
	codeBlockLinesMu   sync.Mutex
	piBlockLinesMu     sync.Mutex
	layer0Mu           sync.Mutex
	inlineBlocksMu     sync.Mutex
	emphasisParasMu    sync.Mutex
	proseRangesMu      sync.Mutex
	codeSpansMu        sync.Mutex
	lineStringsMu      sync.Mutex
	linkRefsMu         sync.Mutex
	headingTextCacheMu sync.Mutex

	// StripFrontMatter records whether this file was parsed in
	// front-matter-stripping mode. Rules that read other files
	// from the corpus should mirror the same mode so that line
	// numbers in cross-file diagnostics are computed against the
	// same coordinate system as the current file.
	StripFrontMatter bool

	// DryRun, when true, signals that the surrounding fix run must
	// not touch the filesystem or the git index. Fixable rules whose
	// Fix method has side effects beyond returning the new file
	// bytes (e.g. writing a sibling repo file, staging via git)
	// must check this flag and skip the side effect.
	DryRun bool

	LineOffset int

	// MaxInputBytes is the maximum file size in bytes that rules
	// should enforce when reading secondary files (includes, schemas,
	// cross-references). Zero or negative means unlimited.
	MaxInputBytes int64
}

// memoEntry guards a single Memo key so build runs exactly once even
// when several rule passes (or concurrent LSP readers) race for the
// same key. atomic.Bool + mutex is used instead of sync.Once because
// once.Do takes a function value as a parameter — the closure
// `func() { e.val = build() }` Memo would pass captures `e` and
// `build`, both escape-tracking pointers, so it allocates per call.
// On hot per-File memos (astutil.CollectSectionParagraphs feeds
// every paragraph-aware rule), that single closure escape is the
// dominant per-Check allocation the MDS024 budget gate sees. The
// atomic flag is a double-checked-lock pattern: cheap atomic load
// on the warm path, mutex-guarded build on the cold path.
type memoEntry struct {
	val  any
	done atomic.Bool
	mu   sync.Mutex
}

// Memo returns the value for key, computing it once via build on the
// first request within this File's lifetime and serving the cached
// value thereafter. It exists so a rule whose passes would otherwise
// recompute the same expensive per-Check derivation can share one
// result: the catalog directive's resolved entries, for example, are
// otherwise rebuilt by the generate, injection, and case-mismatch
// passes — three globs and front-matter reads of every matched file
// per directive. The File is discarded after each Check, so nothing
// is cached across files or runs.
//
// build is invoked directly (no wrapping closure) so the call adds
// no per-Memo-call allocation beyond the cold-path memoEntry itself.
// The warm path checks Load before LoadOrStore for the same reason:
// LoadOrStore's second argument (&memoEntry{}) is constructed before
// the call and discarded whenever the key already exists, so a plain
// LoadOrStore would allocate one on every call regardless of hit or
// miss.
//
// Panic safety mirrors sync.Once: if build panics, the entry is
// still marked done (via the deferred Store) and the mutex is
// released (via the deferred Unlock), so the panic propagates
// without leaving the per-File memo in a deadlocked state.
// Subsequent calls on the same key serve the zero-value cached
// result instead of re-running build, matching upstream sync.Once.
func (f *File) Memo(key string, build func() any) any {
	if v, ok := f.scratch.Load(key); ok {
		return memoLoad(v.(*memoEntry), build)
	}
	ei, _ := f.scratch.LoadOrStore(key, &memoEntry{})
	return memoLoad(ei.(*memoEntry), build)
}

// memoLoad runs build at most once for e, then returns the cached
// value — the double-checked-lock body shared by Memo and MemoFile.
func memoLoad(e *memoEntry, build func() any) any {
	if e.done.Load() {
		return e.val
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.done.Load() {
		defer e.done.Store(true)
		e.val = build()
	}
	return e.val
}

// MemoFile is the *File-passing variant of Memo: build receives this
// File as an argument instead of capturing it in a closure. Callers
// whose build needs nothing beyond File data can pass a package-
// level function value, which avoids the per-call closure allocation
// the plain `Memo` form forces on every invocation. The hot
// astutil.CollectSectionParagraphs path is the canonical user.
//
// Panic safety matches Memo's contract: defer Unlock + defer
// done.Store(true) keep the per-entry mutex from leaking a lock and
// match sync.Once's "panic still marks done" semantics. The warm
// path checks Load before LoadOrStore for the same reason Memo does.
func (f *File) MemoFile(key string, build func(*File) any) any {
	var e *memoEntry
	if v, ok := f.scratch.Load(key); ok {
		e = v.(*memoEntry)
	} else {
		ei, _ := f.scratch.LoadOrStore(key, &memoEntry{})
		e = ei.(*memoEntry)
	}
	if e.done.Load() {
		return e.val
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.done.Load() {
		defer e.done.Store(true)
		e.val = build(f)
	}
	return e.val
}

// headingTextCacheKey pairs a heading node with the base offset its
// text was (or would be) extracted with. HeadingText and
// HeadingTextBase agree on a heading's text only when base == 0
// (HeadingTextBase(h, src, 0) == HeadingText(h, src), per
// astutil_test.go's own equivalence tests), so a heading pointer
// alone is not a safe cache key: the AST path always queries base 0,
// and the parse-skipped path always queries its own run-local base,
// and today the two are mutually exclusive per File (every caller
// gates on f.AST == nil, and internal/lint's inline-block arena never
// recycles a node pointer within one File's lifetime) — but nothing
// in the type system enforces that invariant. Including base means a
// future caller that queries the same heading at two different bases
// gets two independent entries instead of the second one silently
// reading the first one's answer.
type headingTextCacheKey struct {
	heading *ast.Heading
	base    int
}

// HeadingTextCache memoizes compute's result for (heading, base),
// keyed by the heading node's pointer identity plus base — see
// headingTextCacheKey. A plain mutex-guarded map is used rather than
// the sync.Map-backed scratch facility behind Memo/MemoFile: headings
// are a write-once, read-a-few-times keyset per File (a handful of
// headings, each queried by a handful of rules), and sync.Map's
// per-insert entry/dirty-map bookkeeping cost more in benchmarking
// than the redundant computation it avoided — sync.Map is tuned for a
// stable, read-mostly keyset, not this shape.
//
// Several default rules read a heading's text this way — no-trailing-
// punctuation and no-duplicate-headings for every heading;
// heading-increment and first-line-heading too, on a subset gated by
// their own rule-specific conditions — so more than one can
// independently walk the same heading's children within one Check
// pass over f. astutil.HeadingText's buf.String() alone was 28% of
// BenchmarkCheckCorpusLarge's total allocations (68% of that from
// HeadingText/HeadingTextBase, per docs/development/high-performance
// -go.md's memoization guidance) — this cache lets only the first
// caller for a given (heading, base) pay for the walk and the string
// conversion.
func (f *File) HeadingTextCache(heading *ast.Heading, base int, compute func() string) string {
	key := headingTextCacheKey{heading: heading, base: base}
	f.headingTextCacheMu.Lock()
	defer f.headingTextCacheMu.Unlock()
	if v, ok := f.headingTextCache[key]; ok {
		return v
	}
	v := compute()
	if f.headingTextCache == nil {
		f.headingTextCache = make(map[headingTextCacheKey]string, 4)
	}
	f.headingTextCache[key] = v
	return v
}

// Reference is a link reference definition discovered during the parse,
// re-exported from goldmark so callers of LinkReferences need not import
// the parser package.
type Reference = parser.Reference

// SetRootDir configures the project root directory and its fs.FS together.
// The returned fs.FS is backed by os.OpenRoot so that symlinks inside the
// workspace cannot escape to files outside dir (RESOLVE_BENEATH semantics).
func (f *File) SetRootDir(dir string) {
	f.RootDir = dir
	f.RootFS = OpenRootFS(dir)
}

// GetGitignore returns the gitignore matcher for this file, creating it
// lazily on first call. Returns nil if no GitignoreFunc was configured.
func (f *File) GetGitignore() *gitignore.Matcher {
	f.gitignoreOnce.Do(func() {
		if f.GitignoreFunc != nil {
			f.gitignoreVal = f.GitignoreFunc()
		}
	})
	return f.gitignoreVal
}

// NewParser returns mdsmith's canonical goldmark parser, forwarded
// from pkg/markdown. Rules that need to re-inspect a document (for
// example, to consult the link reference definition map) should use
// this so that processing-instruction blocks and other
// mdsmith-specific parsing decisions stay consistent with the
// original lint parse.
func NewParser() parser.Parser {
	return markdown.NewParser()
}

// NewPooledParser forwards markdown.NewPooledParser for callers that
// place the parser into a sync.Pool.  The returned reset closure
// MUST be invoked before returning the parser to the pool; otherwise
// the pool slot retains the last parsed document's source bytes via
// the link-ref transformer's reusable BlockReader.
func NewPooledParser() (parser.Parser, func()) {
	return markdown.NewPooledParser()
}

// NewFile parses source as Markdown and returns a File. The parse
// itself is delegated to pkg/markdown's pooled canonical parser, so a
// single goldmark configuration backs every parse path.
func NewFile(path string, source []byte) (*File, error) {
	pc := parser.NewContext()
	node := markdown.ParseContext(source, pc)

	lines := bytes.Split(source, []byte("\n"))

	return &File{
		Path:     path,
		Source:   source,
		Lines:    lines,
		AST:      node,
		parseCtx: pc,
	}, nil
}

// NewFileLines builds a File from source without parsing the goldmark
// AST: it sets Source and Lines and leaves AST nil. It is the parse-
// skipped constructor for the Layer 0 gate — when every enabled rule
// resolves to Layer 0, the block-level projections
// (CollectCodeBlockLines, CollectPIBlockLines) serve from the Layer 0
// scan instead of the tree, so the goldmark parse is pure waste. A File
// built this way must only be linted by rules that never navigate f.AST;
// the engine gate is responsible for that precondition.
func NewFileLines(path string, source []byte) *File {
	return &File{
		Path:   path,
		Source: source,
		Lines:  bytes.Split(source, []byte("\n")),
	}
}

// NewFileLinesFromSource is NewFileFromSource's parse-skipping sibling: it
// strips front matter (when stripFrontMatter is set), records the prefix
// and line offset exactly as NewFileFromSource does, and builds the body
// File via NewFileLines so AST stays nil. The engine's Layer 0 gate uses
// it when every enabled rule resolves to Layer 0, so the goldmark parse is
// skipped entirely. The resulting File must only be linted by Layer 0
// rules — the gate guarantees that precondition.
func NewFileLinesFromSource(path string, source []byte, stripFrontMatter bool) *File {
	var fm []byte
	var offset int
	content := source
	if stripFrontMatter {
		fm, content = StripFrontMatter(source)
		offset = CountLines(fm)
	}
	f := NewFileLines(path, content)
	f.FrontMatter = fm
	f.LineOffset = offset
	f.StripFrontMatter = stripFrontMatter
	return f
}

// LinkReferences returns the link reference definitions goldmark found
// in this document. It is computed once and cached.
//
// Three paths, in priority order:
//
//  1. The File came from NewFile, so the parse already collected the
//     references into parseCtx — read them, no extra work.
//  2. The File has no parse context (the parse-skipped Layer 0/1 path,
//     or a struct-literal File) AND its block structure is safe for the
//     byte-level reference scanner (no `]:` nested in a block quote or
//     list the scanner does not descend into) — scan the paragraph
//     heads, no goldmark parse.
//  3. Otherwise fall back to a single lazy full parse, which guarantees
//     byte-identity for the rare nested-definition shapes.
//
// Memoised via the double-checked atomic.Bool + mutex pair rather
// than sync.Once so the build path does not heap-allocate the
// `func(){...}` once.Do would otherwise force — see the
// newlineOffsets field comment for why this pattern is preferred
// on the alloc-budget hot path.
func (f *File) LinkReferences() []Reference {
	if f.linkRefsDone.Load() {
		return f.linkRefs
	}
	f.linkRefsMu.Lock()
	defer f.linkRefsMu.Unlock()
	if !f.linkRefsDone.Load() {
		defer f.linkRefsDone.Store(true)
		switch {
		case f.parseCtx != nil:
			f.linkRefs = f.parseCtx.References()
			f.parseCtx = nil // context no longer needed; let it GC
		case f.Lines != nil && !scanNeedsFallback(f):
			// byte-level scanner: Lines already populated at construction
			// (NewFile/NewFileLines), so no write to f.Lines is needed
			// here and the scanner is safe to call without a data race.
			f.linkRefs = scanLinkReferences(f)
		default:
			ctx := parser.NewContext()
			markdown.ParseContext(f.Source, ctx)
			f.linkRefs = ctx.References()
		}
	}
	return f.linkRefs
}

// NewFileFromSource creates a File from raw source bytes. When
// stripFrontMatter is true it strips YAML front matter, stores
// the prefix in FrontMatter, computes LineOffset via CountLines,
// and parses only the stripped content.
func NewFileFromSource(path string, source []byte, stripFrontMatter bool) (*File, error) {
	var fm []byte
	var offset int
	content := source
	if stripFrontMatter {
		fm, content = StripFrontMatter(source)
		offset = CountLines(fm)
	}

	f, err := NewFile(path, content)
	if err != nil {
		return nil, err
	}
	f.FrontMatter = fm
	f.LineOffset = offset
	f.StripFrontMatter = stripFrontMatter
	return f, nil
}

// AdjustDiagnostics adds the file's LineOffset to each diagnostic's Line.
func (f *File) AdjustDiagnostics(diags []Diagnostic) {
	if f.LineOffset == 0 {
		return
	}
	for i := range diags {
		diags[i].Line += f.LineOffset
	}
}

// FullSource prepends the stored FrontMatter to body.
// It allocates a new slice to avoid mutating FrontMatter's backing array.
func (f *File) FullSource(body []byte) []byte {
	if len(f.FrontMatter) == 0 {
		return body
	}
	out := make([]byte, 0, len(f.FrontMatter)+len(body))
	out = append(out, f.FrontMatter...)
	out = append(out, body...)
	return out
}

// lineIndex returns the cached offsets of every '\n' in Source,
// building it once on first use. The size hint
// `bytes.Count(f.Source, "\n")` lets the loop append into a
// right-sized backing slice instead of geometrically growing from
// cap 0, which on a 150-line synthetic doc pays ~8 grow allocations
// per file before the slice settles. The atomic.Bool + mutex memo
// avoids the closure box once.Do would otherwise force (see the
// newlineOffsets field comment).
func (f *File) lineIndex() []int {
	if f.newlineOffsetsDone.Load() {
		return f.newlineOffsets
	}
	f.newlineOffsetsMu.Lock()
	defer f.newlineOffsetsMu.Unlock()
	if !f.newlineOffsetsDone.Load() {
		defer f.newlineOffsetsDone.Store(true)
		// bytes.IndexByte is SIMD-accelerated; a hand-rolled byte loop is
		// not vectorized by the compiler and showed up at ~5% of a parity
		// check on long prose (the Rust Book benchmark corpus). Walk the
		// newlines with IndexByte instead, pre-sizing from bytes.Count
		// (also SIMD) so the append never re-grows. Same offsets, no per-
		// call allocation beyond the one pre-sized slice.
		nl := make([]int, 0, bytes.Count(f.Source, lineIndexNewline))
		for base := 0; ; {
			i := bytes.IndexByte(f.Source[base:], '\n')
			if i < 0 {
				break
			}
			nl = append(nl, base+i)
			base += i + 1
		}
		f.newlineOffsets = nl
	}
	return f.newlineOffsets
}

var lineIndexNewline = []byte{'\n'}

// LineOfOffset converts a byte offset in Source to a 1-based line
// number. The line is 1 plus the number of newlines that occur
// strictly before offset (a newline exactly at offset starts the
// next line, so it does not count) — identical to a linear scan,
// but O(log n) via binary search over the cached newline index.
func (f *File) LineOfOffset(offset int) int {
	nl := f.lineIndex()
	return 1 + newlineSearch(nl, offset)
}

// ColumnOfOffset converts a byte offset in Source to a 1-based column
// number on its line.
func (f *File) ColumnOfOffset(offset int) int {
	if offset > len(f.Source) {
		offset = len(f.Source)
	}
	if offset < 0 {
		offset = 0
	}
	// Reuses LineOfOffset's cached newline index via the same binary
	// search instead of scanning backward from offset byte by byte —
	// O(log n) in the newline count instead of O(line length). A single
	// very long line (a minified table, a long URL list) used to make
	// every diagnostic on that line pay for a full backward scan.
	nl := f.lineIndex()
	lo := newlineSearch(nl, offset)
	start := 0
	if lo > 0 {
		start = nl[lo-1] + 1
	}
	return offset - start + 1
}

// newlineSearch returns the index of the first entry in nl (a sorted
// list of newline byte offsets) that is >= offset, or len(nl) if none.
// Inlined rather than sort.Search: sort.Search's comparison closure
// would capture nl and offset and escape to the heap (engine-bench
// profiling attributed ~64 k allocations per 10-iteration run to that
// closure box before plan 195 inlined the binary search here).
func newlineSearch(nl []int, offset int) int {
	lo, hi := 0, len(nl)
	for lo < hi {
		mid := int(uint(lo+hi) >> 1)
		if nl[mid] >= offset {
			hi = mid
		} else {
			lo = mid + 1
		}
	}
	return lo
}
