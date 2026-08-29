package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jeduden/mdsmith/internal/query"
)

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
