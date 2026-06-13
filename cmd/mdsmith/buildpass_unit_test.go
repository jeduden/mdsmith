package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	buildexec "github.com/jeduden/mdsmith/internal/build"
	"github.com/jeduden/mdsmith/internal/config"
)

// mockBuilder is a test-only Builder whose Build fn is injected per test.
type mockBuilder struct {
	fn func(ctx context.Context, target buildexec.Target) error
}

func (m *mockBuilder) Build(ctx context.Context, target buildexec.Target) error {
	return m.fn(ctx, target)
}

func (m *mockBuilder) BuildWithResult(
	ctx context.Context, target buildexec.Target, _ buildexec.Options,
) buildexec.Result {
	return buildexec.Result{Err: m.fn(ctx, target)}
}

// buildPassCfg returns a minimal *config.Config with the given recipe
// YAML snippet (already indented under recipes:) for buildpass unit tests.
func buildPassCfg(recipesYAML string) *config.Config {
	yml := []byte("build:\n  recipes:\n" + recipesYAML)
	cfg, err := config.ParseBytes(yml)
	if err != nil {
		panic("buildPassCfg: " + err.Error())
	}
	return cfg
}

func TestEnvIsSet_Truthiness(t *testing.T) {
	const name = "MDSMITH_TEST_TRUST_FLAG"
	truthy := []string{"1", "true", "yes", "on", "anything", " 1 ", "TRUE"}
	falsy := []string{"", "0", "false", "no", "off", "FALSE", " 0 ", "  "}
	for _, v := range truthy {
		t.Setenv(name, v)
		assert.True(t, envIsSet(name), "value %q should grant", v)
	}
	for _, v := range falsy {
		t.Setenv(name, v)
		assert.False(t, envIsSet(name), "value %q should not grant", v)
	}
	// Unset is falsy.
	require.NoError(t, os.Unsetenv(name))
	assert.False(t, envIsSet(name))
}

// trustRoot writes a .mdsmith.yml file and an identical trust marker in
// root so the build pass trust gate is satisfied. Unit tests that drive
// runBuildPass to actually execute a recipe call this; the file bytes are
// arbitrary (the gate only checks that config and marker match).
func trustRoot(t *testing.T, root string) {
	t.Helper()
	body := []byte("rules: {}\nbuild:\n  recipes: {}\n")
	require.NoError(t, os.WriteFile(filepath.Join(root, ".mdsmith.yml"), body, 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".mdsmith.yml.trust"), body, 0o600))
}

// buildPassDirective returns a minimal Markdown snippet with one
// <?build?> directive referencing the given recipe and output filename.
func buildPassDirective(recipe, output string) string {
	return "# Build\n\n" +
		"<?build\nrecipe: " + recipe + "\n" +
		"outputs:\n  - " + output + "\n?>\n" +
		"[" + output + "](" + output + ")\n" +
		"<?/build?>\n"
}

func TestCollectBuildTargets_NonExistentFile(t *testing.T) {
	root := t.TempDir()
	targets, errs := collectBuildTargets([]string{"/nonexistent/path.md"}, root, "", 0)
	assert.Empty(t, targets)
	assert.Len(t, errs, 1)
	assert.Contains(t, errs[0].Error(), "reading")
}

func TestCollectBuildTargets_FileWithDirective(t *testing.T) {
	root := t.TempDir()
	md := buildPassDirective("cp", "out.txt")
	p := filepath.Join(root, "doc.md")
	require.NoError(t, os.WriteFile(p, []byte(md), 0o644))

	targets, errs := collectBuildTargets([]string{p}, root, "", 0)
	assert.Empty(t, errs)
	require.Len(t, targets, 1)
	assert.Equal(t, "cp", targets[0].target.Recipe)
	assert.Equal(t, []string{"out.txt"}, targets[0].target.Outputs)
}

func TestCollectBuildTargets_RecipeFilter(t *testing.T) {
	root := t.TempDir()
	md := buildPassDirective("cp", "out.txt")
	p := filepath.Join(root, "doc.md")
	require.NoError(t, os.WriteFile(p, []byte(md), 0o644))

	targets, errs := collectBuildTargets([]string{p}, root, "other", 0)
	assert.Empty(t, errs)
	assert.Empty(t, targets, "recipe filter 'other' should exclude the 'cp' directive")
}

func TestRunBuildPass_DryRun(t *testing.T) {
	root := t.TempDir()
	cfg := buildPassCfg("    cp:\n      command: cp {inputs} {outputs}\n")
	cfgPath := filepath.Join(root, ".mdsmith.yml")

	md := buildPassDirective("cp", "out.txt")
	p := filepath.Join(root, "doc.md")
	require.NoError(t, os.WriteFile(p, []byte(md), 0o644))

	var buf strings.Builder
	code := runBuildPass(cfg, cfgPath, []string{p}, buildPassOpts{dryRun: true, timeout: time.Second}, &buf)
	assert.Equal(t, 0, code)
	assert.Contains(t, buf.String(), "STALE")
}

func TestRunBuildPass_NoTargets(t *testing.T) {
	root := t.TempDir()
	cfg := buildPassCfg("")
	cfgPath := filepath.Join(root, ".mdsmith.yml")

	// File with no <?build?> directive.
	p := filepath.Join(root, "plain.md")
	require.NoError(t, os.WriteFile(p, []byte("# Hello\n"), 0o644))

	var buf strings.Builder
	code := runBuildPass(cfg, cfgPath, []string{p}, buildPassOpts{timeout: time.Second}, &buf)
	assert.Equal(t, 0, code)
	assert.Empty(t, buf.String())
}

func TestRunBuildPass_NoTargetsWithReadError(t *testing.T) {
	root := t.TempDir()
	cfg := buildPassCfg("")
	cfgPath := filepath.Join(root, ".mdsmith.yml")

	var buf strings.Builder
	code := runBuildPass(cfg, cfgPath, []string{"/nonexistent/file.md"}, buildPassOpts{timeout: time.Second}, &buf)
	assert.Equal(t, 2, code)
	assert.Contains(t, buf.String(), "reading")
}

func TestRunBuildPass_IgnoresConfigIgnoredFiles(t *testing.T) {
	root := t.TempDir()
	// Config ignores "fixture/**".
	cfg := buildPassCfg("    cp:\n      command: cp {inputs} {outputs}\n")
	cfg.Ignore = []string{"fixture/**"}
	cfgPath := filepath.Join(root, ".mdsmith.yml")

	fixtureDir := filepath.Join(root, "fixture")
	require.NoError(t, os.MkdirAll(fixtureDir, 0o755))
	md := buildPassDirective("cp", "out.txt")
	p := filepath.Join(fixtureDir, "doc.md")
	require.NoError(t, os.WriteFile(p, []byte(md), 0o644))

	var buf strings.Builder
	code := runBuildPass(cfg, cfgPath, []string{p}, buildPassOpts{timeout: time.Second}, &buf)
	// No targets collected (file was ignored) → exit 0 with no output.
	assert.Equal(t, 0, code)
	assert.Empty(t, buf.String())
}

