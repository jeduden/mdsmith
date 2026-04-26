package metrics

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestByteCount_AuthoredOnly verifies that ByteCount returns the authored-only
// byte count (excluding generated section content) for a host file.
func TestByteCount_AuthoredOnly(t *testing.T) {
	// Host file with a generated section. The generated section content
	// "Embedded line\n" (14 bytes) should not count toward ByteCount.
	hostSrc := "# Host\n\n<?include\nfile: frag.md\n?>\nEmbedded line\n<?/include?>\n\nHost end\n"
	doc := NewDocument("host.md", []byte(hostSrc))
	fullBytes := len(hostSrc)
	embeddedBytes := len("Embedded line\n")

	authoredBytes := doc.ByteCount()
	assert.Equal(t, fullBytes-embeddedBytes, authoredBytes,
		"ByteCount should exclude generated section content (%d embedded bytes)", embeddedBytes)
}

// TestLineCount_AuthoredOnly verifies that LineCount returns the authored-only
// line count (excluding generated section content).
func TestLineCount_AuthoredOnly(t *testing.T) {
	// Host file with a generated section. The embedded line should not count.
	hostSrc := "# Host\n\n<?include\nfile: frag.md\n?>\nEmbedded line\n<?/include?>\n\nHost end\n"
	doc := NewDocument("host.md", []byte(hostSrc))

	count := doc.LineCount()
	// Full content has 9 lines; embedded content is 1 line. Authored = 8.
	assert.Equal(t, 8, count,
		"LineCount should exclude the 1 generated section line")
}

// --- Collect tests ---

func TestCollect_BasicFile(t *testing.T) {
	dir := t.TempDir()
	mdFile := filepath.Join(dir, "test.md")
	require.NoError(t, os.WriteFile(mdFile, []byte("# Hello\n\nworld\n"), 0o644))

	defs := Defaults(ScopeFile)
	rows, err := Collect([]string{mdFile}, defs, 0)
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

	rows, err := Collect([]string{f1, f2}, defs, 0)
	require.NoError(t, err)
	require.Len(t, rows, 2)
}

func TestCollect_NonexistentFile(t *testing.T) {
	defs := Defaults(ScopeFile)
	missing := filepath.Join(t.TempDir(), "does-not-exist.md")
	_, err := Collect([]string{missing}, defs, 0)
	assert.Error(t, err)
}

