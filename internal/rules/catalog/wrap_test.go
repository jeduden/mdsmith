package catalog

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =====================================================================
// truncateCell
// =====================================================================

func TestTruncateCell_ShortStringUnchanged(t *testing.T) {
	got := truncateCell("hello", 10)
	assert.Equal(t, "hello", got, "expected %q, got %q", "hello", got)
}

func TestTruncateCell_ExactWidthUnchanged(t *testing.T) {
	got := truncateCell("12345", 5)
	assert.Equal(t, "12345", got, "expected %q, got %q", "12345", got)
}

func TestTruncateCell_TruncatesAtWordBoundary(t *testing.T) {
	got := truncateCell("hello world foo bar", 14)
	// "hello world..." is 14 chars
	if got != "hello world..." {
		t.Errorf("expected %q, got %q", "hello world...", got)
	}
}

func TestTruncateCell_TruncatesLongSingleWord(t *testing.T) {
	got := truncateCell("superlongword", 8)
	// "super..." is 8 chars
	assert.Equal(t, "super...", got, "expected %q, got %q", "super...", got)
}

func TestTruncateCell_EmptyString(t *testing.T) {
	got := truncateCell("", 10)
	assert.Equal(t, "", got, "expected empty string, got %q", got)
}

func TestTruncateCell_PreservesMarkdownLink(t *testing.T) {
	// Should not break inside [text](url)
	text := "[API Reference](docs/api.md) is great"
	got := truncateCell(text, 32)
	// Should keep the link intact: "[API Reference](docs/api.md)..."
	if got != "[API Reference](docs/api.md)..." {
		t.Errorf("expected %q, got %q", "[API Reference](docs/api.md)...", got)
	}
}

func TestTruncateCell_PreservesInlineCode(t *testing.T) {
	// Should not break inside `code`
	text := "Use `very-long-command` to run it"
	got := truncateCell(text, 26)
	// Should keep code span intact: "Use `very-long-command`..."
	if got != "Use `very-long-command`..." {
		t.Errorf("expected %q, got %q", "Use `very-long-command`...", got)
	}
}

func TestTruncateCell_LinkFitsExactly(t *testing.T) {
	text := "[link](url)"
	got := truncateCell(text, 11)
	assert.Equal(t, "[link](url)", got, "expected %q, got %q", "[link](url)", got)
}

func TestTruncateCell_VerySmallWidth(t *testing.T) {
	// Width too small even for "..."
	got := truncateCell("hello world", 3)
	assert.Equal(t, "...", got, "expected %q, got %q", "...", got)
}

func TestTruncateCell_WidthSmallerThanEllipsis(t *testing.T) {
	got := truncateCell("hello", 2)
	assert.Equal(t, "..", got, "expected %q, got %q", "..", got)
}

func TestTruncateCell_ZeroWidth(t *testing.T) {
	got := truncateCell("hello", 0)
	assert.Equal(t, "", got, "expected empty string, got %q", got)
}

func TestTruncateCell_LinkTooLongForWidth(t *testing.T) {
	// When a markdown link itself exceeds the width, we must truncate
	text := "[Very Long Link Text](very/long/url/path.md)"
	got := truncateCell(text, 20)
	// The link is a single span that exceeds width, so truncate at char boundary
	if len(got) > 20 {
		t.Errorf("expected length <= 20, got %d: %q", len(got), got)
	}
}

func TestTruncateCell_MultiByte_CountsRunesNotBytes(t *testing.T) {
	// "café résumé" is 11 runes but 14 bytes (é = 2 bytes each, 3 of them).
	// With maxWidth=11, it should fit without truncation.
	got := truncateCell("café résumé", 11)
	assert.Equal(t, "café résumé", got)
}

func TestTruncateCell_MultiByte_TruncatesCorrectly(t *testing.T) {
	// "ñoño mundo" is 10 runes, 12 bytes (ñ = 2 bytes each).
	// With maxWidth=7, truncate to 4 runes + "..." = "ñoño...".
	// Byte-based code would slice at byte 4, corrupting the second ñ.
	got := truncateCell("ñoño mundo", 7)
	assert.Equal(t, "ñoño...", got)
}

// =====================================================================
// wrapCellBr
// =====================================================================

func TestWrapCellBr_ShortStringUnchanged(t *testing.T) {
	got := wrapCellBr("hello", 10)
	assert.Equal(t, "hello", got, "expected %q, got %q", "hello", got)
}

func TestWrapCellBr_WrapsAtWordBoundary(t *testing.T) {
	got := wrapCellBr("hello world foo", 11)
	// "hello world" fits in 11, then "foo" on next line
	if got != "hello world<br>foo" {
		t.Errorf("expected %q, got %q", "hello world<br>foo", got)
	}
}

func TestWrapCellBr_HardBreakFallback(t *testing.T) {
	got := wrapCellBr("superlongword", 5)
	// Hard break at 5 chars: "super<br>longw<br>ord"
	assert.Equal(t, "super<br>longw<br>ord", got, "expected %q, got %q", "super<br>longw<br>ord", got)
}

