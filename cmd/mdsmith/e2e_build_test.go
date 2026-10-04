package main_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	buildexec "github.com/jeduden/mdsmith/internal/build"
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
// It also writes a matching .mdsmith.yml.trust marker so the build pass
// trust gate is satisfied, mirroring a developer who has run
// `mdsmith trust` on their clone.
func writeBuildRepo(t *testing.T, recipesYAML string) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".git"), 0o755))
	cfg := "rules: {}\nbuild:\n  recipes:\n" + recipesYAML
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".mdsmith.yml"), []byte(cfg), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".mdsmith.yml.trust"), []byte(cfg), 0o600))
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
	assert.Contains(t, stderr, "STALE", "--build-dry-run prints a STALE | FRESH verdict")
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
	assert.Contains(t, stderr, "TIMEOUT")
	// Only Windows is skipped, so name the kill this platform sends:
	// plan9 has no SIGTERM.
	assert.Contains(t, stderr, buildexec.TimeoutKillAction)
}

// --- Plan 103: staleness and dependency tracking ---

// fileMTime returns the modification time of root/rel.
func fileMTime(t *testing.T, root, rel string) (mtime int64) {
	t.Helper()
	info, err := os.Stat(filepath.Join(root, rel))
	require.NoError(t, err)
	return info.ModTime().UnixNano()
}

func TestE2E_Build_SecondRunSkipsFresh(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("cp is not available on Windows")
	}
	dir := writeBuildRepo(t, "    copy:\n      command: cp {inputs} {outputs}\n")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src.txt"), []byte("hello"), 0o644))
	writeFixture(t, dir, "doc.md", buildDirective("copy", "src.txt", "dst.txt"))

	_, stderr1, code1 := runBinaryInDir(t, dir, "", "fix", "--no-color", "--build-only", "doc.md")
	require.Equal(t, 0, code1, stderr1)
	assert.Contains(t, stderr1, "OK")

	// Capture the output's mtime, then run again: it must SKIP and not rewrite.
	before := fileMTime(t, dir, "dst.txt")
	_, stderr2, code2 := runBinaryInDir(t, dir, "", "fix", "--no-color", "--build-only", "doc.md")
	require.Equal(t, 0, code2, stderr2)
	assert.Contains(t, stderr2, "SKIP", "a fresh target must SKIP on the second run")
	assert.NotContains(t, stderr2, ": OK")
	assert.Equal(t, before, fileMTime(t, dir, "dst.txt"), "fresh target must not be rewritten")
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

func TestE2E_Build_RebuildsOnInputContentChange(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("cp is not available on Windows")
	}
	dir := writeBuildRepo(t, "    copy:\n      command: cp {inputs} {outputs}\n")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src.txt"), []byte("v1"), 0o644))
	writeFixture(t, dir, "doc.md", buildDirective("copy", "src.txt", "dst.txt"))

	_, _, code1 := runBinaryInDir(t, dir, "", "fix", "--no-color", "--build-only", "doc.md")
	require.Equal(t, 0, code1)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "src.txt"), []byte("v2"), 0o644))
	_, stderr2, code2 := runBinaryInDir(t, dir, "", "fix", "--no-color", "--build-only", "doc.md")
	require.Equal(t, 0, code2, stderr2)
	assert.Contains(t, stderr2, "OK", "an input content change must trigger a rebuild")

	got, err := os.ReadFile(filepath.Join(dir, "dst.txt"))
	require.NoError(t, err)
	assert.Equal(t, "v2", string(got))
}

func TestE2E_Build_MtimeOnlyChangeDoesNotRebuild(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("cp is not available on Windows")
	}
	dir := writeBuildRepo(t, "    copy:\n      command: cp {inputs} {outputs}\n")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src.txt"), []byte("same"), 0o644))
	writeFixture(t, dir, "doc.md", buildDirective("copy", "src.txt", "dst.txt"))

	_, _, code1 := runBinaryInDir(t, dir, "", "fix", "--no-color", "--build-only", "doc.md")
	require.Equal(t, 0, code1)

	// Rewrite identical content (bumps mtime, content unchanged).
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src.txt"), []byte("same"), 0o644))
	_, stderr2, code2 := runBinaryInDir(t, dir, "", "fix", "--no-color", "--build-only", "doc.md")
	require.Equal(t, 0, code2, stderr2)
	assert.Contains(t, stderr2, "SKIP", "an mtime-only change must not rebuild")
}

