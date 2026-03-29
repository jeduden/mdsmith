package main_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// --- Query subcommand tests ---

func TestE2E_Query_Match_ExitsZero(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "a.md", "---\nstatus: \"✅\"\nid: 1\n---\n# Done\n\nContent here.\n")
	writeFixture(t, dir, "b.md", "---\nstatus: \"🔲\"\nid: 2\n---\n# Todo\n\nContent here.\n")

	stdout, _, exitCode := runBinaryInDir(t, dir, "", "query", `status: "✅"`, dir)
	assert.Equal(t, 0, exitCode, "expected exit 0 on match")
	assert.Contains(t, stdout, "a.md")
	assert.NotContains(t, stdout, "b.md")
}

func TestE2E_Query_NoMatch_ExitsOne(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "a.md", "---\nstatus: \"🔲\"\n---\n# Todo\n\nContent here.\n")

	_, _, exitCode := runBinaryInDir(t, dir, "", "query", `status: "✅"`, dir)
	assert.Equal(t, 1, exitCode, "expected exit 1 when no files match")
}

func TestE2E_Query_InvalidCUE_ExitsTwo(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "a.md", "---\nstatus: \"✅\"\n---\n# Title\n\nContent here.\n")

	_, stderr, exitCode := runBinaryInDir(t, dir, "", "query", `status: [[[`, dir)
	assert.Equal(t, 2, exitCode, "expected exit 2 for invalid CUE")
	assert.Contains(t, stderr, "invalid CUE expression")
}

func TestE2E_Query_NulDelimiter(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "a.md", "---\nstatus: \"✅\"\n---\n# Done\n\nContent here.\n")
	writeFixture(t, dir, "b.md", "---\nstatus: \"✅\"\n---\n# Also done\n\nContent here.\n")

	stdout, _, exitCode := runBinaryInDir(t, dir, "", "query", "-0", `status: "✅"`, dir)
	assert.Equal(t, 0, exitCode)
	// NUL-delimited: split by \x00
	parts := strings.Split(stdout, "\x00")
	// Last element is empty (trailing NUL)
	matched := 0
	for _, p := range parts {
		if strings.TrimSpace(p) != "" {
			matched++
		}
	}
	assert.Equal(t, 2, matched, "expected 2 NUL-delimited paths")
	assert.NotContains(t, stdout, "\n", "NUL mode should not contain newlines")
}

func TestE2E_Query_NoFrontMatter_Skipped(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "no-fm.md", "# No front matter\n\nJust content here.\n")

	_, _, exitCode := runBinaryInDir(t, dir, "", "query", `status: "✅"`, dir)
	assert.Equal(t, 1, exitCode, "expected exit 1 when no files have front matter")
}

func TestE2E_Query_ProtoSkipped(t *testing.T) {
	dir := t.TempDir()
	// Proto file has schema strings, not concrete values.
	writeFixture(t, dir, "proto.md", "---\nstatus: '\"🔲\" | \"🔳\" | \"✅\"'\n---\n# Proto\n\nTemplate.\n")

	_, _, exitCode := runBinaryInDir(t, dir, "", "query", `status: "✅"`, dir)
	assert.Equal(t, 1, exitCode, "proto template should not match")
}

func TestE2E_Query_NoArgs_ExitsTwo(t *testing.T) {
	_, stderr, exitCode := runBinary(t, "", "query")
	assert.Equal(t, 2, exitCode, "expected exit 2 with no args")
	assert.Contains(t, stderr, "requires a CUE expression")
}

func TestE2E_Query_Verbose_ShowsSkipped(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "no-fm.md", "# No front matter\n\nJust content.\n")

	_, stderr, exitCode := runBinaryInDir(t, dir, "", "query", "-v", `status: "✅"`, dir)
	assert.Equal(t, 1, exitCode)
	assert.Contains(t, stderr, "no front matter")
}

func TestE2E_Query_CompoundExpression(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "a.md", "---\nstatus: \"✅\"\nid: 60\n---\n# High ID\n\nContent here.\n")
	writeFixture(t, dir, "b.md", "---\nstatus: \"✅\"\nid: 30\n---\n# Low ID\n\nContent here.\n")

	stdout, _, exitCode := runBinaryInDir(t, dir, "", "query", `status: "✅", id: >50`, dir)
	assert.Equal(t, 0, exitCode)
	assert.Contains(t, stdout, "a.md")
	assert.NotContains(t, stdout, "b.md")
}

func TestE2E_Query_MultipleFiles_PrintsOnePath(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "match.md", "---\nstatus: \"✅\"\n---\n# Match\n\nContent here.\n")

	stdout, _, exitCode := runBinaryInDir(t, dir, "", "query", `status: "✅"`, filepath.Join(dir, "match.md"))
	assert.Equal(t, 0, exitCode)
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	assert.Len(t, lines, 1, "expected exactly one matching path")
}

func TestE2E_Query_EmptyFrontMatter_Skipped(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "empty-fm.md", "---\n---\n# Empty front matter\n\nContent here.\n")

	_, _, exitCode := runBinaryInDir(t, dir, "", "query", `status: "✅"`, dir)
	assert.Equal(t, 1, exitCode, "empty front matter should not match")
}

func TestE2E_Query_NoFileArgs_DefaultsCwd(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "a.md", "---\nstatus: \"✅\"\n---\n# Done\n\nContent here.\n")

	stdout, _, exitCode := runBinaryInDir(t, dir, "", "query", `status: "✅"`)
	assert.Equal(t, 0, exitCode, "should discover files from cwd")
	assert.Contains(t, stdout, "a.md")
}

func TestE2E_Query_HelpFlag(t *testing.T) {
	_, stderr, exitCode := runBinary(t, "", "query", "--help")
	assert.Equal(t, 2, exitCode)
	assert.Contains(t, stderr, "Usage: mdsmith query")
}
