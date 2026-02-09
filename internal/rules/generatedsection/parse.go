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
	name   string
	params map[string]string
}

const startPrefix = "<!-- tidymark:gen:start"
const endMarker = "<!-- tidymark:gen:end -->"

// findMarkerPairs scans the file for start/end marker pairs, skipping
// markers inside code blocks or HTML blocks.
func findMarkerPairs(f *lint.File) ([]markerPair, []lint.Diagnostic) {
	ignored := collectIgnoredLines(f)

	var pairs []markerPair
	var diags []lint.Diagnostic
	var current *markerPair
	inYAMLBody := false

	for i, line := range f.Lines {
		lineNum := i + 1
		if ignored[lineNum] {
			continue
		}

		trimmed := strings.TrimSpace(string(line))

		if current != nil && inYAMLBody {
			// Looking for --> terminator
			if trimmed == "-->" {
				current.contentFrom = lineNum + 1
				inYAMLBody = false
				continue
			}
			current.yamlBody += string(line) + "\n"
			continue
		}

		if current != nil && !inYAMLBody {
			// Looking for end marker
			if strings.HasPrefix(trimmed, "<!-- tidymark:gen:start") {
				// Nested start marker
				diags = append(diags, lint.Diagnostic{
					File:     f.Path,
					Line:     lineNum,
					Column:   1,
					RuleID:   "TM019",
					RuleName: "generated-section",
					Severity: lint.Error,
					Message:  "nested generated section markers are not allowed",
				})
				continue
			}
			if trimmed == endMarker {
				current.endLine = lineNum
				current.contentTo = lineNum - 1
				pairs = append(pairs, *current)
				current = nil
				continue
			}
			continue
		}

		// Not inside a marker pair
		if trimmed == endMarker {
			diags = append(diags, lint.Diagnostic{
				File:     f.Path,
				Line:     lineNum,
				Column:   1,
				RuleID:   "TM019",
				RuleName: "generated-section",
				Severity: lint.Error,
				Message:  "unexpected generated section end marker",
			})
			continue
		}

		if strings.HasPrefix(trimmed, startPrefix) {
			mp := markerPair{
				startLine: lineNum,
				firstLine: trimmed,
			}

			// Check if --> is on the same line (single-line marker)
			rest := trimmed[len(startPrefix):]
			if strings.HasSuffix(rest, "-->") {
				rest = strings.TrimSuffix(rest, "-->")
				mp.firstLine = startPrefix + rest
				mp.contentFrom = lineNum + 1
				inYAMLBody = false
			} else {
				inYAMLBody = true
			}

			current = &mp
		}
	}

	if current != nil {
		diags = append(diags, lint.Diagnostic{
			File:     f.Path,
			Line:     current.startLine,
			Column:   1,
			RuleID:   "TM019",
			RuleName: "generated-section",
			Severity: lint.Error,
			Message:  "generated section has no closing marker",
		})
	}

	return pairs, diags
}

// parseDirective extracts the directive name and YAML parameters from a marker pair.
func parseDirective(f *lint.File, mp markerPair) (*directive, []lint.Diagnostic) {
	var diags []lint.Diagnostic

	// Extract directive name from first line.
	rest := mp.firstLine[len(startPrefix):]
	rest = strings.TrimSpace(rest)

	if rest == "" {
		diags = append(diags, lint.Diagnostic{
			File:     f.Path,
			Line:     mp.startLine,
			Column:   1,
			RuleID:   "TM019",
			RuleName: "generated-section",
			Severity: lint.Error,
			Message:  "generated section marker missing directive name",
		})
		return nil, diags
	}

	// Only the first word is the directive name.
	name := strings.Fields(rest)[0]

	// Parse YAML body.
	var rawMap map[string]any
	if mp.yamlBody != "" {
		if err := yaml.Unmarshal([]byte(mp.yamlBody), &rawMap); err != nil {
			diags = append(diags, lint.Diagnostic{
				File:     f.Path,
				Line:     mp.startLine,
				Column:   1,
				RuleID:   "TM019",
				RuleName: "generated-section",
				Severity: lint.Error,
				Message:  fmt.Sprintf("generated section has invalid YAML: %v", err),
			})
			return nil, diags
		}
	}

	if rawMap == nil {
		rawMap = map[string]any{}
	}

	// Validate all values are strings.
	params := make(map[string]string)
	for k, v := range rawMap {
		s, ok := v.(string)
		if !ok {
			diags = append(diags, lint.Diagnostic{
				File:     f.Path,
				Line:     mp.startLine,
				Column:   1,
				RuleID:   "TM019",
				RuleName: "generated-section",
				Severity: lint.Error,
				Message:  fmt.Sprintf("generated section has non-string value for key %q", k),
			})
		} else {
			params[k] = s
		}
	}

	if len(diags) > 0 {
		return nil, diags
	}

	return &directive{
		name:   name,
		params: params,
	}, nil
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
