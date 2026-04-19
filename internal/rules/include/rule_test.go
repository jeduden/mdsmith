package include

import (
	"fmt"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/jeduden/mdsmith/internal/lint"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestFile(
	t *testing.T, path, source string, fs ...fstest.MapFS,
) *lint.File {
	t.Helper()
	f, err := lint.NewFile(path, []byte(source))
	require.NoError(t, err)
	if len(fs) > 0 {
		f.FS = fs[0]
		f.RootFS = fs[0]
	}
	return f
}

func expectDiags(
	t *testing.T, diags []lint.Diagnostic, count int,
) {
	t.Helper()
	if len(diags) != count {
		msgs := make([]string, len(diags))
		for i, d := range diags {
			msgs[i] = d.Message
		}
		t.Fatalf(
			"expected %d diagnostic(s), got %d: %v",
			count, len(diags), msgs,
		)
	}
}

func expectDiagMsg(
	t *testing.T, diags []lint.Diagnostic, msg string,
) {
	t.Helper()
	if len(diags) == 0 {
		t.Fatalf(
			"expected diagnostic with message %q, got none", msg,
		)
	}
	for _, d := range diags {
		if strings.Contains(d.Message, msg) {
			return
		}
	}
	msgs := make([]string, len(diags))
	for i, d := range diags {
		msgs[i] = d.Message
	}
	t.Errorf(
		"no diagnostic contains %q, got: %v", msg, msgs,
	)
}

// =====================================================================
// Rule metadata
// =====================================================================

func TestRule_ID(t *testing.T) {
	r := &Rule{}
	if r.ID() != "MDS021" {
		t.Errorf("expected ID MDS021, got %s", r.ID())
	}
}

func TestRule_Name(t *testing.T) {
	r := &Rule{}
	if r.Name() != "include" {
		t.Errorf("expected Name include, got %s", r.Name())
	}
}

// =====================================================================
// Basic include
// =====================================================================

