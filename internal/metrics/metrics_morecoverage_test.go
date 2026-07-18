package metrics

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- concisenessScore: tokens == 0 branch ---

func TestConcisenessScore_EmptyText(t *testing.T) {
	// Empty text produces no tokens — should return 100.0.
	score := concisenessScore("")
	assert.Equal(t, 100.0, score)
}

func TestConcisenessScore_PunctuationOnly(t *testing.T) {
	// Only punctuation: no token-pattern matches → returns 100.0.
	score := concisenessScore("!!! ??? ---")
	assert.Equal(t, 100.0, score)
}

// --- concisenessScore: sentences < 1 → set to 1 branch ---

func TestConcisenessScore_NoSentenceEnding(t *testing.T) {
	// A phrase with no sentence-ending punctuation: CountSentences may
	// return 0, which is then clamped to 1 inside concisenessScore.
	score := concisenessScore("hello world foo bar")
	assert.GreaterOrEqual(t, score, 0.0)
	assert.LessOrEqual(t, score, 100.0)
}

// --- listDocsFromFS: entry.IsDir() == false branch ---

func TestListDocsFromFS_SkipsNonDirEntries(t *testing.T) {
	// A file (not dir) at the root level must be skipped.
	fsys := fstest.MapFS{
		"README.md": &fstest.MapFile{
			Data: []byte("# root level file\n"),
		},
		"good/README.md": &fstest.MapFile{
			Data: []byte("---\nid: MET999\nname: test\ndescription: A test.\n---\n# MET999\n"),
		},
	}

	docs, err := listDocsFromFS(fsys)
	require.NoError(t, err)
	require.Len(t, docs, 1)
	assert.Equal(t, "MET999", docs[0].ID)
}

// --- listDocsFromFS: ReadFile failure branch (dir has no README.md) ---

func TestListDocsFromFS_SkipsDirWithoutReadme(t *testing.T) {
	fsys := fstest.MapFS{
		"noreadme/other.md": &fstest.MapFile{
			Data: []byte("some content\n"),
		},
		"good/README.md": &fstest.MapFile{
			Data: []byte("---\nid: MET998\nname: mymetric\ndescription: desc.\n---\n# MET998\n"),
		},
	}

	docs, err := listDocsFromFS(fsys)
	require.NoError(t, err)
	require.Len(t, docs, 1)
	assert.Equal(t, "MET998", docs[0].ID)
}

// --- parseFrontMatter: missing opening delimiter ---

func TestParseFrontMatter_MissingOpenDelimiter(t *testing.T) {
	_, err := parseFrontMatter("# No front matter\n")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing front matter")
}

// --- parseFrontMatter: missing id ---

func TestParseFrontMatter_MissingID(t *testing.T) {
	_, err := parseFrontMatter("---\nname: test\ndescription: desc.\n---\n")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing id")
}

// --- parseFrontMatter: missing name ---

func TestParseFrontMatter_MissingName(t *testing.T) {
	_, err := parseFrontMatter("---\nid: MET001\ndescription: desc.\n---\n")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing name")
}

// --- parseYAMLLine: no colon in line ---

func TestParseYAMLLine_NoColon(t *testing.T) {
	key, val, ok := parseYAMLLine("no colon here")
	assert.False(t, ok)
	assert.Empty(t, key)
	assert.Empty(t, val)
}

// --- parseFrontMatter: line without colon (triggers !ok continue branch) ---

func TestParseFrontMatter_LineWithoutColon(t *testing.T) {
	// Front matter that contains a blank line (no colon) should be parsed
	// successfully, with the blank line skipped.
	content := "---\nid: MET001\n\nname: bytes\ndescription: File size.\n---\n"
	info, err := parseFrontMatter(content)
	require.NoError(t, err)
	assert.Equal(t, "MET001", info.ID)
	assert.Equal(t, "bytes", info.Name)
}

// --- FormatValue: JSONValue returns a type that doesn't match int64 or float64 ---
// JSONValue's default branch returns value.Number (float64). FormatValue's
// switch matches float64 → fmt.Sprintf. To hit the "default: return -" branch
// in FormatValue we need JSONValue to return something that is not int64 or float64.
// But JSONValue always returns float64, int64, or nil. The only way to reach
// FormatValue's default is if JSONValue returns a type we don't handle.
// This is currently not reachable unless we create a special stub.
// Instead, let's cover the Collect error path.

// --- Collect: definition Compute returns an error ---

func TestCollect_ComputeError(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/test.md"
	require.NoError(t, os.WriteFile(path, []byte("# Hello\n"), 0o644))

	defs := []Definition{
		{
			Name: "failing",
			Kind: KindInteger,
			Compute: func(doc *Document) (Value, error) {
				return UnavailableValue(), assert.AnError
			},
		},
	}

	_, err := Collect([]string{path}, defs, 0)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "computing")
}

// --- SplitList: empty / whitespace-only raw string ---

func TestSplitList_EmptyString(t *testing.T) {
	result := SplitList("")
	assert.Nil(t, result)
}

func TestSplitList_WhitespaceOnly(t *testing.T) {
	result := SplitList("   ")
	assert.Nil(t, result)
}

// --- SortRows: both unavailable, tiebreak by path ---

