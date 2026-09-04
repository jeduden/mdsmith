package schema

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecodeFilenameField_SingleString(t *testing.T) {
	pats, err := DecodeFilenameField("[0-9]*_*.md")
	require.NoError(t, err)
	assert.Equal(t, []string{"[0-9]*_*.md"}, pats)
}

func TestDecodeFilenameField_List(t *testing.T) {
	pats, err := DecodeFilenameField([]any{"[0-9]*_*.md", "plan.md"})
	require.NoError(t, err)
	assert.Equal(t, []string{"[0-9]*_*.md", "plan.md"}, pats)
}

func TestDecodeFilenameField_StringSliceList(t *testing.T) {
	// The pre-typed []string path is used when a caller hands the
	// decoder an already-decoded slice (e.g. a test or a non-YAML
	// source).
	pats, err := DecodeFilenameField([]string{"a-*.md", "b.md"})
	require.NoError(t, err)
	assert.Equal(t, []string{"a-*.md", "b.md"}, pats)
}

func TestDecodeFilenameField_NilAndEmptyAreNoConstraint(t *testing.T) {
	for _, v := range []any{nil, "", []any{}, []string{}} {
		pats, err := DecodeFilenameField(v)
		require.NoError(t, err)
		assert.Nil(t, pats)
	}
}

func TestDecodeFilenameField_EmptyListEntryRejected(t *testing.T) {
	_, err := DecodeFilenameField([]any{"[0-9]*_*.md", ""})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "non-empty")
}

func TestDecodeFilenameField_StringSliceEmptyEntryRejected(t *testing.T) {
	// []string with an empty entry is rejected, consistent with the []any path.
	_, err := DecodeFilenameField([]string{"*.md", ""})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "non-empty")
}

func TestDecodeFilenameField_NonStringListEntryRejected(t *testing.T) {
	_, err := DecodeFilenameField([]any{"ok.md", 42})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "filename must be a string or list of strings")
}

func TestDecodeFilenameField_WrongTypeRejected(t *testing.T) {
	_, err := DecodeFilenameField(42)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "filename must be a string or list of strings")
}

func TestMatchFilename_NoConstraint(t *testing.T) {
	matched, bad, err := MatchFilename(nil, "anything.md")
	require.NoError(t, err)
	assert.True(t, matched)
	assert.Empty(t, bad)
}

func TestMatchFilename_MatchesAnyGlob(t *testing.T) {
	// The basename matches the second glob but not the first — the
	// OR semantics issue 817 asked for.
	matched, bad, err := MatchFilename([]string{"[0-9]*_*.md", "plan.md"}, "plan.md")
	require.NoError(t, err)
	assert.True(t, matched)
	assert.Empty(t, bad)
}

func TestMatchFilename_NoMatch(t *testing.T) {
	matched, bad, err := MatchFilename([]string{"[0-9]*_*.md", "plan.md"}, "notes.md")
	require.NoError(t, err)
	assert.False(t, matched)
	assert.Empty(t, bad)
}

func TestMatchFilename_MalformedGlobReportsPattern(t *testing.T) {
	matched, bad, err := MatchFilename([]string{"ok-*.md", "[unterminated"}, "notes.md")
	require.Error(t, err)
	assert.False(t, matched)
	assert.Equal(t, "[unterminated", bad)
}

func TestMatchFilename_ValidMatchWinsOverEarlierMalformedGlob(t *testing.T) {
	// A malformed glob ahead of a matching one must not mask the
	// match: OR matching is order-independent, so the base still
	// passes and no error is surfaced.
	matched, bad, err := MatchFilename([]string{"[unterminated", "plan.md"}, "plan.md")
	require.NoError(t, err)
	assert.True(t, matched)
	assert.Empty(t, bad)
}

func TestFilenameExpected_SingleKeepsHistoricalWording(t *testing.T) {
	assert.Equal(t,
		"filename matching glob [0-9]*_*.md",
		FilenameExpected([]string{"[0-9]*_*.md"}))
}

func TestFilenameExpected_MultipleListsAll(t *testing.T) {
	assert.Equal(t,
		"filename matching one of globs [0-9]*_*.md, plan.md",
		FilenameExpected([]string{"[0-9]*_*.md", "plan.md"}))
}
