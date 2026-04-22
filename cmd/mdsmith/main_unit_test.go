package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/query"
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
	assert.Equal(t, lint.DefaultMaxInputBytes, n)
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
	opts := resolveOpts(cfg, walkCLI{followSymlinks: true})
	assert.True(t, opts.FollowSymlinks)
}

func TestResolveOpts_ConfigFollowSymlinks_Propagated(t *testing.T) {
	cfg := &config.Config{FollowSymlinks: true}
	opts := resolveOpts(cfg, walkCLI{})
	assert.True(t, opts.FollowSymlinks)
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
