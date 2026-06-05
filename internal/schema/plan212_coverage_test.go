package schema

// Branch-coverage tests for the error and edge paths left uncovered
// on this PR's footprint (internal/schema). Each test names the
// specific branch it drives. Grouped by source file.

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yuin/goldmark/ast"
	extast "github.com/yuin/goldmark/extension/ast"
)

// ---- parse_file.go ----

// TestParseFile_FrontmatterCommentOnlyYieldsNoConstraints drives the
// `len(raw) == 0` early return in parseFileFrontmatter: a front-matter
// body that is only a YAML comment unmarshals to an empty map, so no
// constraints are recorded and parsing still succeeds.
func TestParseFile_FrontmatterCommentOnlyYieldsNoConstraints(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "proto.md", "---\n# just a comment\n---\n# ?\n")
	sch, err := ParseFile(&FileReader{}, p)
	require.NoError(t, err)
	assert.Empty(t, sch.Frontmatter,
		"comment-only front matter must yield no constraints")
}

// TestParseFile_EmptyRequireDirectiveSkipped drives the `body == ""`
// continue in extractRequireFilename: a <?require?> directive with an
// empty body is skipped, leaving no filename constraint.
func TestParseFile_EmptyRequireDirectiveSkipped(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "proto.md", "<?require ?>\n\n# ?\n")
	sch, err := ParseFile(&FileReader{}, p)
	require.NoError(t, err)
	assert.Empty(t, sch.Filename,
		"an empty <?require?> body must not set a filename constraint")
}

// TestParseFile_IncludeWithMalformedRequireSurfaces drives the
// extractRequireFilename error branch inside expandInclude: an
// included fragment whose <?require?> body is invalid YAML surfaces
// the error from the include path (not the top-level parse).
func TestParseFile_IncludeWithMalformedRequireSurfaces(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "frag.md"),
		[]byte("<?require\nfilename: [unterminated\n?>\n\n## Tasks\n"), 0o644))
	p := writeFile(t, dir, "proto.md",
		"# ?\n\n<?include\nfile: frag.md\n?>\n")
	_, err := ParseFile(&FileReader{}, p)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid <?require?>")
}

// TestParseFile_NestedIncludeFilenamePropagatesUp drives the
// `fpFrag != "" && fp == ""` branch in expandInclude: proto includes
// mid.md (no require), mid.md includes leaf.md (which carries a
// <?require filename?>). The leaf's filename bubbles up through the
// intermediate include that has no filename of its own.
func TestParseFile_NestedIncludeFilenamePropagatesUp(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "leaf.md"),
		[]byte("<?require\nfilename: \"leaf-*.md\"\n?>\n\n### Detail\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "mid.md"),
		[]byte("## Mid\n\n<?include\nfile: leaf.md\n?>\n"), 0o644))
	p := writeFile(t, dir, "proto.md",
		"# ?\n\n<?include\nfile: mid.md\n?>\n")
	sch, err := ParseFile(&FileReader{}, p)
	require.NoError(t, err)
	assert.Equal(t, "leaf-*.md", sch.Filename,
		"leaf require filename should propagate up through mid's include")
}

// ---- validate.go ----

// TestValidateFrontmatter_JSONMarshalFailureReported drives the
// json.Marshal error branch in ValidateFrontmatter: a front-matter
// value json cannot encode (a channel) surfaces the "serialize front
// matter" error rather than panicking.
func TestValidateFrontmatter_JSONMarshalFailureReported(t *testing.T) {
	sch := &Schema{Frontmatter: map[string]string{"id": "string"}}
	err := ValidateFrontmatter(sch, map[string]any{"id": make(chan int)})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "serialize front matter")
}

// TestValidateFrontmatter_ConcreteViolationReturnsError drives the
// merged.Validate error branch in ValidateFrontmatter: a document
// value that conflicts with the schema constraint (string where int
// is required) fails CUE's concrete validation.
func TestValidateFrontmatter_ConcreteViolationReturnsError(t *testing.T) {
	sch := &Schema{Frontmatter: map[string]string{"id": "int"}}
	err := ValidateFrontmatter(sch, map[string]any{"id": "not-an-int"})
	require.Error(t, err)
}

// ---- validate_content.go ----

// TestInferBlockLine_Cases drives both uncovered returns in
// inferBlockLine: the next-sibling-with-line==1 path (returns 1, not
// line-1) and the final fallback when no sibling has a known line.
func TestInferBlockLine_Cases(t *testing.T) {
	// Next sibling with line == 1 → returns 1 (the line>1 guard is false).
	assert.Equal(t, 1, inferBlockLine([]contentBlock{{line: 0}, {line: 1}}, 0))
	// Next sibling with line > 1 → returns line-1.
	assert.Equal(t, 4, inferBlockLine([]contentBlock{{line: 0}, {line: 5}}, 0))
	// No known sibling after, one before → previous line + 1.
	assert.Equal(t, 4, inferBlockLine([]contentBlock{{line: 3}, {line: 0}}, 1))
	// No known sibling at all → fallback 1.
	assert.Equal(t, 1, inferBlockLine([]contentBlock{{line: 0}}, 0))
}

