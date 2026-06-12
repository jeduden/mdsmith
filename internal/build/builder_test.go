package build

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recipeCmd builds a RecipeSpec whose command is the given string.
func recipeCmd(command string) RecipeSpec {
	return RecipeSpec{Command: command}
}

func TestNewCustomBuilderExec(t *testing.T) {
	recipes := map[string]RecipeSpec{"r": recipeCmd("echo")}
	ec := ExecConfig{Path: "/custom/bin"}
	b := NewCustomBuilderExec(recipes, ec)
	require.NotNil(t, b)
	assert.Equal(t, ec.Path, b.exec.Path)
	assert.Len(t, b.recipes, 1)
}

func TestBuild_SingleOutputCp(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("cp is not available on Windows")
	}
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "src.txt"), []byte("hello"), 0o644))

	b := NewCustomBuilder(map[string]RecipeSpec{
		"copy": recipeCmd("cp {inputs} {outputs}"),
	})
	err := b.Build(context.Background(), Target{
		Recipe:  "copy",
		Root:    root,
		Inputs:  []string{"src.txt"},
		Outputs: []string{"dst.txt"},
	})
	require.NoError(t, err)

	got, err := os.ReadFile(filepath.Join(root, "dst.txt"))
	require.NoError(t, err)
	assert.Equal(t, "hello", string(got))
}

// writeScript writes an executable shell script into dir and returns its
// absolute path. The script body must use only space-free argv tokens
// when referenced from a recipe command, since recipe commands are
// whitespace-tokenized (no shell quoting).
func writeScript(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(p, []byte("#!/bin/sh\n"+body+"\n"), 0o755))
	return p
}

func TestBuild_MultiOutputTee(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("tee/sh is not available on Windows")
	}
	root := t.TempDir()
	bindir := t.TempDir()
	// Script writes "payload" to every argument (each staged output).
	script := writeScript(t, bindir, "dup.sh", `for f in "$@"; do printf payload > "$f"; done`)

	b := NewCustomBuilder(map[string]RecipeSpec{
		"dup": recipeCmd(script + " {outputs}"),
	})
	err := b.Build(context.Background(), Target{
		Recipe:  "dup",
		Root:    root,
		Outputs: []string{"a.txt", "b.txt"},
	})
	require.NoError(t, err)

	a, err := os.ReadFile(filepath.Join(root, "a.txt"))
	require.NoError(t, err)
	bb, err := os.ReadFile(filepath.Join(root, "b.txt"))
	require.NoError(t, err)
	assert.Equal(t, "payload", string(a))
	assert.Equal(t, "payload", string(bb))
}

func TestBuild_FailingRecipeLeavesNoPartialOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sh is not available on Windows")
	}
	root := t.TempDir()

	// Recipe writes the first output then exits non-zero. No final output
	// should be touched.
	bindir := t.TempDir()
	script := writeScript(t, bindir, "halffail.sh", `printf x > "$1"; exit 3`)
	b := NewCustomBuilder(map[string]RecipeSpec{
		"halffail": recipeCmd(script + " {outputs}"),
	})
	err := b.Build(context.Background(), Target{
		Recipe:  "halffail",
		Root:    root,
		Outputs: []string{"a.txt", "b.txt"},
	})
	require.Error(t, err)
	assert.NoFileExists(t, filepath.Join(root, "a.txt"))
	assert.NoFileExists(t, filepath.Join(root, "b.txt"))
}

func TestBuild_FailingRecipePreservesExistingOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sh is not available on Windows")
	}
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "out.txt"), []byte("original"), 0o644))

	bindir := t.TempDir()
	script := writeScript(t, bindir, "fail.sh", `exit 1`)
	b := NewCustomBuilder(map[string]RecipeSpec{
		"fail": recipeCmd(script + " {outputs}"),
	})
	err := b.Build(context.Background(), Target{
		Recipe:  "fail",
		Root:    root,
		Outputs: []string{"out.txt"},
	})
	require.Error(t, err)
	got, err := os.ReadFile(filepath.Join(root, "out.txt"))
	require.NoError(t, err)
	assert.Equal(t, "original", string(got))
}