func TestBuildRecipeSpecs_Empty(t *testing.T) {
	cfg := &config.Config{}
	specs := buildRecipeSpecs(cfg)
	assert.Empty(t, specs)
}

func TestBuildRecipeSpecs_WithRecipe(t *testing.T) {
	cfg := buildPassCfg("    copy:\n      command: cp {inputs} {outputs}\n")
	specs := buildRecipeSpecs(cfg)
	require.Contains(t, specs, "copy")
	assert.Equal(t, "cp {inputs} {outputs}", specs["copy"].Command)
}

func TestRunBuildPass_OK(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("touch not available on Windows")
	}
	root := t.TempDir()
	trustRoot(t, root)
	cfg := buildPassCfg("    mk:\n      command: touch {outputs}\n")
	cfgPath := filepath.Join(root, ".mdsmith.yml")

	md := buildPassDirective("mk", "out.txt")
	p := filepath.Join(root, "doc.md")
	require.NoError(t, os.WriteFile(p, []byte(md), 0o644))

	var buf strings.Builder
	// timeout: 0 exercises the "timeout <= 0 → 30s default" branch.
	code := runBuildPass(cfg, cfgPath, []string{p}, buildPassOpts{timeout: 0}, &buf)
	assert.Equal(t, 0, code)
	assert.Contains(t, buf.String(), "OK")
	assert.FileExists(t, filepath.Join(root, "out.txt"))
}

func TestRunBuildPass_FAIL(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("false not available on Windows")
	}
	root := t.TempDir()
	trustRoot(t, root)
	cfg := buildPassCfg("    boom:\n      command: false\n")
	cfgPath := filepath.Join(root, ".mdsmith.yml")

	md := buildPassDirective("boom", "out.txt")
	p := filepath.Join(root, "doc.md")
	require.NoError(t, os.WriteFile(p, []byte(md), 0o644))

	var buf strings.Builder
	code := runBuildPass(cfg, cfgPath, []string{p}, buildPassOpts{timeout: time.Second}, &buf)
	assert.Equal(t, 2, code)
	assert.Contains(t, buf.String(), "FAIL")
	assert.NoFileExists(t, filepath.Join(root, "out.txt"))
}

func TestDirectiveParams_NonStructuralKey(t *testing.T) {
	params := map[string]string{
		"recipe":  "cp",
		"inputs":  "src.txt",
		"outputs": "dst.txt",
		"theme":   "dark",
	}
	got := directiveParams(params)
	assert.Equal(t, map[string]string{"theme": "dark"}, got)
}

func TestDirectiveParams_AllStructural(t *testing.T) {
	params := map[string]string{
		"recipe":  "cp",
		"outputs": "dst.txt",
	}
	got := directiveParams(params)
	assert.Empty(t, got)
}

func TestCollectBuildTargets_MultipleFilesSort(t *testing.T) {
	root := t.TempDir()
	md := buildPassDirective("cp", "out.txt")
	p1 := filepath.Join(root, "a_doc.md")
	p2 := filepath.Join(root, "b_doc.md")
	require.NoError(t, os.WriteFile(p1, []byte(md), 0o644))
	require.NoError(t, os.WriteFile(p2, []byte(md), 0o644))

	// Pass in reverse order; expect sorted by filename.
	targets, errs := collectBuildTargets([]string{p2, p1}, root, "", 0)
	assert.Empty(t, errs)
	require.Len(t, targets, 2)
	assert.Equal(t, p1, targets[0].file)
	assert.Equal(t, p2, targets[1].file)
}

func TestCollectBuildTargets_EmptyRecipeSkipped(t *testing.T) {
	root := t.TempDir()
	// Directive with an explicitly empty string recipe (not null, which would
	// cause ParseDirective to fail earlier): must be skipped at the recipe==""
	// check.
	md := "# Build\n\n" +
		"<?build\nrecipe: \"\"\noutputs:\n  - out.txt\n?>\n" +
		"[out.txt](out.txt)\n<?/build?>\n"
	p := filepath.Join(root, "doc.md")
	require.NoError(t, os.WriteFile(p, []byte(md), 0o644))

	targets, errs := collectBuildTargets([]string{p}, root, "", 0)
	assert.Empty(t, errs)
	assert.Empty(t, targets, "directive with empty recipe must be skipped")
}

func TestCollectBuildTargets_EmptyOutputsSkipped(t *testing.T) {
	root := t.TempDir()
	// Directive with recipe but no outputs: must be skipped silently.
	md := "# Build\n\n" +
		"<?build\nrecipe: cp\n?>\n" +
		"<?/build?>\n"
	p := filepath.Join(root, "doc.md")
	require.NoError(t, os.WriteFile(p, []byte(md), 0o644))

	targets, errs := collectBuildTargets([]string{p}, root, "", 0)
	assert.Empty(t, errs)
	assert.Empty(t, targets, "directive with no outputs must be skipped")
}

func TestCollectBuildTargets_TwoDirectivesSameFileSortedByLine(t *testing.T) {
	root := t.TempDir()
	// Two directives in the same file: sort must use line number.
	md := "# First\n\n" +
		"<?build\nrecipe: cp\noutputs:\n  - a.txt\n?>\n" +
		"[a.txt](a.txt)\n<?/build?>\n\n" +
		"# Second\n\n" +
		"<?build\nrecipe: cp\noutputs:\n  - b.txt\n?>\n" +
		"[b.txt](b.txt)\n<?/build?>\n"
	p := filepath.Join(root, "doc.md")
	require.NoError(t, os.WriteFile(p, []byte(md), 0o644))

	targets, errs := collectBuildTargets([]string{p}, root, "", 0)
	assert.Empty(t, errs)
	require.Len(t, targets, 2)
	assert.Less(t, targets[0].line, targets[1].line)
}

func TestCollectBuildTargets_MalformedDirectiveSkipped(t *testing.T) {
	root := t.TempDir()
	// Directive whose YAML body contains a non-string param value causes
	// ParseDirective to return nil with diagnostics — must be skipped.
	md := "# Build\n\n" +
		"<?build\nrecipe:\n  nested: map\noutputs:\n  - out.txt\n?>\n" +
		"[out.txt](out.txt)\n<?/build?>\n"
	p := filepath.Join(root, "doc.md")
	require.NoError(t, os.WriteFile(p, []byte(md), 0o644))

	targets, errs := collectBuildTargets([]string{p}, root, "", 0)
	assert.Empty(t, errs)
	assert.Empty(t, targets, "directive with non-string recipe param must be skipped")
}

// --- resolveDefaultInputs ---

func TestResolveDefaultInputs_ParamToken_Resolved(t *testing.T) {
	out := resolveDefaultInputs([]string{"{tape}"}, map[string]string{"tape": "demo.tape"})
	assert.Equal(t, []string{"demo.tape"}, out)
}

func TestResolveDefaultInputs_ParamToken_NotInParams_PassesThrough(t *testing.T) {
	// A {token} whose name is absent from params falls through as a literal.
	out := resolveDefaultInputs([]string{"{unknown}"}, map[string]string{})
	assert.Equal(t, []string{"{unknown}"}, out)
}

