package schema

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/jeduden/mdsmith/internal/lint"
)

// acronymToken matches all-caps alphanumeric tokens of length 2-6.
// Must begin with a capital letter; trailing digits are allowed
// (e.g. "OAuth2" is not flagged because the lowercase rules it out,
// but "OAUTH2" would match).
var acronymToken = regexp.MustCompile(`\b[A-Z][A-Z0-9]{1,5}\b`)

// ValidateAcronyms flags all-caps tokens (length 2-6) that appear
// for the first time inside a configured scope without a
// parenthesised expansion. KnownSafe tokens are exempt; a missing
// scope list applies the check document-wide.
//
// First-use state is per-scope. An acronym defined inside "Check"
// must be re-introduced when it first appears inside "Expected" —
// the rule treats the two scope passes independently so each named
// scope reads as self-contained.
//
// Heading lines are excluded from the scan in both modes: prose
// rules apply to body text, and a section heading like
// "## OIDC configuration" should not be flagged as the first use
// of OIDC when the body that follows immediately spells out the
// expansion.
func ValidateAcronyms(
	f *lint.File, sch *Schema, docFM map[string]any, mkDiag MakeDiag,
) []lint.Diagnostic {
	if sch == nil || sch.Acronyms == nil {
		return nil
	}
	a := sch.Acronyms
	known := buildKnownSet(a.KnownSafe)

	headingLines := documentHeadingLines(f)
	ranges := acronymRanges(f, sch, a.Scope, docFM)
	var diags []lint.Diagnostic
	for _, rng := range ranges {
		diags = append(diags, checkAcronymsInRange(f, rng, known, headingLines, mkDiag)...)
	}
	return diags
}

// documentHeadingLines returns the set of 1-based line numbers
// occupied by Markdown headings in f. Used to skip heading lines
// during acronym scans so a "## OIDC configuration" heading does
// not consume the "first use" slot before the body's
// parenthesised expansion.
func documentHeadingLines(f *lint.File) map[int]bool {
	out := map[int]bool{}
	for _, h := range ExtractDocHeadings(f) {
		out[h.Line] = true
	}
	return out
}

func buildKnownSet(list []string) map[string]bool {
	out := make(map[string]bool, len(list))
	for _, s := range list {
		out[s] = true
	}
	return out
}

// lineRange identifies a half-open 1-based line window for an
// acronym scope pass: Start is inclusive, End is exclusive.
type lineRange struct {
	Start int
	End   int
}

// acronymRanges returns the line windows the acronym check should
// scan. An empty scope list applies to the whole document.
// Otherwise the schema scope tree is walked and one line range is
// emitted per occurrence: a repeated scope (`repeat.max > 1` or
// unbounded) contributes a range for each matched heading, so a
// scope name like "Diagnosis" applied to two `## Diagnosis`
// sections scans both bodies for first-use acronyms.
func acronymRanges(f *lint.File, sch *Schema, scope []string, docFM map[string]any) []lineRange {
	if len(scope) == 0 {
		return []lineRange{{Start: 1, End: len(f.Lines) + 1}}
	}
	heads := ExtractDocHeadings(f)
	rootLevel := sch.EffectiveRootLevel()
	body := skipBelow(heads, rootLevel)

	matchSet := make(map[string]bool, len(scope))
	for _, s := range scope {
		matchSet[s] = true
	}

	var out []lineRange
	walkRanges(sch.Sections, body, rootLevel, 1, len(f.Lines)+1, docFM,
		func(sc Scope, headingText string, start, end int) {
			// walkRanges already skips preamble and slot scopes, so
			// any sc reaching here has a literal Matcher. Match the
			// scope-name allowlist against (a) the actual matched
			// heading text and (b) the schema label. The
			// heading-text match keeps disjunctive regexes like
			// `Symptoms|Indicators` aligned with intuitive
			// `acronyms.scope: ["Indicators"]` config; plan 156
			// dropped the scope-level `aliases:` field, so without
			// (a) users would have to mirror the regex body
			// verbatim. The `Matcher.Regex` branch is intentionally
			// not checked separately — the parser sets
			// `sc.Heading == sc.Matcher.Regex` for mapping-form
			// entries, so (b) already covers it.
			if matchSet[headingText] {
				out = append(out, lineRange{Start: start, End: end})
				return
			}
			if matchSet[sc.Heading] {
				out = append(out, lineRange{Start: start, End: end})
				return
			}
		})
	return out
}

