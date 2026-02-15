package conciseness

import (
	"bytes"
	"fmt"
	"math"
	"strings"
	"unicode"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/mdtext"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/yuin/goldmark/ast"
)

const (
	defaultMinScore            = 55.0
	defaultMinWords            = 20
	defaultMinContentRatio     = 0.45
	defaultFillerWeight        = 1.0
	defaultHedgeWeight         = 0.8
	defaultVerbosePhraseWeight = 4.0
	defaultContentWeight       = 1.2
)

var (
	defaultFillerWords = []string{
		"actually",
		"basically",
		"clearly",
		"obviously",
		"quite",
		"really",
		"simply",
		"very",
	}

	defaultHedgeWords = []string{
		"apparently",
		"arguably",
		"generally",
		"likely",
		"maybe",
		"might",
		"perhaps",
		"possibly",
		"probably",
		"seems",
		"somewhat",
	}

	defaultVerbosePhrases = []string{
		"at this point in time",
		"due to the fact that",
		"for all intents and purposes",
		"in most cases",
		"in order to",
		"in the event that",
		"it is important to note",
		"on the same page",
		theFactThatPhrase,
	}
)

const theFactThatPhrase = "the fact that"

func init() {
	r := &Rule{}
	_ = r.ApplySettings(r.DefaultSettings())
	rule.Register(r)
}

// Rule scores paragraph conciseness using a weighted heuristic and flags
// paragraphs that fall below MinScore.
type Rule struct {
	MinScore            float64
	MinWords            int
	MinContentRatio     float64
	FillerWeight        float64
	HedgeWeight         float64
	VerbosePhraseWeight float64
	ContentWeight       float64

	FillerWords    []string
	HedgeWords     []string
	VerbosePhrases []string

	fillerSet          map[string]struct{}
	hedgeSet           map[string]struct{}
	verbosePhraseWords [][]string
}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "MDS026" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "conciseness" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "meta" }

// Check implements rule.Rule.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	var diags []lint.Diagnostic

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
			if words < r.MinWords {
				return ast.WalkContinue, nil
			}

			tokens := normalizeWords(text)
			if len(tokens) == 0 {
				return ast.WalkContinue, nil
			}

			m := r.score(tokens, words)
			if m.score < r.MinScore {
				line := paragraphLine(para, f)
				diags = append(diags, lint.Diagnostic{
					File:     f.Path,
					Line:     line,
					Column:   1,
					RuleID:   r.ID(),
					RuleName: r.Name(),
					Severity: lint.Warning,
					Message: fmt.Sprintf(
						"paragraph conciseness score too low "+
							"(%.1f < %.1f; filler %.1f%%, hedge %.1f%%, "+
							"content %.1f%%, phrases %d)",
						m.score,
						r.MinScore,
						m.fillerRatio*100,
						m.hedgeRatio*100,
						m.contentRatio*100,
						m.verbosePhraseCount,
					),
				})
			}

			return ast.WalkContinue, nil
		},
	)

	return diags
}

type metrics struct {
	score              float64
	fillerRatio        float64
	hedgeRatio         float64
	contentRatio       float64
	verbosePhraseCount int
}

func (r *Rule) score(tokens []string, totalWords int) metrics {
	if totalWords <= 0 {
		return metrics{score: 100.0}
	}

	total := float64(totalWords)
	fillerCount := countWords(tokens, r.fillerSet)
	hedgeCount := countWords(tokens, r.hedgeSet)
	contentCount := countContentWords(tokens, r.fillerSet, r.hedgeSet)
	verbosePhraseCount := countPhraseMatches(tokens, r.verbosePhraseWords)

	fillerRatio := float64(fillerCount) / total
	hedgeRatio := float64(hedgeCount) / total
	contentRatio := float64(contentCount) / total
	phrasePer100Words := float64(verbosePhraseCount) * 100.0 / total

	contentDeficit := 0.0
	if contentRatio < r.MinContentRatio {
		contentDeficit = r.MinContentRatio - contentRatio
	}

	penalty :=
		(fillerRatio * 100.0 * r.FillerWeight) +
			(hedgeRatio * 100.0 * r.HedgeWeight) +
			(phrasePer100Words * r.VerbosePhraseWeight) +
			(contentDeficit * 100.0 * r.ContentWeight)

	score := 100.0 - penalty
	score = math.Max(0.0, math.Min(100.0, score))
	score = math.Round(score*10) / 10

	return metrics{
		score:              score,
		fillerRatio:        fillerRatio,
		hedgeRatio:         hedgeRatio,
		contentRatio:       contentRatio,
		verbosePhraseCount: verbosePhraseCount,
	}
}