func TestResolveDefaultInputs_LiteralEntry(t *testing.T) {
	out := resolveDefaultInputs([]string{"assets/logo.svg"}, map[string]string{"tape": "demo.tape"})
	assert.Equal(t, []string{"assets/logo.svg"}, out)
}

func TestResolveDefaultInputs_Empty(t *testing.T) {
	assert.Nil(t, resolveDefaultInputs(nil, nil))
	assert.Nil(t, resolveDefaultInputs([]string{}, nil))
}

// --- loadBuildCache ---

func TestLoadBuildCache_NoCacheFlag_ReturnsEmpty(t *testing.T) {
	root := t.TempDir()
	var buf strings.Builder
	c := loadBuildCache(root, buildPassOpts{noCache: true}, &buf)
	assert.Empty(t, c.Entries)
	assert.Empty(t, buf.String(), "noCache must not print anything")
}

func TestLoadBuildCache_CorruptFile_ReturnsEmptyAndWarns(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".mdsmith"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(root, ".mdsmith", "build-cache.json"), []byte("{bad json"), 0o644))
	var buf strings.Builder
	c := loadBuildCache(root, buildPassOpts{}, &buf)
	assert.Empty(t, c.Entries, "corrupt cache should yield empty cache")
	assert.Contains(t, buf.String(), "stale")
}

// --- refreshCacheEntry ---

func TestRefreshCacheEntry_NoCache_IsNoop(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "src.txt"), []byte("x"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "out.txt"), []byte("x"), 0o644))
	stin := buildexec.StalenessInput{
		Target: buildexec.Target{
			Recipe:  "cp",
			Root:    root,
			Inputs:  []string{"src.txt"},
			Outputs: []string{"out.txt"},
		},
		Command: "cp {inputs} {outputs}",
	}
	entry, err := buildCacheEntry(stin, buildPassOpts{noCache: true}, false)
	require.NoError(t, err)
	assert.Nil(t, entry, "noCache must not produce a cache entry")
}

// --- targetVerdict with force + error ---

func TestTargetVerdict_ForceAndMissingInput_ReturnsError(t *testing.T) {
	root := t.TempDir()
	// No src.txt on disk; --build-force still resolves inputs to catch errors.
	stin := buildexec.StalenessInput{
		Target: buildexec.Target{
			Recipe:  "cp",
			Root:    root,
			Inputs:  []string{"absent.txt"},
			Outputs: []string{"out.txt"},
		},
		Command: "cp {inputs} {outputs}",
	}
	_, err := targetVerdict(stin, buildexec.NewCache(), buildPassOpts{force: true})
	require.Error(t, err, "--build-force must still surface missing-input errors")
}

func TestTargetVerdict_NoCacheAndMissingInput_ReturnsError(t *testing.T) {
	root := t.TempDir()
	stin := buildexec.StalenessInput{
		Target: buildexec.Target{
			Recipe:  "cp",
			Root:    root,
			Inputs:  []string{"absent.txt"},
			Outputs: []string{"out.txt"},
		},
		Command: "cp {inputs} {outputs}",
	}
	_, err := targetVerdict(stin, buildexec.NewCache(), buildPassOpts{noCache: true})
	require.Error(t, err, "--build-no-cache must still surface missing-input errors")
}

// --- dispatchTargets exit-code paths ---

func TestRunBuildPass_RebuildNoCache_ExitsZero(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("touch not available on Windows")
	}
	root := t.TempDir()
	trustRoot(t, root)
	cfg := buildPassCfg("    mk:\n      command: touch {outputs}\n")
	cfgPath := filepath.Join(root, ".mdsmith.yml")

	md := buildPassDirective("mk", "out.txt")
	p := filepath.Join(root, "doc.md")
	require.NoError(t, os.WriteFile(p, []byte(md), 0o644))

	var buf strings.Builder
	// --build-no-cache + success: must exit 0, must not write cache.
	code := runBuildPass(cfg, cfgPath, []string{p}, buildPassOpts{noCache: true, timeout: time.Second}, &buf)
	assert.Equal(t, 0, code)
	assert.Contains(t, buf.String(), "OK")
	_, err := os.Stat(filepath.Join(root, ".mdsmith", "build-cache.json"))
	assert.True(t, os.IsNotExist(err), "cache must not be written when --build-no-cache is set")
}

func TestRunBuildPass_CheckStale_FreshTarget_ExitsZero(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("touch not available on Windows")
	}
	root := t.TempDir()
	trustRoot(t, root)
	cfg := buildPassCfg("    mk:\n      command: touch {outputs}\n")
	cfgPath := filepath.Join(root, ".mdsmith.yml")

	md := buildPassDirective("mk", "out.txt")
	p := filepath.Join(root, "doc.md")
	require.NoError(t, os.WriteFile(p, []byte(md), 0o644))

	// Run once to build and prime the cache.
	var buf strings.Builder
	code := runBuildPass(cfg, cfgPath, []string{p}, buildPassOpts{timeout: time.Second}, &buf)
	require.Equal(t, 0, code)

	// Now check-stale: target is fresh, so exit 0.
	buf.Reset()
	code = runBuildPass(cfg, cfgPath, []string{p}, buildPassOpts{checkStale: true, timeout: time.Second}, &buf)
	assert.Equal(t, 0, code)
	assert.NotContains(t, buf.String(), "STALE")
}

func TestRunBuildPass_CheckStale_StaleTarget_ExitsTwo(t *testing.T) {
	root := t.TempDir()
	cfg := buildPassCfg("    mk:\n      command: touch {outputs}\n")
	cfgPath := filepath.Join(root, ".mdsmith.yml")

	md := buildPassDirective("mk", "out.txt")
	p := filepath.Join(root, "doc.md")
	require.NoError(t, os.WriteFile(p, []byte(md), 0o644))

	// No prior run — target is stale.
	var buf strings.Builder
	code := runBuildPass(cfg, cfgPath, []string{p}, buildPassOpts{checkStale: true, timeout: time.Second}, &buf)
	assert.Equal(t, 2, code)
	assert.Contains(t, buf.String(), "STALE")
}

func TestRunBuildPass_SaveCacheError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	if runtime.GOOS == "windows" {
		t.Skip("touch and chmod not reliable on Windows")
	}
	root := t.TempDir()
	trustRoot(t, root)
	cfg := buildPassCfg("    mk:\n      command: touch {outputs}\n")
	cfgPath := filepath.Join(root, ".mdsmith.yml")

	md := buildPassDirective("mk", "out.txt")
	p := filepath.Join(root, "doc.md")
	require.NoError(t, os.WriteFile(p, []byte(md), 0o644))

	// Create .mdsmith dir and make it unwritable so cache.Save fails.
	mdsmithDir := filepath.Join(root, ".mdsmith")
	require.NoError(t, os.MkdirAll(mdsmithDir, 0o500))
	t.Cleanup(func() { _ = os.Chmod(mdsmithDir, 0o755) })

	var buf strings.Builder
	code := runBuildPass(cfg, cfgPath, []string{p}, buildPassOpts{timeout: time.Second}, &buf)
	// Recipe ran (OK) but cache.Save failed → exit 2.
	assert.Equal(t, 2, code)
}

