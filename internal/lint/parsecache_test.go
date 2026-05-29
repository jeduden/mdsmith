package lint

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParseCache_GetMissOnEmpty pins that Get on an empty cache
// returns (nil, false) — the LSP fast path must distinguish a real
// miss from a zero-value entry.
func TestParseCache_GetMissOnEmpty(t *testing.T) {
	c := NewParseCache()
	got, ok := c.Get("docs/foo.md", 1)
	assert.False(t, ok)
	assert.Nil(t, got)
}

// TestParseCache_PutThenGet pins the happy path: a Put at version v
// is returned by a subsequent Get(path, v).
func TestParseCache_PutThenGet(t *testing.T) {
	c := NewParseCache()
	f, err := NewFileFromSource("docs/foo.md", []byte("# Title\n"), false)
	require.NoError(t, err)

	c.Put("docs/foo.md", 1, f)

	got, ok := c.Get("docs/foo.md", 1)
	require.True(t, ok)
	assert.Same(t, f, got)
}

// TestParseCache_GetVersionMismatchMiss pins that asking for a
// different version than the stored entry returns a miss. The
// caller (engine.Runner) parses fresh on a miss.
func TestParseCache_GetVersionMismatchMiss(t *testing.T) {
	c := NewParseCache()
	f, err := NewFileFromSource("docs/foo.md", []byte("# Title\n"), false)
	require.NoError(t, err)
	c.Put("docs/foo.md", 1, f)

	got, ok := c.Get("docs/foo.md", 2)
	assert.False(t, ok, "Get must miss when the stored version differs")
	assert.Nil(t, got)
}

// TestParseCache_Invalidate pins the LSP didClose/didChange seam:
// after Invalidate(path) the next Get on that path misses
// regardless of version.
func TestParseCache_Invalidate(t *testing.T) {
	c := NewParseCache()
	f, err := NewFileFromSource("docs/foo.md", []byte("# Title\n"), false)
	require.NoError(t, err)
	c.Put("docs/foo.md", 5, f)

	c.Invalidate("docs/foo.md")

	got, ok := c.Get("docs/foo.md", 5)
	assert.False(t, ok, "Invalidate must drop the entry")
	assert.Nil(t, got)
}

// TestParseCache_StalePutRejected pins that Put with a version
// below the existing entry's version is dropped on the floor. Two
// overlapping parses of the same path may complete out of order;
// the older one must not overwrite the newer cached value.
func TestParseCache_StalePutRejected(t *testing.T) {
	c := NewParseCache()
	newer, err := NewFileFromSource("docs/foo.md", []byte("# Newer\n"), false)
	require.NoError(t, err)
	older, err := NewFileFromSource("docs/foo.md", []byte("# Older\n"), false)
	require.NoError(t, err)

	c.Put("docs/foo.md", 5, newer)
	c.Put("docs/foo.md", 3, older)

	got, ok := c.Get("docs/foo.md", 5)
	require.True(t, ok, "the newer entry must still be present")
	assert.Same(t, newer, got, "stale Put must not overwrite a newer entry")

	// The older version itself must still miss — the stored entry
	// is at version 5.
	got, ok = c.Get("docs/foo.md", 3)
	assert.False(t, ok)
	assert.Nil(t, got)
}

// TestParseCache_PutSameVersionOverwrites pins that a Put with the
// same version replaces the entry. Two concurrent parses of the
// same (path, version) may both land; whichever wins is the value
// the next Get observes. The cache contract just says the entry
// stays at that version.
func TestParseCache_PutSameVersionOverwrites(t *testing.T) {
	c := NewParseCache()
	first, err := NewFileFromSource("docs/foo.md", []byte("# A\n"), false)
	require.NoError(t, err)
	second, err := NewFileFromSource("docs/foo.md", []byte("# A\n"), false)
	require.NoError(t, err)

	c.Put("docs/foo.md", 7, first)
	c.Put("docs/foo.md", 7, second)

	got, ok := c.Get("docs/foo.md", 7)
	require.True(t, ok)
	assert.Same(t, second, got, "same-version Put must overwrite — the later landing wins")
}

