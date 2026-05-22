package linkrefparagraph

// Vendored from goldmark@v1.8.2:
//   - parseLinkReferenceDefinition: parser/link_ref.go (top-level
//     parser-of-one-definition called from Transform)
//   - parseLinkDestination: parser/link.go:342
//   - linkFindClosureOptions: parser/link.go:255
//   - newASTReference, astReference: parser/parser.go:40, 60
//
// Body is byte-for-byte identical to upstream. Only the package
// boundary moves so the fork can reach them. See UPSTREAM_LICENSE.

import (
	"fmt"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

var linkFindClosureOptions = text.FindClosureOptions{
	Nesting: false,
	Newline: true,
	Advance: true,
}

// parseLinkReferenceDefinition is byte-for-byte vendored from goldmark
// parser/link_ref.go:60. Length matches upstream; do not refactor here.
//
//nolint:funlen // vendored 1:1 from goldmark@v1.8.2
func parseLinkReferenceDefinition(block text.Reader, pc parser.Context) (ast.Node, int, int) {
	block.SkipSpaces()
	line, _ := block.PeekLine()
	if line == nil {
		return nil, -1, -1
	}
	startLine, _ := block.Position()
	width, pos := util.IndentWidth(line, 0)
	if width > 3 {
		return nil, -1, -1
	}
	if width != 0 {
		pos++
	}
	if line[pos] != '[' {
		return nil, -1, -1
	}
	_, startPos := block.Position()
	block.Advance(pos + 1)
	segments, found := block.FindClosure('[', ']', linkFindClosureOptions)
	if !found {
		return nil, -1, -1
	}
	var label []byte
	if segments.Len() == 1 {
		label = block.Value(segments.At(0))
	} else {
		for i := range segments.Len() {
			s := segments.At(i)
			label = append(label, block.Value(s)...)
		}
	}
	if util.IsBlank(label) {
		return nil, -1, -1
	}
	if block.Peek() != ':' {
		return nil, -1, -1
	}
	block.Advance(1)
	block.SkipSpaces()
	destination, ok := parseLinkDestination(block)
	if !ok {
		return nil, -1, -1
	}
	line, _ = block.PeekLine()
	isNewLine := line == nil || util.IsBlank(line)

	endLine, _ := block.Position()
	_, spaces, _ := block.SkipSpaces()
	opener := block.Peek()
	if opener != '"' && opener != '\'' && opener != '(' {
		if !isNewLine {
			return nil, -1, -1
		}
		ref := ast.NewLinkReferenceDefinition(label, destination, nil)
		ref.Lines().Append(startPos)
		pc.AddReference(newASTReference(ref))
		return ref, startLine, endLine + 1
	}
	if spaces == 0 {
		return nil, -1, -1
	}
	block.Advance(1)
	closer := opener
	if opener == '(' {
		closer = ')'
	}
	segments, found = block.FindClosure(opener, closer, linkFindClosureOptions)
	if !found {
		if !isNewLine {
			return nil, -1, -1
		}
		ref := ast.NewLinkReferenceDefinition(label, destination, nil)
		ref.Lines().Append(startPos)
		pc.AddReference(newASTReference(ref))
		block.AdvanceLine()
		return ref, startLine, endLine + 1
	}
	var title []byte
	if segments.Len() == 1 {
		title = block.Value(segments.At(0))
	} else {
		for i := range segments.Len() {
			s := segments.At(i)
			title = append(title, block.Value(s)...)
		}
	}

	line, _ = block.PeekLine()
	if line != nil && !util.IsBlank(line) {
		if !isNewLine {
			return nil, -1, -1
		}
		ref := ast.NewLinkReferenceDefinition(label, destination, title)
		ref.Lines().Append(startPos)
		pc.AddReference(newASTReference(ref))
		return ref, startLine, endLine
	}

	endLine, _ = block.Position()
	ref := ast.NewLinkReferenceDefinition(label, destination, title)
	ref.Lines().Append(startPos)
	pc.AddReference(newASTReference(ref))
	return ref, startLine, endLine + 1
}

func parseLinkDestination(block text.Reader) ([]byte, bool) {
	block.SkipSpaces()
	line, _ := block.PeekLine()
	if block.Peek() == '<' {
		i := 1
		for i < len(line) {
			c := line[i]
			if c == '\\' && i < len(line)-1 && util.IsPunct(line[i+1]) {
				i += 2
				continue
			} else if c == '>' {
				block.Advance(i + 1)
				return line[1:i], true
			}
			i++
		}
		return nil, false
	}
	opened := 0
	i := 0
	for i < len(line) {
		c := line[i]
		if c == '\\' && i < len(line)-1 && util.IsPunct(line[i+1]) {
			i += 2
			continue
		} else if c == '(' {
			opened++
		} else if c == ')' {
			opened--
			if opened < 0 {
				break
			}
		} else if util.IsSpace(c) {
			break
		}
		i++
	}
	block.Advance(i)
	return line[:i], len(line[:i]) != 0
}

func newASTReference(v *ast.LinkReferenceDefinition) parser.Reference {
	return &astReference{v: v}
}

type astReference struct {
	v *ast.LinkReferenceDefinition
}

func (r *astReference) Label() []byte       { return r.v.Label }
func (r *astReference) Destination() []byte { return r.v.Destination }
func (r *astReference) Title() []byte       { return r.v.Title }
func (r *astReference) String() string {
	return fmt.Sprintf("Reference{Label:%s, Destination:%s, Title:%s}", r.v.Label, r.v.Destination, r.v.Title)
}
