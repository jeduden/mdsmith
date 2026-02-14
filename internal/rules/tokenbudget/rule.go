package tokenbudget

import (
	"fmt"
	"math"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/gobwas/glob"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/mdtext"
	"github.com/jeduden/mdsmith/internal/rule"
)

const (
	defaultMax       = 8000
	defaultMode      = "heuristic"
	defaultRatio     = 0.75
	defaultTokenizer = "builtin"
	defaultEncoding  = "cl100k_base"
	validEncodings   = "cl100k_base, p50k_base, r50k_base, gpt2"
)

var (
	// cl100kPattern approximates OpenAI-style pre-tokenization over
	// Unicode text and whitespace.
	cl100kPattern = regexp.MustCompile(`'s|'t|'re|'ve|'m|'ll|'d| ?\p{L}+| ?\p{N}+| ?[^\s\p{L}\p{N}]+|\s+`)
	// p50kPattern approximates GPT-2/p50k token pre-splitting with
	// ASCII-oriented word classes.
	p50kPattern = regexp.MustCompile(`'s|'t|'re|'ve|'m|'ll|'d| ?[A-Za-z]+| ?[0-9]+| ?[^\sA-Za-z0-9]+|\s+`)
)

type budgetOverride struct {
	Glob    string
	Max     int
	matcher glob.Glob
}

func init() {
	rule.Register(&Rule{
		Max:       defaultMax,
		Mode:      defaultMode,
		Ratio:     defaultRatio,
		Tokenizer: defaultTokenizer,
		Encoding:  defaultEncoding,
	})
}

// Rule checks that a file does not exceed a configurable token budget.
// It supports a simple heuristic mode and a tokenizer mode.
type Rule struct {
	Max       int
	Mode      string
	Ratio     float64
	Tokenizer string
	Encoding  string
	Budgets   []budgetOverride
}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "MDS028" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "token-budget" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "meta" }

// Check implements rule.Rule.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	budget := r.activeBudget(f.Path)
	count, modeLabel := r.tokenCountAndMode(string(f.Source))
	if count <= budget {
		return nil
	}

	return []lint.Diagnostic{{
		File:     f.Path,
		Line:     1,
		Column:   1,
		RuleID:   r.ID(),
		RuleName: r.Name(),
		Severity: lint.Warning,
		Message: fmt.Sprintf(
			"token budget exceeded (%d > %d, mode=%s)",
			count,
			budget,
			modeLabel,
		),
	}}
}

func (r *Rule) activeBudget(path string) int {
	budget := r.Max
	if budget <= 0 {
		budget = defaultMax
	}
	for _, override := range r.Budgets {
		if override.matcher == nil {
			continue
		}
		for _, candidate := range pathCandidates(path) {
			if override.matcher.Match(candidate) {
				budget = override.Max
				break
			}
		}
	}
	return budget
}

