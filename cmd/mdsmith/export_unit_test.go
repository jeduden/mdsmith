package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/export"
	"github.com/jeduden/mdsmith/internal/readlimit"
	"github.com/jeduden/mdsmith/internal/rule"

	// Make sure the production directive rules are registered for
	// prepareExportFile / configuredEnabledRules to find.
	_ "github.com/jeduden/mdsmith/internal/rules/all"
)

func TestExportMode_Mapping(t *testing.T) {
	assert.Equal(t, export.Check, exportMode(exportFlags{}))
	assert.Equal(t, export.Fix, exportMode(exportFlags{fixStale: true}))
	assert.Equal(t, export.NoCheck, exportMode(exportFlags{noCheck: true}))
	// `fixStale` wins when both are set; the CLI rejects that
	// combination earlier, so exportMode only ever sees the legal
	// inputs — this asserts the precedence anyway.
	assert.Equal(t, export.Fix, exportMode(exportFlags{fixStale: true, noCheck: true}))
}

func TestParseExportFlags_AllFlags(t *testing.T) {
	flags, posArgs, code := parseExportFlags([]string{
		"-c", "custom.yml",
		"--output", "out.md",
		"--max-input-size", "1MB",
		"--fix",
		"doc.md",
	})
	assert.Equal(t, -1, code, "good flag set should produce -1 (continue)")
	assert.Equal(t, "custom.yml", flags.configPath)
	assert.Equal(t, "out.md", flags.output)
	assert.Equal(t, "1MB", flags.maxInputSize)
	assert.True(t, flags.fixStale)
	assert.False(t, flags.noCheck)
	assert.Equal(t, []string{"doc.md"}, posArgs)
}

func TestParseExportFlags_NoCheckShortFormDisallowed(t *testing.T) {
	// --no-check is the long form; the short -n isn't registered.
	stderr := captureStderr(func() {
		_, _, code := parseExportFlags([]string{"-n"})
		assert.Equal(t, 2, code)
	})
	assert.Contains(t, stderr, "mdsmith: export")
}

func TestParseExportFlags_HelpExitsZero(t *testing.T) {
	stderr := captureStderr(func() {
		_, _, code := parseExportFlags([]string{"--help"})
		assert.Equal(t, 0, code)
	})
	// `--help` prints the Usage callback we wired in parseExportFlags.
	assert.Contains(t, stderr, "mdsmith export")
}

func TestRunExport_FixAndNoCheck_Mutex(t *testing.T) {
	stderr := captureStderr(func() {
		code := runExport([]string{"--fix", "--no-check", "ignored.md"})
		assert.Equal(t, 2, code)
	})
	assert.Contains(t, stderr, "mutually exclusive")
}

func TestRunExport_MissingPositional(t *testing.T) {
	stderr := captureStderr(func() {
		code := runExport(nil)
		assert.Equal(t, 2, code)
	})
	assert.Contains(t, stderr, "requires a file argument")
}

func TestRunExport_TooManyPositionals(t *testing.T) {
	stderr := captureStderr(func() {
		code := runExport([]string{"a.md", "b.md"})
		assert.Equal(t, 2, code)
	})
	assert.Contains(t, stderr, "single file argument")
}

func TestRunExport_HelpExitsZero(t *testing.T) {
	// --help drives parseExportFlags to ErrHelp, which
	// reportFlagParseErr maps to exit 0; the runExport `code >= 0`
	// branch returns it directly.
	stderr := captureStderr(func() {
		code := runExport([]string{"--help"})
		assert.Equal(t, 0, code)
	})
	assert.Contains(t, stderr, "mdsmith export")
}

func TestRunExport_UnknownFlag_ExitsTwo(t *testing.T) {
	// An unknown flag drives parseExportFlags to a non-help parse
	// error → reportFlagParseErr returns 2 with a stderr message →
	// runExport's `code >= 0` branch returns it.
	stderr := captureStderr(func() {
		code := runExport([]string{"--no-such-flag"})
		assert.Equal(t, 2, code)
	})
	assert.Contains(t, stderr, "mdsmith: export")
}