// walkRanges pairs each non-slot, non-preamble scope with the line
// ranges covered by its matching doc headings, recursing into
// nested sections. Repeated scopes contribute one range per
// occurrence so per-scope checks fire on every matched section,
// not just the first. The walker mirrors the structural
// validator's claim semantics (regex matching) but is simpler —
// no out-of-order detection.
func walkRanges(
	scopes []Scope, heads []DocHeading,
	expectedLevel, parentStart, parentEnd int,
	docFM map[string]any,
	visit func(sc Scope, headingText string, start, end int),
) {
	claimed := make(map[int]bool, len(heads))
	for i, sc := range scopes {
		if sc.Preamble || isSlotMatcher(sc.Matcher) {
			continue
		}
		// ScopeRunIndices applies the structural validator's
		// run + yield semantics: contiguous matches only, with
		// broad-and-after-min yielding to later named scopes.
		for _, idx := range ScopeRunIndices(
			scopes, i, heads, expectedLevel, parentStart, parentEnd, claimed, docFM) {
			claimed[idx] = true
			start := heads[idx].Line
			end := nextSectionLine(heads, idx, heads[idx].Level, parentEnd)
			visit(sc, heads[idx].Text, start, end)
			if len(sc.Sections) > 0 {
				walkRanges(sc.Sections, heads, expectedLevel+1, start, end, docFM, visit)
			}
		}
	}
}

func nextSectionLine(heads []DocHeading, idx, level, parentEnd int) int {
	for j := idx + 1; j < len(heads); j++ {
		if heads[j].Level <= level {
			if heads[j].Line >= parentEnd {
				return parentEnd
			}
			return heads[j].Line
		}
	}
	return parentEnd
}

func checkAcronymsInRange(
	f *lint.File, rng lineRange, known map[string]bool,
	headingLines map[int]bool, mkDiag MakeDiag,
) []lint.Diagnostic {
	seen := map[string]bool{}
	var diags []lint.Diagnostic
	for ln := rng.Start; ln < rng.End && ln-1 < len(f.Lines); ln++ {
		if headingLines[ln] {
			continue
		}
		raw := string(f.Lines[ln-1])
		matches := acronymToken.FindAllStringIndex(raw, -1)
		for _, m := range matches {
			tok := raw[m[0]:m[1]]
			if known[tok] || seen[tok] {
				continue
			}
			seen[tok] = true
			if hasParenExpansion(raw, m[1]) {
				continue
			}
			diags = append(diags, mkDiag(f.Path, ln,
				fmt.Sprintf(
					"acronym %q used without parenthesised expansion on first use",
					tok)))
		}
	}
	return diags
}

// hasParenExpansion reports whether the text starting at offset
// includes a "(Some Words)" expansion. Any amount of intervening
// ASCII space (including none) between the acronym and the
// opening paren is tolerated — prose styles vary on this point
// and the rule is interested in whether an expansion is present,
// not in punctuation pedantry.
func hasParenExpansion(line string, offset int) bool {
	rest := line[offset:]
	rest = strings.TrimLeft(rest, " ")
	if !strings.HasPrefix(rest, "(") {
		return false
	}
	closeIdx := strings.IndexByte(rest, ')')
	if closeIdx < 2 {
		return false
	}
	inside := strings.TrimSpace(rest[1:closeIdx])
	return inside != ""
}