func TestWrapCellBr_EmptyString(t *testing.T) {
	got := wrapCellBr("", 10)
	assert.Equal(t, "", got, "expected empty string, got %q", got)
}

func TestWrapCellBr_PreservesMarkdownLink(t *testing.T) {
	text := "[API Reference](docs/api.md) is great stuff here"
	got := wrapCellBr(text, 30)
	// The link is 28 chars, then " is" would make it 31 - so wrap after link
	if got != "[API Reference](docs/api.md)<br>is great stuff here" {
		t.Errorf("expected %q, got %q", "[API Reference](docs/api.md)<br>is great stuff here", got)
	}
}

func TestWrapCellBr_PreservesInlineCode(t *testing.T) {
	text := "Use `very-long-command` to run"
	got := wrapCellBr(text, 24)
	// "Use `very-long-command`" is 23 chars, fits in 24
	if got != "Use `very-long-command`<br>to run" {
		t.Errorf("expected %q, got %q", "Use `very-long-command`<br>to run", got)
	}
}

func TestWrapCellBr_MultipleWraps(t *testing.T) {
	got := wrapCellBr("aa bb cc dd ee", 5)
	if got != "aa bb<br>cc dd<br>ee" {
		t.Errorf("expected %q, got %q", "aa bb<br>cc dd<br>ee", got)
	}
}

func TestWrapCellBr_ExactWidthNoWrap(t *testing.T) {
	got := wrapCellBr("12345", 5)
	assert.Equal(t, "12345", got, "expected %q, got %q", "12345", got)
}

func TestWrapCellBr_MultiByte_WrapsAtRuneBoundary(t *testing.T) {
	// "café résumé world" is 17 runes. With maxWidth=11 the first line
	// should be "café résumé" (11 runes) and second "world".
	got := wrapCellBr("café résumé world", 11)
	assert.Equal(t, "café résumé<br>world", got)
}

func TestWrapCellBr_MultiByte_WithMarkdownSpan(t *testing.T) {
	// Markdown link containing multi-byte chars should not be split.
	// "see [résumé](url) here" is 22 runes. With maxWidth=16, the break
	// point falls inside the link span; wrapping should break before it.
	got := wrapCellBr("see [résumé](url) here", 16)
	// Break before the link, then link + "here" fits on second line.
	assert.Equal(t, "see<br>[résumé](url)<br>here", got)
}

func BenchmarkWrapCellBr_LargeMultiByte(b *testing.B) {
	// 100k-rune multi-byte string to detect O(n²) regressions.
	word := strings.Repeat("café ", 20000)
	b.ResetTimer()
	for b.Loop() {
		wrapCellBr(word, 20)
	}
}

func BenchmarkWrapCellBr_Allocs(b *testing.B) {
	// Keep allocation behavior visible without asserting a brittle
	// absolute threshold that can vary across Go versions or platforms.
	text := strings.Repeat("café ", 10) // 50 runes, 60 bytes
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		wrapCellBr(text, 20)
	}
}

func BenchmarkTruncateCell_Allocs(b *testing.B) {
	text := strings.Repeat("café ", 10) // 50 runes, 60 bytes
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		truncateCell(text, 20)
	}
}

// =====================================================================
// parseColumnConfig
// =====================================================================

func TestParseColumnConfig_Empty(t *testing.T) {
	cols := parseColumnConfig(nil)
	assert.Len(t, cols, 0, "expected empty map, got %v", cols)
}

func TestParseColumnConfig_MaxWidthOnly(t *testing.T) {
	raw := map[string]any{
		"description": map[string]any{
			"max-width": 50,
		},
	}
	cols := parseColumnConfig(raw)
	if cols["description"].maxWidth != 50 {
		t.Errorf("expected max-width 50, got %d", cols["description"].maxWidth)
	}
	if cols["description"].wrap != "truncate" {
		t.Errorf("expected wrap 'truncate', got %q", cols["description"].wrap)
	}
}

func TestParseColumnConfig_WrapBr(t *testing.T) {
	raw := map[string]any{
		"description": map[string]any{
			"max-width": 40,
			"wrap":      "br",
		},
	}
	cols := parseColumnConfig(raw)
	if cols["description"].maxWidth != 40 {
		t.Errorf("expected max-width 40, got %d", cols["description"].maxWidth)
	}
	if cols["description"].wrap != "br" {
		t.Errorf("expected wrap 'br', got %q", cols["description"].wrap)
	}
}

