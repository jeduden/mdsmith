package integration

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/engine"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/jeduden/mdsmith/internal/rulelayer"
)

// TestLayer0Gate_CorpusDiagnosticsEquivalence is the end-to-end gate
// guard: it lints every Markdown file in the repository corpus with a
// Layer-0-only config twice — once with the parse-skip gate ON
// (MDSMITH_LAYER0_SKIP) and once OFF — and asserts the diagnostics are
// identical. The line-set harness (layer0_equivalence_test.go) proves the
// projections match; this proves the projections actually drive identical
// rule output through the real engine, including the parse-skipped nil-AST
// File. It is the test that would catch a Layer 0 rule silently diverging
// on a skipped file, on real corpus content rather than a synthetic doc.
//
// Only Layer 0 rules are enabled, so the gate engages for every
// directive-free file; a file carrying a `<?` directive stays on the parse
// path under both runs, which is itself the equivalence we want (identical
// output regardless of which path a file takes).
func TestLayer0Gate_CorpusDiagnosticsEquivalence(t *testing.T) {
	root := repoRoot(t)
	files := collectMarkdownCorpus(t, root)
	require.NotEmpty(t, files)

	cfg := layer0OnlyConfigForCorpus()

	run := func(skip bool) []lint.Diagnostic {
		if skip {
			t.Setenv("MDSMITH_LAYER0_SKIP", "1")
		} else {
			t.Setenv("MDSMITH_LAYER0_SKIP", "")
		}
		r := &engine.Runner{
			Config:           cfg,
			Rules:            rule.All(),
			StripFrontMatter: true,
			RootDir:          root,
		}
		return r.Run(files).Diagnostics
	}

	skipped := run(true)
	parsed := run(false)
	assert.Equal(t, diagKeys(parsed), diagKeys(skipped),
		"parse-skip diagnostics must match the full-parse run across the corpus")
}

// layer0OnlyConfigForCorpus enables only the Layer 0 rules and disables
// every other rule, so the gate is eligible for every directive-free file.
func layer0OnlyConfigForCorpus() *config.Config {
	cfg := config.Defaults()
	for _, rl := range rule.All() {
		if rulelayer.IsLayer0(rl.ID()) {
			cfg.Rules[rl.Name()] = config.RuleCfg{Enabled: true}
		} else {
			cfg.Rules[rl.Name()] = config.RuleCfg{Enabled: false}
		}
	}
	return cfg
}

// diagKeys reduces diagnostics to comparable tuples (file, line, column,
// rule, message) so two runs compare on observable output alone.
func diagKeys(diags []lint.Diagnostic) [][5]string {
	out := make([][5]string, 0, len(diags))
	for _, d := range diags {
		out = append(out, [5]string{
			d.File, strconv.Itoa(d.Line), strconv.Itoa(d.Column), d.RuleID, d.Message,
		})
	}
	return out
}
