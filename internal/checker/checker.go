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
	"github.com/jeduden/mdsmith/pkg/goldmark/ast"
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
		runNodeCheckers(f, nodeCheckers)
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

// kindTable is the per-file dispatch plan for the shared NodeChecker
// walk. Kind-scoped rules (rule.KindScopedChecker) are stored in CSR
// form — scoped[offsets[k]:offsets[k+1]] lists the slots interested in
// kind k — so the walk callback touches only the rules that can react
// to the node at hand instead of calling every rule's CheckNode for
// every node twice. Rules without a kind scope go to generic and keep
// the historical call-for-everything behaviour. Tables are pooled:
// the slices are reused across files, so the per-file build allocates
// nothing once the pool is warm.
type kindTable struct {
	offsets []int32     // len = ast.NodeKindCount()+1; CSR row starts
	scoped  []*ruleSlot // CSR storage, grouped by kind
	generic []*ruleSlot // no kind scope: every node, both directions
}

var kindTablePool = sync.Pool{New: func() any { return new(kindTable) }}

// buildKindTable partitions nodeCheckers into the kind-indexed CSR
// buckets and the generic list. Slots appear in each bucket in input
// (rule) order. The returned table comes from kindTablePool; the
// caller must hand it back via releaseKindTable.
func buildKindTable(nodeCheckers []*ruleSlot) *kindTable {
	t := kindTablePool.Get().(*kindTable)
	n := ast.NodeKindCount()
	if cap(t.offsets) < n+1 {
		t.offsets = make([]int32, n+1)
	} else {
		t.offsets = t.offsets[:n+1]
		clear(t.offsets)
	}
	t.generic = t.generic[:0]

	// Pass 1: count interest per kind into offsets[k+1].
	total := 0
	for _, s := range nodeCheckers {
		ks, ok := s.nc.(rule.KindScopedChecker)
		if !ok {
			t.generic = append(t.generic, s)
			continue
		}
		for _, k := range ks.EnteringKinds() {
			t.offsets[k+1]++
			total++
		}
	}
	// Prefix-sum the counts into row starts.
	for i := 1; i <= n; i++ {
		t.offsets[i] += t.offsets[i-1]
	}
	if cap(t.scoped) < total {
		t.scoped = make([]*ruleSlot, total)
	} else {
		t.scoped = t.scoped[:total]
	}
	// Pass 2: fill, advancing each row's cursor. After this loop every
	// offsets[k] has been advanced to the next row's start, i.e. the
	// slice is shifted one row left; shift it back by walking from the
	// end so offsets is again the row-start table.
	for _, s := range nodeCheckers {
		ks, ok := s.nc.(rule.KindScopedChecker)
		if !ok {
			continue
		}
		for _, k := range ks.EnteringKinds() {
			t.scoped[t.offsets[k]] = s
			t.offsets[k]++
		}
	}
	for i := n; i > 0; i-- {
		t.offsets[i] = t.offsets[i-1]
	}
	t.offsets[0] = 0
	return t
}

// releaseKindTable clears the slot pointers (so pooled tables do not
// pin per-file state) and returns the table to the pool.
func releaseKindTable(t *kindTable) {
	clear(t.scoped)
	clear(t.generic)
	kindTablePool.Put(t)
}

// runNodeCheckers drives the single shared walk over f.AST,
// dispatching each node to the kind-scoped rules registered for its
// kind (entering visits only) and to every generic NodeChecker (both
// visit directions), appending diagnostics into each rule's own slot.
//
// The walk is a direct recursion rather than ast.Walk: the closure
// indirection and the unconditional leaving-visit callback cost real
// time at one call per node on every file, and with no generic
// checkers (the production rule set) the leaving visit dispatches
// nothing at all. Node order matches ast.Walk's pre-order exactly.
func runNodeCheckers(f *lint.File, nodeCheckers []*ruleSlot) {
	t := buildKindTable(nodeCheckers)
	if len(t.generic) == 0 {
		dispatchKindScoped(f.AST, f, t)
	} else {
		dispatchWithGeneric(f.AST, f, t)
	}
	releaseKindTable(t)
}

// dispatchKindScoped is the generic-free walk: entering visits only,
// dispatched straight off the CSR table.
func dispatchKindScoped(n ast.Node, f *lint.File, t *kindTable) {
	k := n.Kind()
	for _, s := range t.scoped[t.offsets[k]:t.offsets[k+1]] {
		if ds := s.nc.CheckNode(n, true, f); len(ds) > 0 {
			s.diags = append(s.diags, ds...)
		}
	}
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		dispatchKindScoped(c, f, t)
	}
}

// dispatchWithGeneric preserves the full ast.Walk visit contract for
// rules without a kind scope: every node, entering and leaving.
func dispatchWithGeneric(n ast.Node, f *lint.File, t *kindTable) {
	k := n.Kind()
	for _, s := range t.scoped[t.offsets[k]:t.offsets[k+1]] {
		if ds := s.nc.CheckNode(n, true, f); len(ds) > 0 {
			s.diags = append(s.diags, ds...)
		}
	}
	for _, s := range t.generic {
		if ds := s.nc.CheckNode(n, true, f); len(ds) > 0 {
			s.diags = append(s.diags, ds...)
		}
	}
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		dispatchWithGeneric(c, f, t)
	}
	for _, s := range t.generic {
		if ds := s.nc.CheckNode(n, false, f); len(ds) > 0 {
			s.diags = append(s.diags, ds...)
		}
	}
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
//
// Each window is a sub-slice of the File's cached zero-copy line
// strings (lint.(*File).LineStrings), so populating context costs no
// allocation per diagnostic. The strings alias the source buffer —
// see LineStrings for the immutability invariant — and stay valid
// for as long as the diagnostics live.
func PopulateSourceContext(f *lint.File, diags []lint.Diagnostic, context int) {
	if len(diags) == 0 {
		return
	}
	lineStrings := f.LineStrings()
	// bytes.Split produces an empty trailing element when source ends
	// with a newline. Exclude it so context windows don't include a
	// phantom empty line.
	numLines := len(lineStrings)
	if numLines > 0 && len(lineStrings[numLines-1]) == 0 {
		numLines--
	}

	for i := range diags {
		lineIdx := diags[i].Line - f.LineOffset - 1 // 0-based into f.Lines
		if lineIdx < 0 || lineIdx >= numLines {
			continue
		}
		start := max(0, lineIdx-context)
		end := min(numLines, lineIdx+context+1)
		diags[i].SourceLines = lineStrings[start:end:end]
		diags[i].SourceStartLine = start + f.LineOffset + 1
	}
}
