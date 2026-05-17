package index

import (
	"bytes"
	"regexp"
	"strings"

	"github.com/jeduden/mdsmith/internal/linkgraph"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

// CompletionTag classifies the context in which a completion was triggered.
type CompletionTag int

const (
	// CompletionNone means the cursor is not in a completable context.
	CompletionNone CompletionTag = iota
	// CompletionAnchorCurrentFile means the cursor is inside [text](#prefix.
	CompletionAnchorCurrentFile
	// CompletionAnchorOtherFile means the cursor is inside [text](path.md#prefix.
	CompletionAnchorOtherFile
	// CompletionRefLabel means the cursor is inside [text][prefix.
	CompletionRefLabel
	// CompletionKindValue means the cursor is on a front-matter kind:/kinds: value.
	CompletionKindValue
	// CompletionDirectivePath means the cursor is on a directive file/source/glob arg.
	CompletionDirectivePath
)

// CompletionContext is the result of Locator.CompletionContext.
type CompletionContext struct {
	Tag CompletionTag
	// Prefix is the partial text typed so far.
	Prefix string
	// TargetFile is the workspace-relative target file for CompletionAnchorOtherFile.
	TargetFile string
	// DirectiveName is the directive name for CompletionDirectivePath.
	DirectiveName string
	// DirectiveArg is the directive argument key for CompletionDirectivePath.
	DirectiveArg string
	// FrontMatterKey is "kind" or "kinds" for CompletionKindValue.
	FrontMatterKey string
}

// compAnchorCurrentFileRE matches [text](#prefix at end of string.
// The (?:^|[^!]) prefix excludes image links (![ prefix).
// Group 1 captures the partial anchor prefix after '#'.
var compAnchorCurrentFileRE = regexp.MustCompile(`(?:^|[^!])\[[^\]]*\]\(#([^)#\s]*)$`)

// compAnchorOtherFileRE matches [text](path.md#prefix at end of string.
// Group 1 captures the file path, group 2 the partial anchor prefix.
// The extension match is case-insensitive so .MD/.MARKDOWN also trigger.
var compAnchorOtherFileRE = regexp.MustCompile(`(?:^|[^!])\[[^\]]*\]\(([^)#\s]+\.(?i:md|markdown))#([^)#\s]*)$`)

// compRefLabelRE matches [text][prefix at end of string.
// Group 1 captures the partial label prefix.
var compRefLabelRE = regexp.MustCompile(`(?:^|[^!])\[[^\]]*\]\[([^\][]*)$`)

// CompletionContext determines the completion context at (line, col) in
// source. line and col are 1-based. Returns a CompletionContext describing
// the trigger context and the prefix typed so far.
//
// The function operates on whatever bytes the caller hands in, so the LSP
// layer can call it on the live editor buffer without first landing the
// change in the index.
func (l Locator) CompletionContext(source []byte, line, col int) CompletionContext {
	if line < 1 {
		line = 1
	}
	if col < 1 {
		col = 1
	}
	fmBytes, body := lint.StripFrontMatter(source)
	fmOffset := 0
	if len(fmBytes) > 0 {
		fmOffset = bytes.Count(fmBytes, []byte{'\n'})
	}
	if line <= fmOffset {
		return completionContextFrontMatter(fmBytes, line, col)
	}
	bodyLine := line - fmOffset
	bodyLines := bytes.Split(body, []byte("\n"))
	if bodyLine < 1 || bodyLine > len(bodyLines) {
		return CompletionContext{Tag: CompletionNone}
	}
	return completionContextBody(l.Path, body, bodyLines, bodyLine, col)
}

// completionContextFrontMatter handles completion when the cursor is inside
// the YAML front matter.
func completionContextFrontMatter(fmBytes []byte, line, col int) CompletionContext {
	res := locateInFrontMatter(fmBytes, line, col)
	if res.Tag == TokenFrontMatterValue && (res.FrontMatterKey == "kind" || res.FrontMatterKey == "kinds") {
		return CompletionContext{
			Tag:            CompletionKindValue,
			Prefix:         res.FrontMatterValue,
			FrontMatterKey: res.FrontMatterKey,
		}
	}
	return CompletionContext{Tag: CompletionNone}
}

// completionContextBody handles completion when the cursor is in the document
// body (below any front matter).
func completionContextBody(srcPath string, body []byte, bodyLines [][]byte, bodyLine, col int) CompletionContext {
	root := lint.NewParser().Parse(text.NewReader(body), parser.WithContext(parser.NewContext()))
	if insideCodeBlock(root, body, bodyLine) {
		return CompletionContext{Tag: CompletionNone}
	}
	var foundPI *lint.ProcessingInstruction
	_ = ast.Walk(root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if pi, ok := n.(*lint.ProcessingInstruction); ok {
			if piContainsLine(body, pi, bodyLine) {
				foundPI = pi
			}
		}
		return ast.WalkContinue, nil
	})
	if foundPI != nil {
		return directiveCompletionContext(foundPI, bodyLines, bodyLine, col)
	}
	return completionContextLinks(srcPath, bodyLines, bodyLine, col)
}