func TestE2E_Build_RecipeCommandChangeRebuilds(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("cp/install not available on Windows")
	}
	dir := writeBuildRepo(t, "    copy:\n      command: cp {inputs} {outputs}\n")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src.txt"), []byte("x"), 0o644))
	writeFixture(t, dir, "doc.md", buildDirective("copy", "src.txt", "dst.txt"))

	_, _, code1 := runBinaryInDir(t, dir, "", "fix", "--no-color", "--build-only", "doc.md")
	require.Equal(t, 0, code1)

	// Edit the recipe command; the ActionID changes, so the target rebuilds.
	// Editing the config invalidates the trust marker, so re-trust it (as a
	// developer would via `mdsmith trust`) before the second build.
	cfg := "rules: {}\nbuild:\n  recipes:\n    copy:\n      command: install {inputs} {outputs}\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".mdsmith.yml"), []byte(cfg), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".mdsmith.yml.trust"), []byte(cfg), 0o600))
	_, stderr2, code2 := runBinaryInDir(t, dir, "", "fix", "--no-color", "--build-only", "doc.md")
	require.Equal(t, 0, code2, stderr2)
	assert.Contains(t, stderr2, "OK", "a recipe command change must invalidate the target")
}

func TestE2E_Build_RebuildsWhenOneOutputDeleted(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("touch not available on Windows")
	}
	dir := writeBuildRepo(t, "    dup:\n      command: touch {outputs}\n")
	doc := "# Build\n\n" +
		"<?build\nrecipe: dup\noutputs:\n  - a.txt\n  - b.txt\n?>\n" +
		"[a.txt](a.txt)\n[b.txt](b.txt)\n" +
		"<?/build?>\n"
	writeFixture(t, dir, "doc.md", doc)

	_, _, code1 := runBinaryInDir(t, dir, "", "fix", "--no-color", "--build-only", "doc.md")
	require.Equal(t, 0, code1)

	// Second run: both fresh → SKIP.
	_, stderr2, _ := runBinaryInDir(t, dir, "", "fix", "--no-color", "--build-only", "doc.md")
	assert.Contains(t, stderr2, "SKIP")

	// Delete one output → rebuild.
	require.NoError(t, os.Remove(filepath.Join(dir, "b.txt")))
	_, stderr3, code3 := runBinaryInDir(t, dir, "", "fix", "--no-color", "--build-only", "doc.md")
	require.Equal(t, 0, code3, stderr3)
	assert.Contains(t, stderr3, "OK", "a deleted output must trigger a rebuild")
	assert.FileExists(t, filepath.Join(dir, "b.txt"))
}

func TestE2E_Build_GlobMatchingZeroFilesIsError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("cp not available on Windows")
	}
	dir := writeBuildRepo(t, "    copy:\n      command: cp {inputs} {outputs}\n")
	doc := "# Build\n\n" +
		"<?build\nrecipe: copy\ninputs:\n  - \"*.none\"\noutputs:\n  - dst.txt\n?>\n" +
		"[dst.txt](dst.txt)\n<?/build?>\n"
	writeFixture(t, dir, "doc.md", doc)

	_, stderr, code := runBinaryInDir(t, dir, "", "fix", "--no-color", "--build-only", "doc.md")
	assert.Equal(t, 2, code, "a glob matching zero files is a build error")
	assert.Contains(t, stderr, "FAIL")
	assert.NoFileExists(t, filepath.Join(dir, "dst.txt"))
}

func TestE2E_Build_OverlappingOutputsIsError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("touch not available on Windows")
	}
	dir := writeBuildRepo(t, "    mk:\n      command: touch {outputs}\n")
	// Two directives whose outputs overlap by directory prefix.
	doc := "# Build\n\n" +
		"<?build\nrecipe: mk\noutputs:\n  - book/\n?>\n[book/](book/)\n<?/build?>\n\n" +
		"<?build\nrecipe: mk\noutputs:\n  - book/index.html\n?>\n" +
		"[book/index.html](book/index.html)\n<?/build?>\n"
	writeFixture(t, dir, "doc.md", doc)

	_, stderr, code := runBinaryInDir(t, dir, "", "fix", "--no-color", "--build-only", "doc.md")
	assert.Equal(t, 2, code, "overlapping outputs is a build error")
	assert.Contains(t, stderr, "overlap")
}

