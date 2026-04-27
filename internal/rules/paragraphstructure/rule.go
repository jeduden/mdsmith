package paragraphstructure

import (
	"fmt"
	"strings"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/mdtext"
	"github.com/jeduden/mdsmith/internal/placeholders"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/jeduden/mdsmith/internal/rules/astutil"
	"github.com/jeduden/mdsmith/internal/rules/settings"
	"github.com/yuin/goldmark/ast"
)

func init() {
	rule.Register(&Rule{MaxSentences: 6, MaxWords: 40})
}

// Rule checks that paragraphs do not exceed sentence and word limits.
type Rule struct {
	MaxSentences int
	MaxWords     int
	Placeholders []string // placeholder tokens to treat as opaque
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
		if astutil.IsTable(para, f) {
			return ast.WalkContinue, nil
		}

		text := mdtext.ExtractPlainText(para, f.Source)
		if len(r.Placeholders) > 0 {
			text = placeholders.MaskBodyTokens(text, r.Placeholders)
		}
		sentences := mdtext.SplitSentences(text)
		line := astutil.ParagraphLine(para, f)

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

// ApplySettings implements rule.Configurable.
func (r *Rule) ApplySettings(s map[string]any) error {
	for k, v := range s {
		switch k {
		case "max-sentences":
			n, ok := settings.ToInt(v)
			if !ok {
				return fmt.Errorf(
					"paragraph-structure: max-sentences must be an integer, got %T", v,
				)
			}
			r.MaxSentences = n
		case "max-words-per-sentence":
			n, ok := settings.ToInt(v)
			if !ok {
				return fmt.Errorf(
					"paragraph-structure: max-words-per-sentence must be an integer, got %T", v,
				)
			}
			r.MaxWords = n
		case "placeholders":
			toks, ok := settings.ToStringSlice(v)
			if !ok {
				return fmt.Errorf(
					"paragraph-structure: placeholders must be a list of strings, got %T", v,
				)
			}
			if err := placeholders.Validate(toks); err != nil {
				return fmt.Errorf("paragraph-structure: %w", err)
			}
			r.Placeholders = toks
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
		"placeholders":           []string{},
	}
}

// SettingMergeMode implements rule.ListMerger.
func (r *Rule) SettingMergeMode(key string) rule.MergeMode {
	if key == "placeholders" {
		return rule.MergeAppend
	}
	return rule.MergeReplace
}

var (
	_ rule.Configurable = (*Rule)(nil)
	_ rule.ListMerger   = (*Rule)(nil)
)
