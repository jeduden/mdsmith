package schema

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParseFile_ExtendsInheritsParentFrontmatter is the happy path:
// a proto.md schema that declares `extends: parent.md` picks up the
// parent's frontmatter constraints.
func TestParseFile_ExtendsInheritsParentFrontmatter(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "base.md"),
		[]byte("---\nid: 'string'\n---\n# ?\n"), 0o644))
	child := writeFile(t, dir, "child.md",
		"---\nextends: base.md\nstatus: '\"ratified\"'\n---\n# ?\n")
	sch, err := ParseFile(&FileReader{}, child)
	require.NoError(t, err)
	assert.Contains(t, sch.Frontmatter, "id", "parent frontmatter must flow through")
	assert.Contains(t, sch.Frontmatter, "status", "child frontmatter survives")
}

// TestParseFile_ExtendsStripsReservedKey verifies the `extends:`
// key never appears in the parsed schema's frontmatter; it is a
// schema-engine directive, not a frontmatter constraint.
func TestParseFile_ExtendsStripsReservedKey(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "base.md"),
		[]byte("---\nid: 'string'\n---\n# ?\n"), 0o644))
	child := writeFile(t, dir, "child.md",
		"---\nextends: base.md\n---\n# ?\n")
	sch, err := ParseFile(&FileReader{}, child)
	require.NoError(t, err)
	assert.NotContains(t, sch.Frontmatter, "extends",
		"extends is a schema directive, not a frontmatter constraint")
}

// TestParseFile_ExtendsChildSectionsReplaceParent locks down the
// plan-135 acceptance criterion: child sections wholly replace
// parent's; the parent's headings must not appear in the
// effective schema.
func TestParseFile_ExtendsChildSectionsReplaceParent(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "base.md"),
		[]byte("# ?\n\n## Context\n\n## Decision\n"), 0o644))
	child := writeFile(t, dir, "child.md",
		"---\nextends: base.md\n---\n# ?\n\n## Summary\n")
	sch, err := ParseFile(&FileReader{}, child)
	require.NoError(t, err)
	require.Len(t, sch.Sections, 1)
	require.Len(t, sch.Sections[0].Sections, 1,
		"child sections must wholly replace parent's, not merge")
	assert.Equal(t, "Summary", sch.Sections[0].Sections[0].Heading)
}

// TestParseFile_ExtendsParentSectionsFlowThrough verifies the
// inverse: a child that declares no sections inherits the parent's
// section tree verbatim.
func TestParseFile_ExtendsParentSectionsFlowThrough(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "base.md"),
		[]byte("# ?\n\n## Context\n\n## Decision\n"), 0o644))
	child := writeFile(t, dir, "child.md",
		"---\nextends: base.md\nstatus: 'string'\n---\n")
	sch, err := ParseFile(&FileReader{}, child)
	require.NoError(t, err)
	require.Len(t, sch.Sections, 1)
	children := sch.Sections[0].Sections
	require.Len(t, children, 2)
	assert.Equal(t, "Context", children[0].Heading)
	assert.Equal(t, "Decision", children[1].Heading)
}

// TestParseFile_ExtendsFrontmatterUnifies exercises the CUE
// refinement path: child narrows a parent disjunction; the unified
// expression joins with `&`.
func TestParseFile_ExtendsFrontmatterUnifies(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "base.md"),
		[]byte("---\nstatus: '\"open\" | \"closed\" | \"ratified\"'\n---\n# ?\n"),
		0o644))
	child := writeFile(t, dir, "child.md",
		"---\nextends: base.md\nstatus: '\"ratified\"'\n---\n# ?\n")
	sch, err := ParseFile(&FileReader{}, child)
	require.NoError(t, err)
	assert.Contains(t, sch.Frontmatter["status"], "&")
	assert.Contains(t, sch.Frontmatter["status"], "ratified")
}

// TestParseFile_ExtendsFrontmatterConflictReportsBothLayers is the
// conflict diagnostic acceptance criterion: a child whose
// expression cannot unify with the parent's surfaces an
// UnsatisfiableKeyError naming both schema layers.
func TestParseFile_ExtendsFrontmatterConflictReportsBothLayers(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "base.md"),
		[]byte("---\nstatus: 'int'\n---\n# ?\n"), 0o644))
	child := writeFile(t, dir, "child.md",
		"---\nextends: base.md\nstatus: 'string'\n---\n# ?\n")
	_, err := ParseFile(&FileReader{}, child)
	require.Error(t, err)
	var keyErr *UnsatisfiableKeyError
	require.True(t, errors.As(err, &keyErr))
	assert.Equal(t, "status", keyErr.Key)
}

