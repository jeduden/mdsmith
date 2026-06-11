package main_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// buildDirective renders a <?build?> generated section whose body matches
// the default body-template for a single output, so the lint pass stays
// green around the directive.
func buildDirective(recipe, input, output string) string {
	inputsBlock := ""
	if input != "" {
		inputsBlock = "inputs:\n  - " + input + "\n"
	}
	return "# Build\n\n" +
		"<?build\nrecipe: " + recipe + "\n" + inputsBlock +
		"outputs:\n  - " + output + "\n?>\n" +
		"[" + output + "](" + output + ")\n" +
		"<?/build?>\n"
}

// writeBuildRepo sets up an isolated repo with a .mdsmith.yml carrying
// the given build.recipes YAML block (already indented under recipes:).
func writeBuildRepo(t *testing.T, recipesYAML string) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".git"), 0o755))
	cfg := "rules: {}\nbuild:\n  recipes:\n" + recipesYAML
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".mdsmith.yml"), []byte(cfg), 0o644))
	return dir
}

func TestE2E_Build_SingleOutputCp(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("cp is not available on Windows")
	}
	dir := writeBuildRepo(t, "    copy:\n      command: cp {inputs} {outputs}\n")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src.txt"), []byte("hello"), 0o644))
	writeFixture(t, dir, "doc.md", buildDirective("copy", "src.txt", "dst.txt"))

	stdout, stderr, code := runBinaryInDir(t, dir, "", "fix", "--no-color", "doc.md")
	out := stdout + stderr
	assert.Equal(t, 0, code, "fix should succeed: %s", out)
	assert.Contains(t, out, "OK")

	got, err := os.ReadFile(filepath.Join(dir, "dst.txt"))
	require.NoError(t, err)
	assert.Equal(t, "hello", string(got))
}

func TestE2E_Build_MultiOutputTee(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sh/tee not available on Windows")
	}
	dir := writeBuildRepo(t, "    dup:\n      command: tee {outputs}\n")
	// A <?build?> with two outputs. We feed stdin via the recipe being
	// tee — but the build pass attaches no stdin, so tee writes empty
	// files. That is fine: we assert both outputs exist and are atomic.
	doc := "# Build\n\n" +
		"<?build\nrecipe: dup\noutputs:\n  - a.txt\n  - b.txt\n?>\n" +
		"[a.txt](a.txt)\n[b.txt](b.txt)\n" +
		"<?/build?>\n"
	writeFixture(t, dir, "doc.md", doc)

	_, stderr, code := runBinaryInDir(t, dir, "", "fix", "--no-color", "--build-only", "doc.md")
	assert.Equal(t, 0, code, "build-only should succeed: %s", stderr)
	assert.FileExists(t, filepath.Join(dir, "a.txt"))
	assert.FileExists(t, filepath.Join(dir, "b.txt"))
}

func TestE2E_Build_FailingRecipeLeavesNoPartialOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("false is not available on Windows")
	}
	dir := writeBuildRepo(t, "    boom:\n      command: false {outputs}\n")
	writeFixture(t, dir, "doc.md", buildDirective("boom", "", "out.txt"))

	_, stderr, code := runBinaryInDir(t, dir, "", "fix", "--no-color", "--build-only", "doc.md")
	assert.Equal(t, 2, code, "a failing recipe exits non-zero")
	assert.Contains(t, stderr, "FAIL")
	assert.NoFileExists(t, filepath.Join(dir, "out.txt"))
}

func TestE2E_Build_NoBuildSkipsBuildPass(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("cp is not available on Windows")
	}
	dir := writeBuildRepo(t, "    copy:\n      command: cp {inputs} {outputs}\n")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src.txt"), []byte("hi"), 0o644))
	writeFixture(t, dir, "doc.md", buildDirective("copy", "src.txt", "dst.txt"))

	_, _, code := runBinaryInDir(t, dir, "", "fix", "--no-color", "--no-build", "doc.md")
	assert.Equal(t, 0, code)
	assert.NoFileExists(t, filepath.Join(dir, "dst.txt"), "--no-build must not run the recipe")
}

func TestE2E_Build_NoBuildAndBuildOnlyConflict(t *testing.T) {
	dir := writeBuildRepo(t, "    copy:\n      command: cp {inputs} {outputs}\n")
	writeFixture(t, dir, "doc.md", buildDirective("copy", "", "dst.txt"))

	_, stderr, code := runBinaryInDir(t, dir, "", "fix", "--no-build", "--build-only", "doc.md")
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "mutually exclusive")
}

