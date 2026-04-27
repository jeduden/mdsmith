package paragraphreadability

import (
	"fmt"
	"math"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/mdtext"
	"github.com/jeduden/mdsmith/internal/placeholders"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/jeduden/mdsmith/internal/rules/astutil"
	"github.com/jeduden/mdsmith/internal/rules/settings"
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
	MaxIndex     float64
	MinWords     int
	Index        IndexFunc
	Placeholders []string // placeholder tokens to treat as opaque
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
			if astutil.IsTable(para, f) {
				return ast.WalkContinue, nil
			}

			text := mdtext.ExtractPlainText(para, f.Source)
			if len(r.Placeholders) > 0 {
				text = placeholders.MaskBodyTokens(text, r.Placeholders)
			}
			words := mdtext.CountWords(text)
			if words < minWords {
				return ast.WalkContinue, nil
			}

			score := index(text)
			if score > maxIndex {
				line := astutil.ParagraphLine(para, f)
				diags = append(diags, lint.Diagnostic{
					File:     f.Path,
					Line:     line,
					Column:   1,
					RuleID:   r.ID(),
					RuleName: r.Name(),
					Severity: lint.Warning,
					Message:  readabilityMessage(score, maxIndex, words, text),
				})
			}

			return ast.WalkContinue, nil
		},
	)

	return diags
}

func readabilityMessage(score, maxIndex float64, words int, text string) string {
	rounded := math.Round(score*10) / 10
	sentences := mdtext.CountSentences(text)
	avgSentLen := 0
	if sentences > 0 {
		avgSentLen = words / sentences
	}
	return fmt.Sprintf(
		"paragraph too hard to read (readability index: %.1f, max %.1f)"+
			"; avg sentence length %d words — try splitting long sentences",
		rounded, maxIndex, avgSentLen,
	)
}

// ApplySettings implements rule.Configurable.
func (r *Rule) ApplySettings(s map[string]any) error {
	for k, v := range s {
		switch k {
		case "max-index":
			n, ok := settings.ToFloat(v)
			if !ok {
				return fmt.Errorf(
					"paragraph-readability: max-index must be a number, got %T",
					v,
				)
			}
			r.MaxIndex = n
		case "min-words":
			n, ok := settings.ToInt(v)
			if !ok {
				return fmt.Errorf(
					"paragraph-readability: min-words must be an integer, got %T",
					v,
				)
			}
			r.MinWords = n
		case "placeholders":
			toks, ok := settings.ToStringSlice(v)
			if !ok {
				return fmt.Errorf(
					"paragraph-readability: placeholders must be a list of strings, got %T",
					v,
				)
			}
			if err := placeholders.Validate(toks); err != nil {
				return fmt.Errorf("paragraph-readability: %w", err)
			}
			r.Placeholders = toks
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
		"max-index":    14.0,
		"min-words":    20,
		"placeholders": []string{},
	}
}

// ListMergeMode implements rule.ListMerger: placeholders concatenate
// across config layers.
func (r *Rule) ListMergeMode(key string) rule.ListMergeMode {
	if key == "placeholders" {
		return rule.ListAppend
	}
	return rule.ListReplace
}

var (
	_ rule.Configurable = (*Rule)(nil)
	_ rule.ListMerger   = (*Rule)(nil)
)
