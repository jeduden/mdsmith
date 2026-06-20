// Package checker provides the shared rule-checking primitives used by
// both internal/engine (the full linting runner) and internal/fix (the
// auto-fix pipeline). It sits below both callers in the dependency
// graph so neither needs to import the other.
package checker

import (
	"fmt"
	"runtime/debug"
	"sync"

	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/jeduden/mdsmith/pkg/goldmark/ast"
)

// ConfigureEnabledRules returns the enabled rules from rules, each
// configured with its effective settings, in input order, plus any
// settings-application errors. The result depends only on (rules,
// effective) — never on the File — so a caller that lints many files
// under one config can configure once and reuse the slice across files
// (via CheckConfiguredRules) instead of re-cloning every Configurable
// rule per file. Reuse is safe because a rule's Check carries no state
// across calls: the engine already shares the no-settings clones across
// every file in a worker, so sharing the settings clones too is the same
// contract.
func ConfigureEnabledRules(
	rules []rule.Rule, effective map[string]config.RuleCfg,
) ([]rule.Rule, []error) {
	configured := make([]rule.Rule, 0, len(rules))
	var errs []error
	for _, rl := range rules {
		cfg, ok := effective[rl.Name()]
		if !ok || !cfg.Enabled {
			continue
		}
		cr, err := ConfigureRule(rl, cfg)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		configured = append(configured, cr)
	}
	return configured, errs
}

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
	configured, errs := ConfigureEnabledRules(rules, effective)
	return CheckConfiguredRules(f, configured, skipSourceContext, intraFileCap), errs
}

