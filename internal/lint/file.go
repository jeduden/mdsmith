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
	Path   string
	Source []byte
	Lines  [][]byte
	AST    ast.Node
	FS     fs.FS
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
