package pack_test

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/pack"
	"github.com/jeduden/mdsmith/internal/wordlist"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGet_KnownAndUnknown(t *testing.T) {
	p, ok := pack.Get("wordlists")
	require.True(t, ok, "wordlists pack is registered")
	assert.Equal(t, "wordlists", p.Name)
	assert.NotEmpty(t, p.Summary)

	_, ok = pack.Get("nope")
	assert.False(t, ok)
}

func TestNames_SortedAndContainsWordlists(t *testing.T) {
	names := pack.Names()
	assert.Contains(t, names, "wordlists")
	assert.IsIncreasing(t, names, "Names() is sorted")
}

func TestAll_MatchesNames(t *testing.T) {
	all := pack.All()
	names := pack.Names()
	require.Len(t, all, len(names))
	for i, p := range all {
		assert.Equal(t, names[i], p.Name)
		assert.NotEmpty(t, p.Summary, "every pack has a summary for --list")
	}
}

func TestErrUnknown_ListsValidNames(t *testing.T) {
	err := pack.ErrUnknown("bogus")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `unknown pack "bogus"`)
	assert.Contains(t, err.Error(), "wordlists")
}

func TestWordlistsPack_Files(t *testing.T) {
	p, ok := pack.Get("wordlists")
	require.True(t, ok)
	files := p.Files()
	require.Len(t, files, 2, "one file per curated no-llm-tells list")

	byPath := map[string][]byte{}
	for _, f := range files {
		byPath[f.Path] = f.Data
	}
	// Paths are workspace-relative under .mdsmith/wordlists/.
	require.Contains(t, byPath, ".mdsmith/wordlists/ai-speak.yaml")
	require.Contains(t, byPath, ".mdsmith/wordlists/ai-openers.yaml")

	// Each body carries the wiring hint in its header and round-trips
	// through wordlist.Parse back to a non-empty entry list.
	speak := string(byPath[".mdsmith/wordlists/ai-speak.yaml"])
	assert.Contains(t, speak, "lists: [ai-speak]")
	assert.Contains(t, speak, "forbidden-text")
	_, entries, err := wordlist.Parse([]byte(speak))
	require.NoError(t, err)
	assert.NotEmpty(t, entries)
}
