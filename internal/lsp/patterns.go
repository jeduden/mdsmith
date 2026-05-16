package lsp

import (
	"fmt"

	"github.com/jeduden/mdsmith/internal/rules"
)

type rulePattern struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Signal        string `json:"signal"`
	Fix           string `json:"fix"`
	ForDiagnostic bool   `json:"for-diagnostic"`
}

func (s *Server) handleRulePatterns(msg *requestMessage) {
	all, err := rules.ListRules()
	if err != nil {
		_ = s.t.writeError(msg.ID, codeInternalError, fmt.Sprintf("listing rules: %v", err))
		return
	}
	out := make([]rulePattern, 0)
	for _, r := range all {
		if r.Maintainability == nil {
			continue
		}
		out = append(out, rulePattern{r.ID, r.Name, r.Maintainability.Signal, r.Maintainability.Fix, r.Maintainability.ForDiagnostic})
	}
	_ = s.t.writeResponse(msg.ID, out)
}