func TestE2E_Build_ForceRebuildsFresh(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("cp not available on Windows")
	}
	dir := writeBuildRepo(t, "    copy:\n      command: cp {inputs} {outputs}\n")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src.txt"), []byte("x"), 0o644))
	writeFixture(t, dir, "doc.md", buildDirective("copy", "src.txt", "dst.txt"))

	_, _, code1 := runBinaryInDir(t, dir, "", "fix", "--no-color", "--build-only", "doc.md")
	require.Equal(t, 0, code1)

	// Fresh now, but --build-force rebuilds anyway.
	_, stderr2, code2 := runBinaryInDir(t, dir, "", "fix", "--no-color", "--build-only", "--build-force", "doc.md")
	require.Equal(t, 0, code2, stderr2)
	assert.Contains(t, stderr2, "OK", "--build-force must rebuild even a fresh target")
	assert.NotContains(t, stderr2, "SKIP")
}

func TestE2E_Build_CheckStaleNonZeroThenZero(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("cp not available on Windows")
	}
	dir := writeBuildRepo(t, "    copy:\n      command: cp {inputs} {outputs}\n")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src.txt"), []byte("x"), 0o644))
	writeFixture(t, dir, "doc.md", buildDirective("copy", "src.txt", "dst.txt"))

	checkArgs := []string{"fix", "--no-color", "--build-only", "--build-check-stale", "doc.md"}

	// Stale before any build → exit non-zero, no recipe runs.
	_, stderr1, code1 := runBinaryInDir(t, dir, "", checkArgs...)
	assert.Equal(t, 2, code1)
	assert.Contains(t, stderr1, "STALE")
	assert.NoFileExists(t, filepath.Join(dir, "dst.txt"), "--build-check-stale must not run a recipe")

	// Build it, then check-stale again → exit zero.
	_, _, code2 := runBinaryInDir(t, dir, "", "fix", "--no-color", "--build-only", "doc.md")
	require.Equal(t, 0, code2)
	_, _, code3 := runBinaryInDir(t, dir, "", checkArgs...)
	assert.Equal(t, 0, code3, "--build-check-stale exits zero when all fresh")
}

func TestE2E_Build_NoCacheRebuildsAndWritesNothing(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("cp not available on Windows")
	}
	dir := writeBuildRepo(t, "    copy:\n      command: cp {inputs} {outputs}\n")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src.txt"), []byte("x"), 0o644))
	writeFixture(t, dir, "doc.md", buildDirective("copy", "src.txt", "dst.txt"))

	_, stderr, code := runBinaryInDir(t, dir, "", "fix", "--no-color", "--build-only", "--build-no-cache", "doc.md")
	require.Equal(t, 0, code, stderr)
	assert.Contains(t, stderr, "OK")
	assert.NoFileExists(t, filepath.Join(dir, ".mdsmith", "build-cache.json"),
		"--build-no-cache must not write a cache file")
}

func TestE2E_Build_ForceWithCheckStaleIsUsageError(t *testing.T) {
	dir := writeBuildRepo(t, "    copy:\n      command: cp {inputs} {outputs}\n")
	writeFixture(t, dir, "doc.md", buildDirective("copy", "", "dst.txt"))

	_, stderr, code := runBinaryInDir(t, dir, "", "fix", "--build-force", "--build-check-stale", "doc.md")
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "--build-force")
}

