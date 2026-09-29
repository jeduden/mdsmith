// Package occurrence implements MDS060, which flags a scope that contains
// a configured token or pattern too many or too few times.
package occurrence

import (
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/jeduden/mdsmith/internal/rules/astutil"
	"github.com/jeduden/mdsmith/internal/rules/settings"
)

// compiledPatterns caches compiled regexes by source string so
// ApplySettings does not rebuild the NFA on every file check.
var compiledPatterns sync.Map // map[string]*regexp.Regexp

func init() {
	rule.Register(&Rule{})
}

// Rule implements MDS060. It counts how often each token or a regex
// pattern occurs within each configured scope unit (file, section, or
// paragraph), and emits a diagnostic whenever the count falls outside
// the configured [min, max] band.
type Rule struct {
	Scope         string
	Tokens        []string
	lowerTokens   []string // pre-lowercased tokens for case-insensitive matching
	patternSource string
	Pattern       *regexp.Regexp
	literal       string // patternSource when it has no regex metacharacters
	lowerLiteral  string // literal, lowercased for case-insensitive matching
	Min           int
	Max           int  // -1 means unbounded
	maxSet        bool // true once any ApplySettings call has set max
	Count         string
	CaseSensitive bool
}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "MDS060" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "occurrence" }

// WordlistTarget implements rule.WordlistConsumer: resolved lists: entries
// union into the "tokens" setting.
func (r *Rule) WordlistTarget() string { return "tokens" }

var _ rule.WordlistConsumer = (*Rule)(nil)

// Category implements rule.Rule.
func (r *Rule) Category() string { return "prose" }

// EnabledByDefault implements rule.Defaultable.
func (r *Rule) EnabledByDefault() bool { return false }

// Check implements rule.Rule.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	if f.AST == nil {
		return nil
	}
	if len(r.Tokens) == 0 && r.Pattern == nil {
		return nil
	}
	pr := collectProse(f, r.needsLower(), r.Scope == "section")
	paras := pr.paras
	// totals holds one tally per token, then one for the pattern,
	// cleared per scope unit. Typical configs fit the stack array.
	n := len(r.Tokens)
	if r.Pattern != nil {
		n++
	}
	var small [16]int
	var totals []int
	if n <= len(small) {
		totals = small[:n]
	} else {
		totals = make([]int, n)
	}
	switch r.Scope {
	case "file":
		for i := range paras {
			r.tally(paras[i].Text, paras[i].Lower, totals)
		}
		return r.emit(nil, totals, 1, "file", f.Path)
	case "section":
		return r.checkSections(f, pr, totals)
	default:
		var diags []lint.Diagnostic
		for i := range paras {
			clear(totals)
			r.tally(paras[i].Text, paras[i].Lower, totals)
			diags = r.emit(diags, totals, paras[i].Line, "paragraph", f.Path)
		}
		return diags
	}
}

// checkSections counts per heading-bounded section. A shallow
// heading's window (astutil.SectionEnd) extends through its nested
// subsections, and each subsection is still counted against its own
// narrower window. lo is a skip-ahead-only cursor: headings ascend, so
// a paragraph before one heading's start is before every later one's.
// It never advances past a window's paragraphs, since a nested
// heading's window may need them again.
func (r *Rule) checkSections(f *lint.File, pr prose, totals []int) []lint.Diagnostic {
	paras, headings := pr.paras, pr.headings
	totalLines := len(f.Lines)
	if totalLines > 0 && len(f.Lines[totalLines-1]) == 0 {
		totalLines--
	}
	var diags []lint.Diagnostic
	lo := 0
	for i, h := range headings {
		end := astutil.SectionEnd(headings, i, totalLines)
		for lo < len(paras) && paras[lo].Line < h.Line {
			lo++
		}
		clear(totals)
		for j := lo; j < len(paras) && paras[j].Line < end; j++ {
			r.tally(paras[j].Text, paras[j].Lower, totals)
		}
		diags = r.emit(diags, totals, h.Line, "section", f.Path)
	}
	return diags
}

// needsLower reports whether any configured matcher compares against
// lowercased text: tokens and literal patterns when matching ignores
// case. A regex carries its own (?i) flag and reads the original text.
func (r *Rule) needsLower() bool {
	return !r.CaseSensitive && (len(r.Tokens) > 0 || r.literal != "")
}

// tally adds one scope unit's match counts into totals: one slot per
// token, then the pattern's slot last. text is the original paragraph
// text; search is text lowercased when needsLower, else text itself.
func (r *Rule) tally(text, search string, totals []int) {
	for ti := range r.Tokens {
		totals[ti] += r.countToken(search, ti)
	}
	if r.Pattern != nil {
		if r.literal != "" {
			totals[len(r.Tokens)] += r.countLiteral(text, search)
		} else {
			totals[len(r.Tokens)] += r.countPattern(text)
		}
	}
}