func TestE2E_Build_DryRunRunsNothing(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("cp is not available on Windows")
	}
	dir := writeBuildRepo(t, "    copy:\n      command: cp {inputs} {outputs}\n")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src.txt"), []byte("hi"), 0o644))
	writeFixture(t, dir, "doc.md", buildDirective("copy", "src.txt", "dst.txt"))

	_, stderr, code := runBinaryInDir(t, dir, "", "fix", "--no-color", "--build-dry-run", "--build-only", "doc.md")
	assert.Equal(t, 0, code)
	assert.Contains(t, stderr, "DRY-RUN")
	assert.NoFileExists(t, filepath.Join(dir, "dst.txt"), "--build-dry-run must not run the recipe")
}

func TestE2E_Build_RecipeFilter(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("cp is not available on Windows")
	}
	recipes := "    copy:\n      command: cp {inputs} {outputs}\n" +
		"    copy2:\n      command: cp {inputs} {outputs}\n"
	dir := writeBuildRepo(t, recipes)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src.txt"), []byte("x"), 0o644))
	doc := buildDirective("copy", "src.txt", "a.txt") + "\n" +
		"<?build\nrecipe: copy2\ninputs:\n  - src.txt\noutputs:\n  - b.txt\n?>\n" +
		"[b.txt](b.txt)\n<?/build?>\n"
	writeFixture(t, dir, "doc.md", doc)

	_, _, code := runBinaryInDir(t, dir, "", "fix", "--no-color", "--build-only", "--build-recipe", "copy", "doc.md")
	assert.Equal(t, 0, code)
	assert.FileExists(t, filepath.Join(dir, "a.txt"))
	assert.NoFileExists(t, filepath.Join(dir, "b.txt"), "--build-recipe copy must skip copy2 directives")
}

func TestE2E_Build_TimeoutKillsRecipe(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sleep is not available on Windows")
	}
	dir := writeBuildRepo(t, "    slow:\n      command: sleep 30\n")
	writeFixture(t, dir, "doc.md", buildDirective("slow", "", "out.txt"))

	_, stderr, code := runBinaryInDir(t, dir, "", "fix", "--no-color",
		"--build-only", "--build-timeout", "200ms", "doc.md")
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "FAIL")
}

func TestE2E_Build_LintDryRunSkipsBuildPass(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("cp is not available on Windows")
	}
	dir := writeBuildRepo(t, "    copy:\n      command: cp {inputs} {outputs}\n")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src.txt"), []byte("hi"), 0o644))
	writeFixture(t, dir, "doc.md", buildDirective("copy", "src.txt", "dst.txt"))

	_, _, code := runBinaryInDir(t, dir, "", "fix", "--no-color", "--dry-run", "doc.md")
	assert.Equal(t, 0, code)
	assert.NoFileExists(t, filepath.Join(dir, "dst.txt"),
		"a lint --dry-run preview must not run any recipe")
}

func TestE2E_Build_LintViolationsKeepNonZeroExit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("cp is not available on Windows")
	}
	dir := writeBuildRepo(t, "    copy:\n      command: cp {inputs} {outputs}\n")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src.txt"), []byte("hi"), 0o644))
	// A broken .md link is an unfixable diagnostic, so the lint pass
	// exits 1 while the build pass still succeeds — the combined run
	// must keep the lint exit code rather than masking it with build OK.
	doc := buildDirective("copy", "src.txt", "dst.txt") +
		"\nSee [missing](missing-page.md) for details.\n"
	writeFixture(t, dir, "doc.md", doc)

	stdout, stderr, code := runBinaryInDir(t, dir, "", "fix", "--no-color", "doc.md")
	out := stdout + stderr
	assert.Equal(t, 1, code, "lint violations must keep exit 1: %s", out)
	assert.Contains(t, out, "OK", "the build pass still runs")
	assert.FileExists(t, filepath.Join(dir, "dst.txt"))
}

func TestE2E_Check_RunsNoRecipe(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("cp is not available on Windows")
	}
	dir := writeBuildRepo(t, "    copy:\n      command: cp {inputs} {outputs}\n")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src.txt"), []byte("x"), 0o644))
	writeFixture(t, dir, "doc.md", buildDirective("copy", "src.txt", "dst.txt"))

	_, _, _ = runBinaryInDir(t, dir, "", "check", "--no-color", "doc.md")
	assert.NoFileExists(t, filepath.Join(dir, "dst.txt"), "check must never run a recipe")
}
