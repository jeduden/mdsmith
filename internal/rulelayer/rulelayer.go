// Package rulelayer records the lazy-parse layer each lint rule resolves
// to, derived from the rule-walk audit manifest (plan 2606022126). The
// engine consults it to decide whether a run can skip the goldmark parse:
// the parse is skippable only when every enabled rule is a Layer 0 rule —
// one that never *requires* f.AST because it has a nil-AST path served by
// the Layer-0 line/block scan and the Layer-1 inline index.
//
// The manifest is embedded from a checked-in copy of
// internal/integration/testdata/rule_walk_audit.json; a contract test
// (rulelayer_test.go) keeps the two byte-identical so the audit gate and
// the engine gate can never disagree.
package rulelayer

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

//go:embed rule_walk_audit.json
var auditJSON []byte

// Layer is a rule's resolved lazy-parse layer.
type Layer int

const (
	// LayerUnknown is the zero value for a rule absent from the manifest.
	// The engine treats it conservatively as AST-requiring.
	LayerUnknown Layer = iota
	// Layer0 means the rule can run with a nil AST: it reads f.Lines, the
	// block-level projections, and the Layer-1 inline index, never requiring
	// the goldmark tree. These are the manifest's "A-no-skipping" rules plus
	// the nil-AST-safe "B-prose-only" rules — rules that read code-block,
	// code-span, or inline-HTML content the validated projections reproduce.
	Layer0
	// LayerAST means the rule needs the goldmark AST (the manifest's
	// "hybrid", "ast-required", and inconclusive categories). The engine
	// must parse when any enabled rule resolves here.
	LayerAST
)

// auditEntry is the subset of the rule-walk manifest the layer mapping
// needs: the rule id, its AST-dependence category, and the probe's
// nil-AST-safety signal (which gates the B-prose-only promotion).
type auditEntry struct {
	ID         string `json:"id"`
	Category   string `json:"category"`
	NilASTSafe bool   `json:"nil_ast_safe"`
}

// layerByID maps each rule id to its resolved layer, built once from the
// embedded manifest at package init.
var layerByID = buildLayerMap()

// astProjectionConsumers lists rules the audit manifest would otherwise
// promote to Layer 0 ("A-no-skipping" or nil-AST-safe "B-prose-only") — they
// never crash with a nil AST — but which still read an AST-derived projection
// that Layer 0 / Layer 1 does not reproduce, so their output silently diverges
// on a parse-skipped File. The audit's probe measured crash-safety, not output
// equivalence.
//
// MDS047 (ambiguous-emphasis) and MDS054 (no-undefined-reference-labels)
// formerly sat here: both consume the inline code-span ranges
// (CodeSpanContentRanges / CodeSpanLiteralRanges), which returned empty
// without a parse and caused false positives inside backtick spans. Those
// ranges are now served on the nil-AST path from the shared run-grouped
// inline parse (lint.InlineBlocks, via collectCodeSpanRangesFromInlineBlocks
// in internal/lint/codespans.go), byte-identical to the AST walk by
// construction and gated across the parse-skip-eligible corpus by the
// equivalence harness — so both rules resolve to Layer 0 straight from their
// A-no-skipping audit category and no override remains.
//
// The map stays as the seam for any future projection-only consumer the
// audit marks A-no-skipping but Layer N does not yet back. It is empty
// today.
var astProjectionConsumers = map[string]bool{}

// knownNilASTSafe lists rules confirmed nil-AST-safe by code inspection but
// whose walk-audit probe cannot fire them, so the audit classifies them
// inconclusive-not-fired rather than A-no-skipping. These are cross-file
// rules that need a multi-file RunCache the bad-fixture probe does not wire
// up, so the probe never emits a diagnostic and the nil-AST/code-block
// signals prove nothing. Adding a rule here is a manual commitment that its
// Check never dereferences f.AST — keep it limited to rules whose manifest
// entry has reads_file_ast: false (enforced by rulelayer_test.go).
var knownNilASTSafe = map[string]bool{
	"MDS069": true, // unique-frontmatter: reads f.FrontMatter + RunCache, never f.AST
}

