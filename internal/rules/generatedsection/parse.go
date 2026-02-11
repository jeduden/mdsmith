package generatedsection

import (
	"fmt"
	"strings"

	"github.com/jeduden/tidymark/internal/lint"
	"github.com/yuin/goldmark/ast"
	"gopkg.in/yaml.v3"
)

// markerPair holds the line numbers and parsed content of a start/end marker pair.
type markerPair struct {
	startLine   int // 1-based line of "<!-- tidymark:gen:start ..."
	endLine     int // 1-based line of "<!-- tidymark:gen:end -->"
	contentFrom int // 1-based line of the first content line (line after -->)
	contentTo   int // 1-based line of the last content line (line before end marker)
	firstLine   string
	yamlBody    string
}

// directive holds the parsed directive from a marker pair.
type directive struct {
	name    string
	params  map[string]string
	columns map[string]columnConfig
}

const startPrefix = "<!-- tidymark:gen:start"
const endMarker = "<!-- tidymark:gen:end -->"

// makeDiag creates a TM019 error diagnostic at the given line.
func makeDiag(filePath string, line int, message string) lint.Diagnostic {
	return lint.Diagnostic{
		File:     filePath,
		Line:     line,
		Column:   1,
		RuleID:   "TM019",
		RuleName: "generated-section",
		Severity: lint.Error,
		Message:  message,
	}
}

// markerScanState tracks state while scanning for marker pairs.
type markerScanState struct {
	pairs      []markerPair
	diags      []lint.Diagnostic
	current    *markerPair
	inYAMLBody bool
}

// findMarkerPairs scans the file for start/end marker pairs, skipping
// markers inside code blocks or HTML blocks.
func findMarkerPairs(f *lint.File) ([]markerPair, []lint.Diagnostic) {
	ignored := collectIgnoredLines(f)
	state := &markerScanState{}

	for i, line := range f.Lines {
		lineNum := i + 1
		if ignored[lineNum] {
			continue
		}
		trimmed := strings.TrimSpace(string(line))
		processMarkerLine(f, state, lineNum, string(line), trimmed)
	}

	if state.current != nil {
		state.diags = append(state.diags, makeDiag(f.Path, state.current.startLine,
			"generated section has no closing marker"))
	}

	return state.pairs, state.diags
}

// processMarkerLine processes a single line during marker pair scanning.
func processMarkerLine(f *lint.File, s *markerScanState, lineNum int, line, trimmed string) {
	if s.current != nil && s.inYAMLBody {
		if trimmed == "-->" {
			s.current.contentFrom = lineNum + 1
			s.inYAMLBody = false
			return
		}
		s.current.yamlBody += line + "\n"
		return
	}

	if s.current != nil {
		processLineInsidePair(f, s, lineNum, trimmed)
		return
	}

	processLineOutsidePair(f, s, lineNum, trimmed)
}

// processLineInsidePair handles a line that is inside an open marker pair
// (after the YAML body has been closed).
func processLineInsidePair(f *lint.File, s *markerScanState, lineNum int, trimmed string) {
	if strings.HasPrefix(trimmed, startPrefix) {
		s.diags = append(s.diags, makeDiag(f.Path, lineNum,
			"nested generated section markers are not allowed"))
		return
	}
	if trimmed == endMarker {
		s.current.endLine = lineNum
		s.current.contentTo = lineNum - 1
		s.pairs = append(s.pairs, *s.current)
		s.current = nil
	}
}

// processLineOutsidePair handles a line that is not inside any marker pair.
func processLineOutsidePair(f *lint.File, s *markerScanState, lineNum int, trimmed string) {
	if trimmed == endMarker {
		s.diags = append(s.diags, makeDiag(f.Path, lineNum,
			"unexpected generated section end marker"))
		return
	}

	if strings.HasPrefix(trimmed, startPrefix) {
		mp := markerPair{startLine: lineNum, firstLine: trimmed}
		rest := trimmed[len(startPrefix):]
		if strings.HasSuffix(rest, "-->") {
			rest = strings.TrimSuffix(rest, "-->")
			mp.firstLine = startPrefix + rest
			mp.contentFrom = lineNum + 1
		} else {
			s.inYAMLBody = true
		}
		s.current = &mp
	}
}