// TestRunBuildPass_ReadErrorWithSuccessfulBuild_ExitsTwo covers the
// len(errs) > 0 && code == 0 path: one file fails to read (goes into errs),
// another file's target builds successfully (code == 0), so the function
// returns 2 rather than masking the read error.
func TestRunBuildPass_ReadErrorWithSuccessfulBuild_ExitsTwo(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("touch not available on Windows")
	}
	root := t.TempDir()
	trustRoot(t, root)
	cfg := buildPassCfg("    mk:\n      command: touch {outputs}\n")
	cfgPath := filepath.Join(root, ".mdsmith.yml")

	md := buildPassDirective("mk", "out.txt")
	p := filepath.Join(root, "doc.md")
	require.NoError(t, os.WriteFile(p, []byte(md), 0o644))

	var buf strings.Builder
	// p is a valid file with a target; the nonexistent path becomes an error.
	// The build succeeds (code == 0), but len(errs) > 0 triggers return 2.
	code := runBuildPass(cfg, cfgPath, []string{p, "/nonexistent/ghost.md"},
		buildPassOpts{noCache: true, timeout: time.Second}, &buf)
	assert.Equal(t, 2, code)
	assert.Contains(t, buf.String(), "OK")
	assert.Contains(t, buf.String(), "reading")
}

// TestRunBuildPass_OverlappingOutputsReturnsTwo covers the detectOverlap error
// branch: two directives writing the same output file returns exit 2 before
// any recipe runs.
func TestRunBuildPass_OverlappingOutputsReturnsTwo(t *testing.T) {
	root := t.TempDir()
	cfg := buildPassCfg("    cp:\n      command: cp {inputs} {outputs}\n")
	cfgPath := filepath.Join(root, ".mdsmith.yml")

	md := "# A\n\n" +
		"<?build\nrecipe: cp\noutputs:\n  - out.txt\n?>\n[out.txt](out.txt)\n<?/build?>\n\n" +
		"# B\n\n" +
		"<?build\nrecipe: cp\noutputs:\n  - out.txt\n?>\n[out.txt](out.txt)\n<?/build?>\n"
	p := filepath.Join(root, "doc.md")
	require.NoError(t, os.WriteFile(p, []byte(md), 0o644))

	var buf strings.Builder
	code := runBuildPass(cfg, cfgPath, []string{p}, buildPassOpts{timeout: time.Second}, &buf)
	assert.Equal(t, 2, code)
	assert.Contains(t, buf.String(), "overlap")
}

// TestRunBuildPass_TrustDenied covers the !trust.Trusted branch: the build pass
// exits 2 with a trust-related message when no trust marker exists.
func TestRunBuildPass_TrustDenied(t *testing.T) {
	root := t.TempDir()
	// Write a config with no trust marker so CheckTrust returns not-trusted.
	cfgBody := []byte("build:\n  recipes:\n    mk:\n      command: touch {outputs}\n")
	cfgPath := filepath.Join(root, ".mdsmith.yml")
	require.NoError(t, os.WriteFile(cfgPath, cfgBody, 0o644))

	cfg := buildPassCfg("    mk:\n      command: touch {outputs}\n")

	md := buildPassDirective("mk", "out.txt")
	p := filepath.Join(root, "doc.md")
	require.NoError(t, os.WriteFile(p, []byte(md), 0o644))

	var buf strings.Builder
	// Not dryRun, not checkStale → trust gate runs; no marker → denied.
	code := runBuildPass(cfg, cfgPath, []string{p}, buildPassOpts{timeout: time.Second}, &buf)
	assert.Equal(t, 2, code)
	assert.Contains(t, buf.String(), "mdsmith:")
}

// TestRunBuildPass_TrustGate_EmptyCfgPath covers the cfgPath=="" branch inside
// the trust gate, which falls back to ConfigPathForRoot(root).
func TestRunBuildPass_TrustGate_EmptyCfgPath(t *testing.T) {
	root := t.TempDir()
	// Write a root .mdsmith.yml with no trust marker so CheckTrust denies.
	cfgBody := []byte("build:\n  recipes: {}\n")
	require.NoError(t, os.WriteFile(filepath.Join(root, ".mdsmith.yml"), cfgBody, 0o644))

	cfg := buildPassCfg("    mk:\n      command: touch {outputs}\n")

	md := buildPassDirective("mk", "out.txt")
	p := filepath.Join(root, "doc.md")
	require.NoError(t, os.WriteFile(p, []byte(md), 0o644))

	var buf strings.Builder
	// cfgPath="" triggers the fallback branch; no trust marker → denied.
	code := runBuildPass(cfg, "", []string{p}, buildPassOpts{timeout: time.Second}, &buf)
	assert.Equal(t, 2, code)
}

// TestDispatchOne_RefreshCacheEntryError covers the refreshCacheEntry error
// path inside dispatchOne. The mock builder creates the declared output and
// then replaces the input with a directory; when refreshCacheEntry calls
// RecordBuild it tries to hash the input and fails.
func TestDispatchOne_RefreshCacheEntryError(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "src.txt"), []byte("content"), 0o644))

	cfg := buildPassCfg("    cp:\n      command: cp {inputs} {outputs}\n")
	bt := buildTarget{
		file: "test.md",
		line: 1,
		target: buildexec.Target{
			Recipe:  "cp",
			Root:    root,
			Inputs:  []string{"src.txt"},
			Outputs: []string{"out.txt"},
		},
	}

	builder := &mockBuilder{fn: func(_ context.Context, target buildexec.Target) error {
		// Create the declared output so RecordBuild's resolveOutputs passes.
		_ = os.WriteFile(filepath.Join(root, "out.txt"), []byte("result"), 0o644)
		// Replace the input with a directory so hashFile fails inside RecordBuild.
		_ = os.Remove(filepath.Join(root, "src.txt"))
		_ = os.MkdirAll(filepath.Join(root, "src.txt"), 0o755)
		return nil
	}}

	cache := buildexec.NewCache()
	var buf strings.Builder
	outcome := dispatchOne(builder, bt, cfg, buildPassOpts{}, cache, time.Second, &buf)
	assert.Equal(t, outcomeFailed, outcome)
	assert.Contains(t, buf.String(), "FAIL")
}

// --- resolveHooks ---

func TestResolveHooks_Nil_ReturnsEmpty(t *testing.T) {
	assert.Empty(t, resolveHooks(nil))
}

func TestResolveHooks_EmptyCommand_Skipped(t *testing.T) {
	hooks := []config.HookCfg{{Command: ""}}
	assert.Empty(t, resolveHooks(hooks))
}

func TestResolveHooks_WithParamsAndName(t *testing.T) {
	hooks := []config.HookCfg{{
		Command: "scripts/wait {port}",
		Params:  map[string]string{"port": "3000"},
		Name:    "wait-server",
	}}
	result := resolveHooks(hooks)
	require.Len(t, result, 1)
	assert.Equal(t, []string{"scripts/wait", "3000"}, result[0].Tokens)
	assert.Equal(t, "wait-server", result[0].Name)
}

