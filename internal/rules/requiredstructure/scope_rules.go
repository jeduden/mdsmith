package requiredstructure

import (
	"fmt"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/jeduden/mdsmith/internal/schema"
)

// applyScopeRules walks the schema tree to find scopes that declare
// per-scope rule overrides and re-runs each named rule against the
// document, filtering diagnostics to the scope's line range. This is
// the entry point for plan 146's per-scope rule-config feature.
//
// The implementation is intentionally minimal: the override applies
// on top of the rule's defaults rather than the file's full
// effective config. The fixture for this feature (same prose in two
// sections, one with a stricter override) is met by this baseline;
// the full file→scope merge is a follow-up.
func (r *Rule) applyScopeRules(f *lint.File, sch *schema.Schema) []lint.Diagnostic {
	if sch == nil {
		return nil
	}
	heads := schema.ExtractDocHeadings(f)
	rootLevel := sch.EffectiveRootLevel()
	body := skipBelow(heads, rootLevel)
	var diags []lint.Diagnostic
	claimed := make(map[int]bool, len(body))
	walkScopes(sch.Sections, body, rootLevel, 1, len(f.Lines)+1, claimed,
		func(sc schema.Scope, startLine, endLine int) {
			if len(sc.Rules) == 0 {
				return
			}
			diags = append(diags, runScopeRules(f, sc, startLine, endLine)...)
		})
	return diags
}

// skipBelow strips heading entries whose level is shallower than
// rootLevel so the walker starts at the first heading the schema
// actually covers.
func skipBelow(heads []schema.DocHeading, rootLevel int) []schema.DocHeading {
	for i, h := range heads {
		if h.Level >= rootLevel {
			return heads[i:]
		}
	}
	return nil
}

// walkScopes pairs each scope with a doc heading and invokes visit
// with the inclusive 1-based start line and the exclusive end line
// of the scope's content range. The walker mirrors the validator's
// claim logic: a doc heading that appears out of order still
// matches its scope so per-scope rule overrides apply even when the
// surrounding document is currently invalid.
//
// claimed tracks heading indices already paired with a scope. The
// boundary parentEnd is the exclusive end line of the enclosing
// section (or fileEnd at the root) so a nested walk does not match
// headings that belong to an ancestor.
func walkScopes(
	scopes []schema.Scope, heads []schema.DocHeading,
	expectedLevel, parentStart, parentEnd int,
	claimed map[int]bool,
	visit func(sc schema.Scope, startLine, endLine int),
) {
	for _, sc := range scopes {
		if sc.Wildcard {
			continue
		}
		matched := findMatchingHead(sc, heads, expectedLevel, parentStart, parentEnd, claimed)
		if matched < 0 {
			continue
		}
		claimed[matched] = true
		dh := heads[matched]
		end := scopeEndLine(heads, matched, expectedLevel, parentEnd)
		visit(sc, dh.Line, end)
		if len(sc.Sections) > 0 {
			walkScopes(sc.Sections, heads,
				expectedLevel+1, dh.Line, end, claimed, visit)
		}
	}
}

// findMatchingHead returns the earliest unclaimed heading index in
// heads whose level matches expectedLevel and whose text matches
// sc, restricted to the [parentStart, parentEnd) line window. When
// no in-window heading at the expected level matches, it falls back
// to an in-window heading at any level — the same level-mismatch
// case the validator's matchScope claims. The fallback stays inside
// the parent window so the walker never pairs a scope with a
// heading the validator could not have claimed.
func findMatchingHead(
	sc schema.Scope, heads []schema.DocHeading,
	expectedLevel, parentStart, parentEnd int,
	claimed map[int]bool,
) int {
	if idx := scanHeads(sc, heads, parentStart, parentEnd, claimed, expectedLevel); idx >= 0 {
		return idx
	}
	return scanHeads(sc, heads, parentStart, parentEnd, claimed, -1)
}

// scanHeads returns the first unclaimed heading in heads whose line
// is in [parentStart, parentEnd) and whose level equals
// requireLevel (or any level when requireLevel < 0), and whose text
// matches sc.
func scanHeads(
	sc schema.Scope, heads []schema.DocHeading,
	parentStart, parentEnd int, claimed map[int]bool,
	requireLevel int,
) int {
	for j, dh := range heads {
		if claimed[j] {
			continue
		}
		if dh.Line < parentStart || dh.Line >= parentEnd {
			continue
		}
		if requireLevel >= 0 && dh.Level != requireLevel {
			continue
		}
		if schema.MatchesHeading(sc, dh) {
			return j
		}
	}
	return -1
}

// scopeEndLine returns the exclusive end-line of the section
// beginning at heads[matched]. The section ends at the first
// subsequent heading whose level is <= expectedLevel and whose line
// falls inside the parent window, or at parentEnd when no such
// heading follows.
func scopeEndLine(
	heads []schema.DocHeading, matched, expectedLevel, parentEnd int,
) int {
	for j := matched + 1; j < len(heads); j++ {
		if heads[j].Line >= parentEnd {
			break
		}
		if heads[j].Level <= expectedLevel {
			return heads[j].Line
		}
	}
	return parentEnd
}

// runScopeRules executes each rule named in sc.Rules and returns
// diagnostics that fall within the scope's line range. Each rule is
// cloned and configured with its defaults deep-merged with the
// scope's override.
//
// Misconfigurations (unknown rule name, ApplySettings error) surface
// as MDS020 diagnostics at the scope's heading line so users see the
// problem instead of the override silently no-op'ing.
func runScopeRules(
	f *lint.File, sc schema.Scope, startLine, endLine int,
) []lint.Diagnostic {
	var diags []lint.Diagnostic
	for name, override := range sc.Rules {
		base := rule.ByName(name)
		if base == nil {
			diags = append(diags, makeDiag(f.Path, startLine,
				fmt.Sprintf(
					"scope rule override for unknown rule %q", name)))
			continue
		}
		configured := rule.CloneRule(base)
		if c, ok := configured.(rule.Configurable); ok {
			if err := c.ApplySettings(override); err != nil {
				diags = append(diags, makeDiag(f.Path, startLine,
					fmt.Sprintf(
						"scope rule override for %q is invalid: %v",
						name, err)))
				continue
			}
		}
		for _, d := range configured.Check(f) {
			if d.Line >= startLine && d.Line < endLine {
				diags = append(diags, d)
			}
		}
	}
	return diags
}
