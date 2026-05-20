package lint

import (
	"bytes"
	"io/fs"
	"os"
	"sort"
	"sync"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"

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

	// MaxInputBytes is the maximum file size in bytes that rules
	// should enforce when reading secondary files (includes, schemas,
	// cross-references). Zero or negative means unlimited.
	MaxInputBytes int64

	// GitignoreFunc is a lazy factory for the gitignore matcher.
	// It is called at most once (on first access via GetGitignore)
	// and the result is cached. Rules that do not call GetGitignore
	// never trigger matcher construction. sync.Once keeps the lazy
	// build race-free if a *File is shared across goroutines.
	GitignoreFunc func() *GitignoreMatcher
	gitignoreOnce sync.Once
	gitignoreVal  *GitignoreMatcher

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
	// not only via NewFile. sync.Once makes the lazy build safe
	// even if a *File is read from multiple goroutines (e.g. the
	// LSP serving concurrent requests for one document).
	newlineOffsets     []int
	newlineOffsetsOnce sync.Once

	// codeBlockLines / piBlockLines cache the line-set walks behind
	// CollectCodeBlockLines / CollectPIBlockLines. Both are pure
	// functions of the immutable f.AST, yet up to a dozen default
	// rules each called them independently — ~20 redundant full AST
	// walks per file over the 600-file check gate (plan 175
	// profiling). The cached map is shared read-only with every
	// caller; no caller mutates it. sync.Once keeps the lazy build
	// race-free across the LSP's concurrent readers, matching
	// newlineOffsets above.
	codeBlockLines     map[int]bool
	codeBlockLinesOnce sync.Once
	piBlockLines       map[int]bool
	piBlockLinesOnce   sync.Once

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
	linkRefsOnce sync.Once

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
	RunCache *RunCache
}

// memoEntry guards a single Memo key so build runs exactly once even
// when several rule passes (or concurrent LSP readers) race for the
// same key.
type memoEntry struct {
	once sync.Once
	val  any
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
func (f *File) Memo(key string, build func() any) any {
	ei, _ := f.scratch.LoadOrStore(key, &memoEntry{})
	e := ei.(*memoEntry)
	e.once.Do(func() { e.val = build() })
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
func (f *File) GetGitignore() *GitignoreMatcher {
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
func (f *File) LinkReferences() []Reference {
	f.linkRefsOnce.Do(func() {
		ctx := f.parseCtx
		if ctx == nil {
			ctx = parser.NewContext()
			markdown.ParseContext(f.Source, ctx)
		}
		f.linkRefs = ctx.References()
		f.parseCtx = nil // context no longer needed; let it GC
	})
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
// building it once on first use.
func (f *File) lineIndex() []int {
	f.newlineOffsetsOnce.Do(func() {
		var nl []int
		for i := 0; i < len(f.Source); i++ {
			if f.Source[i] == '\n' {
				nl = append(nl, i)
			}
		}
		f.newlineOffsets = nl
	})
	return f.newlineOffsets
}

// LineOfOffset converts a byte offset in Source to a 1-based line
// number. The line is 1 plus the number of newlines that occur
// strictly before offset (a newline exactly at offset starts the
// next line, so it does not count) — identical to a linear scan,
// but O(log n) via binary search over the cached newline index.
func (f *File) LineOfOffset(offset int) int {
	nl := f.lineIndex()
	return 1 + sort.Search(len(nl), func(i int) bool { return nl[i] >= offset })
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
