package integration

// rule_walk_audit_test.go is plan 215's survey and regression gate.
//
// Phase one (survey): classifyRuleWalk probes every registered rule
// with two runtime experiments and a static-source scan, then writes a
// JSON manifest classifying each rule A / B / ast-required / hybrid.
//
//   - Probe one (nil-AST safety): run the rule normally, then again
//     with f.AST = nil. f.AST is an exported field, not a method, so a
//     wrapper cannot intercept reads; nilling it is the only way to
//     observe whether Check actually walks the tree. A rule that
//     survives the nil run with identical diagnostics never depended on
//     the tree for this input.
//   - Probe two (code-block sensitivity): run the rule on the fixture,
//     then on a perturbation where only bytes inside code blocks /
//     code spans change. A rule whose diagnostics move was relying on
//     the AST's implicit code-skipping filter and must keep it.
//
// The two probes yield three signals — nil-AST safety, code-block
// sensitivity, diagnostic equality — that map onto the four categories
// in plan 215's "Phase one — survey" section.
//
// Phase three (gate): TestRuleWalkAuditManifest recomputes the
// classification and fails if it drifts from the checked-in manifest.
// A converted rule that regresses to reading f.AST flips its static
// readsFileAST signal (or, for a Category A rule, its nilASTSafe
// signal) and the manifest comparison fails until the JSON is
// regenerated — which forces a human to acknowledge the change.
//
// Regenerate the manifest after an intended change with:
//
//	MDSMITH_REGEN_WALK_AUDIT=1 go test ./internal/integration/ \
//	    -run TestRuleWalkAuditManifest

import (
	"encoding/json"
	"go/ast"
	"go/types"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"testing"

	"github.com/jeduden/mdsmith/internal/checker"
	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/stretchr/testify/require"
	gmast "github.com/yuin/goldmark/ast"
	"golang.org/x/tools/go/packages"
)

// walkCategory is the four-way classification plan 215 assigns to each
// rule. The string values land verbatim in the JSON manifest.
type walkCategory string

const (
	// catA — no skipping. The nil-AST run matches the normal run AND
	// the code-block perturbation produces the same diagnostics: the
	// rule applies to every line regardless of code context. Direct
	// f.Lines conversion, no scaffold.
	catA walkCategory = "A-no-skipping"
	// catB — prose-only. The nil-AST run matches the normal run, but
	// the code-block perturbation changes diagnostics: the rule needs
	// the code-skipping filter. Rewrite drives f.ProseRanges().
	catB walkCategory = "B-prose-only"
	// catASTRequired — the nil-AST run panics or produces different
	// diagnostics on unperturbed input. Keep the AST.
	catASTRequired walkCategory = "ast-required"
	// catHybrid — the nil-AST run survives but emits a different
	// diagnostic set on unperturbed input. Out of scope for rewrite.
	catHybrid walkCategory = "hybrid"
	// catInconclusive — the rule emitted no diagnostics on the audit
	// fixture (commonly an opt-in rule with an empty default config, or
	// a rule whose trigger the shared fixture does not contain). The
	// nil-AST and code-block probes cannot tell "Lines-only" from "did
	// not fire" when both runs are empty, so such a rule is NOT a
	// rewrite candidate on probe evidence alone. The static signals are
	// still recorded; a contributor converting one of these must
	// ground-truth it by reading the rule. The gate still tracks its
	// static f.AST signal so a regression is caught.
	catInconclusive walkCategory = "inconclusive-not-fired"
)

// ruleWalkEntry is one rule's row in the manifest. The runtime signals
// (NilASTSafe, CodeBlockSensitive) and the static signals (UsesASTWalk,
// ReadsFileAST) are both recorded so a reviewer can see why a rule
// landed in its category, and so the gate catches a converted rule that
// silently regrows an f.AST read.
type ruleWalkEntry struct {
	ID       string       `json:"id"`
	Name     string       `json:"name"`
	Category walkCategory `json:"category"`
	// NilASTSafe: Check ran on f.AST == nil without panicking and
	// returned the same diagnostics as the normal run.
	NilASTSafe bool `json:"nil_ast_safe"`
	// CodeBlockSensitive: mutating only code-block / code-span bytes
	// changed the rule's diagnostics.
	CodeBlockSensitive bool `json:"code_block_sensitive"`
	// Fired: the normal run produced at least one diagnostic on the
	// audit fixture. When false, the nil-AST and code-block probes are
	// inconclusive (empty == empty proves nothing), so the rule is
	// classified inconclusive-not-fired regardless of the other
	// signals.
	Fired bool `json:"fired"`
	// IsNodeChecker: the rule's Check delegates to the engine's shared
	// multiplex walk (rule.NodeChecker). Plan 215 leaves these alone —
	// their walk is already amortized — but records the signal so the
	// manifest explains why a nil-AST-unsafe NodeChecker is not a
	// rewrite target.
	IsNodeChecker bool `json:"is_node_checker"`
	// UsesASTWalk / ReadsFileAST: static-scan signals. UsesASTWalk is
	// true when the rule package calls goldmark ast.Walk; ReadsFileAST
	// is true when it reads the lint.File.AST field. A Category A or B
	// rule that has been converted clears ReadsFileAST; the gate fails
	// if it regrows.
	UsesASTWalk  bool `json:"uses_ast_walk"`
	ReadsFileAST bool `json:"reads_file_ast"`
}

