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

// TestLayer0Gate_ParsesWhenAnASTRuleIsEnabled confirms the gate stands
// down when any enabled rule needs the AST: line-length (MDS001) is
// AST-requiring, so the File must carry a parsed tree.
func TestLayer0Gate_ParsesWhenAnASTRuleIsEnabled(t *testing.T) {
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

// TestLayer0Gate_CodeBlockForcesParse proves the SourceMayHaveCodeBlock
// guard: the Layer 0 scanner does not descend into a list item's content, so
// a file that may hold a code block (here a hard tab — also a fence or a
// four-space indent) forces the full parse rather than risk a CodeBlockLines
// divergence, even when every enabled rule is otherwise Layer 0.
func TestLayer0Gate_CodeBlockForcesParse(t *testing.T) {
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
	assert.False(t, probe.sawNilAST,
		"a source that may hold a code block must keep the AST parse")
}

// TestLayer0Gate_CodeSpanRuleForcesParse guards the soundness fix for the
// inline-code-span gap: MDS054 (no-undefined-reference-labels) is
// "A-no-skipping" in the AST-dependence audit but reads code-span ranges
// that go empty without a parse, so it must force the parse even though it
// never crashes on a nil AST. Without the parse it would emit a false
// positive for `[ref]` inside a backtick span.
func TestLayer0Gate_CodeSpanRuleForcesParse(t *testing.T) {
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
	assert.False(t, probe.sawNilAST,
		"a code-span-consuming rule must keep the AST parse")
	// The bracket text sits inside a code span, so neither path reports it.
	assert.Empty(t, res.Diagnostics)
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
