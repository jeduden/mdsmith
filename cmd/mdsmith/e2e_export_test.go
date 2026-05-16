package main_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// freshTOCFile is a Markdown source whose TOC body matches what the
// engine would generate, so default-mode export accepts it.
const freshTOCFile = `# Title

<?toc?>

- [Section](#section)

<?/toc?>

## Section

Body content.
`

// staleTOCFile points to a heading that doesn't exist; default-mode
// export must refuse and Fix mode must regenerate.
const staleTOCFile = `# Title

<?toc?>

- [Wrong](#wrong)

<?/toc?>

## Section

Body content.
`

func TestE2E_Export_DefaultMode_Fresh(t *testing.T) {
	dir := t.TempDir()
	isolateDir(t, dir)
	path := writeFixture(t, dir, "doc.md", freshTOCFile)

	stdout, stderr, code := runBinary(t, "", "export", path)
	require.Equal(t, 0, code, "expected exit 0, got %d (stderr=%s)", code, stderr)
	assert.NotContains(t, stdout, "<?toc")
	assert.NotContains(t, stdout, "<?/toc")
	assert.Contains(t, stdout, "- [Section](#section)")
	assert.Contains(t, stdout, "## Section")
}

func TestE2E_Export_DefaultMode_StaleBody_Refuses(t *testing.T) {
	dir := t.TempDir()
	isolateDir(t, dir)
	path := writeFixture(t, dir, "doc.md", staleTOCFile)

	stdout, stderr, code := runBinary(t, "", "export", path)
	assert.Equal(t, 1, code, "expected exit 1, got %d", code)
	assert.Empty(t, stdout, "stale body must emit no output")
	assert.Contains(t, stderr, "out of date",
		"diagnostic should describe the stale body, got: %s", stderr)
	assert.Contains(t, stderr, "MDS038",
		"diagnostic should name the toc rule, got: %s", stderr)

	// Source file must not be modified.
	bytes, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, staleTOCFile, string(bytes),
		"source file must be unchanged after a refused export")
}

func TestE2E_Export_FixMode_RegeneratesStaleBody(t *testing.T) {
	dir := t.TempDir()
	isolateDir(t, dir)
	path := writeFixture(t, dir, "doc.md", staleTOCFile)

	stdout, stderr, code := runBinary(t, "", "export", "--fix", path)
	require.Equal(t, 0, code, "expected exit 0, got %d (stderr=%s)", code, stderr)
	assert.NotContains(t, stdout, "<?toc")
	// Regenerated body links to the real heading.
	assert.Contains(t, stdout, "- [Section](#section)")
	assert.NotContains(t, stdout, "Wrong")

	// Source file is still the stale original — fix mode is in-memory.
	bytes, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, staleTOCFile, string(bytes),
		"source file must be unchanged in --fix mode")
}

func TestE2E_Export_NoCheckMode_StaleBody_ExportsAsIs(t *testing.T) {
	dir := t.TempDir()
	isolateDir(t, dir)
	path := writeFixture(t, dir, "doc.md", staleTOCFile)

	stdout, stderr, code := runBinary(t, "", "export", "--no-check", path)
	require.Equal(t, 0, code, "expected exit 0, got %d (stderr=%s)", code, stderr)
	assert.NotContains(t, stdout, "<?toc")
	// On-disk body kept verbatim, even though it's stale.
	assert.Contains(t, stdout, "- [Wrong](#wrong)")
}

func TestE2E_Export_FixAndNoCheck_Conflict(t *testing.T) {
	dir := t.TempDir()
	isolateDir(t, dir)
	path := writeFixture(t, dir, "doc.md", freshTOCFile)

	_, stderr, code := runBinary(t, "", "export", "--fix", "--no-check", path)
	assert.Equal(t, 2, code, "passing both flags should exit 2")
	assert.Contains(t, stderr, "mutually exclusive")
}

func TestE2E_Export_OutputFlag_WritesFile(t *testing.T) {
	dir := t.TempDir()
	isolateDir(t, dir)
	src := writeFixture(t, dir, "doc.md", freshTOCFile)
	dst := filepath.Join(dir, "out.md")

	stdout, stderr, code := runBinary(t, "", "export", "-o", dst, src)
	require.Equal(t, 0, code, "expected exit 0, got %d (stderr=%s)", code, stderr)
	assert.Empty(t, stdout, "stdout should be empty when -o is given")

	written, err := os.ReadFile(dst)
	require.NoError(t, err)
	assert.NotContains(t, string(written), "<?toc")
	assert.Contains(t, string(written), "- [Section](#section)")
}

func TestE2E_Export_NoArgs_ExitsTwo(t *testing.T) {
	_, stderr, code := runBinary(t, "", "export")
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "requires a file argument")
}