// --- allFresh ---

func TestAllFresh_Force_ReturnsFalse(t *testing.T) {
	assert.False(t, allFresh(nil, &config.Config{}, buildexec.NewCache(), buildPassOpts{force: true}))
}

func TestAllFresh_NoCache_ReturnsFalse(t *testing.T) {
	assert.False(t, allFresh(nil, &config.Config{}, buildexec.NewCache(), buildPassOpts{noCache: true}))
}

func TestAllFresh_EmptyTargets_ReturnsTrue(t *testing.T) {
	assert.True(t, allFresh(nil, &config.Config{}, buildexec.NewCache(), buildPassOpts{}))
}

func TestAllFresh_StaleTarget_ReturnsFalse(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "src.txt")
	require.NoError(t, os.WriteFile(src, []byte("content"), 0o644))
	cfg := buildPassCfg("    cp:\n      command: cp {inputs} {outputs}\n")
	bt := buildTarget{
		file: filepath.Join(root, "doc.md"),
		line: 1,
		target: buildexec.Target{
			Recipe:  "cp",
			Root:    root,
			Inputs:  []string{"src.txt"},
			Outputs: []string{"out.txt"},
		},
	}
	// Empty cache → target has never been built → Stale.
	assert.False(t, allFresh([]buildTarget{bt}, cfg, buildexec.NewCache(), buildPassOpts{}))
}

// --- listHooksForDryRun ---

func TestListHooksForDryRun_Empty_NoOutput(t *testing.T) {
	var buf strings.Builder
	listHooksForDryRun("before", nil, &buf)
	assert.Empty(t, buf.String())
}

func TestListHooksForDryRun_WithName(t *testing.T) {
	var buf strings.Builder
	listHooksForDryRun("before", []config.HookCfg{{Command: "make start", Name: "start-server"}}, &buf)
	assert.Contains(t, buf.String(), "start-server")
	assert.Contains(t, buf.String(), "DRY-RUN")
}

func TestListHooksForDryRun_WithoutName_UsesFirstToken(t *testing.T) {
	var buf strings.Builder
	listHooksForDryRun("after", []config.HookCfg{{Command: "make stop"}}, &buf)
	assert.Contains(t, buf.String(), "make")
	assert.Contains(t, buf.String(), "DRY-RUN")
}

// --- checkMDS040Gate ---

func TestCheckMDS040Gate_NoRecipeSafetyRule_ReturnsTrue(t *testing.T) {
	cfg := &config.Config{Rules: map[string]config.RuleCfg{}}
	var buf strings.Builder
	assert.True(t, checkMDS040Gate(cfg, "cfg.yml", &buf))
}

func TestCheckMDS040Gate_DisabledRule_ReturnsTrue(t *testing.T) {
	cfg := &config.Config{Rules: map[string]config.RuleCfg{
		"recipe-safety": {Enabled: false},
	}}
	var buf strings.Builder
	assert.True(t, checkMDS040Gate(cfg, "cfg.yml", &buf))
}

func TestCheckMDS040Gate_EnabledNoErrors_ReturnsTrue(t *testing.T) {
	root := t.TempDir()
	cfgPath := filepath.Join(root, ".mdsmith.yml")
	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"recipe-safety": {Enabled: true, Settings: map[string]any{
				"config-path": cfgPath,
				"recipes":     map[string]any{},
			}},
		},
	}
	var buf strings.Builder
	assert.True(t, checkMDS040Gate(cfg, cfgPath, &buf))
}

func TestCheckMDS040Gate_EnabledWithBadHook_ReturnsFalse(t *testing.T) {
	root := t.TempDir()
	cfgPath := filepath.Join(root, ".mdsmith.yml")
	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"recipe-safety": {Enabled: true, Settings: map[string]any{
				"config-path": cfgPath,
				"recipes":     map[string]any{},
				"hooks-before": []any{map[string]any{
					"command": "bash unsafe.sh",
				}},
			}},
		},
	}
	var buf strings.Builder
	assert.False(t, checkMDS040Gate(cfg, cfgPath, &buf))
	assert.Contains(t, buf.String(), "MDS040")
}

// --- dispatchWithHooks ---

func TestDispatchWithHooks_BeforeAndAfterHooksRun(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("touch not available on Windows")
	}
	root := t.TempDir()
	beforeSentinel := filepath.Join(root, "before.txt")
	afterSentinel := filepath.Join(root, "after.txt")
	cfg := &config.Config{
		Build: config.BuildConfig{
			Hooks: config.HooksCfg{
				Before: []config.HookCfg{{Command: "touch before.txt"}},
				After:  []config.HookCfg{{Command: "touch after.txt"}},
			},
		},
	}
	builder := &mockBuilder{fn: func(_ context.Context, _ buildexec.Target) error { return nil }}
	var buf strings.Builder
	code := dispatchWithHooks(builder, nil, cfg, root, buildPassOpts{}, buildexec.NewCache(), time.Second, nil, &buf)
	assert.Equal(t, 0, code)
	assert.FileExists(t, beforeSentinel, "before-hook must have run")
	assert.FileExists(t, afterSentinel, "after-hook must have run")
}

func TestDispatchWithHooks_BeforeHookFails_AbortsAfterHook(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("false command not available on Windows")
	}
	root := t.TempDir()
	afterSentinel := filepath.Join(root, "after.txt")
	cfg := &config.Config{
		Build: config.BuildConfig{
			Hooks: config.HooksCfg{
				Before: []config.HookCfg{{Command: "false"}},
				After:  []config.HookCfg{{Command: "touch after.txt"}},
			},
		},
	}
	builder := &mockBuilder{fn: func(_ context.Context, _ buildexec.Target) error { return nil }}
	var buf strings.Builder
	code := dispatchWithHooks(builder, nil, cfg, root, buildPassOpts{}, buildexec.NewCache(), time.Second, nil, &buf)
	assert.NotEqual(t, 0, code)
	assert.NoFileExists(t, afterSentinel, "after-hook must not run when before-hook fails")
}

func TestDispatchWithHooks_SkipHooksWhenFreshEmptyTargets(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("touch not available on Windows")
	}
	root := t.TempDir()
	beforeSentinel := filepath.Join(root, "before.txt")
	cfg := &config.Config{
		Build: config.BuildConfig{
			Hooks: config.HooksCfg{
				Before: []config.HookCfg{{Command: "touch before.txt"}},
			},
		},
	}
	builder := &mockBuilder{fn: func(_ context.Context, _ buildexec.Target) error { return nil }}
	var buf strings.Builder
	// With no targets, allFresh returns true → hooks are skipped.
	code := dispatchWithHooks(builder, nil, cfg, root,
		buildPassOpts{skipHooksWhenFresh: true}, buildexec.NewCache(), time.Second, nil, &buf)
	assert.Equal(t, 0, code)
	assert.NoFileExists(t, beforeSentinel, "hooks must be skipped when all targets are fresh")
}

