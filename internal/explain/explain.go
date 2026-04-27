// Package explain attaches per-leaf rule provenance to lint
// diagnostics. It bridges internal/config (which computes a file's
// effective rule resolution) and internal/lint (which carries the
// optional Explanation payload on a Diagnostic), so the engine and
// fixer share one implementation rather than duplicating the loop.
package explain

import (
	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/lint"
)

// Attach populates Diagnostic.Explanation for each diag emitted at a
// file path using the rule's resolved per-leaf provenance.
//
// Diagnostics whose RuleName is not present in the file's effective
// rule config are left untouched: the explain trailer is best-effort
// and never invents provenance for rules that were never resolved.
func Attach(diags []lint.Diagnostic, cfg *config.Config, path string, fmKinds []string) {
	if len(diags) == 0 {
		return
	}
	res := config.ResolveFile(cfg, path, fmKinds)
	for i := range diags {
		rr, ok := res.Rules[diags[i].RuleName]
		if !ok {
			continue
		}
		leaves := make([]lint.ExplanationLeaf, 0, len(rr.Leaves))
		for _, l := range rr.Leaves {
			leaves = append(leaves, lint.ExplanationLeaf{
				Path: l.Path, Value: l.Value, Source: l.Source(),
			})
		}
		diags[i].Explanation = &lint.Explanation{
			Rule:   diags[i].RuleName,
			Leaves: leaves,
		}
	}
}