// parseDirective extracts the directive name and YAML parameters from a marker pair.
func parseDirective(f *lint.File, mp markerPair) (*directive, []lint.Diagnostic) {
	// Extract directive name from first line.
	rest := strings.TrimSpace(mp.firstLine[len(startPrefix):])
	if rest == "" {
		return nil, []lint.Diagnostic{makeDiag(f.Path, mp.startLine,
			"generated section marker missing directive name")}
	}
	name := strings.Fields(rest)[0]

	// Parse YAML body.
	rawMap, diags := parseYAMLBody(f.Path, mp)
	if len(diags) > 0 {
		return nil, diags
	}

	// Extract columns config before string validation.
	columnsRaw := extractColumnsRaw(rawMap)

	// Validate all remaining values are strings.
	params, diags := validateStringParams(f.Path, mp.startLine, rawMap)
	if len(diags) > 0 {
		return nil, diags
	}

	return &directive{
		name:    name,
		params:  params,
		columns: parseColumnConfig(columnsRaw),
	}, nil
}

// parseYAMLBody unmarshals the YAML body of a marker pair.
func parseYAMLBody(filePath string, mp markerPair) (map[string]any, []lint.Diagnostic) {
	var rawMap map[string]any
	if mp.yamlBody != "" {
		if err := yaml.Unmarshal([]byte(mp.yamlBody), &rawMap); err != nil {
			return nil, []lint.Diagnostic{makeDiag(filePath, mp.startLine,
				fmt.Sprintf("generated section has invalid YAML: %v", err))}
		}
	}
	if rawMap == nil {
		rawMap = map[string]any{}
	}
	return rawMap, nil
}

// extractColumnsRaw removes and returns the "columns" key from rawMap.
func extractColumnsRaw(rawMap map[string]any) map[string]any {
	v, ok := rawMap["columns"]
	if !ok {
		return nil
	}
	delete(rawMap, "columns")
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return nil
}

// validateStringParams checks that all values in rawMap are strings.
func validateStringParams(filePath string, line int, rawMap map[string]any) (map[string]string, []lint.Diagnostic) {
	var diags []lint.Diagnostic
	params := make(map[string]string)
	for k, v := range rawMap {
		s, ok := v.(string)
		if !ok {
			diags = append(diags, makeDiag(filePath, line,
				fmt.Sprintf("generated section has non-string value for key %q", k)))
		} else {
			params[k] = s
		}
	}
	if len(diags) > 0 {
		return nil, diags
	}
	return params, nil
}

// collectIgnoredLines returns a set of 1-based line numbers inside fenced
// code blocks or HTML blocks, where markers should be ignored.
func collectIgnoredLines(f *lint.File) map[int]bool {
	lines := map[int]bool{}

	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		switch cb := n.(type) {
		case *ast.FencedCodeBlock:
			addBlockLineRange(f, cb, lines)
		case *ast.CodeBlock:
			addBlockLineRange(f, cb, lines)
		case *ast.HTMLBlock:
			addHTMLBlockLines(f, cb, lines)
		}

		return ast.WalkContinue, nil
	})

	return lines
}

// addBlockLineRange marks all lines spanned by a code block node.
func addBlockLineRange(f *lint.File, n ast.Node, set map[int]bool) {
	if n.Lines().Len() == 0 {
		return
	}

	// For fenced code blocks, include the fence lines.
	firstSeg := n.Lines().At(0)
	lastSeg := n.Lines().At(n.Lines().Len() - 1)

	startLine := f.LineOfOffset(firstSeg.Start)
	endLine := f.LineOfOffset(lastSeg.Start)

	// For fenced code blocks, the opening fence is one line before content.
	if _, ok := n.(*ast.FencedCodeBlock); ok {
		if startLine > 1 {
			startLine--
		}
		// Closing fence is one line after content.
		endLine++
	}

	for ln := startLine; ln <= endLine && ln <= len(f.Lines); ln++ {
		set[ln] = true
	}
}

// addHTMLBlockLines marks all lines spanned by an HTML block.
// HTML blocks that are tidymark markers are not ignored, since
// the markers are HTML comments that goldmark parses as HTML blocks.
func addHTMLBlockLines(f *lint.File, n *ast.HTMLBlock, set map[int]bool) {
	if n.Lines().Len() == 0 {
		return
	}
	firstSeg := n.Lines().At(0)

	// Check if this HTML block is a tidymark marker; if so, do not ignore it.
	firstLineText := strings.TrimSpace(string(firstSeg.Value(f.Source)))
	if strings.HasPrefix(firstLineText, startPrefix) || firstLineText == endMarker {
		return
	}

	lastSeg := n.Lines().At(n.Lines().Len() - 1)

	startLine := f.LineOfOffset(firstSeg.Start)
	endLine := f.LineOfOffset(lastSeg.Start)

	// Include the closing line if present.
	if n.HasClosure() {
		closureLine := f.LineOfOffset(n.ClosureLine.Start)
		if closureLine > endLine {
			endLine = closureLine
		}
	}

	for ln := startLine; ln <= endLine && ln <= len(f.Lines); ln++ {
		set[ln] = true
	}
}
