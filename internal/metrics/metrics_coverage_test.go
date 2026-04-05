package metrics

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Collect tests ---

func TestCollect_BasicFile(t *testing.T) {
	dir := t.TempDir()
	mdFile := filepath.Join(dir, "test.md")
	require.NoError(t, os.WriteFile(mdFile, []byte("# Hello\n\nworld\n"), 0o644))

	defs := Defaults(ScopeFile)
	rows, err := Collect([]string{mdFile}, defs)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, mdFile, rows[0].Path)
	assert.True(t, rows[0].Metrics["bytes"].Available)
	assert.True(t, rows[0].Metrics["lines"].Available)
}

func TestCollect_MultipleFiles(t *testing.T) {
	dir := t.TempDir()
	f1 := filepath.Join(dir, "a.md")
	f2 := filepath.Join(dir, "b.md")
	require.NoError(t, os.WriteFile(f1, []byte("# A\n"), 0o644))
	require.NoError(t, os.WriteFile(f2, []byte("# B\nsome text\n"), 0o644))

	defs := []Definition{
		{Name: "bytes", Kind: KindInteger, Compute: func(doc *Document) (Value, error) {
			return AvailableValue(float64(doc.ByteCount())), nil
		}},
	}

	rows, err := Collect([]string{f1, f2}, defs)
	require.NoError(t, err)
	require.Len(t, rows, 2)
}

func TestCollect_NonexistentFile(t *testing.T) {
	defs := Defaults(ScopeFile)
	_, err := Collect([]string{"/nonexistent/file.md"}, defs)
	assert.Error(t, err)
}

func TestCollect_EmptyPaths(t *testing.T) {
	defs := Defaults(ScopeFile)
	rows, err := Collect([]string{}, defs)
	require.NoError(t, err)
	assert.Empty(t, rows)
}

// --- LimitRows tests ---

func TestLimitRows_ZeroTop(t *testing.T) {
	rows := []Row{{Path: "a.md"}, {Path: "b.md"}}
	limited := LimitRows(rows, 0)
	assert.Len(t, limited, 2, "top=0 should return all rows")
}

func TestLimitRows_NegativeTop(t *testing.T) {
	rows := []Row{{Path: "a.md"}, {Path: "b.md"}}
	limited := LimitRows(rows, -1)
	assert.Len(t, limited, 2, "negative top should return all rows")
}

func TestLimitRows_TopGreaterThanLen(t *testing.T) {
	rows := []Row{{Path: "a.md"}}
	limited := LimitRows(rows, 10)
	assert.Len(t, limited, 1, "top > len should return all rows")
}

func TestLimitRows_TopEqualToLen(t *testing.T) {
	rows := []Row{{Path: "a.md"}, {Path: "b.md"}}
	limited := LimitRows(rows, 2)
	assert.Len(t, limited, 2, "top == len should return all rows")
}

func TestLimitRows_TopLessThanLen(t *testing.T) {
	rows := []Row{{Path: "a.md"}, {Path: "b.md"}, {Path: "c.md"}}
	limited := LimitRows(rows, 1)
	assert.Len(t, limited, 1)
	assert.Equal(t, "a.md", limited[0].Path)
}

// --- JSONValue tests ---

func TestJSONValue_Unavailable(t *testing.T) {
	def := Definition{Kind: KindInteger}
	v := JSONValue(def, UnavailableValue())
	assert.Nil(t, v)
}

func TestJSONValue_Integer(t *testing.T) {
	def := Definition{Kind: KindInteger}
	v := JSONValue(def, AvailableValue(42.6))
	assert.Equal(t, int64(43), v)
}

func TestJSONValue_Float(t *testing.T) {
	def := Definition{Kind: KindFloat, Precision: 2}
	v := JSONValue(def, AvailableValue(3.14159))
	assert.Equal(t, 3.14, v)
}

