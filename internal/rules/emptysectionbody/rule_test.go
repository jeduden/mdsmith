package emptysectionbody

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

func TestCheck_EmptySectionAtEOF(t *testing.T) {
	src := []byte("# Doc\n\n## Empty\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 1, "expected 1 diagnostic, got %d", len(diags))

	d := diags[0]
	if d.RuleID != "MDS030" {
		t.Errorf("expected MDS030, got %s", d.RuleID)
	}
	if d.Line != 3 {
		t.Errorf("expected line 3, got %d", d.Line)
	}
	assert.Contains(t, d.Message, "## Empty", "expected heading text in diagnostic, got: %s", d.Message)
}

func TestCheck_CommentOnlySection(t *testing.T) {
	src := []byte(
		"# Doc\n\n## Placeholder\n\n<!-- TODO: fill in later -->\n\n## Next\n\nBody.\n",
	)
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 1, "expected 1 diagnostic, got %d", len(diags))
	if diags[0].Line != 3 {
		t.Errorf("expected line 3, got %d", diags[0].Line)
	}
}

func TestCheck_AllowMarkerSkipsDiagnostic(t *testing.T) {
	src := []byte(
		"# Doc\n\n## Template Slot\n\n<?allow-empty-section?>\n",
	)
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 0, "expected 0 diagnostics, got %d", len(diags))
}

func TestCheck_PrefixedMarkerDoesNotSkipByDefault(t *testing.T) {
	src := []byte(
		"# Doc\n\n## Template Slot\n\n<!-- mdsmith: allow-empty-section -->\n",
	)
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 1, "expected 1 diagnostic, got %d", len(diags))
}

func TestCheck_MarkerCaseSensitive(t *testing.T) {
	src := []byte(
		"# Doc\n\n## Template Slot\n\n<!-- ALLOW-EMPTY-SECTION -->\n",
	)
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 1, "expected 1 diagnostic, got %d", len(diags))
}

func TestCheck_CustomAllowMarkerUsesExactString(t *testing.T) {
	src := []byte(
		"# Doc\n\n## Template Slot\n\n<?allow-empty-section?>\n",
	)
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	r := &Rule{
		MinLevel:    defaultMinLevel,
		MaxLevel:    defaultMaxLevel,
		AllowMarker: "docs: intentionally-empty",
	}
	diags := r.Check(f)
	require.Len(t, diags, 1, "expected 1 diagnostic, got %d", len(diags))
}

func TestCheck_ListContentIsMeaningful(t *testing.T) {
	src := []byte("# Doc\n\n## Steps\n\n- first\n- second\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 0, "expected 0 diagnostics, got %d", len(diags))
}

func TestCheck_CodeContentIsMeaningful(t *testing.T) {
	src := []byte("# Doc\n\n## Example\n\n```go\nfmt.Println(\"hello\")\n```\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 0, "expected 0 diagnostics, got %d", len(diags))
}

func TestCheck_TableContentIsMeaningful(t *testing.T) {
	src := []byte(
		"# Doc\n\n## Matrix\n\n| A | B |\n|---|---|\n| 1 | 2 |\n",
	)
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 0, "expected 0 diagnostics, got %d", len(diags))
}

func TestCheck_NestedHeadingWithContentIsNotEmpty(t *testing.T) {
	src := []byte("# Doc\n\n## Parent\n\n### Child\n\nDetails.\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 0, "expected 0 diagnostics, got %d", len(diags))
}

func TestCheck_NestedHeadingWithoutContentReportsBothSections(t *testing.T) {
	src := []byte("# Doc\n\n## Parent\n\n### Child\n\n## Next\n\nBody.\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 2, "expected 2 diagnostics, got %d", len(diags))
	if diags[0].Line != 3 {
		t.Errorf("expected first diagnostic at line 3, got %d", diags[0].Line)
	}
	if diags[1].Line != 5 {
		t.Errorf("expected second diagnostic at line 5, got %d", diags[1].Line)
	}
}

func TestCheck_MinLevelSkipsH2WhenSetTo3(t *testing.T) {
	src := []byte("# Doc\n\n## Parent\n\n### Child\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	r := &Rule{MinLevel: 3, MaxLevel: 6, AllowMarker: defaultAllowMarker}
	diags := r.Check(f)
	require.Len(t, diags, 1, "expected 1 diagnostic, got %d", len(diags))
	if diags[0].Line != 5 {
		t.Errorf("expected line 5, got %d", diags[0].Line)
	}
}

func TestApplySettings_Valid(t *testing.T) {
	r := &Rule{MinLevel: defaultMinLevel, MaxLevel: defaultMaxLevel, AllowMarker: defaultAllowMarker}
	err := r.ApplySettings(map[string]any{
		"min-level":    3,
		"max-level":    5,
		"allow-marker": "intentionally-empty",
	})
	require.NoError(t, err, "unexpected error: %v", err)
	if r.MinLevel != 3 {
		t.Errorf("expected MinLevel=3, got %d", r.MinLevel)
	}
	if r.MaxLevel != 5 {
		t.Errorf("expected MaxLevel=5, got %d", r.MaxLevel)
	}
	if r.AllowMarker != "intentionally-empty" {
		t.Errorf("unexpected allow marker: %s", r.AllowMarker)
	}
}

func TestApplySettings_InvalidType(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"min-level": "two"})
	require.Error(t, err, "expected error for invalid type")
}

func TestApplySettings_AllowMarkerWhitespaceOnly(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"allow-marker": "   "})
	require.Error(t, err, "expected error for whitespace-only allow-marker")
}

