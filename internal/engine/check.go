package engine

import (
	"github.com/jeduden/mdsmith/internal/checker"
	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
)

// ConfigureRule clones a rule and applies settings from cfg if the rule
// implements Configurable and cfg has settings. Returns the configured
// rule (or the original if no settings apply) and any error from
// ApplySettings.
//
// Deprecated: call checker.ConfigureRule directly. This wrapper exists
// so internal/engine callers need not be updated all at once.
func ConfigureRule(rl rule.Rule, cfg config.RuleCfg) (rule.Rule, error) {
	return checker.ConfigureRule(rl, cfg)
}

// CheckRules runs all enabled rules against f, cloning and applying
// settings for Configurable rules. It adjusts diagnostics using
// f.AdjustDiagnostics and returns the collected diagnostics and any
// settings-application errors. Source context is populated; callers
// that discard SourceLines should use checker.CheckRulesWithIntraFile
// with skipSourceContext=true to avoid that allocation.
//
// Deprecated: call checker.CheckRules directly. This wrapper exists
// so external callers need not be updated all at once.
func CheckRules(f *lint.File, rules []rule.Rule, effective map[string]config.RuleCfg) ([]lint.Diagnostic, []error) {
	return checker.CheckRules(f, rules, effective)
}

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
