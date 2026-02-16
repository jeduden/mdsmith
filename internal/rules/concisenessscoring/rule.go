package concisenessscoring

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

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
		MinScore:            defaultMinScore,
		MinWords:            defaultMinWords,
		Mode:                defaultMode,
		Threshold:           defaultClassifierThreshold,
		ClassifierTimeoutMS: defaultClassifierTimeoutMS,
	})
}

// Rule checks paragraph conciseness using a classifier backend with a
// deterministic heuristic fallback.
type Rule struct {
	MinScore            float64
	MinWords            int
	Mode                string
	Threshold           float64
	ClassifierTimeoutMS int
	ClassifierModelPath string
	ClassifierChecksum  string
	FillerWords         []string
	HedgePhrases        []string
	VerbosePhrases      []string

	classifier       classifier
	classifierErr    error
	classifierLoaded bool
	classifierMu     sync.Mutex
}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "MDS029" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "conciseness-scoring" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "meta" }

// EnabledByDefault implements rule.Defaultable.
func (r *Rule) EnabledByDefault() bool { return false }

type runtimeState struct {
	heur                heuristics
	minScore            float64
	minWords            int
	useClassifier       bool
	classifier          classifier
	classifierThreshold float64
	inferTimeout        time.Duration
}

// Check implements rule.Rule.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	state := r.runtimeState()
	diags := make([]lint.Diagnostic, 0, 4)

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
			diag := r.checkParagraph(f, para, &state)
			if diag != nil {
				diags = append(diags, *diag)
			}

			return ast.WalkContinue, nil
		},
	)

	return diags
}

func (r *Rule) runtimeState() runtimeState {
	state := runtimeState{
		heur:         r.heuristics(),
		minScore:     r.MinScore,
		minWords:     r.MinWords,
		inferTimeout: classifierTimeout(r.ClassifierTimeoutMS),
	}
	if state.minScore <= 0 || state.minScore > 1 {
		state.minScore = defaultMinScore
	}
	if state.minWords <= 0 {
		state.minWords = defaultMinWords
	}

	mode := normalizeMode(r.Mode)
	if mode == "" {
		mode = defaultMode
	}
	if !isValidMode(mode) {
		mode = "heuristic"
	}
	if mode == "heuristic" {
		return state
	}

	loaded, err := r.resolveClassifier()
	if err != nil {
		return state
	}

	state.useClassifier = true
	state.classifier = loaded
	state.classifierThreshold = r.classifierThreshold(loaded)
	return state
}

func (r *Rule) checkParagraph(
	f *lint.File, para *ast.Paragraph, state *runtimeState,
) *lint.Diagnostic {
	if isTable(para, f) {
		return nil
	}

	text := mdtext.ExtractPlainText(para, f.Source)
	signals := analyzeParagraph(text, state.heur)
	if signals.WordCount < state.minWords {
		return nil
	}

	score, target := scoreForParagraph(signals, state)
	if score >= target {
		return nil
	}

	line := paragraphLine(para, f)
	message := fmt.Sprintf(
		"conciseness score too low (%.2f < %.2f); target >= %.2f",
		score, target, target,
	)
	examples := formatExamples(signals.Examples)
	if examples != "" {
		message += fmt.Sprintf(
			"; reduce filler or hedge cues (e.g., %s)",
			examples,
		)
	}

	return &lint.Diagnostic{
		File:     f.Path,
		Line:     line,
		Column:   1,
		RuleID:   r.ID(),
		RuleName: r.Name(),
		Severity: lint.Warning,
		Message:  message,
	}
}

func scoreForParagraph(
	signals paragraphSignals, state *runtimeState,
) (float64, float64) {
	score := heuristicScore(signals)
	target := state.minScore
	if !state.useClassifier || state.classifier == nil {
		return score, target
	}

	ctx, cancel := context.WithTimeout(
		context.Background(), state.inferTimeout,
	)
	risk, err := predictWithTimeout(ctx, state.classifier, signals)
	cancel()
	if err != nil {
		// Degrade remaining checks to heuristic mode.
		state.useClassifier = false
		return score, target
	}

	return 1 - risk, 1 - state.classifierThreshold
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
	if err := r.validateClassifierSettings(); err != nil {
		return err
	}
	r.resetClassifierCache()
	return nil
}