// CheckConfiguredRules runs an already-configured, all-enabled rule list
// (from ConfigureEnabledRules) against f. It is CheckRulesWithIntraFile
// without the per-file rule-configuration pass, so a caller that caches
// the configured rules across files sharing one config pays the clone +
// ApplySettings cost once per config rather than once per file. The
// configuration errors are returned by ConfigureEnabledRules at cache
// build time, so this function returns diagnostics only.
func CheckConfiguredRules(
	f *lint.File,
	configured []rule.Rule,
	skipSourceContext bool,
	intraFileCap int,
) []lint.Diagnostic {
	slots, nodeCheckers, blockCheckers := classifyConfiguredSlots(f, configured)

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
	//
	// On a parse-skipped File (f.AST nil), classifyRules routes every
	// rule into blockCheckers instead, so nodeCheckers is empty and the
	// AST walk is never entered with a nil tree.
	if len(nodeCheckers) > 0 {
		runNodeCheckers(f, nodeCheckers)
	}
	if len(blockCheckers) > 0 {
		runBlockCheckers(f, blockCheckers)
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
	return diags
}

// ruleSlot is one rule's diagnostic bucket. NodeCheckers append to it
// from the shared AST walk; BlockCheckers from the shared block-span
// dispatch on a parse-skipped File; non-NodeCheckers fill it once via
// Check. Slots are kept in rules order so the final concatenation
// reproduces the sequential output exactly. Configure-failed rules never
// get a slot — they short-circuit in classifyRules with an entry in errs.
type ruleSlot struct {
	nc    rule.NodeChecker
	bc    rule.BlockChecker // non-nil for the nil-AST block-span path
	check rule.Rule         // non-nil for non-NodeChecker slots
	diags []lint.Diagnostic
}

// classifyRules walks the rules list once, configures each enabled
// rule via ConfigureRule (which clones and applies settings only
// when cfg.Settings is non-nil and the rule is Configurable —
// otherwise the worker's existing clone is reused as-is), and splits
// the result into per-rule slots. The slots slice keeps every
// enabled rule in input order (so the final concatenation is
// deterministic); nodeCheckers and blockCheckers are the subsets the
// two shared dispatches fill.
//
// The two dispatches are mutually exclusive per File: when f.AST is set
// (the parsed path) a NodeChecker goes to nodeCheckers and the shared
// AST walk drives its CheckNode; when f.AST is nil (the Layer 0
// parse-skip path) a rule that is also a rule.BlockChecker goes to
// blockCheckers and the shared block-span dispatch drives its CheckBlock
// instead. A nil-AST NodeChecker that is NOT a BlockChecker cannot run
// on the tree-less File, so it is dropped from both — the engine's gate
// guarantees that case never reaches here (it parses unless every
// enabled rule is Layer 0), so dropping it is a defensive belt, not a
// behaviour the production path relies on.
func classifyRules(
	f *lint.File, rules []rule.Rule, effective map[string]config.RuleCfg,
) (slots []ruleSlot, nodeCheckers, blockCheckers []*ruleSlot, errs []error) {
	configured, errs := ConfigureEnabledRules(rules, effective)
	slots, nodeCheckers, blockCheckers = classifyConfiguredSlots(f, configured)
	return slots, nodeCheckers, blockCheckers, errs
}

// classifyConfiguredSlots is the per-file half of classifyRules: it splits
// an already-configured, all-enabled rule list (from ConfigureEnabledRules)
// into per-rule slots and the node/block dispatch subsets. The expensive
// clone + ApplySettings work is done once up front by the caller, so the
// only per-file cost here is the cheap, astNil-dependent slot typing —
// which lets a multi-file run that shares one config configure each rule
// once instead of once per file.
func classifyConfiguredSlots(
	f *lint.File, configured []rule.Rule,
) (slots []ruleSlot, nodeCheckers, blockCheckers []*ruleSlot) {
	// Pre-size at the configured-rule count. Allocating slots as a value
	// slice (rather than a slice of `*ruleSlot`) collapses the 50+ per-file
	// pointer allocations the previous shape paid into one backing-array
	// allocation. nodeCheckers stays a pointer slice but references entries
	// by `&slots[i]`, which is stable because the slots cap was pre-set to
	// len(configured) — no append grows the backing, so the index-derived
	// pointers do not dangle.
	astNil := f != nil && f.AST == nil
	slots = make([]ruleSlot, 0, len(configured))
	for _, cr := range configured {
		slots = append(slots, classifySlot(cr, astNil))
	}
	for i := range slots {
		switch {
		case slots[i].bc != nil:
			if blockCheckers == nil {
				blockCheckers = make([]*ruleSlot, 0, len(slots)/2+1)
			}
			blockCheckers = append(blockCheckers, &slots[i])
		case slots[i].nc != nil:
			if nodeCheckers == nil {
				nodeCheckers = make([]*ruleSlot, 0, len(slots)/2+1)
			}
			nodeCheckers = append(nodeCheckers, &slots[i])
		}
	}
	return slots, nodeCheckers, blockCheckers
}

// classifySlot builds one rule's slot. On a parse-skipped File (astNil)
// a rule.BlockChecker is routed to the block-span dispatch; on a parsed
// File a rule.NodeChecker is routed to the shared AST walk. A non-node
// rule fills its own slot via Check. A nil-AST NodeChecker that is not a
// BlockChecker gets an empty slot (no dispatch claims it); see
// classifyRules for why that path is unreachable in production.
func classifySlot(checkRule rule.Rule, astNil bool) ruleSlot {
	if nc, ok := checkRule.(rule.NodeChecker); ok {
		if astNil {
			if bc, ok := checkRule.(rule.BlockChecker); ok {
				return ruleSlot{bc: bc}
			}
			// An InlineChecker serves the nil-AST path from its own Check
			// (reading lint.InlineBlocks), so route it to the plain-Check
			// slot rather than dropping it.
			if ic, ok := checkRule.(rule.InlineChecker); ok && ic.InlineCapable() {
				return ruleSlot{check: checkRule}
			}
			return ruleSlot{}
		}
		return ruleSlot{nc: nc}
	}
	return ruleSlot{check: checkRule}
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

// dispatchScoped runs the kind-scoped rules registered for n's kind
// (entering visit). Shared by both walk variants below.
func dispatchScoped(n ast.Node, f *lint.File, t *kindTable) {
	k := n.Kind()
	for _, s := range t.scoped[t.offsets[k]:t.offsets[k+1]] {
		if ds := s.nc.CheckNode(n, true, f); len(ds) > 0 {
			s.diags = append(s.diags, ds...)
		}
	}
}

// dispatchKindScoped is the generic-free walk: entering visits only,
// dispatched straight off the CSR table.
func dispatchKindScoped(n ast.Node, f *lint.File, t *kindTable) {
	dispatchScoped(n, f, t)
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		dispatchKindScoped(c, f, t)
	}
}

// dispatchWithGeneric preserves the full ast.Walk visit contract for
// rules without a kind scope: every node, entering and leaving.
func dispatchWithGeneric(n ast.Node, f *lint.File, t *kindTable) {
	dispatchScoped(n, f, t)
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

// runBlockCheckers drives the shared block-span dispatch for a
// parse-skipped File (f.AST nil): it walks the Layer 0 block scan once,
// in document order, and for each span calls CheckBlock on every
// block-checker slot whose rule reacts to that span's kind, appending
// diagnostics into each rule's own slot. Spans are visited in document
// order and slots in rule order, so the concatenated result is grouped
// by rule exactly as the AST walk groups CheckNode output — the byte-
// identical contract rule.BlockChecker promises.
//
// The block-checker count is tiny (the migrated structural rules), so a
// per-span linear scan over the slots beats building a kind-indexed
// table and allocates nothing beyond the diagnostics themselves.
func runBlockCheckers(f *lint.File, blockCheckers []*ruleSlot) {
	scan := lint.Layer0(f)
	for _, span := range scan.BlockSpans {
		for _, s := range blockCheckers {
			if !blockCheckerReactsTo(s.bc, span.Kind) {
				continue
			}
			if ds := s.bc.CheckBlock(span, f); len(ds) > 0 {
				s.diags = append(s.diags, ds...)
			}
		}
	}
}

// blockCheckerReactsTo reports whether bc declares interest in kind. The
// kind set per rule is tiny (one or two kinds), so a linear scan over
// BlockKinds() beats a map and allocates nothing.
func blockCheckerReactsTo(bc rule.BlockChecker, kind lint.BlockKind) bool {
	for _, want := range bc.BlockKinds() {
		if want == kind {
			return true
		}
	}
	return false
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
			defer func() {
				if rv := recover(); rv != nil {
					stack := debug.Stack()
					slot.diags = []lint.Diagnostic{{
						File:     f.Path,
						Line:     1,
						RuleID:   "internal-panic",
						RuleName: "internal-panic",
						Severity: lint.Error,
						Message: fmt.Sprintf(
							"internal error: rule panic: %v\n%s", rv, stack),
					}}
				}
			}()
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
