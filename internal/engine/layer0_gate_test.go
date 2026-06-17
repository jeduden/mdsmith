package engine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
)

// withLayer0Skip flips the package gate toggle for the duration of a test
// and restores it after, so the gate can be exercised without mutating the
// process environment (which is read only at init).
func withLayer0Skip(t *testing.T, on bool) {
	t.Helper()
	prev := layer0SkipOverride
	layer0SkipOverride = &on
	t.Cleanup(func() { layer0SkipOverride = prev })
}

// layer0OnlyConfig is a config whose only enabled rules resolve to Layer 0
// (no-trailing-spaces, no-hard-tabs, max-file-length); every other rule is
// disabled, so a run over it should skip the goldmark parse.
func layer0OnlyConfig() *config.Config {
	cfg := config.Defaults()
	for name := range cfg.Rules {
		cfg.Rules[name] = config.RuleCfg{Enabled: false}
	}
	for _, name := range []string{"no-trailing-spaces", "no-hard-tabs", "max-file-length"} {
		cfg.Rules[name] = config.RuleCfg{Enabled: true}
	}
	return cfg
}

func writeDoc(t *testing.T, body string) (dir, path string) {
	t.Helper()
	dir = t.TempDir()
	path = filepath.Join(dir, "doc.md")
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	return dir, path
}

// TestLayer0Gate_SkipsParseForLayer0OnlyConfig proves the gate's headline
// claim: with the toggle on and a Layer-0-only config, the engine never
// builds the AST. The probe is a custom rule that records whether the File
// it received had a nil AST.
func TestLayer0Gate_SkipsParseForLayer0OnlyConfig(t *testing.T) {
	withLayer0Skip(t, true)
	dir, path := writeDoc(t, "# Title\n\nA line with trailing space \n")

	probe := &astProbeRule{}
	cfg := layer0OnlyConfig()
	cfg.Rules[probe.Name()] = config.RuleCfg{Enabled: true}

	r := &Runner{
		Config:           cfg,
		Rules:            append(rule.All(), probe),
		StripFrontMatter: true,
		RootDir:          dir,
	}
	res := r.Run([]string{path})
	require.Empty(t, res.Errors)
	assert.True(t, probe.sawNilAST, "expected the parse to be skipped (nil AST)")
}

// TestLayer0Gate_LineCapableRuleSkipsParse proves the gate unification
// (plan 2606171400): line-length (MDS001) is not in the static rulelayer
// Layer-0 set, but with no per-heading limit it reports rule.LineCapable,
// so it lints from f.Lines via the ClassifyLines projection the skip File
// carries. The gate must therefore skip the parse for a line-capable rule,
// where the static-only gate forced it.
func TestLayer0Gate_LineCapableRuleSkipsParse(t *testing.T) {
	withLayer0Skip(t, true)
	dir, path := writeDoc(t, "# Title\n\nbody\n")

	probe := &astProbeRule{}
	cfg := layer0OnlyConfig()
	cfg.Rules["line-length"] = config.RuleCfg{Enabled: true}
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
		"line-capable rule (line-length, no heading limit) must skip the parse")
}

// TestLayer0Gate_LineCapableDiagnosticsMatchFullParse is the equivalence
// guarantee for the unification: a config that adds the line-capable
// line-length rule produces identical diagnostics with the gate on (nil
// AST, ClassifyLines) and off (full parse). The fixture has an over-length
// line so line-length actually fires on both paths.
func TestLayer0Gate_LineCapableDiagnosticsMatchFullParse(t *testing.T) {
	longLine := "This is a deliberately long prose line that exceeds the default eighty column line-length budget.\n"
	dir, path := writeDoc(t, "# Title\n\n"+longLine)
	cfg := layer0OnlyConfig()
	cfg.Rules["line-length"] = config.RuleCfg{Enabled: true}

	run := func(skip bool) []lint.Diagnostic {
		withLayer0Skip(t, skip)
		r := &Runner{
			Config:           cfg,
			Rules:            rule.All(),
			StripFrontMatter: true,
			RootDir:          dir,
		}
		return r.Run([]string{path}).Diagnostics
	}

	skipped := run(true)
	parsed := run(false)
	require.NotEmpty(t, parsed, "line-length must fire so the comparison is not vacuous")
	assert.Equal(t, parsed, skipped,
		"line-capable parse-skip must match the full parse")
}

