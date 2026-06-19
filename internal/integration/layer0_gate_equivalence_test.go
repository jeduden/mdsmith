package integration

import (
	"os"
	"path/filepath"
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

// TestLayer0Gate_LineCapableCorpusEquivalence is the gate-unification
// (plan 2606171400) corpus guard. It enables the static Layer 0 rules plus
// line-length — a rule that is NOT in the rulelayer Layer 0 set but reports
// rule.LineCapable under its default config — and asserts the parse-skip
// run matches the full-parse run across the repository corpus. With the
// unified gate, line-length no longer forces the parse; its diagnostics
// must come out byte-identical from the ClassifyLines projection on the
// parse-skipped File, on real content (over-length lines, code, the lot).
func TestLayer0Gate_LineCapableCorpusEquivalence(t *testing.T) {
	root := repoRoot(t)
	files := collectMarkdownCorpus(t, root)
	require.NotEmpty(t, files)

	cfg := layer0OnlyConfigForCorpus()
	cfg.Rules["line-length"] = config.RuleCfg{Enabled: true}

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
	require.NotEmpty(t, parsed, "line-length must fire somewhere in the corpus")
	assert.Equal(t, diagKeys(parsed), diagKeys(skipped),
		"line-capable parse-skip must match the full-parse run across the corpus")
}

// TestLayer0Gate_ParityCorpusDiagnosticsEquivalence is plan 2606171258's
// acceptance criterion 2: the parse-skip run matches the full-parse run
// across the repository corpus with the FULL parity rule set enabled — not
// just the static Layer 0 subset. It loads the built-in `parity` convention
// exactly as `mdsmith check -c parity` does (a `convention: parity` file
// merged onto the registry defaults) and diffs the two runs. With every
// parity rule now Layer 0 (MDS066 commands-show-output was the last to
// migrate), the gate engages for every directive/list/quote-free file, so
// this exercises the parse-skip path on real corpus content for the whole
// set the benchmark runs.
func TestLayer0Gate_ParityCorpusDiagnosticsEquivalence(t *testing.T) {
	root := repoRoot(t)
	files := collectMarkdownCorpus(t, root)
	require.NotEmpty(t, files)

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".mdsmith.yml")
	require.NoError(t, os.WriteFile(cfgPath, []byte("convention: parity\n"), 0o644))
	loaded, err := config.Load(cfgPath)
	require.NoError(t, err)
	cfg := config.Merge(config.Defaults(), loaded)

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
	require.NotEmpty(t, parsed, "the parity set must fire somewhere in the corpus")
	assert.Equal(t, diagKeys(parsed), diagKeys(skipped),
		"parity parse-skip diagnostics must match the full-parse run across the corpus")
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
