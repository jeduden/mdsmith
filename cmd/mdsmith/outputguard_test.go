package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// guardWorkspace makes a temp working directory holding notes.md,
// docs/a.md, docs/sub/, other/, and a non-Markdown data.txt, and
// changes into it for the test.
func guardWorkspace(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Chdir(dir)
	require.NoError(t, os.MkdirAll(filepath.Join("docs", "sub"), 0o755))
	require.NoError(t, os.MkdirAll("other", 0o755))
	for _, f := range []string{"notes.md", filepath.Join("docs", "a.md"), "data.txt"} {
		require.NoError(t, os.WriteFile(f, []byte("# X\n"), 0o644))
	}
	return dir
}

// symlinkOrSkip makes link point at target, or skips the test where
// the platform or user cannot create symlinks.
func symlinkOrSkip(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("cannot create symlinks here: %v", err)
	}
}

// An existing -o path is an input when it is the same file as a
// resolved input, however it is spelled.
func TestOutputIsInput_ExistingFile(t *testing.T) {
	dir := guardWorkspace(t)
	in := runInputs{files: []string{"notes.md", filepath.Join("docs", "a.md")}, args: []string{"notes.md", "docs"}}
	for _, tc := range []struct {
		output string
		want   bool
	}{
		{"notes.md", true},
		{"./notes.md", true},
		{filepath.Join("docs", "..", "notes.md"), true},
		{filepath.Join(dir, "docs", "a.md"), true},
		{"data.txt", false},
		{os.DevNull, false},
	} {
		assert.Equal(t, tc.want, outputIsInput(tc.output, in), "-o %s", tc.output)
	}
}

// A hard link or a symlink to an input is the input.
func TestOutputIsInput_LinksToAnInput(t *testing.T) {
	guardWorkspace(t)
	in := runInputs{files: []string{"notes.md"}, args: []string{"notes.md"}}
	require.NoError(t, os.Link("notes.md", "hard.json"))
	assert.True(t, outputIsInput("hard.json", in), "hard link")
	symlinkOrSkip(t, "notes.md", "soft.json")
	assert.True(t, outputIsInput("soft.json", in), "symlink")
}

// A missing -o path is an input when the run would pick it up once
// the report creates it.
func TestOutputIsInput_WouldBeInput(t *testing.T) {
	dir := guardWorkspace(t)
	for _, tc := range []struct {
		name   string
		output string
		in     runInputs
		want   bool
	}{
		{"markdown in a directory argument", filepath.Join("docs", "report.md"),
			runInputs{args: []string{"docs"}}, true},
		{"markdown deep in a directory argument", filepath.Join("docs", "sub", "report.markdown"),
			runInputs{args: []string{"docs"}}, true},
		{"absolute output, relative argument", filepath.Join(dir, "docs", "report.md"),
			runInputs{args: []string{"docs"}}, true},
		{"relative output, absolute argument", filepath.Join("docs", "report.md"),
			runInputs{args: []string{filepath.Join(dir, "docs")}}, true},
		{"markdown in the working directory, argument .", "report.md",
			runInputs{args: []string{"."}}, true},
		{"upper-case extension", filepath.Join("docs", "REPORT.MD"),
			runInputs{args: []string{"docs"}}, true},
		{"output in the parent of cwd, argument ..", filepath.Join("..", "report.md"),
			runInputs{args: []string{".."}}, true},
		{"non-Markdown in a directory argument", filepath.Join("docs", "report.json"),
			runInputs{args: []string{"docs"}}, false},
		{"markdown outside the arguments", filepath.Join("other", "report.md"),
			runInputs{args: []string{"docs", "notes.md"}}, false},
		{"file argument only", "report.md",
			runInputs{args: []string{"notes.md"}}, false},
		{"missing parent directory", filepath.Join("missing", "report.md"),
			runInputs{args: []string{"."}}, false},
		{"glob argument", filepath.Join("docs", "report.md"),
			runInputs{args: []string{"docs/*.md"}}, true},
		{"glob argument, other directory", filepath.Join("other", "report.md"),
			runInputs{args: []string{"docs/*.md"}}, false},
		{"glob matching an ancestor directory", filepath.Join("docs", "sub", "report.md"),
			runInputs{args: []string{"d*"}}, true},
		{"glob not matching the name", filepath.Join("docs", "report.md"),
			runInputs{args: []string{"docs/*.txt"}}, false},
		{"glob with a missing base", filepath.Join("docs", "report.md"),
			runInputs{args: []string{"missing/*.md"}}, false},
		{"glob matched without regard to case", filepath.Join("docs", "report.md"),
			runInputs{args: []string{"docs/*.MD"}}, true},
		{"discovery pattern", "report.md",
			runInputs{patterns: []string{"**/*.md"}}, true},
		{"discovery pattern, subdirectory", filepath.Join("docs", "sub", "report.md"),
			runInputs{patterns: []string{"docs/**/*.md"}}, true},
		{"discovery pattern not matching", filepath.Join("other", "report.md"),
			runInputs{patterns: []string{"docs/**/*.md"}}, false},
		{"discovery, output above the working directory", filepath.Join("..", "report.md"),
			runInputs{patterns: []string{"**/*.md"}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, outputIsInput(tc.output, tc.in))
		})
	}
}