const ruleWalkAuditPath = "testdata/rule_walk_audit.json"

// auditProbeInput is one probe fixture: the markdown body, the settings
// to apply to the rule, and a label for diagnostics.
type auditProbeInput struct {
	label    string
	settings map[string]any
	content  []byte
}

// classifyRuleWalk runs the two runtime probes for a single rule and
// returns the category plus the raw signals. It never fails the test;
// classification of every rule is the point, so a panicking rule is
// recorded rather than aborting the survey.
//
// Each rule is driven against its OWN bad fixtures (internal/rules/
// MDS###-*/bad/*.md). A bad fixture is the representative input that, by
// construction, triggers the rule: it carries the settings that
// configure opt-in rules and contains the exact pattern the rule flags.
// A single shared fixture cannot do this — most rules are compliant on
// any given document, so a shared fixture leaves the nil-AST and
// code-block probes comparing empty-to-empty, which proves nothing.
//
// Signals aggregate across the rule's bad fixtures:
//   - Fired: some bad fixture produced ≥1 diagnostic.
//   - NilASTSafe: on every fired fixture, the nil-AST run neither
//     panicked nor changed the diagnostics.
//   - CodeBlockSensitive: on some fired fixture, scrambling only
//     code-block / code-span bytes changed the diagnostics.
func classifyRuleWalk(tb testing.TB, r rule.Rule, static staticWalkSignal, inputs []auditProbeInput) ruleWalkEntry {
	tb.Helper()

	_, isNC := r.(rule.NodeChecker)

	var fired, nilPanicked, nilDiverged, codeSensitive bool
	for _, in := range inputs {
		normal := runRuleForAudit(tb, r, in, in.content, false)
		if len(normal.diags) == 0 {
			// This fixture did not trigger the rule (e.g. a bad fixture
			// for a sibling rule sharing the directory, or a setting the
			// probe could not reconstruct). Skip — it carries no signal.
			continue
		}
		fired = true

		nilAST := runRuleForAudit(tb, r, in, in.content, true)
		switch {
		case nilAST.panicked:
			nilPanicked = true
		case !diagsEqual(normal.diags, nilAST.diags):
			nilDiverged = true
		}
		perturbed := runRuleForAudit(tb, r, in, perturbCodeBytes(in.content), false)
		if !diagsEqual(normal.diags, perturbed.diags) {
			codeSensitive = true
		}
	}

	nilUnsafe := nilPanicked || nilDiverged
	entry := ruleWalkEntry{
		ID:                 r.ID(),
		Name:               r.Name(),
		NilASTSafe:         fired && !nilUnsafe,
		CodeBlockSensitive: codeSensitive,
		Fired:              fired,
		IsNodeChecker:      isNC,
		UsesASTWalk:        static.usesASTWalk,
		ReadsFileAST:       static.readsFileAST,
	}

	switch {
	case !fired:
		// No bad fixture triggered the rule; the nil-AST and code-block
		// signals prove nothing. Do not route it to A or B on probe
		// evidence alone.
		entry.Category = catInconclusive
	case nilPanicked:
		// On some fired fixture the nil-AST run panicked: the rule
		// dereferences the tree (directly or via a helper that walks
		// it) for that input. AST-required.
		entry.Category = catASTRequired
	case nilDiverged:
		// The nil-AST run survived but produced a different diagnostic
		// set on unperturbed input — the plan's hybrid case: the rule
		// reads the tree but degrades rather than crashing. Out of
		// scope for a mechanical rewrite.
		entry.Category = catHybrid
	case codeSensitive:
		entry.Category = catB
	default:
		entry.Category = catA
	}
	return entry
}