func TestCheck_IncludeUpToDate(t *testing.T) {
	fsys := fstest.MapFS{
		"data.md": {Data: []byte("Hello world\n")},
	}
	src := "# Doc\n\n<?include\nfile: data.md\n?>\nHello world\n<?/include?>\n"
	f := newTestFile(t, "doc.md", src, fsys)
	r := &Rule{}
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

func TestCheck_IncludeOutOfDate(t *testing.T) {
	fsys := fstest.MapFS{
		"data.md": {Data: []byte("Updated content\n")},
	}
	src := "# Doc\n\n<?include\nfile: data.md\n?>\nOld content\n<?/include?>\n"
	f := newTestFile(t, "doc.md", src, fsys)
	r := &Rule{}
	diags := r.Check(f)
	expectDiagMsg(t, diags, "generated section is out of date")
}

// =====================================================================
// Strip frontmatter
// =====================================================================

func TestCheck_StripFrontmatterDefault(t *testing.T) {
	fsys := fstest.MapFS{
		"data.md": {
			Data: []byte(
				"---\nid: MDS001\n---\nHello world\n",
			),
		},
	}
	src := "# Doc\n\n<?include\nfile: data.md\n?>\nHello world\n<?/include?>\n"
	f := newTestFile(t, "doc.md", src, fsys)
	r := &Rule{}
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

func TestCheck_StripFrontmatterOff(t *testing.T) {
	fsys := fstest.MapFS{
		"data.md": {
			Data: []byte(
				"---\nid: MDS001\n---\nHello world\n",
			),
		},
	}
	src := "# Doc\n\n<?include\nfile: data.md\n" +
		"strip-frontmatter: \"false\"\n?>\n" +
		"---\nid: MDS001\n---\nHello world\n<?/include?>\n"
	f := newTestFile(t, "doc.md", src, fsys)
	r := &Rule{}
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

// =====================================================================
// Wrap in code fence
// =====================================================================

func TestCheck_WrapInCodeFence(t *testing.T) {
	fsys := fstest.MapFS{
		"example.md": {Data: []byte("# Hello\n")},
	}
	src := "# Doc\n\n<?include\nfile: example.md\n" +
		"wrap: markdown\n?>\n\n```markdown\n# Hello\n```\n\n" +
		"<?/include?>\n"
	f := newTestFile(t, "doc.md", src, fsys)
	r := &Rule{}
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

func TestCheck_WrapWithBacktickContent(t *testing.T) {
	// Fixture contains triple-backtick fences; wrap must use a longer
	// fence (e.g., ````) so the code block is not prematurely closed.
	fsys := fstest.MapFS{
		"example.md": {Data: []byte("# Title\n\n```go\nfmt.Println(\"hi\")\n```\n")},
	}
	src := "# Doc\n\n<?include\nfile: example.md\n" +
		"wrap: markdown\n?>\n\n````markdown\n# Title\n\n```go\n" +
		"fmt.Println(\"hi\")\n```\n````\n\n<?/include?>\n"
	f := newTestFile(t, "doc.md", src, fsys)
	r := &Rule{}
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

func TestFix_WrapWithBacktickContent(t *testing.T) {
	// Fix should produce a fence longer than any backtick run in the content.
	fsys := fstest.MapFS{
		"example.md": {Data: []byte("# Title\n\n```go\nfmt.Println(\"hi\")\n```\n")},
	}
	src := "# Doc\n\n<?include\nfile: example.md\n" +
		"wrap: markdown\n?>\nold\n<?/include?>\n"
	f := newTestFile(t, "doc.md", src, fsys)
	r := &Rule{}
	got := string(r.Fix(f))
	// Must use ```` (4 backticks) since content has ``` (3 backticks).
	want := "# Doc\n\n<?include\nfile: example.md\n" +
		"wrap: markdown\n?>\n\n````markdown\n# Title\n\n```go\n" +
		"fmt.Println(\"hi\")\n```\n````\n\n<?/include?>\n"
	assert.Equal(t, want, got, "Fix output mismatch\ngot:\n%s\nwant:\n%s", got, want)
}

// =====================================================================
// Missing file
// =====================================================================

func TestCheck_MissingFile(t *testing.T) {
	fsys := fstest.MapFS{}
	src := "# Doc\n\n<?include\nfile: missing.md\n?>\nold\n<?/include?>\n"
	f := newTestFile(t, "doc.md", src, fsys)
	r := &Rule{}
	diags := r.Check(f)
	expectDiagMsg(t, diags, "cannot read include file")
}

// =====================================================================
// Validation errors
// =====================================================================

func TestCheck_MissingFileParam(t *testing.T) {
	fsys := fstest.MapFS{}
	src := "# Doc\n\n<?include\n?>\nold\n<?/include?>\n"
	f := newTestFile(t, "doc.md", src, fsys)
	r := &Rule{}
	diags := r.Check(f)
	expectDiagMsg(t, diags, `missing required "file" parameter`)
}

func TestCheck_AbsoluteFilePath(t *testing.T) {
	fsys := fstest.MapFS{}
	src := "# Doc\n\n<?include\nfile: /etc/passwd\n?>\nold\n<?/include?>\n"
	f := newTestFile(t, "doc.md", src, fsys)
	r := &Rule{}
	diags := r.Check(f)
	expectDiagMsg(t, diags, "absolute file path")
}

func TestCheck_DotDotPathResolvesWithRootFS(t *testing.T) {
	// file: ../CLAUDE.md from .github/copilot.md resolves
	// to CLAUDE.md at the project root via RootFS.
	fsys := fstest.MapFS{
		"CLAUDE.md": {Data: []byte("Project info.\n")},
	}
	src := "# Doc\n\n<?include\nfile: ../CLAUDE.md\n?>\n" +
		"Project info.\n<?/include?>\n"
	f := newTestFile(t, ".github/copilot.md", src, fsys)
	r := &Rule{}
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

func TestCheck_DoubleDotInFilenameNotRejected(t *testing.T) {
	// A filename like "foo..bar.md" should not be treated as path traversal.
	fsys := fstest.MapFS{
		"foo..bar.md": {Data: []byte("Content\n")},
	}
	src := "# Doc\n\n<?include\nfile: foo..bar.md\n?>\nContent\n<?/include?>\n"
	f := newTestFile(t, "doc.md", src, fsys)
	r := &Rule{}
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

func TestCheck_DotDotEscapesProjectRoot(t *testing.T) {
	// When the resolved path escapes the project root, a diagnostic
	// should be emitted.
	fsys := fstest.MapFS{}
	src := "# Doc\n\n<?include\nfile: ../../escape.md\n?>\nold\n<?/include?>\n"
	f := newTestFile(t, "doc.md", src, fsys)
	r := &Rule{}
	diags := r.Check(f)
	expectDiagMsg(t, diags, "escapes project root")
}

func TestCheck_DotDotPathWithoutRootFS(t *testing.T) {
	// When RootFS is nil and file contains "..", a clear diagnostic
	// should be emitted instead of a confusing "not found" error.
	src := "# Doc\n\n<?include\nfile: ../CLAUDE.md\n?>\nold\n<?/include?>\n"
	f, err := lint.NewFile("sub/doc.md", []byte(src))
	require.NoError(t, err)
	f.FS = fstest.MapFS{}
	// RootFS intentionally left nil.
	r := &Rule{}
	diags := r.Check(f)
	expectDiagMsg(t, diags, "project root is not configured")
}

// =====================================================================
// Fix
// =====================================================================

func TestFix_UpdatesContent(t *testing.T) {
	fsys := fstest.MapFS{
		"data.md": {Data: []byte("New content\n")},
	}
	src := "# Doc\n\n<?include\nfile: data.md\n?>\nOld content\n<?/include?>\n"
	f := newTestFile(t, "doc.md", src, fsys)
	r := &Rule{}
	got := string(r.Fix(f))
	want := "# Doc\n\n<?include\nfile: data.md\n?>\nNew content\n<?/include?>\n"
	if got != want {
		t.Errorf(
			"Fix output mismatch\ngot:\n%s\nwant:\n%s", got, want,
		)
	}
}

// =====================================================================
// Link adjustment
// =====================================================================

func TestCheck_LinkAdjustmentSubdir(t *testing.T) {
	// file: sub/content.md from docs/guide.md → resolved to
	// docs/sub/content.md via RootFS. Link images/pic.png
	// rewrites to sub/images/pic.png from docs/guide.md.
	fsys := fstest.MapFS{
		"docs/sub/content.md": {
			Data: []byte("See [pic](images/pic.png) here.\n"),
		},
	}
	src := "# Guide\n\n<?include\nfile: sub/content.md\n?>\n" +
		"See [pic](sub/images/pic.png) here.\n<?/include?>\n"
	f := newTestFile(t, "docs/guide.md", src, fsys)
	r := &Rule{}
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

func TestCheck_LinkAdjustmentSameDir(t *testing.T) {
	// file: other.md in same dir as including file → no rewriting.
	fsys := fstest.MapFS{
		"other.md": {
			Data: []byte("See [link](foo.md) here.\n"),
		},
	}
	src := "# Doc\n\n<?include\nfile: other.md\n?>\n" +
		"See [link](foo.md) here.\n<?/include?>\n"
	f := newTestFile(t, "doc.md", src, fsys)
	r := &Rule{}
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

func TestFix_LinkAdjustment(t *testing.T) {
	// file: sub/content.md from docs/guide.md → resolved to
	// docs/sub/content.md via RootFS; links rewrite from sub/.
	fsys := fstest.MapFS{
		"docs/sub/content.md": {
			Data: []byte("See [layout](internal/rules/) for details.\n"),
		},
	}
	src := "# Guide\n\n<?include\nfile: sub/content.md\n?>\nold\n<?/include?>\n"
	f := newTestFile(t, "docs/guide.md", src, fsys)
	r := &Rule{}
	got := string(r.Fix(f))
	want := "# Guide\n\n<?include\nfile: sub/content.md\n?>\n" +
		"See [layout](sub/internal/rules/) for details.\n<?/include?>\n"
	assert.Equal(t, want, got, "Fix output mismatch\ngot:\n%s\nwant:\n%s", got, want)
}

// =====================================================================
// Heading level adjustment
// =====================================================================

func TestCheck_HeadingLevelAbsolute(t *testing.T) {
	fsys := fstest.MapFS{
		"data.md": {
			Data: []byte("## Build\n\nBuild steps.\n\n### Sub\n\nDetails.\n"),
		},
	}
	// Parent is ## Project (level 2), source has ## (level 2) and ###
	// shift = 2 - 2 + 1 = 1 → ### Build, #### Sub
	src := "# Doc\n\n## Project\n\n<?include\nfile: data.md\n" +
		"heading-level: \"absolute\"\n?>\n" +
		"### Build\n\nBuild steps.\n\n#### Sub\n\nDetails.\n<?/include?>\n"
	f := newTestFile(t, "doc.md", src, fsys)
	r := &Rule{}
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

func TestCheck_HeadingLevelOmitted(t *testing.T) {
	fsys := fstest.MapFS{
		"data.md": {
			Data: []byte("## Build\n\nSteps.\n"),
		},
	}
	// Without heading-level, headings stay unchanged.
	src := "# Doc\n\n## Project\n\n<?include\nfile: data.md\n?>\n" +
		"## Build\n\nSteps.\n<?/include?>\n"
	f := newTestFile(t, "doc.md", src, fsys)
	r := &Rule{}
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

func TestCheck_HeadingLevelAtDocRoot(t *testing.T) {
	fsys := fstest.MapFS{
		"data.md": {
			Data: []byte("## Build\n\nSteps.\n"),
		},
	}
	// No heading before marker → parent level 0 → no shift.
	src := "<?include\nfile: data.md\n" +
		"heading-level: \"absolute\"\n?>\n" +
		"## Build\n\nSteps.\n<?/include?>\n"
	f := newTestFile(t, "doc.md", src, fsys)
	r := &Rule{}
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

func TestCheck_HeadingLevelUnderH3(t *testing.T) {
	fsys := fstest.MapFS{
		"data.md": {
			Data: []byte("## Topic\n\nText.\n"),
		},
	}
	// Parent is ### Details (level 3), source h2
	// shift = 3 - 2 + 1 = 2 → #### Topic
	src := "# Doc\n\n## Section\n\n### Details\n\n<?include\nfile: data.md\n" +
		"heading-level: \"absolute\"\n?>\n" +
		"#### Topic\n\nText.\n<?/include?>\n"
	f := newTestFile(t, "doc.md", src, fsys)
	r := &Rule{}
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

func TestCheck_InvalidHeadingLevel(t *testing.T) {
	fsys := fstest.MapFS{
		"data.md": {Data: []byte("content\n")},
	}
	src := "# Doc\n\n<?include\nfile: data.md\n" +
		"heading-level: \"relative\"\n?>\ncontent\n<?/include?>\n"
	f := newTestFile(t, "doc.md", src, fsys)
	r := &Rule{}
	diags := r.Check(f)
	expectDiagMsg(t, diags, `"heading-level" must be "absolute"`)
}

func TestFix_HeadingLevelAbsolute(t *testing.T) {
	fsys := fstest.MapFS{
		"data.md": {
			Data: []byte("## Build\n\nSteps.\n\n### Sub\n\nMore.\n"),
		},
	}
	src := "# Doc\n\n## Project\n\n<?include\nfile: data.md\n" +
		"heading-level: \"absolute\"\n?>\nold\n<?/include?>\n"
	f := newTestFile(t, "doc.md", src, fsys)
	r := &Rule{}
	got := string(r.Fix(f))
	want := "# Doc\n\n## Project\n\n<?include\nfile: data.md\n" +
		"heading-level: \"absolute\"\n?>\n" +
		"### Build\n\nSteps.\n\n#### Sub\n\nMore.\n<?/include?>\n"
	assert.Equal(t, want, got, "Fix output mismatch\ngot:\n%s\nwant:\n%s", got, want)
}

// =====================================================================
// Combined link adjustment and heading level
// =====================================================================

func TestCheck_LinkAndHeadingCombined(t *testing.T) {
	// file: sub/dev.md from docs/guide.md → resolved to
	// docs/sub/dev.md. Heading shift + link rewrite both apply.
	fsys := fstest.MapFS{
		"docs/sub/dev.md": {
			Data: []byte("## Build\n\nSee [rules](internal/rules/).\n"),
		},
	}
	// Parent ## Project (level 2), shift=1 → ### Build
	// Link rewritten: internal/rules/ → sub/internal/rules/
	src := "# Doc\n\n## Project\n\n<?include\nfile: sub/dev.md\n" +
		"heading-level: \"absolute\"\n?>\n" +
		"### Build\n\nSee [rules](sub/internal/rules/).\n<?/include?>\n"
	f := newTestFile(t, "docs/guide.md", src, fsys)
	r := &Rule{}
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

// =====================================================================
// Cycle detection
// =====================================================================

func TestCheck_DirectCycle(t *testing.T) {
	// File includes itself.
	fsys := fstest.MapFS{
		"doc.md": {Data: []byte("# Doc\n\n<?include\nfile: doc.md\n?>\nold\n<?/include?>\n")},
	}
	src := "# Doc\n\n<?include\nfile: doc.md\n?>\nold\n<?/include?>\n"
	f := newTestFile(t, "doc.md", src, fsys)
	r := &Rule{}
	diags := r.Check(f)
	expectDiagMsg(t, diags, "cyclic include: doc.md -> doc.md")
}

func TestCheck_IndirectCycle(t *testing.T) {
	// A includes B, B includes A.
	fsys := fstest.MapFS{
		"b.md": {Data: []byte("# B\n\n<?include\nfile: a.md\n?>\nold\n<?/include?>\n")},
		"a.md": {Data: []byte("dummy\n")},
	}
	src := "# A\n\n<?include\nfile: b.md\n?>\nold\n<?/include?>\n"
	f := newTestFile(t, "a.md", src, fsys)
	r := &Rule{}
	diags := r.Check(f)
	expectDiagMsg(t, diags, "cyclic include: a.md -> b.md -> a.md")
}

func TestCheck_ThreeLevelCycle(t *testing.T) {
	// A includes B, B includes C, C includes A.
	fsys := fstest.MapFS{
		"b.md": {Data: []byte("# B\n\n<?include\nfile: c.md\n?>\nold\n<?/include?>\n")},
		"c.md": {Data: []byte("# C\n\n<?include\nfile: a.md\n?>\nold\n<?/include?>\n")},
		"a.md": {Data: []byte("dummy\n")},
	}
	src := "# A\n\n<?include\nfile: b.md\n?>\nold\n<?/include?>\n"
	f := newTestFile(t, "a.md", src, fsys)
	r := &Rule{}
	diags := r.Check(f)
	expectDiagMsg(t, diags, "cyclic include: a.md -> b.md -> c.md -> a.md")
}

func TestCheck_MaxDepthExceeded(t *testing.T) {
	// Build a chain of 12 files (exceeds maxIncludeDepth=10).
	fsys := fstest.MapFS{}
	for i := 1; i <= 12; i++ {
		next := fmt.Sprintf("f%d.md", i+1)
		if i == 12 {
			fsys[fmt.Sprintf("f%d.md", i)] = &fstest.MapFile{
				Data: []byte("leaf\n"),
			}
		} else {
			fsys[fmt.Sprintf("f%d.md", i)] = &fstest.MapFile{
				Data: []byte(fmt.Sprintf(
					"# F%d\n\n<?include\nfile: %s\n?>\nold\n<?/include?>\n", i, next)),
			}
		}
	}
	src := "# Root\n\n<?include\nfile: f1.md\n?>\nold\n<?/include?>\n"
	f := newTestFile(t, "root.md", src, fsys)
	r := &Rule{}
	diags := r.Check(f)
	expectDiagMsg(t, diags, "include depth exceeds maximum (10)")
}

func TestCheck_NoCycle(t *testing.T) {
	// A includes B, B includes C (no cycle). No errors expected.
	bContent := "# B\n\n<?include\nfile: c.md\n?>\n" +
		"Final content\n<?/include?>\n"
	fsys := fstest.MapFS{
		"b.md": {Data: []byte(bContent)},
		"c.md": {Data: []byte("Final content\n")},
	}
	src := "# A\n\n<?include\nfile: b.md\n?>\n" +
		"# B\n\n<?include\nfile: c.md\n?>\n" +
		"Final content\n<?/include?>\n<?/include?>\n"
	f := newTestFile(t, "a.md", src, fsys)
	r := &Rule{}
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

func TestFix_NestedInclude(t *testing.T) {
	// A includes B, B includes C. Fix should produce B's content
	// (which contains C's include markers) as-is.
	bContent := "# B\n\n<?include\nfile: c.md\n?>\n" +
		"Final content\n<?/include?>\n"
	fsys := fstest.MapFS{
		"b.md": {Data: []byte(bContent)},
		"c.md": {Data: []byte("Final content\n")},
	}
	src := "# A\n\n<?include\nfile: b.md\n?>\nold\n<?/include?>\n"
	f := newTestFile(t, "a.md", src, fsys)
	r := &Rule{}
	got := string(r.Fix(f))
	want := "# A\n\n<?include\nfile: b.md\n?>\n" +
		"# B\n\n<?include\nfile: c.md\n?>\n" +
		"Final content\n<?/include?>\n<?/include?>\n"
	assert.Equal(t, want, got,
		"Fix output mismatch\ngot:\n%s\nwant:\n%s", got, want)
}

func TestCheck_NestedIncludeUpToDate(t *testing.T) {
	// A includes B, B itself contains a catalog section.
	// The expanded content in A should be accepted without errors.
	bContent := "# B\n\n<?catalog\nglob: \"*.md\"\n?>\n" +
		"- item\n<?/catalog?>\n"
	fsys := fstest.MapFS{
		"b.md": {Data: []byte(bContent)},
	}
	src := "# A\n\n<?include\nfile: b.md\n?>\n" +
		"# B\n\n<?catalog\nglob: \"*.md\"\n?>\n" +
		"- item\n<?/catalog?>\n<?/include?>\n"
	f := newTestFile(t, "a.md", src, fsys)
	r := &Rule{}
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

// =====================================================================
// Recursive expansion
// =====================================================================

func TestFix_RecursiveExpansion(t *testing.T) {
	// A includes B, B includes C. B.md on disk has stale content for C.
	// Fix A should recursively expand B's includes to produce correct
	// content in a single pass.
	bContent := "# B\n\n<?include\nfile: c.md\n?>\n" +
		"stale\n<?/include?>\n"
	fsys := fstest.MapFS{
		"b.md": {Data: []byte(bContent)},
		"c.md": {Data: []byte("Fresh from C\n")},
	}
	src := "# A\n\n<?include\nfile: b.md\n?>\nold\n<?/include?>\n"
	f := newTestFile(t, "a.md", src, fsys)
	r := &Rule{}
	got := string(r.Fix(f))
	want := "# A\n\n<?include\nfile: b.md\n?>\n" +
		"# B\n\n<?include\nfile: c.md\n?>\n" +
		"Fresh from C\n<?/include?>\n<?/include?>\n"
	assert.Equal(t, want, got,
		"Fix output mismatch\ngot:\n%s\nwant:\n%s", got, want)
}

func TestCheck_RecursiveExpansion(t *testing.T) {
	// A includes B, B includes C. B.md on disk has stale C content, but
	// A has the correct recursively-expanded content. Check should pass.
	bContent := "# B\n\n<?include\nfile: c.md\n?>\n" +
		"stale\n<?/include?>\n"
	fsys := fstest.MapFS{
		"b.md": {Data: []byte(bContent)},
		"c.md": {Data: []byte("Fresh from C\n")},
	}
	src := "# A\n\n<?include\nfile: b.md\n?>\n" +
		"# B\n\n<?include\nfile: c.md\n?>\n" +
		"Fresh from C\n<?/include?>\n<?/include?>\n"
	f := newTestFile(t, "a.md", src, fsys)
	r := &Rule{}
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}

func TestFix_RecursiveExpansionWithFrontmatter(t *testing.T) {
	// B.md has frontmatter and a nested include. Recursive expansion
	// should preserve frontmatter when reconstructing B's content.
	bContent := "---\ntitle: B\n---\n# B\n\n<?include\n" +
		"file: c.md\n?>\nstale\n<?/include?>\n"
	fsys := fstest.MapFS{
		"b.md": {Data: []byte(bContent)},
		"c.md": {Data: []byte("Fresh from C\n")},
	}
	src := "# A\n\n<?include\nfile: b.md\n?>\nold\n<?/include?>\n"
	f := newTestFile(t, "a.md", src, fsys)
	r := &Rule{}
	got := string(r.Fix(f))
	// Frontmatter is stripped by default, so only body appears.
	want := "# A\n\n<?include\nfile: b.md\n?>\n" +
		"# B\n\n<?include\nfile: c.md\n?>\n" +
		"Fresh from C\n<?/include?>\n<?/include?>\n"
	assert.Equal(t, want, got,
		"Fix output mismatch\ngot:\n%s\nwant:\n%s", got, want)
}

func TestFix_RecursiveExpansionSubdir(t *testing.T) {
	// B is in a subdirectory and includes C relative to itself.
	// Recursive expansion must resolve C correctly via readFS,
	// not double the subdirectory path.
	fsys := fstest.MapFS{
		"sub/b.md": {Data: []byte(
			"# B\n\n<?include\nfile: c.md\n?>\n" +
				"stale\n<?/include?>\n")},
		"sub/c.md": {Data: []byte("Fresh from C\n")},
	}
	src := "# A\n\n<?include\nfile: sub/b.md\n?>\n" +
		"old\n<?/include?>\n"
	f := newTestFile(t, "a.md", src, fsys)
	r := &Rule{}
	got := string(r.Fix(f))
	want := "# A\n\n<?include\nfile: sub/b.md\n?>\n" +
		"# B\n\n<?include\nsource-dir: \"sub\"\nfile: c.md\n?>\n" +
		"Fresh from C\n<?/include?>\n<?/include?>\n"
	assert.Equal(t, want, got,
		"Fix output mismatch\ngot:\n%s\nwant:\n%s", got, want)
}

// =====================================================================
// source-dir injection for nested directives (#133)
// =====================================================================

func TestFix_InjectsSourceDirIntoCatalog(t *testing.T) {
	// When an included file contains a <?catalog?> directive, the
	// include expansion should inject source-dir so the catalog
	// resolves globs relative to the included file's directory.
	fsys := fstest.MapFS{
		"docs/dev/index.md": {Data: []byte(
			"<?catalog\nglob: \"*.md\"\n?>\n<?/catalog?>\n",
		)},
	}
	src := "<?include\nfile: docs/dev/index.md\n?>\nold\n<?/include?>\n"
	f := newTestFile(t, "README.md", src, fsys)
	r := &Rule{}
	result := string(r.Fix(f))

	assert.Contains(t, result, "source-dir:")
	assert.Contains(t, result, "docs/dev")
}

func TestFix_InjectsSourceDirPreservesExistingParams(t *testing.T) {
	// source-dir injection should not disturb other catalog parameters.
	fsys := fstest.MapFS{
		"sub/catalog.md": {Data: []byte(
			"<?catalog\nglob: \"*.md\"\nsort: path\n?>\n<?/catalog?>\n",
		)},
	}
	src := "<?include\nfile: sub/catalog.md\n?>\nold\n<?/include?>\n"
	f := newTestFile(t, "root.md", src, fsys)
	r := &Rule{}
	result := string(r.Fix(f))

	assert.Contains(t, result, "source-dir:")
	assert.Contains(t, result, "glob:")
	assert.Contains(t, result, "sort: path")
}

func TestFix_NoSourceDirWhenSameDir(t *testing.T) {
	// When the included file is in the same directory as the including
	// file, no source-dir is needed.
	fsys := fstest.MapFS{
		"catalog.md": {Data: []byte(
			"<?catalog\nglob: \"*.md\"\n?>\n<?/catalog?>\n",
		)},
	}
	src := "<?include\nfile: catalog.md\n?>\nold\n<?/include?>\n"
	f := newTestFile(t, "index.md", src, fsys)
	r := &Rule{}
	result := string(r.Fix(f))

	assert.NotContains(t, result, "source-dir:")
}

func TestFix_InjectsSourceDirForRootInclude(t *testing.T) {
	// When a subdir file includes a root-level file, source-dir: "."
	// should be injected so catalog globs resolve from the project root.
	fsys := fstest.MapFS{
		"root-catalog.md": {Data: []byte(
			"<?catalog\nglob: \"*.md\"\n?>\n<?/catalog?>\n",
		)},
	}
	src := "<?include\nfile: ../root-catalog.md\n?>\nold\n<?/include?>\n"
	f := newTestFile(t, "docs/index.md", src, fsys)
	r := &Rule{}
	result := string(r.Fix(f))

	assert.Contains(t, result, `source-dir: "."`)
}

func TestFix_SkipsSourceDirForSingleLinePI(t *testing.T) {
	// Single-line PIs like <?foo?> should not get source-dir injected.
	fsys := fstest.MapFS{
		"sub/content.md": {Data: []byte(
			"<?foo?>\nSome text\n",
		)},
	}
	src := "<?include\nfile: sub/content.md\n?>\nold\n<?/include?>\n"
	f := newTestFile(t, "root.md", src, fsys)
	r := &Rule{}
	result := string(r.Fix(f))

	assert.NotContains(t, result, "source-dir:")
}

func TestFix_SkipsSourceDirIfAlreadyPresent(t *testing.T) {
	// If a PI already has source-dir, don't inject a second one.
	fsys := fstest.MapFS{
		"sub/content.md": {Data: []byte(
			"<?catalog\nsource-dir: \"other\"\nglob: \"*.md\"\n?>\n<?/catalog?>\n",
		)},
	}
	src := "<?include\nfile: sub/content.md\n?>\nold\n<?/include?>\n"
	f := newTestFile(t, "root.md", src, fsys)
	r := &Rule{}
	result := string(r.Fix(f))

	// Should keep the existing source-dir, not add another.
	assert.Equal(t, 1, strings.Count(result, "source-dir:"))
}

func TestFix_NoSourceDirForPlainText(t *testing.T) {
	// Included content with no PIs should not get source-dir.
	fsys := fstest.MapFS{
		"sub/plain.md": {Data: []byte("# Plain\n\nJust text.\n")},
	}
	src := "<?include\nfile: sub/plain.md\n?>\nold\n<?/include?>\n"
	f := newTestFile(t, "root.md", src, fsys)
	r := &Rule{}
	result := string(r.Fix(f))

	assert.NotContains(t, result, "source-dir:")
}

// =====================================================================
// No FS
// =====================================================================

func TestCheck_NoFS(t *testing.T) {
	f, err := lint.NewFile("test.md", []byte("# Hello\n"))
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}