// TestLayer0Gate_ParsesWhenAnASTRuleIsEnabled confirms the gate stands
// down when any enabled rule needs the AST: line-length WITH a per-heading
// limit is not line-capable (it must walk headings), so the File must carry
// a parsed tree. (Plain line-length, with no heading limit, is line-capable
// and skips — see TestLayer0Gate_LineCapableRuleSkipsParse.)
func TestLayer0Gate_ParsesWhenAnASTRuleIsEnabled(t *testing.T) {
	withLayer0Skip(t, true)
	dir, path := writeDoc(t, "# Title\n\nbody\n")

	probe := &astProbeRule{}
	cfg := layer0OnlyConfig()
	cfg.Rules["line-length"] = config.RuleCfg{
		Enabled:  true,
		Settings: map[string]any{"heading-max": 60},
	}
	cfg.Rules[probe.Name()] = config.RuleCfg{Enabled: true}

	r := &Runner{
		Config:           cfg,
		Rules:            append(rule.All(), probe),
		StripFrontMatter: true,
		RootDir:          dir,
	}
	res := r.Run([]string{path})
	require.Empty(t, res.Errors)
	assert.False(t, probe.sawNilAST, "AST rule enabled: parse must not be skipped")
}

// TestLayer0Gate_ParsesWhenDirectivePresent confirms the gate stands down
// when the source carries a `<?` directive, since generated-section
// suppression walks the AST.
func TestLayer0Gate_ParsesWhenDirectivePresent(t *testing.T) {
	withLayer0Skip(t, true)
	dir, path := writeDoc(t, "# Title\n\n<?toc?>\n")

	probe := &astProbeRule{}
	cfg := layer0OnlyConfig()
	cfg.Rules[probe.Name()] = config.RuleCfg{Enabled: true}

	r := &Runner{
		Config:           cfg,
		Rules:            append(rule.All(), probe),
		StripFrontMatter: true,
		RootDir:          dir,
	}
	res := r.Run([]string{path})
	require.Empty(t, res.Errors)
	assert.False(t, probe.sawNilAST, "directive present: parse must not be skipped")
}

// TestLayer0Gate_OffByDefault confirms that without the toggle the parse
// always runs, even for a Layer-0-only config.
func TestLayer0Gate_OffByDefault(t *testing.T) {
	withLayer0Skip(t, false)
	dir, path := writeDoc(t, "# Title\n\ntrailing \n")

	probe := &astProbeRule{}
	cfg := layer0OnlyConfig()
	cfg.Rules[probe.Name()] = config.RuleCfg{Enabled: true}

	r := &Runner{
		Config:           cfg,
		Rules:            append(rule.All(), probe),
		StripFrontMatter: true,
		RootDir:          dir,
	}
	res := r.Run([]string{path})
	require.Empty(t, res.Errors)
	assert.False(t, probe.sawNilAST, "gate off: parse must run")
}

// TestLayer0Gate_DiagnosticsMatchFullParse is the equivalence guarantee:
// the diagnostics a Layer-0-only run produces with the gate on are
// identical to the same run with the gate off (full parse). The fixture is
// code-free (no fence, tab, or four-space indent) so the gate actually skips
// the parse — a file with a tab or fence would force the parse under the
// SourceMayHaveCodeBlock guard and make this comparison vacuous.
func TestLayer0Gate_DiagnosticsMatchFullParse(t *testing.T) {
	dir, path := writeDoc(t, "# Title\n\nA line with trailing space \nmore trailing text \n")
	cfg := layer0OnlyConfig()

	run := func(skip bool) []lint.Diagnostic {
		withLayer0Skip(t, skip)
		r := &Runner{
			Config:           cfg,
			Rules:            rule.All(),
			StripFrontMatter: true,
			RootDir:          dir,
		}
		return r.Run([]string{path}).Diagnostics
	}

	skipped := run(true)
	parsed := run(false)
	assert.Equal(t, parsed, skipped,
		"Layer 0 parse-skip must produce identical diagnostics to a full parse")
}

// TestLayer0Gate_CodeBlockSkipsParse proves the parse-skip now engages on
// code-bearing files. The old SourceMayHaveCodeBlock guard bailed on any
// code marker (here a hard tab — also a fence or a four-space indent) to
// avoid a CodeBlockLines divergence; the skip File now carries the flat
// ClassifyLines projection (gated byte-identical to the AST on code-bearing
// corpus files), so a code marker no longer forces the parse when every
// enabled rule is Layer 0.
func TestLayer0Gate_CodeBlockSkipsParse(t *testing.T) {
	withLayer0Skip(t, true)
	dir, path := writeDoc(t, "# Title\n\nA line with a hard\ttab.\n")

	probe := &astProbeRule{}
	cfg := layer0OnlyConfig()
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
		"a code-bearing source with only Layer 0 rules must skip the parse")
}

