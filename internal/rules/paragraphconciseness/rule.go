package paragraphconciseness

import (
	"bytes"
	"fmt"
	"math"
	"strings"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/mdtext"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/yuin/goldmark/ast"
)

const (
	defaultMaxVerbosity   = 30.0
	defaultMinWords       = 20
	defaultMinContentRate = 0.45
)

func init() {
	rule.Register(defaultRule())
}

// Rule checks paragraph conciseness using filler, hedge, verbose-phrase,
// and content-ratio heuristics.
type Rule struct {
	MaxVerbosity   float64
	MinWords       int
	MinContentRate float64
	FillerWords    []string
	HedgePhrases   []string
	VerbosePhrases []string
}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "MDS026" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "paragraph-conciseness" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "meta" }

// Check implements rule.Rule.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	state := r.runtimeState()
	diags := make([]lint.Diagnostic, 0)
	_ = ast.Walk(f.AST, func(
		n ast.Node, entering bool,
	) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		para, ok := n.(*ast.Paragraph)
		if !ok {
			return ast.WalkContinue, nil
		}
		diag := r.checkParagraph(f, para, state)
		if diag != nil {
			diags = append(diags, *diag)
		}
		return ast.WalkContinue, nil
	})
	return diags
}

// ApplySettings implements rule.Configurable.
func (r *Rule) ApplySettings(settings map[string]any) error {
	for k, v := range settings {
		var err error
		switch k {
		case "max-verbosity":
			err = r.applyMaxVerbosity(v)
		case "min-words":
			err = r.applyMinWords(v)
		case "min-content-ratio":
			err = r.applyMinContentRate(v)
		case "filler-words":
			err = r.applyFillerWords(v)
		case "hedge-phrases":
			err = r.applyHedgePhrases(v)
		case "verbose-phrases":
			err = r.applyVerbosePhrases(v)
		default:
			err = fmt.Errorf(
				"paragraph-conciseness: unknown setting %q", k,
			)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// DefaultSettings implements rule.Configurable.
func (r *Rule) DefaultSettings() map[string]any {
	return map[string]any{
		"max-verbosity":     defaultMaxVerbosity,
		"min-words":         defaultMinWords,
		"min-content-ratio": defaultMinContentRate,
		"filler-words":      append([]string(nil), defaultFillerWords...),
		"hedge-phrases":     append([]string(nil), defaultHedgePhrases...),
		"verbose-phrases":   append([]string(nil), defaultVerbosePhrases...),
	}
}

func defaultRule() *Rule {
	return &Rule{
		MaxVerbosity:   defaultMaxVerbosity,
		MinWords:       defaultMinWords,
		MinContentRate: defaultMinContentRate,
		FillerWords:    append([]string(nil), defaultFillerWords...),
		HedgePhrases:   append([]string(nil), defaultHedgePhrases...),
		VerbosePhrases: append([]string(nil), defaultVerbosePhrases...),
	}
}

type runtimeState struct {
	maxVerbosity   float64
	minWords       int
	minContentRate float64
	model          scoringModel
}

func (r *Rule) runtimeState() runtimeState {
	minContentRate := r.minContentRate()
	return runtimeState{
		maxVerbosity:   r.maxVerbosity(),
		minWords:       r.minWords(),
		minContentRate: minContentRate,
		model: newScoringModel(
			minContentRate,
			r.fillerWords(),
			r.hedgePhrases(),
			r.verbosePhrases(),
		),
	}
}

func (r *Rule) checkParagraph(
	f *lint.File, para *ast.Paragraph, state runtimeState,
) *lint.Diagnostic {
	if isTable(para, f) {
		return nil
	}
	text := mdtext.ExtractPlainText(para, f.Source)
	if mdtext.CountWords(text) < state.minWords {
		return nil
	}
	score := state.model.score(text)
	if score.Verbosity <= state.maxVerbosity {
		return nil
	}
	return &lint.Diagnostic{
		File:     f.Path,
		Line:     paragraphLine(para, f),
		Column:   1,
		RuleID:   r.ID(),
		RuleName: r.Name(),
		Severity: lint.Warning,
		Message:  buildMessage(score, state.maxVerbosity, state.minContentRate),
	}
}

func buildMessage(
	score scoreBreakdown,
	maxVerbosity float64,
	minContentRate float64,
) string {
	verbosity := roundTenth(score.Verbosity)
	conciseness := roundTenth(score.Conciseness)
	targetConcise := roundTenth(100 - maxVerbosity)
	message := fmt.Sprintf(
		"paragraph verbosity too high "+
			"(%.1f > %.1f; conciseness %.1f, target >= %.1f)",
		verbosity, maxVerbosity, conciseness, targetConcise,
	)
	if score.ContentRatio < minContentRate {
		message += fmt.Sprintf(
			"; content ratio %.2f < %.2f",
			score.ContentRatio, minContentRate,
		)
	}
	if len(score.Samples) > 0 {
		message += fmt.Sprintf(
			"; trim phrases like %s",
			quoteSamples(score.Samples, 2),
		)
	}
	return message
}

func (r *Rule) maxVerbosity() float64 {
	if r.MaxVerbosity == 0 {
		return defaultMaxVerbosity
	}
	return r.MaxVerbosity
}

func (r *Rule) minWords() int {
	if r.MinWords == 0 {
		return defaultMinWords
	}
	return r.MinWords
}

func (r *Rule) minContentRate() float64 {
	if r.MinContentRate == 0 {
		return defaultMinContentRate
	}
	return r.MinContentRate
}

func (r *Rule) fillerWords() []string {
	if r.FillerWords == nil {
		return defaultFillerWords
	}
	return r.FillerWords
}

func (r *Rule) hedgePhrases() []string {
	if r.HedgePhrases == nil {
		return defaultHedgePhrases
	}
	return r.HedgePhrases
}

func (r *Rule) verbosePhrases() []string {
	if r.VerbosePhrases == nil {
		return defaultVerbosePhrases
	}
	return r.VerbosePhrases
}

func (r *Rule) applyMaxVerbosity(v any) error {
	n, ok := toFloat(v)
	if !ok {
		return fmt.Errorf(
			"paragraph-conciseness: max-verbosity must be a number, got %T",
			v,
		)
	}
	if n < 0 || n > 100 {
		return fmt.Errorf(
			"paragraph-conciseness: max-verbosity must be between 0 and 100, got %.2f",
			n,
		)
	}
	r.MaxVerbosity = n
	return nil
}

func (r *Rule) applyMinWords(v any) error {
	n, ok := toInt(v)
	if !ok {
		return fmt.Errorf(
			"paragraph-conciseness: min-words must be an integer, got %T",
			v,
		)
	}
	if n < 1 {
		return fmt.Errorf(
			"paragraph-conciseness: min-words must be >= 1, got %d",
			n,
		)
	}
	r.MinWords = n
	return nil
}

func (r *Rule) applyMinContentRate(v any) error {
	n, ok := toFloat(v)
	if !ok {
		return fmt.Errorf(
			"paragraph-conciseness: min-content-ratio must be a number, got %T",
			v,
		)
	}
	if n <= 0 || n > 1 {
		return fmt.Errorf(
			"paragraph-conciseness: min-content-ratio must be > 0 and <= 1, got %.2f",
			n,
		)
	}
	r.MinContentRate = n
	return nil
}

func (r *Rule) applyFillerWords(v any) error {
	words, ok := toStringSlice(v)
	if !ok {
		return fmt.Errorf(
			"paragraph-conciseness: filler-words must be a list of strings, got %T",
			v,
		)
	}
	normalized, err := normalizeFillerWords(words)
	if err != nil {
		return err
	}
	r.FillerWords = normalized
	return nil
}

func (r *Rule) applyHedgePhrases(v any) error {
	phrases, ok := toStringSlice(v)
	if !ok {
		return fmt.Errorf(
			"paragraph-conciseness: hedge-phrases must be a list of strings, got %T",
			v,
		)
	}
	r.HedgePhrases = normalizePhraseList(phrases)
	return nil
}

func (r *Rule) applyVerbosePhrases(v any) error {
	phrases, ok := toStringSlice(v)
	if !ok {
		return fmt.Errorf(
			"paragraph-conciseness: verbose-phrases must be a list of strings, got %T",
			v,
		)
	}
	r.VerbosePhrases = normalizePhraseList(phrases)
	return nil
}

func normalizeFillerWords(words []string) ([]string, error) {
	seen := make(map[string]struct{}, len(words))
	out := make([]string, 0, len(words))
	for _, w := range words {
		w = strings.TrimSpace(strings.ToLower(w))
		if w == "" {
			continue
		}
		tokens := tokenizeWords(w)
		if len(tokens) != 1 {
			return nil, fmt.Errorf(
				"paragraph-conciseness: filler-words must contain single words, got %q",
				w,
			)
		}
		if _, ok := seen[tokens[0]]; ok {
			continue
		}
		seen[tokens[0]] = struct{}{}
		out = append(out, tokens[0])
	}
	return out, nil
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
		bytes.TrimSpace(f.Source[seg.Start:seg.Stop]), []byte("|"),
	)
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
	switch list := v.(type) {
	case []string:
		return append([]string(nil), list...), true
	case []any:
		out := make([]string, 0, len(list))
		for _, item := range list {
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

func quoteSamples(samples []string, max int) string {
	quoted := make([]string, 0, len(samples))
	for _, sample := range firstN(samples, max) {
		quoted = append(quoted, fmt.Sprintf("%q", sample))
	}
	return strings.Join(quoted, ", ")
}

func roundTenth(v float64) float64 {
	return math.Round(v*10) / 10
}

var _ rule.Configurable = (*Rule)(nil)