func TestApplySettings_AllowMarkerContainsWhitespace(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"allow-marker": "docs: empty"})
	require.Error(t, err, "expected error for allow-marker containing whitespace")
}

func TestApplySettings_InvalidRange(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"max-level": 8})
	require.Error(t, err, "expected error for invalid max-level")
}

func TestApplySettings_MinGreaterThanMax(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{
		"min-level": 5,
		"max-level": 3,
	})
	require.Error(t, err, "expected error when min-level > max-level")
}

func TestApplySettings_UnknownKey(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"unknown": true})
	require.Error(t, err, "expected error for unknown setting")
}

func TestDefaultSettings(t *testing.T) {
	r := &Rule{}
	ds := r.DefaultSettings()
	if ds["min-level"] != defaultMinLevel {
		t.Errorf("expected min-level=%d, got %v", defaultMinLevel, ds["min-level"])
	}
	if ds["max-level"] != defaultMaxLevel {
		t.Errorf("expected max-level=%d, got %v", defaultMaxLevel, ds["max-level"])
	}
	if ds["allow-marker"] != defaultAllowMarker {
		t.Errorf("expected allow-marker=%q, got %v", defaultAllowMarker, ds["allow-marker"])
	}
}

func TestID(t *testing.T) {
	r := &Rule{}
	if r.ID() != "MDS030" {
		t.Errorf("expected MDS030, got %s", r.ID())
	}
}

func TestName(t *testing.T) {
	r := &Rule{}
	if r.Name() != "empty-section-body" {
		t.Errorf("expected empty-section-body, got %s", r.Name())
	}
}

func TestCategory(t *testing.T) {
	r := &Rule{}
	if r.Category() != "heading" {
		t.Errorf("expected heading, got %s", r.Category())
	}
}

// --- toInt coverage ---

func TestToInt_Int64(t *testing.T) {
	n, ok := toInt(int64(42))
	assert.True(t, ok)
	assert.Equal(t, 42, n)
}

func TestToInt_Float64_NonWhole(t *testing.T) {
	_, ok := toInt(float64(2.5))
	assert.False(t, ok, "non-whole float64 should return false")
}

func TestToInt_Float64_Whole(t *testing.T) {
	n, ok := toInt(float64(3))
	assert.True(t, ok)
	assert.Equal(t, 3, n)
}

// --- headingLine setext coverage ---

