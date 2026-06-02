// Package checker provides the shared rule-checking primitives used by
// both internal/engine (the full linting runner) and internal/fix (the
// auto-fix pipeline). It sits below both callers in the dependency
// graph so neither needs to import the other.
package checker

import (
	"fmt"
	"sync"

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
// that discard SourceLines should use CheckRulesWithIntraFile with
// skipSourceContext=true to avoid that allocation.
func CheckRules(f *lint.File, rules []rule.Rule, effective map[string]config.RuleCfg) ([]lint.Diagnostic, []error) {
	return CheckRulesWithIntraFile(f, rules, effective, false, 1)
}

// CheckRulesWithIntraFile is the core implementation that accepts an
// explicit skip-source-context flag and intra-file concurrency cap.
// The lintFile path in engine resolves the cap once per Run (via
// resolveIntraFileWorkers) and passes it in here so the per-file
// workers do not each query GOMAXPROCS.
func CheckRulesWithIntraFile(
	f *lint.File,
	rules []rule.Rule,
	effective map[string]config.RuleCfg,
	skipSourceContext bool,
	intraFileCap int,
) ([]lint.Diagnostic, []error) {
	slots, nodeCheckers, errs := classifyRules(rules, effective)

	// Run non-NodeChecker rules. With cap=1 the loop stays serial and
	// matches the legacy code path byte-for-byte. With cap>1, slots
	// run concurrently into their own buckets; rules order is
	// preserved because the concatenation step reads `slots` in
	// index order at the end.
	runNonNodeCheckers(f, slots, intraFileCap)

	// The shared walk runs after the goroutine workers join, so its
	// node visitor and the rules running inside it never race for
	// any per-rule state. NodeCheckers stay internally serial: one
	// goroutine, one walk, one rule per node — fast enough that
	// splitting per rule would lose the cache locality the multiplex
	// just won.
	if len(nodeCheckers) > 0 {
		_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
			for _, s := range nodeCheckers {
				s.diags = append(s.diags, s.nc.CheckNode(n, entering, f)...)
			}
			return ast.WalkContinue, nil
		})
	}

	var diags []lint.Diagnostic
	for _, s := range slots {
		diags = append(diags, s.diags...)
	}

	diags = FilterGeneratedDiags(diags, f.GeneratedRanges)
	f.AdjustDiagnostics(diags)
	if !skipSourceContext {
		PopulateSourceContext(f, diags, 2)
	}
	return diags, errs
}

// ruleSlot is one rule's diagnostic bucket. NodeCheckers append to
// it from the shared walk; non-NodeCheckers fill it once via Check.
// Slots are kept in rules order so the final concatenation reproduces
// the sequential output exactly. Configure-failed rules never get a
// slot — they short-circuit in classifyRules with an entry in errs.
type ruleSlot struct {
	nc    rule.NodeChecker
	check rule.Rule // non-nil for non-NodeChecker slots
	diags []lint.Diagnostic
}

// classifyRules walks the rules list once, configures each enabled
// rule via ConfigureRule (which clones and applies settings only
// when cfg.Settings is non-nil and the rule is Configurable —
// otherwise the worker's existing clone is reused as-is), and splits
// the result into per-rule slots. The slots slice keeps every
// enabled rule in input order (so the final concatenation is
// deterministic); the nodeCheckers slice is the subset whose group
// will be filled by the shared walk.
func classifyRules(
	rules []rule.Rule, effective map[string]config.RuleCfg,
) (slots []ruleSlot, nodeCheckers []*ruleSlot, errs []error) {
	// Pre-size at the registered-rule count. In production all but a
	// handful of rules are enabled by default. Allocating slots as a
	// value slice (rather than a slice of `*ruleSlot`) collapses the
	// 50+ per-file pointer allocations the previous shape paid into
	// one backing-array allocation. nodeCheckers stays a pointer
	// slice but references entries by `&slots[i]`, which is stable
	// because the slots cap was pre-set to len(rules) — no append
	// grows the backing, so the index-derived pointers do not
	// dangle.
	slots = make([]ruleSlot, 0, len(rules))
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
			slots = append(slots, ruleSlot{nc: nc})
			continue
		}
		slots = append(slots, ruleSlot{check: checkRule})
	}
	for i := range slots {
		if slots[i].nc != nil {
			if nodeCheckers == nil {
				nodeCheckers = make([]*ruleSlot, 0, len(slots)/2+1)
			}
			nodeCheckers = append(nodeCheckers, &slots[i])
		}
	}
	return slots, nodeCheckers, errs
}

// runNonNodeCheckers fills the non-NodeChecker slots' diags fields.
// With cap<=1, runs serially (matches pre-plan-190 behaviour). With
// cap>1, runs slots concurrently bounded by a semaphore so no more
// than cap rule.Check calls execute at the same time. Each goroutine
// writes only to its own slot, so the result needs no lock — slots
// are concatenated in rules order after the workers join.
//
// The slots backing was pre-sized in classifyRules so `&slots[i]`
// is stable for the lifetime of this call.
func runNonNodeCheckers(f *lint.File, slots []ruleSlot, intraFileCap int) {
	if intraFileCap <= 1 {
		for i := range slots {
			if slots[i].check == nil {
				continue
			}
			slots[i].diags = slots[i].check.Check(f)
		}
		return
	}
	var wg sync.WaitGroup
	sem := make(chan struct{}, intraFileCap)
	for i := range slots {
		if slots[i].check == nil {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(slot *ruleSlot) {
			defer wg.Done()
			defer func() { <-sem }()
			slot.diags = slot.check.Check(f)
		}(&slots[i])
	}
	wg.Wait()
}

// FilterGeneratedDiags removes diagnostics whose line falls within any
// of the generated section ranges. Called before AdjustDiagnostics, so
// lines are still in post-front-matter coordinates matching the ranges.
func FilterGeneratedDiags(diags []lint.Diagnostic, ranges []lint.LineRange) []lint.Diagnostic {
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

// PopulateSourceContext fills each diagnostic's SourceLines and
// SourceStartLine with surrounding context from f.Lines.
func PopulateSourceContext(f *lint.File, diags []lint.Diagnostic, context int) {
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
