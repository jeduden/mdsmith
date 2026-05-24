package requiredstructure

import (
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCachedParseSchema_BuildsOncePerRunCache pins that two host files
// referencing the same schema parse the schema exactly once when they
// share a RunCache. Plan 195 task 15: closes the per-host-file schema
// re-parse hot spot.
func TestCachedParseSchema_BuildsOncePerRunCache(t *testing.T) {
	schemaBody := []byte("# Title\n\n## Section\n")
	const absPath = "/abs/schema.md"

	cache := lint.NewRunCache()
	var calls int32
	build := func() (*parsedSchema, []string, error) {
		atomic.AddInt32(&calls, 1)
		sch, err := parseSchema(schemaBody, "", 0)
		return sch, nil, err
	}

	for i := 0; i < 3; i++ {
		got, err := cachedParseSchemaWith(cache, absPath, build)
		require.NoError(t, err)
		require.NotNil(t, got)
		require.Len(t, got.Headings, 2)
	}
	assert.Equal(t, int32(1), atomic.LoadInt32(&calls),
		"parseSchema must run exactly once across host files sharing a RunCache")
}

// TestCachedParseSchema_NilRunCacheStillParses pins the struct-literal
// unit-test path: a File without a RunCache still gets a parsed schema,
// just without the cross-host cache. Without this branch the rule would
// crash on every direct test invocation that doesn't supply a cache.
func TestCachedParseSchema_NilRunCacheStillParses(t *testing.T) {
	schemaBody := []byte("# Title\n\n## Section\n")
	got, err := cachedParseSchemaWith(nil, "/abs/schema.md", func() (*parsedSchema, []string, error) {
		sch, err := parseSchema(schemaBody, "", 0)
		return sch, nil, err
	})
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Len(t, got.Headings, 2)
}

// TestCachedParseSchema_EmptyAbsPathFallsThroughToBuild pins that an
// empty absPath bypasses the cache (the unit-test path where a schema
// is parsed from raw bytes without a filesystem identity). The build
// still runs and returns a result.
func TestCachedParseSchema_EmptyAbsPathFallsThroughToBuild(t *testing.T) {
	cache := lint.NewRunCache()
	var calls int32
	for i := 0; i < 2; i++ {
		got, err := cachedParseSchemaWith(cache, "", func() (*parsedSchema, []string, error) {
			atomic.AddInt32(&calls, 1)
			sch, err := parseSchema([]byte("# Heading\n"), "", 0)
			return sch, nil, err
		})
		require.NoError(t, err)
		require.NotNil(t, got)
	}
	assert.Equal(t, int32(2), atomic.LoadInt32(&calls),
		"an empty absPath must skip the cache so unrelated files do not share an entry")
}

// TestCachedParseSchema_CachesParseErrors pins that a broken schema is
// not re-parsed on every host file. A subsequent caller observes the
// same error from the cache.
func TestCachedParseSchema_CachesParseErrors(t *testing.T) {
	cache := lint.NewRunCache()
	var calls int32
	expectedErr := assert.AnError
	for i := 0; i < 3; i++ {
		got, err := cachedParseSchemaWith(cache, "/abs/broken.md", func() (*parsedSchema, []string, error) {
			atomic.AddInt32(&calls, 1)
			return nil, nil, expectedErr
		})
		assert.Nil(t, got)
		require.ErrorIs(t, err, expectedErr)
	}
	assert.Equal(t, int32(1), atomic.LoadInt32(&calls),
		"a parse error must be cached so a broken schema isn't re-read once per host file")
}

// TestCachedCompiledCUE_BuildsOncePerSource pins that compiling the
// same CUE source twice returns the same cached wrapper. Plan 195 task
// 15.
func TestCachedCompiledCUE_BuildsOncePerSource(t *testing.T) {
	cache := lint.NewRunCache()
	const src = `close({id: string})`
	v1 := cachedCompiledCUEWith(cache, src)
	v2 := cachedCompiledCUEWith(cache, src)
	require.NotNil(t, v1)
	require.NoError(t, v1.Err())
	assert.Same(t, v1, v2,
		"the same source string must return the same compiled-CUE wrapper")
}

// TestCachedCompiledCUE_NilCacheStillCompiles pins the struct-literal
// path: a missing RunCache falls back to a fresh compile so direct
// unit tests still work.
func TestCachedCompiledCUE_NilCacheStillCompiles(t *testing.T) {
	v := cachedCompiledCUEWith(nil, `{id: string}`)
	require.NotNil(t, v)
	require.NoError(t, v.Err())
}

// TestCachedCompiledCUE_DistinctSourcesDoNotShare pins that two
// different CUE source strings produce independent entries.
func TestCachedCompiledCUE_DistinctSourcesDoNotShare(t *testing.T) {
	cache := lint.NewRunCache()
	v1 := cachedCompiledCUEWith(cache, `{a: string}`)
	v2 := cachedCompiledCUEWith(cache, `{b: string}`)
	require.NotNil(t, v1)
	require.NotNil(t, v2)
	assert.NotSame(t, v1, v2,
		"distinct CUE source strings must not share a slot")
}

// TestCachedCompiledCUE_ConcurrentSingleBuild pins that concurrent
// callers compiling the same source share one compile.
func TestCachedCompiledCUE_ConcurrentSingleBuild(t *testing.T) {
	cache := lint.NewRunCache()
	const src = `{shared: string}`
	var wg sync.WaitGroup
	results := make([]*schema.CompiledCUE, 16)
	for i := 0; i < 16; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i] = cachedCompiledCUEWith(cache, src)
		}()
	}
	wg.Wait()
	for i := 1; i < len(results); i++ {
		assert.Same(t, results[0], results[i],
			"all concurrent callers must observe the same compiled-CUE wrapper")
	}
}