func TestBuild_ParamSubstitutionNoShell(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sh is not available on Windows")
	}
	root := t.TempDir()

	// A param value containing shell metacharacters must be passed as a
	// single literal argv entry, never interpreted by a shell. We write
	// the param value to the output verbatim. $1 is the staged output,
	// $2 is the param value (one argv entry even though it has spaces).
	bindir := t.TempDir()
	script := writeScript(t, bindir, "echo.sh", `printf %s "$2" > "$1"`)
	b := NewCustomBuilder(map[string]RecipeSpec{
		"echo": recipeCmd(script + " {outputs} {value}"),
	})
	danger := "foo; rm -rf /"
	err := b.Build(context.Background(), Target{
		Recipe:  "echo",
		Root:    root,
		Params:  map[string]string{"value": danger},
		Outputs: []string{"out.txt"},
	})
	require.NoError(t, err)
	got, err := os.ReadFile(filepath.Join(root, "out.txt"))
	require.NoError(t, err)
	assert.Equal(t, danger, string(got))
}

func TestBuild_UnknownRecipeErrors(t *testing.T) {
	root := t.TempDir()
	b := NewCustomBuilder(map[string]RecipeSpec{})
	err := b.Build(context.Background(), Target{Recipe: "missing", Root: root, Outputs: []string{"x"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing")
}

func TestBuild_InputGlobResolves(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("cat is not available on Windows")
	}
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "src"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "src/a.txt"), []byte("A"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "src/b.txt"), []byte("B"), 0o644))

	bindir := t.TempDir()
	// $1 is the staged output; $2.. are the resolved inputs.
	script := writeScript(t, bindir, "cat.sh", `out="$1"; shift; cat "$@" > "$out"`)
	b := NewCustomBuilder(map[string]RecipeSpec{
		"cat": recipeCmd(script + " {outputs} {inputs}"),
	})
	err := b.Build(context.Background(), Target{
		Recipe:  "cat",
		Root:    root,
		Inputs:  []string{"src/*.txt"},
		Outputs: []string{"all.txt"},
	})
	require.NoError(t, err)
	got, err := os.ReadFile(filepath.Join(root, "all.txt"))
	require.NoError(t, err)
	// Globs resolve in sorted order: a then b.
	assert.Equal(t, "AB", string(got))
}

func TestBuild_InputEscapingRootErrors(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation is unreliable on Windows CI")
	}
	root := t.TempDir()
	outside := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("s"), 0o644))
	require.NoError(t, os.Symlink(filepath.Join(outside, "secret.txt"), filepath.Join(root, "leak.txt")))

	b := NewCustomBuilder(map[string]RecipeSpec{
		"copy": recipeCmd("cp {inputs} {outputs}"),
	})
	err := b.Build(context.Background(), Target{
		Recipe:  "copy",
		Root:    root,
		Inputs:  []string{"leak.txt"},
		Outputs: []string{"out.txt"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "project root")
}

func TestBuild_Timeout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sleep is not available on Windows")
	}
	root := t.TempDir()
	bindir := t.TempDir()
	script := writeScript(t, bindir, "slow.sh", `sleep 5`)
	b := NewCustomBuilder(map[string]RecipeSpec{
		"slow": recipeCmd(script + " {outputs}"),
	})
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	start := time.Now()
	err := b.Build(ctx, Target{Recipe: "slow", Root: root, Outputs: []string{"out.txt"}})
	require.Error(t, err)
	assert.Less(t, time.Since(start), 4*time.Second)
	assert.NoFileExists(t, filepath.Join(root, "out.txt"))
}

func TestBuild_EmptyCommandErrors(t *testing.T) {
	root := t.TempDir()
	b := NewCustomBuilder(map[string]RecipeSpec{
		"empty": recipeCmd("   "),
	})
	err := b.Build(context.Background(), Target{Recipe: "empty", Root: root, Outputs: []string{"x"}})
	require.Error(t, err)
}