func TestHeadingLine_SetextHeading(t *testing.T) {
	// Setext headings have Lines().Len() > 0; headingLine should use Lines().At(0).Start.
	// Use a true setext heading to trigger Lines().Len() > 0 branch.
	src := []byte("Setext Section\n==============\n\nContent.\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	// With min-level=1 to also check level-1 headings
	r := &Rule{MinLevel: 1, MaxLevel: 6, AllowMarker: defaultAllowMarker}
	diags := r.Check(f)
	// The section has content, so no diagnostic expected
	assert.Len(t, diags, 0)
}

func TestHeadingLine_SetextEmptySection(t *testing.T) {
	// Setext level-1 heading with empty body to trigger headingLine via Lines()
	src := []byte("Empty Section\n=============\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	r := &Rule{MinLevel: 1, MaxLevel: 6, AllowMarker: defaultAllowMarker}
	diags := r.Check(f)
	require.Len(t, diags, 1)
	// headingLine for setext heading should report line 1
	assert.Equal(t, 1, diags[0].Line)
}

// --- nodeHasText via Check ---

func TestNodeHasText_ParagraphContent(t *testing.T) {
	// A paragraph node hits the default branch in hasMeaningfulContent -> nodeHasText
	src := []byte("# Doc\n\n## Section\n\nSome paragraph text.\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)

	r := &Rule{}
	diags := r.Check(f)
	assert.Len(t, diags, 0, "section with paragraph should not be empty")
}

func TestApplySettings_Int64MinLevel(t *testing.T) {
	// ApplySettings passes int64 values through toInt (via YAML decoding scenario)
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"min-level": int64(2)})
	require.NoError(t, err)
	assert.Equal(t, 2, r.MinLevel)
}

// =====================================================================
// Phase 5: additional branch coverage
// =====================================================================

// Check: f.AST == nil → return nil
func TestCheck_NilAST(t *testing.T) {
	f := &lint.File{Path: "test.md"}
	r := &Rule{MinLevel: 2, MaxLevel: 6, AllowMarker: defaultAllowMarker}
	diags := r.Check(f)
	assert.Empty(t, diags, "nil AST should return no diagnostics")
}

// Check: len(nodes) == 0 → return nil
func TestCheck_EmptyDocument(t *testing.T) {
	// An empty file produces a Document with no children.
	src := []byte("")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{MinLevel: 2, MaxLevel: 6, AllowMarker: defaultAllowMarker}
	diags := r.Check(f)
	assert.Empty(t, diags, "empty document should return no diagnostics")
}

// applySetting: max-level type error (toInt returns false for string)
func TestApplySettings_MaxLevelInvalidType(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"max-level": "six"})
	require.Error(t, err, "expected error for string max-level")
	assert.Contains(t, err.Error(), "max-level must be an integer")
}

// validateLevels: maxLevel > 6
func TestApplySettings_MaxLevelTooHigh(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"max-level": 9})
	require.Error(t, err, "expected error for max-level > 6")
	assert.Contains(t, err.Error(), "max-level must be between 1 and 6")
}

// validateLevels: minLevel < 1
func TestApplySettings_MinLevelTooLow(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"min-level": 0})
	require.Error(t, err, "expected error for min-level < 1")
	assert.Contains(t, err.Error(), "min-level must be between 1 and 6")
}

// effectiveSettings: out-of-range stored values → fallback to defaults
func TestEffectiveSettings_OutOfRangeFallsBackToDefaults(t *testing.T) {
	// Store invalid values directly to force effectiveSettings fallback.
	r := &Rule{MinLevel: 10, MaxLevel: 0, AllowMarker: defaultAllowMarker}
	min, max, marker := r.effectiveSettings()
	assert.Equal(t, defaultMinLevel, min, "should fallback to default min-level")
	assert.Equal(t, defaultMaxLevel, max, "should fallback to default max-level")
	assert.Equal(t, defaultAllowMarker, marker, "should fallback to default allow-marker")
}

