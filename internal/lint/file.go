package lint

import (
	"bytes"
	"io/fs"
	"os"
	"sync"
	"sync/atomic"

	"github.com/jeduden/mdsmith/pkg/goldmark/ast"
	"github.com/jeduden/mdsmith/pkg/goldmark/parser"

	"github.com/jeduden/mdsmith/internal/gitignore"
	"github.com/jeduden/mdsmith/pkg/markdown"
)

// File holds a parsed Markdown document and its source.
type File struct {
	Path        string
	Source      []byte
	Lines       [][]byte
	AST         ast.Node
	FS          fs.FS
	RootFS      fs.FS
	RootDir     string
	FrontMatter []byte
	LineOffset  int

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

	// MaxInputBytes is the maximum file size in bytes that rules
	// should enforce when reading secondary files (includes, schemas,
	// cross-references). Zero or negative means unlimited.
	MaxInputBytes int64

	// GitignoreFunc is a lazy factory for the gitignore matcher.
	// It is called at most once (on first access via GetGitignore)
	// and the result is cached. Rules that do not call GetGitignore
	// never trigger matcher construction. sync.Once keeps the lazy
	// build race-free if a *File is shared across goroutines.
	GitignoreFunc func() *gitignore.Matcher
	gitignoreOnce sync.Once
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
	newlineOffsets     []int
	newlineOffsetsDone atomic.Bool
	newlineOffsetsMu   sync.Mutex

	// codeBlockLines / piBlockLines cache the line-set walks behind
	// CollectCodeBlockLines / CollectPIBlockLines. Both are pure
	// functions of the immutable f.AST, yet up to a dozen default
	// rules each called them independently — ~20 redundant full AST
	// walks per file over the 600-file check gate (plan 175
	// profiling). The cached map is shared read-only with every
	// caller; no caller mutates it. atomic.Bool + mutex matches
	// newlineOffsets above for the same closure-box reason.
	codeBlockLines     map[int]struct{}
	codeBlockLinesDone atomic.Bool
	codeBlockLinesMu   sync.Mutex

	// lineClass, when non-nil, is the flat Layer-0 line classifier built
	// in place of the goldmark parse on the engine's parse-skip path
	// (plan 2606142147, Runner.FlatLayer0). CollectCodeBlockLines and
	// FlatHeadingLines serve from it instead of walking f.AST, which is
	// nil on that path. Set only by NewFileFlatPooled; nil on every
	// normal (AST) parse, so the AST fallback is the default everywhere.
	lineClass        *LineClassifier
	piBlockLines     map[int]struct{}
	piBlockLinesDone atomic.Bool
	piBlockLinesMu   sync.Mutex

	// proseRanges caches the byte-offset projection behind ProseRanges:
	// the source spans inside prose nodes (paragraph, heading, list-item
	// and blockquote text) with code blocks, code spans, HTML, autolinks
	// and inline raw HTML excluded. It is a pure function of the
	// immutable f.AST. Plan 215 routes every Lines-only prose rule
	// (proper-name casing, forbidden text, …) through it instead of each
	// rule re-walking the tree to rediscover the same code-skipping
	// filter: one walk per file, amortized across all of them. atomic.Bool
	// + mutex matches codeBlockLines above for the same closure-box reason
	// (sync.Once would heap-allocate the build closure on the alloc gate).
	proseRanges     []Range
	proseRangesDone atomic.Bool
	proseRangesMu   sync.Mutex

	// codeSpanContent / codeSpanLiteral cache the projections behind
	// CodeSpanContentRanges / CodeSpanLiteralRanges: each inline code
	// span's text bounds and its backtick-extended literal range.
	// Several rules each re-walked the AST for these; one walk now
	// fills both. atomic.Bool + mutex matches the caches above.
	codeSpanContent []Range
	codeSpanLiteral []Range
	codeSpansDone   atomic.Bool
	codeSpansMu     sync.Mutex

	// lineStrings caches the zero-copy string views of Lines behind
	// LineStrings. atomic.Bool + mutex matches the caches above.
	lineStrings     []string
	lineStringsDone atomic.Bool
	lineStringsMu   sync.Mutex

	// parseCtx is the goldmark parser.Context produced by the one
	// parse NewFile already runs. It is the source for LinkReferences
	// so MDS053/MDS054 no longer each re-parse the whole document
	// just to read its link reference definitions — the single
	// largest hot spot on the 600-file check gate (~10% CPU, plan
	// 175 profiling). nil when the File was built as a struct literal
	// rather than via NewFile; LinkReferences then parses once on
	// demand. Released once linkRefs is materialized.
	parseCtx     parser.Context
	linkRefs     []Reference
	linkRefsDone atomic.Bool
	linkRefsMu   sync.Mutex

	// scratch backs Memo: per-Check rule memoization. A *File is
	// built fresh for each Check and discarded after, so values
	// cached here never outlive a single Check — no cross-file or
	// cross-run staleness, the same scope as the cross-file rule's
	// per-Check cache. sync.Map keeps it safe for the concurrent
	// readers the LSP may run against one document.
	scratch sync.Map

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
//
// Panic safety mirrors sync.Once: if build panics, the entry is
// still marked done (via the deferred Store) and the mutex is
// released (via the deferred Unlock), so the panic propagates
// without leaving the per-File memo in a deadlocked state.
// Subsequent calls on the same key serve the zero-value cached
// result instead of re-running build, matching upstream sync.Once.
func (f *File) Memo(key string, build func() any) any {
	ei, _ := f.scratch.LoadOrStore(key, &memoEntry{})
	e := ei.(*memoEntry)
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
// match sync.Once's "panic still marks done" semantics.
func (f *File) MemoFile(key string, build func(*File) any) any {
	ei, _ := f.scratch.LoadOrStore(key, &memoEntry{})
	e := ei.(*memoEntry)
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

// Reference is a link reference definition discovered during the parse,
// re-exported from goldmark so callers of LinkReferences need not import
// the parser package.
type Reference = parser.Reference

// SetRootDir configures the project root directory and its fs.FS together.
func (f *File) SetRootDir(dir string) {
	f.RootDir = dir
	f.RootFS = os.DirFS(dir)
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

// LinkReferences returns the link reference definitions goldmark found
// in this document. It is computed once and cached. On the normal path
// it reads the context from the parse NewFile already performed (no
// extra parse); a File built as a struct literal has no such context,
// so the first call parses Source once. The returned slice is shared
// read-only.
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
		ctx := f.parseCtx
		if ctx == nil {
			ctx = parser.NewContext()
			markdown.ParseContext(f.Source, ctx)
		}
		f.linkRefs = ctx.References()
		f.parseCtx = nil // context no longer needed; let it GC
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
		nl := make([]int, 0, bytes.Count(f.Source, lineIndexNewline))
		for i := 0; i < len(f.Source); i++ {
			if f.Source[i] == '\n' {
				nl = append(nl, i)
			}
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
// The search is inlined (sort.Search would force the comparison
// callback to capture `nl` and `offset` and escape to the heap;
// engine-bench profiling attributed ~64 k allocations per
// 10-iteration run to that closure box before plan 195 inlined
// the binary search here).
func (f *File) LineOfOffset(offset int) int {
	nl := f.lineIndex()
	lo, hi := 0, len(nl)
	for lo < hi {
		mid := int(uint(lo+hi) >> 1)
		if nl[mid] >= offset {
			hi = mid
		} else {
			lo = mid + 1
		}
	}
	return 1 + lo
}

// ColumnOfOffset converts a byte offset in Source to a 1-based column
// number on its line.
func (f *File) ColumnOfOffset(offset int) int {
	if offset > len(f.Source) {
		offset = len(f.Source)
	}
	start := offset
	for start > 0 && f.Source[start-1] != '\n' {
		start--
	}
	return offset - start + 1
}