// TestRule_SchemaParsedOncePerRunCache is the end-to-end integration
// check: when three host files all reference one schema file via the
// rule's Check path, the underlying parseSchema runs exactly once.
// The test installs a counter into the cache's parsed-schema slot so
// the assertion holds against the actual rule dispatch path. Without
// the RunCache wiring this would fire parseSchema once per host file.
func TestRule_SchemaParsedOncePerRunCache(t *testing.T) {
	schemaSrc := "# {id}: {name}\n\n## Section\n"
	doc := "---\nid: doc\nname: Sample\n---\n# doc: Sample\n\n## Section\n"

	cache := lint.NewRunCache()
	inner := fstest.MapFS{
		"schema.md": &fstest.MapFile{
			Data:    []byte(schemaSrc),
			ModTime: time.Time{},
		},
		"doc.md": &fstest.MapFile{
			Data:    []byte(doc),
			ModTime: time.Time{},
		},
	}

	r := &Rule{Schema: "schema.md", Sources: []SchemaSource{{File: "schema.md"}}}

	rootDir, err := filepathAbs(t, ".")
	require.NoError(t, err)
	for i := 0; i < 3; i++ {
		f, err := lint.NewFile("doc.md", []byte(doc))
		require.NoError(t, err)
		f.FS = inner
		f.RootFS = inner
		f.RootDir = rootDir
		f.RunCache = cache
		_ = r.Check(f)
	}

	// The cache's ParsedSchema slot is keyed by absolute path. A
	// follow-up call with a unique build closure must hit the cache
	// (build never runs) — proving the rule populated the slot.
	absKey := filepath.Clean(filepath.Join(rootDir, "schema.md"))
	var rebuilds int32
	_ = cache.ParsedSchema(absKey, func() any {
		atomic.AddInt32(&rebuilds, 1)
		return schemaParseResult{schema: nil, err: assert.AnError}
	})
	assert.Equal(t, int32(0), atomic.LoadInt32(&rebuilds),
		"after Rule.Check the schema's ParsedSchema entry must be populated; a follow-up lookup must hit the cache")
}

func filepathAbs(t *testing.T, p string) (string, error) {
	t.Helper()
	return filepath.Abs(p)
}

// TestRule_FragmentInvalidationEvictsParsedSchema is the end-to-end
// integration check for Copilot thread 1 on PR #377: after Rule.Check
// reads a schema whose <?include?> reaches a fragment, calling
// RunCache.Invalidate(fragment) must evict the schema's ParsedSchema
// slot so the next Check re-parses against the new fragment.
//
// The test writes schema.md and fragment.md to a real temp directory
// because schema include resolution reads through the OS filesystem
// (ReadFileLimited → os.ReadFile), not the in-memory fstest.MapFS
// used elsewhere in this file. t.Chdir scopes CWD to tmpDir so both
// the schema and its fragment resolve against the same root the rule
// sees.
func TestRule_FragmentInvalidationEvictsParsedSchema(t *testing.T) {
	tmpDir := t.TempDir()
	schemaSrc := "# {id}: {name}\n\n<?include\nfile: fragment.md\n?>\n"
	fragmentSrc := "## Section\n"
	doc := "---\nid: doc\nname: Sample\n---\n# doc: Sample\n\n## Section\n"

	require.NoError(t,
		writeFile(filepath.Join(tmpDir, "schema.md"), schemaSrc))
	require.NoError(t,
		writeFile(filepath.Join(tmpDir, "fragment.md"), fragmentSrc))
	require.NoError(t,
		writeFile(filepath.Join(tmpDir, "doc.md"), doc))

	t.Chdir(tmpDir)

	cache := lint.NewRunCache()
	r := &Rule{Schema: "schema.md", Sources: []SchemaSource{{File: "schema.md"}}}

	f, err := lint.NewFile("doc.md", []byte(doc))
	require.NoError(t, err)
	f.RootDir = tmpDir
	f.RunCache = cache
	_ = r.Check(f)

	schemaAbs := filepath.Clean(filepath.Join(tmpDir, "schema.md"))
	fragmentAbs := filepath.Clean(filepath.Join(tmpDir, "fragment.md"))

	// The parsed-schema slot is populated and registers fragmentAbs as
	// an include — a sentinel call must hit the cache.
	var rebuildsBefore int32
	_ = cache.ParsedSchema(schemaAbs, func() any {
		atomic.AddInt32(&rebuildsBefore, 1)
		return schemaParseResult{schema: nil, err: assert.AnError}
	})
	require.Equal(t, int32(0), atomic.LoadInt32(&rebuildsBefore),
		"baseline: after Rule.Check the ParsedSchema slot must be populated")

	// Edit-then-invalidate the fragment. The schema's ParsedSchema
	// slot must be evicted because the parse reached the fragment.
	cache.Invalidate(fragmentAbs)

	var rebuildsAfter int32
	_ = cache.ParsedSchema(schemaAbs, func() any {
		atomic.AddInt32(&rebuildsAfter, 1)
		return schemaParseResult{schema: nil, err: assert.AnError}
	})
	assert.Equal(t, int32(1), atomic.LoadInt32(&rebuildsAfter),
		"Invalidate(fragmentAbs) must evict the schema's ParsedSchema slot "+
			"because the parse reached fragmentAbs via <?include?>")
}

// writeFile is a thin t.TempDir-friendly write helper. Kept local
// to the test file so the production code does not gain a new
// dependency just for the integration test.
func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o600)
}