// TestFirstContentHeadingLine_SkipsOutOfWindowHeadings drives the
// out-of-window `continue` in firstContentHeadingLine: headings before
// parentStart or at/after parentEnd are skipped before the first
// in-window heading at the expected level is returned.
func TestFirstContentHeadingLine_SkipsOutOfWindowHeadings(t *testing.T) {
	heads := []DocHeading{
		{Level: 2, Line: 1},  // before parentStart → skipped
		{Level: 2, Line: 50}, // at/after parentEnd → skipped
		{Level: 2, Line: 10}, // in window, right level → returned
	}
	assert.Equal(t, 10, firstContentHeadingLine(heads, 2, 5, 40))
	// No matching heading in the window → returns parentEnd.
	assert.Equal(t, 40,
		firstContentHeadingLine([]DocHeading{{Level: 3, Line: 10}}, 2, 5, 40))
}

// TestNodeMatchesKind_KnownAndUnknownKinds drives the default
// fall-through `return false` for an unknown kind, alongside a known
// positive match so the contract is pinned both ways.
func TestNodeMatchesKind_KnownAndUnknownKinds(t *testing.T) {
	assert.True(t, nodeMatchesKind(ContentKindParagraph, &ast.Paragraph{}))
	assert.False(t, nodeMatchesKind("not-a-real-kind", &ast.Paragraph{}),
		"an unknown kind name must fall through to return false")
}

// TestTableHeaderColumns_EmptyTableReturnsNil drives the
// `header == nil` guard: a table with no header row yields nil.
func TestTableHeaderColumns_EmptyTableReturnsNil(t *testing.T) {
	assert.Nil(t, tableHeaderColumns(&extast.Table{}, nil))
}

// TestTableHeaderColumns_SkipsNonCellChildren drives the non-TableCell
// `continue`: a header row whose only child is not a *extast.TableCell
// is ignored, so no columns are extracted.
func TestTableHeaderColumns_SkipsNonCellChildren(t *testing.T) {
	tbl := &extast.Table{}
	header := &extast.TableHeader{}
	tbl.AppendChild(tbl, header)
	header.AppendChild(header, ast.NewParagraph()) // not a TableCell
	assert.Empty(t, tableHeaderColumns(tbl, []byte("")),
		"non-TableCell header children must be skipped")
}

// ---- index.go ----

// TestRejectSymlinkTarget_StatErrorSurfaces drives the non-NotExist
// Lstat error branch: a target path whose parent component is a
// regular file makes os.Lstat return ENOTDIR (not IsNotExist), which
// must surface as a "stat target" error.
func TestRejectSymlinkTarget_StatErrorSurfaces(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "file")
	require.NoError(t, os.WriteFile(file, []byte("x"), 0o644))
	// "file/under" — "file" is not a directory, so Lstat fails ENOTDIR.
	err := rejectSymlinkTarget(filepath.Join(file, "under"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "stat target")
}

// TestWriteIndex_SurfacesAtomicRenameFailure drives the
// atomicWriteIndex error branch in WriteIndex (and the rename failure
// inside writeAndRename): when the resolved output path is an existing
// directory, it is not a symlink (so rejectSymlinkTarget passes) but
// renaming the sibling temp file onto a directory fails. WriteIndex
// must surface and cache that error.
func TestWriteIndex_SurfacesAtomicRenameFailure(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "doc.md")
	require.NoError(t, os.WriteFile(path, []byte("# T\n"), 0o644))
	// Pre-create the resolved output path as a directory.
	require.NoError(t, os.Mkdir(filepath.Join(root, "out.json"), 0o755))
	f, err := lint.NewFile(path, []byte("# T\n"))
	require.NoError(t, err)
	f.RootDir = root
	sch := &Schema{Source: "test", RootLevel: 2, Index: &IndexSpec{
		Output:  "out.json",
		Include: []string{IndexIncludeHeadingsFlat},
	}}
	err = WriteIndex(f, sch)
	require.Error(t, err)
	// The cached error surfaces on the next Check.
	diags := ValidateIndex(f, sch, makeDiagForTest)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "write failed")
}

// TestWriteAndRename_WriteErrorSurfaces drives the tmp.Write error
// branch in writeAndRename: handing it an already-closed file makes
// the write fail, which the helper must return after attempting
// cleanup.
func TestWriteAndRename_WriteErrorSurfaces(t *testing.T) {
	dir := t.TempDir()
	tmp, err := os.CreateTemp(dir, "x-*.tmp")
	require.NoError(t, err)
	tmpPath := tmp.Name()
	require.NoError(t, tmp.Close()) // close so the subsequent Write fails
	err = writeAndRename(tmp, tmpPath, filepath.Join(dir, "out.json"), []byte("data"))
	require.Error(t, err)
}

// TestWriteAndRename_ChmodErrorSurfaces drives the os.Chmod error
// branch in writeAndRename. We hand it a tmpPath that does not exist
// while tmp is a live handle: Write and Close succeed against the real
// open file, then os.Chmod(tmpPath) fails — simulating a chmod failure
// after a successful write so the helper's error handling is pinned.
func TestWriteAndRename_ChmodErrorSurfaces(t *testing.T) {
	dir := t.TempDir()
	tmp, err := os.CreateTemp(dir, "x-*.tmp")
	require.NoError(t, err)
	err = writeAndRename(tmp, filepath.Join(dir, "absent.tmp"),
		filepath.Join(dir, "out.json"), []byte("data"))
	require.Error(t, err)
}