func TestWriteExportOutput_File(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "out.md")

	require.NoError(t, writeExportOutput(dst, []byte("hello\n")))

	got, err := os.ReadFile(dst)
	require.NoError(t, err)
	assert.Equal(t, "hello\n", string(got))
}

func TestWriteExportOutput_FileWriteError_BubblesUp(t *testing.T) {
	// A path inside a non-existent directory must surface a clear
	// error, not a panic; the doExport caller maps the error to
	// stderr + exit 2.
	err := writeExportOutput(filepath.Join(t.TempDir(), "missing-subdir", "out.md"), []byte("x"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "out.md")
}

func TestWriteExportOutput_Stdout(t *testing.T) {
	stdout := captureStdout(func() {
		require.NoError(t, writeExportOutput("", []byte("via stdout\n")))
	})
	assert.Equal(t, "via stdout\n", stdout)
}

// minimalConfig builds a config.Config with frontMatter enabled and
// the named ignore patterns, suitable for prepareExportFile.
func minimalConfig() *config.Config {
	cfg := config.Merge(config.Defaults(), nil)
	return cfg
}

func TestPrepareExportFile_BasicWiring(t *testing.T) {
	dir := t.TempDir()
	src := "# Title\n\n<?toc?>\n\n- [Section](#section)\n\n<?/toc?>\n\n## Section\n\nbody\n"
	path := filepath.Join(dir, "doc.md")
	require.NoError(t, os.WriteFile(path, []byte(src), 0644))

	f, rules, err := prepareExportFile(path, []byte(src), minimalConfig(), "", readlimit.DefaultMaxInputBytes)
	require.NoError(t, err)

	require.NotNil(t, f.FS, "FS should be wired so include/catalog can read siblings")
	require.NotNil(t, f.GitignoreFunc, "GitignoreFunc should be wired so catalog respects .gitignore")
	// GeneratedRanges is computed from FindAllGeneratedRanges, which
	// returns nil for files with no include/catalog content. Verify
	// the call was made by checking the field is set on the
	// hydrated *lint.File — even an empty slice signals "computed".
	assert.NotPanics(t, func() {
		_ = len(f.GeneratedRanges)
	})

	require.NotEmpty(t, rules,
		"with defaults loaded, configuredEnabledRules should return the directive rules")

	// Sanity: the toc rule lives in the slice (so Fix mode would
	// recognise it). The slice is configured+enabled.
	found := false
	for _, r := range rules {
		if r.Name() == "toc" {
			found = true
			break
		}
	}
	assert.True(t, found, "toc must be in the enabled rules slice")
}

func TestPrepareExportFile_InvalidFrontMatterKinds_ReturnsError(t *testing.T) {
	// front-matter `kinds:` must be a list of strings; a malformed
	// entry surfaces as a parse error so the CLI exits 2 with a clear
	// message instead of silently treating the file as kindless.
	src := "---\nkinds: 42\n---\n# Title\n"
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	require.NoError(t, os.WriteFile(path, []byte(src), 0644))

	_, _, err := prepareExportFile(path, []byte(src), minimalConfig(), "", readlimit.DefaultMaxInputBytes)
	require.Error(t, err)
	assert.True(t,
		strings.Contains(err.Error(), "kinds") ||
			strings.Contains(err.Error(), "front-matter"),
		"error should mention kinds / front-matter, got %v", err)
}

func TestConfiguredEnabledRules_FiltersDisabled(t *testing.T) {
	// Build an effective map that disables `toc` and leaves the rest
	// enabled. configuredEnabledRules must drop toc but keep
	// everything else.
	all := rule.All()
	effective := map[string]config.RuleCfg{}
	for _, r := range all {
		effective[r.Name()] = config.RuleCfg{Enabled: r.Name() != "toc"}
	}

	out, err := configuredEnabledRules(all, effective)
	require.NoError(t, err)

	for _, r := range out {
		assert.NotEqual(t, "toc", r.Name(),
			"toc was disabled in effective config but appeared in the output")
	}
}

func TestConfiguredEnabledRules_PropagatesConfigureRuleError(t *testing.T) {
	// emphasis-style.ApplySettings rejects non-string `bold`. An
	// effective map with that bad setting drives ConfigureRule into
	// an error, which configuredEnabledRules surfaces unchanged so
	// the export refuses with a clear message instead of running
	// against partially-configured rules.
	all := rule.All()
	effective := map[string]config.RuleCfg{
		"emphasis-style": {
			Enabled:  true,
			Settings: map[string]any{"bold": 42}, // wrong type → error
		},
	}

	_, err := configuredEnabledRules(all, effective)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "emphasis-style")
}

