package schema

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"github.com/jeduden/mdsmith/internal/fieldinterp"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/yuin/goldmark/ast"
)

// DocHeading is a heading collected from the document under
// validation.
type DocHeading struct {
	Level int
	Text  string
	Line  int
}

// ExtractDocHeadings walks the document AST and collects every
// heading in source order, with its source line.
func ExtractDocHeadings(f *lint.File) []DocHeading {
	var out []DocHeading
	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		h, ok := n.(*ast.Heading)
		if !ok {
			return ast.WalkContinue, nil
		}
		text := headingText(h, f.Source)
		line := f.LineOfOffset(h.Lines().At(0).Start)
		out = append(out, DocHeading{Level: h.Level, Text: text, Line: line})
		return ast.WalkContinue, nil
	})
	return out
}

// MakeDiag is the diagnostic constructor the validator uses. Callers
// supply it so the schema package stays free of rule-ID coupling.
type MakeDiag func(file string, line int, msg string) lint.Diagnostic

// Validate walks the document AST against sch, emitting diagnostics
// for missing/extra/out-of-order sections, level mismatches,
// frontmatter that fails the schema's CUE constraints, and filename
// patterns. mkDiag builds the diagnostic with the caller's rule ID.
//
// docFM is the document's parsed front matter (nil when absent).
// When fmIsCUE is true, the front-matter values are themselves CUE
// expressions (the `cue-frontmatter` placeholder); the CUE check is
// skipped because the values are not concrete data.
func Validate(
	f *lint.File, sch *Schema, docFM map[string]any, fmIsCUE bool,
	mkDiag MakeDiag,
) []lint.Diagnostic {
	if sch == nil || sch.IsEmpty() {
		return nil
	}
	var diags []lint.Diagnostic

	diags = append(diags, validateFilename(f, sch, mkDiag)...)

	if !fmIsCUE {
		if err := ValidateFrontmatter(sch, docFM); err != nil {
			diags = append(diags, mkDiag(f.Path, 1,
				fmt.Sprintf(
					"front matter does not satisfy schema CUE constraints: %v",
					err)))
		}
	}

	rootLevel := sch.EffectiveRootLevel()
	heads := ExtractDocHeadings(f)
	body := skipBelow(heads, rootLevel)

	_, sd := validateScopes(f, sch.Sections, sch.Closed, body, 0, rootLevel, mkDiag)
	diags = append(diags, sd...)

	return diags
}

// skipBelow returns heads starting from the first heading at level
// >= rootLevel. Headings shallower than rootLevel (typically the H1
// document title when validating against an inline schema) are
// dropped so they do not appear as "unexpected" sections.
func skipBelow(heads []DocHeading, rootLevel int) []DocHeading {
	for i, h := range heads {
		if h.Level >= rootLevel {
			return heads[i:]
		}
	}
	return nil
}

// validateScopes walks scopes (the listed children of a single level)
// against docHeads starting at docIdx. expectedLevel is the heading
// level these scopes should appear at. Returns the new docIdx
// (position after consuming this scope-list) and emitted diagnostics.
//
// closed controls handling of unlisted headings at this level: when
// true, an unlisted heading flags a diagnostic; when false, it is
// tolerated. A wildcard scope ("...") always tolerates unlisted
// headings at its position.
func validateScopes(
	f *lint.File, scopes []Scope, closed bool, docHeads []DocHeading,
	docIdx int, expectedLevel int, mkDiag MakeDiag,
) (int, []lint.Diagnostic) {
	var diags []lint.Diagnostic
	requiredByText := buildRequiredByText(scopes)
	claimed := make(map[int]bool)
	allowExtra := false

	for i, sc := range scopes {
		if sc.Wildcard {
			allowExtra = true
			continue
		}
		if claimed[i] {
			continue
		}
		newIdx, scDiags, found := matchScope(
			f, scopes, i, expectedLevel, docHeads, docIdx,
			requiredByText, claimed, allowExtra, closed, mkDiag)
		diags = append(diags, scDiags...)
		docIdx = newIdx
		if found {
			allowExtra = false
		} else if !claimed[i] && sc.Required && !sc.Repeats {
			diags = append(diags, mkDiag(f.Path, 1,
				fmt.Sprintf("missing required section %q",
					formatHeading(expectedLevel, sc.Heading))))
		}
	}

	// Handle remaining doc headings: in a closed scope without a
	// trailing wildcard, the leftovers are flagged; otherwise they
	// are skipped past so the caller (a parent scope) does not see
	// them again.
	for docIdx < len(docHeads) {
		dh := docHeads[docIdx]
		if dh.Level < expectedLevel {
			break
		}
		if dh.Level == expectedLevel && closed && !allowExtra {
			diags = append(diags, mkDiag(f.Path, dh.Line,
				fmt.Sprintf("unexpected section %q",
					formatHeading(dh.Level, dh.Text))))
		}
		docIdx++
	}
	return docIdx, diags
}

