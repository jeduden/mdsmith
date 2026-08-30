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
	Min           int
	Max           int // -1 means unbounded
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
	switch r.Scope {
	case "file":
		return r.checkFile(f)
	case "section":
		return r.checkSections(f)
	default:
		return r.checkParagraphs(f)
	}
}

// checkFile counts across all prose paragraphs in the file as one unit.
func (r *Rule) checkFile(f *lint.File) []lint.Diagnostic {
	paragraphs := astutil.CollectSectionParagraphsWithText(f)
	if r.Count == "combined" {
		combined := 0
		for i := range paragraphs {
			combined += r.countCombined(paragraphs[i].ExtractText(f.Source))
		}
		return r.diagCombined(combined, 1, "file", f.Path)
	}
	// "each" mode: iterate paragraphs in the outer loop so each paragraph's
	// text is lowercased at most once regardless of token count. Skip the
	// lowercasing entirely when r.Tokens is empty (Pattern-only config) —
	// searchText's strings.ToLower would otherwise allocate once per
	// paragraph for a loop that never runs.
	var diags []lint.Diagnostic
	if len(r.Tokens) > 0 {
		totals := make([]int, len(r.Tokens))
		for i := range paragraphs {
			stext := r.searchText(paragraphs[i].ExtractText(f.Source))
			for ti := range r.Tokens {
				totals[ti] += r.countToken(stext, ti)
			}
		}
		for ti, tok := range r.Tokens {
			diags = append(diags, r.diagEach(totals[ti], 1, "file", tok, f.Path)...)
		}
	}
	if r.Pattern != nil {
		total := 0
		for i := range paragraphs {
			total += r.countPattern(paragraphs[i].ExtractText(f.Source))
		}
		diags = append(diags, r.diagEach(total, 1, "file", r.patternSource, f.Path)...)
	}
	return diags
}

// checkSections counts per heading-bounded section.
func (r *Rule) checkSections(f *lint.File) []lint.Diagnostic {
	headings := astutil.CollectSectionHeadings(f)
	if len(headings) == 0 {
		return nil
	}
	paragraphs := astutil.CollectSectionParagraphsWithText(f)
	totalLines := len(f.Lines)
	if totalLines > 0 && len(f.Lines[totalLines-1]) == 0 {
		totalLines--
	}
	var diags []lint.Diagnostic
	// combinedLo, tokensLo, and patternLo are skip-ahead-only cursors
	// into paragraphs, each threaded across the whole heading loop.
	// Headings are ascending, so a paragraph before headings[i].Line is
	// also before every later heading's start — safe to skip for good,
	// matching astutil.SectionBodies's lo cursor. None of them advance
	// past the paragraphs a heading's own window collects: unlike
	// maxsectionlength's flat, non-overlapping partition,
	// astutil.SectionEnd's window for a shallow heading extends past
	// its nested subsections, so a subsection's own iteration must
	// still be able to collect paragraphs an ancestor heading's wider
	// window already walked. This still turns the "skip the
	// already-passed prefix" work into O(paragraphs) total per active
	// mode instead of O(headings) rescans of it; the "collect this
	// heading's window" work stays proportional to that section's own
	// size, which is unavoidable for a hierarchical section model. See
	// docs/development/high-performance-go.md's "Skip work you don't
	// need". combined/tokens/pattern never advance in the same Check
	// call (r.Count picks exactly one branch), but each keeps its own
	// cursor since a single heading's "each" branch can drive both the
	// tokens loop and countPatternInRange over the same window.
	var combinedLo, tokensLo, patternLo int
	for i, h := range headings {
		end := astutil.SectionEnd(headings, i, totalLines)
		if r.Count == "combined" {
			combined := r.countCombinedInRange(paragraphs, f.Source, &combinedLo, h.Line, end)
			diags = append(diags, r.diagCombined(combined, h.Line, "section", f.Path)...)
		} else {
			// "each" mode: iterate paragraphs once, pre-lowercasing text per
			// paragraph so case-insensitive matching allocates one string per
			// paragraph, not one per (paragraph × token). Skip entirely when
			// r.Tokens is empty (Pattern-only config) — searchText's
			// strings.ToLower would otherwise allocate once per paragraph
			// for a loop that never runs.
			if len(r.Tokens) > 0 {
				totals := make([]int, len(r.Tokens))
				for tokensLo < len(paragraphs) && paragraphs[tokensLo].Line < h.Line {
					tokensLo++
				}
				for j := tokensLo; j < len(paragraphs) && paragraphs[j].Line < end; j++ {
					stext := r.searchText(paragraphs[j].ExtractText(f.Source))
					for ti := range r.Tokens {
						totals[ti] += r.countToken(stext, ti)
					}
				}
				for ti, tok := range r.Tokens {
					diags = append(diags, r.diagEach(totals[ti], h.Line, "section", tok, f.Path)...)
				}
			}
			if r.Pattern != nil {
				cnt := r.countPatternInRange(paragraphs, f.Source, &patternLo, h.Line, end)
				diags = append(diags, r.diagEach(cnt, h.Line, "section", r.patternSource, f.Path)...)
			}
		}
	}
	return diags
}

