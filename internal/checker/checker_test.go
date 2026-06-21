package checker_test

import (
	"errors"
	"testing"

	"github.com/jeduden/mdsmith/internal/checker"
	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/jeduden/mdsmith/pkg/goldmark/ast"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- stubs ---

type plainRule struct{ id string }

func (r *plainRule) ID() string                           { return r.id }
func (r *plainRule) Name() string                         { return r.id }
func (r *plainRule) Category() string                     { return "test" }
func (r *plainRule) Check(_ *lint.File) []lint.Diagnostic { return nil }

type diagRule struct {
	plainRule
	diag lint.Diagnostic
}

func (r *diagRule) Check(_ *lint.File) []lint.Diagnostic {
	return []lint.Diagnostic{r.diag}
}

type nodeCheckerRule struct {
	plainRule
	diag lint.Diagnostic
}

func (r *nodeCheckerRule) Check(_ *lint.File) []lint.Diagnostic { return nil }
func (r *nodeCheckerRule) CheckNode(_ ast.Node, entering bool, _ *lint.File) []lint.Diagnostic {
	if entering {
		return []lint.Diagnostic{r.diag}
	}
	return nil
}

var _ rule.NodeChecker = (*nodeCheckerRule)(nil)

// blockCheckerRule emits one diagnostic per thematic-break span (block
// path) and one per ThematicBreak node (AST path), so a test can assert
// the engine drives the same output through both seams.
type blockCheckerRule struct {
	plainRule
	message string
}

func (r *blockCheckerRule) Check(f *lint.File) []lint.Diagnostic {
	if f != nil && f.AST == nil {
		return rule.WalkBlocks(r, f)
	}
	return rule.WalkNodes(r, f)
}

func (r *blockCheckerRule) CheckNode(n ast.Node, entering bool, f *lint.File) []lint.Diagnostic {
	if entering && n.Kind() == ast.KindThematicBreak {
		return []lint.Diagnostic{{Line: 1, RuleID: r.id, Message: r.message}}
	}
	return nil
}

func (r *blockCheckerRule) CheckBlock(span lint.BlockSpan, _ *lint.File) []lint.Diagnostic {
	return []lint.Diagnostic{{Line: span.Start, RuleID: r.id, Message: r.message}}
}

func (r *blockCheckerRule) BlockKinds() []lint.BlockKind {
	return []lint.BlockKind{lint.BlockThematicBreak}
}

func (r *blockCheckerRule) EnteringKinds() []ast.NodeKind {
	return []ast.NodeKind{ast.KindThematicBreak}
}

var _ rule.BlockChecker = (*blockCheckerRule)(nil)

// inlineCheckerRule is a NodeChecker that is NOT a BlockChecker but
// implements rule.InlineChecker: its Check serves the nil-AST path itself.
// It lets a test pin that the engine routes such a rule to its own Check on
// a parse-skipped File rather than dropping it.
type inlineCheckerRule struct {
	plainRule
	message string
}

func (r *inlineCheckerRule) Check(f *lint.File) []lint.Diagnostic {
	if f != nil && f.AST == nil {
		// Stand in for the real rules' lint.InlineBlocks consumption.
		return []lint.Diagnostic{{Line: 1, RuleID: r.id, Message: r.message}}
	}
	return rule.WalkNodes(r, f)
}

func (r *inlineCheckerRule) CheckNode(n ast.Node, entering bool, _ *lint.File) []lint.Diagnostic {
	if entering && n.Kind() == ast.KindParagraph {
		return []lint.Diagnostic{{Line: 1, RuleID: r.id, Message: r.message}}
	}
	return nil
}

func (r *inlineCheckerRule) InlineCapable() bool { return true }

var _ rule.InlineChecker = (*inlineCheckerRule)(nil)

// linesCheckerRule is a NodeChecker that is NOT a BlockChecker but implements
// rule.LinesChecker: its Check serves the nil-AST path from f.Lines (standing
// in for the list rules' listscan consumption). It pins that the engine
// routes such a rule to its own Check on a parse-skipped File.
type linesCheckerRule struct {
	plainRule
	message string
}

func (r *linesCheckerRule) Check(f *lint.File) []lint.Diagnostic {
	if f != nil && f.AST == nil {
		return []lint.Diagnostic{{Line: 1, RuleID: r.id, Message: r.message}}
	}
	return rule.WalkNodes(r, f)
}

func (r *linesCheckerRule) CheckNode(n ast.Node, entering bool, _ *lint.File) []lint.Diagnostic {
	if entering && n.Kind() == ast.KindParagraph {
		return []lint.Diagnostic{{Line: 1, RuleID: r.id, Message: r.message}}
	}
	return nil
}

func (r *linesCheckerRule) LinesCapable() bool { return true }

var _ rule.LinesChecker = (*linesCheckerRule)(nil)

type goodConfigurable struct {
	plainRule
	Applied map[string]any
}

func (r *goodConfigurable) ApplySettings(s map[string]any) error {
	r.Applied = s
	return nil
}
func (r *goodConfigurable) DefaultSettings() map[string]any { return nil }

var _ rule.Configurable = (*goodConfigurable)(nil)

type errConfigurable struct{ plainRule }

func (r *errConfigurable) ApplySettings(_ map[string]any) error {
	return errors.New("intentional settings error")
}
func (r *errConfigurable) DefaultSettings() map[string]any { return nil }

var _ rule.Configurable = (*errConfigurable)(nil)

// --- helpers ---

func newTestFile(t *testing.T, src string) *lint.File {
	t.Helper()
	f, err := lint.NewFile("doc.md", []byte(src))
	require.NoError(t, err)
	f.RootDir = "."
	f.RunCache = lint.NewRunCache()
	return f
}

func enabled(ids ...string) map[string]config.RuleCfg {
	m := make(map[string]config.RuleCfg, len(ids))
	for _, id := range ids {
		m[id] = config.RuleCfg{Enabled: true}
	}
	return m
}

// --- ConfigureRule ---

func TestConfigureRule_nilSettings(t *testing.T) {
	r := &plainRule{id: "TST001"}
	got, err := checker.ConfigureRule(r, config.RuleCfg{Settings: nil})
	require.NoError(t, err)
	assert.Same(t, r, got.(*plainRule))
}

func TestConfigureRule_nonConfigurableWithSettings(t *testing.T) {
	r := &plainRule{id: "TST001"}
	got, err := checker.ConfigureRule(r, config.RuleCfg{Settings: map[string]any{"x": 1}})
	require.NoError(t, err)
	assert.Same(t, r, got.(*plainRule))
}

func TestConfigureRule_appliesSettings(t *testing.T) {
	r := &goodConfigurable{plainRule: plainRule{id: "TST001"}}
	got, err := checker.ConfigureRule(r, config.RuleCfg{Settings: map[string]any{"k": "v"}})
	require.NoError(t, err)
	clone := got.(*goodConfigurable)
	assert.NotSame(t, r, clone)
	assert.Equal(t, map[string]any{"k": "v"}, clone.Applied)
}

func TestConfigureRule_settingsError(t *testing.T) {
	r := &errConfigurable{plainRule: plainRule{id: "TST001"}}
	_, err := checker.ConfigureRule(r, config.RuleCfg{Settings: map[string]any{"x": 1}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "intentional settings error")
}

// --- CheckRules (thin wrapper) ---

func TestCheckRules_basic(t *testing.T) {
	f := newTestFile(t, "# Hello\n\nParagraph.\n")
	rules := []rule.Rule{&plainRule{id: "TST001"}}
	diags, errs := checker.CheckRules(f, rules, enabled("TST001"))
	assert.Empty(t, errs)
	assert.Empty(t, diags)
}

// --- CheckRulesWithIntraFile ---

func TestCheckRulesWithIntraFile_serial(t *testing.T) {
	f := newTestFile(t, "# Hello\n\nParagraph.\n")
	d := lint.Diagnostic{Line: 1, RuleID: "TST001", Message: "test"}
	rules := []rule.Rule{&diagRule{plainRule: plainRule{id: "TST001"}, diag: d}}
	diags, errs := checker.CheckRulesWithIntraFile(f, rules, enabled("TST001"), true, 1)
	assert.Empty(t, errs)
	require.Len(t, diags, 1)
	assert.Equal(t, "TST001", diags[0].RuleID)
}

func TestCheckRulesWithIntraFile_concurrent(t *testing.T) {
	f := newTestFile(t, "# Hello\n\nParagraph.\n")
	rules := []rule.Rule{
		&plainRule{id: "TST001"},
		&plainRule{id: "TST002"},
	}
	diags, errs := checker.CheckRulesWithIntraFile(f, rules, enabled("TST001", "TST002"), true, 4)
	assert.Empty(t, errs)
	assert.Empty(t, diags)
}

func TestCheckRulesWithIntraFile_nodeChecker(t *testing.T) {
	f := newTestFile(t, "# Hello\n\nParagraph.\n")
	d := lint.Diagnostic{Line: 1, RuleID: "TST001", Message: "node hit"}
	rules := []rule.Rule{&nodeCheckerRule{plainRule: plainRule{id: "TST001"}, diag: d}}
	diags, errs := checker.CheckRulesWithIntraFile(f, rules, enabled("TST001"), true, 1)
	assert.Empty(t, errs)
	assert.NotEmpty(t, diags)
}

// TestCheckRulesWithIntraFile_nodeCheckerNilAST exercises the defensive branch
// in classifySlot where a NodeChecker that is not a BlockChecker runs against a
// parse-skipped File (AST nil). The rule cannot run, so no diagnostics are emitted.
func TestCheckRulesWithIntraFile_nodeCheckerNilAST(t *testing.T) {
	f := lint.NewFileLines("doc.md", []byte("# Hello\n\nParagraph.\n"))
	f.RootDir = "."
	f.RunCache = lint.NewRunCache()
	d := lint.Diagnostic{Line: 1, RuleID: "TST001", Message: "node hit"}
	rules := []rule.Rule{&nodeCheckerRule{plainRule: plainRule{id: "TST001"}, diag: d}}
	diags, errs := checker.CheckRulesWithIntraFile(f, rules, enabled("TST001"), true, 1)
	assert.Empty(t, errs)
	assert.Empty(t, diags, "NodeChecker without BlockChecker emits nothing on nil-AST path")
}

// TestCheckRulesWithIntraFile_inlineCheckerNilAST pins the InlineChecker
// branch of classifySlot: a NodeChecker that is not a BlockChecker but
// implements rule.InlineChecker is routed to its own Check on a
// parse-skipped File (AST nil), so its diagnostics surface from the shared
// inline parse rather than being dropped.
func TestCheckRulesWithIntraFile_inlineCheckerNilAST(t *testing.T) {
	f := lint.NewFileLines("doc.md", []byte("# Hello\n\nParagraph.\n"))
	f.RootDir = "."
	f.RunCache = lint.NewRunCache()
	rules := []rule.Rule{&inlineCheckerRule{plainRule: plainRule{id: "TST001"}, message: "inline hit"}}
	diags, errs := checker.CheckRulesWithIntraFile(f, rules, enabled("TST001"), true, 1)
	assert.Empty(t, errs)
	require.Len(t, diags, 1, "InlineChecker fires through its own Check on the nil-AST path")
	assert.Equal(t, "inline hit", diags[0].Message)
}

// TestCheckRulesWithIntraFile_inlineCheckerASTPath pins that the same rule
// still runs through the shared AST walk when a tree is present.
func TestCheckRulesWithIntraFile_inlineCheckerASTPath(t *testing.T) {
	f := newTestFile(t, "# Hello\n\nParagraph.\n")
	rules := []rule.Rule{&inlineCheckerRule{plainRule: plainRule{id: "TST001"}, message: "inline hit"}}
	diags, errs := checker.CheckRulesWithIntraFile(f, rules, enabled("TST001"), true, 1)
	assert.Empty(t, errs)
	require.Len(t, diags, 1, "InlineChecker fires through the shared AST walk on the parsed path")
}

// TestCheckRulesWithIntraFile_linesCheckerNilAST pins the LinesChecker branch
// of classifySlot: a NodeChecker that is not a BlockChecker but implements
// rule.LinesChecker (the list rules) is routed to its own Check on a
// parse-skipped File (AST nil), so its diagnostics surface from f.Lines
// rather than being dropped — the dead-code defect this capability fixes.
func TestCheckRulesWithIntraFile_linesCheckerNilAST(t *testing.T) {
	f := lint.NewFileLines("doc.md", []byte("- item\n- item two\n"))
	f.RootDir = "."
	f.RunCache = lint.NewRunCache()
	rules := []rule.Rule{&linesCheckerRule{plainRule: plainRule{id: "TST002"}, message: "lines hit"}}
	diags, errs := checker.CheckRulesWithIntraFile(f, rules, enabled("TST002"), true, 1)
	assert.Empty(t, errs)
	require.Len(t, diags, 1, "LinesChecker fires through its own Check on the nil-AST path")
	assert.Equal(t, "lines hit", diags[0].Message)
}

// TestCheckRulesWithIntraFile_blockCheckerNilAST pins the BlockSpan
// dispatch: on a parse-skipped File (AST nil) the engine drives a
// rule.BlockChecker over the Layer 0 block spans, so its diagnostics
// still surface even though the shared AST walk has no tree to traverse.
func TestCheckRulesWithIntraFile_blockCheckerNilAST(t *testing.T) {
	f := lint.NewFileLines("doc.md", []byte("para\n\n---\n\nmore\n"))
	f.RootDir = "."
	f.RunCache = lint.NewRunCache()
	rules := []rule.Rule{&blockCheckerRule{plainRule: plainRule{id: "TST001"}, message: "hr seen"}}
	diags, errs := checker.CheckRulesWithIntraFile(f, rules, enabled("TST001"), true, 1)
	assert.Empty(t, errs)
	require.Len(t, diags, 1, "block checker fires on the nil-AST path")
	assert.Equal(t, 3, diags[0].Line, "thematic break at line 3")
}

// TestCheckRulesWithIntraFile_blockCheckerASTPath pins that the same
// rule still runs through the shared AST walk when a tree is present, so
// the migration adds the BlockSpan seam without disturbing the parsed
// path.
func TestCheckRulesWithIntraFile_blockCheckerASTPath(t *testing.T) {
	f := newTestFile(t, "para\n\n---\n\nmore\n")
	rules := []rule.Rule{&blockCheckerRule{plainRule: plainRule{id: "TST001"}, message: "hr seen"}}
	diags, errs := checker.CheckRulesWithIntraFile(f, rules, enabled("TST001"), true, 1)
	assert.Empty(t, errs)
	require.Len(t, diags, 1, "block checker fires on the AST path")
	assert.Equal(t, 1, diags[0].Line)
}

func TestCheckRulesWithIntraFile_settingsError(t *testing.T) {
	f := newTestFile(t, "# Hello\n")
	rules := []rule.Rule{&errConfigurable{plainRule: plainRule{id: "TST001"}}}
	eff := map[string]config.RuleCfg{
		"TST001": {Enabled: true, Settings: map[string]any{"x": 1}},
	}
	diags, errs := checker.CheckRulesWithIntraFile(f, rules, eff, true, 1)
	assert.Empty(t, diags)
	require.Len(t, errs, 1)
	assert.Contains(t, errs[0].Error(), "intentional settings error")
}

func TestCheckRulesWithIntraFile_disabledRule(t *testing.T) {
	f := newTestFile(t, "# Hello\n")
	d := lint.Diagnostic{Line: 1, RuleID: "TST001", Message: "should not appear"}
	rules := []rule.Rule{&diagRule{plainRule: plainRule{id: "TST001"}, diag: d}}
	eff := map[string]config.RuleCfg{"TST001": {Enabled: false}}
	diags, errs := checker.CheckRulesWithIntraFile(f, rules, eff, true, 1)
	assert.Empty(t, errs)
	assert.Empty(t, diags)
}

func TestCheckRulesWithIntraFile_AdjustsLineOffset(t *testing.T) {
	// Front matter is 3 lines → LineOffset=3. Rule reports at body-relative line 1.
	// AdjustDiagnostics must shift it to absolute line 4.
	src := "---\ntitle: x\n---\n# Heading\n"
	f, err := lint.NewFileFromSource("doc.md", []byte(src), true)
	require.NoError(t, err)
	f.RootDir = "."
	f.RunCache = lint.NewRunCache()

	d := lint.Diagnostic{Line: 1, RuleID: "TST001", Message: "raw line"}
	rules := []rule.Rule{&diagRule{plainRule: plainRule{id: "TST001"}, diag: d}}
	diags, errs := checker.CheckRulesWithIntraFile(f, rules, enabled("TST001"), true, 1)
	assert.Empty(t, errs)
	require.Len(t, diags, 1)
	assert.Equal(t, 4, diags[0].Line, "AdjustDiagnostics must shift body-relative line 1 to absolute line 4")
}

func TestCheckRulesWithIntraFile_FiltersGeneratedRanges(t *testing.T) {
	// Rule emits diags at lines 2 and 3; lines 3-4 are a generated range.
	// Only the line-2 diagnostic must survive FilterGeneratedDiags.
	f := newTestFile(t, "line1\nline2\nline3\nline4\n")
	f.GeneratedRanges = []lint.LineRange{{From: 3, To: 4}}

	d2 := lint.Diagnostic{Line: 2, RuleID: "TST001", Message: "keep"}
	d3 := lint.Diagnostic{Line: 3, RuleID: "TST002", Message: "drop"}
	rules := []rule.Rule{
		&diagRule{plainRule: plainRule{id: "TST001"}, diag: d2},
		&diagRule{plainRule: plainRule{id: "TST002"}, diag: d3},
	}
	diags, errs := checker.CheckRulesWithIntraFile(f, rules, enabled("TST001", "TST002"), true, 1)
	assert.Empty(t, errs)
	require.Len(t, diags, 1, "only line-2 diagnostic should survive GeneratedRanges filter")
	assert.Equal(t, 2, diags[0].Line)
}

// --- FilterGeneratedDiags ---

func TestFilterGeneratedDiags_noRanges(t *testing.T) {
	diags := []lint.Diagnostic{{Line: 5, RuleID: "TST001"}}
	out := checker.FilterGeneratedDiags(diags, nil)
	assert.Equal(t, diags, out)
}

func TestFilterGeneratedDiags_removesInsideRange(t *testing.T) {
	diags := []lint.Diagnostic{
		{Line: 3, RuleID: "TST001"},
		{Line: 7, RuleID: "TST002"},
		{Line: 10, RuleID: "TST003"},
	}
	ranges := []lint.LineRange{{From: 5, To: 8}}
	out := checker.FilterGeneratedDiags(diags, ranges)
	require.Len(t, out, 2)
	assert.Equal(t, 3, out[0].Line)
	assert.Equal(t, 10, out[1].Line)
}

func TestFilterGeneratedDiags_keepOutsideRange(t *testing.T) {
	diags := []lint.Diagnostic{{Line: 1, RuleID: "TST001"}}
	ranges := []lint.LineRange{{From: 5, To: 8}}
	out := checker.FilterGeneratedDiags(diags, ranges)
	require.Len(t, out, 1)
}

// --- PopulateSourceContext ---

func TestPopulateSourceContext_validLine(t *testing.T) {
	f := newTestFile(t, "line1\nline2\nline3\n")
	diags := []lint.Diagnostic{{Line: 2, RuleID: "TST001"}}
	checker.PopulateSourceContext(f, diags, 1)
	assert.NotNil(t, diags[0].SourceLines)
	assert.Equal(t, 1, diags[0].SourceStartLine)
}

func TestPopulateSourceContext_outOfBoundLine(t *testing.T) {
	f := newTestFile(t, "# Hello\n")
	diags := []lint.Diagnostic{{Line: 0, RuleID: "TST001"}}
	checker.PopulateSourceContext(f, diags, 2)
	assert.Nil(t, diags[0].SourceLines)
}

func TestPopulateSourceContext_emptyTrailingLine(t *testing.T) {
	// Source ending with \n produces an empty trailing element in f.Lines;
	// PopulateSourceContext must not include it in context windows.
	f := newTestFile(t, "line1\nline2\n")
	diags := []lint.Diagnostic{{Line: 1, RuleID: "TST001"}}
	checker.PopulateSourceContext(f, diags, 0)
	require.Len(t, diags[0].SourceLines, 1)
	assert.Equal(t, "line1", diags[0].SourceLines[0])
}

func TestCheckRulesWithIntraFile_concurrent_withNodeChecker(t *testing.T) {
	// Mix a NodeChecker (check==nil slot) and a plain rule (check!=nil slot)
	// with intraFileCap>1 so the concurrent branch hits the check==nil skip.
	f := newTestFile(t, "# Hello\n\nParagraph.\n")
	d := lint.Diagnostic{Line: 1, RuleID: "TST002", Message: "node"}
	rules := []rule.Rule{
		&plainRule{id: "TST001"},
		&nodeCheckerRule{plainRule: plainRule{id: "TST002"}, diag: d},
	}
	diags, errs := checker.CheckRulesWithIntraFile(f, rules, enabled("TST001", "TST002"), true, 4)
	assert.Empty(t, errs)
	assert.NotEmpty(t, diags)
}

// --- kind-scoped NodeChecker dispatch ---

// visit records one CheckNode invocation.
type visit struct {
	kind     ast.NodeKind
	entering bool
}

// kindScopedRule is a NodeChecker that declares interest in a fixed
// set of node kinds and records every CheckNode call it receives.
type kindScopedRule struct {
	plainRule
	kinds  []ast.NodeKind
	visits []visit
}

func (r *kindScopedRule) Check(_ *lint.File) []lint.Diagnostic { return nil }
func (r *kindScopedRule) CheckNode(n ast.Node, entering bool, _ *lint.File) []lint.Diagnostic {
	r.visits = append(r.visits, visit{kind: n.Kind(), entering: entering})
	if entering && n.Kind() == ast.KindHeading {
		return []lint.Diagnostic{{Line: 1, RuleID: r.id, Message: "heading"}}
	}
	return nil
}
func (r *kindScopedRule) EnteringKinds() []ast.NodeKind { return r.kinds }

var _ rule.KindScopedChecker = (*kindScopedRule)(nil)

func TestKindScopedChecker_DispatchedOnlyForDeclaredKinds(t *testing.T) {
	f := newTestFile(t, "# Hello\n\nParagraph one.\n\n- item\n")
	r := &kindScopedRule{
		plainRule: plainRule{id: "TST001"},
		kinds:     []ast.NodeKind{ast.KindHeading},
	}
	diags, errs := checker.CheckRulesWithIntraFile(f, []rule.Rule{r}, enabled("TST001"), true, 1)
	assert.Empty(t, errs)
	require.Len(t, diags, 1)
	require.NotEmpty(t, r.visits)
	for _, v := range r.visits {
		assert.Equal(t, ast.KindHeading, v.kind,
			"kind-scoped rule must only see its declared kinds")
		assert.True(t, v.entering,
			"kind-scoped rule must only see entering visits")
	}
}

func TestKindScopedChecker_MixedWithGenericNodeChecker(t *testing.T) {
	// A kind-scoped rule and a plain NodeChecker run in one walk; the
	// generic rule still sees every node while the scoped one is
	// filtered, and both contribute diagnostics in rule order.
	f := newTestFile(t, "# Hello\n\nParagraph.\n")
	scoped := &kindScopedRule{
		plainRule: plainRule{id: "TST001"},
		kinds:     []ast.NodeKind{ast.KindHeading},
	}
	d := lint.Diagnostic{Line: 1, RuleID: "TST002", Message: "node hit"}
	generic := &nodeCheckerRule{plainRule: plainRule{id: "TST002"}, diag: d}
	diags, errs := checker.CheckRulesWithIntraFile(
		f, []rule.Rule{scoped, generic}, enabled("TST001", "TST002"), true, 1)
	assert.Empty(t, errs)
	require.NotEmpty(t, diags)
	assert.Equal(t, "TST001", diags[0].RuleID,
		"rule-order grouping must survive kind-scoped dispatch")
	sawGeneric := false
	for _, dg := range diags {
		if dg.RuleID == "TST002" {
			sawGeneric = true
		}
	}
	assert.True(t, sawGeneric)
}

func TestKindScopedChecker_MultipleKindsShareOneBucketEntry(t *testing.T) {
	f := newTestFile(t, "# Hello\n\nSome *emphasis* text.\n")
	r := &kindScopedRule{
		plainRule: plainRule{id: "TST001"},
		kinds:     []ast.NodeKind{ast.KindHeading, ast.KindEmphasis},
	}
	_, errs := checker.CheckRulesWithIntraFile(f, []rule.Rule{r}, enabled("TST001"), true, 1)
	assert.Empty(t, errs)
	kindsSeen := map[ast.NodeKind]bool{}
	for _, v := range r.visits {
		kindsSeen[v.kind] = true
	}
	assert.True(t, kindsSeen[ast.KindHeading])
	assert.True(t, kindsSeen[ast.KindEmphasis])
	assert.Len(t, kindsSeen, 2)
}

func TestPopulateSourceContext_NoDiagnosticsIsNoOp(t *testing.T) {
	// The empty fast path must not build the per-file line-string
	// cache: most files produce no diagnostics.
	f := newTestFile(t, "line1\nline2\n")
	checker.PopulateSourceContext(f, nil, 2)
	checker.PopulateSourceContext(f, []lint.Diagnostic{}, 2)
}

// leavingDiagRule is a generic (kind-less) NodeChecker that emits on
// the leaving visit of the document node, pinning that the generic
// path still delivers exit visits and collects their diagnostics.
type leavingDiagRule struct{ plainRule }

func (r *leavingDiagRule) Check(_ *lint.File) []lint.Diagnostic { return nil }
func (r *leavingDiagRule) CheckNode(n ast.Node, entering bool, _ *lint.File) []lint.Diagnostic {
	if !entering && n.Kind() == ast.KindDocument {
		return []lint.Diagnostic{{Line: 1, RuleID: r.id, Message: "leaving"}}
	}
	return nil
}

func TestGenericNodeChecker_LeavingVisitDiagnosticsCollected(t *testing.T) {
	f := newTestFile(t, "# Hello\n\nParagraph.\n")
	r := &leavingDiagRule{plainRule: plainRule{id: "TST001"}}
	diags, errs := checker.CheckRulesWithIntraFile(f, []rule.Rule{r}, enabled("TST001"), true, 1)
	assert.Empty(t, errs)
	require.Len(t, diags, 1)
	assert.Equal(t, "leaving", diags[0].Message)
}
