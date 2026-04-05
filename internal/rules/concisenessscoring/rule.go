package concisenessscoring

import (
	"bytes"
	"fmt"
	"strings"
	"sync"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/mdtext"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/yuin/goldmark/ast"
)

const (
	defaultMinScore = 0.20
	defaultMinWords = 20
)

var (
	scorerOnce      sync.Once
	globalScorer    *Scorer
	scorerErr       error
	errReportedOnce sync.Once
)

func loadScorer() (*Scorer, error) {
	scorerOnce.Do(func() {
		globalScorer, scorerErr = NewScorer()
	})
	return globalScorer, scorerErr
}

func init() {
	rule.Register(&Rule{
		MinScore: defaultMinScore,
		MinWords: defaultMinWords,
	})
}

// Rule checks paragraph conciseness using the embedded classifier.
type Rule struct {
	MinScore float64
	MinWords int
}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "MDS029" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "conciseness-scoring" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "meta" }

// EnabledByDefault implements rule.Defaultable.
func (r *Rule) EnabledByDefault() bool { return false }

// Check implements rule.Rule.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	scorer, err := loadScorer()
	if err != nil {
		return r.loadErrorDiag(f, err)
	}

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
		result := scorer.Score(text)
		if result.WordCount < r.MinWords || result.Conciseness >= r.MinScore {
			return ast.WalkContinue, nil
		}

		line := paragraphLine(para, f)
		message := fmt.Sprintf(
			"conciseness score too low (%.2f < %.2f); target >= %.2f",
			result.Conciseness, r.MinScore, r.MinScore,
		)
		examples := formatExamples(result.Cues)
		if examples != "" {
			message += fmt.Sprintf(
				"; reduce verbose cues (e.g., %s)",
				examples,
			)
		}

		diags = append(diags, lint.Diagnostic{
			File:     f.Path,
			Line:     line,
			Column:   1,
			RuleID:   r.ID(),
			RuleName: r.Name(),
			Severity: lint.Warning,
			Message:  message,
		})
		return ast.WalkContinue, nil
	})
	return diags
}

func (r *Rule) loadErrorDiag(
	f *lint.File, err error,
) []lint.Diagnostic {
	var diag []lint.Diagnostic
	errReportedOnce.Do(func() {
		diag = []lint.Diagnostic{{
			File:     f.Path,
			Line:     1,
			Column:   1,
			RuleID:   r.ID(),
			RuleName: r.Name(),
			Severity: lint.Error,
			Message: fmt.Sprintf(
				"classifier load failed: %v", err,
			),
		}}
	})
	return diag
}

func formatExamples(examples []string) string {
	if len(examples) == 0 {
		return ""
	}
	limit := 2
	if len(examples) < limit {
		limit = len(examples)
	}
	values := make([]string, 0, limit)
	for i := 0; i < limit; i++ {
		values = append(values, fmt.Sprintf("%q", examples[i]))
	}
	return strings.Join(values, ", ")
}

func paragraphLine(para *ast.Paragraph, f *lint.File) int {
	lines := para.Lines()
	if lines.Len() > 0 {
		return f.LineOfOffset(lines.At(0).Start)
	}
	return 1
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
	return bytes.HasPrefix(
		bytes.TrimSpace(f.Source[seg.Start:seg.Stop]),
		[]byte("|"),
	)
}

// ApplySettings implements rule.Configurable.
func (r *Rule) ApplySettings(settings map[string]any) error {
	for k, v := range settings {
		switch k {
		case "min-score":
			if err := r.setMinScore(v); err != nil {
				return err
			}
		case "min-words":
			if err := r.setMinWords(v); err != nil {
				return err
			}
		default:
			return fmt.Errorf("conciseness-scoring: unknown setting %q", k)
		}
	}
	return nil
}

func (r *Rule) setMinScore(v any) error {
	n, ok := toFloat(v)
	if !ok {
		return fmt.Errorf(
			"conciseness-scoring: min-score must be a number, got %T",
			v,
		)
	}
	if n <= 0 || n > 1 {
		return fmt.Errorf(
			"conciseness-scoring: min-score must be > 0 and <= 1, got %.2f",
			n,
		)
	}
	r.MinScore = n
	return nil
}

func (r *Rule) setMinWords(v any) error {
	n, ok := toInt(v)
	if !ok {
		return fmt.Errorf(
			"conciseness-scoring: min-words must be an integer, got %T",
			v,
		)
	}
	if n <= 0 {
		return fmt.Errorf(
			"conciseness-scoring: min-words must be > 0, got %d",
			n,
		)
	}
	r.MinWords = n
	return nil
}

// DefaultSettings implements rule.Configurable.
func (r *Rule) DefaultSettings() map[string]any {
	return map[string]any{
		"min-score": defaultMinScore,
		"min-words": defaultMinWords,
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

var _ rule.Configurable = (*Rule)(nil)
var _ rule.Defaultable = (*Rule)(nil)
