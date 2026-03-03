package gensection

import (
	"fmt"
	"strings"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/yuin/goldmark/ast"
	"gopkg.in/yaml.v3"
)

// MarkerPair holds the line numbers and parsed content of a start/end marker pair.
type MarkerPair struct {
	StartLine   int // 1-based line of start marker
	EndLine     int // 1-based line of end marker
	ContentFrom int // 1-based line of the first content line
	ContentTo   int // 1-based line of the last content line
	FirstLine   string
	YAMLBody    string
}

// ParsedDirective holds the parsed directive from a marker pair.
type ParsedDirective struct {
	Name    string
	Params  map[string]string
	Columns map[string]ColumnConfig
}

// markerScanState tracks state while scanning for marker pairs.
type markerScanState struct {
	pairs      []MarkerPair
	diags      []lint.Diagnostic
	current    *MarkerPair
	inYAMLBody bool
}

// MakeDiag creates an error diagnostic at the given line using the
// provided rule ID and name.
func MakeDiag(ruleID, ruleName, filePath string, line int, message string) lint.Diagnostic {
	return lint.Diagnostic{
		File:     filePath,
		Line:     line,
		Column:   1,
		RuleID:   ruleID,
		RuleName: ruleName,
		Severity: lint.Error,
		Message:  message,
	}
}

// FindMarkerPairs scans the file for start/end marker pairs, skipping
// markers inside code blocks or HTML blocks. The startPrefix and
// endMarker are derived from the directive name.
func FindMarkerPairs(
	f *lint.File,
	startPrefix, endMarker, terminator, ruleID, ruleName string,
) ([]MarkerPair, []lint.Diagnostic) {
	ignored := CollectIgnoredLines(f, startPrefix, endMarker)
	state := &markerScanState{}

	for i, line := range f.Lines {
		lineNum := i + 1
		if ignored[lineNum] {
			continue
		}
		trimmed := strings.TrimSpace(string(line))
		processMarkerLine(
			f, state, lineNum, string(line), trimmed,
			startPrefix, endMarker, terminator, ruleID, ruleName,
		)
	}

	if state.current != nil {
		state.diags = append(state.diags,
			MakeDiag(ruleID, ruleName, f.Path, state.current.StartLine,
				"generated section has no closing marker"))
	}

	return state.pairs, state.diags
}

// processMarkerLine processes a single line during marker pair scanning.
func processMarkerLine(
	f *lint.File, s *markerScanState, lineNum int, line, trimmed string,
	startPrefix, endMarker, terminator, ruleID, ruleName string,
) {
	if s.current != nil && s.inYAMLBody {
		if trimmed == terminator {
			s.current.ContentFrom = lineNum + 1
			s.inYAMLBody = false
			return
		}
		s.current.YAMLBody += line + "\n"
		return
	}

	if s.current != nil {
		processLineInsidePair(f, s, lineNum, trimmed, startPrefix, endMarker, terminator, ruleID, ruleName)
		return
	}

	processLineOutsidePair(f, s, lineNum, trimmed, startPrefix, endMarker, terminator, ruleID, ruleName)
}

// processLineInsidePair handles a line that is inside an open marker pair
// (after the YAML body has been closed).
func processLineInsidePair(
	f *lint.File, s *markerScanState, lineNum int, trimmed string,
	startPrefix, endMarker, terminator, ruleID, ruleName string,
) {
	if strings.HasPrefix(trimmed, startPrefix) {
		s.diags = append(s.diags,
			MakeDiag(ruleID, ruleName, f.Path, lineNum,
				"nested generated section markers are not allowed"))
		return
	}
	if trimmed == endMarker {
		s.current.EndLine = lineNum
		s.current.ContentTo = lineNum - 1
		s.pairs = append(s.pairs, *s.current)
		s.current = nil
	}
}

// processLineOutsidePair handles a line that is not inside any marker pair.
func processLineOutsidePair(
	f *lint.File, s *markerScanState, lineNum int, trimmed string,
	startPrefix, endMarker, terminator, ruleID, ruleName string,
) {
	if trimmed == endMarker {
		s.diags = append(s.diags,
			MakeDiag(ruleID, ruleName, f.Path, lineNum,
				"unexpected generated section end marker"))
		return
	}

	if strings.HasPrefix(trimmed, startPrefix) {
		mp := MarkerPair{StartLine: lineNum, FirstLine: trimmed}
		rest := trimmed[len(startPrefix):]
		if strings.HasSuffix(rest, terminator) {
			rest = strings.TrimSuffix(rest, terminator)
			mp.FirstLine = startPrefix + rest
			mp.ContentFrom = lineNum + 1
		} else {
			s.inYAMLBody = true
		}
		s.current = &mp
	}
}

// ParseDirective extracts the YAML parameters from a marker pair.
func ParseDirective(
	filePath string, mp MarkerPair, ruleID, ruleName string,
) (*ParsedDirective, []lint.Diagnostic) {
	rawMap, diags := ParseYAMLBody(filePath, mp, ruleID, ruleName)
	if len(diags) > 0 {
		return nil, diags
	}

	columnsRaw := ExtractColumnsRaw(rawMap)

	params, diags := ValidateStringParams(filePath, mp.StartLine, rawMap, ruleID, ruleName)
	if len(diags) > 0 {
		return nil, diags
	}

	return &ParsedDirective{
		Params:  params,
		Columns: ParseColumnConfig(columnsRaw),
	}, nil
}