// buildLayerMap decodes the embedded manifest into the id→layer table.
// "A-no-skipping" and nil-AST-safe "B-prose-only" rules are Layer 0 unless
// they appear in astProjectionConsumers (which need an AST-only projection
// Layer N does not back); a rule in knownNilASTSafe is promoted from another
// category; every other category needs the AST. A decode failure is a
// build-time contract violation (the embedded JSON is checked in), so it
// panics rather than silently degrading.
func buildLayerMap() map[string]Layer {
	return buildLayerMapFrom(auditJSON)
}

// buildLayerMapFrom decodes manifest JSON into the id→layer table. It is the
// testable core of buildLayerMap: the package-level builder feeds it the
// embedded manifest, and a unit test feeds it malformed bytes to drive the
// decode-failure panic. A decode failure is a build-time contract violation
// (the embedded JSON is checked in), so it panics rather than degrading.
func buildLayerMapFrom(manifest []byte) map[string]Layer {
	var entries []auditEntry
	if err := json.Unmarshal(manifest, &entries); err != nil {
		panic(fmt.Sprintf("rulelayer: decoding embedded audit manifest: %v", err))
	}
	m := make(map[string]Layer, len(entries))
	for _, e := range entries {
		if !astProjectionConsumers[e.ID] && nilASTBackable(e) {
			m[e.ID] = Layer0
		} else {
			m[e.ID] = LayerAST
		}
	}
	return m
}

// nilASTBackable reports whether a manifest entry's rule can lint on the
// parse-skipped (nil-AST) File, before the astProjectionConsumers veto the
// caller applies. Three classes qualify:
//
//   - knownNilASTSafe: a manual override for a rule the bad-fixture probe
//     cannot fire, confirmed by inspection to never read f.AST
//     (reads_file_ast: false, enforced by rulelayer_test.go).
//   - "A-no-skipping": the probe fired the rule, the nil-AST run neither
//     panicked nor diverged from the AST run, and scrambling code bytes did
//     not change its output — it reads no code content, so the Layer-0 /
//     Layer-1 projections back it trivially.
//   - "B-prose-only" with nil_ast_safe: the probe fired the rule and the
//     nil-AST run matched the AST run, but the rule is code-content-sensitive
//     (scrambling code bytes changed its output). It reads code-block bodies,
//     code spans, or inline HTML — content the validated ClassifyLines and
//     Layer-1 inline projections reproduce byte-identically on the nil-AST
//     File (the parse-skip-on-code measurement, plan 2606171258, plus each
//     rule's TestCheck_NilASTMatchesAST guard). The code guard that once
//     forced these rules to parse is gone, so code-sensitivity alone no
//     longer requires the AST. The nil_ast_safe gate is belt-and-braces: the
//     B-prose-only category already implies it (a divergent rule lands in
//     "hybrid"), but reading the signal keeps the promotion honest if the
//     manifest is ever hand-edited, and TestBProseOnlyRulesAreNilASTSafe pins
//     the live invariant.
func nilASTBackable(e auditEntry) bool {
	if knownNilASTSafe[e.ID] {
		return true
	}
	if e.Category == "A-no-skipping" {
		return true
	}
	return e.Category == "B-prose-only" && e.NilASTSafe
}

// Of returns the resolved layer for a rule id, or LayerUnknown when the id
// is absent from the manifest. Callers that gate the parse skip treat
// LayerUnknown as AST-requiring (see IsLayer0).
func Of(ruleID string) Layer {
	return layerByID[ruleID]
}

// IsLayer0 reports whether a rule id is a Layer 0 rule — one the engine may
// run without parsing the AST. An unknown id is conservatively not Layer 0,
// so a rule the manifest does not cover never causes the parse to be
// skipped.
func IsLayer0(ruleID string) bool {
	return layerByID[ruleID] == Layer0
}
