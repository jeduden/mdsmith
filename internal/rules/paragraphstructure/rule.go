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
func (r *Rule) Category() string { return "prose" }

// EnabledByDefault implements rule.Defaultable. MDS024 is opt-in
// because exact sentence counting and per-sentence word counting
// require the trained Punkt segmenter
// (github.com/neurosnap/sentences), which the neutral CPU profile
// recorded in plan 187 attributes ~20% of mdsmith's wall time on
// prose-heavy input. Punkt's cost is the trained model's regex
// execution (english.MultiPunctWordAnnotation.tokenAnnotation
// runs reAbbr and the token-type matchers with backtracking on
// every period-ending token), and no pure-Go Punkt-equivalent
// faster segmenter exists — plan 187 records the negative with a
// reusable equivalence harness. Users who want the diagnostic
// enable it explicitly; the default check path stops paying the
// ~20%.
//
// NOTE: This is a behaviour change. Before this rule implemented
// rule.Defaultable, MDS024 ran on every default check. Existing
// .mdsmith.yml configs that did not pin paragraph-structure will
// no longer emit prose-structure diagnostics until they opt in
// via `rules: { paragraph-structure: true }`.
func (r *Rule) EnabledByDefault() bool { return false }

// Check implements rule.Rule.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	var diags []lint.Diagnostic
	// Iterate the per-File memoized non-table paragraph collection so
	// the AST walk and per-paragraph ExtractPlainText are shared with
	// the other prose rules instead of re-run here (the two hot
	// default rules on prose-heavy input).
	for _, p := range astutil.CollectSectionParagraphs(f) {
		diags = append(diags, r.checkParagraph(p.Text, p.Line, f.Path)...)
	}
	return diags
}

// checkParagraph evaluates one paragraph against the sentence-count
// and per-sentence word limits. text is the raw extracted plain
// text; line is its 1-based source line; both come from the shared
// collector. Placeholder masking stays per-rule so the shared text
// is not coupled to one rule's config.
func (r *Rule) checkParagraph(text string, line int, filePath string) []lint.Diagnostic {
	if len(r.Placeholders) > 0 {
		text = placeholders.MaskBodyTokens(text, r.Placeholders)
	}
	// Fast path: skip the Punkt tokenizer when this paragraph
	// provably cannot violate either limit. Punkt places a boundary
	// only at '.'/'!'/'?' and yields >=1 sentence, so
	// (terminal-punct + 1) is an upper bound on its sentence count;
	// and every sentence's word count is <= the whole paragraph's.
	// mdtext.SplitSentences dominated allocations (~2 GB over a
	// 600-file corpus, plan 175 profiling); most real paragraphs
	// clear this guard with zero allocation.
	if sentUB, words := cheapBounds(text); sentUB <= r.MaxSentences && words <= r.MaxWords {
		return nil
	}

	sentences := mdtext.SplitSentences(text)
	var diags []lint.Diagnostic

	if len(sentences) > r.MaxSentences {
		diags = append(diags, lint.Diagnostic{
			File:     filePath,
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
				File:     filePath,
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
	return diags
}

// cheapBounds returns, in one allocation-free pass, an upper bound
// on the Punkt sentence count (terminal-punct + 1) and the exact
// word count (whitespace-delimited, matching strings.Fields). Both
// are conservative for the rule's checks: Punkt never splits without
// '.'/'!'/'?' and always yields >=1 sentence, and no single sentence
// has more words than the whole paragraph.
func cheapBounds(s string) (sentUB, words int) {
	punct := 0
	inWord := false
	for _, r := range s {
		if r == '.' || r == '!' || r == '?' {
			punct++
		}
		if mdtext.IsSpace(r) {
			inWord = false
		} else if !inWord {
			inWord = true
			words++
		}
	}
	return punct + 1, words
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
	_ rule.Defaultable  = (*Rule)(nil)
)