func TestE2E_Build_TamperedOutputRebuilds(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("cp not available on Windows")
	}
	dir := writeBuildRepo(t, "    copy:\n      command: cp {inputs} {outputs}\n")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src.txt"), []byte("real"), 0o644))
	writeFixture(t, dir, "doc.md", buildDirective("copy", "src.txt", "dst.txt"))

	_, _, code1 := runBinaryInDir(t, dir, "", "fix", "--no-color", "--build-only", "doc.md")
	require.Equal(t, 0, code1)

	// Hand-edit the artifact: ActionID unchanged, output hash differs → rebuild.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "dst.txt"), []byte("tampered"), 0o644))
	_, stderr2, code2 := runBinaryInDir(t, dir, "", "fix", "--no-color", "--build-only", "doc.md")
	require.Equal(t, 0, code2, stderr2)
	assert.Contains(t, stderr2, "OK", "a hand-edited artifact must trigger a rebuild")
	got, err := os.ReadFile(filepath.Join(dir, "dst.txt"))
	require.NoError(t, err)
	assert.Equal(t, "real", string(got), "rebuild restores the artifact")
}

func TestE2E_Build_CacheFileSchema(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("cp not available on Windows")
	}
	dir := writeBuildRepo(t, "    copy:\n      command: cp {inputs} {outputs}\n")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src.txt"), []byte("x"), 0o644))
	writeFixture(t, dir, "doc.md", buildDirective("copy", "src.txt", "dst.txt"))

	_, _, code := runBinaryInDir(t, dir, "", "fix", "--no-color", "--build-only", "doc.md")
	require.Equal(t, 0, code)

	data, err := os.ReadFile(filepath.Join(dir, ".mdsmith", "build-cache.json"))
	require.NoError(t, err)
	s := string(data)
	assert.Contains(t, s, "\"version\": 1")
	assert.Contains(t, s, "\"action-id\": \"sha256-")
	assert.Contains(t, s, "\"built-at\"")
	assert.Contains(t, s, "\"outputs\"")
	assert.Contains(t, s, "\"dst.txt\"")
	assert.Contains(t, s, "\"inputs\"")
	assert.Contains(t, s, "\"recipe\": \"copy\"")
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

// --- Plan 104: build lifecycle hooks ---

// writeBuildRepoWithHooks sets up a repo with recipes and hooks. It also
// writes a matching .mdsmith.yml.trust marker so the build pass trust gate
// is satisfied, mirroring a developer who has run `mdsmith trust`.
func writeBuildRepoWithHooks(t *testing.T, recipesYAML, hooksYAML string) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".git"), 0o755))
	cfg := "rules: {}\nbuild:\n  recipes:\n" + recipesYAML + "\n  hooks:\n" + hooksYAML
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".mdsmith.yml"), []byte(cfg), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".mdsmith.yml.trust"), []byte(cfg), 0o600))
	return dir
}

func TestE2E_Build_BeforeHookRunsBeforeRecipe(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("touch/cp not available on Windows")
	}
	dir := writeBuildRepoWithHooks(t,
		"    copy:\n      command: cp {inputs} {outputs}\n",
		"    before:\n      - command: touch before-sentinel.txt\n",
	)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src.txt"), []byte("hi"), 0o644))
	writeFixture(t, dir, "doc.md", buildDirective("copy", "src.txt", "dst.txt"))

	stdout, stderr, code := runBinaryInDir(t, dir, "", "fix", "--no-color", "--build-only", "doc.md")
	out := stdout + stderr
	assert.Equal(t, 0, code, "fix with before hook should succeed: %s", out)
	assert.FileExists(t, filepath.Join(dir, "before-sentinel.txt"), "before hook must have run")
	assert.FileExists(t, filepath.Join(dir, "dst.txt"), "recipe must have run")
	assert.Contains(t, out, "hook touch: OK")
}

func TestE2E_Build_AfterHookRunsAfterRecipe(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("touch/cp not available on Windows")
	}
	dir := writeBuildRepoWithHooks(t,
		"    copy:\n      command: cp {inputs} {outputs}\n",
		"    after:\n      - command: touch after-sentinel.txt\n",
	)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src.txt"), []byte("hi"), 0o644))
	writeFixture(t, dir, "doc.md", buildDirective("copy", "src.txt", "dst.txt"))

	stdout, stderr, code := runBinaryInDir(t, dir, "", "fix", "--no-color", "--build-only", "doc.md")
	out := stdout + stderr
	assert.Equal(t, 0, code, "fix with after hook should succeed: %s", out)
	assert.FileExists(t, filepath.Join(dir, "dst.txt"), "recipe must have run")
	assert.FileExists(t, filepath.Join(dir, "after-sentinel.txt"), "after hook must have run")
}

