package lint

import (
	"bytes"
	"io/fs"
	"os"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
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
	// never trigger matcher construction.
	GitignoreFunc func() *GitignoreMatcher
	gitignoreOnce bool
	gitignoreVal  *GitignoreMatcher

	// GeneratedRanges records the content line ranges of generated
	// sections (<?include?> / <?catalog?> bodies). Diagnostics whose
	// line falls within these ranges are suppressed when linting the
	// host file — the source file is responsible for those bytes.
	GeneratedRanges []LineRange
}

// SetRootDir configures the project root directory and its fs.FS together.
func (f *File) SetRootDir(dir string) {
	f.RootDir = dir
	f.RootFS = os.DirFS(dir)
}

// GetGitignore returns the gitignore matcher for this file, creating it
// lazily on first call. Returns nil if no GitignoreFunc was configured.
func (f *File) GetGitignore() *GitignoreMatcher {
	if f.gitignoreOnce {
		return f.gitignoreVal
	}
	f.gitignoreOnce = true
	if f.GitignoreFunc != nil {
		f.gitignoreVal = f.GitignoreFunc()
	}
	return f.gitignoreVal
}

// NewParser returns a goldmark parser configured identically to the one
// used by NewFile. Rules that need to re-inspect a document (for example,
// to consult the link reference definition map) should use this so that
// processing-instruction blocks and other mdsmith-specific parsing
// decisions stay consistent with the original lint parse.
func NewParser() parser.Parser {
	return parser.NewParser(
		parser.WithBlockParsers(
			append(parser.DefaultBlockParsers(),
				PIBlockParserPrioritized(),
			)...,
		),
		parser.WithInlineParsers(
			parser.DefaultInlineParsers()...,
		),
		parser.WithParagraphTransformers(
			parser.DefaultParagraphTransformers()...,
		),
	)
}

// NewFile parses source as Markdown and returns a File.
func NewFile(path string, source []byte) (*File, error) {
	reader := text.NewReader(source)
	node := NewParser().Parse(reader)

	lines := bytes.Split(source, []byte("\n"))

	return &File{
		Path:   path,
		Source: source,
		Lines:  lines,
		AST:    node,
	}, nil
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

// LineOfOffset converts a byte offset in Source to a 1-based line number.
func (f *File) LineOfOffset(offset int) int {
	line := 1
	for i := 0; i < offset && i < len(f.Source); i++ {
		if f.Source[i] == '\n' {
			line++
		}
	}
	return line
}