// TestLayer0Gate_CodeSpanRuleSkipsParse proves the Layer 1 code-span fix
// closed the old code-span soundness gap: MDS054
// (no-undefined-reference-labels) is "A-no-skipping" and reads code-span
// ranges. Those ranges used to go empty without a parse, so the rule was
// forced to AST; the shared run-grouped inline parse (lint.InlineBlocks) now
// reproduces them on the nil-AST path, so the rule resolves to Layer 0 and
// skips the whole-document parse. The `[undefined]` bracket text sits inside
// a code span, which that parse identifies, so the parse-skipped run reports
// nothing — byte-identical to the full-parse run.
func TestLayer0Gate_CodeSpanRuleSkipsParse(t *testing.T) {
	withLayer0Skip(t, true)
	dir, path := writeDoc(t, "# T\n\nUse `[undefined]` inline.\n")

	probe := &astProbeRule{}
	cfg := layer0OnlyConfig()
	cfg.Rules["no-undefined-reference-labels"] = config.RuleCfg{Enabled: true}
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
		"a code-span-consuming rule backed by the inline index must skip the parse")
	// The bracket text sits inside a code span the inline index identifies,
	// so neither path reports it.
	assert.Empty(t, res.Diagnostics)
}

// TestLayer0Gate_BlockCheckerRulesSkipParse proves acceptance criterion 1
// of plan 2606141903: the migrated block NodeCheckers (MDS013
// blank-line-around-headings, MDS044 horizontal-rule-style) run with no
// parse. With only those two block rules enabled and the gate on, the
// File reaches the rules with a nil AST, and the block-span dispatch still
// produces their diagnostics.
func TestLayer0Gate_BlockCheckerRulesSkipParse(t *testing.T) {
	withLayer0Skip(t, true)
	// A heading with no blank line after (MDS013) and a non-dash thematic
	// break with surrounding blanks (MDS044). No fence, tab, or four-space
	// indent, so the gate skips the parse.
	dir, path := writeDoc(t, "# Title\nbody\n\n***\n\nmore\n")

	probe := &astProbeRule{}
	cfg := config.Defaults()
	for name := range cfg.Rules {
		cfg.Rules[name] = config.RuleCfg{Enabled: false}
	}
	cfg.Rules["blank-line-around-headings"] = config.RuleCfg{Enabled: true}
	cfg.Rules["horizontal-rule-style"] = config.RuleCfg{Enabled: true}
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
		"block-checker-only config must skip the parse")
	assert.NotEmpty(t, res.Diagnostics,
		"block checkers must still produce diagnostics on the parse-skipped path")
}

// TestLayer0Gate_BlockCheckerDiagnosticsMatchFullParse is the per-rule
// equivalence for the migrated block NodeCheckers driven through the real
// engine: the diagnostics with the gate on (block-span dispatch, nil AST)
// are identical to the gate off (AST walk).
func TestLayer0Gate_BlockCheckerDiagnosticsMatchFullParse(t *testing.T) {
	dir, path := writeDoc(t, "# Title\nbody\n\n***\n\nmore\n\n## Next\nx\n")
	cfg := config.Defaults()
	for name := range cfg.Rules {
		cfg.Rules[name] = config.RuleCfg{Enabled: false}
	}
	cfg.Rules["blank-line-around-headings"] = config.RuleCfg{Enabled: true}
	cfg.Rules["horizontal-rule-style"] = config.RuleCfg{Enabled: true}

	run := func(skip bool) []lint.Diagnostic {
		withLayer0Skip(t, skip)
		r := &Runner{
			Config:           cfg,
			Rules:            rule.All(),
			StripFrontMatter: true,
			RootDir:          dir,
		}
		return r.Run([]string{path}).Diagnostics
	}

	skipped := run(true)
	parsed := run(false)
	require.NotEmpty(t, parsed)
	assert.Equal(t, parsed, skipped,
		"block-checker parse-skip must match the full parse")
}

// astProbeRule is a test rule that records whether the File it checked had
// a nil AST (the parse-skipped state). It declares itself Layer 0 via an
// id the audit manifest covers... but as a synthetic rule its id is not in
// the manifest, so the gate treats it as AST-requiring. To keep it from
// disabling the gate, the probe carries a known Layer 0 id; see ID().
type astProbeRule struct {
	sawNilAST bool
}

// ID returns MDS006 (no-trailing-spaces), a Layer 0 rule in the audit
// manifest, so the probe itself does not force a parse. The probe never
// emits diagnostics, so borrowing the id does not collide with the real
// rule's output.
func (r *astProbeRule) ID() string       { return "MDS006" }
func (r *astProbeRule) Name() string     { return "ast-probe" }
func (r *astProbeRule) Category() string { return "whitespace" }

func (r *astProbeRule) Check(f *lint.File) []lint.Diagnostic {
	r.sawNilAST = f.AST == nil
	return nil
}