func TestE2E_Build_FailingBeforeHookAbortsRecipesAndAfterHooks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("false/touch not available on Windows")
	}
	dir := writeBuildRepoWithHooks(t,
		"    copy:\n      command: touch {outputs}\n",
		"    before:\n      - command: false\n        name: fail-before\n"+
			"    after:\n      - command: touch after-sentinel.txt\n",
	)
	writeFixture(t, dir, "doc.md", buildDirective("copy", "", "dst.txt"))

	stdout, stderr, code := runBinaryInDir(t, dir, "", "fix", "--no-color", "--build-only", "doc.md")
	out := stdout + stderr
	assert.NotEqual(t, 0, code, "failing before-hook must exit non-zero: %s", out)
	assert.Contains(t, out, "fail-before: FAIL")
	assert.NoFileExists(t, filepath.Join(dir, "dst.txt"), "recipe must not run after before-fail")
	assert.NoFileExists(t, filepath.Join(dir, "after-sentinel.txt"), "after-hook must not run after before-fail")
}

func TestE2E_Build_FailingAfterHookReportedButContinues(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("touch/false not available on Windows")
	}
	dir := writeBuildRepoWithHooks(t,
		"    copy:\n      command: touch {outputs}\n",
		"    after:\n"+
			"      - command: false\n        name: fail-after\n"+
			"      - command: touch after-second.txt\n",
	)
	writeFixture(t, dir, "doc.md", buildDirective("copy", "", "dst.txt"))

	stdout, stderr, code := runBinaryInDir(t, dir, "", "fix", "--no-color", "--build-only", "doc.md")
	out := stdout + stderr
	assert.NotEqual(t, 0, code, "failing after-hook must exit non-zero: %s", out)
	assert.Contains(t, out, "fail-after: FAIL")
	assert.FileExists(t, filepath.Join(dir, "dst.txt"), "recipe must still have run")
	assert.FileExists(t, filepath.Join(dir, "after-second.txt"), "second after-hook must still run")
}

func TestE2E_Build_NoHooksSkipsBothLists(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("touch not available on Windows")
	}
	dir := writeBuildRepoWithHooks(t,
		"    copy:\n      command: touch {outputs}\n",
		"    before:\n      - command: touch before-sentinel.txt\n"+
			"    after:\n      - command: touch after-sentinel.txt\n",
	)
	writeFixture(t, dir, "doc.md", buildDirective("copy", "", "dst.txt"))

	stdout, stderr, code := runBinaryInDir(t, dir, "", "fix",
		"--no-color", "--build-only", "--build-no-hooks", "doc.md")
	out := stdout + stderr
	assert.Equal(t, 0, code, "fix --build-no-hooks should succeed: %s", out)
	assert.FileExists(t, filepath.Join(dir, "dst.txt"), "recipe must still run")
	assert.NoFileExists(t, filepath.Join(dir, "before-sentinel.txt"), "--build-no-hooks must skip before hooks")
	assert.NoFileExists(t, filepath.Join(dir, "after-sentinel.txt"), "--build-no-hooks must skip after hooks")
}

func TestE2E_Build_DryRunListsHooks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("cp not available on Windows")
	}
	dir := writeBuildRepoWithHooks(t,
		"    copy:\n      command: cp {inputs} {outputs}\n",
		"    before:\n      - command: make dev-server-start\n        name: start server\n"+
			"    after:\n      - command: make dev-server-stop\n        name: stop server\n",
	)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src.txt"), []byte("x"), 0o644))
	writeFixture(t, dir, "doc.md", buildDirective("copy", "src.txt", "dst.txt"))

	_, stderr, code := runBinaryInDir(t, dir, "", "fix", "--no-color", "--build-dry-run", "--build-only", "doc.md")
	assert.Equal(t, 0, code, "dry-run should succeed: %s", stderr)
	assert.Contains(t, stderr, "DRY-RUN", "--build-dry-run must list hooks")
	assert.NoFileExists(t, filepath.Join(dir, "dst.txt"), "--build-dry-run must not run recipes")
}