// ParseYAMLBody unmarshals the YAML body of a marker pair.
func ParseYAMLBody(
	filePath string, mp MarkerPair, ruleID, ruleName string,
) (map[string]any, []lint.Diagnostic) {
	var rawMap map[string]any
	if mp.YAMLBody != "" {
		if err := yaml.Unmarshal([]byte(mp.YAMLBody), &rawMap); err != nil {
			return nil, []lint.Diagnostic{
				MakeDiag(ruleID, ruleName, filePath, mp.StartLine,
					fmt.Sprintf("generated section has invalid YAML: %v", err)),
			}
		}
	}
	if rawMap == nil {
		rawMap = map[string]any{}
	}
	return rawMap, nil
}

// ExtractColumnsRaw removes and returns the "columns" key from rawMap.
func ExtractColumnsRaw(rawMap map[string]any) map[string]any {
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

// ValidateStringParams checks that all values in rawMap are strings.
func ValidateStringParams(
	filePath string, line int, rawMap map[string]any, ruleID, ruleName string,
) (map[string]string, []lint.Diagnostic) {
	var diags []lint.Diagnostic
	params := make(map[string]string)
	for k, v := range rawMap {
		s, ok := v.(string)
		if !ok {
			diags = append(diags,
				MakeDiag(ruleID, ruleName, filePath, line,
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

// ParseColumnConfig parses the raw YAML columns map into ColumnConfig entries.
func ParseColumnConfig(raw map[string]any) map[string]ColumnConfig {
	if raw == nil {
		return nil
	}

	result := make(map[string]ColumnConfig, len(raw))
	for name, v := range raw {
		colMap, ok := v.(map[string]any)
		if !ok {
			continue
		}

		cc := ColumnConfig{
			Wrap: "truncate",
		}

		if mw, ok := colMap["max-width"]; ok {
			switch val := mw.(type) {
			case int:
				cc.MaxWidth = val
			case float64:
				cc.MaxWidth = int(val)
			}
		}

		if w, ok := colMap["wrap"]; ok {
			if ws, ok := w.(string); ok {
				cc.Wrap = ws
			}
		}

		result[name] = cc
	}

	return result
}

// CollectIgnoredLines returns a set of 1-based line numbers inside fenced
// code blocks or HTML blocks, where markers should be ignored.
func CollectIgnoredLines(f *lint.File, startPrefix, endMarker string) map[int]bool {
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
			addHTMLBlockLines(f, cb, lines, startPrefix, endMarker)
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

	firstSeg := n.Lines().At(0)
	lastSeg := n.Lines().At(n.Lines().Len() - 1)

	startLine := f.LineOfOffset(firstSeg.Start)
	endLine := f.LineOfOffset(lastSeg.Start)

	if _, ok := n.(*ast.FencedCodeBlock); ok {
		if startLine > 1 {
			startLine--
		}
		endLine++
	}

	for ln := startLine; ln <= endLine && ln <= len(f.Lines); ln++ {
		set[ln] = true
	}
}

// IsDirectiveBlock reports whether text is entirely a processing
// instruction used as an mdsmith directive marker (e.g.
// <?catalog ...?> or <?/include?>). Only matches when the
// trimmed text starts with "<?" and ends with "?>".
func IsDirectiveBlock(text string) bool {
	t := strings.TrimSpace(text)
	return len(t) >= 4 && strings.HasPrefix(t, "<?") && strings.HasSuffix(t, "?>")
}

// IsSingleLineDirective reports whether line is a single-line
// processing instruction with the given name, allowing optional
// whitespace before the closing "?>", e.g. "<?name?>" or "<?name ?>".
func IsSingleLineDirective(line, name string) bool {
	s := strings.TrimSpace(line)

	prefix := "<?" + name
	if !strings.HasPrefix(s, prefix) || !strings.HasSuffix(s, "?>") {
		return false
	}

	// Allow only whitespace between the name and the closing "?>".
	middle := s[len(prefix) : len(s)-2]
	return strings.TrimSpace(middle) == ""
}

// addHTMLBlockLines marks all lines spanned by an HTML block.
// HTML blocks that are mdsmith markers are not ignored, since
// the markers are HTML processing instructions that goldmark parses as HTML blocks.
func addHTMLBlockLines(f *lint.File, n *ast.HTMLBlock, set map[int]bool, startPrefix, endMarker string) {
	if n.Lines().Len() == 0 {
		return
	}
	firstSeg := n.Lines().At(0)

	firstLineText := strings.TrimSpace(string(firstSeg.Value(f.Source)))
	if strings.HasPrefix(firstLineText, startPrefix) || firstLineText == endMarker {
		return
	}

	lastSeg := n.Lines().At(n.Lines().Len() - 1)

	startLine := f.LineOfOffset(firstSeg.Start)
	endLine := f.LineOfOffset(lastSeg.Start)

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