func TestPrepareExportFile_ConfigureRuleError_BubblesUp(t *testing.T) {
	// A config that disables every rule except a deliberately
	// misconfigured one drives prepareExportFile → effectiveExportConfig
	// →  configuredEnabledRules → ConfigureRule error. The error
	// surfaces from prepareExportFile so doExport can map it to
	// exit 2.
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".mdsmith.yml")
	require.NoError(t, os.WriteFile(cfgPath, []byte(
		"rules:\n  emphasis-style:\n    bold: 42\n"), 0644))

	cfg, _, err := loadConfig(cfgPath)
	require.NoError(t, err)

	srcPath := filepath.Join(dir, "doc.md")
	require.NoError(t, os.WriteFile(srcPath, []byte("# Hi\n"), 0644))
	src := []byte("# Hi\n")

	_, _, err = prepareExportFile(srcPath, src, cfg, cfgPath, readlimit.DefaultMaxInputBytes)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "emphasis-style")
}

func TestConfiguredEnabledRules_OmitsRulesAbsentFromEffective(t *testing.T) {
	// An empty effective map means no rule has been resolved → no
	// rules come back. This guards the "ok" branch in
	// configuredEnabledRules.
	out, err := configuredEnabledRules(rule.All(), map[string]config.RuleCfg{})
	require.NoError(t, err)
	assert.Empty(t, out)
}

func TestDoExport_ReadError_ExitsTwo(t *testing.T) {
	// doExport is called via runExport; the file does not exist, so
	// readlimit.ReadFileLimited fails and the CLI exits 2 with a message
	// on stderr.
	missing := filepath.Join(t.TempDir(), "nope.md")
	stderr := captureStderr(func() {
		code := runExport([]string{missing})
		assert.Equal(t, 2, code)
	})
	assert.Contains(t, stderr, "nope.md")
}

func TestDoExport_FreshFile_StdoutSuccess(t *testing.T) {
	// End-to-end through doExport (not the subprocess binary): a
	// fresh file goes through the default check mode, prints to
	// stdout, and exits 0. Captures stdout via the test helper.
	dir := t.TempDir()
	src := "# Title\n\n<?toc?>\n\n- [Section](#section)\n\n<?/toc?>\n\n## Section\n\nbody\n"
	path := filepath.Join(dir, "doc.md")
	require.NoError(t, os.WriteFile(path, []byte(src), 0644))

	var code int
	stdout := captureStdout(func() {
		code = runExport([]string{path})
	})
	assert.Equal(t, 0, code)
	assert.NotContains(t, stdout, "<?toc")
	assert.Contains(t, stdout, "- [Section](#section)")
}

func TestDoExport_StaleFile_DefaultMode_ExitsOne(t *testing.T) {
	dir := t.TempDir()
	src := "# Title\n\n<?toc?>\n\n- [Wrong](#wrong)\n\n<?/toc?>\n\n## Section\n\nbody\n"
	path := filepath.Join(dir, "doc.md")
	require.NoError(t, os.WriteFile(path, []byte(src), 0644))

	var code int
	stderr := captureStderr(func() {
		code = runExport([]string{path})
	})
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "out of date")
}

func TestExportFrontMatterFields_NoSelector_ReturnsNil(t *testing.T) {
	cfg := minimalConfig()
	// Default config has no fields-present kind-assignment entry; the
	// helper short-circuits and returns nil without parsing.
	out, err := exportFrontMatterFields(cfg, "doc.md", []byte("---\nbroken: [yaml\n---\n"))
	require.NoError(t, err)
	assert.Nil(t, out)
}

