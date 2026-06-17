package engine

import (
	"github.com/jeduden/mdsmith/internal/checker"
	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
)

// ConfigureRule applies settings from cfg to a clone of rl.
func ConfigureRule(rl rule.Rule, cfg config.RuleCfg) (rule.Rule, error) {
	return checker.ConfigureRule(rl, cfg)
}

// CheckRules runs rules against f and filters out any diagnostics whose
// line falls within a generated range recorded on f.
func CheckRules(f *lint.File, rules []rule.Rule, effective map[string]config.RuleCfg) ([]lint.Diagnostic, []error) {
	diags, errs := checker.CheckRules(f, rules, effective)
	if len(f.GeneratedRanges) == 0 {
		return diags, errs
	}
	return filterGeneratedDiags(diags, f.GeneratedRanges), errs
}

// filterGeneratedDiags returns a subset of diags with any diagnostic
// whose line falls within one of the generated ranges removed.
func filterGeneratedDiags(diags []lint.Diagnostic, ranges []lint.LineRange) []lint.Diagnostic {
	if len(ranges) == 0 {
		return diags
	}
	out := diags[:0:0]
	for _, d := range diags {
		inRange := false
		for _, r := range ranges {
			if r.Contains(d.Line) {
				inRange = true
				break
			}
		}
		if !inRange {
			out = append(out, d)
		}
	}
	return out
}

// checkRulesWithIntraFile delegates to checker.CheckRulesWithIntraFile.
// Kept as an unexported engine-local shim so runner.go can call it
// without changing its call sites.
func checkRulesWithIntraFile(
	f *lint.File,
	rules []rule.Rule,
	effective map[string]config.RuleCfg,
	skipSourceContext bool,
	intraFileCap int,
) ([]lint.Diagnostic, []error) {
	return checker.CheckRulesWithIntraFile(f, rules, effective, skipSourceContext, intraFileCap)
}