func TestSortRows_BothUnavailable_TieBreakByPath(t *testing.T) {
	def, ok := LookupScope(ScopeFile, "bytes")
	require.True(t, ok)

	rows := []Row{
		{Path: "b.md", Metrics: map[string]Value{"bytes": UnavailableValue()}},
		{Path: "a.md", Metrics: map[string]Value{"bytes": UnavailableValue()}},
	}

	SortRows(rows, def, OrderAsc)
	assert.Equal(t, "a.md", rows[0].Path)
	assert.Equal(t, "b.md", rows[1].Path)
}

// --- Document.File: error path (bad parse) ---
// lint.NewFile never errors on valid markdown, so we use the cached-error path
// already tested in the main coverage file. The uncovered line (68-71) is
// "if err != nil { d.fileErr = ... }". This can't be triggered without bad source.
// Covering it via the caching mechanism (already tested in TestFile_CachedError).

// --- lookupDocFromFS: no match ---

func TestLookupDocFromFS_UnknownReturnsError(t *testing.T) {
	fsys := fstest.MapFS{
		"good/README.md": &fstest.MapFile{
			Data: []byte("---\nid: MET999\nname: test\ndescription: Test.\n---\n# MET999\n"),
		},
	}

	_, err := lookupDocFromFS(fsys, "MET000")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown metric")
}

// --- listDocsFromFS: ReadDir error ---
// Use a custom fs.FS implementation that returns an error from ReadDir.

type errFS struct{}

func (e errFS) Open(name string) (fs.File, error) {
	return nil, fmt.Errorf("broken FS: open %s", name)
}

func TestListDocsFromFS_ReadDirError(t *testing.T) {
	// errFS implements fs.FS but ReadDir fails because Open(".")
	// returns an error, causing fs.ReadDir to fail.
	_, err := listDocsFromFS(errFS{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "reading metrics directory")
}

func TestLookupDocFromFS_ListDocsError(t *testing.T) {
	// When listDocsFromFS fails, lookupDocFromFS returns the error.
	_, err := lookupDocFromFS(errFS{}, "MET001")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "reading metrics directory")
}

// --- MET008/MET009/MET010 compute error paths ---

func TestMET008_Readability_NoWords_ReturnsUnavailable(t *testing.T) {
	def, ok := Lookup("MET008")
	require.True(t, ok, "MET008 must be registered")

	// Empty document has no words; readability must be unavailable, not 0.
	doc := NewDocument("test.md", []byte(""))
	v, err := def.Compute(doc)
	require.NoError(t, err)
	assert.False(t, v.Available)
}

func TestMET008_Readability_PlainTextError(t *testing.T) {
	def, ok := Lookup("MET008")
	require.True(t, ok, "MET008 must be registered")

	doc := NewDocument("test.md", []byte("# Hello\n"))
	sentinel := errors.New("plain-text failure")
	doc.plainTextReady = true
	doc.plainTextErr = sentinel

	v, err := def.Compute(doc)
	assert.Error(t, err)
	assert.ErrorIs(t, err, sentinel)
	assert.False(t, v.Available)
}

func TestMET009_Sentences_PlainTextError(t *testing.T) {
	def, ok := Lookup("MET009")
	require.True(t, ok, "MET009 must be registered")

	doc := NewDocument("test.md", []byte("# Hello\n"))
	sentinel := errors.New("plain-text failure")
	doc.plainTextReady = true
	doc.plainTextErr = sentinel

	v, err := def.Compute(doc)
	assert.Error(t, err)
	assert.ErrorIs(t, err, sentinel)
	assert.False(t, v.Available)
}

func TestMET010_AvgWordsPerSentence_WordCountError(t *testing.T) {
	def, ok := Lookup("MET010")
	require.True(t, ok, "MET010 must be registered")

	doc := NewDocument("test.md", []byte("# Hello\n"))
	sentinel := errors.New("word-count failure")
	doc.wordCountReady = true
	doc.wordCountErr = sentinel
	// PlainText must also be primed to fail so WordCount propagates it.
	doc.plainTextReady = true
	doc.plainTextErr = sentinel

	v, err := def.Compute(doc)
	assert.Error(t, err)
	assert.ErrorIs(t, err, sentinel)
	assert.False(t, v.Available)
}

func TestMET010_AvgWordsPerSentence_PlainTextErrorAfterWordCountSuccess(t *testing.T) {
	def, ok := Lookup("MET010")
	require.True(t, ok, "MET010 must be registered")

	doc := NewDocument("test.md", []byte("# Hello\n"))
	sentinel := errors.New("plain-text failure")
	// WordCount succeeds (cached), but the subsequent PlainText call fails.
	doc.wordCountReady = true
	doc.wordCount = 5
	doc.wordCountErr = nil
	doc.plainTextReady = true
	doc.plainTextErr = sentinel

	v, err := def.Compute(doc)
	assert.Error(t, err)
	assert.ErrorIs(t, err, sentinel)
	assert.False(t, v.Available)
}

func TestMET010_AvgWordsPerSentence_ZeroSentences(t *testing.T) {
	// An empty document produces 0 words and 0 sentences → result is 0.
	def, ok := Lookup("MET010")
	require.True(t, ok, "MET010 must be registered")

	doc := NewDocument("test.md", []byte(""))
	v, err := def.Compute(doc)
	require.NoError(t, err)
	assert.True(t, v.Available)
	assert.Equal(t, 0.0, v.Number)
}