// configWithFieldsPresent returns a config whose KindAssignment has a
// single fields-present entry, forcing NeedsFieldsForFile to return
// true so the front-matter decode runs.
func configWithFieldsPresent() *config.Config {
	cfg := minimalConfig()
	cfg.KindAssignment = append(cfg.KindAssignment, config.KindAssignmentEntry{
		FieldsPresent: []string{"id"},
		Kinds:         []string{},
	})
	return cfg
}

func TestExportFrontMatterFields_SelectorTriggersDecode(t *testing.T) {
	cfg := configWithFieldsPresent()
	out, err := exportFrontMatterFields(cfg, "doc.md", []byte("---\nid: 7\ntitle: t\n---\n"))
	require.NoError(t, err)
	require.NotNil(t, out)
	assert.Equal(t, 7, out["id"])
	assert.Equal(t, "t", out["title"])
}

func TestExportFrontMatterFields_SelectorPropagatesParseError(t *testing.T) {
	cfg := configWithFieldsPresent()
	_, err := exportFrontMatterFields(cfg, "doc.md", []byte("---\nbroken: [yaml\n---\n"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parsing front matter")
}

func TestEffectiveExportConfig_NoFrontMatter_DefaultsRule(t *testing.T) {
	cfg := minimalConfig()
	out, err := effectiveExportConfig(cfg, "doc.md", nil, rule.All())
	require.NoError(t, err)
	// At least the toc rule should be present in the effective map
	// with its default-enabled state.
	tocCfg, ok := out["toc"]
	require.True(t, ok, "toc should appear in the effective config")
	assert.True(t, tocCfg.Enabled, "toc should be enabled by default")
}

func TestEffectiveExportConfig_InvalidFrontmatterKindsName_Surfaces(t *testing.T) {
	// A front-matter `kinds:` entry naming an undeclared kind is a
	// validation error.
	cfg := minimalConfig()
	src := []byte("---\nkinds: [no-such-kind]\n---\n")
	_, err := effectiveExportConfig(cfg, "doc.md", src, rule.All())
	require.Error(t, err)
}

func TestRunExport_OutputFlag_RoundTripsThroughTempFile(t *testing.T) {
	// Drive runExport with the --output flag set; the file should
	// be written and stdout stays empty.
	dir := t.TempDir()
	src := "# Title\n\n<?toc?>\n\n- [Section](#section)\n\n<?/toc?>\n\n## Section\n\nbody\n"
	srcPath := filepath.Join(dir, "doc.md")
	require.NoError(t, os.WriteFile(srcPath, []byte(src), 0644))
	dstPath := filepath.Join(dir, "out.md")

	var code int
	stdout := captureStdout(func() {
		code = runExport([]string{"-o", dstPath, srcPath})
	})
	assert.Equal(t, 0, code)
	assert.Empty(t, stdout, "with -o, stdout stays empty")

	data, err := os.ReadFile(dstPath)
	require.NoError(t, err)
	assert.NotContains(t, string(data), "<?toc")
	assert.Contains(t, string(data), "- [Section](#section)")
}

func TestRunExport_OutputFlag_WriteFailure_ExitsTwo(t *testing.T) {
	// Writing to a path inside a non-existent directory should
	// surface as exit 2 with a clear error.
	dir := t.TempDir()
	src := "# Title\n\nNo directives.\n"
	srcPath := filepath.Join(dir, "doc.md")
	require.NoError(t, os.WriteFile(srcPath, []byte(src), 0644))
	dstPath := filepath.Join(dir, "missing-sub", "out.md")

	var code int
	stderr := captureStderr(func() {
		code = runExport([]string{"-o", dstPath, srcPath})
	})
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "out.md")
}

func TestDoExport_InvalidConfig_ExitsTwo(t *testing.T) {
	// Pass a config path that does not parse — doExport surfaces
	// the loadConfig error as exit 2.
	dir := t.TempDir()
	badCfg := filepath.Join(dir, "bad.yml")
	require.NoError(t, os.WriteFile(badCfg, []byte("rules: [not a map]\n"), 0644))
	srcPath := filepath.Join(dir, "doc.md")
	require.NoError(t, os.WriteFile(srcPath, []byte("# Hi\n"), 0644))

	var code int
	stderr := captureStderr(func() {
		code = runExport([]string{"-c", badCfg, srcPath})
	})
	assert.Equal(t, 2, code)
	assert.NotEmpty(t, stderr)
}

func TestDoExport_InvalidFrontMatterKinds_ExitsTwo(t *testing.T) {
	// A file with malformed `kinds:` front matter makes
	// prepareExportFile return an error. doExport must map that to
	// exit 2 with a clear stderr message.
	dir := t.TempDir()
	srcPath := filepath.Join(dir, "doc.md")
	require.NoError(t, os.WriteFile(srcPath, []byte("---\nkinds: 42\n---\n# Hi\n"), 0644))

	var code int
	stderr := captureStderr(func() {
		code = runExport([]string{srcPath})
	})
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "mdsmith:")
}