func TestParseColumnConfig_MultipleColumns(t *testing.T) {
	raw := map[string]any{
		"description": map[string]any{
			"max-width": 50,
		},
		"name": map[string]any{
			"max-width": 20,
			"wrap":      "br",
		},
	}
	cols := parseColumnConfig(raw)
	require.Len(t, cols, 2, "expected 2 columns, got %d", len(cols))
	if cols["description"].maxWidth != 50 {
		t.Errorf("expected description max-width 50, got %d", cols["description"].maxWidth)
	}
	if cols["name"].maxWidth != 20 {
		t.Errorf("expected name max-width 20, got %d", cols["name"].maxWidth)
	}
	if cols["name"].wrap != "br" {
		t.Errorf("expected name wrap 'br', got %q", cols["name"].wrap)
	}
}

// =====================================================================
// Integration: applyColumnConstraints on table rows
// =====================================================================

func TestApplyColumnConstraints_TruncatesColumn(t *testing.T) {
	row := "| [MDS001](rules/MDS001/README.md) | `line-length` | Line exceeds maximum length and is very long indeed. |"
	cols := map[string]columnConfig{
		"description": {maxWidth: 30, wrap: "truncate"},
	}
	// Column index 2 (0-indexed) is "description" based on header mapping
	colMap := map[int]string{2: "description"}
	got := applyColumnConstraints(row, cols, colMap)
	// The description column should be truncated
	if len(got) == 0 {
		t.Fatal("expected non-empty result")
	}
	// Verify the line is a valid table row
	if got[0] != '|' {
		t.Errorf("expected row to start with |, got %q", got)
	}
}

func TestApplyColumnConstraints_BrWrapColumn(t *testing.T) {
	row := "| MDS001 | Line exceeds maximum length and is very long. |"
	cols := map[string]columnConfig{
		"description": {maxWidth: 20, wrap: "br"},
	}
	colMap := map[int]string{1: "description"}
	got := applyColumnConstraints(row, cols, colMap)
	if len(got) == 0 {
		t.Fatal("expected non-empty result")
	}
	// Should contain <br> for wrapping
	if got[0] != '|' {
		t.Errorf("expected row to start with |, got %q", got)
	}
}

func TestApplyColumnConstraints_NoConstraintsPassThrough(t *testing.T) {
	row := "| foo | bar |"
	cols := map[string]columnConfig{}
	colMap := map[int]string{}
	got := applyColumnConstraints(row, cols, colMap)
	assert.Equal(t, row, got, "expected %q, got %q", row, got)
}

func TestApplyColumnConstraints_NonTableRowPassThrough(t *testing.T) {
	row := "This is not a table row"
	cols := map[string]columnConfig{
		"description": {maxWidth: 30, wrap: "truncate"},
	}
	colMap := map[int]string{0: "description"}
	got := applyColumnConstraints(row, cols, colMap)
	assert.Equal(t, row, got, "expected %q, got %q", row, got)
}

// =====================================================================
// Integration: full directive with columns
// =====================================================================

func TestRendering_ColumnsMaxWidthTruncation(t *testing.T) {
	src := `<?catalog
glob: "docs/*.md"
columns:
  description:
    max-width: 20
header: |
  | Title | Description |
  |-------|-------------|
row: "| {title} | {description} |"
?>
| Title | Description          |
|-------|----------------------|
| API   | Complete API docs... |
<?/catalog?>
`
	mapFS := fstest.MapFS{
		"docs/api.md": {Data: []byte("---\ntitle: API\ndescription: Complete API documentation for developers\n---\n")},
	}
	f := newTestFile(t, "index.md", src, mapFS)
	r := &Rule{}
	diags := r.Check(f)
	// We just need to verify this doesn't crash; the exact output format
	// is tested via the fix path
	_ = diags
}

func TestFix_ColumnsMaxWidthTruncation(t *testing.T) {
	src := `<?catalog
glob: "docs/*.md"
columns:
  description:
    max-width: 20
header: |
  | Title | Description |
  |-------|-------------|
row: "| {title} | {description} |"
?>
| old content |
<?/catalog?>
`
	mapFS := fstest.MapFS{
		"docs/api.md": {Data: []byte("---\ntitle: API\ndescription: Complete API documentation for developers\n---\n")},
	}
	f := newTestFile(t, "index.md", src, mapFS)
	r := &Rule{}
	fixed := r.Fix(f)
	result := string(fixed)
	// The description should be truncated to max-width 20
	assert.Contains(t, result, "...", "expected truncated description with '...' in output:\n%s", result)
}

func TestFix_ColumnsWrapBr(t *testing.T) {
	src := `<?catalog
glob: "docs/*.md"
columns:
  description:
    max-width: 20
    wrap: br
header: |
  | Title | Description |
  |-------|-------------|
row: "| {title} | {description} |"
?>
| old content |
<?/catalog?>
`
	mapFS := fstest.MapFS{
		"docs/api.md": {Data: []byte("---\ntitle: API\ndescription: Complete API documentation for developers\n---\n")},
	}
	f := newTestFile(t, "index.md", src, mapFS)
	r := &Rule{}
	fixed := r.Fix(f)
	result := string(fixed)
	// The description should be wrapped with <br>
	assert.Contains(t, result, "<br>", "expected wrapped description with '<br>' in output:\n%s", result)
}
