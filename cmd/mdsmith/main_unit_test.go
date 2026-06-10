package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/jeduden/mdsmith/internal/bytelimit"
	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/engine"
	fixpkg "github.com/jeduden/mdsmith/internal/fix"
	"github.com/jeduden/mdsmith/internal/lint"
	vlog "github.com/jeduden/mdsmith/internal/log"
	"github.com/jeduden/mdsmith/internal/query"
	ruledocs "github.com/jeduden/mdsmith/internal/rules"
)

// captureStderr temporarily redirects os.Stderr and returns the written content.
// Must NOT be called from parallel tests (t.Parallel()) because it redirects
// the global os.Stderr. Tests using this helper must run sequentially.
func captureStderr(f func()) string {
	r, w, err := os.Pipe()
	if err != nil {
		panic(err)
	}
	defer r.Close() //nolint:errcheck // best-effort close on read-only pipe end
	old := os.Stderr
	os.Stderr = w
	f()
	_ = w.Close()
	os.Stderr = old
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	return buf.String()
}

// captureStdout temporarily redirects os.Stdout and returns the written content.
// Must NOT be called from parallel tests (t.Parallel()) because it redirects
// the global os.Stdout. Tests using this helper must run sequentially.
func captureStdout(f func()) string {
	r, w, err := os.Pipe()
	if err != nil {
		panic(err)
	}
	defer r.Close() //nolint:errcheck // best-effort close on read-only pipe end
	old := os.Stdout
	os.Stdout = w
	f()
	_ = w.Close()
	os.Stdout = old
	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	return buf.String()
}

// --- splitStdinArg ---

func TestSplitStdinArg_NoStdin(t *testing.T) {
	hasStdin, fileArgs := splitStdinArg([]string{"a.md", "b.md"})
	assert.False(t, hasStdin)
	assert.Equal(t, []string{"a.md", "b.md"}, fileArgs)
}

func TestSplitStdinArg_DashOnly(t *testing.T) {
	hasStdin, fileArgs := splitStdinArg([]string{"-"})
	assert.True(t, hasStdin)
	assert.Nil(t, fileArgs)
}

func TestSplitStdinArg_DashAmongFiles(t *testing.T) {
	hasStdin, fileArgs := splitStdinArg([]string{"a.md", "-", "b.md"})
	assert.True(t, hasStdin)
	assert.Equal(t, []string{"a.md", "b.md"}, fileArgs)
}

func TestSplitStdinArg_MultipleDashes(t *testing.T) {
	hasStdin, fileArgs := splitStdinArg([]string{"-", "-"})
	assert.True(t, hasStdin)
	assert.Nil(t, fileArgs)
}

func TestSplitStdinArg_Empty(t *testing.T) {
	hasStdin, fileArgs := splitStdinArg(nil)
	assert.False(t, hasStdin)
	assert.Nil(t, fileArgs)
}

// --- frontMatterEnabled ---

func TestFrontMatterEnabled_NilDefaultsTrue(t *testing.T) {
	cfg := &config.Config{}
	assert.True(t, frontMatterEnabled(cfg))
}

func TestFrontMatterEnabled_ExplicitTrue(t *testing.T) {
	v := true
	cfg := &config.Config{FrontMatter: &v}
	assert.True(t, frontMatterEnabled(cfg))
}

func TestFrontMatterEnabled_ExplicitFalse(t *testing.T) {
	v := false
	cfg := &config.Config{FrontMatter: &v}
	assert.False(t, frontMatterEnabled(cfg))
}

// --- rootDirFromConfig ---

func TestRootDirFromConfig_EmptyUsesWorkingDir(t *testing.T) {
	wd, err := os.Getwd()
	require.NoError(t, err)
	assert.Equal(t, wd, rootDirFromConfig(""))
}

func TestRootDirFromConfig_AbsoluteConfig(t *testing.T) {
	assert.Equal(t, "/some/dir", rootDirFromConfig("/some/dir/.mdsmith.yml"))
}

func TestRootDirFromConfig_RelativeConfig(t *testing.T) {
	assert.Equal(t, "sub", rootDirFromConfig("sub/.mdsmith.yml"))
}

func TestRootDirFromConfig_JustFilename(t *testing.T) {
	assert.Equal(t, ".", rootDirFromConfig(".mdsmith.yml"))
}

// --- resolveMaxInputBytes ---

func TestResolveMaxInputBytes_BothEmpty_UsesDefault(t *testing.T) {
	cfg := &config.Config{}
	n, err := resolveMaxInputBytes(cfg, "")
	require.NoError(t, err)
	assert.Equal(t, bytelimit.DefaultMaxInputBytes, n)
}

func TestResolveMaxInputBytes_CLIFlagOverridesConfig(t *testing.T) {
	cfg := &config.Config{MaxInputSize: "1MB"}
	n, err := resolveMaxInputBytes(cfg, "500KB")
	require.NoError(t, err)
	assert.Equal(t, int64(500*1024), n)
}

func TestResolveMaxInputBytes_ConfigUsedWhenCLIEmpty(t *testing.T) {
	cfg := &config.Config{MaxInputSize: "1MB"}
	n, err := resolveMaxInputBytes(cfg, "")
	require.NoError(t, err)
	assert.Equal(t, int64(1024*1024), n)
}

func TestResolveMaxInputBytes_ZeroUnlimited(t *testing.T) {
	cfg := &config.Config{}
	n, err := resolveMaxInputBytes(cfg, "0")
	require.NoError(t, err)
	assert.Equal(t, int64(0), n)
}

func TestResolveMaxInputBytes_InvalidCLI_Error(t *testing.T) {
	cfg := &config.Config{}
	_, err := resolveMaxInputBytes(cfg, "notasize")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid max-input-size")
}

// --- resolveOpts ---

func TestResolveOpts_BothFalse_GitignoreEnabled(t *testing.T) {
	cfg := &config.Config{}
	opts := resolveOpts(cfg, walkCLI{})
	require.NotNil(t, opts.UseGitignore)
	assert.True(t, *opts.UseGitignore)
	assert.False(t, opts.FollowSymlinks)
}

func TestResolveOpts_NoGitignore_DisablesFilter(t *testing.T) {
	cfg := &config.Config{}
	opts := resolveOpts(cfg, walkCLI{noGitignore: true})
	require.NotNil(t, opts.UseGitignore)
	assert.False(t, *opts.UseGitignore)
}

func TestResolveOpts_FollowSymlinksFlag_OptsIn(t *testing.T) {
	cfg := &config.Config{}
	yes := true
	opts := resolveOpts(cfg, walkCLI{followSymlinks: &yes})
	assert.True(t, opts.FollowSymlinks)
}