func buildRequiredByText(scopes []Scope) map[string][]int {
	out := map[string][]int{}
	for i, sc := range scopes {
		if sc.Wildcard {
			continue
		}
		if fieldinterp.ContainsField(sc.Heading) {
			continue
		}
		out[sc.Heading] = append(out[sc.Heading], i)
		for _, a := range sc.Aliases {
			out[a] = append(out[a], i)
		}
	}
	return out
}

// matchScope advances docIdx looking for a heading matching the
// scope at scopes[idx]. Intervening doc headings either belong to a
// later listed scope (out-of-order), are unexpected (closed + no
// wildcard), or are descended into as part of an earlier scope's
// subtree. Returns the new docIdx, diagnostics, and whether the
// scope was matched.
func matchScope(
	f *lint.File, scopes []Scope, idx, expectedLevel int,
	docHeads []DocHeading, docIdx int,
	requiredByText map[string][]int, claimed map[int]bool,
	allowExtra, closed bool, mkDiag MakeDiag,
) (int, []lint.Diagnostic, bool) {
	sc := scopes[idx]
	var diags []lint.Diagnostic

	for docIdx < len(docHeads) {
		dh := docHeads[docIdx]
		// Shallower than us belongs to an ancestor — unless the text
		// still matches, in which case we claim it here with a
		// level-mismatch diagnostic. Without this branch a wrong-level
		// match would surface as both "missing required" (here) and
		// "unexpected" (when the caller revisits it).
		if dh.Level < expectedLevel {
			if scopeMatchesHeading(sc, dh) {
				return claimMatch(f, sc, idx, expectedLevel, docHeads, docIdx, claimed, mkDiag, diags)
			}
			return docIdx, diags, false
		}
		if scopeMatchesHeading(sc, dh) {
			return claimMatch(f, sc, idx, expectedLevel, docHeads, docIdx, claimed, mkDiag, diags)
		}
		if ooIdx := nextUnclaimed(requiredByText[dh.Text], claimed, idx+1); ooIdx >= 0 {
			newIdx, ooDiags := claimOutOfOrder(
				f, scopes, idx, ooIdx, expectedLevel, docHeads, docIdx, claimed, mkDiag)
			diags = append(diags, ooDiags...)
			docIdx = newIdx
			continue
		}
		// dh did not match this scope or any later listed scope by
		// text. Deeper than expected: orphan child of some unmatched
		// parent — consume silently. Same level: treat as unexpected
		// when closed and no wildcard has opened the door.
		if dh.Level > expectedLevel {
			docIdx++
			continue
		}
		if !allowExtra && closed {
			diags = append(diags, mkDiag(f.Path, dh.Line,
				fmt.Sprintf("unexpected section %q (expected %q)",
					formatHeading(dh.Level, dh.Text),
					formatHeading(expectedLevel, sc.Heading))))
		}
		docIdx++
	}
	return docIdx, diags, false
}

func levelDiagIfNeeded(
	f *lint.File, dh DocHeading, expectedLevel int, mkDiag MakeDiag,
) []lint.Diagnostic {
	if dh.Level == expectedLevel {
		return nil
	}
	return []lint.Diagnostic{mkDiag(f.Path, dh.Line,
		fmt.Sprintf("heading level mismatch for %q: expected h%d, got h%d",
			dh.Text, expectedLevel, dh.Level))}
}

// claimMatch marks scopes[idx] as matched against docHeads[docIdx],
// appending the level-mismatch diagnostic when applicable and
// recursing into the scope's child sections. Returns the advanced
// docIdx, combined diagnostics, and true.
func claimMatch(
	f *lint.File, sc Scope, idx, expectedLevel int,
	docHeads []DocHeading, docIdx int, claimed map[int]bool,
	mkDiag MakeDiag, prior []lint.Diagnostic,
) (int, []lint.Diagnostic, bool) {
	diags := append(prior, levelDiagIfNeeded(f, docHeads[docIdx], expectedLevel, mkDiag)...)
	claimed[idx] = true
	docIdx++
	if len(sc.Sections) > 0 {
		newIdx, childDiags := validateScopes(
			f, sc.Sections, sc.Closed, docHeads, docIdx,
			expectedLevel+1, mkDiag)
		diags = append(diags, childDiags...)
		docIdx = newIdx
	}
	return docIdx, diags, true
}

// claimOutOfOrder records that docHeads[docIdx] matches scopes[ooIdx]
// (a later listed scope), emits the out-of-order diagnostic, and
// recurses into the matched scope's child sections.
func claimOutOfOrder(
	f *lint.File, scopes []Scope, idx, ooIdx, expectedLevel int,
	docHeads []DocHeading, docIdx int, claimed map[int]bool,
	mkDiag MakeDiag,
) (int, []lint.Diagnostic) {
	sc := scopes[idx]
	ooSc := scopes[ooIdx]
	dh := docHeads[docIdx]
	diags := []lint.Diagnostic{mkDiag(f.Path, dh.Line,
		fmt.Sprintf("section %q out of order: expected after %q",
			formatHeading(dh.Level, dh.Text),
			formatHeading(expectedLevel, sc.Heading)))}
	diags = append(diags, levelDiagIfNeeded(f, dh, expectedLevel, mkDiag)...)
	claimed[ooIdx] = true
	docIdx++
	if len(ooSc.Sections) > 0 {
		newIdx, childDiags := validateScopes(
			f, ooSc.Sections, ooSc.Closed, docHeads, docIdx,
			expectedLevel+1, mkDiag)
		diags = append(diags, childDiags...)
		docIdx = newIdx
	}
	return docIdx, diags
}