// auditRun bundles one probe execution's outcome.
type auditRun struct {
	diags    []lint.Diagnostic
	panicked bool
}

// runRuleForAudit configures a clone of r with the probe input's
// settings (the rule's own bad-fixture front matter), builds a fresh
// lint.File from src, optionally nils the AST, and runs Check. Panics
// are recovered and surfaced via auditRun.panicked so the survey can
// classify a nil-AST-fragile rule instead of crashing. A fresh File is
// built per call so per-File memos start cold, matching production.
func runRuleForAudit(tb testing.TB, r rule.Rule, in auditProbeInput, src []byte, nilAST bool) (out auditRun) {
	tb.Helper()

	// Layer the fixture's settings over the rule's defaults so opt-in
	// rules (empty default config) fire, while rules with meaningful
	// defaults keep them unless the fixture overrides a key.
	settings := map[string]any{}
	if c, ok := r.(rule.Configurable); ok {
		for k, v := range c.DefaultSettings() {
			settings[k] = v
		}
	}
	for k, v := range in.settings {
		settings[k] = v
	}
	var applied map[string]any
	if len(settings) > 0 {
		applied = settings
	}

	cr, err := checker.ConfigureRule(r, config.RuleCfg{Settings: applied})
	require.NoError(tb, err, "configuring %s for fixture %s", r.ID(), in.label)

	f, err := lint.NewFile("doc.md", src)
	require.NoError(tb, err, "parsing %s fixture %s", r.ID(), in.label)
	f.FS = allocBudgetFS()
	f.RootDir = "."
	f.RunCache = lint.NewRunCache()
	if nilAST {
		f.AST = nil
	}

	defer func() {
		if rec := recover(); rec != nil {
			out.panicked = true
		}
	}()
	out.diags = cr.Check(f)
	return out
}

// perturbCodeBytes returns src with every byte inside a fenced code
// block, an indented code block, an HTML block, or an inline code span
// replaced by 'z' — preserving length (and therefore every line number
// and column) so a diagnostic-set difference can only come from the
// changed code content, not from shifted offsets. The ranges come from
// the AST, the same source the rule itself would consult, so the
// perturbation matches the skipping a real rule relies on.
func perturbCodeBytes(src []byte) []byte {
	f, err := lint.NewFile("perturb.md", src)
	if err != nil {
		return src
	}
	out := make([]byte, len(f.Source))
	copy(out, f.Source)
	scramble := func(start, stop int) {
		for b := start; b < stop && b < len(out); b++ {
			if out[b] != '\n' {
				out[b] = 'z'
			}
		}
	}
	_ = gmast.Walk(f.AST, func(n gmast.Node, entering bool) (gmast.WalkStatus, error) {
		if !entering {
			return gmast.WalkContinue, nil
		}
		switch n.(type) {
		case *gmast.FencedCodeBlock, *gmast.CodeBlock, *gmast.HTMLBlock:
			// Block nodes expose their content via Lines().
			lines := n.Lines()
			for i := 0; i < lines.Len(); i++ {
				seg := lines.At(i)
				scramble(seg.Start, seg.Stop)
			}
		case *gmast.CodeSpan:
			// Lines() panics on inline nodes ("can not call with
			// inline nodes"); a code span's content is its child Text
			// segments instead.
			for c := n.FirstChild(); c != nil; c = c.NextSibling() {
				if t, ok := c.(*gmast.Text); ok {
					seg := t.Segment
					scramble(seg.Start, seg.Stop)
				}
			}
		}
		return gmast.WalkContinue, nil
	})
	return out
}

// diagsEqual compares two diagnostic slices on the fields a rule
// controls (Line, Column, RuleID, Message, Severity), ignoring source
// context which the probe never populates. Order-insensitive: the two
// probe runs walk the same document, but a defensive sort keeps the
// comparison robust to incidental ordering.
func diagsEqual(a, b []lint.Diagnostic) bool {
	if len(a) != len(b) {
		return false
	}
	key := func(d lint.Diagnostic) string {
		return d.RuleID + "\x00" + d.Message + "\x00" +
			strconv.Itoa(d.Line) + "\x00" + strconv.Itoa(d.Column) +
			"\x00" + string(d.Severity)
	}
	ka := make([]string, len(a))
	kb := make([]string, len(b))
	for i := range a {
		ka[i] = key(a[i])
	}
	for i := range b {
		kb[i] = key(b[i])
	}
	sort.Strings(ka)
	sort.Strings(kb)
	for i := range ka {
		if ka[i] != kb[i] {
			return false
		}
	}
	return true
}

