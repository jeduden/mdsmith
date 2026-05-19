package engine

import (
	"fmt"

	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/yuin/goldmark/ast"
)

// ConfigureRule clones a rule and applies settings from cfg if the rule
// implements Configurable and cfg has settings. Returns the configured
// rule (or the original if no settings apply) and any error from
// ApplySettings.
func ConfigureRule(rl rule.Rule, cfg config.RuleCfg) (rule.Rule, error) {
	if cfg.Settings == nil {
		return rl, nil
	}
	if _, ok := rl.(rule.Configurable); !ok {
		return rl, nil
	}
	clone := rule.CloneRule(rl)
	if c, ok := clone.(rule.Configurable); ok {
		if err := c.ApplySettings(cfg.Settings); err != nil {
			return nil, fmt.Errorf("applying settings for %s: %w", rl.Name(), err)
		}
	}
	return clone, nil
}

// CheckRules runs all enabled rules against f, cloning and applying
// settings for Configurable rules. It adjusts diagnostics using
// f.AdjustDiagnostics and returns the collected diagnostics and any
// settings-application errors. Source context is populated; callers
// that discard SourceLines should use checkRules with
// skipSourceContext=true to avoid that allocation.
func CheckRules(f *lint.File, rules []rule.Rule, effective map[string]config.RuleCfg) ([]lint.Diagnostic, []error) {
	return checkRules(f, rules, effective, false)
}

// checkRules is the core CheckRules implementation. skipSourceContext
// suppresses populateSourceContext, whose per-diagnostic string copies
// and []string windows were the single largest object count on the
// check gate (~315 MB / 3.8M objects, plan 175 profiling) and are
// unused when the caller never renders SourceLines (the benchmark, and
// machine output that omits them).
func checkRules(
	f *lint.File,
	rules []rule.Rule,
	effective map[string]config.RuleCfg,
	skipSourceContext bool,
) ([]lint.Diagnostic, []error) {
	var diags []lint.Diagnostic
	var errs []error

	// Each enabled rule contributes one contiguous diagnostic group,
	// kept in rules order. Rules implementing rule.NodeChecker have
	// their group filled by a single shared ast.Walk instead of each
	// re-walking the whole tree (goldmark walkHelper was ~44%
	// cumulative). Because every group is still placed in rules order
	// and a NodeChecker's bucket is fed the identical pre-order node
	// stream its own Check (rule.WalkNodes) would use, the combined
	// slice is byte-identical to running every rule's Check
	// sequentially — order-independent consumers see no change.
	type ruleSlot struct {
		nc    rule.NodeChecker
		diags []lint.Diagnostic
	}
	var slots []*ruleSlot
	var nodeCheckers []*ruleSlot

	for _, rl := range rules {
		cfg, ok := effective[rl.Name()]
		if !ok || !cfg.Enabled {
			continue
		}

		checkRule, err := ConfigureRule(rl, cfg)
		if err != nil {
			errs = append(errs, err)
			continue
		}

		if nc, ok := checkRule.(rule.NodeChecker); ok {
			s := &ruleSlot{nc: nc}
			slots = append(slots, s)
			nodeCheckers = append(nodeCheckers, s)
			continue
		}
		slots = append(slots, &ruleSlot{diags: checkRule.Check(f)})
	}

	if len(nodeCheckers) > 0 {
		_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
			for _, s := range nodeCheckers {
				s.diags = append(s.diags, s.nc.CheckNode(n, entering, f)...)
			}
			return ast.WalkContinue, nil
		})
	}

	for _, s := range slots {
		diags = append(diags, s.diags...)
	}

	diags = filterGeneratedDiags(diags, f.GeneratedRanges)
	f.AdjustDiagnostics(diags)
	if !skipSourceContext {
		populateSourceContext(f, diags, 2)
	}
	return diags, errs
}

// filterGeneratedDiags removes diagnostics whose line falls within any
// of the generated section ranges. Called before AdjustDiagnostics, so
// lines are still in post-front-matter coordinates matching the ranges.
func filterGeneratedDiags(diags []lint.Diagnostic, ranges []lint.LineRange) []lint.Diagnostic {
	if len(ranges) == 0 {
		return diags
	}
	out := diags[:0:len(diags)]
	for _, d := range diags {
		keep := true
		for _, r := range ranges {
			if r.Contains(d.Line) {
				keep = false
				break
			}
		}
		if keep {
			out = append(out, d)
		}
	}
	return out
}

// populateSourceContext fills each diagnostic's SourceLines and
// SourceStartLine with surrounding context from f.Lines.
func populateSourceContext(f *lint.File, diags []lint.Diagnostic, context int) {
	// bytes.Split produces an empty trailing element when source ends
	// with a newline. Exclude it so context windows don't include a
	// phantom empty line.
	numLines := len(f.Lines)
	if numLines > 0 && len(f.Lines[numLines-1]) == 0 {
		numLines--
	}

	for i := range diags {
		lineIdx := diags[i].Line - f.LineOffset - 1 // 0-based into f.Lines
		if lineIdx < 0 || lineIdx >= numLines {
			continue
		}
		start := max(0, lineIdx-context)
		end := min(numLines, lineIdx+context+1)
		lines := make([]string, end-start)
		for j := start; j < end; j++ {
			lines[j-start] = string(f.Lines[j])
		}
		diags[i].SourceLines = lines
		diags[i].SourceStartLine = start + f.LineOffset + 1
	}
}