// TestParseFile_ExtendsDetectsCycle covers cycle detection: a → b → a
// must surface as an extends-cycle error naming the cycle path.
func TestParseFile_ExtendsDetectsCycle(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.md"),
		[]byte("---\nextends: b.md\n---\n# ?\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.md"),
		[]byte("---\nextends: a.md\n---\n# ?\n"), 0o644))
	_, err := ParseFile(&FileReader{}, filepath.Join(dir, "a.md"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "extends cycle")
}

// TestParseFile_ExtendsRejectsAbsolutePath enforces the same path
// safety as `<?include?>`: absolute parent paths are rejected.
func TestParseFile_ExtendsRejectsAbsolutePath(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "child.md",
		"---\nextends: /etc/passwd\n---\n# ?\n")
	_, err := ParseFile(&FileReader{}, p)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "absolute extends path")
}

// TestParseFile_ExtendsRejectsTraversal enforces the no-`..`-traversal
// rule shared with `<?include?>`.
func TestParseFile_ExtendsRejectsTraversal(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "child.md",
		"---\nextends: ../leak.md\n---\n# ?\n")
	_, err := ParseFile(&FileReader{}, p)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `".."`)
}

// TestParseFile_ExtendsRejectsNonStringValue ensures a typoed YAML
// shape (number, mapping) is rejected with a clear error rather
// than silently dropped.
func TestParseFile_ExtendsRejectsNonStringValue(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "child.md",
		"---\nextends: 42\n---\n# ?\n")
	_, err := ParseFile(&FileReader{}, p)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "extends")
}

// TestParseFile_ExtendsRejectsWhitespaceOnly catches the common
// `extends: " "` typo: an empty-after-trim value would otherwise
// be silently treated as "no extends", masking a configuration
// mistake.
func TestParseFile_ExtendsRejectsWhitespaceOnly(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "child.md",
		"---\nextends: \"   \"\n---\n# ?\n")
	_, err := ParseFile(&FileReader{}, p)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "extends")
	assert.Contains(t, err.Error(), "non-empty")
}

// TestParseFile_ExtendsMissingParentReturnsError covers the file
// I/O failure path: a typoed parent name surfaces as a clear error.
func TestParseFile_ExtendsMissingParentReturnsError(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "child.md",
		"---\nextends: missing.md\n---\n# ?\n")
	_, err := ParseFile(&FileReader{}, p)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "extends parent")
}

// TestParseFile_ExtendsMultiHopChain walks more than one level of
// inheritance to confirm the recursion terminates correctly.
func TestParseFile_ExtendsMultiHopChain(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.md"),
		[]byte("---\na: 'string'\n---\n# ?\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.md"),
		[]byte("---\nextends: a.md\nb: 'string'\n---\n# ?\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "c.md"),
		[]byte("---\nextends: b.md\nc: 'string'\n---\n# ?\n"), 0o644))
	sch, err := ParseFile(&FileReader{}, filepath.Join(dir, "c.md"))
	require.NoError(t, err)
	assert.Contains(t, sch.Frontmatter, "a")
	assert.Contains(t, sch.Frontmatter, "b")
	assert.Contains(t, sch.Frontmatter, "c")
}

func TestExtractExtendsKey_PresentString(t *testing.T) {
	raw := map[string]any{"extends": "base.md", "other": "x"}
	out, err := extractExtendsKey(raw)
	require.NoError(t, err)
	assert.Equal(t, "base.md", out)
	assert.NotContains(t, raw, "extends",
		"extracted key must be removed so it never lands in Frontmatter")
}

func TestExtractExtendsKey_Absent(t *testing.T) {
	raw := map[string]any{"other": "x"}
	out, err := extractExtendsKey(raw)
	require.NoError(t, err)
	assert.Empty(t, out)
}

func TestExtractExtendsKey_NilValue(t *testing.T) {
	raw := map[string]any{"extends": nil}
	out, err := extractExtendsKey(raw)
	require.NoError(t, err)
	assert.Empty(t, out)
}

func TestExtractExtendsKey_RejectsNonString(t *testing.T) {
	raw := map[string]any{"extends": 42}
	_, err := extractExtendsKey(raw)
	require.Error(t, err)
}

func TestResolveExtendsPath_RelativeWithinDir(t *testing.T) {
	out, err := resolveExtendsPath("/work/schemas/child.md", "base.md")
	require.NoError(t, err)
	assert.Equal(t, filepath.Clean("/work/schemas/base.md"), out)
}

func TestResolveExtendsPath_RejectsAbsolute(t *testing.T) {
	_, err := resolveExtendsPath("/work/schemas/child.md", "/etc/passwd")
	require.Error(t, err)
}

func TestResolveExtendsPath_RejectsTraversal(t *testing.T) {
	_, err := resolveExtendsPath("/work/schemas/child.md", "../leak.md")
	require.Error(t, err)
}

func TestResolveExtendsPath_RejectsEmpty(t *testing.T) {
	_, err := resolveExtendsPath("/work/schemas/child.md", "")
	require.Error(t, err)
}
