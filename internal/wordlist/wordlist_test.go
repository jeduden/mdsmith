package wordlist

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParse_EntriesAndExtends(t *testing.T) {
	ext, entries, err := Parse([]byte("extends: ai-speak\nentries:\n  - foo\n  - \"deep dive\"\n"))
	require.NoError(t, err)
	assert.Equal(t, "ai-speak", ext)
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

func TestBuiltin_AiSpeak(t *testing.T) {
	wl, ok := Builtin("ai-speak")
	require.True(t, ok)
	assert.Contains(t, wl.Entries, "delve")
	assert.Contains(t, wl.Entries, "honest")
	assert.Contains(t, wl.Entries, "it's important to note that")
}

func TestBuiltin_AiOpeners(t *testing.T) {
	wl, ok := Builtin("ai-openers")
	require.True(t, ok)
	assert.Contains(t, wl.Entries, "Moreover,")
	assert.Contains(t, wl.Entries, "To sum up,")
}

func TestBuiltinNames_Sorted(t *testing.T) {
	assert.Equal(t, []string{"ai-openers", "ai-speak"}, BuiltinNames())
}

func TestLookup_UserShadowsBuiltinButBuiltinStillResolves(t *testing.T) {
	user := map[string]Wordlist{"mine": {Name: "mine", Entries: []string{"x"}}}
	wl, err := Lookup("mine", user)
	require.NoError(t, err)
	assert.Equal(t, []string{"x"}, wl.Entries)

	wl2, err := Lookup("ai-speak", user)
	require.NoError(t, err)
	assert.Contains(t, wl2.Entries, "delve")
}

func TestLookup_UnknownListsValidNames(t *testing.T) {
	_, err := Lookup("nope", nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ai-speak")
	assert.Contains(t, err.Error(), "ai-openers")
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

func TestResolve_ExtendsBuiltin(t *testing.T) {
	user := map[string]Wordlist{"x": {Name: "x", Extends: "ai-speak", Entries: []string{"zzz"}}}
	got, err := Resolve("x", user)
	require.NoError(t, err)
	assert.Contains(t, got, "delve")
	assert.Contains(t, got, "zzz")
}

func TestResolve_NoExtends(t *testing.T) {
	got, err := Resolve("ai-openers", nil)
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
