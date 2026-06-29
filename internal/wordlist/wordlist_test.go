package wordlist

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParse_EntriesAndExtends(t *testing.T) {
	ext, entries, err := Parse([]byte("extends: base\nentries:\n  - foo\n  - \"deep dive\"\n"))
	require.NoError(t, err)
	assert.Equal(t, "base", ext)
	assert.Equal(t, []string{"foo", "deep dive"}, entries)
}

func TestParse_NoExtends(t *testing.T) {
	ext, entries, err := Parse([]byte("entries:\n  - a\n  - b\n"))
	require.NoError(t, err)
	assert.Equal(t, "", ext)
	assert.Equal(t, []string{"a", "b"}, entries)
}

func TestParse_RejectsUnknownKey(t *testing.T) {
	_, _, err := Parse([]byte("bogus: 1\nentries: [a]\n"))
	require.Error(t, err)
}

func TestParse_RejectsAliases(t *testing.T) {
	_, _, err := Parse([]byte("entries: &a [x]\nmore: *a\n"))
	require.Error(t, err)
}

func TestRenderFile_RoundTrips(t *testing.T) {
	// Entries with an apostrophe and a trailing comma must survive the
	// marshal/parse round-trip unchanged.
	entries := []string{"delve", "it's important to note that", "Moreover,"}
	data, err := RenderFile("# header line\n#\n# more\n", entries)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(string(data), "# header line\n#\n# more\n"))

	ext, got, err := Parse(data)
	require.NoError(t, err)
	assert.Equal(t, "", ext)
	assert.Equal(t, entries, got)
}

func TestRenderFile_NoHeader(t *testing.T) {
	data, err := RenderFile("", []string{"a", "b"})
	require.NoError(t, err)
	assert.False(t, strings.HasPrefix(string(data), "#"))

	_, got, err := Parse(data)
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b"}, got)
}

func TestRenderFile_HeaderWithoutTrailingNewlineStillParses(t *testing.T) {
	// A header lacking a trailing newline must not glue onto the
	// `entries:` block; RenderFile inserts the separator so the output
	// still round-trips.
	data, err := RenderFile("# no trailing newline", []string{"a", "b"})
	require.NoError(t, err)
	assert.Contains(t, string(data), "# no trailing newline\nentries:")

	_, got, err := Parse(data)
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b"}, got)
}

func TestRenderFile_EmptyEntriesErrors(t *testing.T) {
	_, err := RenderFile("", nil)
	require.Error(t, err)
	_, err = RenderFile("# h\n", []string{})
	require.Error(t, err)
}

func TestLookup_UserList(t *testing.T) {
	user := map[string]Wordlist{"mine": {Name: "mine", Entries: []string{"x"}}}
	wl, err := Lookup("mine", user)
	require.NoError(t, err)
	assert.Equal(t, []string{"x"}, wl.Entries)
}

func TestLookup_UnknownListsValidNames(t *testing.T) {
	user := map[string]Wordlist{
		"alpha": {Name: "alpha"},
		"beta":  {Name: "beta"},
	}
	_, err := Lookup("nope", user)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "alpha")
	assert.Contains(t, err.Error(), "beta")
}

func TestLookup_UnknownNoListsDeclared(t *testing.T) {
	// With no user lists declared the error must not degrade to a bare
	// "(valid: )" tail.
	_, err := Lookup("ai-speak", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no word-lists are declared")
}

func TestResolve_ExtendsChainParentFirstDeduped(t *testing.T) {
	user := map[string]Wordlist{
		"base":  {Name: "base", Entries: []string{"a", "b"}},
		"child": {Name: "child", Extends: "base", Entries: []string{"b", "c"}},
	}
	got, err := Resolve("child", user)
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b", "c"}, got)
}

func TestResolve_NoExtends(t *testing.T) {
	user := map[string]Wordlist{"solo": {Name: "solo", Entries: []string{"Basically,"}}}
	got, err := Resolve("solo", user)
	require.NoError(t, err)
	assert.Contains(t, got, "Basically,")
}

func TestResolve_CycleDetected(t *testing.T) {
	user := map[string]Wordlist{
		"a": {Name: "a", Extends: "b", Entries: []string{"1"}},
		"b": {Name: "b", Extends: "a", Entries: []string{"2"}},
	}
	_, err := Resolve("a", user)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cycle")
}

func TestResolve_MissingParent(t *testing.T) {
	user := map[string]Wordlist{"a": {Name: "a", Extends: "ghost", Entries: []string{"1"}}}
	_, err := Resolve("a", user)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ghost")
}
