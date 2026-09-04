package integration

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jeduden/mdsmith/internal/checker"
	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/jeduden/mdsmith/internal/rulelayer"
	_ "github.com/jeduden/mdsmith/internal/rules/all"
)

// TestSkipSafeNodeCheckersHaveNilASTRoute pins the dispatch invariant a
// rule silently violates when it becomes a rule.NodeChecker without also
// declaring how it serves the parse-skipped path.
//
// checker.classifySlot routes a NodeChecker on a nil-AST File to the block
// dispatch (rule.BlockChecker) or to the rule's own Check (rule.InlineChecker
// / rule.LinesChecker), and gives it NO slot at all otherwise — the rule
// produces nothing. Meanwhile the parse-skip gate admits a file as soon as
// every enabled rule is rulelayer.IsLayer0 or rule.LineCapable. A rule that
// satisfies the gate but has no nil-AST route therefore disappears on
// exactly the files the gate skips, with no error and no failing corpus
// gate unless some corpus file happens to violate it.
//
// Every rule that can be reached on a skipped file must declare one of the
// three routes. A rule that is not skip-safe never sees a nil-AST File, so
// it needs none.
func TestSkipSafeNodeCheckersHaveNilASTRoute(t *testing.T) {
	var checked int
	for _, rl := range rule.All() {
		nc, isNode := rl.(rule.NodeChecker)
		if !isNode {
			continue
		}
		lc, isLineCapable := rl.(rule.LineCapable)
		skipSafe := rulelayer.IsLayer0(rl.ID()) || (isLineCapable && lc.LineCapable())
		if !skipSafe {
			continue
		}
		checked++

		t.Run(rl.ID(), func(t *testing.T) {
			_, isBlock := nc.(rule.BlockChecker)
			ic, isInline := nc.(rule.InlineChecker)
			lnc, isLines := nc.(rule.LinesChecker)
			assert.True(t,
				isBlock || (isInline && ic.InlineCapable()) || (isLines && lnc.LinesCapable()),
				"%s (%s) is a NodeChecker the parse-skip gate admits, so it must implement "+
					"rule.BlockChecker, rule.InlineChecker or rule.LinesChecker; without one "+
					"checker.classifySlot drops it on every parse-skipped file",
				rl.ID(), rl.Name())
		})
	}
	require.NotZero(t, checked, "expected at least one skip-safe NodeChecker rule")
}

// headingRuleNilASTCases are documents whose diagnostics must survive the
// parse skip for the two heading rules converted to kind-scoped node
// checkers (MDS003, MDS005). Each is directive-free and code-block-free so
// the production gate would admit it.
var headingRuleNilASTCases = map[string]string{
	"first heading below level 1": "### Third only\n\nbody text\n",
	"skipped level":               "# Title\n\n### Skipped\n\nbody text\n",
	"duplicate heading":           "# Title\n\n## Same\n\ntext\n\n## Same\n\nmore text\n",
	"setext first heading":        "Sub\n---\n\nbody text\n",
	"clean document":              "# Title\n\n## One\n\ntext\n\n## Two\n\nmore text\n",
}

// TestHeadingRules_NilASTDispatchEquivalence drives the two heading rules
// through checker.CheckRules — the real dispatch, not each rule's Check —
// on a parsed File and on a parse-skipped File, and requires identical
// diagnostics. The rule-level Check equivalence harnesses cannot catch a
// routing regression, because they call Check directly and so bypass the
// classifySlot decision that drops an unrouted NodeChecker.
func TestHeadingRules_NilASTDispatchEquivalence(t *testing.T) {
	rules := rulesByID(t, "MDS003", "MDS005")
	eff := map[string]config.RuleCfg{
		"heading-increment":     {Enabled: true},
		"no-duplicate-headings": {Enabled: true},
	}

	for name, src := range headingRuleNilASTCases {
		t.Run(name, func(t *testing.T) {
			body := []byte(src)

			astFile, err := lint.NewFile("t.md", body)
			require.NoError(t, err)
			parsed, errs := checker.CheckRules(astFile, rules, eff)
			require.Empty(t, errs)

			lineFile := lint.NewFileLines("t.md", body)
			require.Nil(t, lineFile.AST, "the line-only File must have no AST")
			skipped, errs := checker.CheckRules(lineFile, rules, eff)
			require.Empty(t, errs)

			assert.Equal(t, diagKeys(parsed), diagKeys(skipped),
				"the parse-skipped dispatch must reproduce the parsed dispatch exactly")
		})
	}
}

// TestHeadingRules_NilASTDispatchStillFires guards the equivalence test
// above against passing vacuously: the violating documents must actually
// produce diagnostics on the parse-skipped path.
func TestHeadingRules_NilASTDispatchStillFires(t *testing.T) {
	rules := rulesByID(t, "MDS003", "MDS005")
	eff := map[string]config.RuleCfg{
		"heading-increment":     {Enabled: true},
		"no-duplicate-headings": {Enabled: true},
	}

	for _, tc := range []struct{ name, src, wantID string }{
		{"MDS003", headingRuleNilASTCases["first heading below level 1"], "MDS003"},
		{"MDS005", headingRuleNilASTCases["duplicate heading"], "MDS005"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := lint.NewFileLines("t.md", []byte(tc.src))
			diags, errs := checker.CheckRules(f, rules, eff)
			require.Empty(t, errs)
			require.Len(t, diags, 1)
			assert.Equal(t, tc.wantID, diags[0].RuleID)
		})
	}
}

// rulesByID returns the registered rule singletons for the given IDs, in
// the order requested, failing the test if one is missing.
func rulesByID(t *testing.T, ids ...string) []rule.Rule {
	t.Helper()
	byID := make(map[string]rule.Rule, len(ids))
	for _, rl := range rule.All() {
		byID[rl.ID()] = rl
	}
	out := make([]rule.Rule, 0, len(ids))
	for _, id := range ids {
		rl, ok := byID[id]
		require.Truef(t, ok, "rule %s is not registered", id)
		out = append(out, rl)
	}
	return out
}