func TestBuild_GlobCapExceeded(t *testing.T) {
	old := builderGlobCapFn
	builderGlobCapFn = func(_ int) error { return errors.New("cap exceeded") }
	defer func() { builderGlobCapFn = old }()

	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "a.txt"), []byte("a"), 0o644))
	b := NewCustomBuilder(map[string]RecipeSpec{"r": recipeCmd("echo hi")})
	err := b.Build(context.Background(), Target{
		Recipe:  "r",
		Root:    root,
		Inputs:  []string{"*.txt"},
		Outputs: []string{"out.txt"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cap exceeded")
}

func TestArgvExpansion_ListsExpandPerEntry(t *testing.T) {
	// {outputs} and {inputs} each expand to one argv per resolved entry.
	argv := expandArgv(
		strings.Fields("tool {inputs} -o {outputs}"),
		map[string]string{},
		[]string{"in1", "in2"},
		[]string{"out1"},
	)
	assert.Equal(t, []string{"tool", "in1", "in2", "-o", "out1"}, argv)
}

func TestArgvExpansion_ParamWhitespaceStaysOneEntry(t *testing.T) {
	argv := expandArgv(
		strings.Fields("tool {value}"),
		map[string]string{"value": "a b c"},
		nil, nil,
	)
	assert.Equal(t, []string{"tool", "a b c"}, argv)
}

func TestSubstituteParams_UnclosedBrace(t *testing.T) {
	// An unclosed { is written through literally rather than panicking.
	assert.Equal(t, "{unclosed", substituteParams("{unclosed", nil))
}

func TestSubstituteParams_AbsentOptionalParam(t *testing.T) {
	// A {name} placeholder with no matching param expands to the empty string.
	assert.Equal(t, "", substituteParams("{missing}", map[string]string{}))
}

func TestBuild_RecipeDoesNotProduceOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sh is not available on Windows")
	}
	root := t.TempDir()
	bindir := t.TempDir()
	// Recipe exits 0 but never writes the staged output file.
	script := writeScript(t, bindir, "noop.sh", `exit 0`)
	b := NewCustomBuilder(map[string]RecipeSpec{
		"noop": recipeCmd(script),
	})
	err := b.Build(context.Background(), Target{
		Recipe:  "noop",
		Root:    root,
		Outputs: []string{"out.txt"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "did not produce declared output")
}

func TestBuild_OutputEscapingRootErrors(t *testing.T) {
	root := t.TempDir()
	b := NewCustomBuilder(map[string]RecipeSpec{
		"echo": recipeCmd("echo hi"),
	})
	err := b.Build(context.Background(), Target{
		Recipe:  "echo",
		Root:    root,
		Outputs: []string{"../escape.txt"},
	})
	require.Error(t, err)
}

func TestBuild_InputGlobMalformed(t *testing.T) {
	root := t.TempDir()
	b := NewCustomBuilder(map[string]RecipeSpec{
		"echo": recipeCmd("echo hi"),
	})
	// "[" is an unclosed character class — doublestar returns a syntax error.
	err := b.Build(context.Background(), Target{
		Recipe:  "echo",
		Root:    root,
		Inputs:  []string{"["},
		Outputs: []string{"out.txt"},
	})
	require.Error(t, err)
}

func TestBuild_InputGlobMatchEscapesRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation is unreliable on Windows CI")
	}
	root := t.TempDir()
	outside := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("s"), 0o644))
	require.NoError(t, os.Symlink(filepath.Join(outside, "secret.txt"), filepath.Join(root, "leak.txt")))

	b := NewCustomBuilder(map[string]RecipeSpec{
		"copy": recipeCmd("cp {inputs} {outputs}"),
	})
	// Glob "*.txt" matches leak.txt; ResolvePathInRoot follows the symlink
	// and detects it escapes the project root.
	err := b.Build(context.Background(), Target{
		Recipe:  "copy",
		Root:    root,
		Inputs:  []string{"*.txt"},
		Outputs: []string{"out.txt"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "project root")
}

func TestCommitOutputs_MkdirAllError(t *testing.T) {
	root := t.TempDir()

	// Create a regular file where a directory is expected. MkdirAll will
	// fail because "file.txt" exists as a file, not a directory.
	require.NoError(t, os.WriteFile(filepath.Join(root, "file.txt"), []byte("x"), 0o644))

	stageDir := t.TempDir()
	stage := filepath.Join(stageDir, "out0")
	require.NoError(t, os.WriteFile(stage, []byte("data"), 0o644))

	finals := []string{filepath.Join(root, "file.txt", "result.txt")}
	err := commitOutputs(finals, []string{"file.txt/result.txt"}, []string{stage})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "creating output dir")
}

