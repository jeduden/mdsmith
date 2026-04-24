package toc

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newFile(t *testing.T, src string) *lint.File {
	t.Helper()
	f, err := lint.NewFile("test.md", []byte(src))
	require.NoError(t, err)
	return f
}

func TestID(t *testing.T) {
	assert.Equal(t, "MDS038", (&Rule{}).ID())
}

func TestName(t *testing.T) {
	assert.Equal(t, "toc", (&Rule{}).Name())
}

func TestCategory(t *testing.T) {
	assert.Equal(t, "meta", (&Rule{}).Category())
}

func TestCheck_UpToDate(t *testing.T) {
	src := "# Doc\n\n<?toc?>\n\n- [Section](#section)\n\n<?/toc?>\n\n## Section\n"
	diags := (&Rule{}).Check(newFile(t, src))
	assert.Empty(t, diags)
}

func TestCheck_Stale(t *testing.T) {
	src := "# Doc\n\n<?toc?>\nstale content\n<?/toc?>\n\n## Section\n"
	diags := (&Rule{}).Check(newFile(t, src))
	require.Len(t, diags, 1)
	assert.Equal(t, "MDS038", diags[0].RuleID)
	assert.Contains(t, diags[0].Message, "out of date")
}

func TestCheck_EmptyBodyUpToDate(t *testing.T) {
	// No headings in range → empty body is correct.
	src := "# Doc\n\n<?toc\nmin-level: 2\n?>\n<?/toc?>\n"
	diags := (&Rule{}).Check(newFile(t, src))
	assert.Empty(t, diags)
}

func TestFix_Basic(t *testing.T) {
	src := "# Doc\n\n<?toc?>\nstale\n<?/toc?>\n\n## Alpha\n\n## Beta\n"
	fixed := string((&Rule{}).Fix(newFile(t, src)))
	assert.Contains(t, fixed, "- [Alpha](#alpha)\n")
	assert.Contains(t, fixed, "- [Beta](#beta)\n")
	assert.NotContains(t, fixed, "stale")
}

func TestFix_DefaultExcludesH1(t *testing.T) {
	// Default min-level=2 should exclude the H1.
	src := "# Title\n\n<?toc?>\n<?/toc?>\n\n## Section\n"
	fixed := string((&Rule{}).Fix(newFile(t, src)))
	assert.NotContains(t, fixed, "- [Title]")
	assert.Contains(t, fixed, "- [Section](#section)")
}

func TestFix_MinMaxLevel(t *testing.T) {
	src := "# Doc\n\n<?toc\nmin-level: 2\nmax-level: 3\n?>\n<?/toc?>\n\n## H2\n\n### H3\n\n#### H4\n"
	fixed := string((&Rule{}).Fix(newFile(t, src)))
	assert.Contains(t, fixed, "- [H2](#h2)")
	assert.Contains(t, fixed, "  - [H3](#h3)")
	assert.NotContains(t, fixed, "- [H4]")
}

func TestFix_NestedStructure(t *testing.T) {
	// H2 → H4 → H2: H4 is child of first H2, second H2 is sibling.
	src := "<?toc?>\n<?/toc?>\n\n## A\n\n#### B\n\n## C\n"
	fixed := string((&Rule{}).Fix(newFile(t, src)))
	assert.Contains(t, fixed, "- [A](#a)\n  - [B](#b)\n- [C](#c)\n")
}

func TestFix_DuplicateHeadings(t *testing.T) {
	src := "<?toc?>\n<?/toc?>\n\n## Foo\n\n## Foo\n"
	fixed := string((&Rule{}).Fix(newFile(t, src)))
	assert.Contains(t, fixed, "- [Foo](#foo)\n")
	assert.Contains(t, fixed, "- [Foo](#foo-1)\n")
}

func TestValidate_InvalidMinLevel(t *testing.T) {
	r := &Rule{}
	diags := r.Validate("f.md", 1, map[string]string{"min-level": "0"}, nil)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "min-level")
}

func TestValidate_MaxLessThanMin(t *testing.T) {
	r := &Rule{}
	diags := r.Validate("f.md", 1, map[string]string{
		"min-level": "4",
		"max-level": "2",
	}, nil)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "max-level")
}

func TestFix_SingleLevel(t *testing.T) {
	src := "<?toc?>\n<?/toc?>\n\n## Only\n"
	fixed := string((&Rule{}).Fix(newFile(t, src)))
	assert.Contains(t, fixed, "- [Only](#only)\n")
}

func TestValidate_InvalidMaxLevel(t *testing.T) {
	r := &Rule{}
	diags := r.Validate("f.md", 1, map[string]string{"max-level": "7"}, nil)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "max-level")
}

func TestGenerate_InvalidParams(t *testing.T) {
	// Generate with invalid params returns diagnostics, not content.
	r := &Rule{}
	f := newFile(t, "## Sec\n")
	content, diags := r.Generate(f, "f.md", 1, map[string]string{"min-level": "bad"}, nil)
	assert.Empty(t, content)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "min-level")
}

func TestFix_SpecialCharsInHeading(t *testing.T) {
	// Headings with brackets and backslashes should be escaped in link text.
	src := "<?toc?>\n<?/toc?>\n\n## A ] B\n\n## C [ D\n\n## E \\ F\n"
	fixed := string((&Rule{}).Fix(newFile(t, src)))
	// Link text must escape special characters.
	// Special chars are removed from anchors by Slugify: "A ] B" → "a-b"
	assert.Contains(t, fixed, "- [A \\] B](#a-b)")
	assert.Contains(t, fixed, "- [C \\[ D](#c-d)")
	assert.Contains(t, fixed, "- [E \\\\ F](#e-f)")
}

func TestFix_DuplicateAnchorCollision(t *testing.T) {
	// Headings "Foo", "Foo", "Foo-1" should get anchors foo, foo-1, foo-2
	// (not foo, foo-1, foo-1 which would collide).
	src := "<?toc?>\n<?/toc?>\n\n## Foo\n\n## Foo\n\n## Foo-1\n"
	fixed := string((&Rule{}).Fix(newFile(t, src)))
	assert.Contains(t, fixed, "- [Foo](#foo)\n")
	assert.Contains(t, fixed, "- [Foo](#foo-1)\n")
	assert.Contains(t, fixed, "- [Foo-1](#foo-1-1)\n")
}
