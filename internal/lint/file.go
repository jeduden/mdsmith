package lint

import (
	"bytes"
	"io/fs"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// File holds a parsed Markdown document and its source.
type File struct {
	Path        string
	Source      []byte
	Lines       [][]byte
	AST         ast.Node
	FS          fs.FS
	FrontMatter []byte
	LineOffset  int
}

// NewFile parses source as Markdown and returns a File.
func NewFile(path string, source []byte) (*File, error) {
	reader := text.NewReader(source)
	parser := goldmark.DefaultParser()
	node := parser.Parse(reader)

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
