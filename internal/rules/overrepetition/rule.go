// Package overrepetition implements MDS075, which flags any content
// word that repeats more than a configured ceiling within a scope.
package overrepetition

import (
	"fmt"
	"slices"
	"strings"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/mdtext"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/jeduden/mdsmith/internal/rules/astutil"
	"github.com/jeduden/mdsmith/internal/rules/settings"
)

func init() {
	rule.Register(&Rule{})
}

// Rule implements MDS075. It counts every content word per scope unit
// (file, section, or paragraph) and emits a diagnostic when any word's
// count exceeds Max. Words in Stopwords are subtracted before checking.
// The rule is off by default and requires Max > 0 to fire.
type Rule struct {
	Scope          string
	Max            int // 0 = unconfigured (skip); -1 = unlimited (skip)
	MinLength      int
	Stopwords      []string
	lowerStopwords map[string]struct{}
}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "MDS075" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "over-repetition" }

// WordlistTarget implements rule.WordlistConsumer: resolved lists:
// entries union into the "stopwords" setting.
func (r *Rule) WordlistTarget() string { return "stopwords" }

var _ rule.WordlistConsumer = (*Rule)(nil)

// Category implements rule.Rule.
func (r *Rule) Category() string { return "prose" }

// EnabledByDefault implements rule.Defaultable.
func (r *Rule) EnabledByDefault() bool { return false }

// Check implements rule.Rule.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	if f.AST == nil || r.Max <= 0 {
		return nil
	}
	switch r.Scope {
	case "file":
		return r.checkFile(f)
	case "paragraph":
		return r.checkParagraphs(f)
	default:
		return r.checkSections(f)
	}
}

// checkFile checks word frequency across all prose in the file as one unit.
func (r *Rule) checkFile(f *lint.File) []lint.Diagnostic {
	paragraphs := astutil.CollectSectionParagraphsWithText(f)
	freq := make(map[string]int)
	for i := range paragraphs {
		r.accum(freq, paragraphs[i].ExtractText(f.Source))
	}
	r.removeStopwords(freq)
	return r.diagFromFreq(freq, 1, "file", f.Path)
}

// checkSections checks word frequency per heading-bounded section.
// Prose before the first heading is treated as an implicit preamble section
// anchored at line 1. A single freq map is reused across sections (cleared
// between them) to keep the active-path alloc count within the ≤10 budget.
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

	freq := make(map[string]int, 32) // 32 covers typical prose vocabulary per section

	// Check preamble paragraphs (before the first heading) as one implicit section.
	// Skip the allocation when no preamble paragraphs exist (the common case).
	firstHeadingLine := headings[0].Line
	for j := range paragraphs {
		if paragraphs[j].Line < firstHeadingLine {
			r.accum(freq, paragraphs[j].ExtractText(f.Source))
		}
	}
	var diags []lint.Diagnostic
	if len(freq) > 0 {
		r.removeStopwords(freq)
		diags = append(diags, r.diagFromFreq(freq, 1, "section", f.Path)...)
	}

	for i, h := range headings {
		end := astutil.SectionEnd(headings, i, totalLines)
		clear(freq)
		for j := range paragraphs {
			if paragraphs[j].Line < h.Line || paragraphs[j].Line >= end {
				continue
			}
			r.accum(freq, paragraphs[j].ExtractText(f.Source))
		}
		r.removeStopwords(freq)
		diags = append(diags, r.diagFromFreq(freq, h.Line, "section", f.Path)...)
	}
	return diags
}

// checkParagraphs checks word frequency per paragraph.
// A single freq map is reused across paragraphs (cleared between them).
func (r *Rule) checkParagraphs(f *lint.File) []lint.Diagnostic {
	paragraphs := astutil.CollectSectionParagraphsWithText(f)
	//nolint:prealloc // diagFromFreq returns nil on no violation; preallocating wastes memory
	var diags []lint.Diagnostic
	freq := make(map[string]int, 32) // 32 covers typical prose vocabulary per paragraph
	for i := range paragraphs {
		clear(freq)
		r.accum(freq, paragraphs[i].ExtractText(f.Source))
		r.removeStopwords(freq)
		diags = append(diags, r.diagFromFreq(freq, paragraphs[i].Line, "paragraph", f.Path)...)
	}
	return diags
}