// TestCheckMDS040Gate_ApplySettingsError_ReturnsFalse covers the error path
// (lines 68-71) when ApplySettings rejects an unknown settings key.
func TestCheckMDS040Gate_ApplySettingsError_ReturnsFalse(t *testing.T) {
	root := t.TempDir()
	cfgPath := filepath.Join(root, ".mdsmith.yml")
	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"recipe-safety": {Enabled: true, Settings: map[string]any{
				"unknown-key": "triggers-error",
			}},
		},
	}
	var buf strings.Builder
	assert.False(t, checkMDS040Gate(cfg, cfgPath, &buf))
	assert.Contains(t, buf.String(), "settings error")
}

// TestRunBuildPass_MDS040GateFails_Returns2 covers the gate-fail branch
// (lines 102-104) where checkMDS040Gate returns false inside runBuildPass.
func TestRunBuildPass_MDS040GateFails_Returns2(t *testing.T) {
	root := t.TempDir()
	cfgPath := filepath.Join(root, ".mdsmith.yml")
	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"recipe-safety": {Enabled: true, Settings: map[string]any{
				"config-path": cfgPath,
				"recipes":     map[string]any{},
				"hooks-before": []any{map[string]any{
					"command": "bash unsafe.sh",
				}},
			}},
		},
	}
	var buf strings.Builder
	code := runBuildPass(cfg, cfgPath, nil, buildPassOpts{}, &buf)
	assert.Equal(t, 2, code)
	assert.Contains(t, buf.String(), "MDS040")
}

// TestDispatchWithHooks_BeforeHookFail_CollectionErrors_Returns2 covers the
// branch (lines 178-180) where collection errors take priority over a
// before-hook failure.
func TestDispatchWithHooks_BeforeHookFail_CollectionErrors_Returns2(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("false command not available on Windows")
	}
	root := t.TempDir()
	cfg := &config.Config{
		Build: config.BuildConfig{
			Hooks: config.HooksCfg{
				Before: []config.HookCfg{{Command: "false", Name: "fail-hook"}},
			},
		},
	}
	builder := &mockBuilder{fn: func(_ context.Context, _ buildexec.Target) error { return nil }}
	var buf strings.Builder
	errs := []error{errors.New("collection error")}
	code := dispatchWithHooks(
		builder, nil, cfg, root, buildPassOpts{}, buildexec.NewCache(), time.Second, errs, &buf,
	)
	assert.Equal(t, 2, code, "collection errors must take priority over before-hook failure")
}

// TestDispatchWithHooks_AfterHookFails_ReturnsNonZero covers the afterCode
// path (lines 204-206) when an after-hook exits non-zero.
func TestDispatchWithHooks_AfterHookFails_ReturnsNonZero(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("false command not available on Windows")
	}
	root := t.TempDir()
	cfg := &config.Config{
		Build: config.BuildConfig{
			Hooks: config.HooksCfg{
				After: []config.HookCfg{{Command: "false", Name: "cleanup"}},
			},
		},
	}
	builder := &mockBuilder{fn: func(_ context.Context, _ buildexec.Target) error { return nil }}
	var buf strings.Builder
	code := dispatchWithHooks(
		builder, nil, cfg, root, buildPassOpts{}, buildexec.NewCache(), time.Second, nil, &buf,
	)
	assert.NotEqual(t, 0, code, "after-hook failure exit code must be propagated")
}

// --- S002: MDS040 gate hardening ---

// TestCheckMDS040Gate_DisabledRule_WithShellRecipe_ReturnsFalse is the RED
// test for S002: even when recipe-safety is disabled, a recipe using a shell
// interpreter must be rejected by checkMDS040Gate.
func TestCheckMDS040Gate_DisabledRule_WithShellRecipe_ReturnsFalse(t *testing.T) {
	root := t.TempDir()
	cfgPath := filepath.Join(root, ".mdsmith.yml")
	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"recipe-safety": {Enabled: false},
		},
		Build: config.BuildConfig{
			Recipes: map[string]config.RecipeCfg{
				"danger": {Command: "sh -c 'echo pwned'"},
			},
		},
	}
	var buf strings.Builder
	// The gate must reject this even though recipe-safety is disabled.
	assert.False(t, checkMDS040Gate(cfg, cfgPath, &buf))
	assert.Contains(t, buf.String(), "MDS040")
}

// TestCheckMDS040Gate_NoRule_WithShellRecipe_ReturnsFalse is the RED test for
// S002: even when recipe-safety is absent from config.Rules, a recipe using a
// shell interpreter must be rejected by checkMDS040Gate.
func TestCheckMDS040Gate_NoRule_WithShellRecipe_ReturnsFalse(t *testing.T) {
	root := t.TempDir()
	cfgPath := filepath.Join(root, ".mdsmith.yml")
	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{},
		Build: config.BuildConfig{
			Recipes: map[string]config.RecipeCfg{
				"danger": {Command: "bash unsafe.sh"},
			},
		},
	}
	var buf strings.Builder
	// The gate must reject this even though recipe-safety is absent.
	assert.False(t, checkMDS040Gate(cfg, cfgPath, &buf))
	assert.Contains(t, buf.String(), "MDS040")
}

// TestCheckMDS040Gate_DisabledRule_NoRecipes_ReturnsTrue verifies that the
// existing bypass (no recipes) still opens the gate when recipe-safety is
// disabled — this is safe because there are no recipes to execute.
func TestCheckMDS040Gate_DisabledRule_NoRecipes_ReturnsTrue(t *testing.T) {
	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"recipe-safety": {Enabled: false},
		},
		Build: config.BuildConfig{
			Recipes: map[string]config.RecipeCfg{},
		},
	}
	var buf strings.Builder
	assert.True(t, checkMDS040Gate(cfg, "cfg.yml", &buf))
}

// TestCheckMDS040Gate_DisabledRule_HooksOnly_ReturnsFalse verifies that a
// project declaring only hooks (no recipes) with recipe-safety disabled is
// still rejected by checkMDS040Gate — hooks are executable surfaces too.
func TestCheckMDS040Gate_DisabledRule_HooksOnly_ReturnsFalse(t *testing.T) {
	root := t.TempDir()
	cfgPath := filepath.Join(root, ".mdsmith.yml")
	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"recipe-safety": {Enabled: false},
		},
		Build: config.BuildConfig{
			Recipes: map[string]config.RecipeCfg{},
			Hooks: config.HooksCfg{
				Before: []config.HookCfg{{Command: "sh -c 'echo pwned'"}},
			},
		},
	}
	var buf strings.Builder
	assert.False(t, checkMDS040Gate(cfg, cfgPath, &buf))
}

// TestCheckMDS040Gate_DisabledRule_NoRecipesNoHooks_ReturnsTrue verifies that
// the gate stays open when there is nothing to execute.
func TestCheckMDS040Gate_DisabledRule_NoRecipesNoHooks_ReturnsTrue(t *testing.T) {
	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"recipe-safety": {Enabled: false},
		},
		Build: config.BuildConfig{
			Recipes: map[string]config.RecipeCfg{},
			Hooks:   config.HooksCfg{},
		},
	}
	var buf strings.Builder
	assert.True(t, checkMDS040Gate(cfg, "cfg.yml", &buf))
}

