// Package rulelayer records the lazy-parse layer each lint rule resolves
// to, derived from the rule-walk audit manifest (plan 2606022126). The
// engine consults it to decide whether a run can skip the goldmark parse:
// the parse is skippable only when every enabled rule is a Layer 0 rule —
// one that reads f.Lines and the block-level projections but never
// navigates f.AST.
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
	// Layer0 means the rule reads only f.Lines and the block-level
	// projections — it can run with a nil AST. These are the manifest's
	// "A-no-skipping" rules.
	Layer0
	// LayerAST means the rule needs the goldmark AST (the manifest's
	// "hybrid", "ast-required", and inconclusive categories). The engine
	// must parse when any enabled rule resolves here.
	LayerAST
)

// auditEntry is the subset of the rule-walk manifest the layer mapping
// needs: the rule id and its AST-dependence category.
type auditEntry struct {
	ID       string `json:"id"`
	Category string `json:"category"`
}

// layerByID maps each rule id to its resolved layer, built once from the
// embedded manifest at package init.
var layerByID = buildLayerMap()

// buildLayerMap decodes the embedded manifest into the id→layer table.
// "A-no-skipping" rules are Layer 0; every other category needs the AST.
// A decode failure is a build-time contract violation (the embedded JSON
// is checked in), so it panics rather than silently degrading.
func buildLayerMap() map[string]Layer {
	var entries []auditEntry
	if err := json.Unmarshal(auditJSON, &entries); err != nil {
		panic(fmt.Sprintf("rulelayer: decoding embedded audit manifest: %v", err))
	}
	m := make(map[string]Layer, len(entries))
	for _, e := range entries {
		if e.Category == "A-no-skipping" {
			m[e.ID] = Layer0
		} else {
			m[e.ID] = LayerAST
		}
	}
	return m
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
