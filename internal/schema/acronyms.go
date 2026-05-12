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
func ValidateAcronyms(
	f *lint.File, sch *Schema, mkDiag MakeDiag,
) []lint.Diagnostic {
	if sch == nil || sch.Acronyms == nil {
		return nil
	}
	a := sch.Acronyms
	known := buildKnownSet(a.KnownSafe)

	ranges := acronymRanges(f, sch, a.Scope)
	var diags []lint.Diagnostic
	for _, rng := range ranges {
		diags = append(diags, checkAcronymsInRange(f, rng, known, mkDiag)...)
	}
	return diags
}

func buildKnownSet(list []string) map[string]bool {
	out := make(map[string]bool, len(list))
	for _, s := range list {
		out[s] = true
	}
	return out
}

// lineRange identifies an inclusive 1-based line window for an
// acronym scope pass.
type lineRange struct {
	Start int
	End   int // exclusive
}

// acronymRanges returns the line windows the acronym check should
// scan. Empty scope applies to the whole document; otherwise the
// schema scope tree is walked and every matching scope's range is
// included.
func acronymRanges(f *lint.File, sch *Schema, scope []string) []lineRange {
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
	walkRanges(sch.Sections, body, rootLevel, 1, len(f.Lines)+1,
		func(sc Scope, start, end int) {
			if sc.Heading == "" {
				return
			}
			if matchSet[sc.Heading] {
				out = append(out, lineRange{Start: start, End: end})
				return
			}
			for _, a := range sc.Aliases {
				if matchSet[a] {
					out = append(out, lineRange{Start: start, End: end})
					return
				}
			}
		})
	return out
}

// walkRanges pairs each non-wildcard, non-preamble scope with the
// line range covered by its first matching doc heading, recursing
// into nested sections. It mirrors the validator's claim semantics
// (text-equality matching, aliases honoured) but is simpler — no
// out-of-order detection — because per-scope acronym detection
// applies wherever a section appears.
func walkRanges(
	scopes []Scope, heads []DocHeading,
	expectedLevel, parentStart, parentEnd int,
	visit func(sc Scope, start, end int),
) {
	for _, sc := range scopes {
		if sc.Wildcard || sc.Preamble {
			continue
		}
		idx := findHead(sc, heads, expectedLevel, parentStart, parentEnd)
		if idx < 0 {
			continue
		}
		start := heads[idx].Line
		end := nextSectionLine(heads, idx, heads[idx].Level, parentEnd)
		visit(sc, start, end)
		if len(sc.Sections) > 0 {
			walkRanges(sc.Sections, heads, expectedLevel+1, start, end, visit)
		}
	}
}

func findHead(
	sc Scope, heads []DocHeading, expectedLevel, parentStart, parentEnd int,
) int {
	for i, h := range heads {
		if h.Line < parentStart || h.Line >= parentEnd {
			continue
		}
		if h.Level != expectedLevel {
			continue
		}
		if scopeMatchesHeading(sc, h) {
			return i
		}
	}
	return -1
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
	f *lint.File, rng lineRange, known map[string]bool, mkDiag MakeDiag,
) []lint.Diagnostic {
	seen := map[string]bool{}
	var diags []lint.Diagnostic
	for ln := rng.Start; ln < rng.End && ln-1 < len(f.Lines); ln++ {
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

// hasParenExpansion reports whether the text starting at offset look
// includes a "(Some Words)" expansion. We accept a single space
// between the acronym and the opening paren.
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
