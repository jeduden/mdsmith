package requiredstructure

import (
	"fmt"
	"sort"

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
func (r *Rule) applyScopeRules(
	f *lint.File, sch *schema.Schema, docFM map[string]any,
) []lint.Diagnostic {
	if sch == nil {
		return nil
	}
	heads := schema.ExtractDocHeadings(f)
	rootLevel := sch.EffectiveRootLevel()
	body := skipBelow(heads, rootLevel)
	var diags []lint.Diagnostic
	claimed := make(map[int]bool, len(body))
	walkScopes(sch.Sections, body, rootLevel, 1, len(f.Lines)+1, claimed, docFM,
		func(sc schema.Scope, startLine, endLine int) {
			if len(sc.Rules) == 0 {
				return
			}
			diags = append(diags, runScopeRules(f, sc, startLine, endLine)...)
		})
	return diags
}

// skipBelow returns heads with every entry shallower than
// rootLevel filtered out. A stray higher-level heading (e.g. a
// second H1 mid-document) must not break the walker the same way
// it would have terminated the validator: the walker scans by
// level + window, and shallow leftovers would otherwise stay in
// the slice and confuse scopeEndLine.
func skipBelow(heads []schema.DocHeading, rootLevel int) []schema.DocHeading {
	out := make([]schema.DocHeading, 0, len(heads))
	for _, h := range heads {
		if h.Level >= rootLevel {
			out = append(out, h)
		}
	}
	return out
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
	claimed map[int]bool, docFM map[string]any,
	visit func(sc schema.Scope, startLine, endLine int),
) {
	for i, sc := range scopes {
		if sc.Preamble {
			// Preamble range: [parentStart, first heading at this
			// level in the window). Empty if the very first doc
			// node is already a heading; visit() with an empty
			// range is still useful — `rules:` on a preamble of an
			// empty preamble simply has nothing to check.
			end := firstHeadingLine(heads, expectedLevel, parentStart, parentEnd)
			visit(sc, parentStart, end)
			continue
		}
		if isSlotScope(sc) {
			continue
		}
		// schema.ScopeRunIndices applies the structural validator's
		// run + yield semantics: contiguous matches only, with
		// broad-and-after-min yielding to later named scopes.
		for _, matched := range schema.ScopeRunIndices(
			scopes, i, heads, expectedLevel, parentStart, parentEnd, claimed, docFM) {
			dh := heads[matched]
			claimed[matched] = true
			// The section's end boundary follows the doc heading's
			// real level, not the schema's expectedLevel. When the
			// two differ (level-mismatch fallback), basing the end
			// on expectedLevel would let sibling sections at the
			// doc's level leak into the scope (deeper doc level)
			// or truncate the section short (shallower doc level).
			end := scopeEndLine(heads, matched, dh.Level, parentEnd)
			visit(sc, dh.Line, end)
			if len(sc.Sections) > 0 {
				walkScopes(sc.Sections, heads,
					expectedLevel+1, dh.Line, end, claimed, docFM, visit)
			}
		}
	}
}

// scopeEndLine returns the exclusive end-line of the section
// beginning at heads[matched]. The section ends at the first
// subsequent heading whose level is <= boundaryLevel and whose line
// falls inside the parent window, or at parentEnd when no such
// heading follows. boundaryLevel is normally the matched heading's
// own level so the section range tracks the document's nesting.
func scopeEndLine(
	heads []schema.DocHeading, matched, boundaryLevel, parentEnd int,
) int {
	for j := matched + 1; j < len(heads); j++ {
		if heads[j].Line >= parentEnd {
			break
		}
		if heads[j].Level <= boundaryLevel {
			return heads[j].Line
		}
	}
	return parentEnd
}

// firstHeadingLine returns the line of the first heading at level
// expectedLevel inside the parent window, or parentEnd when no
// such heading exists. Used to size the preamble range: from the
// parent's start line up to (but not including) the first listed
// heading at this level.
func firstHeadingLine(
	heads []schema.DocHeading, expectedLevel, parentStart, parentEnd int,
) int {
	for _, h := range heads {
		if h.Line < parentStart || h.Line >= parentEnd {
			continue
		}
		if h.Level == expectedLevel {
			return h.Line
		}
	}
	return parentEnd
}

// isSlotScope reports whether sc is the wildcard-slot shape (plan
// 156: `regex: '.+', repeat: { min: 0 }`). The per-scope walker
// skips slots because they have no fixed identity to attach rule
// overrides to.
func isSlotScope(sc schema.Scope) bool {
	m := sc.Matcher
	if m == nil {
		return false
	}
	if m.Regex != ".+" {
		return false
	}
	if !m.Repeat.Set || m.Repeat.Min != 0 || m.Repeat.Max != 0 {
		return false
	}
	return true
}

// runScopeRules executes each rule named in sc.Rules and returns
// diagnostics that fall within the scope's line range. Each rule is
// cloned with its DefaultSettings and then has the scope's override
// applied via ApplySettings — keys touched by the override replace
// the defaults wholesale; nested maps and list merge modes are NOT
// honoured the way config-layer merging does. Implementing a true
// config-style deep-merge for scope overrides is part of the
// follow-up tracked on plan 146.
//
// Rule names are sorted before execution so emitted diagnostics
// land in a stable order regardless of map iteration randomness;
// fixture assertions in the integration harness compare by index.
//
// Misconfigurations (unknown rule name, ApplySettings error) surface
// as MDS020 diagnostics at the scope's heading line so users see the
// problem instead of the override silently no-op'ing.
func runScopeRules(
	f *lint.File, sc schema.Scope, startLine, endLine int,
) []lint.Diagnostic {
	names := make([]string, 0, len(sc.Rules))
	for name := range sc.Rules {
		names = append(names, name)
	}
	sort.Strings(names)
	var diags []lint.Diagnostic
	for _, name := range names {
		override := sc.Rules[name]
		base := rule.ByName(name)
		if base == nil {
			diags = append(diags, makeDiag(f.Path, startLine,
				fmt.Sprintf(
					"scope rule override for unknown rule %q", name)))
			continue
		}
		configured := rule.CloneRule(base)
		c, ok := configured.(rule.Configurable)
		if !ok {
			if len(override) > 0 {
				diags = append(diags, makeDiag(f.Path, startLine,
					fmt.Sprintf(
						"scope rule override for %q has no effect: "+
							"the rule does not accept settings", name)))
				continue
			}
			// Empty override on a non-configurable rule is a
			// request to re-run the rule with its defaults inside
			// this scope; fall through to the Check call.
		} else if err := c.ApplySettings(override); err != nil {
			diags = append(diags, makeDiag(f.Path, startLine,
				fmt.Sprintf(
					"scope rule override for %q is invalid: %v",
					name, err)))
			continue
		}
		for _, d := range configured.Check(f) {
			if d.Line >= startLine && d.Line < endLine {
				diags = append(diags, d)
			}
		}
	}
	return diags
}