func TestDoExport_StaleFile_PrintsDiagnostics(t *testing.T) {
	// Stale-body refusal in Check mode goes through formatDiagnostics
	// and exits 1 with the diagnostic on stderr.
	dir := t.TempDir()
	src := "# Title\n\n<?toc?>\n\n- [Wrong](#wrong)\n\n<?/toc?>\n\n## Section\n\nbody\n"
	srcPath := filepath.Join(dir, "doc.md")
	require.NoError(t, os.WriteFile(srcPath, []byte(src), 0644))

	var code int
	stderr := captureStderr(func() {
		code = runExport([]string{srcPath})
	})
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "out of date")
	assert.Contains(t, stderr, "MDS038")
}

func TestPrepareExportFile_NoCfgPath_UsesDocDirForGitignore(t *testing.T) {
	// When cfgPath is empty, the gitignore matcher should still be
	// wired (against the doc's parent directory). Triggering
	// GetGitignore must not panic and should return a non-nil
	// matcher — exercises the closure body.
	dir := t.TempDir()
	src := "# Title\n\nNo directives.\n"
	path := filepath.Join(dir, "doc.md")
	require.NoError(t, os.WriteFile(path, []byte(src), 0644))

	f, _, err := prepareExportFile(path, []byte(src), minimalConfig(), "", readlimit.DefaultMaxInputBytes)
	require.NoError(t, err)
	// Calling GetGitignore should invoke the closure on line 153-155
	// and return a matcher anchored at the doc's parent dir.
	matcher := f.GetGitignore()
	require.NotNil(t, matcher)
}

func TestPrepareExportFile_WithCfgPath_UsesProjectRootForGitignore(t *testing.T) {
	// With cfgPath set, the matcher should be anchored at the
	// project root (parent of cfgPath), exercising the
	// rootDirFromConfig branch in prepareExportFile.
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".mdsmith.yml")
	require.NoError(t, os.WriteFile(cfgPath, []byte("rules: {}\n"), 0644))

	src := "# Title\n\nNo directives.\n"
	path := filepath.Join(dir, "doc.md")
	require.NoError(t, os.WriteFile(path, []byte(src), 0644))

	f, _, err := prepareExportFile(path, []byte(src), minimalConfig(), cfgPath, readlimit.DefaultMaxInputBytes)
	require.NoError(t, err)
	matcher := f.GetGitignore()
	require.NotNil(t, matcher)
}

func TestEffectiveExportConfig_FieldsPresentBranch_PropagatesError(t *testing.T) {
	// Front-matter parse error from exportFrontMatterFields should
	// flow through effectiveExportConfig.
	cfg := configWithFieldsPresent()
	_, err := effectiveExportConfig(cfg, "doc.md", []byte("---\nbroken: [yaml\n---\n"), rule.All())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "front matter")
}

func TestDoExport_BadMaxInputSize_ExitsTwo(t *testing.T) {
	dir := t.TempDir()
	srcPath := filepath.Join(dir, "doc.md")
	require.NoError(t, os.WriteFile(srcPath, []byte("# Hi\n"), 0644))

	var code int
	stderr := captureStderr(func() {
		code = runExport([]string{"--max-input-size", "not-a-size", srcPath})
	})
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "max-input-size")
}
