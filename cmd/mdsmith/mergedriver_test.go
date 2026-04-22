package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStripSectionConflicts_Diff3CatalogConflict(t *testing.T) {
	// diff3-style conflict markers include a ||||||| base section
	// between <<<<<<< and =======. The merge driver must strip all
	// four marker types inside regenerable sections.
	input := "# Doc\n\n" +
		"<?catalog\nglob: \"plans/*.md\"\nsort: title\n" +
		"header: |\n  | Title |\n  |-------|\nrow: \"| [{title}]({filename}) |\"\n?>\n" +
		"<<<<<<< ours\n" +
		"| [Alpha](plans/alpha.md) |\n" +
		"| [Beta](plans/beta.md) |\n" +
		"||||||| base\n" +
		"| [Alpha](plans/alpha.md) |\n" +
		"=======\n" +
		"| [Alpha](plans/alpha.md) |\n" +
		"| [Gamma](plans/gamma.md) |\n" +
		">>>>>>> theirs\n" +
		"<?/catalog?>\n"

	result := string(stripSectionConflicts([]byte(input)))

	assert.NotContains(t, result, "<<<<<<<", "expected <<<<<<< marker stripped")
	assert.NotContains(t, result, "|||||||", "expected ||||||| base marker stripped")
	assert.NotContains(t, result, "=======", "expected ======= separator stripped")
	assert.NotContains(t, result, ">>>>>>>", "expected >>>>>>> marker stripped")
}

func TestStripSectionConflicts_Diff3OutsideSection_Preserved(t *testing.T) {
	// diff3 conflict markers outside regenerable sections must be
	// preserved so the user can resolve them manually.
	input := "# Doc\n\n" +
		"<<<<<<< ours\n" +
		"ours text\n" +
		"||||||| base\n" +
		"base text\n" +
		"=======\n" +
		"theirs text\n" +
		">>>>>>> theirs\n"

	result := string(stripSectionConflicts([]byte(input)))

	assert.Contains(t, result, "<<<<<<<", "expected <<<<<<< marker preserved outside section")
	assert.Contains(t, result, "|||||||", "expected ||||||| marker preserved outside section")
	assert.Contains(t, result, "=======", "expected ======= separator preserved outside section")
	assert.Contains(t, result, ">>>>>>>", "expected >>>>>>> marker preserved outside section")
}

// --- isConflictOpen ---

func TestIsConflictOpen_True(t *testing.T) {
	assert.True(t, isConflictOpen([]byte("<<<<<<< HEAD")))
	assert.True(t, isConflictOpen([]byte("<<<<<<<extra")))
}

func TestIsConflictOpen_False(t *testing.T) {
	assert.False(t, isConflictOpen([]byte("normal text")))
	assert.False(t, isConflictOpen([]byte("<<<<<< only six")))
}

// --- isConflictBase ---

func TestIsConflictBase_True(t *testing.T) {
	assert.True(t, isConflictBase([]byte("||||||| base")))
	assert.True(t, isConflictBase([]byte("|||||||")))
}

func TestIsConflictBase_False(t *testing.T) {
	assert.False(t, isConflictBase([]byte("normal")))
	assert.False(t, isConflictBase([]byte("<<<<<<< HEAD")))
}

// --- isConflictSeparator ---

func TestIsConflictSeparator_True(t *testing.T) {
	assert.True(t, isConflictSeparator([]byte("=======")))
	assert.True(t, isConflictSeparator([]byte("======= extra")))
}

func TestIsConflictSeparator_False(t *testing.T) {
	assert.False(t, isConflictSeparator([]byte("<<<<<<< HEAD")))
	assert.False(t, isConflictSeparator([]byte("======")))
}

// --- isConflictClose ---

func TestIsConflictClose_True(t *testing.T) {
	assert.True(t, isConflictClose([]byte(">>>>>>> theirs")))
	assert.True(t, isConflictClose([]byte(">>>>>>>")))
}

func TestIsConflictClose_False(t *testing.T) {
	assert.False(t, isConflictClose([]byte("normal")))
	assert.False(t, isConflictClose([]byte("<<<<<<< HEAD")))
}

// --- hasConflictMarkers ---

func TestHasConflictMarkers_WithOpenClose(t *testing.T) {
	content := []byte("line one\n<<<<<<< HEAD\nours\n=======\ntheirs\n>>>>>>> theirs\n")
	assert.True(t, hasConflictMarkers(content))
}

func TestHasConflictMarkers_OpenOnly(t *testing.T) {
	content := []byte("<<<<<<< HEAD\nours\n")
	assert.True(t, hasConflictMarkers(content))
}

func TestHasConflictMarkers_CloseOnly(t *testing.T) {
	content := []byte("some text\n>>>>>>> theirs\n")
	assert.True(t, hasConflictMarkers(content))
}

func TestHasConflictMarkers_None(t *testing.T) {
	content := []byte("# Clean file\n\nSome content.\n")
	assert.False(t, hasConflictMarkers(content))
}

func TestHasConflictMarkers_SetextHeading_NotConflict(t *testing.T) {
	// "=======" on its own is a setext heading underline, not a conflict
	content := []byte("Heading\n=======\n\nContent.\n")
	assert.False(t, hasConflictMarkers(content))
}