func pathCandidates(path string) []string {
	clean := filepath.Clean(path)
	base := filepath.Base(path)

	uniq := map[string]struct{}{}
	for _, p := range []string{
		path,
		clean,
		base,
		filepath.ToSlash(path),
		filepath.ToSlash(clean),
		filepath.ToSlash(base),
	} {
		if strings.TrimSpace(p) == "" {
			continue
		}
		uniq[p] = struct{}{}
	}

	out := make([]string, 0, len(uniq))
	for p := range uniq {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

func (r *Rule) tokenCountAndMode(text string) (int, string) {
	switch normalizeMode(r.Mode) {
	case "tokenizer":
		tok := normalizeTokenizer(r.Tokenizer)
		enc := normalizeEncoding(r.Encoding)
		count := tokenizerCount(text, tok, enc)
		return count, fmt.Sprintf("tokenizer:%s/%s", tok, enc)
	default:
		ratio := r.Ratio
		if ratio <= 0 {
			ratio = defaultRatio
		}
		words := mdtext.CountWords(text)
		count := int(math.Round(float64(words) * ratio))
		if count < 0 {
			count = 0
		}
		return count, fmt.Sprintf("heuristic:ratio=%.2f", ratio)
	}
}

func tokenizerCount(text, tokenizer, encoding string) int {
	_ = tokenizer // reserved for future tokenizer families.

	var re *regexp.Regexp
	switch encoding {
	case "p50k_base", "r50k_base", "gpt2":
		re = p50kPattern
	default:
		re = cl100kPattern
	}

	return len(re.FindAllStringIndex(text, -1))
}

// ApplySettings implements rule.Configurable.
func (r *Rule) ApplySettings(settings map[string]any) error {
	for k, v := range settings {
		if err := r.applySetting(k, v); err != nil {
			return err
		}
	}

	if err := validateTokenizerAndEncoding(r.Tokenizer, r.Encoding); err != nil {
		return err
	}

	return nil
}

func (r *Rule) applySetting(k string, v any) error {
	switch k {
	case "max":
		return r.applyMax(v)
	case "mode":
		return r.applyMode(v)
	case "ratio":
		return r.applyRatio(v)
	case "tokenizer":
		return r.applyTokenizer(v)
	case "encoding":
		return r.applyEncoding(v)
	case "budgets":
		return r.applyBudgets(v)
	default:
		return fmt.Errorf("token-budget: unknown setting %q", k)
	}
}

func (r *Rule) applyMax(v any) error {
	n, ok := toInt(v)
	if !ok {
		return fmt.Errorf("token-budget: max must be an integer, got %T", v)
	}
	if n <= 0 {
		return fmt.Errorf("token-budget: max must be positive, got %d", n)
	}
	r.Max = n
	return nil
}

func (r *Rule) applyMode(v any) error {
	s, ok := v.(string)
	if !ok {
		return fmt.Errorf("token-budget: mode must be a string, got %T", v)
	}
	m := normalizeMode(s)
	if !isValidMode(m) {
		return fmt.Errorf(
			"token-budget: invalid mode %q (valid: heuristic, tokenizer)",
			s,
		)
	}
	r.Mode = m
	return nil
}

func (r *Rule) applyRatio(v any) error {
	n, ok := toFloat(v)
	if !ok {
		return fmt.Errorf("token-budget: ratio must be a number, got %T", v)
	}
	if n <= 0 {
		return fmt.Errorf("token-budget: ratio must be positive, got %.4g", n)
	}
	r.Ratio = n
	return nil
}

func (r *Rule) applyTokenizer(v any) error {
	s, ok := v.(string)
	if !ok {
		return fmt.Errorf("token-budget: tokenizer must be a string, got %T", v)
	}
	tok := normalizeTokenizer(s)
	if !isValidTokenizer(tok) {
		return fmt.Errorf("token-budget: invalid tokenizer %q (valid: builtin)", s)
	}
	r.Tokenizer = tok
	return nil
}

func (r *Rule) applyEncoding(v any) error {
	s, ok := v.(string)
	if !ok {
		return fmt.Errorf("token-budget: encoding must be a string, got %T", v)
	}
	enc := normalizeEncoding(s)
	if !isValidEncoding(enc) {
		return fmt.Errorf(
			"token-budget: invalid encoding %q (valid: %s)",
			s,
			validEncodings,
		)
	}
	r.Encoding = enc
	return nil
}

func (r *Rule) applyBudgets(v any) error {
	budgets, err := parseBudgets(v)
	if err != nil {
		return err
	}
	r.Budgets = budgets
	return nil
}

func validateTokenizerAndEncoding(tokenizer, encoding string) error {
	tok := normalizeTokenizer(tokenizer)
	if !isValidTokenizer(tok) {
		return fmt.Errorf("token-budget: invalid tokenizer %q", tokenizer)
	}
	enc := normalizeEncoding(encoding)
	if !isValidEncoding(enc) {
		return fmt.Errorf("token-budget: invalid encoding %q", encoding)
	}
	return nil
}

func parseBudgets(v any) ([]budgetOverride, error) {
	items, ok := v.([]any)
	if !ok {
		if ms, ok2 := v.([]map[string]any); ok2 {
			items = make([]any, 0, len(ms))
			for _, m := range ms {
				items = append(items, m)
			}
		} else {
			return nil, fmt.Errorf("token-budget: budgets must be a list, got %T", v)
		}
	}

	result := make([]budgetOverride, 0, len(items))
	for idx, raw := range items {
		m, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("token-budget: budgets[%d] must be a mapping, got %T", idx, raw)
		}

		for key := range m {
			if key != "glob" && key != "max" {
				return nil, fmt.Errorf("token-budget: budgets[%d] has unknown key %q", idx, key)
			}
		}

		globStr, ok := m["glob"].(string)
		if !ok {
			return nil, fmt.Errorf("token-budget: budgets[%d].glob must be a string", idx)
		}
		if strings.TrimSpace(globStr) == "" {
			return nil, fmt.Errorf("token-budget: budgets[%d].glob must not be empty", idx)
		}

		maxVal, ok := toInt(m["max"])
		if !ok {
			return nil, fmt.Errorf("token-budget: budgets[%d].max must be an integer", idx)
		}
		if maxVal <= 0 {
			return nil, fmt.Errorf("token-budget: budgets[%d].max must be positive, got %d", idx, maxVal)
		}

		matcher, err := glob.Compile(globStr)
		if err != nil {
			return nil, fmt.Errorf("token-budget: budgets[%d].glob is invalid: %v", idx, err)
		}

		result = append(result, budgetOverride{
			Glob:    globStr,
			Max:     maxVal,
			matcher: matcher,
		})
	}

	return result, nil
}

// DefaultSettings implements rule.Configurable.
func (r *Rule) DefaultSettings() map[string]any {
	return map[string]any{
		"max":       defaultMax,
		"mode":      defaultMode,
		"ratio":     defaultRatio,
		"tokenizer": defaultTokenizer,
		"encoding":  defaultEncoding,
	}
}

func normalizeMode(s string) string {
	if strings.TrimSpace(s) == "" {
		return defaultMode
	}
	return strings.ToLower(strings.TrimSpace(s))
}

func normalizeTokenizer(s string) string {
	if strings.TrimSpace(s) == "" {
		return defaultTokenizer
	}
	return strings.ToLower(strings.TrimSpace(s))
}

func normalizeEncoding(s string) string {
	if strings.TrimSpace(s) == "" {
		return defaultEncoding
	}
	return strings.ToLower(strings.TrimSpace(s))
}

func isValidMode(s string) bool {
	return s == "heuristic" || s == "tokenizer"
}

func isValidTokenizer(s string) bool {
	return s == "builtin"
}

func isValidEncoding(s string) bool {
	switch s {
	case "cl100k_base", "p50k_base", "r50k_base", "gpt2":
		return true
	default:
		return false
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

var _ rule.Configurable = (*Rule)(nil)