func TestResolveOpts_ConfigFollowSymlinks_Propagated(t *testing.T) {
	cfg := &config.Config{FollowSymlinks: true}
	opts := resolveOpts(cfg, walkCLI{})
	assert.True(t, opts.FollowSymlinks)
}

func TestResolveOpts_ExplicitFalseFlag_OverridesConfigOptIn(t *testing.T) {
	cfg := &config.Config{FollowSymlinks: true}
	no := false
	opts := resolveOpts(cfg, walkCLI{followSymlinks: &no})
	assert.False(t, opts.FollowSymlinks,
		"--follow-symlinks=false must force deny over a config opt-in")
}

// --- printRunStats ---

func TestPrintRunStats_NormalOutputContainsAllFields(t *testing.T) {
	got := captureStderr(func() {
		printRunStats("text", false, runStats{Checked: 10, Fixed: 2, Failures: 3, Unfixed: 1})
	})
	assert.Contains(t, got, "checked=10")
	assert.Contains(t, got, "fixed=2")
	assert.Contains(t, got, "failures=3")
	assert.Contains(t, got, "unfixed=1")
}

func TestPrintRunStats_QuietSuppressesOutput(t *testing.T) {
	got := captureStderr(func() {
		printRunStats("text", true, runStats{Checked: 5})
	})
	assert.Empty(t, got)
}

func TestPrintRunStats_JSONFormatSuppressesOutput(t *testing.T) {
	got := captureStderr(func() {
		printRunStats("json", false, runStats{Checked: 5})
	})
	assert.Empty(t, got)
}

func TestPrintRunStats_ZeroValues(t *testing.T) {
	got := captureStderr(func() {
		printRunStats("text", false, runStats{})
	})
	assert.Contains(t, got, "checked=0")
	assert.Contains(t, got, "fixed=0")
	assert.Contains(t, got, "failures=0")
	assert.Contains(t, got, "unfixed=0")
}

func TestPrintRunStats_DryRunIncludesWouldFix(t *testing.T) {
	got := captureStderr(func() {
		printRunStats("text", false, runStats{
			Checked:  12,
			Fixed:    0,
			Failures: 4,
			Unfixed:  0,
			WouldFix: 8,
			DryRun:   true,
		})
	})
	assert.Contains(t, got, "checked=12")
	assert.Contains(t, got, "fixed=0")
	assert.Contains(t, got, "failures=4")
	assert.Contains(t, got, "unfixed=0")
	assert.Contains(t, got, "would-fix=8")
}

func TestPrintRunStats_NonDryRunOmitsWouldFix(t *testing.T) {
	got := captureStderr(func() {
		printRunStats("text", false, runStats{
			Checked: 1, Fixed: 1, Failures: 1, Unfixed: 0,
		})
	})
	assert.NotContains(t, got, "would-fix",
		"would-fix field must be hidden on non-dry-run; got: %s", got)
}

// --- formatWouldFixSummary / printDryRunPreview / writeDryRunJSON ---

func TestFormatWouldFixSummary_SingleRuleSingleCount(t *testing.T) {
	got := formatWouldFixSummary(fixpkg.WouldFixFile{
		Path:  "a.md",
		Count: 1,
		Rules: []fixpkg.RuleFixCount{{RuleID: "MDS006", Count: 1}},
	})
	assert.Equal(t, "1 violation (MDS006)", got)
}

func TestFormatWouldFixSummary_MultipleRulesWithCounts(t *testing.T) {
	got := formatWouldFixSummary(fixpkg.WouldFixFile{
		Path:  "a.md",
		Count: 3,
		Rules: []fixpkg.RuleFixCount{
			{RuleID: "MDS001", Count: 2},
			{RuleID: "MDS006", Count: 1},
		},
	})
	assert.Equal(t, "3 violations (MDS001 ×2, MDS006)", got)
}

func TestFormatWouldFixSummary_EmptyRulesPrintsCountOnly(t *testing.T) {
	got := formatWouldFixSummary(fixpkg.WouldFixFile{
		Path:  "a.md",
		Count: 2,
		Rules: nil,
	})
	assert.Equal(t, "2 violations", got)
}

func TestPrintDryRunPreview_BytesOnlyChangeReportsRegeneration(t *testing.T) {
	var buf bytes.Buffer
	printDryRunPreview(&buf, &fixpkg.Result{
		WouldFixFiles: []fixpkg.WouldFixFile{
			{Path: "docs/index.md", Count: 0, Rules: nil},
		},
	})
	assert.Equal(t, "docs/index.md: would update generated content\n", buf.String())
}

func TestPrintDryRunPreview_MultipleFiles(t *testing.T) {
	var buf bytes.Buffer
	printDryRunPreview(&buf, &fixpkg.Result{
		WouldFixFiles: []fixpkg.WouldFixFile{
			{
				Path:  "a.md",
				Count: 2,
				Rules: []fixpkg.RuleFixCount{
					{RuleID: "MDS006", Count: 2},
				},
			},
			{
				Path:  "b.md",
				Count: 1,
				Rules: []fixpkg.RuleFixCount{
					{RuleID: "MDS001", Count: 1},
				},
			},
		},
	})
	out := buf.String()
	assert.Contains(t, out, "a.md: would fix 2 violations (MDS006 ×2)\n")
	assert.Contains(t, out, "b.md: would fix 1 violation (MDS001)\n")
}

func TestWriteDryRunJSON_EmitsPerFileRecords(t *testing.T) {
	var buf bytes.Buffer
	code := writeDryRunJSON(&buf, &fixpkg.Result{
		WouldFixFiles: []fixpkg.WouldFixFile{
			{
				Path:  "a.md",
				Count: 3,
				Rules: []fixpkg.RuleFixCount{
					{RuleID: "MDS001", Count: 2},
					{RuleID: "MDS006", Count: 1},
				},
			},
		},
		Diagnostics: []lint.Diagnostic{
			{File: "a.md", Line: 7, Column: 1, RuleID: "MDS017",
				RuleName: "no-trailing-punctuation-in-heading",
				Severity: lint.Warning, Message: "trailing punctuation"},
		},
	})
	assert.Equal(t, 0, code)

	var records []map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &records),
		"output must be valid JSON; got: %s", buf.String())
	require.Len(t, records, 1)

	rec := records[0]
	assert.Equal(t, "a.md", rec["path"])
	assert.EqualValues(t, 3, rec["would_fix"])
	assert.Equal(t, []any{"MDS001", "MDS006"}, rec["rules"])

	diags, ok := rec["diagnostics"].([]any)
	require.True(t, ok, "diagnostics must be a JSON array")
	require.Len(t, diags, 1)
	diag := diags[0].(map[string]any)
	assert.Equal(t, "MDS017", diag["rule"])
}