// ApplySettings implements rule.Configurable.
func (r *Rule) ApplySettings(settings map[string]any) error {
	handlers := map[string]func(any) error{
		"min-score":             r.applyMinScore,
		"min-words":             r.applyMinWords,
		"min-content-ratio":     r.applyMinContentRatio,
		"filler-weight":         r.applyFillerWeight,
		"hedge-weight":          r.applyHedgeWeight,
		"verbose-phrase-weight": r.applyVerbosePhraseWeight,
		"content-weight":        r.applyContentWeight,
		"filler-words":          r.applyFillerWords,
		"hedge-words":           r.applyHedgeWords,
		"verbose-phrases":       r.applyVerbosePhrases,
	}

	for k, v := range settings {
		handler, ok := handlers[k]
		if !ok {
			return fmt.Errorf("conciseness: unknown setting %q", k)
		}
		if err := handler(v); err != nil {
			return err
		}
	}

	r.fillerSet = toWordSet(r.FillerWords)
	r.hedgeSet = toWordSet(r.HedgeWords)
	r.verbosePhraseWords = compilePhraseWords(r.VerbosePhrases)

	return nil
}

func (r *Rule) applyMinScore(v any) error {
	n, ok := toFloat(v)
	if !ok {
		return fmt.Errorf("conciseness: min-score must be a number, got %T", v)
	}
	if n < 0 || n > 100 {
		return fmt.Errorf(
			"conciseness: min-score must be between 0 and 100, got %.2f", n,
		)
	}
	r.MinScore = n
	return nil
}

func (r *Rule) applyMinWords(v any) error {
	n, ok := toInt(v)
	if !ok {
		return fmt.Errorf("conciseness: min-words must be an integer, got %T", v)
	}
	if n < 1 {
		return fmt.Errorf("conciseness: min-words must be >= 1, got %d", n)
	}
	r.MinWords = n
	return nil
}

func (r *Rule) applyMinContentRatio(v any) error {
	n, ok := toFloat(v)
	if !ok {
		return fmt.Errorf(
			"conciseness: min-content-ratio must be a number, got %T", v,
		)
	}
	if n < 0 || n > 1 {
		return fmt.Errorf(
			"conciseness: min-content-ratio must be between 0 and 1, got %.2f", n,
		)
	}
	r.MinContentRatio = n
	return nil
}

func (r *Rule) applyFillerWeight(v any) error {
	n, ok := toFloat(v)
	if !ok {
		return fmt.Errorf(
			"conciseness: filler-weight must be a number, got %T", v,
		)
	}
	if n < 0 {
		return fmt.Errorf("conciseness: filler-weight must be >= 0, got %.2f", n)
	}
	r.FillerWeight = n
	return nil
}

func (r *Rule) applyHedgeWeight(v any) error {
	n, ok := toFloat(v)
	if !ok {
		return fmt.Errorf("conciseness: hedge-weight must be a number, got %T", v)
	}
	if n < 0 {
		return fmt.Errorf("conciseness: hedge-weight must be >= 0, got %.2f", n)
	}
	r.HedgeWeight = n
	return nil
}

func (r *Rule) applyVerbosePhraseWeight(v any) error {
	n, ok := toFloat(v)
	if !ok {
		return fmt.Errorf(
			"conciseness: verbose-phrase-weight must be a number, got %T", v,
		)
	}
	if n < 0 {
		return fmt.Errorf(
			"conciseness: verbose-phrase-weight must be >= 0, got %.2f", n,
		)
	}
	r.VerbosePhraseWeight = n
	return nil
}

func (r *Rule) applyContentWeight(v any) error {
	n, ok := toFloat(v)
	if !ok {
		return fmt.Errorf(
			"conciseness: content-weight must be a number, got %T", v,
		)
	}
	if n < 0 {
		return fmt.Errorf("conciseness: content-weight must be >= 0, got %.2f", n)
	}
	r.ContentWeight = n
	return nil
}

func (r *Rule) applyFillerWords(v any) error {
	words, err := parseWordList(v, "filler-words")
	if err != nil {
		return err
	}
	r.FillerWords = words
	return nil
}

func (r *Rule) applyHedgeWords(v any) error {
	words, err := parseWordList(v, "hedge-words")
	if err != nil {
		return err
	}
	r.HedgeWords = words
	return nil
}

func (r *Rule) applyVerbosePhrases(v any) error {
	phrases, err := parsePhraseList(v)
	if err != nil {
		return err
	}
	r.VerbosePhrases = phrases
	return nil
}

// DefaultSettings implements rule.Configurable.
func (r *Rule) DefaultSettings() map[string]any {
	return map[string]any{
		"min-score":             defaultMinScore,
		"min-words":             defaultMinWords,
		"min-content-ratio":     defaultMinContentRatio,
		"filler-weight":         defaultFillerWeight,
		"hedge-weight":          defaultHedgeWeight,
		"verbose-phrase-weight": defaultVerbosePhraseWeight,
		"content-weight":        defaultContentWeight,
		"filler-words":          cloneStrings(defaultFillerWords),
		"hedge-words":           cloneStrings(defaultHedgeWords),
		"verbose-phrases":       cloneStrings(defaultVerbosePhrases),
	}
}

