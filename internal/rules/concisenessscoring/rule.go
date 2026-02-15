package concisenessscoring

import (
	"bytes"
	"fmt"
	"strings"

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
	defaultFillerWords = []string{
		"actually",
		"basically",
		"just",
		"really",
		"very",
	}
	defaultHedgePhrases = []string{
		"i think",
		"it seems",
		"it appears",
		"kind of",
		"sort of",
		"in my opinion",
	}
	defaultVerbosePhrases = []string{
		"in order to",
		"due to the fact that",
		"at this point in time",
		"for the purpose of",
		"it should be noted that",
	}
)

func init() {
	rule.Register(&Rule{
		MinScore: defaultMinScore,
		MinWords: defaultMinWords,
	})
}

// Rule checks paragraph conciseness using a heuristic score
// based on content ratio and verbose language cues.
type Rule struct {
	MinScore       float64
	MinWords       int
	FillerWords    []string
	HedgePhrases   []string
	VerbosePhrases []string
}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "MDS029" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "conciseness-scoring" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "meta" }

// Check implements rule.Rule.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	var diags []lint.Diagnostic

	heur := r.heuristics()
	minScore := r.MinScore
	minWords := r.MinWords

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
			result := scoreParagraph(text, heur)
			if result.WordCount < minWords || result.Score >= minScore {
				return ast.WalkContinue, nil
			}

			line := paragraphLine(para, f)
			message := fmt.Sprintf(
				"conciseness score too low (%.2f < %.2f); target >= %.2f",
				result.Score, minScore, minScore,
			)
			examples := formatExamples(result.Examples)
			if examples != "" {
				message += fmt.Sprintf(
					"; reduce filler or hedge cues (e.g., %s)",
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
		},
	)

	return diags
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

// ApplySettings implements rule.Configurable.
func (r *Rule) ApplySettings(settings map[string]any) error {
	for k, v := range settings {
		if err := r.applySetting(k, v); err != nil {
			return err
		}
	}
	return nil
}

func (r *Rule) applySetting(k string, v any) error {
	switch k {
	case "min-score":
		return r.setMinScore(v)
	case "min-words":
		return r.setMinWords(v)
	case "filler-words":
		return r.setWordList("filler-words", v, func(values []string) {
			r.FillerWords = values
		})
	case "hedge-phrases":
		return r.setWordList("hedge-phrases", v, func(values []string) {
			r.HedgePhrases = values
		})
	case "verbose-phrases":
		return r.setWordList("verbose-phrases", v, func(values []string) {
			r.VerbosePhrases = values
		})
	default:
		return fmt.Errorf("conciseness-scoring: unknown setting %q", k)
	}
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

func (r *Rule) setWordList(
	name string, v any, set func(values []string),
) error {
	values, ok := toStringSlice(v)
	if !ok {
		return fmt.Errorf(
			"conciseness-scoring: %s must be a list of strings, got %T",
			name, v,
		)
	}
	set(values)
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

func toStringSlice(v any) ([]string, bool) {
	switch values := v.(type) {
	case []string:
		return values, true
	case []any:
		out := make([]string, 0, len(values))
		for _, item := range values {
			s, ok := item.(string)
			if !ok {
				return nil, false
			}
			out = append(out, s)
		}
		return out, true
	default:
		return nil, false
	}
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

func (r *Rule) heuristics() heuristics {
	filler := r.FillerWords
	if filler == nil {
		filler = defaultFillerWords
	}

	hedge := r.HedgePhrases
	if hedge == nil {
		hedge = defaultHedgePhrases
	}

	verbose := r.VerbosePhrases
	if verbose == nil {
		verbose = defaultVerbosePhrases
	}

	return newHeuristics(filler, hedge, verbose)
}

var _ rule.Configurable = (*Rule)(nil)
