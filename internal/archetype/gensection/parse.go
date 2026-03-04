package gensection

import (
	"fmt"
	"strings"

	"github.com/jeduden/mdsmith/internal/lint"
	"gopkg.in/yaml.v3"
)

// MarkerPair holds the line numbers and parsed content of a start/end marker pair.
type MarkerPair struct {
	StartLine   int // 1-based line of start marker
	EndLine     int // 1-based line of end marker
	ContentFrom int // 1-based line of the first content line
	ContentTo   int // 1-based line of the last content line
	YAMLBody    string
}

// ParsedDirective holds the parsed directive from a marker pair.
type ParsedDirective struct {
	Name    string
	Params  map[string]string
	Columns map[string]ColumnConfig
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

// FindMarkerPairs walks top-level AST children for ProcessingInstruction
// nodes matching the given directive name, pairing start/end markers.
func FindMarkerPairs(
	f *lint.File, directiveName, ruleID, ruleName string,
) ([]MarkerPair, []lint.Diagnostic) {
	var pairs []MarkerPair
	var diags []lint.Diagnostic
	var current *MarkerPair

	endName := "/" + directiveName

	for n := f.AST.FirstChild(); n != nil; n = n.NextSibling() {
		pi, ok := n.(*lint.ProcessingInstruction)
		if !ok {
			continue
		}

		switch pi.Name {
		case directiveName:
			current, diags = handleStartMarker(f, pi, current, diags, ruleID, ruleName)
		case endName:
			current, pairs, diags = handleEndMarker(f, pi, current, pairs, diags, ruleID, ruleName)
		}
	}

	if current != nil {
		diags = append(diags,
			MakeDiag(ruleID, ruleName, f.Path, current.StartLine,
				"generated section has no closing marker"))
	}

	return pairs, diags
}

func handleStartMarker(
	f *lint.File, pi *lint.ProcessingInstruction,
	current *MarkerPair, diags []lint.Diagnostic,
	ruleID, ruleName string,
) (*MarkerPair, []lint.Diagnostic) {
	piLine := f.LineOfOffset(pi.Lines().At(0).Start)
	if current != nil {
		return current, append(diags,
			MakeDiag(ruleID, ruleName, f.Path, piLine,
				"nested generated section markers are not allowed"))
	}
	if !pi.HasClosure() {
		return nil, append(diags,
			MakeDiag(ruleID, ruleName, f.Path, piLine,
				"generated section has no closing marker"))
	}

	mp := MarkerPair{
		StartLine:   piLine,
		YAMLBody:    extractYAMLBody(pi, f.Source),
		ContentFrom: piClosureEndLine(pi, f) + 1,
	}
	return &mp, diags
}

func handleEndMarker(
	f *lint.File, pi *lint.ProcessingInstruction,
	current *MarkerPair, pairs []MarkerPair, diags []lint.Diagnostic,
	ruleID, ruleName string,
) (*MarkerPair, []MarkerPair, []lint.Diagnostic) {
	piLine := f.LineOfOffset(pi.Lines().At(0).Start)
	if current == nil {
		return nil, pairs, append(diags,
			MakeDiag(ruleID, ruleName, f.Path, piLine,
				"unexpected generated section end marker"))
	}
	current.EndLine = piLine
	current.ContentTo = piLine - 1
	return nil, append(pairs, *current), diags
}

// extractYAMLBody returns the YAML content from a PI's Lines(),
// skipping the first line (the <?name line).
func extractYAMLBody(pi *lint.ProcessingInstruction, source []byte) string {
	lines := pi.Lines()
	if lines.Len() <= 1 {
		return ""
	}
	var b strings.Builder
	for i := 1; i < lines.Len(); i++ {
		seg := lines.At(i)
		b.Write(seg.Value(source))
	}
	return b.String()
}

// piClosureEndLine returns the 1-based line number of the PI's closure
// (or last body line for single-line PIs).
func piClosureEndLine(pi *lint.ProcessingInstruction, f *lint.File) int {
	if pi.HasClosure() && pi.ClosureLine.Start != pi.Lines().At(0).Start {
		// Multi-line PI: closure is on a separate line.
		return f.LineOfOffset(pi.ClosureLine.Start)
	}
	// Single-line PI: closure is the same line as opening.
	lastSeg := pi.Lines().At(pi.Lines().Len() - 1)
	return f.LineOfOffset(lastSeg.Start)
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
