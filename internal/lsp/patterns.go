package lsp

import (
	"github.com/jeduden/mdsmith/internal/rules"
)

type rulePattern struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Signal        string `json:"signal"`
	Fix           string `json:"fix"`
	ForDiagnostic bool   `json:"for-diagnostic"`
}

// handleRulePatterns serves the `mdsmith/rulePatterns` LSP request. It returns
// every rule with a non-null maintainability block in the same shape as
// `mdsmith help patterns -f json`. rules.ListRules reads an embedded FS that
// cannot fail at runtime, so any error degrades silently to an empty list —
// the same response shape as a workspace where every rule is `maintainability: null`.
func (s *Server) handleRulePatterns(msg *requestMessage) {
	all, _ := rules.ListRules()
	out := make([]rulePattern, 0)
	for _, r := range all {
		if r.Maintainability == nil {
			continue
		}
		out = append(out, rulePattern{
			ID:            r.ID,
			Name:          r.Name,
			Signal:        r.Maintainability.Signal,
			Fix:           r.Maintainability.Fix,
			ForDiagnostic: r.Maintainability.ForDiagnostic,
		})
	}
	_ = s.t.writeResponse(msg.ID, out)
}