// accum tokenizes text into freq using the shared WordFrequencyInto tokenizer.
// Stopword removal is done once per scope unit by removeStopwords, not here,
// so this path makes zero allocations when prose words are already lowercase.
func (r *Rule) accum(freq map[string]int, text string) {
	mdtext.WordFrequencyInto(freq, text, r.MinLength)
}

// removeStopwords deletes every stopword entry from freq. Called once per
// scope unit after all paragraphs in that unit have been accumulated.
func (r *Rule) removeStopwords(freq map[string]int) {
	for w := range r.lowerStopwords {
		delete(freq, w)
	}
}

// diagFromFreq emits a diagnostic for every word in freq that exceeds Max.
// Results are sorted by message so output order is deterministic.
func (r *Rule) diagFromFreq(freq map[string]int, line int, scope, path string) []lint.Diagnostic {
	var diags []lint.Diagnostic
	for word, cnt := range freq {
		if cnt > r.Max {
			diags = append(diags, lint.Diagnostic{
				File:     path,
				Line:     line,
				Column:   1,
				RuleID:   r.ID(),
				RuleName: r.Name(),
				Severity: lint.Warning,
				Message:  fmt.Sprintf("%q repeated %d time(s) in %s (max %d)", word, cnt, scope, r.Max),
			})
		}
	}
	slices.SortFunc(diags, cmpDiagMessage)
	return diags
}

// cmpDiagMessage is a package-level comparison function so it does not
// escape to the heap on every slices.SortFunc call.
func cmpDiagMessage(a, b lint.Diagnostic) int {
	return strings.Compare(a.Message, b.Message)
}

// ApplySettings implements rule.Configurable.
func (r *Rule) ApplySettings(s map[string]any) error {
	for k, v := range s {
		var err error
		switch k {
		case "scope":
			err = r.applyScope(v)
		case "max":
			err = r.applyMax(v)
		case "min-length":
			err = r.applyMinLength(v)
		case "stopwords":
			err = r.applyStopwords(v)
		default:
			return fmt.Errorf("over-repetition: unknown setting %q", k)
		}
		if err != nil {
			return err
		}
	}
	r.buildStopwordSet()
	return nil
}

func (r *Rule) applyScope(v any) error {
	str, ok := v.(string)
	if !ok {
		return fmt.Errorf("over-repetition: scope must be a string, got %T", v)
	}
	switch str {
	case "file", "section", "paragraph":
		r.Scope = str
		return nil
	default:
		return fmt.Errorf("over-repetition: scope must be file, section, or paragraph, got %q", str)
	}
}

func (r *Rule) applyMax(v any) error {
	n, ok := settings.ToInt(v)
	if !ok {
		return fmt.Errorf("over-repetition: max must be an integer, got %T", v)
	}
	r.Max = n
	return nil
}

func (r *Rule) applyMinLength(v any) error {
	n, ok := settings.ToInt(v)
	if !ok {
		return fmt.Errorf("over-repetition: min-length must be an integer, got %T", v)
	}
	if n < 0 {
		return fmt.Errorf("over-repetition: min-length must be >= 0, got %d", n)
	}
	r.MinLength = n
	return nil
}

func (r *Rule) applyStopwords(v any) error {
	ss, ok := settings.ToStringSlice(v)
	if !ok {
		return fmt.Errorf("over-repetition: stopwords must be a list of strings, got %T", v)
	}
	r.Stopwords = ss
	return nil
}

// buildStopwordSet converts Stopwords into a fast-lookup set with
// case-folded keys, matching WordFrequency's output.
func (r *Rule) buildStopwordSet() {
	if len(r.Stopwords) == 0 {
		r.lowerStopwords = nil
		return
	}
	r.lowerStopwords = make(map[string]struct{}, len(r.Stopwords))
	for _, w := range r.Stopwords {
		r.lowerStopwords[strings.ToLower(w)] = struct{}{}
	}
}

// DefaultSettings implements rule.Configurable.
func (r *Rule) DefaultSettings() map[string]any {
	return map[string]any{
		"scope":      "section",
		"max":        -1,
		"min-length": 4,
		"stopwords":  []string{},
	}
}

var (
	_ rule.Configurable = (*Rule)(nil)
	_ rule.Defaultable  = (*Rule)(nil)
)
