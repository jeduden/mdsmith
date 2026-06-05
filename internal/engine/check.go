package engine

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
// machine output that omits them). Defaults intra-file concurrency to
// 1 (serial); callers that already resolved the cap use
// checkRulesWithIntraFile directly.
func checkRules(
	f *lint.File,
	rules []rule.Rule,
	effective map[string]config.RuleCfg,
	skipSourceContext bool,
) ([]lint.Diagnostic, []error) {
	return checkRulesWithIntraFile(f, rules, effective, skipSourceContext, 1)
}

// ruleSlot is one rule's diagnostic bucket. NodeCheckers and stateful
// NodeVisitorRules append to it from the one shared walk;
// non-walking rules fill it once via Check. Slots are kept in rules
// order so the final concatenation reproduces the sequential output
// exactly. Configure-failed rules never get a slot — they
// short-circuit in classifyRules with an entry in errs.
//
// Exactly one of nc, visitor, or check is set per slot:
//   - nc: a stateless rule.NodeChecker, shown every node.
//   - visitor: the fresh per-file rule.NodeVisitor of a
//     NodeVisitorRule, shown only the kinds in want. A NodeVisitorRule
//     whose NewNodeVisitor returned nil gets no slot at all (it
//     contributes nothing for this file).
//   - check: a non-walking rule, run once via Check.
type ruleSlot struct {
	nc      rule.NodeChecker
	visitor rule.NodeVisitor
	want    map[ast.NodeKind]struct{} // declared kinds for visitor; nil = all
	check   rule.Rule                 // non-nil for non-walking slots
	diags   []lint.Diagnostic
}

// checkRulesWithIntraFile is the core implementation that accepts an
// explicit intra-file concurrency cap. The lintFile path resolves the
// cap once per Run (via resolveIntraFileWorkers) and passes it in here
// so the per-file workers do not each query GOMAXPROCS.
func checkRulesWithIntraFile(
	f *lint.File,
	rules []rule.Rule,
	effective map[string]config.RuleCfg,
	skipSourceContext bool,
	intraFileCap int,
) ([]lint.Diagnostic, []error) {
	slots, walkSlots, errs := classifyRules(f, rules, effective)

	// Run non-walking rules. With cap=1 the loop stays serial and
	// matches the legacy code path byte-for-byte. With cap>1, slots
	// run concurrently into their own buckets; rules order is
	// preserved because the concatenation step reads `slots` in
	// index order at the end.
	runNonNodeCheckers(f, slots, intraFileCap)

	// The one shared walk runs after the goroutine workers join, so its
	// node visitors and the rules running inside it never race for
	// any per-rule state. Walk-driven rules stay internally serial:
	// one goroutine, one walk, every interested rule per node — fast
	// enough that splitting per rule would lose the cache locality the
	// multiplex just won. A NodeChecker is shown every node; a
	// NodeVisitor is shown only the kinds it declared (want == nil
	// means all), so the same single traversal serves both stateless
	// and stateful per-node rules.
	if len(walkSlots) > 0 {
		_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
			kind := n.Kind()
			for _, s := range walkSlots {
				switch {
				case s.nc != nil:
					s.diags = append(s.diags, s.nc.CheckNode(n, entering, f)...)
				case rule.WantsKind(s.want, kind):
					s.diags = append(s.diags, s.visitor.VisitNode(n, entering, f)...)
				}
			}
			return ast.WalkContinue, nil
		})
	}

	var diags []lint.Diagnostic
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

// classifyRules walks the rules list once, configures each enabled
// rule via ConfigureRule (which clones and applies settings only
// when cfg.Settings is non-nil and the rule is Configurable —
// otherwise the worker's existing clone is reused as-is), and splits
// the result into per-rule slots. The slots slice keeps every
// enabled rule in input order (so the final concatenation is
// deterministic); the walkSlots slice is the subset (NodeCheckers and
// stateful NodeVisitorRules) whose group is filled by the one shared
// ast.Walk.
//
// f is needed to build a NodeVisitorRule's fresh per-file visitor
// (NewNodeVisitor(f)). That visitor may carry per-walk state; building
// it here, once per file, is what keeps the state from leaking across
// files or goroutines.
func classifyRules(
	f *lint.File, rules []rule.Rule, effective map[string]config.RuleCfg,
) (slots []ruleSlot, walkSlots []*ruleSlot, errs []error) {
	// Pre-size at the registered-rule count. In production all but a
	// handful of rules are enabled by default. Allocating slots as a
	// value slice (rather than a slice of `*ruleSlot`) collapses the
	// 50+ per-file pointer allocations the previous shape paid into
	// one backing-array allocation. walkSlots stays a pointer
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
		if vr, ok := checkRule.(rule.NodeVisitorRule); ok {
			// A nil visitor means the rule has nothing to do for this
			// file (e.g. an unconfigured opt-in rule); it gets no slot
			// and contributes nothing, matching its standalone Check.
			if v := vr.NewNodeVisitor(f); v != nil {
				slots = append(slots, ruleSlot{
					visitor: v,
					want:    rule.NewKindSet(v.Kinds()),
				})
			}
			continue
		}
		slots = append(slots, ruleSlot{check: checkRule})
	}
	for i := range slots {
		if slots[i].nc != nil || slots[i].visitor != nil {
			if walkSlots == nil {
				walkSlots = make([]*ruleSlot, 0, len(slots)/2+1)
			}
			walkSlots = append(walkSlots, &slots[i])
		}
	}
	return slots, walkSlots, errs
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