func TestE2E_Export_MultipleArgs_ExitsTwo(t *testing.T) {
	dir := t.TempDir()
	isolateDir(t, dir)
	a := writeFixture(t, dir, "a.md", freshTOCFile)
	b := writeFixture(t, dir, "b.md", freshTOCFile)

	_, stderr, code := runBinary(t, "", "export", a, b)
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "single file argument")
}

func TestE2E_Export_MissingFile_ExitsTwo(t *testing.T) {
	dir := t.TempDir()
	isolateDir(t, dir)
	_, stderr, code := runBinary(t, "", "export", filepath.Join(dir, "nope.md"))
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "no such file")
}

func TestE2E_Export_NoDirectiveFile_Noop(t *testing.T) {
	dir := t.TempDir()
	isolateDir(t, dir)
	src := "# Title\n\nNo directives here at all.\n"
	path := writeFixture(t, dir, "doc.md", src)

	stdout, _, code := runBinary(t, "", "export", path)
	require.Equal(t, 0, code)
	assert.Equal(t, src, stdout)
}

func TestE2E_Export_Idempotent(t *testing.T) {
	dir := t.TempDir()
	isolateDir(t, dir)
	path := writeFixture(t, dir, "doc.md", freshTOCFile)

	first, _, code := runBinary(t, "", "export", path)
	require.Equal(t, 0, code)

	// Save the first output and export it again.
	roundTrip := writeFixture(t, dir, "round.md", first)
	second, _, code := runBinary(t, "", "export", roundTrip)
	require.Equal(t, 0, code)

	assert.Equal(t, first, second,
		"export should be idempotent: exporting an exported file is a no-op")
}

func TestE2E_Export_FreshOutputPassesCheck(t *testing.T) {
	dir := t.TempDir()
	isolateDir(t, dir)
	path := writeFixture(t, dir, "doc.md", staleTOCFile)

	out, stderr, code := runBinary(t, "", "export", "--fix", path)
	require.Equal(t, 0, code, "export --fix should succeed: %s", stderr)
	exported := writeFixture(t, dir, "exported.md", out)

	// The exported file should pass `mdsmith check`.
	_, stderr, code = runBinary(t, "", "check", exported)
	require.Equal(t, 0, code,
		"exported file should pass `mdsmith check`, but got exit %d: %s",
		code, stderr)

	// And exporting it again should be a clean no-op.
	out2, _, code := runBinary(t, "", "export", exported)
	require.Equal(t, 0, code)
	assert.Equal(t, out, out2,
		"idempotence: a second export should produce identical bytes")
}

func TestE2E_Export_IncludeDirective_BodyInlined(t *testing.T) {
	// Include resolution joins path.Dir(filePath) with the directive's
	// `file:` value; with RootFS set, the joined path is consulted on
	// the project-root DirFS, so the run has to happen with the file's
	// directory as CWD (and as the config root) for "snippet.md" to
	// resolve.
	dir := t.TempDir()
	isolateDir(t, dir)
	writeFixture(t, dir, "snippet.md", "snippet body line one\nsnippet body line two\n")
	src := `# Title

<?include
file: snippet.md
?>
snippet body line one
snippet body line two
<?/include?>

After.
`
	writeFixture(t, dir, "doc.md", src)

	stdout, stderr, code := runBinaryInDir(t, dir, "", "export", "doc.md")
	require.Equal(t, 0, code, "expected exit 0, got %d (stderr=%s)", code, stderr)
	assert.NotContains(t, stdout, "<?include")
	assert.NotContains(t, stdout, "<?/include")
	assert.Contains(t, stdout, "snippet body line one")
	assert.Contains(t, stdout, "snippet body line two")
	assert.Contains(t, stdout, "After.")
}

func TestE2E_Export_MarkerlessRequire_RemovedFromFrontmatterDoc(t *testing.T) {
	dir := t.TempDir()
	isolateDir(t, dir)
	src := `---
id: 1
title: Plan
---
<?require
filename: "[0-9]*.md"
?>

# Plan

Body.
`
	path := writeFixture(t, dir, "doc.md", src)
	stdout, stderr, code := runBinary(t, "", "export", path)
	require.Equal(t, 0, code, "expected exit 0, got %d (stderr=%s)", code, stderr)
	assert.NotContains(t, stdout, "<?require")
	// Front matter is preserved.
	assert.True(t, strings.HasPrefix(stdout, "---\nid: 1\ntitle: Plan\n---\n"),
		"front matter should be preserved verbatim, got: %q", stdout)
	assert.Contains(t, stdout, "# Plan")
	assert.Contains(t, stdout, "Body.")
}