func TestWriteDryRunJSON_EmptyResultEmitsEmptyArray(t *testing.T) {
	var buf bytes.Buffer
	code := writeDryRunJSON(&buf, &fixpkg.Result{})
	assert.Equal(t, 0, code)
	assert.Equal(t, "[]\n", buf.String())
}

// alwaysErrorWriter rejects all writes, used to drive error paths.
type alwaysErrorWriter struct{}

func (w *alwaysErrorWriter) Write(_ []byte) (int, error) {
	return 0, fmt.Errorf("write failed")
}

func TestWriteDryRunJSON_WriteErrorReturns2(t *testing.T) {
	var code int
	captureStderr(func() {
		code = writeDryRunJSON(&alwaysErrorWriter{}, &fixpkg.Result{
			WouldFixFiles: []fixpkg.WouldFixFile{{Path: "f.md", Count: 1}},
		})
	})
	assert.Equal(t, 2, code)
}

func TestWriteDryRunJSON_PopulatesSourceLinesAndExplanation(t *testing.T) {
	var buf bytes.Buffer
	code := writeDryRunJSON(&buf, &fixpkg.Result{
		WouldFixFiles: []fixpkg.WouldFixFile{
			{Path: "a.md", Count: 1, Rules: []fixpkg.RuleFixCount{{RuleID: "MDS001", Count: 1}}},
		},
		Diagnostics: []lint.Diagnostic{
			{
				File: "a.md", Line: 5, Column: 1,
				RuleID: "MDS017", RuleName: "no-trailing-punct",
				Severity: lint.Warning, Message: "trailing punct",
				SourceLines:     []string{"## Heading."},
				SourceStartLine: 5,
				Explanation: &lint.Explanation{
					Rule: "MDS017",
					Leaves: []lint.ExplanationLeaf{
						{Path: "rules.no-trailing-punct.enabled", Value: true, Source: "default"},
					},
				},
			},
		},
	})
	require.Equal(t, 0, code)

	var records []map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &records))
	require.Len(t, records, 1)
	diags, ok := records[0]["diagnostics"].([]any)
	require.True(t, ok)
	require.Len(t, diags, 1)
	d := diags[0].(map[string]any)
	srcLines, ok := d["source_lines"].([]any)
	require.True(t, ok, "source_lines must be present")
	assert.Equal(t, []any{"## Heading."}, srcLines)
	assert.EqualValues(t, 5, d["source_start_line"])
	exp, ok := d["explanation"].(map[string]any)
	require.True(t, ok, "explanation must be present")
	assert.Equal(t, "MDS017", exp["rule"])
}

func TestWriteDryRunJSON_IncludesUnfixableDiagFiles(t *testing.T) {
	var buf bytes.Buffer
	code := writeDryRunJSON(&buf, &fixpkg.Result{
		WouldFixFiles: []fixpkg.WouldFixFile{},
		Diagnostics: []lint.Diagnostic{
			{File: "b.md", Line: 3, Column: 1, RuleID: "MDS099",
				RuleName: "unfixable-rule", Severity: lint.Error, Message: "unfixable"},
		},
	})
	assert.Equal(t, 0, code)

	var records []map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &records),
		"output must be valid JSON; got: %s", buf.String())
	require.Len(t, records, 1, "unfixable-diag file must appear in output")

	rec := records[0]
	assert.Equal(t, "b.md", rec["path"])
	assert.EqualValues(t, 0, rec["would_fix"])
	assert.Equal(t, []any{}, rec["rules"])

	diags, ok := rec["diagnostics"].([]any)
	require.True(t, ok, "diagnostics must be a JSON array")
	require.Len(t, diags, 1)
	diag := diags[0].(map[string]any)
	assert.Equal(t, "MDS099", diag["rule"])
}

// --- reportFixResult ---

func TestReportFixResult_DryRunTextPreview(t *testing.T) {
	opts := fixCLIOpts{dryRun: true, format: "text"}
	result := &fixpkg.Result{
		FilesChecked: 2,
		Failures:     1,
		WouldFix:     1,
		WouldFixFiles: []fixpkg.WouldFixFile{
			{Path: "f.md", Count: 1, Rules: []fixpkg.RuleFixCount{{RuleID: "MDS001", Count: 1}}},
		},
	}
	var code int
	stderr := captureStderr(func() {
		code = reportFixResult(opts, result, &vlog.Logger{})
	})
	assert.Equal(t, 0, code)
	assert.Contains(t, stderr, "f.md: would fix 1 violation")
	assert.Contains(t, stderr, "would-fix=1")
}

func TestReportFixResult_DryRunJSONOutput(t *testing.T) {
	opts := fixCLIOpts{dryRun: true, format: "json"}
	result := &fixpkg.Result{
		FilesChecked: 1,
		WouldFix:     1,
		WouldFixFiles: []fixpkg.WouldFixFile{
			{Path: "f.md", Count: 1, Rules: []fixpkg.RuleFixCount{{RuleID: "MDS001", Count: 1}}},
		},
	}
	var code int
	var stdout string
	stderr := captureStderr(func() {
		stdout = captureStdout(func() {
			code = reportFixResult(opts, result, &vlog.Logger{})
		})
	})
	assert.Equal(t, 0, code)
	assert.NotContains(t, stdout, "[", "dry-run JSON must go to stderr, not stdout")

	// Stats are suppressed in JSON mode; stderr is just the array.
	var records []map[string]any
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(stderr)), &records))
	require.Len(t, records, 1)
	assert.Equal(t, "f.md", records[0]["path"])
}

func TestReportFixResult_DryRunJSONQuietSuppressesOutput(t *testing.T) {
	opts := fixCLIOpts{dryRun: true, format: "json", quiet: true}
	result := &fixpkg.Result{
		WouldFix: 1,
		WouldFixFiles: []fixpkg.WouldFixFile{
			{Path: "f.md", Count: 1},
		},
	}
	var code int
	var stdout string
	stderr := captureStderr(func() {
		stdout = captureStdout(func() {
			code = reportFixResult(opts, result, &vlog.Logger{})
		})
	})
	assert.Equal(t, 0, code)
	assert.Empty(t, stdout)
	assert.NotContains(t, stderr, "{", "--quiet must suppress dry-run JSON on stderr too")
}