func nextUnclaimed(cands []int, claimed map[int]bool, minIdx int) int {
	for _, i := range cands {
		if i >= minIdx && !claimed[i] {
			return i
		}
	}
	return -1
}

// MatchesHeading reports whether sc matches the heading text in dh.
// Exported so callers outside the validator (notably the per-scope
// rule walker in internal/rules/requiredstructure) reuse the same
// matching semantics — anchored regex for field-interpolated
// patterns, exact text otherwise, plus aliases and the "?"
// wildcard.
func MatchesHeading(sc Scope, dh DocHeading) bool {
	return scopeMatchesHeading(sc, dh)
}

func scopeMatchesHeading(sc Scope, dh DocHeading) bool {
	if sc.Wildcard {
		return false // wildcards never match a specific heading directly.
	}
	if sc.Heading == "?" {
		return true
	}
	if matchesText(sc.Heading, dh.Text) {
		return true
	}
	for _, a := range sc.Aliases {
		if matchesText(a, dh.Text) {
			return true
		}
	}
	return false
}

// patternRegexCache memoises compiled regexes for field-interpolated
// heading patterns. Recompiling per-call would be O(scopes ×
// headings) on every validation pass; caching by pattern string
// keeps the hot loop allocation-free after warm-up.
var patternRegexCache sync.Map // map[string]*regexp.Regexp; nil entry means compile error

func matchesText(pattern, text string) bool {
	if !fieldinterp.ContainsField(pattern) {
		return pattern == text
	}
	re := patternRegex(pattern)
	if re == nil {
		return false
	}
	return re.MatchString(text)
}

func patternRegex(pattern string) *regexp.Regexp {
	if v, ok := patternRegexCache.Load(pattern); ok {
		if v == nil {
			return nil
		}
		return v.(*regexp.Regexp)
	}
	parts := fieldinterp.SplitOnFields(pattern)
	var b strings.Builder
	b.WriteString("^")
	for i, p := range parts {
		b.WriteString(regexp.QuoteMeta(p))
		if i < len(parts)-1 {
			b.WriteString(".+")
		}
	}
	b.WriteString("$")
	re, err := regexp.Compile(b.String())
	if err != nil {
		patternRegexCache.Store(pattern, (*regexp.Regexp)(nil))
		return nil
	}
	patternRegexCache.Store(pattern, re)
	return re
}

func formatHeading(level int, text string) string {
	return strings.Repeat("#", level) + " " + text
}

// validateFilename checks that the document basename matches the
// schema's filename pattern (if configured).
func validateFilename(
	f *lint.File, sch *Schema, mkDiag MakeDiag,
) []lint.Diagnostic {
	pattern := sch.Require.Filename
	if pattern == "" {
		return nil
	}
	base := filepath.Base(f.Path)
	matched, err := filepath.Match(pattern, base)
	if err != nil {
		return []lint.Diagnostic{mkDiag(f.Path, 1,
			fmt.Sprintf("invalid filename pattern %q: %v", pattern, err))}
	}
	if !matched {
		return []lint.Diagnostic{mkDiag(f.Path, 1,
			fmt.Sprintf("filename %q does not match required pattern %q",
				base, pattern))}
	}
	return nil
}

// ValidateFrontmatter compiles sch.Frontmatter into a CUE schema and
// unifies it with fm (the document's parsed front matter).
func ValidateFrontmatter(sch *Schema, fm map[string]any) error {
	expr := sch.FrontmatterCUE()
	if strings.TrimSpace(expr) == "" {
		return nil
	}
	ctx := cuecontext.New()
	schemaVal := ctx.CompileString(expr)
	if err := schemaVal.Err(); err != nil {
		return fmt.Errorf("invalid CUE schema: %w", err)
	}
	if fm == nil {
		fm = map[string]any{}
	}
	data, err := json.Marshal(fm)
	if err != nil {
		return fmt.Errorf("serialize front matter: %w", err)
	}
	dataVal := ctx.CompileBytes(data)
	if err := dataVal.Err(); err != nil {
		return fmt.Errorf("compile front matter: %w", err)
	}
	merged := schemaVal.Unify(dataVal)
	if err := merged.Validate(cue.Concrete(true)); err != nil {
		return err
	}
	return nil
}

// ValidateFrontmatterSyntax checks that the schema's frontmatter
// constraints compile as CUE. Returns nil if there are no
// constraints.
func ValidateFrontmatterSyntax(sch *Schema) error {
	expr := sch.FrontmatterCUE()
	if strings.TrimSpace(expr) == "" {
		return nil
	}
	ctx := cuecontext.New()
	v := ctx.CompileString(expr)
	if err := v.Err(); err != nil {
		return fmt.Errorf("invalid schema frontmatter CUE: %w", err)
	}
	return nil
}