func TestCommitOutputs_RenameFallbackToCopyFails(t *testing.T) {
	root := t.TempDir()

	// Pre-create the final path as a directory. os.Rename(file→dir) fails
	// with EISDIR and copyFile also fails (can't open a directory for
	// writing), exercising the error return at the end of the fallback.
	// This requires no special permissions and works on all supported OSes.
	require.NoError(t, os.MkdirAll(filepath.Join(root, "result"), 0o755))

	stageDir := t.TempDir()
	stage := filepath.Join(stageDir, "out0")
	require.NoError(t, os.WriteFile(stage, []byte("data"), 0o644))

	finals := []string{filepath.Join(root, "result")}
	err := commitOutputs(finals, []string{"result"}, []string{stage})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "writing output")
}

func TestCopyFile_Success(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "src.txt")
	dst := filepath.Join(root, "dst.txt")
	require.NoError(t, os.WriteFile(src, []byte("hello"), 0o644))
	require.NoError(t, copyFile(src, dst))
	got, err := os.ReadFile(dst)
	require.NoError(t, err)
	assert.Equal(t, "hello", string(got))
}

func TestCopyFile_ReadError(t *testing.T) {
	root := t.TempDir()
	err := copyFile(filepath.Join(root, "nonexistent.txt"), filepath.Join(root, "dst.txt"))
	require.Error(t, err)
}

func TestCopyFile_StreamReadError(t *testing.T) {
	// A directory opens fine but reading from it fails (EISDIR), so the
	// error surfaces from io.Copy rather than os.Open — exercising the
	// mid-stream error branch.
	root := t.TempDir()
	srcDir := filepath.Join(root, "srcdir")
	require.NoError(t, os.MkdirAll(srcDir, 0o755))
	err := copyFile(srcDir, filepath.Join(root, "dst.txt"))
	require.Error(t, err)
}

func TestCopyFile_PreservesSourceMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix permission bits are not meaningful on Windows")
	}
	root := t.TempDir()
	src := filepath.Join(root, "src.bin")
	dst := filepath.Join(root, "dst.bin")
	require.NoError(t, os.WriteFile(src, []byte("x"), 0o600))
	require.NoError(t, copyFile(src, dst))
	info, err := os.Stat(dst)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestBuild_StagingRootCreationFails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("ENOTDIR semantics differ on Windows")
	}
	root := t.TempDir()
	// Plant a regular file at .mdsmith so MkdirAll(.mdsmith/build-staging)
	// fails with ENOTDIR before any recipe runs.
	require.NoError(t, os.WriteFile(filepath.Join(root, ".mdsmith"), []byte("x"), 0o644))
	b := NewCustomBuilder(map[string]RecipeSpec{
		"echo": recipeCmd("echo hi"),
	})
	err := b.Build(context.Background(), Target{
		Recipe:  "echo",
		Root:    root,
		Outputs: []string{"out.txt"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "staging root")
}

func TestVerifyOutputsExist_StatNonNotExistError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("ENOTDIR semantics differ on Windows")
	}
	// A stage path that descends through a regular file makes os.Lstat
	// fail with ENOTDIR — a non-ErrNotExist error.
	blocker := filepath.Join(t.TempDir(), "blocker")
	require.NoError(t, os.WriteFile(blocker, []byte("x"), 0o644))
	stage := filepath.Join(blocker, "out0")

	err := verifyOutputsExist([]string{"out.txt"}, []string{stage})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "staging output")
}

func TestVerifyOutputsExist_MissingOutput(t *testing.T) {
	stageDir := t.TempDir()
	stage := filepath.Join(stageDir, "out0") // never created
	err := verifyOutputsExist([]string{"out.txt"}, []string{stage})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "did not produce declared output")
}

func TestBuild_OutputUnderMdsmithRefused(t *testing.T) {
	root := t.TempDir()
	b := NewCustomBuilder(map[string]RecipeSpec{"r": recipeCmd("echo hi {outputs}")})
	err := b.Build(context.Background(), Target{
		Recipe:  "r",
		Root:    root,
		Outputs: []string{".mdsmith/build-cache.json"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), ".mdsmith/")
	assert.NoFileExists(t, filepath.Join(root, ".mdsmith", "build-cache.json"))
}

func TestVerifyOutputsExist_StagedSymlinkRefused(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics")
	}
	stageDir := t.TempDir()
	target := filepath.Join(t.TempDir(), "secret")
	require.NoError(t, os.WriteFile(target, []byte("x"), 0o644))
	stage := filepath.Join(stageDir, "out0")
	require.NoError(t, os.Symlink(target, stage))

	err := verifyOutputsExist([]string{"out.txt"}, []string{stage})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "symlink")
}