// TestParseCache_PutNewerVersionEvictsOlder pins that a Put with a
// strictly newer version replaces the prior entry. LSP edits bump
// the version monotonically, so the older entry is dead the moment
// a new parse lands.
func TestParseCache_PutNewerVersionEvictsOlder(t *testing.T) {
	c := NewParseCache()
	old, err := NewFileFromSource("docs/foo.md", []byte("# Old\n"), false)
	require.NoError(t, err)
	fresh, err := NewFileFromSource("docs/foo.md", []byte("# New\n"), false)
	require.NoError(t, err)

	c.Put("docs/foo.md", 1, old)
	c.Put("docs/foo.md", 2, fresh)

	got, ok := c.Get("docs/foo.md", 2)
	require.True(t, ok)
	assert.Same(t, fresh, got)

	// The old version's lookup must now miss — it has been
	// evicted by the newer Put.
	got, ok = c.Get("docs/foo.md", 1)
	assert.False(t, ok)
	assert.Nil(t, got)
}

// TestParseCache_StalePutAfterInvalidateRejected pins the LSP race:
// a slow parse for version V cannot re-insert a *File for the slot
// that Invalidate just cleared. Without the tombstone watermark the
// stale Put would land (the entry is absent so the version-comparison
// short-circuits) and the next Get(path, V) would return the dead
// parse.
func TestParseCache_StalePutAfterInvalidateRejected(t *testing.T) {
	c := NewParseCache()
	first, err := NewFileFromSource("docs/foo.md", []byte("# Old\n"), false)
	require.NoError(t, err)
	stale, err := NewFileFromSource("docs/foo.md", []byte("# Stale\n"), false)
	require.NoError(t, err)

	c.Put("docs/foo.md", 5, first)
	c.Invalidate("docs/foo.md")
	c.Put("docs/foo.md", 5, stale)

	got, ok := c.Get("docs/foo.md", 5)
	assert.False(t, ok, "stale Put after Invalidate must not re-fill the slot")
	assert.Nil(t, got)
}

// TestParseCache_PutAfterInvalidateAdvancesWatermark pins that the
// next legitimate Put — at a strictly newer version — claims the
// slot after Invalidate. The tombstone admits only versions above
// the cleared one.
func TestParseCache_PutAfterInvalidateAdvancesWatermark(t *testing.T) {
	c := NewParseCache()
	old, err := NewFileFromSource("docs/foo.md", []byte("# Old\n"), false)
	require.NoError(t, err)
	fresh, err := NewFileFromSource("docs/foo.md", []byte("# Fresh\n"), false)
	require.NoError(t, err)

	c.Put("docs/foo.md", 5, old)
	c.Invalidate("docs/foo.md")
	c.Put("docs/foo.md", 6, fresh)

	got, ok := c.Get("docs/foo.md", 6)
	require.True(t, ok)
	assert.Same(t, fresh, got)
}

// TestParseCache_InvalidateAll pins that InvalidateAll drops every
// entry. The LSP calls this when the workspace root changes
// (configPath moves between resolveConfig calls), invalidating every
// key that was relative to the old root.
func TestParseCache_InvalidateAll(t *testing.T) {
	c := NewParseCache()
	a, err := NewFileFromSource("docs/a.md", []byte("# A\n"), false)
	require.NoError(t, err)
	b, err := NewFileFromSource("docs/b.md", []byte("# B\n"), false)
	require.NoError(t, err)
	c.Put("docs/a.md", 1, a)
	c.Put("docs/b.md", 1, b)

	c.InvalidateAll()

	_, ok := c.Get("docs/a.md", 1)
	assert.False(t, ok)
	_, ok = c.Get("docs/b.md", 1)
	assert.False(t, ok)

	// After InvalidateAll a Put at any version takes the slot —
	// the watermark is reset.
	refreshed, err := NewFileFromSource("docs/a.md", []byte("# A2\n"), false)
	require.NoError(t, err)
	c.Put("docs/a.md", 1, refreshed)
	got, ok := c.Get("docs/a.md", 1)
	require.True(t, ok)
	assert.Same(t, refreshed, got)
}

// TestParseCache_DistinctPathsIndependent pins that two paths are
// stored in independent slots — the LSP-relative key must not
// alias across documents.
func TestParseCache_DistinctPathsIndependent(t *testing.T) {
	c := NewParseCache()
	a, err := NewFileFromSource("docs/a.md", []byte("# A\n"), false)
	require.NoError(t, err)
	b, err := NewFileFromSource("docs/b.md", []byte("# B\n"), false)
	require.NoError(t, err)

	c.Put("docs/a.md", 1, a)
	c.Put("docs/b.md", 1, b)

	gotA, ok := c.Get("docs/a.md", 1)
	require.True(t, ok)
	assert.Same(t, a, gotA)
	gotB, ok := c.Get("docs/b.md", 1)
	require.True(t, ok)
	assert.Same(t, b, gotB)
}