func TestReportFixResult_DiagnosticsReturnsCode1(t *testing.T) {
	opts := fixCLIOpts{format: "text"}
	result := &fixpkg.Result{
		FilesChecked: 1,
		Failures:     1,
		Diagnostics: []lint.Diagnostic{
			{File: "f.md", Line: 1, Column: 1, RuleID: "MDS001",
				RuleName: "test-rule", Severity: lint.Warning, Message: "issue"},
		},
	}
	var code int
	captureStderr(func() {
		code = reportFixResult(opts, result, &vlog.Logger{})
	})
	assert.Equal(t, 1, code)
}

func TestReportFixResult_ErrorsOnlyReturnsCode2(t *testing.T) {
	opts := fixCLIOpts{format: "text"}
	result := &fixpkg.Result{
		FilesChecked: 1,
		Errors:       []error{fmt.Errorf("disk error")},
	}
	var code int
	stderr := captureStderr(func() {
		code = reportFixResult(opts, result, &vlog.Logger{})
	})
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "disk error")
}

func TestReportFixResultTo_DryRunJSONWriteErrorReturns2(t *testing.T) {
	opts := fixCLIOpts{dryRun: true, format: "json"}
	result := &fixpkg.Result{
		WouldFixFiles: []fixpkg.WouldFixFile{{Path: "f.md", Count: 1}},
	}
	code := reportFixResultTo(opts, result, &vlog.Logger{}, &alwaysErrorWriter{})
	assert.Equal(t, 2, code)
}

func TestReportFixResultTo_DiagWriteErrorReturns2(t *testing.T) {
	opts := fixCLIOpts{format: "text"}
	result := &fixpkg.Result{
		FilesChecked: 1,
		Failures:     1,
		Diagnostics: []lint.Diagnostic{
			{File: "f.md", Line: 1, Column: 1, RuleID: "MDS001",
				RuleName: "test-rule", Severity: lint.Warning, Message: "issue"},
		},
	}
	code := reportFixResultTo(opts, result, &vlog.Logger{}, &alwaysErrorWriter{})
	assert.Equal(t, 2, code)
}

// --- reportCheckResultTo ---

func TestReportCheckResultTo_DiagWriteErrorReturns2(t *testing.T) {
	opts := checkCLIOpts{format: "text"}
	result := &engine.Result{
		FilesChecked: 1,
		Diagnostics: []lint.Diagnostic{
			{File: "f.md", Line: 1, Column: 1, RuleID: "MDS001",
				RuleName: "test-rule", Severity: lint.Warning, Message: "issue"},
		},
	}
	code := reportCheckResultTo(result, opts, &vlog.Logger{}, &alwaysErrorWriter{})
	assert.Equal(t, 2, code)
}

// --- checkStdin / checkDiscovered / fixDiscovered: bad max-input-size ---
//
// Each handler resolves max-input-size after its config load, so the
// error branch is unreachable from the loadAndResolve path tested by
// the other check / fix entry points. These three tests drive the
// branch directly — pushing what would otherwise be three separate
// binary-invocation e2e tests down to the same package.

func TestCheckStdin_BadMaxInputSize_ExitsTwo(t *testing.T) {
	// Isolate from any project .mdsmith.yml so loadConfig falls
	// through to defaults and resolveMaxInputBytes runs against the
	// CLI flag alone.
	t.Chdir(t.TempDir())

	var code int
	stderr := captureStderr(func() {
		code = checkStdin(checkCLIOpts{maxInputSize: "not-a-size"})
	})
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "max-input-size")
}

func TestCheckDiscovered_BadMaxInputSize_ExitsTwo(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".mdsmith.yml"),
		[]byte("files: [\"**/*.md\"]\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "test.md"),
		[]byte("# Title\n\nContent.\n"), 0o644))
	t.Chdir(dir)

	var code int
	stderr := captureStderr(func() {
		code = checkDiscovered(checkCLIOpts{maxInputSize: "not-a-size"})
	})
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "max-input-size")
}

func TestFixDiscovered_BadMaxInputSize_ExitsTwo(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".mdsmith.yml"),
		[]byte("files: [\"**/*.md\"]\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "test.md"),
		[]byte("# Title\n\nContent.\n"), 0o644))
	t.Chdir(dir)

	var code int
	stderr := captureStderr(func() {
		code = fixDiscovered(fixCLIOpts{maxInputSize: "not-a-size"})
	})
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "max-input-size")
}

// --- printErrors ---

func TestPrintErrors_Empty_NoOutput(t *testing.T) {
	got := captureStderr(func() {
		printErrors(nil)
	})
	assert.Empty(t, got)
}

func TestPrintErrors_SingleError(t *testing.T) {
	got := captureStderr(func() {
		printErrors([]error{fmt.Errorf("something went wrong")})
	})
	assert.Contains(t, got, "something went wrong")
}

func TestPrintErrors_MultipleErrors(t *testing.T) {
	got := captureStderr(func() {
		printErrors([]error{fmt.Errorf("err one"), fmt.Errorf("err two")})
	})
	assert.Contains(t, got, "err one")
	assert.Contains(t, got, "err two")
}

// --- readFrontMatterRaw ---

func TestReadFrontMatterRaw_WithFrontMatter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.md")
	require.NoError(t, os.WriteFile(path, []byte("---\ntitle: hello\nauthor: alice\n---\n# H\n\nBody.\n"), 0644))

	fm, err := readFrontMatterRaw(path, 0)
	require.NoError(t, err)
	require.NotNil(t, fm)
	assert.Equal(t, "hello", fm["title"])
	assert.Equal(t, "alice", fm["author"])
}

func TestReadFrontMatterRaw_NoFrontMatter_ReturnsNil(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.md")
	require.NoError(t, os.WriteFile(path, []byte("# Just a heading\n\nContent.\n"), 0644))

	fm, err := readFrontMatterRaw(path, 0)
	require.NoError(t, err)
	assert.Nil(t, fm)
}

func TestReadFrontMatterRaw_EmptyFrontMatter_ReturnsEmptyMap(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.md")
	require.NoError(t, os.WriteFile(path, []byte("---\n---\n# H\n"), 0644))

	fm, err := readFrontMatterRaw(path, 0)
	require.NoError(t, err)
	assert.NotNil(t, fm)
	assert.Empty(t, fm)
}

func TestReadFrontMatterRaw_NumericValues_Preserved(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.md")
	require.NoError(t, os.WriteFile(path, []byte("---\ncount: 42\n---\n# H\n"), 0644))

	fm, err := readFrontMatterRaw(path, 0)
	require.NoError(t, err)
	assert.Equal(t, 42, fm["count"])
}

func TestReadFrontMatterRaw_FileNotFound_Error(t *testing.T) {
	_, err := readFrontMatterRaw("/no/such/file.md", 0)
	assert.Error(t, err)
}