// staticWalkSignal records, per rule package, whether the package's
// non-test source statically references goldmark ast.Walk or reads the
// lint.File.AST field. Type information disambiguates: a `.AST` selector
// only counts when its receiver type is lint.File (or *lint.File), and
// ast.Walk only counts when it resolves to the goldmark package — so an
// unrelated identifier named AST in some other type does not produce a
// false positive.
type staticWalkSignal struct {
	usesASTWalk  bool
	readsFileAST bool
}

// staticScanRules loads every rule package once with go/packages (full
// type info) and records, per package import path, the two static
// signals plus the rule Name the package declares. The runtime registry
// exposes a rule's ID and Name but not its source package; the Name
// literal bridges a registered rule to the package's static signals.
func staticScanRules(tb testing.TB) (sigByPath map[string]staticWalkSignal, nameByPath map[string]string) {
	tb.Helper()

	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles |
			packages.NeedSyntax | packages.NeedTypes |
			packages.NeedTypesInfo | packages.NeedDeps |
			packages.NeedImports,
		Tests: false,
	}
	pkgs, err := packages.Load(cfg, "github.com/jeduden/mdsmith/internal/rules/...")
	require.NoError(tb, err, "loading rule packages")

	sigByPath = make(map[string]staticWalkSignal)
	nameByPath = make(map[string]string)
	for _, p := range pkgs {
		if len(p.Errors) > 0 {
			// A load error on one package should not silently drop its
			// signal; fail loudly so the manifest cannot go stale.
			for _, e := range p.Errors {
				tb.Fatalf("go/packages error in %s: %v", p.PkgPath, e)
			}
		}
		sigByPath[p.PkgPath] = scanPackageSyntax(p)
		if name := ruleNameLiteral(p); name != "" {
			nameByPath[p.PkgPath] = name
		}
	}
	return sigByPath, nameByPath
}

// scanPackageSyntax inspects one loaded package's syntax + type info for
// the two static signals.
func scanPackageSyntax(p *packages.Package) staticWalkSignal {
	var sig staticWalkSignal
	for _, file := range p.Syntax {
		ast.Inspect(file, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			// ast.Walk: resolve the selector to the goldmark ast.Walk
			// function via type info.
			if obj := p.TypesInfo.Uses[sel.Sel]; obj != nil {
				if fn, ok := obj.(*types.Func); ok {
					if fn.Name() == "Walk" && fn.Pkg() != nil &&
						fn.Pkg().Path() == "github.com/yuin/goldmark/ast" {
						sig.usesASTWalk = true
					}
				}
			}
			// .AST field on lint.File: the selector's base expression
			// must have type lint.File or *lint.File.
			if sel.Sel.Name == "AST" {
				if tv, ok := p.TypesInfo.Types[sel.X]; ok && isLintFile(tv.Type) {
					sig.readsFileAST = true
				}
			}
			return true
		})
	}
	return sig
}

// isLintFile reports whether t is lint.File or *lint.File.
func isLintFile(t types.Type) bool {
	if ptr, ok := t.(*types.Pointer); ok {
		t = ptr.Elem()
	}
	named, ok := t.(*types.Named)
	if !ok {
		return false
	}
	obj := named.Obj()
	return obj != nil && obj.Name() == "File" && obj.Pkg() != nil &&
		obj.Pkg().Path() == "github.com/jeduden/mdsmith/internal/lint"
}

// buildManifest computes the full classification for every registered
// rule, pairing each with the static signals from its package and the
// rule's own bad fixtures as probe input.
func buildManifest(t *testing.T) []ruleWalkEntry {
	t.Helper()

	sigByPath, nameByPath := staticScanRules(t)
	fixturesByID := loadBadFixturesByID(t)

	// Bridge each registered rule to its package's static signals via
	// the Name literal the package declares.
	sigByName := make(map[string]staticWalkSignal, len(nameByPath))
	for path, name := range nameByPath {
		sigByName[name] = sigByPath[path]
	}

	rules := rule.All()
	entries := make([]ruleWalkEntry, 0, len(rules))
	for _, r := range rules {
		static := sigByName[r.Name()]
		entries = append(entries, classifyRuleWalk(t, r, static, fixturesByID[r.ID()]))
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].ID != entries[j].ID {
			return entries[i].ID < entries[j].ID
		}
		return entries[i].Name < entries[j].Name
	})
	return entries
}