// TestCheckMDS040Gate_DisabledRule_AfterHookOnly_ReturnsFalse covers the
// noHooks right-hand side (Hooks.After non-empty, Hooks.Before empty), so
// the short-circuit of len(Before)==0 && len(After)==0 is fully exercised.
func TestCheckMDS040Gate_DisabledRule_AfterHookOnly_ReturnsFalse(t *testing.T) {
	root := t.TempDir()
	cfgPath := filepath.Join(root, ".mdsmith.yml")
	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"recipe-safety": {Enabled: false},
		},
		Build: config.BuildConfig{
			Recipes: map[string]config.RecipeCfg{},
			Hooks: config.HooksCfg{
				After: []config.HookCfg{{Command: "sh -c 'echo pwned'"}},
			},
		},
	}
	var buf strings.Builder
	assert.False(t, checkMDS040Gate(cfg, cfgPath, &buf))
}

// TestCheckMDS040Gate_DisabledRule_EmptyCfgPath_ReturnsFalse covers the
// cfgPath=="" branch inside the else block (disabled rule, non-empty recipes).
func TestCheckMDS040Gate_DisabledRule_EmptyCfgPath_ReturnsFalse(t *testing.T) {
	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"recipe-safety": {Enabled: false},
		},
		Build: config.BuildConfig{
			Recipes: map[string]config.RecipeCfg{
				"danger": {Command: "sh -c 'echo pwned'"},
			},
		},
	}
	var buf strings.Builder
	assert.False(t, checkMDS040Gate(cfg, "", &buf))
}

// --- TestAllFresh_CheckStalenessError_ReturnsFalse covers the error branch
// (lines 245-247) when CheckStaleness returns an error (glob matching no files).
func TestAllFresh_CheckStalenessError_ReturnsFalse(t *testing.T) {
	root := t.TempDir()
	cfg := buildPassCfg("    cp:\n      command: cp {inputs} {outputs}\n")
	bt := buildTarget{
		file: filepath.Join(root, "doc.md"),
		line: 1,
		target: buildexec.Target{
			Recipe:  "cp",
			Root:    root,
			Inputs:  []string{"*.nonexistent"},
			Outputs: []string{"out.txt"},
		},
	}
	// A glob matching no files causes CheckStaleness to return an error;
	// allFresh must return false rather than true.
	assert.False(t, allFresh([]buildTarget{bt}, cfg, buildexec.NewCache(), buildPassOpts{}))
}

// --- buildFixFlags.conflict ---

func TestConflict_NoBuildAndBuildOnly_Conflict(t *testing.T) {
	b := buildFixFlags{noBuild: true, buildOnly: true}
	assert.NotEmpty(t, b.conflict())
}

func TestConflict_ForceAndCheckStale_Conflict(t *testing.T) {
	b := buildFixFlags{force: true, checkStale: true}
	assert.NotEmpty(t, b.conflict())
}

func TestConflict_ForceAndNoCache_Conflict(t *testing.T) {
	b := buildFixFlags{force: true, noCache: true}
	assert.NotEmpty(t, b.conflict())
}

func TestConflict_ExplainAndVerify_Conflict(t *testing.T) {
	b := buildFixFlags{explain: "out.txt", verify: true}
	assert.NotEmpty(t, b.conflict())
}

func TestConflict_ExplainAndDryRun_Conflict(t *testing.T) {
	b := buildFixFlags{explain: "out.txt", dryRun: true}
	assert.NotEmpty(t, b.conflict())
}

func TestConflict_ExplainAndCheckStale_Conflict(t *testing.T) {
	b := buildFixFlags{explain: "out.txt", checkStale: true}
	assert.NotEmpty(t, b.conflict())
}

func TestConflict_VerifyAndDryRun_Conflict(t *testing.T) {
	b := buildFixFlags{verify: true, dryRun: true}
	assert.NotEmpty(t, b.conflict())
}

func TestConflict_VerifyAndCheckStale_Conflict(t *testing.T) {
	b := buildFixFlags{verify: true, checkStale: true}
	assert.NotEmpty(t, b.conflict())
}

func TestConflict_JobsLessThanOne_Conflict(t *testing.T) {
	b := buildFixFlags{jobs: 0}
	assert.NotEmpty(t, b.conflict())
}

func TestConflict_ValidFlags_NoConflict(t *testing.T) {
	b := buildFixFlags{jobs: 2, explain: "out.txt"}
	assert.Empty(t, b.conflict())
}

// --- buildFixFlags.toPassOpts ---

func TestToPassOpts_MapsAllNewFlags(t *testing.T) {
	b := buildFixFlags{
		noBuild:            true,
		buildOnly:          false,
		dryRun:             true,
		force:              false,
		checkStale:         false,
		noCache:            true,
		recipe:             "copy",
		timeout:            5 * time.Second,
		noHooks:            true,
		skipHooksWhenFresh: true,
		stream:             true,
		verify:             false,
		jobs:               4,
		explain:            "out.txt",
	}
	opts := b.toPassOpts()
	assert.True(t, opts.noBuild)
	assert.False(t, opts.buildOnly)
	assert.True(t, opts.dryRun)
	assert.False(t, opts.force)
	assert.False(t, opts.checkStale)
	assert.True(t, opts.noCache)
	assert.Equal(t, "copy", opts.recipe)
	assert.Equal(t, 5*time.Second, opts.timeout)
	assert.True(t, opts.noHooks)
	assert.True(t, opts.skipHooksWhenFresh)
	assert.True(t, opts.stream)
	assert.False(t, opts.verify)
	assert.Equal(t, 4, opts.jobs)
	assert.Equal(t, "out.txt", opts.explain)
}

func TestToPassOpts_VerifyAndStreamMapped(t *testing.T) {
	b := buildFixFlags{
		verify: true,
		stream: true,
		jobs:   8,
	}
	opts := b.toPassOpts()
	assert.True(t, opts.verify)
	assert.True(t, opts.stream)
	assert.Equal(t, 8, opts.jobs)
}

// TestBuildFixFlags_Register_RegistersNewFlags covers the register() lines
// that wire up --build-stream, --build-verify, --build-jobs, --build-explain.
func TestBuildFixFlags_Register_RegistersNewFlags(t *testing.T) {
	var b buildFixFlags
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	b.register(fs)

	require.NoError(t, fs.Parse([]string{
		"--build-stream",
		"--build-verify",
		"--build-jobs", "4",
		"--build-explain", "out.txt",
	}))
	assert.True(t, b.stream)
	assert.True(t, b.verify)
	assert.Equal(t, 4, b.jobs)
	assert.Equal(t, "out.txt", b.explain)
}

// --- runBuildPass extra branches ---

