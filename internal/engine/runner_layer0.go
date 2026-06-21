package engine

import (
	"bytes"
	"os"

	"github.com/jeduden/mdsmith/internal/checker"
	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/jeduden/mdsmith/internal/rulelayer"
)

// layer0SkipOverride, when non-nil, forces the Layer 0 parse-skip gate
// on (*true) or off (*false), bypassing the environment. The gate's
// in-package equivalence tests set it so they can flip the toggle without
// mutating the process environment. nil (the default) defers to
// MDSMITH_LAYER0_SKIP.
var layer0SkipOverride *bool

// piOpenScan is the directive opener the gate scans for; any occurrence
// disqualifies the parse skip.
var piOpenScan = []byte("<?")

// layer0SkipEnabled reports whether the master Layer 0 parse-skip toggle
// is on. It honours an in-process override when set, otherwise reads the
// MDSMITH_LAYER0_SKIP environment variable each call so a test in any
// package can flip it via t.Setenv.
//
// Default OFF. The parse-skip path is correct (byte-identical to the parsed
// path, held so by the TestLayer0Gate_* corpus equivalence gates, now for
// list-bearing files too), but it is not yet a measured net win. Profiled
// on the eligible-only subset it is cost-neutral: the skip path's cost is
// dominated by lint.InlineBlocks (~51%), which the parity link/reference
// rules read on the nil-AST path and which still runs a full goldmark parse
// per run — so skipping the block parse saves almost nothing while the
// cheap line scans (~10%) ride on top. It stays an opt-in seam
// (MDSMITH_LAYER0_SKIP=1) until InlineBlocks becomes the light inline scan
// Layer 1 was meant to be — see
// docs/research/benchmarks/parity-parse-skip-findings.md.
func layer0SkipEnabled() bool {
	if layer0SkipOverride != nil {
		return *layer0SkipOverride
	}
	return os.Getenv("MDSMITH_LAYER0_SKIP") != ""
}

// layer0SkipEligible reports whether this file's run can skip the goldmark
// parse and lint from the Layer 0 scan alone. Five conditions must hold:
//
//  1. The MDSMITH_LAYER0_SKIP toggle is set (default off).
//  2. Every enabled rule resolves to Layer 0 (rulelayer.IsLayer0) — no
//     enabled rule navigates f.AST. An unknown rule id is treated as
//     AST-requiring, so a rule the audit does not cover is never skipped.
//  3. The source carries no `<?` directive marker. Generated-section
//     suppression walks the AST for processing instructions, so a file
//     with directives must be parsed.
//  4. The source may contain no block quote (lint.SourceMayHaveBlockQuote
//     is false — i.e. no `>` byte). The block-span scanner collapses a
//     block quote into a single BlockQuote span and never emits the
//     heading/fenced-code spans that block-kind rules (MDS002, MDS015)
//     react to for quote-nested content, so a quote-nested heading would
//     be flagged on the AST path but missed on the parse-skip path.
//     This `>`-free condition doubles as the HTML-block guard: every HTML
//     block opener carries a `>` somewhere it can be excluded on (`<div>`,
//     a closed `-->` comment, a self-closing `<br/>`), and listscan does
//     not model an HTML block's opaque interior, so excluding any `>`
//     keeps those files on the parse path too. A more precise per-line
//     quote scan (so code-heavy `>`-bearing files could skip) needs the
//     block-span scanner to descend into block quotes first; until then
//     the coarse `>` guard is the sound choice.
//
// Lists no longer disqualify the skip. The list rules (MDS014/016/045/
// 046/061) re-derive list structure from f.Lines via listscan on the
// nil-AST path (rule.LinesChecker routes them to Check), and the flat
// ClassifyLines code-line projection descends into list items, so a fence
// or indent nested in a list is classified correctly. Block-kind rules
// read the scanLayer0 block spans, which match goldmark on every
// list-nested heading/fence the corpus exercises. All of this is held
// byte-identical to the AST by TestLayer0Gate_CorpusDiagnosticsEquivalence,
// which now engages on list-bearing files.
//
// The block-only and flat-Layer-0 spike flags force their own
// constructors, so the gate stands down when either is set.
func (r *Runner) layer0SkipEligible(
	source []byte, rules []rule.Rule, effective map[string]config.RuleCfg,
) bool {
	if !layer0SkipEnabled() || r.BlockOnlyParse || r.FlatLayer0 {
		return false
	}
	if bytes.Contains(source, piOpenScan) {
		return false
	}
	if lint.SourceMayHaveBlockQuote(source) {
		return false
	}
	return allEnabledRulesSkipSafe(rules, effective)
}