// completionContextLinks checks for anchor and ref-label completion patterns
// on the current line up to the cursor position.
func completionContextLinks(srcPath string, bodyLines [][]byte, bodyLine, col int) CompletionContext {
	currentLine := bodyLines[bodyLine-1]
	cursorByteCol := col - 1
	if cursorByteCol > len(currentLine) {
		cursorByteCol = len(currentLine)
	}
	lineStr := string(currentLine[:cursorByteCol])
	if m := compAnchorOtherFileRE.FindStringSubmatch(lineStr); m != nil {
		return CompletionContext{
			Tag:        CompletionAnchorOtherFile,
			Prefix:     m[2],
			TargetFile: linkgraph.ResolveRelTarget(srcPath, m[1]),
		}
	}
	if m := compAnchorCurrentFileRE.FindStringSubmatch(lineStr); m != nil {
		return CompletionContext{Tag: CompletionAnchorCurrentFile, Prefix: m[1]}
	}
	if m := compRefLabelRE.FindStringSubmatch(lineStr); m != nil {
		return CompletionContext{Tag: CompletionRefLabel, Prefix: m[1]}
	}
	return CompletionContext{Tag: CompletionNone}
}

// piArgCompletion maps a (directiveName, argKey, argVal) triple to a
// CompletionDirectivePath context. Returns (ctx, true) on a recognised pair.
func piArgCompletion(piName, argKey, argVal string) (CompletionContext, bool) {
	type pair struct{ name, key string }
	m := map[pair]string{
		{"include", "file"}: "file",
		{"build", "source"}: "source",
		{"catalog", "glob"}: "glob",
	}
	if arg, ok := m[pair{piName, argKey}]; ok {
		return CompletionContext{
			Tag:           CompletionDirectivePath,
			Prefix:        argVal,
			DirectiveName: piName,
			DirectiveArg:  arg,
		}, true
	}
	return CompletionContext{}, false
}

