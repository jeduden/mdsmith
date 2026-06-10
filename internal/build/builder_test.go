package build

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recipeCmd builds a RecipeSpec whose command is the given string.
func recipeCmd(command string) RecipeSpec {
	return RecipeSpec{Command: command}
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
	// A glob cap breach is a build error. We simulate by setting a tiny
	// cap is not possible (const), so instead verify the helper wiring by
	// matching a directory with many files would be too slow; instead we
	// confirm a normal small glob does not error (covered elsewhere) and
	// trust CheckGlobMatchCap unit tests in the rules/build package.
	t.Skip("cap breach exercised via rules/build unit tests; 10k files too slow here")
}

func TestArgvExpansion_ListsExpandPerEntry(t *testing.T) {
	// {outputs} and {inputs} each expand to one argv per resolved entry.
	argv, err := expandArgv(
		strings.Fields("tool {inputs} -o {outputs}"),
		map[string]string{},
		[]string{"in1", "in2"},
		[]string{"out1"},
	)
	require.NoError(t, err)
	assert.Equal(t, []string{"tool", "in1", "in2", "-o", "out1"}, argv)
}

func TestArgvExpansion_ParamWhitespaceStaysOneEntry(t *testing.T) {
	argv, err := expandArgv(
		strings.Fields("tool {value}"),
		map[string]string{"value": "a b c"},
		nil, nil,
	)
	require.NoError(t, err)
	assert.Equal(t, []string{"tool", "a b c"}, argv)
}

func TestSubstituteParams_UnclosedBrace(t *testing.T) {
	// An unclosed { is written through literally rather than panicking.
	out, err := substituteParams("{unclosed", nil)
	require.NoError(t, err)
	assert.Equal(t, "{unclosed", out)
}

func TestSubstituteParams_AbsentOptionalParam(t *testing.T) {
	// A {name} placeholder with no matching param expands to the empty string.
	out, err := substituteParams("{missing}", map[string]string{})
	require.NoError(t, err)
	assert.Equal(t, "", out)
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
	// "[z-a]" is a character class with inverted range — doublestar returns
	// an error for it.
	err := b.Build(context.Background(), Target{
		Recipe:  "echo",
		Root:    root,
		Inputs:  []string{"[z-a]"},
		Outputs: []string{"out.txt"},
	})
	require.Error(t, err)
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

func TestSubstituteParams_PrefixBeforePlaceholder(t *testing.T) {
	// A token with literal prefix text before a {name} placeholder exercises
	// the WriteByte branch that copies non-'{' characters one at a time.
	out, err := substituteParams("prefix-{name}", map[string]string{"name": "val"})
	require.NoError(t, err)
	assert.Equal(t, "prefix-val", out)
}

func TestSubstituteParams_EmbeddedListPlaceholder(t *testing.T) {
	// {inputs} or {outputs} embedded inside a larger token (not a standalone
	// token) must pass through literally — the MDS040 validator rejects such
	// commands; here we verify the substituteParams passthrough.
	out, err := substituteParams("prefix-{inputs}-suffix", nil)
	require.NoError(t, err)
	assert.Equal(t, "prefix-{inputs}-suffix", out)
}