func TestJSONValue_FloatNegativePrecision(t *testing.T) {
	def := Definition{Kind: KindFloat, Precision: -1}
	v := JSONValue(def, AvailableValue(3.14159))
	assert.Equal(t, 3.14159, v)
}

func TestJSONValue_DefaultKind(t *testing.T) {
	def := Definition{Kind: ""}
	v := JSONValue(def, AvailableValue(42.5))
	assert.Equal(t, 42.5, v)
}

// --- Lookup tests ---

func TestLookup_ByID(t *testing.T) {
	def, ok := Lookup("MET001")
	assert.True(t, ok)
	assert.Equal(t, "bytes", def.Name)
}

func TestLookup_ByIDCaseInsensitive(t *testing.T) {
	def, ok := Lookup("met001")
	assert.True(t, ok)
	assert.Equal(t, "bytes", def.Name)
}

func TestLookup_ByName(t *testing.T) {
	def, ok := Lookup("bytes")
	assert.True(t, ok)
	assert.Equal(t, "MET001", def.ID)
}

func TestLookup_NotFound(t *testing.T) {
	_, ok := Lookup("nonexistent")
	assert.False(t, ok)
}

func TestLookup_EmptyQuery(t *testing.T) {
	_, ok := Lookup("")
	assert.False(t, ok)
}

// --- Resolve tests ---

func TestResolve_ByName(t *testing.T) {
	defs, err := Resolve(ScopeFile, []string{"bytes", "lines"})
	require.NoError(t, err)
	require.Len(t, defs, 2)
	assert.Equal(t, "bytes", defs[0].Name)
	assert.Equal(t, "lines", defs[1].Name)
}

func TestResolve_Deduplicates(t *testing.T) {
	defs, err := Resolve(ScopeFile, []string{"bytes", "MET001"})
	require.NoError(t, err)
	assert.Len(t, defs, 1, "expected deduplication of bytes/MET001")
}

func TestResolve_SkipsEmpty(t *testing.T) {
	defs, err := Resolve(ScopeFile, []string{"bytes", "", "  "})
	require.NoError(t, err)
	assert.Len(t, defs, 1)
}

func TestResolve_AllEmpty(t *testing.T) {
	_, err := Resolve(ScopeFile, []string{"", "  "})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no metrics selected")
}