// directiveCompletionContext returns the completion context for a cursor
// inside a processing instruction. Handles include/build/catalog directives.
//
// The prefix is extracted from the text up to the cursor (not the full line)
// so partial values like `file: "docs/g` are returned correctly even when
// the line later has a closing quote.
func directiveCompletionContext(pi *lint.ProcessingInstruction, lines [][]byte, line, col int) CompletionContext {
	if line < 1 || line > len(lines) {
		return CompletionContext{Tag: CompletionNone}
	}

	lineBytes := lines[line-1]
	cursorByteCol := col - 1
	if cursorByteCol < 0 {
		cursorByteCol = 0
	}
	if cursorByteCol > len(lineBytes) {
		cursorByteCol = len(lineBytes)
	}
	lineUpToCursor := string(lineBytes[:cursorByteCol])

	// Try key: value on the cursor line, first raw (multi-line PI) then with
	// the "<?name " opener stripped (single-line PI like <?include file: "…"?>).
	m := piArgRE.FindStringSubmatch(lineUpToCursor)
	if m == nil {
		if stripped, ok := strings.CutPrefix(lineUpToCursor, "<?"+pi.Name+" "); ok {
			m = piArgRE.FindStringSubmatch(stripped)
		}
	}
	if len(m) >= 3 {
		argVal := strings.Trim(strings.TrimSpace(m[2]), `"'`)
		if ctx, ok := piArgCompletion(pi.Name, m[1], argVal); ok {
			return ctx
		}
	}

	// Check if the cursor is on a YAML list item line under glob: in a catalog PI.
	if pi.Name == "catalog" {
		if item, ok := yamlListItemValue(lineUpToCursor); ok {
			// line-1 is the 0-based index of the current line.
			parentKey := scanBackwardForPIKey(lines, line-1)
			if parentKey == "glob" {
				return CompletionContext{
					Tag:           CompletionDirectivePath,
					Prefix:        item,
					DirectiveName: "catalog",
					DirectiveArg:  "glob",
				}
			}
		}
	}

	return CompletionContext{Tag: CompletionNone}
}

// yamlListItemValue returns the trimmed value from a YAML block-list line
// ("  - value" or "-" for an empty item). Returns ("", false) for non-list lines.
func yamlListItemValue(row string) (string, bool) {
	trimmed := strings.TrimLeft(row, " \t")
	if !strings.HasPrefix(trimmed, "- ") && trimmed != "-" {
		return "", false
	}
	if trimmed == "-" {
		return "", true
	}
	val := strings.TrimPrefix(trimmed, "- ")
	val = strings.Trim(strings.TrimSpace(val), `"'`)
	return val, true
}

// scanBackwardForPIKey scans backward from the 0-based line index idx to
// find the most recent YAML key line (not a list item). Returns the key name
// or "".
func scanBackwardForPIKey(lines [][]byte, idx int) string {
	for i := idx - 1; i >= 0; i-- {
		row := string(lines[i])
		trimmed := strings.TrimLeft(row, " \t")
		if trimmed == "" {
			continue
		}
		// Skip other list items.
		if strings.HasPrefix(trimmed, "- ") || trimmed == "-" {
			continue
		}
		// Skip YAML comment lines — valid between glob: and its list items.
		if strings.HasPrefix(trimmed, "#") {
			continue
		}
		if c := strings.IndexByte(row, ':'); c >= 0 {
			return strings.TrimSpace(row[:c])
		}
		break
	}
	return ""
}

// insideCodeBlock reports whether bodyLine (1-based in body) falls inside
// a fenced or indented code block. Completion does not fire inside code.
func insideCodeBlock(root ast.Node, body []byte, bodyLine int) bool {
	var found bool
	_ = ast.Walk(root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch n.(type) {
		case *ast.FencedCodeBlock, *ast.CodeBlock:
			if codeNodeContainsLine(body, n, bodyLine) {
				found = true
				return ast.WalkStop, nil
			}
		}
		return ast.WalkContinue, nil
	})
	return found
}

// codeNodeContainsLine reports whether the AST node's content lines include
// bodyLine (1-based in body).
func codeNodeContainsLine(body []byte, n ast.Node, bodyLine int) bool {
	ls := n.Lines()
	if ls == nil || ls.Len() == 0 {
		return false
	}
	start := lineOfOffset(body, ls.At(0).Start)
	stop := ls.At(ls.Len() - 1).Stop
	// Goldmark segments include the trailing newline in Stop, so Stop points
	// to the first byte of the following line. Subtract one when that byte is
	// a newline so the end-line calculation lands on the actual last content
	// line rather than spilling into the line that follows the code block.
	if stop > 0 && stop <= len(body) && body[stop-1] == '\n' {
		stop--
	}
	end := lineOfOffset(body, stop)
	return bodyLine >= start && bodyLine <= end
}
