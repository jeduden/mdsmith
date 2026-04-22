package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- singleNameArg ---

func TestSingleNameArg_OneArg_ReturnsNameNegCode(t *testing.T) {
	name, code := singleNameArg("show", []string{"story"})
	assert.Equal(t, "story", name)
	assert.Equal(t, -1, code)
}

func TestSingleNameArg_HelpLong_ExitsZero(t *testing.T) {
	captureStderr(func() {
		name, code := singleNameArg("show", []string{"--help"})
		assert.Equal(t, "", name)
		assert.Equal(t, 0, code)
	})
}

func TestSingleNameArg_HelpShort_ExitsZero(t *testing.T) {
	captureStderr(func() {
		name, code := singleNameArg("path", []string{"-h"})
		assert.Equal(t, "", name)
		assert.Equal(t, 0, code)
	})
}

func TestSingleNameArg_NoArgs_ExitsTwo(t *testing.T) {
	captureStderr(func() {
		_, code := singleNameArg("show", []string{})
		assert.Equal(t, 2, code)
	})
}

func TestSingleNameArg_TooManyArgs_ExitsTwo(t *testing.T) {
	captureStderr(func() {
		_, code := singleNameArg("show", []string{"a", "b"})
		assert.Equal(t, 2, code)
	})
}

// --- writeIfAbsent ---

func TestWriteIfAbsent_CreatesFileWhenAbsent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "new.md")

	wrote, err := writeIfAbsent(path, "new content")
	require.NoError(t, err)
	assert.True(t, wrote)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "new content", string(data))
}

func TestWriteIfAbsent_SkipsExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "existing.md")
	require.NoError(t, os.WriteFile(path, []byte("original"), 0644))

	wrote, err := writeIfAbsent(path, "overwrite attempt")
	require.NoError(t, err)
	assert.False(t, wrote)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "original", string(data))
}

func TestWriteIfAbsent_UnwritablePath_Error(t *testing.T) {
	_, err := writeIfAbsent("/no/such/dir/file.md", "content")
	assert.Error(t, err)
}

// --- runArchetypes dispatch ---

func TestRunArchetypes_NoArgs_ExitsZero(t *testing.T) {
	captureStderr(func() {
		code := runArchetypes(nil)
		assert.Equal(t, 0, code)
	})
}

func TestRunArchetypes_HelpLong_ExitsZero(t *testing.T) {
	captureStderr(func() {
		code := runArchetypes([]string{"--help"})
		assert.Equal(t, 0, code)
	})
}

func TestRunArchetypes_HelpShort_ExitsZero(t *testing.T) {
	captureStderr(func() {
		code := runArchetypes([]string{"-h"})
		assert.Equal(t, 0, code)
	})
}

func TestRunArchetypes_UnknownSubcommand_ExitsTwo(t *testing.T) {
	got := captureStderr(func() {
		code := runArchetypes([]string{"unknown"})
		assert.Equal(t, 2, code)
	})
	assert.Contains(t, got, "unknown subcommand")
}

// --- runArchetypesInit ---

func TestRunArchetypesInit_HelpLong_ExitsZero(t *testing.T) {
	captureStderr(func() {
		code := runArchetypesInit([]string{"--help"})
		assert.Equal(t, 0, code)
	})
}

func TestRunArchetypesInit_HelpShort_ExitsZero(t *testing.T) {
	captureStderr(func() {
		code := runArchetypesInit([]string{"-h"})
		assert.Equal(t, 0, code)
	})
}

func TestRunArchetypesInit_TooManyArgs_ExitsTwo(t *testing.T) {
	captureStderr(func() {
		code := runArchetypesInit([]string{"a", "b"})
		assert.Equal(t, 2, code)
	})
}

func TestRunArchetypesInit_DefaultDir_CreatesFiles(t *testing.T) {
	dir := t.TempDir()
	oldWd, err := os.Getwd()
	require.NoError(t, err)
	defer func() { _ = os.Chdir(oldWd) }()
	require.NoError(t, os.Chdir(dir))

	captureStderr(func() {
		code := runArchetypesInit(nil)
		assert.Equal(t, 0, code)
	})

	assert.FileExists(t, filepath.Join(dir, "archetypes", "example.md"))
	assert.FileExists(t, filepath.Join(dir, "archetypes", "README.md"))
}

func TestRunArchetypesInit_CustomDir_CreatesFiles(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	captureStderr(func() {
		code := runArchetypesInit([]string{"schemas"})
		assert.Equal(t, 0, code)
	})

	assert.FileExists(t, filepath.Join(dir, "schemas", "example.md"))
	assert.FileExists(t, filepath.Join(dir, "schemas", "README.md"))
}

func TestRunArchetypesInit_ExistingFiles_Preserved(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	require.NoError(t, os.MkdirAll("schemas", 0o755))
	examplePath := filepath.Join(dir, "schemas", "example.md")
	require.NoError(t, os.WriteFile(examplePath, []byte("custom content"), 0644))

	captureStderr(func() {
		code := runArchetypesInit([]string{"schemas"})
		assert.Equal(t, 0, code)
	})

	data, err := os.ReadFile(examplePath)
	require.NoError(t, err)
	assert.Equal(t, "custom content", string(data))
}

func TestRunArchetypesInit_Idempotent(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	captureStderr(func() { runArchetypesInit([]string{"schemas"}) })
	captureStderr(func() {
		code := runArchetypesInit([]string{"schemas"})
		assert.Equal(t, 0, code)
	})
}

func TestRunArchetypesInit_NestedDir_CreatesIntermediary(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	captureStderr(func() {
		code := runArchetypesInit([]string{"a/b/c"})
		assert.Equal(t, 0, code)
	})
	assert.FileExists(t, filepath.Join(dir, "a", "b", "c", "example.md"))
}

// --- runArchetypesList ---

func TestRunArchetypesList_HelpFlag_ExitsZero(t *testing.T) {
	captureStderr(func() {
		code := runArchetypesList([]string{"--help"})
		assert.Equal(t, 0, code)
	})
}

func TestRunArchetypesList_ExtraArg_ExitsTwo(t *testing.T) {
	captureStderr(func() {
		code := runArchetypesList([]string{"extra"})
		assert.Equal(t, 2, code)
	})
}

// --- runArchetypesShow ---

func TestRunArchetypesShow_HelpFlag_ExitsZero(t *testing.T) {
	captureStderr(func() {
		code := runArchetypesShow([]string{"-h"})
		assert.Equal(t, 0, code)
	})
}

func TestRunArchetypesShow_NoArgs_ExitsTwo(t *testing.T) {
	captureStderr(func() {
		code := runArchetypesShow(nil)
		assert.Equal(t, 2, code)
	})
}

// --- runArchetypesPath ---

func TestRunArchetypesPath_HelpFlag_ExitsZero(t *testing.T) {
	captureStderr(func() {
		code := runArchetypesPath([]string{"--help"})
		assert.Equal(t, 0, code)
	})
}

func TestRunArchetypesPath_NoArgs_ExitsTwo(t *testing.T) {
	captureStderr(func() {
		code := runArchetypesPath(nil)
		assert.Equal(t, 2, code)
	})
}