func TestVerifyOutputsExist_StagedDirRefused(t *testing.T) {
	stageDir := t.TempDir()
	stage := filepath.Join(stageDir, "out0")
	require.NoError(t, os.MkdirAll(stage, 0o755))

	err := verifyOutputsExist([]string{"out.txt"}, []string{stage})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "non-regular")
}

func TestSubstituteParams_PrefixBeforePlaceholder(t *testing.T) {
	// A token with literal prefix text before a {name} placeholder exercises
	// the WriteByte branch that copies non-'{' characters one at a time.
	assert.Equal(t, "prefix-val", substituteParams("prefix-{name}", map[string]string{"name": "val"}))
}

func TestSubstituteParams_EmbeddedListPlaceholder(t *testing.T) {
	// {inputs} or {outputs} embedded inside a larger token (not a standalone
	// token) must pass through literally — the MDS040 validator rejects such
	// commands; here we verify the substituteParams passthrough.
	assert.Equal(t, "prefix-{inputs}-suffix", substituteParams("prefix-{inputs}-suffix", nil))
}

func TestBuild_MkdirTempError(t *testing.T) {
	old := mkdirTempFn
	mkdirTempFn = func(_, _ string) (string, error) { return "", errors.New("no temp dir") }
	defer func() { mkdirTempFn = old }()

	root := t.TempDir()
	b := NewCustomBuilder(map[string]RecipeSpec{"r": recipeCmd("echo hi")})
	err := b.Build(context.Background(), Target{
		Recipe:  "r",
		Root:    root,
		Outputs: []string{"out.txt"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "staging dir")
}

func TestVerifyOutputsExist_StatError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("ENOTDIR semantics differ on Windows")
	}
	root := t.TempDir()
	// Create a regular file at the same path as the expected staging dir so
	// os.Lstat on "notadir/out0" fails with ENOTDIR — a non-ENOENT error
	// that hits the "staging output" error branch rather than the
	// "did not produce" branch.
	notadir := filepath.Join(root, "notadir")
	require.NoError(t, os.WriteFile(notadir, []byte("x"), 0o644))

	err := verifyOutputsExist([]string{"out.txt"}, []string{filepath.Join(notadir, "out0")})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "staging output")
}