func TestE2E_Build_NoBuildSkipsHooks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("touch not available on Windows")
	}
	dir := writeBuildRepoWithHooks(t,
		"    copy:\n      command: touch {outputs}\n",
		"    before:\n      - command: touch before-sentinel.txt\n"+
			"    after:\n      - command: touch after-sentinel.txt\n",
	)
	writeFixture(t, dir, "doc.md", buildDirective("copy", "", "dst.txt"))

	// --no-build skips the entire build pass including hooks.
	_, _, code := runBinaryInDir(t, dir, "", "fix", "--no-color", "--no-build", "doc.md")
	assert.Equal(t, 0, code)
	assert.NoFileExists(t, filepath.Join(dir, "before-sentinel.txt"), "--no-build must skip hooks")
	assert.NoFileExists(t, filepath.Join(dir, "after-sentinel.txt"), "--no-build must skip hooks")
}

func TestE2E_Build_SkipHooksWhenFresh_AllFresh_SkipsHooks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("cp/touch not available on Windows")
	}
	dir := writeBuildRepoWithHooks(t,
		"    copy:\n      command: cp {inputs} {outputs}\n",
		"    before:\n      - command: touch before-sentinel.txt\n",
	)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src.txt"), []byte("x"), 0o644))
	writeFixture(t, dir, "doc.md", buildDirective("copy", "src.txt", "dst.txt"))

	// Build once to make everything fresh. Use --build-no-hooks so the
	// sentinel is NOT created on the first run (we want to detect if it gets
	// created on the second run).
	_, _, code1 := runBinaryInDir(t, dir, "", "fix", "--no-color", "--build-only", "--build-no-hooks", "doc.md")
	require.Equal(t, 0, code1)
	// Sentinel must not exist after the first run (hooks were skipped).
	assert.NoFileExists(t, filepath.Join(dir, "before-sentinel.txt"))

	// Run with --build-skip-hooks-when-fresh: all targets are fresh, hooks must be skipped.
	_, _, code2 := runBinaryInDir(t, dir, "", "fix", "--no-color", "--build-only",
		"--build-skip-hooks-when-fresh", "doc.md")
	assert.Equal(t, 0, code2)
	assert.NoFileExists(t, filepath.Join(dir, "before-sentinel.txt"),
		"--build-skip-hooks-when-fresh with all-fresh must skip before hooks")
}

func TestE2E_Build_SkipHooksWhenFresh_AnyStale_RunsHooks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("cp/touch not available on Windows")
	}
	dir := writeBuildRepoWithHooks(t,
		"    copy:\n      command: cp {inputs} {outputs}\n",
		"    before:\n      - command: touch before-sentinel.txt\n",
	)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src.txt"), []byte("v1"), 0o644))
	writeFixture(t, dir, "doc.md", buildDirective("copy", "src.txt", "dst.txt"))

	// Build once then update input to make it stale again.
	_, _, code1 := runBinaryInDir(t, dir, "", "fix", "--no-color", "--build-only", "--build-no-hooks", "doc.md")
	require.Equal(t, 0, code1)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src.txt"), []byte("v2"), 0o644))
	// Remove the sentinel so we can detect if it gets created.
	_ = os.Remove(filepath.Join(dir, "before-sentinel.txt"))

	// Run with --build-skip-hooks-when-fresh: one target is stale, hooks must run.
	_, _, code2 := runBinaryInDir(t, dir, "", "fix", "--no-color", "--build-only",
		"--build-skip-hooks-when-fresh", "doc.md")
	assert.Equal(t, 0, code2)
	assert.FileExists(t, filepath.Join(dir, "before-sentinel.txt"),
		"--build-skip-hooks-when-fresh with a stale target must run hooks")
}

func TestE2E_Build_HookWithParams(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("touch not available on Windows")
	}
	// The hook command references {name} param, which should be substituted.
	dir := writeBuildRepoWithHooks(t,
		"    copy:\n      command: touch {outputs}\n",
		"    before:\n      - command: touch {name}\n        params:\n          name: param-sentinel.txt\n",
	)
	writeFixture(t, dir, "doc.md", buildDirective("copy", "", "dst.txt"))

	stdout, stderr, code := runBinaryInDir(t, dir, "", "fix", "--no-color", "--build-only", "doc.md")
	out := stdout + stderr
	assert.Equal(t, 0, code, "hook with params should succeed: %s", out)
	assert.FileExists(t, filepath.Join(dir, "param-sentinel.txt"), "hook param must be substituted")
}