// allEnabledRulesSkipSafe reports whether every enabled rule in effective
// can lint the parse-skipped (nil-AST) File. A rule qualifies two ways,
// unifying the engine's two parse-skip signals (plan 2606171400):
//
//   - It is a static Layer 0 rule (rulelayer.IsLayer0). Block rules serve
//     the nil-AST path through their rule.BlockChecker dispatch.
//   - Its configured instance reports rule.LineCapable. Line rules whose
//     layer depends on config — line-length is line-capable only without a
//     per-heading limit — serve the nil-AST path through the File's
//     ClassifyLines projection.
//
// A run with no enabled rules trivially qualifies. A disabled rule is
// ignored. An unknown rule that is neither Layer 0 nor line-capable forces
// the parse.
func allEnabledRulesSkipSafe(rules []rule.Rule, effective map[string]config.RuleCfg) bool {
	for _, rl := range rules {
		cfg, ok := effective[rl.Name()]
		if !ok || !cfg.Enabled {
			continue
		}
		if rulelayer.IsLayer0(rl.ID()) {
			continue
		}
		if ruleConfiguredLineCapable(rl, cfg) {
			continue
		}
		return false
	}
	return true
}

// ruleConfiguredLineCapable reports whether rl, once configured with cfg,
// can lint from f.Lines alone (rule.LineCapable). A rule's line-capability
// can depend on its settings, so the rule is configured before the check —
// line-length is line-capable until a per-heading limit is set. A config
// error makes the rule non-skip-safe (the parse path surfaces the error).
func ruleConfiguredLineCapable(rl rule.Rule, cfg config.RuleCfg) bool {
	configured, err := checker.ConfigureRule(rl, cfg)
	if err != nil {
		return false
	}
	lc, ok := configured.(rule.LineCapable)
	return ok && lc.LineCapable()
}

// computeFlatLayer0Active reports whether this run may skip the goldmark
// parse for line-capable files. It requires the opt-in Runner.FlatLayer0
// flag and a config simple enough that the globally-enabled rule set is
// the whole story: every enabled markdown rule must be rule.LineCapable,
// and the config must declare no kinds or overrides (either could enable a
// non-line-capable rule for some file that the empty-path effective config
// does not surface). This conservative gate is sufficient for the prototype
// (plan 2606142147) measurement, whose single-rule line-length config has
// neither; the full Layer-0 plan refines it to a per-file resolution.
func (r *Runner) computeFlatLayer0Active() bool {
	if !r.FlatLayer0 {
		return false
	}
	if len(r.Config.Kinds) > 0 || len(r.Config.Overrides) > 0 {
		return false
	}
	mdRules := markdownRulesFrom(r.Rules, r.ConfigPath)
	effective := r.effectiveWithCategories("", nil, nil)
	for _, rl := range mdRules {
		cfg, ok := effective[rl.Name()]
		if !ok || !cfg.Enabled {
			continue
		}
		// A rule's line-capability can depend on its config (line-length
		// is not line-capable once a per-heading limit is set). With no
		// kinds or overrides, the empty-path effective config is the
		// rule's settings for every file. ruleConfiguredLineCapable
		// applies those settings before asking — the same check the
		// unified parse-skip gate uses.
		if !ruleConfiguredLineCapable(rl, cfg) {
			return false
		}
	}
	return true
}
