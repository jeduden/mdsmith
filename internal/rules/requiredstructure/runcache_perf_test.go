package requiredstructure

import (
	"io/fs"
	"sync/atomic"
	"testing"
	"testing/fstest"

	"github.com/jeduden/mdsmith/internal/lint"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// countingFS wraps an fs.FS and counts Open calls, so a test can
// assert a file was read at most once across several lookups.
type countingFS struct {
	fs.FS
	opens *int32
}

func (c countingFS) Open(name string) (fs.File, error) {
	atomic.AddInt32(c.opens, 1)
	return c.FS.Open(name)
}

// TestDispatchSingleFileSchema_CachesRawSchemaAcrossHostFiles pins
// that a schema referenced by many host files sharing a RunCache is
// read from disk exactly once per run, not once per host file.
// dispatchSingleFileSchema previously called loadSchemaAt and
// schemaDataDeclaresExtends unconditionally on every Check call —
// docs/development/high-performance-go.md's "memoize per-input
// computations" pattern, applied to a workspace-wide kind's schema
// file.
func TestDispatchSingleFileSchema_CachesRawSchemaAcrossHostFiles(t *testing.T) {
	var opens int32
	fsys := countingFS{
		FS: fstest.MapFS{
			"schema.md": &fstest.MapFile{Data: []byte("# ?\n")},
		},
		opens: &opens,
	}
	cache := lint.NewRunCache()
	r := &Rule{Schema: "schema.md", Sources: []SchemaSource{{File: "schema.md"}}}

	for _, name := range []string{"a.md", "b.md", "c.md"} {
		f, err := lint.NewFileFromSource(name, []byte("# Title\n"), true)
		require.NoError(t, err)
		f.RootDir = "/repo"
		f.RootFS = fsys
		f.RunCache = cache
		diags := r.Check(f)
		assert.Empty(t, diags, "host file %q should pass a trivial schema", name)
	}

	assert.Equal(t, int32(1), atomic.LoadInt32(&opens),
		"schema file must be read once across host files sharing a RunCache")
}

// TestDispatchSingleFileSchema_InvalidateRereadsSchema pins that the
// cache is not permanent: invalidating the schema's absolute path
// (the LSP's edit-then-invalidate loop) forces a fresh read on the
// next lookup instead of serving stale bytes forever.
func TestDispatchSingleFileSchema_InvalidateRereadsSchema(t *testing.T) {
	var opens int32
	fsys := countingFS{
		FS: fstest.MapFS{
			"schema.md": &fstest.MapFile{Data: []byte("# ?\n")},
		},
		opens: &opens,
	}
	cache := lint.NewRunCache()
	r := &Rule{Schema: "schema.md", Sources: []SchemaSource{{File: "schema.md"}}}

	newHost := func(name string) *lint.File {
		f, err := lint.NewFileFromSource(name, []byte("# Title\n"), true)
		require.NoError(t, err)
		f.RootDir = "/repo"
		f.RootFS = fsys
		f.RunCache = cache
		return f
	}

	r.Check(newHost("a.md"))
	require.Equal(t, int32(1), atomic.LoadInt32(&opens))

	cache.Invalidate("/repo/schema.md")

	r.Check(newHost("b.md"))
	assert.Equal(t, int32(2), atomic.LoadInt32(&opens),
		"Invalidate must force a fresh schema read on the next lookup")
}

// TestDispatchSingleFileSchema_MissingSchemaErrorUsesCallersOwnSpelling
// pins a review-flagged regression: two kinds that reference the same
// missing schema file via differently-spelled but path-equivalent
// strings ("docs/proto.md" vs "./docs/proto.md" — both resolve to the
// same absSchemaCacheKey) share one RunCache slot. The cached error
// must not embed whichever caller's spelling happened to trigger the
// build first; each caller's diagnostic must name its own configured
// schema path.
func TestDispatchSingleFileSchema_MissingSchemaErrorUsesCallersOwnSpelling(t *testing.T) {
	fsys := fstest.MapFS{}
	cache := lint.NewRunCache()

	rA := &Rule{Schema: "docs/proto.md", Sources: []SchemaSource{{File: "docs/proto.md"}}}
	rB := &Rule{Schema: "./docs/proto.md", Sources: []SchemaSource{{File: "./docs/proto.md"}}}

	newHost := func(name string) *lint.File {
		f, err := lint.NewFileFromSource(name, []byte("# Title\n"), true)
		require.NoError(t, err)
		f.RootDir = "/repo"
		f.RootFS = fsys
		f.RunCache = cache
		return f
	}

	diagsA := rA.Check(newHost("a.md"))
	require.Len(t, diagsA, 1)
	assert.Contains(t, diagsA[0].Message, `"docs/proto.md"`)

	diagsB := rB.Check(newHost("b.md"))
	require.Len(t, diagsB, 1)
	assert.Contains(t, diagsB[0].Message, `"./docs/proto.md"`,
		"error message must reflect this rule's own configured schema "+
			"spelling, not a cached spelling from a different kind that "+
			"happens to resolve to the same absolute schema path")
}