func TestHasConflictMarkers_Empty(t *testing.T) {
	assert.False(t, hasConflictMarkers(nil))
}

// --- matchesAnyStart / matchesAnyEnd ---

func TestMatchesAnyStart_Match(t *testing.T) {
	names := []string{"catalog", "include"}
	assert.True(t, matchesAnyStart([]byte("<?catalog glob: \"**/*.md\" ?>"), names))
	assert.True(t, matchesAnyStart([]byte("<?include file: foo.md ?>"), names))
}

func TestMatchesAnyStart_NoMatch(t *testing.T) {
	names := []string{"catalog", "include"}
	assert.False(t, matchesAnyStart([]byte("regular line"), names))
	assert.False(t, matchesAnyStart([]byte("<?/catalog?>"), names))
}

func TestMatchesAnyEnd_Match(t *testing.T) {
	names := []string{"catalog", "include"}
	assert.True(t, matchesAnyEnd([]byte("<?/catalog?>"), names))
	assert.True(t, matchesAnyEnd([]byte("<?/include?>"), names))
}

func TestMatchesAnyEnd_NoMatch(t *testing.T) {
	names := []string{"catalog", "include"}
	assert.False(t, matchesAnyEnd([]byte("regular line"), names))
	assert.False(t, matchesAnyEnd([]byte("<?catalog glob: \"*\" ?>"), names))
}

// --- ensureGitattributes ---

func TestEnsureGitattributes_CreatesFileWithEntry(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".gitattributes")

	err := ensureGitattributes(path, []string{"PLAN.md"})
	require.NoError(t, err)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(data), "PLAN.md merge=mdsmith")
}

func TestEnsureGitattributes_AppendsMissingEntries(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".gitattributes")
	require.NoError(t, os.WriteFile(path, []byte("*.md text\n"), 0644))

	err := ensureGitattributes(path, []string{"PLAN.md"})
	require.NoError(t, err)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(data), "*.md text")
	assert.Contains(t, string(data), "PLAN.md merge=mdsmith")
}

func TestEnsureGitattributes_Idempotent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".gitattributes")

	require.NoError(t, ensureGitattributes(path, []string{"PLAN.md"}))
	require.NoError(t, ensureGitattributes(path, []string{"PLAN.md"}))

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	// Entry should appear exactly once.
	assert.Equal(t, 1, strings.Count(string(data), "PLAN.md merge=mdsmith"))
}

func TestEnsureGitattributes_MultipleFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".gitattributes")

	err := ensureGitattributes(path, []string{"PLAN.md", "README.md"})
	require.NoError(t, err)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(data), "PLAN.md merge=mdsmith")
	assert.Contains(t, string(data), "README.md merge=mdsmith")
}

func TestEnsureGitattributes_AddsOnlyMissingEntries(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".gitattributes")
	require.NoError(t, os.WriteFile(path, []byte("PLAN.md merge=mdsmith\n"), 0644))

	err := ensureGitattributes(path, []string{"PLAN.md", "README.md"})
	require.NoError(t, err)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, 1, strings.Count(string(data), "PLAN.md merge=mdsmith"))
	assert.Contains(t, string(data), "README.md merge=mdsmith")
}

func TestEnsureGitattributes_NoTrailingNewlineInExisting_Handled(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".gitattributes")
	// File without trailing newline.
	require.NoError(t, os.WriteFile(path, []byte("*.md text"), 0644))

	err := ensureGitattributes(path, []string{"PLAN.md"})
	require.NoError(t, err)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	// Trailing newline should be added before the new entry.
	assert.Contains(t, string(data), "PLAN.md merge=mdsmith")
}

// --- runMergeDriver dispatch ---

func TestRunMergeDriver_NoArgs_ExitsZero(t *testing.T) {
	captureStderr(func() {
		code := runMergeDriver(nil)
		assert.Equal(t, 0, code)
	})
}

func TestRunMergeDriver_HelpLong_ExitsZero(t *testing.T) {
	captureStderr(func() {
		code := runMergeDriver([]string{"--help"})
		assert.Equal(t, 0, code)
	})
}

func TestRunMergeDriver_HelpShort_ExitsZero(t *testing.T) {
	captureStderr(func() {
		code := runMergeDriver([]string{"-h"})
		assert.Equal(t, 0, code)
	})
}

func TestRunMergeDriver_UnknownSubcommand_ExitsTwo(t *testing.T) {
	got := captureStderr(func() {
		code := runMergeDriver([]string{"unknown"})
		assert.Equal(t, 2, code)
	})
	assert.Contains(t, got, "unknown subcommand")
}

func TestRunMergeDriverRun_HelpFlag_ExitsZero(t *testing.T) {
	captureStderr(func() {
		code := runMergeDriverRun([]string{"--help"})
		assert.Equal(t, 0, code)
	})
}

func TestRunMergeDriverRun_TooFewArgs_ExitsTwo(t *testing.T) {
	captureStderr(func() {
		code := runMergeDriverRun([]string{"base", "ours"})
		assert.Equal(t, 2, code)
	})
}

func TestRunMergeDriverInstall_HelpFlag_ExitsZero(t *testing.T) {
	captureStderr(func() {
		code := runMergeDriverInstall([]string{"--help"})
		assert.Equal(t, 0, code)
	})
}