func TestResolve_Unknown(t *testing.T) {
	_, err := Resolve(ScopeFile, []string{"nonexistent"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown metric")
}

// --- LineCount tests ---

func TestLineCount_Empty(t *testing.T) {
	doc := NewDocument("test.md", []byte{})
	assert.Equal(t, 0, doc.LineCount())
}

func TestLineCount_SingleLineNoNewline(t *testing.T) {
	doc := NewDocument("test.md", []byte("hello"))
	assert.Equal(t, 1, doc.LineCount())
}

func TestLineCount_SingleLineWithNewline(t *testing.T) {
	doc := NewDocument("test.md", []byte("hello\n"))
	assert.Equal(t, 1, doc.LineCount())
}

func TestLineCount_MultipleLines(t *testing.T) {
	doc := NewDocument("test.md", []byte("line1\nline2\nline3\n"))
	assert.Equal(t, 3, doc.LineCount())
}

// --- File tests ---

func TestFile_Caching(t *testing.T) {
	doc := NewDocument("test.md", []byte("# Hello\n"))
	f1, err := doc.File()
	require.NoError(t, err)
	f2, err := doc.File()
	require.NoError(t, err)
	assert.Same(t, f1, f2, "expected cached file object")
}

// --- PlainText tests ---

func TestPlainText_Basic(t *testing.T) {
	doc := NewDocument("test.md", []byte("# Hello\n\nworld\n"))
	text, err := doc.PlainText()
	require.NoError(t, err)
	assert.Contains(t, text, "Hello")
	assert.Contains(t, text, "world")
}

func TestPlainText_Caching(t *testing.T) {
	doc := NewDocument("test.md", []byte("# Hello\n"))
	t1, err := doc.PlainText()
	require.NoError(t, err)
	t2, err := doc.PlainText()
	require.NoError(t, err)
	assert.Equal(t, t1, t2)
}

// --- WordCount tests ---

func TestWordCount_Basic(t *testing.T) {
	doc := NewDocument("test.md", []byte("# Hello World\n\none two three\n"))
	count, err := doc.WordCount()
	require.NoError(t, err)
	assert.True(t, count > 0)
}

func TestWordCount_Caching(t *testing.T) {
	doc := NewDocument("test.md", []byte("# Hello\n"))
	c1, err := doc.WordCount()
	require.NoError(t, err)
	c2, err := doc.WordCount()
	require.NoError(t, err)
	assert.Equal(t, c1, c2)
}

// --- HeadingCount tests ---

func TestHeadingCount_Basic(t *testing.T) {
	doc := NewDocument("test.md", []byte("# H1\n\n## H2\n\n### H3\n"))
	count, err := doc.HeadingCount()
	require.NoError(t, err)
	assert.Equal(t, 3, count)
}

func TestHeadingCount_NoHeadings(t *testing.T) {
	doc := NewDocument("test.md", []byte("just text\n"))
	count, err := doc.HeadingCount()
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

func TestHeadingCount_Caching(t *testing.T) {
	doc := NewDocument("test.md", []byte("# H1\n"))
	c1, err := doc.HeadingCount()
	require.NoError(t, err)
	c2, err := doc.HeadingCount()
	require.NoError(t, err)
	assert.Equal(t, c1, c2)
}

// --- stripFrontMatter tests ---

func TestStripFrontMatter_NoFrontMatter(t *testing.T) {
	content := "# Hello\n"
	assert.Equal(t, content, stripFrontMatter(content))
}

func TestStripFrontMatter_WithFrontMatter(t *testing.T) {
	content := "---\ntitle: hello\n---\n# Hello\n"
	result := stripFrontMatter(content)
	assert.NotContains(t, result, "---")
	assert.Contains(t, result, "# Hello")
}

func TestStripFrontMatter_OnlyFrontMatter(t *testing.T) {
	content := "---\ntitle: hello\n---\n"
	result := stripFrontMatter(content)
	assert.Equal(t, "", result)
}

func TestStripFrontMatter_UnclosedFrontMatter(t *testing.T) {
	content := "---\ntitle: hello\n"
	assert.Equal(t, content, stripFrontMatter(content))
}

func TestStripFrontMatter_LeadingBlankLinesStripped(t *testing.T) {
	content := "---\ntitle: hello\n---\n\n\n# Hello\n"
	result := stripFrontMatter(content)
	assert.Equal(t, "# Hello\n", result)
}

// --- FormatValue additional tests ---

func TestFormatValue_Integer(t *testing.T) {
	def := Definition{Kind: KindInteger, Name: "test"}
	got := FormatValue(def, AvailableValue(42.6))
	assert.Equal(t, "43", got)
}

func TestFormatValue_Unavailable(t *testing.T) {
	def := Definition{Kind: KindInteger, Name: "test"}
	got := FormatValue(def, UnavailableValue())
	assert.Equal(t, "-", got)
}

// --- SortRows additional tests ---

func TestSortRows_AscendingOrder(t *testing.T) {
	def, ok := LookupScope(ScopeFile, "bytes")
	require.True(t, ok)

	rows := []Row{
		{Path: "c.md", Metrics: map[string]Value{"bytes": AvailableValue(30)}},
		{Path: "a.md", Metrics: map[string]Value{"bytes": AvailableValue(10)}},
		{Path: "b.md", Metrics: map[string]Value{"bytes": AvailableValue(20)}},
	}

	SortRows(rows, def, OrderAsc)
	assert.Equal(t, "a.md", rows[0].Path)
	assert.Equal(t, "b.md", rows[1].Path)
	assert.Equal(t, "c.md", rows[2].Path)
}
