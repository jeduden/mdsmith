package engine

import (
	"github.com/jeduden/mdsmith/internal/checker"
	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
)

// checkRules is an engine-internal shim that delegates to
// checker.CheckRulesWithIntraFile with skipSourceContext and
// intraFileCap=1. Used by engine tests and runner.go call sites that
// previously called this unexported function directly.
func checkRules(
	f *lint.File,
	rules []rule.Rule,
	effective map[string]config.RuleCfg,
	skipSourceContext bool,
) ([]lint.Diagnostic, []error) {
	return checker.CheckRulesWithIntraFile(f, rules, effective, skipSourceContext, 1)
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

// filterGeneratedDiags delegates to checker.FilterGeneratedDiags.
// Kept for engine-internal test callers that test the filter directly.
func filterGeneratedDiags(diags []lint.Diagnostic, ranges []lint.LineRange) []lint.Diagnostic {
	return checker.FilterGeneratedDiags(diags, ranges)
}
