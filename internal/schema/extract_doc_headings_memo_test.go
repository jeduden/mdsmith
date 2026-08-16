package schema

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestExtractDocHeadings_MemoizedPerFile pins that ExtractDocHeadings
// computes its result once per File and serves the cached slice on
// later calls, instead of re-walking f.AST and re-extracting every
// heading's text each time. Validate, ValidateContent, ValidateAcronyms,
// and requiredstructure.applyScopeRules each call it independently
// within one schema Check pass over the same File — before this
// memoization, every one of those callers re-walked the AST from
// scratch. Comparing the address of the first slice element proves the
// second call returned the memoized slice rather than a freshly built
// one (a value-equal but distinct slice would not share addresses).
func TestExtractDocHeadings_MemoizedPerFile(t *testing.T) {
	f, err := lint.NewFile("doc.md", []byte("# One\n\n## Two\n"))
	require.NoError(t, err)

	first := ExtractDocHeadings(f)
	require.Len(t, first, 2)

	second := ExtractDocHeadings(f)
	require.Len(t, second, 2)

	assert.Same(t, &first[0], &second[0],
		"second call must serve the memoized slice, not a freshly walked one")
}

// TestExtractDocHeadings_EmptyDocument covers the no-heading path
// still returning a usable (nil) slice through the memo.
func TestExtractDocHeadings_EmptyDocument(t *testing.T) {
	f, err := lint.NewFile("doc.md", []byte("just prose\n"))
	require.NoError(t, err)

	got := ExtractDocHeadings(f)
	assert.Empty(t, got)
}
