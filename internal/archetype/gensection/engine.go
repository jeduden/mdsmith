package gensection

import (
	"strings"

	"github.com/jeduden/mdsmith/internal/lint"
)

// Engine orchestrates Check/Fix using a registered Directive.
type Engine struct {
	directive Directive
}

// NewEngine creates a new Engine for the given Directive.
func NewEngine(d Directive) *Engine {
	return &Engine{directive: d}
}

// Check scans the file for marker pairs and returns diagnostics.
func (e *Engine) Check(f *lint.File) []lint.Diagnostic {
	pairs, diags := FindMarkerPairs(
		f, e.directive.Name(),
		e.directive.RuleID(), e.directive.RuleName(),
	)
	for _, mp := range pairs {
		pairDiags := e.checkPair(f, mp)
		diags = append(diags, pairDiags...)
	}
	return diags
}

// Fix regenerates content for all marker pairs.
func (e *Engine) Fix(f *lint.File) []byte {
	pairs, _ := FindMarkerPairs(
		f, e.directive.Name(),
		e.directive.RuleID(), e.directive.RuleName(),
	)

	// Work backwards to preserve line numbers.
	for i := len(pairs) - 1; i >= 0; i-- {
		mp := pairs[i]
		expected, ok := e.generateContent(f, mp)
		if !ok {
			continue
		}

		f.Source = ReplaceContent(f, mp, expected)
		f.Lines = SplitLines(f.Source)
	}

	return f.Source
}

// checkPair checks a single marker pair and returns diagnostics.
func (e *Engine) checkPair(f *lint.File, mp MarkerPair) []lint.Diagnostic {
	dir, diags := ParseDirective(
		f.Path, mp,
		e.directive.RuleID(), e.directive.RuleName(),
	)
	if dir == nil || len(diags) > 0 {
		return diags
	}

	valDiags := e.directive.Validate(f.Path, mp.StartLine, dir.Params, dir.Columns)
	if len(valDiags) > 0 {
		return valDiags
	}

	expected, genDiags := e.directive.Generate(f, f.Path, mp.StartLine, dir.Params, dir.Columns)
	if len(genDiags) > 0 {
		return genDiags
	}

	actual := ExtractContent(f, mp)
	if actual != expected {
		return []lint.Diagnostic{
			MakeDiag(e.directive.RuleID(), e.directive.RuleName(),
				f.Path, mp.StartLine,
				"generated section is out of date"),
		}
	}

	return nil
}

// generateContent generates the expected content for a marker pair.
// Returns the content and true if generation succeeded, or empty and
// false if there were validation errors or generation errors.
func (e *Engine) generateContent(f *lint.File, mp MarkerPair) (string, bool) {
	dir, diags := ParseDirective(
		f.Path, mp,
		e.directive.RuleID(), e.directive.RuleName(),
	)
	if dir == nil || len(diags) > 0 {
		return "", false
	}

	moreDiags := e.directive.Validate(f.Path, mp.StartLine, dir.Params, dir.Columns)
	if len(moreDiags) > 0 {
		return "", false
	}

	content, genDiags := e.directive.Generate(f, f.Path, mp.StartLine, dir.Params, dir.Columns)
	if len(genDiags) > 0 {
		return "", false
	}
	return content, true
}

// ExtractContent returns the content between markers as a string.
func ExtractContent(f *lint.File, mp MarkerPair) string {
	if mp.ContentFrom > mp.ContentTo {
		return ""
	}
	var lines []string
	for i := mp.ContentFrom - 1; i <= mp.ContentTo-1 && i < len(f.Lines); i++ {
		lines = append(lines, string(f.Lines[i]))
	}
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n") + "\n"
}

// ReplaceContent replaces the content between markers with new content.
func ReplaceContent(f *lint.File, mp MarkerPair, content string) []byte {
	var result []byte

	// Lines before content.
	for i := 0; i < mp.ContentFrom-1 && i < len(f.Lines); i++ {
		result = append(result, f.Lines[i]...)
		result = append(result, '\n')
	}

	// New content.
	result = append(result, []byte(content)...)

	// Lines from end marker onward.
	for i := mp.EndLine - 1; i < len(f.Lines); i++ {
		result = append(result, f.Lines[i]...)
		if i < len(f.Lines)-1 {
			result = append(result, '\n')
		}
	}

	return result
}

// SplitLines splits source into lines (like bytes.Split but returns [][]byte).
func SplitLines(source []byte) [][]byte {
	var lines [][]byte
	start := 0
	for i, b := range source {
		if b == '\n' {
			lines = append(lines, source[start:i])
			start = i + 1
		}
	}
	lines = append(lines, source[start:])
	return lines
}

// EnsureTrailingNewline appends \n if s does not already end with \n.
func EnsureTrailingNewline(s string) string {
	if strings.HasSuffix(s, "\n") {
		return s
	}
	return s + "\n"
}