// checkParagraphs counts per paragraph.
func (r *Rule) checkParagraphs(f *lint.File) []lint.Diagnostic {
	paragraphs := astutil.CollectSectionParagraphsWithText(f)
	var diags []lint.Diagnostic
	for i := range paragraphs {
		text := paragraphs[i].ExtractText(f.Source)
		line := paragraphs[i].Line
		if r.Count == "combined" {
			combined := r.countCombined(text)
			diags = append(diags, r.diagCombined(combined, line, "paragraph", f.Path)...)
		} else {
			// Skip searchText's strings.ToLower allocation when r.Tokens is
			// empty (Pattern-only config) — the loop below never runs.
			if len(r.Tokens) > 0 {
				stext := r.searchText(text)
				for ti, tok := range r.Tokens {
					cnt := r.countToken(stext, ti)
					diags = append(diags, r.diagEach(cnt, line, "paragraph", tok, f.Path)...)
				}
			}
			if r.Pattern != nil {
				cnt := r.countPattern(text)
				diags = append(diags, r.diagEach(cnt, line, "paragraph", r.patternSource, f.Path)...)
			}
		}
	}
	return diags
}

// countCombinedInRange sums all match counts for paragraphs in
// [start, end). paragraphs must be in ascending Line order; lo is a
// skip-ahead-only cursor a caller threads across a sequence of
// ascending-start windows over the same paragraphs slice (the windows
// themselves may overlap, e.g. a shallow heading's window containing a
// nested subsection's), so the "skip the already-passed prefix" work
// runs in O(len(paragraphs)) total across the whole sequence of calls
// instead of being repeated per call — see checkSections's cursor
// comment. lo is deliberately NOT advanced past the paragraphs this
// call collects: a later call in the sequence may need to collect
// them again.
func (r *Rule) countCombinedInRange(
	paragraphs []astutil.SectionParagraph, source []byte, lo *int, start, end int,
) int {
	for *lo < len(paragraphs) && paragraphs[*lo].Line < start {
		*lo++
	}
	total := 0
	for i := *lo; i < len(paragraphs) && paragraphs[i].Line < end; i++ {
		total += r.countCombined(paragraphs[i].ExtractText(source))
	}
	return total
}

// countPatternInRange counts pattern matches for paragraphs in
// [start, end). See countCombinedInRange for the lo cursor contract.
func (r *Rule) countPatternInRange(
	paragraphs []astutil.SectionParagraph, source []byte, lo *int, start, end int,
) int {
	for *lo < len(paragraphs) && paragraphs[*lo].Line < start {
		*lo++
	}
	total := 0
	for i := *lo; i < len(paragraphs) && paragraphs[i].Line < end; i++ {
		total += r.countPattern(paragraphs[i].ExtractText(source))
	}
	return total
}

// countCombined returns the total match count for all tokens or the pattern.
func (r *Rule) countCombined(text string) int {
	if r.Pattern != nil {
		return r.countPattern(text)
	}
	// Pre-lowercase once for all tokens to avoid one allocation per token.
	stext := r.searchText(text)
	total := 0
	for ti := range r.Tokens {
		total += r.countToken(stext, ti)
	}
	return total
}

// searchText returns text ready for token matching: lowercased for
// case-insensitive mode, unchanged otherwise. Callers must invoke
// searchText once per scope unit before looping over tokens.
func (r *Rule) searchText(text string) string {
	if !r.CaseSensitive {
		return strings.ToLower(text)
	}
	return text
}

// countToken counts non-overlapping occurrences of tokens[ti] in text.
// text must already be case-normalized via searchText.
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
	r.Max = n
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
		return fmt.Errorf("occurrence: tokens and pattern are mutually exclusive")
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