// emit appends the diagnostics for one scope unit's totals: a single
// bound check on their sum in "combined" mode, else one per token and
// one for the pattern.
func (r *Rule) emit(diags []lint.Diagnostic, totals []int, line int, scope, path string) []lint.Diagnostic {
	if r.Count == "combined" {
		sum := 0
		for _, c := range totals {
			sum += c
		}
		return append(diags, r.diagCombined(sum, line, scope, path)...)
	}
	for ti, tok := range r.Tokens {
		diags = append(diags, r.diagEach(totals[ti], line, scope, tok, path)...)
	}
	if r.Pattern != nil {
		diags = append(diags, r.diagEach(totals[len(r.Tokens)], line, scope, r.patternSource, path)...)
	}
	return diags
}

// countToken counts non-overlapping occurrences of tokens[ti] in text.
// text must already be lowercased when matching ignores case.
func (r *Rule) countToken(text string, ti int) int {
	var tok string
	if r.CaseSensitive {
		tok = r.Tokens[ti]
	} else {
		tok = r.lowerTokens[ti]
	}
	if len(tok) == 0 {
		return 0
	}
	return strings.Count(text, tok)
}

// countLiteral counts a pattern with no regex metacharacters without
// running the regex engine, which allocates per match.
func (r *Rule) countLiteral(text, search string) int {
	if r.CaseSensitive {
		return strings.Count(text, r.literal)
	}
	return strings.Count(search, r.lowerLiteral)
}

// countPattern counts regexp matches in text. Caller must ensure r.Pattern != nil.
func (r *Rule) countPattern(text string) int {
	return len(r.Pattern.FindAllStringIndex(text, -1))
}

// diagEach emits a diagnostic when cnt is outside [min, max] for a single
// token or pattern.
func (r *Rule) diagEach(cnt, line int, scope, label, path string) []lint.Diagnostic {
	if cnt >= r.Min && (r.Max < 0 || cnt <= r.Max) {
		return nil
	}
	return []lint.Diagnostic{{
		File:     path,
		Line:     line,
		Column:   1,
		RuleID:   r.ID(),
		RuleName: r.Name(),
		Severity: lint.Warning,
		Message:  r.boundMessage(label, cnt, scope),
	}}
}

// diagCombined emits a diagnostic when the combined count is outside [min, max].
func (r *Rule) diagCombined(cnt, line int, scope, path string) []lint.Diagnostic {
	if cnt >= r.Min && (r.Max < 0 || cnt <= r.Max) {
		return nil
	}
	label := r.patternSource
	if label == "" {
		label = "tokens"
	}
	return []lint.Diagnostic{{
		File:     path,
		Line:     line,
		Column:   1,
		RuleID:   r.ID(),
		RuleName: r.Name(),
		Severity: lint.Warning,
		Message:  r.boundMessage(label, cnt, scope),
	}}
}

func (r *Rule) boundMessage(label string, cnt int, scope string) string {
	if r.Max >= 0 && cnt > r.Max {
		return fmt.Sprintf("%q appears %d time(s) in %s (max %d)", label, cnt, scope, r.Max)
	}
	return fmt.Sprintf("%q appears %d time(s) in %s (min %d)", label, cnt, scope, r.Min)
}

// ApplySettings implements rule.Configurable.
func (r *Rule) ApplySettings(s map[string]any) error {
	// Collect raw values before deriving compiled state, because map
	// iteration order is undefined and case-sensitive must be known
	// before the pattern is compiled.
	rawPattern := ""
	for k, v := range s {
		var err error
		switch k {
		case "scope":
			err = r.applyScope(v)
		case "tokens":
			err = r.applyTokens(v)
		case "pattern":
			rawPattern, err = extractPattern(v)
		case "min":
			err = r.applyMin(v)
		case "max":
			err = r.applyMax(v)
		case "count":
			err = r.applyCount(v)
		case "case-sensitive":
			err = r.applyCaseSensitive(v)
		default:
			return fmt.Errorf("occurrence: unknown setting %q", k)
		}
		if err != nil {
			return err
		}
	}
	return r.finalizeSettings(rawPattern)
}

func (r *Rule) applyScope(v any) error {
	str, ok := v.(string)
	if !ok {
		return fmt.Errorf("occurrence: scope must be a string, got %T", v)
	}
	switch str {
	case "file", "section", "paragraph":
		r.Scope = str
		return nil
	default:
		return fmt.Errorf("occurrence: scope must be file, section, or paragraph, got %q", str)
	}
}