func (r *Rule) applySetting(k string, v any) error {
	switch k {
	case "min-score":
		return r.setMinScore(v)
	case "min-words":
		return r.setMinWords(v)
	case "mode":
		return r.setMode(v)
	case "threshold":
		return r.setThreshold(v)
	case "classifier-timeout-ms":
		return r.setClassifierTimeout(v)
	case "classifier-model-path":
		return r.setClassifierModelPath(v)
	case "classifier-checksum":
		return r.setClassifierChecksum(v)
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

func (r *Rule) setMode(v any) error {
	s, ok := v.(string)
	if !ok {
		return fmt.Errorf(
			"conciseness-scoring: mode must be a string, got %T",
			v,
		)
	}
	mode := normalizeMode(s)
	if !isValidMode(mode) {
		return fmt.Errorf(
			"conciseness-scoring: invalid mode %q (valid: auto, classifier, heuristic)",
			s,
		)
	}
	r.Mode = mode
	return nil
}

func (r *Rule) setThreshold(v any) error {
	n, ok := toFloat(v)
	if !ok {
		return fmt.Errorf(
			"conciseness-scoring: threshold must be a number, got %T",
			v,
		)
	}
	if n <= 0 || n > 1 {
		return fmt.Errorf(
			"conciseness-scoring: threshold must be > 0 and <= 1, got %.2f",
			n,
		)
	}
	r.Threshold = n
	return nil
}

func (r *Rule) setClassifierTimeout(v any) error {
	n, ok := toInt(v)
	if !ok {
		return fmt.Errorf(
			"conciseness-scoring: classifier-timeout-ms must be an integer, got %T",
			v,
		)
	}
	if n <= 0 {
		return fmt.Errorf(
			"conciseness-scoring: classifier-timeout-ms must be > 0, got %d",
			n,
		)
	}
	r.ClassifierTimeoutMS = n
	return nil
}

func (r *Rule) setClassifierModelPath(v any) error {
	s, ok := v.(string)
	if !ok {
		return fmt.Errorf(
			"conciseness-scoring: classifier-model-path must be a string, got %T",
			v,
		)
	}
	r.ClassifierModelPath = strings.TrimSpace(s)
	return nil
}

func (r *Rule) setClassifierChecksum(v any) error {
	s, ok := v.(string)
	if !ok {
		return fmt.Errorf(
			"conciseness-scoring: classifier-checksum must be a string, got %T",
			v,
		)
	}
	checksum := normalizeChecksum(s)
	if checksum != "" && !isHexChecksum(checksum) {
		return fmt.Errorf(
			"conciseness-scoring: classifier-checksum must be a 64-char hex value",
		)
	}
	r.ClassifierChecksum = checksum
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
		"min-score":             defaultMinScore,
		"min-words":             defaultMinWords,
		"mode":                  defaultMode,
		"threshold":             defaultClassifierThreshold,
		"classifier-timeout-ms": defaultClassifierTimeoutMS,
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

func (r *Rule) validateClassifierSettings() error {
	hasPath := strings.TrimSpace(r.ClassifierModelPath) != ""
	hasChecksum := strings.TrimSpace(r.ClassifierChecksum) != ""

	if hasPath && !hasChecksum {
		return fmt.Errorf(
			"conciseness-scoring: classifier-checksum is required when classifier-model-path is set",
		)
	}
	if hasChecksum && !hasPath {
		return fmt.Errorf(
			"conciseness-scoring: classifier-model-path is required when classifier-checksum is set",
		)
	}
	return nil
}

func (r *Rule) resolveClassifier() (classifier, error) {
	r.classifierMu.Lock()
	defer r.classifierMu.Unlock()

	if r.classifierLoaded {
		return r.classifier, r.classifierErr
	}

	clf, err := loadClassifier(
		strings.TrimSpace(r.ClassifierModelPath),
		strings.TrimSpace(r.ClassifierChecksum),
	)
	r.classifier = clf
	r.classifierErr = err
	r.classifierLoaded = true
	return r.classifier, r.classifierErr
}

func (r *Rule) resetClassifierCache() {
	r.classifierMu.Lock()
	defer r.classifierMu.Unlock()
	r.classifier = nil
	r.classifierErr = nil
	r.classifierLoaded = false
}

func (r *Rule) classifierThreshold(clf classifier) float64 {
	threshold := r.Threshold
	if threshold > 0 && threshold <= 1 {
		return threshold
	}
	if clf != nil {
		fallback := clf.DefaultThreshold()
		if fallback > 0 && fallback <= 1 {
			return fallback
		}
	}
	return defaultClassifierThreshold
}

var _ rule.Configurable = (*Rule)(nil)
var _ rule.Defaultable = (*Rule)(nil)