func TestBuild_SnapshotDirsError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sh is not available on Windows")
	}
	old := snapshotDirsFn
	snapshotDirsFn = func([]string, int, map[string]fileState) (map[string]fileState, error) {
		return nil, errors.New("before-snapshot failed")
	}
	t.Cleanup(func() { snapshotDirsFn = old })

	root := t.TempDir()
	bindir := t.TempDir()
	script := writeScript(t, bindir, "noop.sh", `printf x > "$1"`)
	b := NewCustomBuilder(map[string]RecipeSpec{
		"r": recipeCmd(script + " {outputs}"),
	})
	err := b.Build(context.Background(), Target{
		Recipe:  "r",
		Root:    root,
		Outputs: []string{"out.txt"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "before-snapshot failed")
}

func TestBuild_VerifyNoUndeclaredWritesSnapshotError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sh is not available on Windows")
	}
	// Fail only the SECOND snapshot call (the after-snapshot inside
	// verifyNoUndeclaredWrites); the first (before-snapshot) succeeds so the
	// recipe runs and we reach the after-snapshot error branch.
	var calls atomic.Int32
	old := snapshotDirsFn
	snapshotDirsFn = func(dirs []string, max int, prior map[string]fileState) (map[string]fileState, error) {
		if calls.Add(1) == 2 {
			return nil, errors.New("after-snapshot failed")
		}
		return snapshotDirs(dirs, max, prior)
	}
	t.Cleanup(func() { snapshotDirsFn = old })

	root := t.TempDir()
	bindir := t.TempDir()
	script := writeScript(t, bindir, "noop.sh", `printf x > "$1"`)
	b := NewCustomBuilder(map[string]RecipeSpec{
		"r": recipeCmd(script + " {outputs}"),
	})
	err := b.Build(context.Background(), Target{
		Recipe:  "r",
		Root:    root,
		Outputs: []string{"out.txt"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "after-snapshot failed")
}

func TestBuild_UndeclaredWriteDetected(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sh is not available on Windows")
	}
	root := t.TempDir()
	// The output parent is root itself (output is just "dst.txt"). The recipe
	// copies src to the staged output ($1) and also writes an undeclared
	// sibling at an absolute path in that same parent ($2), which the
	// post-condition snapshot must flag.
	require.NoError(t, os.WriteFile(filepath.Join(root, "src.txt"), []byte("hi"), 0o644))
	sneaky := filepath.Join(root, "sneaky.txt")
	bindir := t.TempDir()
	script := writeScript(t, bindir, "sneak.sh", `printf x > "$1"; echo evil > "$2"`)
	b := NewCustomBuilder(map[string]RecipeSpec{
		"r": recipeCmd(script + " {outputs} " + sneaky),
	})
	err := b.Build(context.Background(), Target{
		Recipe:  "r",
		Root:    root,
		Inputs:  []string{"src.txt"},
		Outputs: []string{"dst.txt"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "wrote outside its declared outputs")
	// The declared output must not be committed when the post-check fails.
	assert.NoFileExists(t, filepath.Join(root, "dst.txt"))
}

func TestCommitOutputs_RefusesSymlinkDest(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics differ on Windows")
	}
	root := t.TempDir()
	// Plant a symlink at the output destination. commitOutputs must refuse to
	// replace it rather than follow it through to the target. (A real Build
	// resolves an output symlink earlier, so commitOutputs is exercised
	// directly here to reach the symlink-destination guard.)
	target := filepath.Join(t.TempDir(), "outside")
	require.NoError(t, os.WriteFile(target, []byte("orig"), 0o644))
	final := filepath.Join(root, "dst.txt")
	require.NoError(t, os.Symlink(target, final))

	stageDir := t.TempDir()
	stage := filepath.Join(stageDir, "out0")
	require.NoError(t, os.WriteFile(stage, []byte("new"), 0o644))

	err := commitOutputs([]string{final}, []string{"dst.txt"}, []string{stage})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "symlink")
	// The symlink target outside the tree must be untouched (not followed).
	got, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.Equal(t, "orig", string(got))
}

func TestBuild_LstatErrorRefusesOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sh is not available on Windows")
	}
	old := lstatFn
	lstatFn = func(string) (os.FileInfo, error) { return nil, os.ErrPermission }
	t.Cleanup(func() { lstatFn = old })

	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "src.txt"), []byte("hi"), 0o644))
	b := NewCustomBuilder(map[string]RecipeSpec{
		"copy": recipeCmd("cp {inputs} {outputs}"),
	})
	err := b.Build(context.Background(), Target{
		Recipe:  "copy",
		Root:    root,
		Inputs:  []string{"src.txt"},
		Outputs: []string{"dst.txt"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "inspecting output")
}

func TestBuild_MultiOutputPartialFailure(t *testing.T) {
	// Force the rename→copy fallback for every output, and make the copy
	// fail on the SECOND output so the i>0 partial-failure branch fires.
	oldRename := renameFn
	renameFn = func(string, string) error { return errors.New("cross-device") }
	t.Cleanup(func() { renameFn = oldRename })

	var copyCalls atomic.Int32
	oldCopy := copyFileImplFn
	copyFileImplFn = func(src, dst string) error {
		if copyCalls.Add(1) == 2 {
			return errors.New("copy failed on second output")
		}
		return copyFile(src, dst)
	}
	t.Cleanup(func() { copyFileImplFn = oldCopy })

	root := t.TempDir()
	stageDir := t.TempDir()
	stage0 := filepath.Join(stageDir, "out0")
	stage1 := filepath.Join(stageDir, "out1")
	require.NoError(t, os.WriteFile(stage0, []byte("a"), 0o644))
	require.NoError(t, os.WriteFile(stage1, []byte("b"), 0o644))

	finals := []string{filepath.Join(root, "a.txt"), filepath.Join(root, "b.txt")}
	err := commitOutputs(finals, []string{"a.txt", "b.txt"}, []string{stage0, stage1})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already committed")
	// The first output committed via the copy fallback.
	got, rerr := os.ReadFile(finals[0])
	require.NoError(t, rerr)
	assert.Equal(t, "a", string(got))
}