func TestReadFrontMatterRaw_YAMLAlias_Error(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.md")
	require.NoError(t, os.WriteFile(path, []byte("---\na: &anchor val\nb: *anchor\n---\n# H\n"), 0644))

	_, err := readFrontMatterRaw(path, 0)
	assert.Error(t, err)
}

// --- parseQueryFlags ---

func TestParseQueryFlags_Defaults(t *testing.T) {
	opts, args, err := parseQueryFlags([]string{"expr", "file.md"})
	require.NoError(t, err)
	assert.False(t, opts.nul)
	assert.False(t, opts.verbose)
	assert.Empty(t, opts.configPath)
	assert.Equal(t, []string{"expr", "file.md"}, args)
}

func TestParseQueryFlags_NullLongFlag(t *testing.T) {
	opts, _, err := parseQueryFlags([]string{"--null", "expr"})
	require.NoError(t, err)
	assert.True(t, opts.nul)
}

func TestParseQueryFlags_NullShortFlag(t *testing.T) {
	opts, _, err := parseQueryFlags([]string{"-0", "expr"})
	require.NoError(t, err)
	assert.True(t, opts.nul)
}

func TestParseQueryFlags_VerboseFlag(t *testing.T) {
	opts, _, err := parseQueryFlags([]string{"-v", "expr"})
	require.NoError(t, err)
	assert.True(t, opts.verbose)
}

func TestParseQueryFlags_ConfigFlag(t *testing.T) {
	opts, _, err := parseQueryFlags([]string{"-c", "/path/cfg.yml", "expr"})
	require.NoError(t, err)
	assert.Equal(t, "/path/cfg.yml", opts.configPath)
}

func TestParseQueryFlags_MaxInputSizeFlag(t *testing.T) {
	opts, _, err := parseQueryFlags([]string{"--max-input-size", "1MB", "expr"})
	require.NoError(t, err)
	assert.Equal(t, "1MB", opts.maxInputSize)
}

// --- parseCheckFlags ---

func TestParseCheckFlags_Defaults(t *testing.T) {
	opts, files, hasStdin, code := parseCheckFlags(nil)
	assert.Equal(t, -1, code)
	assert.False(t, hasStdin)
	assert.Empty(t, files)
	assert.Equal(t, "text", opts.format)
	assert.False(t, opts.noColor)
	assert.False(t, opts.quiet)
	assert.False(t, opts.verbose)
	assert.False(t, opts.explain)
	assert.Empty(t, opts.configPath)
	assert.Empty(t, opts.maxInputSize)
	assert.Nil(t, opts.walk.followSymlinks)
	assert.False(t, opts.walk.noGitignore)
}

func TestParseCheckFlags_FilesAndStdin(t *testing.T) {
	opts, files, hasStdin, code := parseCheckFlags([]string{"a.md", "-", "b.md"})
	assert.Equal(t, -1, code)
	assert.True(t, hasStdin)
	assert.Equal(t, []string{"a.md", "b.md"}, files)
	assert.Empty(t, opts.configPath)
}

func TestParseCheckFlags_QuietSuppressesVerbose(t *testing.T) {
	opts, _, _, code := parseCheckFlags([]string{"-q", "-v"})
	assert.Equal(t, -1, code)
	assert.True(t, opts.quiet)
	assert.False(t, opts.verbose)
}

func TestParseCheckFlags_ConfigAndFormat(t *testing.T) {
	opts, _, _, code := parseCheckFlags([]string{"-c", "/path/cfg.yml", "-f", "json"})
	assert.Equal(t, -1, code)
	assert.Equal(t, "/path/cfg.yml", opts.configPath)
	assert.Equal(t, "json", opts.format)
}

func TestParseCheckFlags_FollowSymlinksOverride(t *testing.T) {
	opts, _, _, code := parseCheckFlags([]string{"--follow-symlinks=false"})
	assert.Equal(t, -1, code)
	require.NotNil(t, opts.walk.followSymlinks)
	assert.False(t, *opts.walk.followSymlinks)
}

func TestParseCheckFlags_ParseErrorReturnsTwo(t *testing.T) {
	stderr := captureStderr(func() {
		_, _, _, code := parseCheckFlags([]string{"--no-such-flag"})
		assert.Equal(t, 2, code)
	})
	assert.Contains(t, stderr, "mdsmith: check")
}

func TestParseCheckFlags_HelpReturnsZero(t *testing.T) {
	stderr := captureStderr(func() {
		_, _, _, code := parseCheckFlags([]string{"--help"})
		assert.Equal(t, 0, code)
	})
	assert.Contains(t, stderr, "Usage: mdsmith check")
}

// --- queryFiles ---

func TestQueryFiles_MatchingFile_ReturnsOneAndWritesPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.md")
	require.NoError(t, os.WriteFile(path, []byte("---\nstatus: done\n---\n# H\n\nContent here.\n"), 0644))

	matcher, err := query.Compile(`status: "done"`)
	require.NoError(t, err)

	out := captureStdout(func() {
		count := queryFiles(matcher, []string{path}, "\n", false, 0)
		assert.Equal(t, 1, count)
	})
	assert.Contains(t, out, path)
}

func TestQueryFiles_NonMatchingFile_ReturnsZero(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.md")
	require.NoError(t, os.WriteFile(path, []byte("---\nstatus: draft\n---\n# H\n\nContent here.\n"), 0644))

	matcher, err := query.Compile(`status: "done"`)
	require.NoError(t, err)

	out := captureStdout(func() {
		count := queryFiles(matcher, []string{path}, "\n", false, 0)
		assert.Equal(t, 0, count)
	})
	assert.Empty(t, out)
}

func TestQueryFiles_NullDelimiter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.md")
	require.NoError(t, os.WriteFile(path, []byte("---\nstatus: done\n---\n# H\n\nContent here.\n"), 0644))

	matcher, err := query.Compile(`status: "done"`)
	require.NoError(t, err)

	out := captureStdout(func() {
		queryFiles(matcher, []string{path}, "\x00", false, 0)
	})
	assert.True(t, strings.HasSuffix(out, "\x00"))
}

func TestQueryFiles_VerboseLogsNoFrontMatter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.md")
	require.NoError(t, os.WriteFile(path, []byte("# Just a heading\n"), 0644))

	matcher, err := query.Compile(`status: "done"`)
	require.NoError(t, err)

	var errOut string
	captureStdout(func() {
		errOut = captureStderr(func() {
			count := queryFiles(matcher, []string{path}, "\n", true, 0)
			assert.Equal(t, 0, count)
		})
	})
	assert.Contains(t, errOut, "no front matter")
}

