//go:build unix || windows

package build

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
