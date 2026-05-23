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
func (r *Rule) Category() string { return "prose" }

// Check implements rule.Rule.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	var diags []lint.Diagnostic
	maxIndex := r.MaxIndex
	minWords := r.MinWords
	index := r.Index
	if index == nil {
		index = ARI
	}

	// Iterate the per-File memoized non-table paragraph collection so
	// the AST walk is shared with other paragraph-walking rules
	// (MDS024 paragraph-structure, MDS057 required-text-patterns,
	// MDS058 required-mentions) when they are enabled. MDS023 is the
	// only default-on rule in this set; the rest are opt-in.
	//
	// minWords is gated on [mdtext.CountWordsInNode], an AST-walking
	// counter that does NOT materialise the paragraph text. On
	// prose-heavy synthetic corpora most paragraphs fall below the
	// floor (the engine bench's "synthetic sentence …" body is
	// 13 words, default minWords is 20); skipping the text alloc on
	// those paragraphs is the bulk of plan 196's win. When
	// placeholders are configured the count goes through CountWords
	// on the masked text, because masking is a string transform and
	// has no AST equivalent.
	for _, p := range astutil.CollectSectionParagraphs(f) {
		var text string
		var words int
		usingPlaceholders := len(r.Placeholders) > 0
		if usingPlaceholders {
			// Masking is a string transform with no AST equivalent;
			// materialise the text, mask, and count on the result.
			// The masked string feeds the index below — even if it
			// happens to be empty, we MUST NOT fall back to
			// unmasked text or the index would see content the
			// caller asked to opaque-out.
			text = placeholders.MaskBodyTokens(p.ExtractText(f.Source), r.Placeholders)
			words = mdtext.CountWords(text)
		} else {
			words = mdtext.CountWordsInNode(p.Node, f.Source)
		}
		if words < minWords {
			continue
		}
		if !usingPlaceholders {
			// Non-placeholder path gated on word count above; only
			// now do we pay for the text. (Placeholders path
			// already has the masked text in `text`.)
			text = p.ExtractText(f.Source)
		}

		score := index(text)
		if score > maxIndex {
			diags = append(diags, lint.Diagnostic{
				File:     f.Path,
				Line:     p.Line,
				Column:   1,
				RuleID:   r.ID(),
				RuleName: r.Name(),
				Severity: lint.Warning,
				Message:  readabilityMessage(score, maxIndex, words, text),
			})
		}
	}

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