func normalizeWords(text string) []string {
	fields := strings.Fields(strings.ToLower(text))
	words := make([]string, 0, len(fields))
	for _, field := range fields {
		w := strings.TrimFunc(field, func(r rune) bool {
			return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '\'' && r != '-'
		})
		if w != "" {
			words = append(words, w)
		}
	}
	return words
}

func parseWordList(v any, key string) ([]string, error) {
	items, ok := toStringSlice(v)
	if !ok {
		return nil, fmt.Errorf("conciseness: %s must be a string list, got %T", key, v)
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		words := normalizeWords(item)
		if len(words) != 1 {
			return nil, fmt.Errorf("conciseness: %s entries must be single words, got %q", key, item)
		}
		out = append(out, words[0])
	}
	return out, nil
}

func parsePhraseList(v any) ([]string, error) {
	items, ok := toStringSlice(v)
	if !ok {
		return nil, fmt.Errorf("conciseness: verbose-phrases must be a string list, got %T", v)
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		words := normalizeWords(item)
		if len(words) == 0 {
			return nil, fmt.Errorf("conciseness: verbose-phrases entries must not be empty")
		}
		out = append(out, strings.Join(words, " "))
	}
	return out, nil
}

func compilePhraseWords(phrases []string) [][]string {
	out := make([][]string, 0, len(phrases))
	for _, phrase := range phrases {
		words := normalizeWords(phrase)
		if len(words) > 0 {
			out = append(out, words)
		}
	}
	return out
}

func countWords(words []string, set map[string]struct{}) int {
	if len(set) == 0 {
		return 0
	}
	count := 0
	for _, word := range words {
		if _, ok := set[word]; ok {
			count++
		}
	}
	return count
}

func countPhraseMatches(words []string, phrases [][]string) int {
	count := 0
	for _, phrase := range phrases {
		if len(phrase) == 0 || len(phrase) > len(words) {
			continue
		}
		for i := 0; i <= len(words)-len(phrase); i++ {
			if phraseAt(words, i, phrase) {
				count++
				i += len(phrase) - 1
			}
		}
	}
	return count
}

func phraseAt(words []string, start int, phrase []string) bool {
	for i := range phrase {
		if words[start+i] != phrase[i] {
			return false
		}
	}
	return true
}

func countContentWords(words []string, filler, hedge map[string]struct{}) int {
	count := 0
	for _, word := range words {
		if isContentWord(word, filler, hedge) {
			count++
		}
	}
	return count
}

func isContentWord(word string, filler, hedge map[string]struct{}) bool {
	if _, ok := filler[word]; ok {
		return false
	}
	if _, ok := hedge[word]; ok {
		return false
	}
	if _, ok := stopWords[word]; ok {
		return false
	}
	if hasDigit(word) {
		return true
	}
	return len([]rune(word)) >= 3
}

func hasDigit(s string) bool {
	for _, r := range s {
		if unicode.IsDigit(r) {
			return true
		}
	}
	return false
}

func toWordSet(words []string) map[string]struct{} {
	set := make(map[string]struct{}, len(words))
	for _, word := range words {
		norm := normalizeWords(word)
		if len(norm) != 1 {
			continue
		}
		set[norm[0]] = struct{}{}
	}
	return set
}

func toStringSlice(v any) ([]string, bool) {
	switch xs := v.(type) {
	case []string:
		return cloneStrings(xs), true
	case []any:
		out := make([]string, 0, len(xs))
		for _, x := range xs {
			s, ok := x.(string)
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

func cloneStrings(in []string) []string {
	out := make([]string, len(in))
	copy(out, in)
	return out
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
	return bytes.HasPrefix(bytes.TrimSpace(f.Source[seg.Start:seg.Stop]), []byte("|"))
}

var stopWords = map[string]struct{}{
	"a": {}, "an": {}, "and": {}, "are": {}, "as": {}, "at": {},
	"be": {}, "been": {}, "being": {}, "but": {}, "by": {},
	"can": {}, "could": {}, "did": {}, "do": {}, "does": {},
	"for": {}, "from": {}, "had": {}, "has": {}, "have": {},
	"he": {}, "her": {}, "hers": {}, "him": {}, "his": {}, "i": {},
	"if": {}, "in": {}, "into": {}, "is": {}, "it": {}, "its": {},
	"just": {}, "may": {}, "me": {}, "more": {}, "most": {},
	"must": {}, "my": {}, "need": {}, "not": {}, "of": {},
	"on": {}, "or": {}, "our": {}, "ours": {}, "she": {},
	"should": {}, "so": {}, "some": {}, "than": {}, "that": {},
	"the": {}, "their": {}, "theirs": {}, "them": {}, "then": {},
	"there": {}, "these": {}, "they": {}, "this": {}, "those": {},
	"to": {}, "too": {}, "was": {}, "we": {}, "were": {}, "what": {},
	"when": {}, "which": {}, "who": {}, "will": {}, "with": {},
	"would": {}, "you": {}, "your": {}, "yours": {},
}

var _ rule.Configurable = (*Rule)(nil)