func (r *Rule) applyTokens(v any) error {
	ss, ok := settings.ToStringSlice(v)
	if !ok {
		return fmt.Errorf("occurrence: tokens must be a list of strings, got %T", v)
	}
	// tokens uses the default replace merge mode (not append); no
	// SettingMergeMode override is needed.
	r.Tokens = ss
	return nil
}

func extractPattern(v any) (string, error) {
	str, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("occurrence: pattern must be a string, got %T", v)
	}
	return str, nil
}

func (r *Rule) applyMin(v any) error {
	n, ok := settings.ToInt(v)
	if !ok {
		return fmt.Errorf("occurrence: min must be an integer, got %T", v)
	}
	if n < 0 {
		return fmt.Errorf("occurrence: min must be >= 0, got %d", n)
	}
	r.Min = n
	return nil
}

func (r *Rule) applyMax(v any) error {
	n, ok := settings.ToInt(v)
	if !ok {
		return fmt.Errorf("occurrence: max must be an integer, got %T", v)
	}
	if n < -1 {
		return fmt.Errorf("occurrence: max must be -1 (unbounded) or >= 0, got %d", n)
	}
	r.Max = n
	r.maxSet = true
	return nil
}

func (r *Rule) applyCount(v any) error {
	str, ok := v.(string)
	if !ok {
		return fmt.Errorf("occurrence: count must be a string, got %T", v)
	}
	switch str {
	case "each", "combined":
		r.Count = str
		return nil
	default:
		return fmt.Errorf("occurrence: count must be each or combined, got %q", str)
	}
}

func (r *Rule) applyCaseSensitive(v any) error {
	b, ok := v.(bool)
	if !ok {
		return fmt.Errorf("occurrence: case-sensitive must be a bool, got %T", v)
	}
	r.CaseSensitive = b
	return nil
}

// finalizeSettings compiles the pattern (if any) and builds lowerTokens.
// Called after all scalar settings are applied so CaseSensitive is final.
func (r *Rule) finalizeSettings(rawPattern string) error {
	if rawPattern != "" {
		if err := r.compileAndSetPattern(rawPattern); err != nil {
			return err
		}
	}
	if len(r.Tokens) > 0 && r.Pattern != nil {
		// Clear the compiled pattern so a subsequent ApplySettings call
		// that supplies only tokens (no pattern) does not see stale state.
		r.Pattern = nil
		r.patternSource = ""
		r.literal, r.lowerLiteral = "", ""
		return fmt.Errorf("occurrence: tokens and pattern are mutually exclusive")
	}
	if !r.maxSet {
		// The zero value of Max is 0, a real bound; an unset max means
		// unbounded.
		r.Max = -1
	}
	if r.Max >= 0 && r.Min > r.Max {
		err := fmt.Errorf("occurrence: min (%d) must be <= max (%d)", r.Min, r.Max)
		// Reset the rejected band so a reused instance does not flag
		// every scope unit.
		r.Min, r.Max = 0, -1
		return err
	}
	r.literal, r.lowerLiteral = "", ""
	if r.Pattern != nil && regexp.QuoteMeta(r.patternSource) == r.patternSource {
		r.literal = r.patternSource
		r.lowerLiteral = strings.ToLower(r.patternSource)
	}
	if !r.CaseSensitive && len(r.Tokens) > 0 {
		r.lowerTokens = make([]string, len(r.Tokens))
		for i, t := range r.Tokens {
			r.lowerTokens[i] = strings.ToLower(t)
		}
	}
	return nil
}

// compileAndSetPattern compiles rawPattern (with (?i) prefix when not
// CaseSensitive) and stores the result in r.Pattern and r.patternSource.
func (r *Rule) compileAndSetPattern(rawPattern string) error {
	src := rawPattern
	if !r.CaseSensitive {
		src = "(?i)" + rawPattern
	}
	if actual, loaded := compiledPatterns.Load(src); loaded {
		r.patternSource = rawPattern
		r.Pattern = actual.(*regexp.Regexp)
		return nil
	}
	re, err := regexp.Compile(src)
	if err != nil {
		return fmt.Errorf("occurrence: pattern %q is not a valid Go RE2 regex: %w", rawPattern, err)
	}
	actual, _ := compiledPatterns.LoadOrStore(src, re)
	r.patternSource = rawPattern
	r.Pattern = actual.(*regexp.Regexp)
	return nil
}

// DefaultSettings implements rule.Configurable.
func (r *Rule) DefaultSettings() map[string]any {
	return map[string]any{
		"scope":          "paragraph",
		"tokens":         []string{},
		"pattern":        "",
		"min":            0,
		"max":            -1,
		"count":          "each",
		"case-sensitive": false,
	}
}

var (
	_ rule.Configurable = (*Rule)(nil)
	_ rule.Defaultable  = (*Rule)(nil)
)