// loadBadFixturesByID reads every rule's bad fixtures into probe inputs
// keyed by rule ID. A rule's bad fixtures live at
// internal/rules/MDS###-<name>/bad/*.md (folder format) or
// internal/rules/MDS###-<name>/bad.md (single-file format). The
// front-matter settings configure opt-in rules; the body contains the
// pattern the rule flags. Rules with no bad fixture (config-target
// rules, helper-backed rules) get an empty slice and classify as
// inconclusive-not-fired.
func loadBadFixturesByID(t *testing.T) map[string][]auditProbeInput {
	t.Helper()

	dirs := discoverFixtureDirs(t)
	out := make(map[string][]auditProbeInput)
	for _, dir := range dirs {
		base := filepath.Base(dir)
		m := ruleIDPattern.FindStringSubmatch(base)
		if m == nil {
			continue
		}
		ruleID := m[1]
		out[ruleID] = append(out[ruleID], loadBadFixtureInputs(t, dir)...)
	}
	return out
}

// loadBadFixtureInputs gathers the bad fixtures for one rule directory.
func loadBadFixtureInputs(t *testing.T, dir string) []auditProbeInput {
	t.Helper()

	var paths []string
	badDir := filepath.Join(dir, "bad")
	if isDir(badDir) {
		files, err := filepath.Glob(filepath.Join(badDir, "*.md"))
		require.NoError(t, err)
		paths = append(paths, files...)
	}
	if single := filepath.Join(dir, "bad.md"); fileExists(single) {
		paths = append(paths, single)
	}

	var inputs []auditProbeInput
	for _, p := range paths {
		raw, err := os.ReadFile(p) //nolint:gosec // test fixture path
		require.NoError(t, err)
		settings, _, content := parseFixtureFrontMatter(t, raw, false)
		inputs = append(inputs, auditProbeInput{
			label:    filepath.Base(p),
			settings: settings,
			content:  content,
		})
	}
	return inputs
}

// ruleNameLiteral finds the string literal returned by a package's
// `Name() string` method — the rule's stable name (e.g. "line-length").
// Returns "" if the package declares no such method (helper packages
// like astutil).
func ruleNameLiteral(p *packages.Package) string {
	for _, file := range p.Syntax {
		var found string
		ast.Inspect(file, func(n ast.Node) bool {
			fn, ok := n.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || fn.Name.Name != "Name" {
				return true
			}
			// Expect a single `return "<literal>"` statement.
			if fn.Body == nil || len(fn.Body.List) != 1 {
				return true
			}
			ret, ok := fn.Body.List[0].(*ast.ReturnStmt)
			if !ok || len(ret.Results) != 1 {
				return true
			}
			lit, ok := ret.Results[0].(*ast.BasicLit)
			if !ok {
				return true
			}
			// Strip the surrounding quotes.
			if len(lit.Value) >= 2 {
				found = lit.Value[1 : len(lit.Value)-1]
			}
			return false
		})
		if found != "" {
			return found
		}
	}
	return ""
}

// TestRuleWalkAuditManifest is the survey + regression gate. It
// recomputes the classification for every registered rule and compares
// it to the checked-in manifest. Regenerate with
// MDSMITH_REGEN_WALK_AUDIT=1 after an intended change.
func TestRuleWalkAuditManifest(t *testing.T) {
	if testing.Short() {
		t.Skip("walk audit skipped in -short mode (runs go/packages)")
	}
	if raceEnabled {
		t.Skip("walk audit skipped under -race; go/packages subprocess " +
			"plus per-rule probes are slow and allocation-sensitive")
	}

	got := buildManifest(t)

	gotJSON, err := json.MarshalIndent(got, "", "  ")
	require.NoError(t, err)
	gotJSON = append(gotJSON, '\n')

	path := filepath.Clean(ruleWalkAuditPath)
	if os.Getenv("MDSMITH_REGEN_WALK_AUDIT") == "1" {
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, gotJSON, 0o644))
		t.Logf("regenerated %s with %d rules", path, len(got))
		return
	}

	want, err := os.ReadFile(path)
	require.NoError(t, err,
		"manifest %s missing; regenerate with MDSMITH_REGEN_WALK_AUDIT=1", path)

	require.Equal(t, string(want), string(gotJSON),
		"rule walk audit manifest drifted; if intentional, regenerate with "+
			"MDSMITH_REGEN_WALK_AUDIT=1 go test ./internal/integration/ "+
			"-run TestRuleWalkAuditManifest")
}