func TestE2E_Build_MDS040GateBlocksShellInterpreterHook(t *testing.T) {
	// The build pass must refuse to run when MDS040 emits an error
	// against build.hooks. A hook using a shell interpreter is an
	// MDS040 error; the gate must print the error and exit 2 without
	// running any hook or recipe.
	dir := writeBuildRepoWithHooks(t,
		"    copy:\n      command: touch {outputs}\n",
		"    before:\n      - command: bash -c echo hello\n",
	)
	writeFixture(t, dir, "doc.md", buildDirective("copy", "", "dst.txt"))

	stdout, stderr, code := runBinaryInDir(t, dir, "", "fix", "--no-color", "--build-only", "doc.md")
	out := stdout + stderr
	assert.Equal(t, 2, code, "MDS040 gate must block: %s", out)
	assert.Contains(t, out, "MDS040", "must report the MDS040 error")
	assert.NoFileExists(t, filepath.Join(dir, "dst.txt"), "recipe must not run when MDS040 blocks")
}

func TestE2E_Build_MDS040GateBlocksShellInterpreterRecipe(t *testing.T) {
	// MDS040 gate blocks a recipe using a shell interpreter.
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".git"), 0o755))
	cfg := "rules: {}\nbuild:\n  recipes:\n    bad:\n      command: bash -c echo\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".mdsmith.yml"), []byte(cfg), 0o644))
	writeFixture(t, dir, "doc.md", buildDirective("bad", "", "dst.txt"))

	stdout, stderr, code := runBinaryInDir(t, dir, "", "fix", "--no-color", "--build-only", "doc.md")
	out := stdout + stderr
	assert.Equal(t, 2, code, "MDS040 gate must block bad recipe: %s", out)
	assert.Contains(t, out, "MDS040", "must report the MDS040 error")
	assert.NoFileExists(t, filepath.Join(dir, "dst.txt"), "recipe must not run when MDS040 blocks")
}

func TestE2E_Build_MDS040GateAllowsNoBuildFlag(t *testing.T) {
	// --no-build skips the build pass entirely, so the MDS040 gate
	// also does not run. The bad hook should not block the lint-fix pass.
	dir := writeBuildRepoWithHooks(t,
		"    copy:\n      command: touch {outputs}\n",
		"    before:\n      - command: bash -c echo hello\n",
	)
	writeFixture(t, dir, "doc.md", buildDirective("copy", "", "dst.txt"))

	_, _, code := runBinaryInDir(t, dir, "", "fix", "--no-color", "--no-build", "doc.md")
	// --no-build skips the build pass; the lint pass may exit 0 or 1
	// (depending on lint issues in doc.md), but never 2 from the gate.
	assert.NotEqual(t, 2, code, "--no-build must not trigger the MDS040 gate")
}

// A recipe declared under [tool.mdsmith.build.recipes] in a
// pyproject-only project runs on fix; the trust gate pins the
// pyproject.toml it loaded.
func TestE2E_Build_PyprojectRecipe(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("cp is not available on Windows")
	}
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".git"), 0o755))
	cfg := "[project]\nname = \"x\"\n\n[tool.mdsmith.build.recipes.copy]\ncommand = \"cp {inputs} {outputs}\"\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte(cfg), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "pyproject.toml.trust"), []byte(cfg), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src.txt"), []byte("hello"), 0o644))
	writeFixture(t, dir, "doc.md", buildDirective("copy", "src.txt", "dst.txt"))

	stdout, stderr, code := runBinaryInDir(t, dir, "", "fix", "--no-color", "doc.md")
	assert.Equal(t, 0, code, "fix should succeed: %s", stdout+stderr)
	got, err := os.ReadFile(filepath.Join(dir, "dst.txt"))
	require.NoError(t, err)
	assert.Equal(t, "hello", string(got))
}
