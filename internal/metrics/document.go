package metrics

import (
	"bytes"
	"fmt"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/mdtext"
	"github.com/yuin/goldmark/ast"
)

// Document is the shared metric input for a single Markdown file.
// Expensive derived values are computed lazily and cached.
type Document struct {
	Path   string
	Source []byte

	file      *lint.File
	fileReady bool
	fileErr   error

	plainText      string
	plainTextReady bool
	plainTextErr   error

	wordCount      int
	wordCountReady bool
	wordCountErr   error

	headingCount      int
	headingCountReady bool
	headingCountErr   error
}

// NewDocument constructs a Document wrapper for metric computation.
func NewDocument(path string, source []byte) *Document {
	return &Document{
		Path:   path,
		Source: source,
	}
}

// ByteCount returns raw file byte count.
func (d *Document) ByteCount() int {
	return len(d.Source)
}

// LineCount returns content line count.
func (d *Document) LineCount() int {
	if len(d.Source) == 0 {
		return 0
	}
	lines := bytes.Count(d.Source, []byte("\n"))
	if d.Source[len(d.Source)-1] != '\n' {
		lines++
	}
	return lines
}

// File returns the parsed Markdown file.
func (d *Document) File() (*lint.File, error) {
	if d.fileReady {
		return d.file, d.fileErr
	}

	f, err := lint.NewFile(d.Path, d.Source)
	if err != nil {
		d.fileErr = fmt.Errorf("parsing markdown: %w", err)
		d.fileReady = true
		return nil, d.fileErr
	}

	d.file = f
	d.fileReady = true
	return d.file, nil
}

// PlainText returns plain text extracted from the Markdown AST.
func (d *Document) PlainText() (string, error) {
	if d.plainTextReady {
		return d.plainText, d.plainTextErr
	}

	f, err := d.File()
	if err != nil {
		d.plainTextErr = err
		d.plainTextReady = true
		return "", err
	}

	d.plainText = mdtext.ExtractPlainText(f.AST, f.Source)
	d.plainTextReady = true
	return d.plainText, nil
}

// WordCount returns word count on extracted plain text.
func (d *Document) WordCount() (int, error) {
	if d.wordCountReady {
		return d.wordCount, d.wordCountErr
	}

	text, err := d.PlainText()
	if err != nil {
		d.wordCountErr = err
		d.wordCountReady = true
		return 0, err
	}

	d.wordCount = mdtext.CountWords(text)
	d.wordCountReady = true
	return d.wordCount, nil
}

// HeadingCount returns number of heading nodes.
func (d *Document) HeadingCount() (int, error) {
	if d.headingCountReady {
		return d.headingCount, d.headingCountErr
	}

	f, err := d.File()
	if err != nil {
		d.headingCountErr = err
		d.headingCountReady = true
		return 0, err
	}

	count := 0
	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering {
			if _, ok := n.(*ast.Heading); ok {
				count++
			}
		}
		return ast.WalkContinue, nil
	})

	d.headingCount = count
	d.headingCountReady = true
	return d.headingCount, nil
}
