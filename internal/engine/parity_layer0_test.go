package engine

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
)

// parityConfig resolves the built-in `mado-parity` convention the way
// the CLI does: parse a `.mdsmith.yml` selecting it, then merge it onto
// the defaults so the convention preset disables the rules mado does
// not run while the rest stay enabled.
//
// mado-parity is the representative parity convention for the
// parse-skip guarantee: of the four <linter>-parity sets it is the
// only one whose entire enabled rule set is parse-skip-safe. The
// others enable cross-file-reference-integrity (MDS027) — and, for
// rumdl/markdownlint, required-structure (MDS020) — which still need
// the goldmark AST, so they do not skip the parse.
func parityConfig(t *testing.T) *config.Config {
	t.Helper()
	loaded, err := config.ParseBytes([]byte("convention: mado-parity\n"))
	require.NoError(t, err)
	return config.Merge(config.Defaults(), loaded)
}

// TestParityConvention_AllEnabledRulesSkipSafe is the parent plan's
// acceptance criterion 1: every parity-enabled rule resolves to a parse-skip
// layer (rulelayer Layer 0 or, for line-length, configured rule.LineCapable).
// MDS066 (commands-show-output) was the lone holdout — a "B-prose-only" rule
// the static category withheld from Layer 0 — until rulelayer.nilASTBackable
// began promoting the nil-AST-safe "B-prose-only" category. If a future parity
// rule regrows an AST dependency, this fails and names it.
func TestParityConvention_AllEnabledRulesSkipSafe(t *testing.T) {
	cfg := parityConfig(t)
	eff := config.Effective(cfg, "doc.md", nil, nil)

	enabled := 0
	var notSafe []string
	for _, rl := range rule.All() {
		c, ok := eff[rl.Name()]
		if !ok || !c.Enabled {
			continue
		}
		enabled++
		if !allEnabledRulesSkipSafe([]rule.Rule{rl}, eff) {
			notSafe = append(notSafe, rl.ID()+" "+rl.Name())
		}
	}
	sort.Strings(notSafe)
	require.NotZero(t, enabled, "parity must enable some rules or the check is vacuous")
	assert.Empty(t, notSafe, "every parity-enabled rule must be parse-skip-safe")
}

// TestParityConvention_SkipsParse is the parent plan's acceptance criterion
// 3: with the gate on, `mdsmith check -c parity` skips the goldmark parse for
// a benchmark-style file (no directive, no list, no block quote — the neutral
// corpus shape). The probe records whether the File it received had a nil AST.
func TestParityConvention_SkipsParse(t *testing.T) {
	withLayer0Skip(t, true)
	// Headings, prose, and a fenced code block of shell prompts so MDS066
	// (commands-show-output) is exercised on the parse-skipped path.
	dir, path := writeDoc(t, "# Title\n\nSome prose paragraph.\n\n```sh\n$ build\n$ test\n```\n")

	probe := &astProbeRule{}
	cfg := parityConfig(t)
	cfg.Rules[probe.Name()] = config.RuleCfg{Enabled: true}

	r := &Runner{
		Config:           cfg,
		Rules:            append(rule.All(), probe),
		StripFrontMatter: true,
		RootDir:          dir,
	}
	res := r.Run([]string{path})
	require.Empty(t, res.Errors)
	assert.True(t, probe.sawNilAST,
		"a parity run on a benchmark-shaped file must skip the parse")

	firedMDS066 := false
	for _, d := range res.Diagnostics {
		if d.RuleID == "MDS066" {
			firedMDS066 = true
			break
		}
	}
	assert.True(t, firedMDS066,
		"MDS066 must fire on the parse-skipped path so the skip is exercised, not just observed")
}

// TestParityConvention_DiagnosticsMatchFullParse is the equivalence guarantee
// for the parity parse-skip: the diagnostics a parity run produces with the
// gate on (nil AST) are identical to the same run with the gate off (full
// parse). The comparison is on the full diagnostic slice — rule id, line,
// column, message — not just the count, so a regression that moves MDS066's
// diagnostic or swaps it for another rule's is caught. The fixture trips
// MDS066 so the comparison is not vacuous.
func TestParityConvention_DiagnosticsMatchFullParse(t *testing.T) {
	dir, path := writeDoc(t, "# Title\n\nProse line.\n\n```sh\n$ make\n$ make test\n```\n")
	cfg := parityConfig(t)

	run := func(skip bool) []lint.Diagnostic {
		withLayer0Skip(t, skip)
		r := &Runner{
			Config:           cfg,
			Rules:            rule.All(),
			StripFrontMatter: true,
			RootDir:          dir,
		}
		res := r.Run([]string{path})
		require.Empty(t, res.Errors)
		return res.Diagnostics
	}

	skipped := run(true)
	parsed := run(false)
	require.NotEmpty(t, parsed, "MDS066 must fire so the comparison is not vacuous")
	assert.Equal(t, parsed, skipped,
		"parity parse-skip must produce the same diagnostics as a full parse")
}