// Symlinks in the output's directory are resolved: a report written
// through a symlinked directory lands in the directory it points at.
func TestOutputIsInput_SymlinkedParent(t *testing.T) {
	guardWorkspace(t)
	symlinkOrSkip(t, "docs", "alias")
	assert.True(t, outputIsInput(filepath.Join("alias", "report.md"), runInputs{args: []string{"docs"}}))
	symlinkOrSkip(t, "other", "out")
	assert.False(t, outputIsInput(filepath.Join("out", "report.md"), runInputs{args: []string{"docs"}}))
	// The resolver never walks a symlinked directory argument.
	assert.False(t, outputIsInput(filepath.Join("docs", "report.md"), runInputs{args: []string{"alias"}}))
}

// A dangling symlink at the -o path is followed: opening it with
// O_CREATE creates its final target, so that target is what the
// would-be-input check sees, whatever the link's own name.
func TestOutputIsInput_DanglingSymlink(t *testing.T) {
	dir := guardWorkspace(t)
	docs := runInputs{args: []string{"docs"}}
	symlinkOrSkip(t, filepath.Join("docs", "new.md"), "report.txt")
	assert.True(t, outputIsInput("report.txt", docs), "a link into a directory argument")

	require.NoError(t, os.Symlink("report.txt", "hop.txt"))
	assert.True(t, outputIsInput("hop.txt", docs), "a chain of links")

	require.NoError(t, os.Symlink(filepath.Join(dir, "docs", "abs.md"), "abs.txt"))
	assert.True(t, outputIsInput("abs.txt", docs), "an absolute target")

	// A relative target is read from the link's own directory, here
	// docs/ reached through the symlinked alias/.
	require.NoError(t, os.Symlink("docs", "alias"))
	require.NoError(t, os.Symlink("rel.md", filepath.Join("docs", "rel.txt")))
	assert.True(t, outputIsInput(filepath.Join("alias", "rel.txt"), docs), "a relative target")

	require.NoError(t, os.Symlink(filepath.Join("other", "new.md"), "away.txt"))
	assert.False(t, outputIsInput("away.txt", docs), "a target outside the arguments")

	require.NoError(t, os.Symlink(filepath.Join("missing", "new.md"), "nodir.txt"))
	assert.False(t, outputIsInput("nodir.txt", docs), "a target whose directory is missing")

	require.NoError(t, os.Symlink("loop-b.md", "loop-a.md"))
	require.NoError(t, os.Symlink("loop-a.md", "loop-b.md"))
	assert.False(t, outputIsInput("loop-a.md", runInputs{args: []string{"."}}), "a link loop")
}

func TestCreateTarget(t *testing.T) {
	dir := guardWorkspace(t)
	got, err := createTarget(filepath.Join("docs", "new.md"))
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "docs", "new.md"), got, "a missing name is its own target")

	_, err = createTarget(filepath.Join("missing", "new.md"))
	assert.Error(t, err, "a missing directory")

	symlinkOrSkip(t, "loop-b.md", "loop-a.md")
	require.NoError(t, os.Symlink("loop-a.md", "loop-b.md"))
	_, err = createTarget("loop-a.md")
	assert.ErrorIs(t, err, errTooManyLinks)
}

func TestRefuseOutputOverInput(t *testing.T) {
	guardWorkspace(t)
	in := runInputs{files: []string{"notes.md"}, args: []string{"notes.md"}}
	for _, output := range []string{"", "-", "report.json"} {
		assert.Equal(t, -1, refuseOutputOverInput("check", output, in), "-o %q", output)
	}
	stderr := captureStderr(func() {
		assert.Equal(t, 2, refuseOutputOverInput("fix", "notes.md", in))
	})
	assert.Equal(t,
		"mdsmith: fix: refusing --output \"notes.md\": it is an input of this run, or would be once written\n",
		stderr)
}

// `check - -o a.md < a.md` would truncate a.md, the file stdin reads,
// with the report. The -o path is compared with stdin by file
// identity, before stdin is read.
func TestRefuseOutputOverStdin(t *testing.T) {
	guardWorkspace(t)
	stdin, err := os.Open("notes.md")
	require.NoError(t, err)
	defer stdin.Close() //nolint:errcheck // test cleanup
	for _, output := range []string{"", "-", "data.txt", "missing.txt"} {
		assert.Equal(t, -1, refuseOutputOverStdin("check", output, stdin), "-o %q", output)
	}
	stderr := captureStderr(func() {
		assert.Equal(t, 2, refuseOutputOverStdin("check", "./notes.md", stdin))
	})
	assert.Equal(t,
		"mdsmith: check: refusing --output \"./notes.md\": it is an input of this run, or would be once written\n",
		stderr)

	// A stdin whose Stat fails has no identity to match.
	closed, err := os.Open("notes.md")
	require.NoError(t, err)
	require.NoError(t, closed.Close())
	assert.Equal(t, -1, refuseOutputOverStdin("check", "notes.md", closed))
}