func TestQueryFiles_VerboseLogsNonMatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.md")
	require.NoError(t, os.WriteFile(path, []byte("---\nstatus: draft\n---\n# H\n\nContent here.\n"), 0644))

	matcher, err := query.Compile(`status: "done"`)
	require.NoError(t, err)

	var errOut string
	captureStdout(func() {
		errOut = captureStderr(func() {
			queryFiles(matcher, []string{path}, "\n", true, 0)
		})
	})
	assert.Contains(t, errOut, "expression not satisfied")
}

func TestQueryFiles_FileReadError_SkipsFile(t *testing.T) {
	matcher, err := query.Compile(`status: "done"`)
	require.NoError(t, err)

	out := captureStdout(func() {
		count := queryFiles(matcher, []string{"/no/such/file.md"}, "\n", false, 0)
		assert.Equal(t, 0, count)
	})
	assert.Empty(t, out)
}

func TestQueryFiles_MultipleFiles(t *testing.T) {
	dir := t.TempDir()
	p1 := filepath.Join(dir, "a.md")
	p2 := filepath.Join(dir, "b.md")
	p3 := filepath.Join(dir, "c.md")
	require.NoError(t, os.WriteFile(p1, []byte("---\nstatus: done\n---\n# H\n\nContent.\n"), 0644))
	require.NoError(t, os.WriteFile(p2, []byte("---\nstatus: done\n---\n# H\n\nContent.\n"), 0644))
	require.NoError(t, os.WriteFile(p3, []byte("---\nstatus: draft\n---\n# H\n\nContent.\n"), 0644))

	matcher, err := query.Compile(`status: "done"`)
	require.NoError(t, err)

	out := captureStdout(func() {
		count := queryFiles(matcher, []string{p1, p2, p3}, "\n", false, 0)
		assert.Equal(t, 2, count)
	})
	assert.Contains(t, out, p1)
	assert.Contains(t, out, p2)
	assert.NotContains(t, out, p3)
}

// --- loadConfigRaw ---

func TestLoadConfigRaw_ExplicitPath_LoadsConfig(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".mdsmith.yml")
	require.NoError(t, os.WriteFile(cfgPath, []byte("rules: {}\n"), 0644))

	cfg, path, err := loadConfigRaw(cfgPath)
	require.NoError(t, err)
	assert.Equal(t, cfgPath, path)
	assert.NotNil(t, cfg)
}

func TestLoadConfigRaw_ExplicitPath_NotFound_Error(t *testing.T) {
	_, _, err := loadConfigRaw("/no/such/dir/.mdsmith.yml")
	assert.Error(t, err)
}

func TestLoadConfigRaw_InvalidYAML_Error(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".mdsmith.yml")
	require.NoError(t, os.WriteFile(cfgPath, []byte("rules: [bad:\n"), 0644))

	_, _, err := loadConfigRaw(cfgPath)
	assert.Error(t, err)
}

func TestLoadConfigRaw_EmptyPath_ReturnsNonNilConfig(t *testing.T) {
	cfg, _, err := loadConfigRaw("")
	require.NoError(t, err)
	assert.NotNil(t, cfg)
}

// --- runInit ---

func TestRunInit_ExtraArgs_ExitsTwo(t *testing.T) {
	captureStderr(func() {
		code := runInit([]string{"extra"})
		assert.Equal(t, 2, code)
	})
}

func TestRunInit_CreatesConfigFile(t *testing.T) {
	dir := t.TempDir()
	oldWd, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(oldWd) }()
	require.NoError(t, os.Chdir(dir))

	captureStderr(func() {
		code := runInit(nil)
		assert.Equal(t, 0, code)
	})

	data, err := os.ReadFile(filepath.Join(dir, ".mdsmith.yml"))
	require.NoError(t, err)
	// Verify it's parseable YAML
	var out map[string]any
	require.NoError(t, yaml.Unmarshal(data, &out))
}

func TestRunInit_AlreadyExists_ExitsTwo(t *testing.T) {
	dir := t.TempDir()
	oldWd, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(oldWd) }()
	require.NoError(t, os.Chdir(dir))
	require.NoError(t, os.WriteFile(".mdsmith.yml", []byte("rules: {}\n"), 0644))

	captureStderr(func() {
		code := runInit(nil)
		assert.Equal(t, 2, code)
	})
}

// --- runHelp ---

func TestRunHelp_NoArgs_ExitsZero(t *testing.T) {
	got := captureStderr(func() {
		code := runHelp(nil)
		assert.Equal(t, 0, code)
	})
	assert.Contains(t, got, "rule")
}

func TestRunHelp_UnknownTopic_ExitsTwo(t *testing.T) {
	got := captureStderr(func() {
		code := runHelp([]string{"bogus"})
		assert.Equal(t, 2, code)
	})
	assert.Contains(t, got, "unknown topic")
}

func TestRunHelp_RuleDispatch_ExitsZero(t *testing.T) {
	captureStdout(func() {
		code := runHelp([]string{"rule"})
		assert.Equal(t, 0, code)
	})
}

func TestRunHelp_MetricsDispatch_ExitsZero(t *testing.T) {
	out := captureStdout(func() {
		code := runHelp([]string{"metrics"})
		assert.Equal(t, 0, code)
	})
	assert.NotEmpty(t, out)
}

func TestRunHelp_KindsDispatch_ExitsZero(t *testing.T) {
	out := captureStdout(func() {
		code := runHelp([]string{"kinds"})
		assert.Equal(t, 0, code)
	})
	assert.Contains(t, out, "DECLARATION")
	assert.Contains(t, out, "kind-assignment")
}

func TestRunHelp_ConceptDispatch_ExitsZero(t *testing.T) {
	out := captureStdout(func() {
		code := runHelp([]string{"placeholder-grammar"})
		assert.Equal(t, 0, code)
	})
	assert.Contains(t, out, "Placeholder grammar")
}

func TestRunHelpConcept_UnknownConcept_ExitsTwo(t *testing.T) {
	code := runHelpConcept("no-such-concept")
	assert.Equal(t, 2, code)
}

// --- runHelpRule ---

func TestRunHelpRule_NoArgs_ListsRules(t *testing.T) {
	out := captureStdout(func() {
		code := runHelpRule(nil)
		assert.Equal(t, 0, code)
	})
	assert.NotEmpty(t, out)
}

func TestRunHelpRule_KnownID_ExitsZero(t *testing.T) {
	// Use a rule ID known to exist in the registry.
	out := captureStdout(func() {
		code := runHelpRule([]string{"no-trailing-spaces"})
		assert.Equal(t, 0, code)
	})
	assert.NotEmpty(t, out)
}

