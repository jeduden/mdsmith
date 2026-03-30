package paragraphreadability

import (
	"bytes"
	"fmt"
	"math"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/mdtext"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/yuin/goldmark/ast"
)

func init() {
	rule.Register(&Rule{
		MaxIndex: 14.0,
		MinWords: 20,
		Index:    ARI,
	})
}

// Rule checks that the paragraph readability index does not exceed
// a configured maximum. Uses the Automated Readability Index by
// default.
type Rule struct {
	MaxIndex float64
	MinWords int
	Index    IndexFunc
}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "MDS023" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "paragraph-readability" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "meta" }

// Check implements rule.Rule.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	var diags []lint.Diagnostic
	maxIndex := r.MaxIndex
	minWords := r.MinWords
	index := r.Index
	if index == nil {
		index = ARI
	}

	_ = ast.Walk(
		f.AST,
		func(n ast.Node, entering bool) (ast.WalkStatus, error) {
			if !entering {
				return ast.WalkContinue, nil
			}
			para, ok := n.(*ast.Paragraph)
			if !ok {
				return ast.WalkContinue, nil
			}
			if isTable(para, f) {
				return ast.WalkContinue, nil
			}

			text := mdtext.ExtractPlainText(para, f.Source)
			words := mdtext.CountWords(text)
			if words < minWords {
				return ast.WalkContinue, nil
			}

			score := index(text)
			if score > maxIndex {
				line := paragraphLine(para, f)
				rounded := math.Round(score*10) / 10
				diags = append(diags, lint.Diagnostic{
					File:     f.Path,
					Line:     line,
					Column:   1,
					RuleID:   r.ID(),
					RuleName: r.Name(),
					Severity: lint.Warning,
					Message: fmt.Sprintf(
						"paragraph too hard to read"+
							" (readability index: %.1f, max %.1f)",
						rounded, maxIndex,
					),
				})
			}

			return ast.WalkContinue, nil
		},
	)

	return diags
}

func paragraphLine(para *ast.Paragraph, f *lint.File) int {
	lines := para.Lines()
	if lines.Len() > 0 {
		return f.LineOfOffset(lines.At(0).Start)
	}
	return 1
}

// ApplySettings implements rule.Configurable.
func (r *Rule) ApplySettings(settings map[string]any) error {
	for k, v := range settings {
		switch k {
		case "max-index":
			n, ok := toFloat(v)
			if !ok {
				return fmt.Errorf(
					"paragraph-readability: max-index must be a number, got %T",
					v,
				)
			}
			r.MaxIndex = n
		case "min-words":
			n, ok := toInt(v)
			if !ok {
				return fmt.Errorf(
					"paragraph-readability: min-words must be an integer, got %T",
					v,
				)
			}
			r.MinWords = n
		default:
			return fmt.Errorf(
				"paragraph-readability: unknown setting %q", k,
			)
		}
	}
	return nil
}

// DefaultSettings implements rule.Configurable.
func (r *Rule) DefaultSettings() map[string]any {
	return map[string]any{
		"max-index": 14.0,
		"min-words": 20,
	}
}

func toFloat(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	}
	return 0, false
}

func toInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case float64:
		return int(n), true
	case int64:
		return int(n), true
	}
	return 0, false
}

// isTable returns true if the paragraph's first line starts with a pipe,
// indicating it is a markdown table (goldmark without the table extension
// parses tables as paragraphs).
func isTable(para *ast.Paragraph, f *lint.File) bool {
	lines := para.Lines()
	if lines.Len() == 0 {
		return false
	}
	seg := lines.At(0)
	return bytes.HasPrefix(bytes.TrimSpace(f.Source[seg.Start:seg.Stop]), []byte("|"))
}

var _ rule.Configurable = (*Rule)(nil)
