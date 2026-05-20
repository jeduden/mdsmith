package listindent

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

func TestCheck_CorrectIndent2Spaces(t *testing.T) {
	src := []byte("- item 1\n  - nested\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Spaces: 2}
	diags := r.Check(f)
	require.Len(t, diags, 0, "expected 0 diagnostics, got %d: %+v", len(diags), diags)
}

func TestCheck_WrongIndent4SpacesWhenExpecting2(t *testing.T) {
	src := []byte("- item 1\n    - nested\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Spaces: 2}
	diags := r.Check(f)
	require.Len(t, diags, 1, "expected 1 diagnostic, got %d: %+v", len(diags), diags)
	if diags[0].RuleID != "MDS016" {
		t.Errorf("expected rule ID MDS016, got %s", diags[0].RuleID)
	}
}

func TestCheck_CorrectIndent4Spaces(t *testing.T) {
	src := []byte("- item 1\n    - nested\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Spaces: 4}
	diags := r.Check(f)
	require.Len(t, diags, 0, "expected 0 diagnostics, got %d: %+v", len(diags), diags)
}

func TestCheck_DeeplyNested(t *testing.T) {
	src := []byte("- level 0\n  - level 1\n    - level 2\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Spaces: 2}
	diags := r.Check(f)
	require.Len(t, diags, 0,
		"expected 0 diagnostics for correctly indented deep nesting, got %d: %+v", len(diags), diags)
}

func TestCheck_OrderedList(t *testing.T) {
	src := []byte("1. item 1\n   1. nested\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Spaces: 3}
	diags := r.Check(f)
	require.Len(t, diags, 0,
		"expected 0 diagnostics for correctly indented ordered list, got %d: %+v", len(diags), diags)
}

func TestCheck_EmptyFile(t *testing.T) {
	src := []byte("")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Spaces: 2}
	diags := r.Check(f)
	require.Len(t, diags, 0, "expected 0 diagnostics, got %d", len(diags))
}

func TestCheck_FlatList(t *testing.T) {
	src := []byte("- item 1\n- item 2\n- item 3\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Spaces: 2}
	diags := r.Check(f)
	require.Len(t, diags, 0, "expected 0 diagnostics for flat list, got %d: %+v", len(diags), diags)
}

func TestFix_AdjustsIndentation(t *testing.T) {
	src := []byte("- item 1\n    - nested\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Spaces: 2}
	result := r.Fix(f)
	expected := "- item 1\n  - nested\n"
	if string(result) != expected {
		t.Errorf("expected %q, got %q", expected, string(result))
	}
}

func TestFix_NoChange(t *testing.T) {
	src := []byte("- item 1\n  - nested\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Spaces: 2}
	result := r.Fix(f)
	if string(result) != string(src) {
		t.Errorf("expected no change, got %q", string(result))
	}
}

// --- Configurable tests ---

func TestApplySettings_ValidSpaces(t *testing.T) {
	r := &Rule{Spaces: 2}
	if err := r.ApplySettings(map[string]any{"spaces": 4}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r.Spaces != 4 {
		t.Errorf("expected Spaces=4, got %d", r.Spaces)
	}
}

func TestApplySettings_InvalidSpacesType(t *testing.T) {
	r := &Rule{Spaces: 2}
	err := r.ApplySettings(map[string]any{"spaces": "not-a-number"})
	require.Error(t, err, "expected error for non-int spaces")
}

func TestApplySettings_UnknownKey(t *testing.T) {
	r := &Rule{Spaces: 2}
	err := r.ApplySettings(map[string]any{"unknown": true})
	require.Error(t, err, "expected error for unknown key")
}

func TestDefaultSettings_ListIndent(t *testing.T) {
	r := &Rule{}
	ds := r.DefaultSettings()
	if ds["spaces"] != 2 {
		t.Errorf("expected spaces=2, got %v", ds["spaces"])
	}
}

func TestCheck_Spaces4_AllowsFourSpaceIndent(t *testing.T) {
	// With Spaces=4, four-space indent should be allowed.
	src := []byte("- item 1\n    - nested\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Spaces: 4}
	diags := r.Check(f)
	require.Len(t, diags, 0, "expected 0 diagnostics with Spaces=4 for 4-space indent, got %d", len(diags))
}

func TestCheck_Spaces4_FlagsTwoSpaceIndent(t *testing.T) {
	// With Spaces=4, two-space indent should be flagged.
	src := []byte("- item 1\n  - nested\n")
	f, err := lint.NewFile("test.md", src)
	require.NoError(t, err)
	r := &Rule{Spaces: 4}
	diags := r.Check(f)
	require.Len(t, diags, 1, "expected 1 diagnostic with Spaces=4 for 2-space indent, got %d", len(diags))
}

func TestCategory(t *testing.T) {
	r := &Rule{}
	if r.Category() == "" {
		t.Error("expected non-empty category")
	}
}

// TestFirstLineOfListItem_LinesPath pins the
// `li.Lines().Len() > 0` branch. Goldmark's parser does not
// populate Lines() on ListItem nodes in normal source — content
// lives in child Paragraph nodes — but the helper handles the
// case in case future goldmark versions or programmatic AST
// construction set Lines() directly. Construct the segment
// manually and pin that the helper returns the line for that
// segment's start offset.
func TestFirstLineOfListItem_LinesPath(t *testing.T) {
	src := []byte("alpha\nbeta\n")
	f, err := lint.NewFile("t.md", src)
	require.NoError(t, err)
	li := ast.NewListItem(0)
	li.Lines().Append(text.NewSegment(6, 10)) // "beta" starts at offset 6 (line 2)
	assert.Equal(t, 2, firstLineOfListItem(f, li))
}

// TestFirstLineOfListItem_Empty_ReturnsZero pins the final
// `return 0` defensive branch: a ListItem with no Lines() and
// no children that yield a positive line falls through to zero.
// Goldmark won't produce this in normal source, but the helper
// must not crash on a directly-constructed empty ListItem (the
// CheckNode bounds guard relies on this contract).
func TestFirstLineOfListItem_Empty_ReturnsZero(t *testing.T) {
	f, err := lint.NewFile("t.md", []byte("\n"))
	require.NoError(t, err)
	li := ast.NewListItem(0)
	assert.Equal(t, 0, firstLineOfListItem(f, li))
}

// TestCheckNode_EmptyNestedListItem_GuardSkips pins CheckNode's
// `line < 1 || line > len(f.Lines)` bounds check: a nested empty
// ListItem has nestingLevel > 0 (passes the early return) but
// firstLineOfListItem returns 0, so the bounds check fires and
// CheckNode returns nil instead of indexing f.Lines[-1].
func TestCheckNode_EmptyNestedListItem_GuardSkips(t *testing.T) {
	f, err := lint.NewFile("t.md", []byte("\n"))
	require.NoError(t, err)
	parent := ast.NewListItem(0)
	nested := ast.NewListItem(0)
	parent.AppendChild(parent, nested)
	r := &Rule{Spaces: 2}
	diags := r.CheckNode(nested, true, f)
	assert.Nil(t, diags, "empty nested ListItem must hit the bounds guard, not panic")
}

// TestFix_EmptyNestedListItem_NoAdjustment pins
// collectIndentAdjustments's parallel bounds check (rule.go
// line ~230). Use the same empty-nested-ListItem shape as the
// CheckNode test; Fix must return the source unchanged.
func TestFix_EmptyNestedListItem_NoAdjustment(t *testing.T) {
	src := []byte("\n")
	f, err := lint.NewFile("t.md", src)
	require.NoError(t, err)
	doc := ast.NewDocument()
	outer := ast.NewList('-')
	outerItem := ast.NewListItem(0)
	innerList := ast.NewList('-')
	innerItem := ast.NewListItem(0) // empty; firstLineOfListItem returns 0
	innerList.AppendChild(innerList, innerItem)
	outerItem.AppendChild(outerItem, innerList)
	outer.AppendChild(outer, outerItem)
	doc.AppendChild(doc, outer)
	f.AST = doc
	got := (&Rule{Spaces: 2}).Fix(f)
	assert.Equal(t, src, got, "empty nested ListItem must hit the Fix bounds guard")
}
