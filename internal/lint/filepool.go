package lint

import (
	"bytes"
	"sync"

	"github.com/jeduden/mdsmith/pkg/goldmark/arena"
	"github.com/jeduden/mdsmith/pkg/goldmark/parser"
	"github.com/jeduden/mdsmith/pkg/markdown"
)

// fileArenaPool recycles parse arenas across the engine's per-file
// lint passes. Each file's parse carved fresh slab memory that became
// garbage with the AST — roughly 40% of all allocation on a `check`
// run. Pooling trades that for slab reuse; the release closure
// returned by NewFileFromSourcePooled is the lifetime boundary.
var fileArenaPool = sync.Pool{New: func() any { return arena.New() }}

// NewFileFromSourcePooled is NewFileFromSource with the AST's slab
// memory drawn from a process-wide arena pool. release returns the
// slabs for reuse; after calling it the File's AST — and anything
// that aliases arena memory (nodes, their Segments) — must not be
// touched. Values extracted as copies (diagnostics, strings, line
// numbers) stay valid. release is idempotent and safe to defer.
// Unlike NewFileFromSource there is no error return: the parse path
// cannot fail (NewFile's error exists only for API compatibility).
//
// Use it only where the File provably dies before release: the
// engine's lintFile owns exactly that boundary. Callers that publish
// the File past the call (the LSP's ParseCache, the RunCache target
// loads) must stay on NewFileFromSource.
func NewFileFromSourcePooled(path string, source []byte, stripFrontMatter bool) (*File, func()) {
	a := fileArenaPool.Get().(*arena.Arena)
	f := newFileFromSourceArena(path, source, stripFrontMatter, a)
	var once sync.Once
	release := func() {
		once.Do(func() {
			a.Reset()
			fileArenaPool.Put(a)
		})
	}
	return f, release
}

// newFileFromSourceArena mirrors NewFileFromSource but threads the
// caller-owned arena into the parse.
func newFileFromSourceArena(path string, source []byte, stripFrontMatter bool, a *arena.Arena) *File {
	var fm []byte
	var offset int
	content := source
	if stripFrontMatter {
		fm, content = StripFrontMatter(source)
		offset = CountLines(fm)
	}

	f := newFileArena(path, content, a)
	f.FrontMatter = fm
	f.LineOffset = offset
	f.StripFrontMatter = stripFrontMatter
	return f
}

// newFileArena mirrors NewFile with a caller-owned arena.
func newFileArena(path string, source []byte, a *arena.Arena) *File {
	pc := parser.NewContext()
	node := markdown.ParseContextArena(source, pc, a)
	return &File{
		Path:     path,
		Source:   source,
		Lines:    bytes.Split(source, []byte("\n")),
		AST:      node,
		parseCtx: pc,
	}
}

// NewFileBlockOnlyPooled is NewFileFromSourcePooled restricted to
// goldmark's block phase: the returned File's AST carries block nodes
// but no inline children (see markdown.ParseBlockOnlyContextArena). It
// is a measurement-only seam for the lazy-parse spike (plan
// 2606141901) — the engine reaches it only through the opt-in,
// default-off Runner.BlockOnlyParse flag, never on a production run.
// The same arena-pool lifetime contract as NewFileFromSourcePooled
// applies: do not touch the File or its AST after release.
func NewFileBlockOnlyPooled(path string, source []byte, stripFrontMatter bool) (*File, func()) {
	a := fileArenaPool.Get().(*arena.Arena)
	f := newFileBlockOnlyArena(path, source, stripFrontMatter, a)
	var once sync.Once
	release := func() {
		once.Do(func() {
			a.Reset()
			fileArenaPool.Put(a)
		})
	}
	return f, release
}

// NewFileFlatPooled builds a File for the flat Layer-0 parse-skip path
// (plan 2606142147): it strips front matter and splits lines exactly like
// NewFileFromSourcePooled, but runs no goldmark parse at all — f.AST stays
// nil and a flat LineClassifier is attached instead, so the
// CollectCodeBlockLines and FlatHeadingLines projections serve from the
// classifier rather than the tree. The engine reaches it only when every
// enabled rule is line-capable (Runner.FlatLayer0 plus the eligibility
// gate), so no rule ever navigates the nil AST.
//
// The returned release is a no-op: there is no parse arena to recycle.
// It keeps NewFileFromSourcePooled's two-value signature so lintFile can
// swap constructors without special-casing the lifetime boundary.
func NewFileFlatPooled(path string, source []byte, stripFrontMatter bool) (*File, func()) {
	var fm []byte
	var offset int
	content := source
	if stripFrontMatter {
		fm, content = StripFrontMatter(source)
		offset = CountLines(fm)
	}
	lines := bytes.Split(content, []byte("\n"))
	f := &File{
		Path:      path,
		Source:    content,
		Lines:     lines,
		lineClass: ClassifyLines(lines),
	}
	f.FrontMatter = fm
	f.LineOffset = offset
	f.StripFrontMatter = stripFrontMatter
	return f, func() {}
}

// newFileBlockOnlyArena mirrors newFileFromSourceArena but parses only
// the block phase.
func newFileBlockOnlyArena(path string, source []byte, stripFrontMatter bool, a *arena.Arena) *File {
	var fm []byte
	var offset int
	content := source
	if stripFrontMatter {
		fm, content = StripFrontMatter(source)
		offset = CountLines(fm)
	}
	pc := parser.NewContext()
	node := markdown.ParseBlockOnlyContextArena(content, pc, a)
	f := &File{
		Path:     path,
		Source:   content,
		Lines:    bytes.Split(content, []byte("\n")),
		AST:      node,
		parseCtx: pc,
	}
	f.FrontMatter = fm
	f.LineOffset = offset
	f.StripFrontMatter = stripFrontMatter
	return f
}