func TestRunHelpRule_UnknownRule_ExitsTwo(t *testing.T) {
	captureStdout(func() {
		captureStderr(func() {
			code := runHelpRule([]string{"no-such-rule"})
			assert.Equal(t, 2, code)
		})
	})
}

// --- listAllRules / showRule ---

func TestListAllRules_PrintsRows(t *testing.T) {
	out := captureStdout(func() {
		code := listAllRules()
		assert.Equal(t, 0, code)
	})
	assert.NotEmpty(t, out)
}

// TestListAllRules_ListError_ExitsTwo swaps the rule lister for a
// fault injection so the otherwise-unreachable ListRules error path
// is exercised.
func TestListAllRules_ListError_ExitsTwo(t *testing.T) {
	prev := listRulesForHelp
	listRulesForHelp = func() ([]ruledocs.RuleInfo, error) {
		return nil, fmt.Errorf("forced list failure")
	}
	defer func() { listRulesForHelp = prev }()

	var stderr string
	captureStdout(func() {
		stderr = captureStderr(func() {
			code := listAllRules()
			assert.Equal(t, 2, code)
		})
	})
	assert.Contains(t, stderr, "forced list failure")
}

func TestShowRule_KnownRule_PrintsContent(t *testing.T) {
	out := captureStdout(func() {
		code := showRule("no-trailing-spaces")
		assert.Equal(t, 0, code)
	})
	assert.NotEmpty(t, out)
}

func TestShowRule_UnknownRule_ExitsTwo(t *testing.T) {
	captureStdout(func() {
		captureStderr(func() {
			code := showRule("no-such-rule")
			assert.Equal(t, 2, code)
		})
	})
}

func TestRunHelpPatterns_JSON(t *testing.T) {
	out := captureStdout(func() {
		code := runHelpPatterns([]string{"-f", "json"})
		assert.Equal(t, 0, code)
	})
	var got []map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &got))
	require.NotEmpty(t, got, "expected at least one rule with maintainability")
	for _, item := range got {
		assert.NotEmpty(t, item["id"], "id must be non-empty")
		assert.NotEmpty(t, item["name"], "name must be non-empty")
		assert.NotEmpty(t, item["signal"], "signal must be non-empty")
		assert.NotEmpty(t, item["fix"], "fix must be non-empty")
		_, hasFlag := item["for-diagnostic"]
		assert.True(t, hasFlag, "for-diagnostic key must be present")
	}
}

func TestRunHelpPatterns_JSON_OmitsNullMaintainability(t *testing.T) {
	out := captureStdout(func() {
		code := runHelpPatterns([]string{"-f", "json"})
		assert.Equal(t, 0, code)
	})
	var got []map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &got))
	for _, item := range got {
		// line-length has maintainability: null and must be omitted.
		assert.NotEqual(t, "line-length", item["name"])
		assert.NotEqual(t, "MDS001", item["id"])
	}
}

func TestRunHelpPatterns_JSON_FlagLongForm(t *testing.T) {
	out := captureStdout(func() {
		code := runHelpPatterns([]string{"--format", "json"})
		assert.Equal(t, 0, code)
	})
	assert.True(t, strings.HasPrefix(strings.TrimSpace(out), "["),
		"--format json must produce a JSON array")
}

func TestRunHelpPatterns_TextDefault(t *testing.T) {
	out := captureStdout(func() {
		code := runHelpPatterns(nil)
		assert.Equal(t, 0, code)
	})
	assert.Contains(t, out, "signal:")
	assert.Contains(t, out, "fix:")
	assert.Contains(t, out, "for-diagnostic:")
}

// TestRunHelpPatterns_TextIncludesAllNonNullRules covers the closed set of
// rules expected to ship a non-null maintainability block and asserts that
// a rule with `maintainability: null` (MDS001) is absent from the text
// output. Adjust the expected set when adding new patterns.
func TestRunHelpPatterns_TextIncludesAllNonNullRules(t *testing.T) {
	out := captureStdout(func() {
		code := runHelpPatterns(nil)
		assert.Equal(t, 0, code)
	})
	expected := []string{
		"MDS019", "catalog",
		"MDS020", "required-structure",
		"MDS021", "include",
		"MDS033", "directory-structure",
		"MDS037", "duplicated-content",
	}
	for _, want := range expected {
		assert.Contains(t, out, want,
			"text output must include %q for the maintainability catalog", want)
	}
	assert.NotContains(t, out, "MDS001",
		"text output must omit MDS001 (maintainability: null)")
	assert.NotContains(t, out, "line-length",
		"text output must omit the line-length rule (maintainability: null)")
}

func TestRunHelpPatterns_TextIncludesKnownRule(t *testing.T) {
	out := captureStdout(func() {
		code := runHelpPatterns(nil)
		assert.Equal(t, 0, code)
	})
	// catalog (MDS019) carries a non-null maintainability block.
	assert.Contains(t, out, "MDS019")
	assert.Contains(t, out, "catalog")
}

func TestRunHelpPatterns_UnknownFormat_ExitsTwo(t *testing.T) {
	var stderr string
	captureStdout(func() {
		stderr = captureStderr(func() {
			code := runHelpPatterns([]string{"-f", "jsno"})
			assert.Equal(t, 2, code)
		})
	})
	assert.Contains(t, stderr, "unknown format")
	assert.Contains(t, stderr, "jsno")
}

func TestRunHelpPatterns_FormatFlagWithoutValue_ExitsTwo(t *testing.T) {
	var stderr string
	captureStdout(func() {
		stderr = captureStderr(func() {
			code := runHelpPatterns([]string{"-f"})
			assert.Equal(t, 2, code)
		})
	})
	assert.Contains(t, stderr, "requires a value")
}

func TestRunHelpPatterns_UnexpectedArg_ExitsTwo(t *testing.T) {
	var stderr string
	captureStdout(func() {
		stderr = captureStderr(func() {
			code := runHelpPatterns([]string{"garbage"})
			assert.Equal(t, 2, code)
		})
	})
	assert.Contains(t, stderr, "unexpected argument")
}

func TestRunHelpPatterns_TrailingArg_ExitsTwo(t *testing.T) {
	var stderr string
	captureStdout(func() {
		stderr = captureStderr(func() {
			code := runHelpPatterns([]string{"-f", "json", "extra"})
			assert.Equal(t, 2, code)
		})
	})
	assert.Contains(t, stderr, "unexpected trailing argument")
	assert.Contains(t, stderr, "extra")
}

