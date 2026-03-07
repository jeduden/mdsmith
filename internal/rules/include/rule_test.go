package include

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/jeduden/mdsmith/internal/lint"
)

func newTestFile(
	t *testing.T, path, source string, fs ...fstest.MapFS,
) *lint.File {
	t.Helper()
	f, err := lint.NewFile(path, []byte(source))
	if err != nil {
		t.Fatal(err)
	}
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
		"wrap: markdown\n?>\n```markdown\n# Hello\n```\n" +
		"<?/include?>\n"
	f := newTestFile(t, "doc.md", src, fsys)
	r := &Rule{}
	diags := r.Check(f)
	expectDiags(t, diags, 0)
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
	expectDiagMsg(t, diags, "not found")
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

func TestCheck_DotDotTraversal(t *testing.T) {
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
	if got != want {
		t.Errorf("Fix output mismatch\ngot:\n%s\nwant:\n%s", got, want)
	}
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
	if got != want {
		t.Errorf("Fix output mismatch\ngot:\n%s\nwant:\n%s", got, want)
	}
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
// No FS
// =====================================================================

func TestCheck_NoFS(t *testing.T) {
	f, err := lint.NewFile("test.md", []byte("# Hello\n"))
	if err != nil {
		t.Fatal(err)
	}
	r := &Rule{}
	diags := r.Check(f)
	expectDiags(t, diags, 0)
}