func TestCollect_EmptyPaths(t *testing.T) {
	defs := Defaults(ScopeFile)
	rows, err := Collect([]string{}, defs, 0)
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

// (dead caching test removed — Equal on strings can't detect broken cache)

// --- WordCount tests ---

func TestWordCount_Basic(t *testing.T) {
	doc := NewDocument("test.md", []byte("# Hello World\n\none two three\n"))
	count, err := doc.WordCount()
	require.NoError(t, err)
	assert.True(t, count > 0)
}

// (dead caching test removed — Equal on ints can't detect broken cache)

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

// (dead caching test removed — Equal on ints can't detect broken cache)

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

// --- Document cache / error path tests ---

// TestFile_CachedError verifies that when fileReady=true and fileErr is set,
// File() returns the cached error without calling lint.NewFile again.
func TestFile_CachedError(t *testing.T) {
	doc := NewDocument("test.md", []byte("# Hello\n"))
	sentinel := errors.New("injected file error")
	doc.fileReady = true
	doc.fileErr = sentinel
	doc.file = nil

	f, err := doc.File()
	assert.Nil(t, f)
	assert.ErrorIs(t, err, sentinel, "expected the injected file error to be returned")

	// A second call must still return the cached error (no re-computation).
	f2, err2 := doc.File()
	assert.Nil(t, f2)
	assert.ErrorIs(t, err2, sentinel)
}

// TestPlainText_CachedError verifies that when plainTextReady=true and
// plainTextErr is set, PlainText() returns the cached error immediately.
func TestPlainText_CachedError(t *testing.T) {
	doc := NewDocument("test.md", []byte("# Hello\n"))
	sentinel := errors.New("injected plain-text error")
	doc.plainTextReady = true
	doc.plainTextErr = sentinel

	text, err := doc.PlainText()
	assert.Equal(t, "", text)
	assert.ErrorIs(t, err, sentinel)

	// Second call also returns the cached error.
	text2, err2 := doc.PlainText()
	assert.Equal(t, "", text2)
	assert.ErrorIs(t, err2, sentinel)
}

// TestPlainText_PropagatesFileError verifies that when File() returns an
// error, PlainText() propagates it and caches it in plainTextErr.
func TestPlainText_PropagatesFileError(t *testing.T) {
	doc := NewDocument("test.md", []byte("# Hello\n"))
	sentinel := errors.New("file parse failure")
	doc.fileReady = true
	doc.fileErr = sentinel

	text, err := doc.PlainText()
	assert.Equal(t, "", text)
	assert.ErrorIs(t, err, sentinel)

	// plainTextErr and plainTextReady should now be set.
	assert.True(t, doc.plainTextReady)
	assert.ErrorIs(t, doc.plainTextErr, sentinel)

	// Subsequent call returns cached error, not the file error directly.
	text2, err2 := doc.PlainText()
	assert.Equal(t, "", text2)
	assert.ErrorIs(t, err2, sentinel)
}

// TestWordCount_CachedError verifies that when wordCountReady=true and
// wordCountErr is set, WordCount() returns the cached error immediately.
func TestWordCount_CachedError(t *testing.T) {
	doc := NewDocument("test.md", []byte("# Hello\n"))
	sentinel := errors.New("injected word-count error")
	doc.wordCountReady = true
	doc.wordCountErr = sentinel

	count, err := doc.WordCount()
	assert.Equal(t, 0, count)
	assert.ErrorIs(t, err, sentinel)

	// Second call also returns the cached error.
	count2, err2 := doc.WordCount()
	assert.Equal(t, 0, count2)
	assert.ErrorIs(t, err2, sentinel)
}

// TestWordCount_PropagatesPlainTextError verifies that when PlainText() fails,
// WordCount() propagates the error and caches it in wordCountErr.
func TestWordCount_PropagatesPlainTextError(t *testing.T) {
	doc := NewDocument("test.md", []byte("# Hello\n"))
	sentinel := errors.New("plain-text failure")
	// Inject error at the plainText layer so PlainText() returns it directly.
	doc.plainTextReady = true
	doc.plainTextErr = sentinel

	count, err := doc.WordCount()
	assert.Equal(t, 0, count)
	assert.ErrorIs(t, err, sentinel)

	// wordCountErr and wordCountReady should now be set.
	assert.True(t, doc.wordCountReady)
	assert.ErrorIs(t, doc.wordCountErr, sentinel)
}

// TestHeadingCount_CachedError verifies that when headingCountReady=true and
// headingCountErr is set, HeadingCount() returns the cached error immediately.
func TestHeadingCount_CachedError(t *testing.T) {
	doc := NewDocument("test.md", []byte("# Hello\n"))
	sentinel := errors.New("injected heading-count error")
	doc.headingCountReady = true
	doc.headingCountErr = sentinel

	count, err := doc.HeadingCount()
	assert.Equal(t, 0, count)
	assert.ErrorIs(t, err, sentinel)

	// Second call also returns the cached error.
	count2, err2 := doc.HeadingCount()
	assert.Equal(t, 0, count2)
	assert.ErrorIs(t, err2, sentinel)
}

// TestHeadingCount_PropagatesFileError verifies that when File() returns an
// error, HeadingCount() propagates it and caches it in headingCountErr.
func TestHeadingCount_PropagatesFileError(t *testing.T) {
	doc := NewDocument("test.md", []byte("# Hello\n"))
	sentinel := errors.New("file parse failure")
	doc.fileReady = true
	doc.fileErr = sentinel

	count, err := doc.HeadingCount()
	assert.Equal(t, 0, count)
	assert.ErrorIs(t, err, sentinel)

	// headingCountErr and headingCountReady should now be set.
	assert.True(t, doc.headingCountReady)
	assert.ErrorIs(t, doc.headingCountErr, sentinel)
}