// TestRunHelpPatterns_ListRulesError_ExitsTwo swaps the rule lister for a
// fault injection so the otherwise-unreachable ListRules error path is
// exercised — keeping behavior consistent with listAllRules, which also
// surfaces the same error and exits 2.
func TestRunHelpPatterns_ListRulesError_ExitsTwo(t *testing.T) {
	prev := listRulesForHelp
	listRulesForHelp = func() ([]ruledocs.RuleInfo, error) {
		return nil, fmt.Errorf("forced list failure")
	}
	defer func() { listRulesForHelp = prev }()

	var stderr string
	captureStdout(func() {
		stderr = captureStderr(func() {
			code := runHelpPatterns(nil)
			assert.Equal(t, 2, code)
		})
	})
	assert.Contains(t, stderr, "forced list failure")
}

// TestRunHelpPatterns_JSON_WriteError_ExitsTwo verifies that a stdout write
// failure during json encoding (e.g. broken pipe when the consumer hung up)
// surfaces as exit 2 with a clear error rather than a silent exit 0.
func TestRunHelpPatterns_JSON_WriteError_ExitsTwo(t *testing.T) {
	r, w, err := os.Pipe()
	require.NoError(t, err)
	require.NoError(t, r.Close()) // close read end so subsequent writes get EPIPE
	require.NoError(t, w.Close()) // close write end so encoder hits "file already closed"

	stderr := captureStderr(func() {
		oldStdout := os.Stdout
		os.Stdout = w
		defer func() { os.Stdout = oldStdout }()
		code := runHelpPatterns([]string{"-f", "json"})
		assert.Equal(t, 2, code)
	})
	assert.Contains(t, stderr, "writing json")
}

func TestRunHelp_PatternsTopicDispatches(t *testing.T) {
	out := captureStdout(func() {
		code := runHelp([]string{"patterns", "-f", "json"})
		assert.Equal(t, 0, code)
	})
	assert.True(t, strings.HasPrefix(strings.TrimSpace(out), "["),
		"runHelp must dispatch 'patterns' topic to runHelpPatterns")
}

func TestHelpUsageText_ListsPatternsTopic(t *testing.T) {
	assert.Contains(t, helpUsageText, "patterns",
		"help usage must advertise the patterns topic")
}

func TestShowRule_NullMaintainability_NoSection(t *testing.T) {
	out := captureStdout(func() {
		code := showRule("line-length")
		assert.Equal(t, 0, code)
	})
	assert.NotContains(t, out, "Maintainability pattern")
}

func TestShowRule_NonNullMaintainability_RendersSection(t *testing.T) {
	out := captureStdout(func() {
		// catalog (MDS019) declares a non-null maintainability block.
		code := showRule("catalog")
		assert.Equal(t, 0, code)
	})
	assert.Contains(t, out, "## Maintainability pattern")
	assert.Contains(t, out, "- Signal:")
	assert.Contains(t, out, "- Fix:")
	assert.Contains(t, out, "- For diagnostic:")
}

func TestShowRule_NonNullMaintainability_ByID(t *testing.T) {
	out := captureStdout(func() {
		code := showRule("MDS037")
		assert.Equal(t, 0, code)
	})
	assert.Contains(t, out, "## Maintainability pattern")
	// MDS037 (duplicated-content) is for-diagnostic: true.
	assert.Contains(t, out, "- For diagnostic: true")
}

// --- printDeprecations ---

func TestPrintDeprecations_NilConfig_NoPanic(t *testing.T) {
	// Guard: nil-safe.
	assert.NotPanics(t, func() { printDeprecations(nil) })
}

func TestPrintDeprecations_EmitsEachMessageAndClears(t *testing.T) {
	cfg := &config.Config{Deprecations: []string{"first", "second"}}
	stderr := captureStderr(func() { printDeprecations(cfg) })

	assert.Contains(t, stderr, "mdsmith: deprecated: first")
	assert.Contains(t, stderr, "mdsmith: deprecated: second")
	assert.Empty(t, cfg.Deprecations,
		"consumed deprecations must be cleared so a second call is a no-op")

	stderr2 := captureStderr(func() { printDeprecations(cfg) })
	assert.Empty(t, stderr2, "second call on the same cfg emits nothing")
}

// TestDispatch covers the subcommand router's terminal arms: the
// in-process "version" command and the unknown-command fallback. The
// run* delegations are exercised through each command's own e2e
// suite, which drives the real binary through dispatch.
func TestDispatch(t *testing.T) {
	assert.Equal(t, 0, dispatch("version", nil))
	var code int
	_ = captureStderr(func() { code = dispatch("totally-unknown", nil) })
	assert.Equal(t, 2, code)
}

// --- init config generation ---

func TestDefaultConfigBytes(t *testing.T) {
	data, err := defaultConfigBytes()
	require.NoError(t, err)

	s := string(data)
	assert.Contains(t, s, "rules:")
	assert.Contains(t, s, "front-matter: true")
	assert.Contains(t, s, "line-length")
}

func TestInitConfigBytes_EmptyFlagUsesDefaults(t *testing.T) {
	var buf bytes.Buffer
	data, source, err := initConfigBytes("", &buf)
	require.NoError(t, err)

	assert.Empty(t, source)
	assert.Contains(t, string(data), "rules:")
	assert.Empty(t, buf.String(), "defaults conversion emits no notes")
}

func TestInitConfigBytes_ConvertsNamedConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".markdownlint.json")
	require.NoError(t, os.WriteFile(path,
		[]byte(`{"MD013": {"line_length": 120}, "MD024": {"siblings_only": true}}`), 0o644))

	var buf bytes.Buffer
	data, source, err := initConfigBytes(path, &buf)
	require.NoError(t, err)

	assert.Equal(t, path, source)
	assert.Contains(t, string(data), "max: 120")
	assert.Contains(t, buf.String(), "siblings_only",
		"untranslated options are echoed as notes")
}

func TestConvertedConfigBytes_AutoDiscovers(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".markdownlint.yaml"),
		[]byte("MD041: false\n"), 0o644))
	t.Chdir(dir)

	var buf bytes.Buffer
	data, source, err := convertedConfigBytes("auto", &buf)
	require.NoError(t, err)

	assert.Equal(t, ".markdownlint.yaml", source)
	assert.Contains(t, string(data), "first-line-heading: false")
}

func TestConvertedConfigBytes_MissingFile(t *testing.T) {
	var buf bytes.Buffer
	_, _, err := convertedConfigBytes(filepath.Join(t.TempDir(), "nope.json"), &buf)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nope.json")
}

func TestConvertedConfigBytes_ParseError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".markdownlintrc")
	require.NoError(t, os.WriteFile(path, []byte(`{"MD013" true}`), 0o644))

	var buf bytes.Buffer
	_, _, err := convertedConfigBytes(path, &buf)
	require.Error(t, err)
	assert.Contains(t, err.Error(), ".markdownlintrc")
}
