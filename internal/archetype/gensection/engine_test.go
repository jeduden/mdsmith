package gensection

import (
	"strings"
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockDirective is a test directive that returns static content.
type mockDirective struct {
	content    string
	genDiags   []lint.Diagnostic
	valDiags   []lint.Diagnostic
	validateFn func(filePath string, line int, params map[string]string,
		columns map[string]ColumnConfig) []lint.Diagnostic
}

func (m *mockDirective) Name() string     { return "mock" }
func (m *mockDirective) RuleID() string   { return "MDS999" }
func (m *mockDirective) RuleName() string { return "mock" }

func (m *mockDirective) Validate(filePath string, line int,
	params map[string]string, columns map[string]ColumnConfig,
) []lint.Diagnostic {
	if m.validateFn != nil {
		return m.validateFn(filePath, line, params, columns)
	}
	return m.valDiags
}

func (m *mockDirective) Generate(f *lint.File, filePath string,
	line int, params map[string]string, columns map[string]ColumnConfig,
) (string, []lint.Diagnostic) {
	return m.content, m.genDiags
}

func newTestFile(t *testing.T, path, source string) *lint.File {
	t.Helper()
	f, err := lint.NewFile(path, []byte(source))
	require.NoError(t, err)
	return f
}

func TestEngine_Check_UpToDate(t *testing.T) {
	src := "<?mock\nkey: value\n?>\nhello world\n<?/mock?>\n"
	f := newTestFile(t, "test.md", src)
	d := &mockDirective{content: "hello world\n"}
	e := NewEngine(d)
	diags := e.Check(f)
	assert.Len(t, diags, 0, "expected 0 diagnostics, got %d: %v", len(diags), diags)
}

func TestEngine_Check_OutOfDate(t *testing.T) {
	src := "<?mock\nkey: value\n?>\nold content\n<?/mock?>\n"
	f := newTestFile(t, "test.md", src)
	d := &mockDirective{content: "new content\n"}
	e := NewEngine(d)
	diags := e.Check(f)
	require.Len(t, diags, 1, "expected 1 diagnostic, got %d", len(diags))
	assert.Contains(t, diags[0].Message, "out of date", "expected 'out of date' message, got %q", diags[0].Message)
	if diags[0].RuleID != "MDS999" {
		t.Errorf("expected rule ID MDS999, got %s", diags[0].RuleID)
	}
	if diags[0].RuleName != "mock" {
		t.Errorf("expected rule name mock, got %s", diags[0].RuleName)
	}
}

func TestEngine_Check_EmptyContent(t *testing.T) {
	src := "<?mock\nkey: value\n?>\n<?/mock?>\n"
	f := newTestFile(t, "test.md", src)
	d := &mockDirective{content: ""}
	e := NewEngine(d)
	diags := e.Check(f)
	assert.Len(t, diags, 0, "expected 0 diagnostics, got %d: %v", len(diags), diags)
}

func TestEngine_Check_UnclosedMarker(t *testing.T) {
	src := "<?mock\nkey: value\n?>\ncontent\n"
	f := newTestFile(t, "test.md", src)
	d := &mockDirective{content: "content\n"}
	e := NewEngine(d)
	diags := e.Check(f)
	require.Len(t, diags, 1, "expected 1 diagnostic, got %d", len(diags))
	assert.Contains(t, diags[0].Message, "no closing marker",
		"expected 'no closing marker' message, got %q", diags[0].Message)
}

func TestEngine_Check_UnterminatedStartMarker(t *testing.T) {
	// Start marker with no ?> terminator — reaches EOF.
	src := "<?mock\nkey: value\n"
	f := newTestFile(t, "test.md", src)
	d := &mockDirective{}
	e := NewEngine(d)
	diags := e.Check(f)
	require.Len(t, diags, 1, "expected 1 diagnostic, got %d: %v", len(diags), diags)
	assert.Contains(t, diags[0].Message, "<?mock",
		"expected message to contain marker name '<?mock', got %q", diags[0].Message)
	assert.Contains(t, diags[0].Message, "?>", "expected message to mention missing '?>', got %q", diags[0].Message)
}

func TestEngine_Check_UnterminatedEndMarker(t *testing.T) {
	// Start marker is valid, end marker has no ?> terminator.
	src := "<?mock?>\ncontent\n<?/mock\n"
	f := newTestFile(t, "test.md", src)
	d := &mockDirective{content: "content\n"}
	e := NewEngine(d)
	diags := e.Check(f)
	found := false
	for _, d := range diags {
		if strings.Contains(d.Message, "<?\\/mock") || strings.Contains(d.Message, "<?/mock") {
			found = true
			assert.Contains(t, d.Message, "?>", "expected message to mention missing '?>', got %q", d.Message)
		}
	}
	assert.True(t, found, "expected diagnostic mentioning '<?/mock', got %v", diags)
}

func TestEngine_Check_OrphanedEndMarker(t *testing.T) {
	src := "text\n<?/mock?>\n"
	f := newTestFile(t, "test.md", src)
	d := &mockDirective{}
	e := NewEngine(d)
	diags := e.Check(f)
	require.Len(t, diags, 1, "expected 1 diagnostic, got %d", len(diags))
	assert.Contains(t, diags[0].Message, "unexpected generated section end marker",
		"expected 'unexpected' message, got %q", diags[0].Message)
}

func TestEngine_Check_NestedMarkers(t *testing.T) {
	// Unbalanced nesting: inner start marker has no matching end → outer
	// pair is unclosed.
	src := "<?mock\nkey: a\n?>\n<?mock\nkey: b\n?>\n<?/mock?>\n"
	f := newTestFile(t, "test.md", src)
	d := &mockDirective{content: ""}
	e := NewEngine(d)
	diags := e.Check(f)
	found := false
	for _, d := range diags {
		if strings.Contains(d.Message, "no closing marker") {
			found = true
		}
	}
	assert.True(t, found, "expected 'no closing marker' diagnostic for unbalanced nesting")
}

func TestEngine_Check_BalancedNestedMarkers(t *testing.T) {
	// Balanced nesting: inner pair has matching start/end markers.
	// The outer pair should be found; inner markers are skipped.
	src := "<?mock\nkey: a\n?>\nprefix\n<?mock\nkey: b\n?>\ninner\n<?/mock?>\nsuffix\n<?/mock?>\n"
	f := newTestFile(t, "test.md", src)
	d := &mockDirective{content: "prefix\n<?mock\nkey: b\n?>\ninner\n<?/mock?>\nsuffix\n"}
	e := NewEngine(d)
	diags := e.Check(f)
	assert.Len(t, diags, 0,
		"expected 0 diagnostics for balanced nested markers, got %d: %v", len(diags), diags)
}

func TestEngine_Fix_BalancedNestedMarkers(t *testing.T) {
	// Fix should replace only the outermost pair's content, preserving
	// inner markers as literal content.
	src := "<?mock\nkey: a\n?>\nold outer\n<?mock\nkey: b\n?>\nold inner\n<?/mock?>\n<?/mock?>\n"
	f := newTestFile(t, "test.md", src)
	d := &mockDirective{content: "new content with <?mock\nkey: nested\n?>\nnested body\n<?/mock?>\n"}
	e := NewEngine(d)
	result := string(e.Fix(f))
	assert.Contains(t, result, "new content with <?mock")
	assert.Contains(t, result, "<?/mock?>")
	assert.NotContains(t, result, "old outer")
}

func TestEngine_Check_MalformedNestedStartMarker(t *testing.T) {
	// A malformed nested start marker (missing ?>) should emit a
	// diagnostic without affecting depth tracking.
	src := "<?mock\nkey: a\n?>\n<?mock\nkey: b\n<?/mock?>\n"
	f := newTestFile(t, "test.md", src)
	d := &mockDirective{content: ""}
	e := NewEngine(d)
	diags := e.Check(f)
	found := false
	for _, d := range diags {
		if strings.Contains(d.Message, "<?mock is missing closing ?>") {
			found = true
		}
	}
	assert.True(t, found,
		"expected malformed nested start marker diagnostic, got %v", diags)
}

func TestEngine_Check_MalformedNestedEndMarker(t *testing.T) {
	// A malformed nested end marker (missing ?>) inside balanced
	// nesting should emit a diagnostic without decrementing depth.
	src := "<?mock\nkey: a\n?>\n<?mock\nkey: b\n?>\ninner\n<?/mock\n<?/mock?>\n<?/mock?>\n"
	f := newTestFile(t, "test.md", src)
	d := &mockDirective{content: ""}
	e := NewEngine(d)
	diags := e.Check(f)
	found := false
	for _, d := range diags {
		if strings.Contains(d.Message, "<?/mock is missing closing ?>") {
			found = true
		}
	}
	assert.True(t, found,
		"expected malformed nested end marker diagnostic, got %v", diags)
}

func TestEngine_Check_NestedEndMarkerTrailingContent(t *testing.T) {
	// A nested end marker with trailing content on its line should
	// emit a diagnostic and not decrement depth.
	src := "<?mock\nkey: a\n?>\n<?mock\nkey: b\n?>\ninner\n<?/mock?> extra\n<?/mock?>\n<?/mock?>\n"
	f := newTestFile(t, "test.md", src)
	d := &mockDirective{content: ""}
	e := NewEngine(d)
	diags := e.Check(f)
	found := false
	for _, d := range diags {
		if strings.Contains(d.Message, "only content on its line") {
			found = true
		}
	}
	assert.True(t, found,
		"expected 'only content on its line' diagnostic, got %v", diags)
}

func TestEngine_Check_InvalidYAML(t *testing.T) {
	src := "<?mock\n: invalid : yaml ::: [\n?>\n<?/mock?>\n"
	f := newTestFile(t, "test.md", src)
	d := &mockDirective{}
	e := NewEngine(d)
	diags := e.Check(f)
	require.Len(t, diags, 1, "expected 1 diagnostic, got %d", len(diags))
	assert.Contains(t, diags[0].Message, "invalid YAML", "expected 'invalid YAML' message, got %q", diags[0].Message)
}

func TestEngine_Check_NonStringValues(t *testing.T) {
	// Integers and floats are now silently coerced to strings; booleans
	// and maps are still rejected as non-string values.
	src := "<?mock\nkey: true\n?>\n<?/mock?>\n"
	f := newTestFile(t, "test.md", src)
	d := &mockDirective{}
	e := NewEngine(d)
	diags := e.Check(f)
	require.Len(t, diags, 1, "expected 1 diagnostic, got %d", len(diags))
	assert.Contains(t, diags[0].Message, "non-string value",
		"expected 'non-string value' message, got %q", diags[0].Message)
}

func TestEngine_Check_IntegerValueCoerced(t *testing.T) {
	// Integer YAML params (e.g. min-level: 2) are coerced to string "2".
	src := "<?mock\nkey: 42\n?>\n<?/mock?>\n"
	f := newTestFile(t, "test.md", src)
	d := &mockDirective{}
	e := NewEngine(d)
	diags := e.Check(f)
	assert.Empty(t, diags, "integer param should be coerced, not rejected")
}

func TestEngine_Check_FloatIntValueCoerced(t *testing.T) {
	// Float64 YAML params that represent integers (e.g. 2.0) are coerced to "2".
	src := "<?mock\nkey: 2.0\n?>\n<?/mock?>\n"
	f := newTestFile(t, "test.md", src)
	d := &mockDirective{validateFn: func(
		_ string, _ int, params map[string]string, _ map[string]ColumnConfig,
	) []lint.Diagnostic {
		if params["key"] != "2" {
			return []lint.Diagnostic{{Message: "expected \"2\", got " + params["key"]}}
		}
		return nil
	}}
	e := NewEngine(d)
	diags := e.Check(f)
	assert.Empty(t, diags, "float-int param should be coerced to \"2\", not rejected")
}

func TestEngine_Check_FloatFractionalValueCoerced(t *testing.T) {
	// Float64 YAML params with fractional parts (e.g. 2.5) are coerced to "2.5".
	src := "<?mock\nkey: 2.5\n?>\n<?/mock?>\n"
	f := newTestFile(t, "test.md", src)
	d := &mockDirective{validateFn: func(
		_ string, _ int, params map[string]string, _ map[string]ColumnConfig,
	) []lint.Diagnostic {
		if params["key"] != "2.5" {
			return []lint.Diagnostic{{Message: "expected \"2.5\", got " + params["key"]}}
		}
		return nil
	}}
	e := NewEngine(d)
	diags := e.Check(f)
	assert.Empty(t, diags, "fractional float param should be coerced to \"2.5\"")
}

func TestEngine_Check_ValidationDiags(t *testing.T) {
	src := "<?mock\nkey: value\n?>\n<?/mock?>\n"
	f := newTestFile(t, "test.md", src)
	d := &mockDirective{
		valDiags: []lint.Diagnostic{
			{Message: "custom validation error", RuleID: "MDS999", RuleName: "mock"},
		},
	}
	e := NewEngine(d)
	diags := e.Check(f)
	require.Len(t, diags, 1, "expected 1 diagnostic, got %d", len(diags))
	if diags[0].Message != "custom validation error" {
		t.Errorf("expected custom validation error, got %q", diags[0].Message)
	}
}

func TestEngine_Check_GenerationDiags(t *testing.T) {
	src := "<?mock\nkey: value\n?>\n<?/mock?>\n"
	f := newTestFile(t, "test.md", src)
	d := &mockDirective{
		genDiags: []lint.Diagnostic{
			{Message: "generation failed", RuleID: "MDS999", RuleName: "mock"},
		},
	}
	e := NewEngine(d)
	diags := e.Check(f)
	require.Len(t, diags, 1, "expected 1 diagnostic, got %d", len(diags))
	if diags[0].Message != "generation failed" {
		t.Errorf("expected 'generation failed', got %q", diags[0].Message)
	}
}

func TestEngine_Fix_RegeneratesContent(t *testing.T) {
	src := "<?mock\nkey: value\n?>\nold content\n<?/mock?>\n"
	f := newTestFile(t, "test.md", src)
	d := &mockDirective{content: "new content\n"}
	e := NewEngine(d)
	result := string(e.Fix(f))
	assert.Contains(t, result, "new content", "expected 'new content' in result, got:\n%s", result)
	assert.NotContains(t, result, "old content", "expected 'old content' to be replaced, got:\n%s", result)
}

func TestEngine_Fix_PreservesMarkers(t *testing.T) {
	src := "<?mock\nkey: value\n?>\nold content\n<?/mock?>\n"
	f := newTestFile(t, "test.md", src)
	d := &mockDirective{content: "new content\n"}
	e := NewEngine(d)
	result := string(e.Fix(f))
	assert.Contains(t, result, "<?mock", "expected start marker in result")
	assert.Contains(t, result, "<?/mock?>", "expected end marker in result")
}

func TestEngine_Fix_Idempotent(t *testing.T) {
	src := "<?mock\nkey: value\n?>\nhello world\n<?/mock?>\n"
	f := newTestFile(t, "test.md", src)
	d := &mockDirective{content: "hello world\n"}
	e := NewEngine(d)
	result := string(e.Fix(f))
	assert.Equal(t, src, result,
		"Fix on up-to-date content should be idempotent.\nExpected:\n%s\nGot:\n%s", src, result)
}

func TestEngine_Fix_MultiplePairs(t *testing.T) {
	src := "<?mock\nkey: a\n?>\nold a\n<?/mock?>\n\n<?mock\nkey: b\n?>\nold b\n<?/mock?>\n"
	f := newTestFile(t, "test.md", src)
	d := &mockDirective{content: "replaced\n"}
	e := NewEngine(d)
	result := string(e.Fix(f))
	count := strings.Count(result, "replaced")
	assert.Equal(t, 2, count, "expected 2 replacements, got %d in:\n%s", count, result)
}

func TestEngine_Fix_SkipsOnValidationError(t *testing.T) {
	src := "<?mock\nkey: value\n?>\nold content\n<?/mock?>\n"
	f := newTestFile(t, "test.md", src)
	d := &mockDirective{
		valDiags: []lint.Diagnostic{
			{Message: "bad param"},
		},
	}
	e := NewEngine(d)
	result := string(e.Fix(f))
	assert.Contains(t, result, "old content", "expected old content to be preserved when validation fails")
}

func TestEngine_Fix_SkipsOnGenerationError(t *testing.T) {
	src := "<?mock\nkey: value\n?>\nold content\n<?/mock?>\n"
	f := newTestFile(t, "test.md", src)
	d := &mockDirective{
		genDiags: []lint.Diagnostic{
			{Message: "gen error"},
		},
	}
	e := NewEngine(d)
	result := string(e.Fix(f))
	assert.Contains(t, result, "old content", "expected old content to be preserved when generation fails")
}

func TestEngine_Check_SingleLineMarker(t *testing.T) {
	src := "<?mock?>\n<?/mock?>\n"
	f := newTestFile(t, "test.md", src)
	d := &mockDirective{content: ""}
	e := NewEngine(d)
	diags := e.Check(f)
	assert.Len(t, diags, 0, "expected 0 diagnostics, got %d: %v", len(diags), diags)
}

func TestEngine_Check_ColumnsPassedToDirective(t *testing.T) {
	src := "<?mock\nkey: value\ncolumns:\n  desc:\n    max-width: 30\n    wrap: br\n?>\nhello\n<?/mock?>\n"
	f := newTestFile(t, "test.md", src)
	var receivedCols map[string]ColumnConfig
	d := &mockDirective{
		content: "hello\n",
		validateFn: func(_ string, _ int, _ map[string]string, columns map[string]ColumnConfig) []lint.Diagnostic {
			receivedCols = columns
			return nil
		},
	}
	e := NewEngine(d)
	diags := e.Check(f)
	assert.Len(t, diags, 0, "expected 0 diagnostics, got %d: %v", len(diags), diags)
	require.NotNil(t, receivedCols, "expected columns to be passed to Validate")
	cc, ok := receivedCols["desc"]
	require.True(t, ok, "expected 'desc' column config")
	if cc.MaxWidth != 30 {
		t.Errorf("expected MaxWidth 30, got %d", cc.MaxWidth)
	}
	if cc.Wrap != "br" {
		t.Errorf("expected Wrap 'br', got %q", cc.Wrap)
	}
}

func TestEngine_Check_MarkerInCodeBlock(t *testing.T) {
	src := "```\n<?mock\nkey: value\n?>\n<?/mock?>\n```\n"
	f := newTestFile(t, "test.md", src)
	d := &mockDirective{}
	e := NewEngine(d)
	diags := e.Check(f)
	assert.Len(t, diags, 0, "expected markers in code block to be ignored, got %d diagnostics", len(diags))
}

func TestEngine_Check_YAMLTerminatorWithWhitespace(t *testing.T) {
	src := "<?mock\nkey: value\n  ?>\nhello\n<?/mock?>\n"
	f := newTestFile(t, "test.md", src)
	d := &mockDirective{content: "hello\n"}
	e := NewEngine(d)
	diags := e.Check(f)
	assert.Len(t, diags, 0, "expected 0 diagnostics, got %d: %v", len(diags), diags)
}

func TestEngine_Check_EndMarkerWithWhitespace(t *testing.T) {
	src := "<?mock\nkey: value\n?>\nhello\n  <?/mock?>\n"
	f := newTestFile(t, "test.md", src)
	d := &mockDirective{content: "hello\n"}
	e := NewEngine(d)
	diags := e.Check(f)
	assert.Len(t, diags, 0, "expected 0 diagnostics, got %d: %v", len(diags), diags)
}

func TestEngine_Check_EndMarkerWithTrailingContent(t *testing.T) {
	src := "<?mock\nkey: value\n?>\nhello\n<?/mock?> extra\n"
	f := newTestFile(t, "test.md", src)
	d := &mockDirective{content: "hello\n"}
	e := NewEngine(d)
	diags := e.Check(f)
	found := false
	for _, d := range diags {
		if strings.Contains(d.Message, "only content on its line") {
			found = true
		}
	}
	assert.True(t, found, "expected 'only content on its line' diagnostic, got %v", diags)
}

func TestEnsureTrailingNewline_AddsNewline(t *testing.T) {
	got := EnsureTrailingNewline("hello")
	assert.Equal(t, "hello\n", got, "expected %q, got %q", "hello\n", got)
}

func TestEnsureTrailingNewline_PreservesExisting(t *testing.T) {
	got := EnsureTrailingNewline("hello\n")
	assert.Equal(t, "hello\n", got, "expected %q, got %q", "hello\n", got)
}

func TestSplitLines_Basic(t *testing.T) {
	lines := SplitLines([]byte("a\nb\nc"))
	require.Len(t, lines, 3, "expected 3 lines, got %d", len(lines))
	if string(lines[0]) != "a" {
		t.Errorf("line 0: got %q", string(lines[0]))
	}
}

func TestSplitLines_Empty(t *testing.T) {
	lines := SplitLines([]byte(""))
	require.Len(t, lines, 1, "expected 1 line, got %d", len(lines))
}

func TestParseColumnConfig_Basic(t *testing.T) {
	raw := map[string]any{
		"desc": map[string]any{
			"max-width": 50,
			"wrap":      "br",
		},
	}
	cols := ParseColumnConfig(raw)
	if cols["desc"].MaxWidth != 50 {
		t.Errorf("expected MaxWidth 50, got %d", cols["desc"].MaxWidth)
	}
	if cols["desc"].Wrap != "br" {
		t.Errorf("expected Wrap 'br', got %q", cols["desc"].Wrap)
	}
}

func TestParseColumnConfig_Nil(t *testing.T) {
	cols := ParseColumnConfig(nil)
	assert.Len(t, cols, 0, "expected empty map, got %v", cols)
}

func TestParseColumnConfig_DefaultWrap(t *testing.T) {
	raw := map[string]any{
		"desc": map[string]any{
			"max-width": 30,
		},
	}
	cols := ParseColumnConfig(raw)
	if cols["desc"].Wrap != "truncate" {
		t.Errorf("expected default wrap 'truncate', got %q", cols["desc"].Wrap)
	}
}

// =====================================================================
// Phase 5: additional branch coverage
// =====================================================================

// TestEngine_Fix_SkipsOnInvalidYAML exercises the generateContent
// `dir == nil || len(diags) > 0` branch in Fix.
// When the YAML body is invalid, generateContent returns (_, false) and Fix skips.
func TestEngine_Fix_SkipsOnInvalidYAML(t *testing.T) {
	src := "<?mock\n: invalid : yaml ::: [\n?>\nold content\n<?/mock?>\n"
	f := newTestFile(t, "test.md", src)
	d := &mockDirective{content: "new content\n"}
	e := NewEngine(d)
	result := string(e.Fix(f))
	// Fix should leave old content intact when YAML parsing fails.
	assert.Contains(t, result, "old content", "expected old content preserved when YAML is invalid")
	assert.NotContains(t, result, "new content")
}
