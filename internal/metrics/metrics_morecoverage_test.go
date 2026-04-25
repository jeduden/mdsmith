package metrics

import (
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
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing front matter")
}

// --- parseFrontMatter: missing id ---

func TestParseFrontMatter_MissingID(t *testing.T) {
	_, err := parseFrontMatter("---\nname: test\ndescription: desc.\n---\n")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing id")
}

// --- parseFrontMatter: missing name ---

func TestParseFrontMatter_MissingName(t *testing.T) {
	_, err := parseFrontMatter("---\nid: MET001\ndescription: desc.\n---\n")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing name")
}

// --- parseYAMLLine: no colon in line ---

func TestParseYAMLLine_NoColon(t *testing.T) {
	key, val, ok := parseYAMLLine("no colon here")
	assert.False(t, ok)
	assert.Empty(t, key)
	assert.Empty(t, val)
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
	require.Error(t, err)
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
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown metric")
}
