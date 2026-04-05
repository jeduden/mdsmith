package paragraphstructure

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/mdtext"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/yuin/goldmark/ast"
)

func init() {
	rule.Register(&Rule{MaxSentences: 6, MaxWords: 40})
}

// Rule checks that paragraphs do not exceed sentence and word limits.
type Rule struct {
	MaxSentences int
	MaxWords     int
}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "MDS024" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "paragraph-structure" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "meta" }

// Check implements rule.Rule.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	var diags []lint.Diagnostic

	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
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
		sentences := mdtext.SplitSentences(text)
		line := paragraphLine(para, f)

		if len(sentences) > r.MaxSentences {
			diags = append(diags, lint.Diagnostic{
				File:     f.Path,
				Line:     line,
				Column:   1,
				RuleID:   r.ID(),
				RuleName: r.Name(),
				Severity: lint.Warning,
				Message: fmt.Sprintf(
					"paragraph has too many sentences (%d > %d)",
					len(sentences), r.MaxSentences,
				),
			})
		}

		for _, sent := range sentences {
			wc := mdtext.CountWords(sent)
			if wc > r.MaxWords {
				diags = append(diags, lint.Diagnostic{
					File:     f.Path,
					Line:     line,
					Column:   1,
					RuleID:   r.ID(),
					RuleName: r.Name(),
					Severity: lint.Warning,
					Message: fmt.Sprintf(
						"sentence too long (%d > %d words): %s",
						wc, r.MaxWords, sentencePreview(sent, 10),
					),
				})
			}
		}

		return ast.WalkContinue, nil
	})

	return diags
}

// sentencePreview returns a quoted preview of the sentence, truncated
// to approximately limit words with "..." appended if truncated.
func sentencePreview(sent string, limit int) string {
	words := strings.Fields(strings.TrimSpace(sent))
	if len(words) <= limit {
		return fmt.Sprintf("%q", strings.Join(words, " "))
	}
	return fmt.Sprintf("%q", strings.Join(words[:limit], " ")+" ...")
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
		case "max-sentences":
			n, ok := toInt(v)
			if !ok {
				return fmt.Errorf(
					"paragraph-structure: max-sentences must be an integer, got %T", v,
				)
			}
			r.MaxSentences = n
		case "max-words-per-sentence":
			n, ok := toInt(v)
			if !ok {
				return fmt.Errorf(
					"paragraph-structure: max-words-per-sentence must be an integer, got %T", v,
				)
			}
			r.MaxWords = n
		default:
			return fmt.Errorf("paragraph-structure: unknown setting %q", k)
		}
	}
	return nil
}

// DefaultSettings implements rule.Configurable.
func (r *Rule) DefaultSettings() map[string]any {
	return map[string]any{
		"max-sentences":          6,
		"max-words-per-sentence": 40,
	}
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