// TestRunBuildPass_Explain_ReturnsResult covers the opts.explain != "" branch
// in runBuildPass (lines 136-138): when explain is set, explainTarget is called
// and its exit code is returned.
func TestRunBuildPass_Explain_ReturnsResult(t *testing.T) {
	root := t.TempDir()
	cfg := buildPassCfg("    cp:\n      command: cp {inputs} {outputs}\n")
	cfgPath := filepath.Join(root, ".mdsmith.yml")
	md := buildPassDirective("cp", "out.txt")
	p := filepath.Join(root, "doc.md")
	require.NoError(t, os.WriteFile(p, []byte(md), 0o644))

	var buf strings.Builder
	// "out.txt" matches the target — explainTarget returns 0.
	code := runBuildPass(cfg, cfgPath, []string{p}, buildPassOpts{explain: "out.txt"}, &buf)
	assert.Equal(t, 0, code)
	assert.Contains(t, buf.String(), "out.txt")
}

// TestRunBuildPass_PruneOrphanLogsError_ContinuesAndWarns covers the
// pruneOrphanLogsFn error branch in runBuildPass: when log pruning fails,
// a warning is printed but the build still proceeds.
func TestRunBuildPass_PruneOrphanLogsError_ContinuesAndWarns(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("touch not available on Windows")
	}
	root := t.TempDir()
	trustRoot(t, root)
	cfg := buildPassCfg("    mk:\n      command: touch {outputs}\n")
	cfgPath := filepath.Join(root, ".mdsmith.yml")
	md := buildPassDirective("mk", "out.txt")
	p := filepath.Join(root, "doc.md")
	require.NoError(t, os.WriteFile(p, []byte(md), 0o644))

	// Inject a failure into pruneOrphanLogsFn.
	orig := pruneOrphanLogsFn
	pruneOrphanLogsFn = func(_ string, _ *buildexec.Cache) error {
		return errors.New("injected prune failure")
	}
	t.Cleanup(func() { pruneOrphanLogsFn = orig })

	var buf strings.Builder
	// Build should still succeed despite the log pruning error.
	code := runBuildPass(cfg, cfgPath, []string{p}, buildPassOpts{timeout: time.Second}, &buf)
	assert.Equal(t, 0, code)
	assert.Contains(t, buf.String(), "injected prune failure")
}

// TestRunBuildPass_JobsGreaterThanOne_RunsConcurrently covers the jobs > 1 branch
// in dispatchTargets (line 375): when jobs is set > 1, runConcurrent is used.
func TestRunBuildPass_JobsGreaterThanOne_RunsConcurrently(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("touch not available on Windows")
	}
	root := t.TempDir()
	trustRoot(t, root)
	cfg := buildPassCfg("    mk:\n      command: touch {outputs}\n")
	cfgPath := filepath.Join(root, ".mdsmith.yml")
	md := buildPassDirective("mk", "out.txt")
	p := filepath.Join(root, "doc.md")
	require.NoError(t, os.WriteFile(p, []byte(md), 0o644))

	var buf strings.Builder
	code := runBuildPass(cfg, cfgPath, []string{p}, buildPassOpts{jobs: 2, timeout: time.Second}, &buf)
	assert.Equal(t, 0, code)
	assert.Contains(t, buf.String(), "OK")
	assert.FileExists(t, filepath.Join(root, "out.txt"))
}

// --- targetName ---

// TestTargetName_WithOutputs covers the first branch (len > 0 path).
func TestTargetName_WithOutputs(t *testing.T) {
	bt := buildTarget{
		target: buildexec.Target{Outputs: []string{"out.txt", "other.txt"}},
	}
	assert.Equal(t, "out.txt", targetName(bt))
}

// TestTargetName_NoOutputs covers the fallback branch (no outputs declared).
func TestTargetName_NoOutputs(t *testing.T) {
	bt := buildTarget{file: "docs/guide.md", line: 42}
	assert.Equal(t, "docs/guide.md:42", targetName(bt))
}

// --- decideAndRun extra branches ---

// TestDispatchOne_VerdictError_OutcomeFailed covers the verdictErr != nil branch
// in decideAndRun: when targetVerdict returns an error, dispatchOne returns
// outcomeFailed and prints a FAIL line.
func TestDispatchOne_VerdictError_OutcomeFailed(t *testing.T) {
	root := t.TempDir()
	cfg := buildPassCfg("    cp:\n      command: cp {inputs} {outputs}\n")
	// "absent.txt" does not exist on disk — resolveInputs returns an error.
	bt := buildTarget{
		file: "doc.md",
		line: 1,
		target: buildexec.Target{
			Recipe:  "cp",
			Root:    root,
			Inputs:  []string{"absent.txt"},
			Outputs: []string{"out.txt"},
		},
	}
	builder := &mockBuilder{fn: func(_ context.Context, _ buildexec.Target) error { return nil }}
	cache := buildexec.NewCache()
	var buf strings.Builder
	outcome := dispatchOne(builder, bt, cfg, buildPassOpts{}, cache, time.Second, &buf)
	assert.Equal(t, outcomeFailed, outcome)
	assert.Contains(t, buf.String(), "FAIL")
}

// TestDispatchOne_VerifyEnabled_RunsTwice covers the opts.verify branch in
// decideAndRun: when verify is set and the recipe succeeds, verifyTarget is
// called (running the recipe twice).
func TestDispatchOne_VerifyEnabled_RunsTwice(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("touch not available on Windows")
	}
	root := t.TempDir()
	cfg := buildPassCfg("    mk:\n      command: touch {outputs}\n")
	md := buildPassDirective("mk", "out.txt")
	p := filepath.Join(root, "doc.md")
	require.NoError(t, os.WriteFile(p, []byte(md), 0o644))

	targets, errs := collectBuildTargets([]string{p}, root, "", 0)
	require.Empty(t, errs)
	require.Len(t, targets, 1)

	builder := buildexec.NewCustomBuilder(buildRecipeSpecs(cfg))
	cache := buildexec.NewCache()
	var buf strings.Builder
	outcome := dispatchOne(builder, targets[0], cfg, buildPassOpts{verify: true, timeout: time.Second},
		cache, time.Second, &buf)
	assert.Equal(t, outcomeRebuilt, outcome)
}

// TestDispatchOne_StreamEnabled_LiveForwards covers the opts.stream branch in
// runOneTarget: when stream is set, recipe stdout/stderr is forwarded to w live.
func TestDispatchOne_StreamEnabled_LiveForwards(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("echo not available on Windows")
	}
	root := t.TempDir()
	bindir := t.TempDir()
	script := writeShScript(t, bindir, "emit.sh", `echo "live output"; touch "$1"`)
	cfg := buildPassCfg("    emit:\n      command: " + script + " {outputs}\n")
	bt := buildTarget{
		file: "doc.md",
		line: 1,
		target: buildexec.Target{
			Recipe:  "emit",
			Root:    root,
			Outputs: []string{"out.txt"},
		},
	}
	builder := buildexec.NewCustomBuilder(map[string]buildexec.RecipeSpec{
		"emit": {Command: script + " {outputs}"},
	})
	cache := buildexec.NewCache()
	var buf strings.Builder
	outcome := dispatchOne(builder, bt, cfg, buildPassOpts{stream: true, timeout: time.Second},
		cache, time.Second, &buf)
	assert.Equal(t, outcomeRebuilt, outcome)
	assert.Contains(t, buf.String(), "live output")
}