// hasMeaningfulContent: code block with only blank lines → not meaningful
func TestCheck_EmptyCodeBlock_ReportsViolation(t *testing.T) {
	// A fenced code block with no content is not meaningful.
	src := []byte("# Doc\n\n## Section\n\n```\n```\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	require.Len(t, diags, 1, "empty code block should not satisfy meaningful content")
}

// nodeHasText: ExtractPlainText is empty → falls through to nodeLinesText
// This is tested indirectly: an HTMLBlock with non-comment content uses nodeLinesText.
func TestCheck_HTMLBlockWithContent_IsMeaningful(t *testing.T) {
	// Raw HTML block is meaningful (non-comment)
	src := []byte("# Doc\n\n## Section\n\n<div>hello</div>\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	diags := r.Check(f)
	assert.Empty(t, diags, "HTML block with content should count as meaningful")
}

// headingLabel: empty heading → uses "(empty heading)" placeholder
func TestHeadingLabel_EmptyHeading(t *testing.T) {
	// Parse a heading that has no text children to make ExtractPlainText return "".
	// Using a setext-style heading that goldmark parses as a heading with no
	// text content is complex. Instead call headingLabel directly via Check,
	// which hits the "(empty heading)" branch when the heading has no text.
	//
	// goldmark always produces a Heading with text, so we test headingLabel
	// by calling it directly with a manually constructed ast.Heading.
	h := ast.NewHeading(2)
	label := headingLabel(h, []byte(""))
	assert.Equal(t, "## (empty heading)", label)
}

// headingLine: ATX heading with Lines()=0 and no *ast.Text child → return 1
func TestHeadingLine_ATXHeadingFallbackLine1(t *testing.T) {
	// Construct a Heading with no lines and no ast.Text children.
	h := ast.NewHeading(2)
	f := &lint.File{Path: "test.md", Source: []byte("## heading\n")}
	line := headingLine(h, f)
	// No lines, no text children → fallback to 1
	assert.Equal(t, 1, line)
}

// nodeLinesText: node with no Lines → returns ""
func TestNodeLinesText_NoLines(t *testing.T) {
	// A Paragraph node has no Lines by default.
	node := ast.NewParagraph()
	result := nodeLinesText(node, []byte("some source"))
	assert.Equal(t, "", result)
}

// hasNonBlankLines: all lines blank → returns false
func TestHasNonBlankLines_AllBlank(t *testing.T) {
	// A fenced code block with only whitespace lines has hasNonBlankLines=false.
	src := []byte("# Doc\n\n## Section\n\n```\n   \n```\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{}
	// The code block contains only blank lines, so section is empty.
	diags := r.Check(f)
	require.Len(t, diags, 1, "code block with only blank lines is not meaningful")
}

// applySetting: allow-marker type error (non-string value)
func TestApplySettings_AllowMarkerInvalidType(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"allow-marker": 42})
	require.Error(t, err, "expected error for non-string allow-marker")
	assert.Contains(t, err.Error(), "allow-marker must be a string")
}

// headingLine: heading has children including non-Text child then Text child
func TestHeadingLine_ChildTextNode(t *testing.T) {
	// Build a heading manually with an ast.Text child but no Lines().
	h := ast.NewHeading(2)
	textNode := ast.NewText()
	textNode.Segment = text.NewSegment(0, 5)
	h.AppendChild(h, textNode)
	src := []byte("## Test\n")
	f, err := lint.NewFile("heading.md", src)
	require.NoError(t, err)
	line := headingLine(h, f)
	// Text child with segment start=0 → LineOfOffset(0) = 1
	assert.Equal(t, 1, line)
}

// nodeHasText: call directly with a node that has Lines but no child Text nodes.
// nodeHasText is called from the default case in hasMeaningfulContent.
func TestNodeHasText_ViaLinesDirectCall(t *testing.T) {
	// Create a paragraph-like node but call nodeHasText directly with a node
	// whose Lines have content but no child Text nodes, so ExtractPlainText
	// returns "" but nodeLinesText returns non-empty.
	// We use a Paragraph node (which normally has text children), but we
	// construct one without children and add a line segment manually.
	node := ast.NewParagraph()
	src := []byte("hello world\n")
	lines := node.Lines()
	lines.Append(text.NewSegment(0, 12))
	result := nodeHasText(node, src)
	assert.True(t, result, "node with non-blank lines should have text")
}
