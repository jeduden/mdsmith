package engine

import (
	"github.com/jeduden/mdsmith/internal/checker"
	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
)

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
