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
//
// Use it only where the File provably dies before release: the
// engine's lintFile owns exactly that boundary. Callers that publish
// the File past the call (the LSP's ParseCache, the RunCache target
// loads) must stay on NewFileFromSource.
func NewFileFromSourcePooled(path string, source []byte, stripFrontMatter bool) (*File, func(), error) {
	a := fileArenaPool.Get().(*arena.Arena)
	f, err := newFileFromSourceArena(path, source, stripFrontMatter, a)
	if err != nil {
		a.Reset()
		fileArenaPool.Put(a)
		return nil, nil, err
	}
	var once sync.Once
	release := func() {
		once.Do(func() {
			a.Reset()
			fileArenaPool.Put(a)
		})
	}
	return f, release, nil
}

// newFileFromSourceArena mirrors NewFileFromSource but threads the
// caller-owned arena into the parse.
func newFileFromSourceArena(path string, source []byte, stripFrontMatter bool, a *arena.Arena) (*File, error) {
	var fm []byte
	var offset int
	content := source
	if stripFrontMatter {
		fm, content = StripFrontMatter(source)
		offset = CountLines(fm)
	}

	f, err := newFileArena(path, content, a)
	if err != nil {
		return nil, err
	}
	f.FrontMatter = fm
	f.LineOffset = offset
	f.StripFrontMatter = stripFrontMatter
	return f, nil
}

// newFileArena mirrors NewFile with a caller-owned arena.
func newFileArena(path string, source []byte, a *arena.Arena) (*File, error) {
	pc := parser.NewContext()
	node := markdown.ParseContextArena(source, pc, a)
	return &File{
		Path:     path,
		Source:   source,
		Lines:    bytes.Split(source, []byte("\n")),
		AST:      node,
		parseCtx: pc,
	}, nil
}
