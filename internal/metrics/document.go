package metrics

import (
	"bytes"
	"fmt"

	"github.com/jeduden/mdsmith/internal/archetype/gensection"
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

	// authoredSource is the file source with generated-section content
	// (lines between <?include?> and <?catalog?> markers) removed.
	// It is computed lazily by AuthoredSource().
	authoredSource      []byte
	authoredSourceReady bool

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

// AuthoredSource returns the file source with generated-section content
// (bytes between <?include?> and <?catalog?> markers) removed. This
// represents only the bytes the author directly wrote — the embedded
// bytes are attributed to their source files.
//
// If parsing fails the full Source is returned unchanged.
func (d *Document) AuthoredSource() []byte {
	if d.authoredSourceReady {
		return d.authoredSource
	}
	d.authoredSourceReady = true

	f, err := d.File()
	if err != nil {
		d.authoredSource = d.Source
		return d.authoredSource
	}

	ranges := gensection.FindGeneratedLineRanges(f)
	if len(ranges) == 0 {
		d.authoredSource = d.Source
		return d.authoredSource
	}

	// Remove the content lines of each generated range.
	// f.Lines matches f.Source (already split by bytes.Split on '\n').
	// Build output by copying only non-generated lines.
	var buf bytes.Buffer
	for i, line := range f.Lines {
		lineNum := i + 1 // 1-based
		if gensection.InGeneratedRange(lineNum, ranges) {
			continue
		}
		buf.Write(line)
		if i < len(f.Lines)-1 {
			buf.WriteByte('\n')
		}
	}
	d.authoredSource = buf.Bytes()
	return d.authoredSource
}

// ByteCount returns the authored byte count: the file size excluding any
// bytes that fall inside generated sections (<?include?> or <?catalog?>
// bodies). This ensures a host file's byte metric counts only its own
// authored content, matching the lint-once attribution model.
func (d *Document) ByteCount() int {
	return len(d.AuthoredSource())
}

// LineCount returns the authored line count, excluding lines that fall
// inside generated sections. See ByteCount for the rationale.
func (d *Document) LineCount() int {
	src := d.AuthoredSource()
	if len(src) == 0 {
		return 0
	}
	lines := bytes.Count(src, []byte("\n"))
	if src[len(src)-1] != '\n' {
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
