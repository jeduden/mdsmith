package bytelimit_test

import (
	"math"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/jeduden/mdsmith/internal/bytelimit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultMaxInputBytes(t *testing.T) {
	assert.Equal(t, int64(2*1024*1024), bytelimit.DefaultMaxInputBytes)
}

func TestReadFileLimited_Normal(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "small.md")
	require.NoError(t, os.WriteFile(p, []byte("hello"), 0o644))

	data, err := bytelimit.ReadFileLimited(p, 100)
	require.NoError(t, err)
	assert.Equal(t, []byte("hello"), data)
}

func TestReadFileLimited_AtLimit(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "exact.md")
	content := make([]byte, 50)
	for i := range content {
		content[i] = 'x'
	}
	require.NoError(t, os.WriteFile(p, content, 0o644))

	data, err := bytelimit.ReadFileLimited(p, 50)
	require.NoError(t, err)
	assert.Equal(t, content, data)
}

func TestReadFileLimited_OverLimit(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "huge.md")
	content := make([]byte, 100)
	require.NoError(t, os.WriteFile(p, content, 0o644))

	_, err := bytelimit.ReadFileLimited(p, 50)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "file too large")
	assert.Contains(t, err.Error(), "100 bytes")
	assert.Contains(t, err.Error(), "max 50")
}

func TestReadFileLimited_ZeroUnlimited(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "big.md")
	content := make([]byte, 10000)
	require.NoError(t, os.WriteFile(p, content, 0o644))

	data, err := bytelimit.ReadFileLimited(p, 0)
	require.NoError(t, err)
	assert.Len(t, data, 10000)
}

func TestReadFileLimited_NegativeUnlimited(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "big.md")
	content := make([]byte, 10000)
	require.NoError(t, os.WriteFile(p, content, 0o644))

	data, err := bytelimit.ReadFileLimited(p, -1)
	require.NoError(t, err)
	assert.Len(t, data, 10000)
}

func TestReadFileLimited_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "empty.md")
	require.NoError(t, os.WriteFile(p, nil, 0o644))

	data, err := bytelimit.ReadFileLimited(p, 100)
	require.NoError(t, err)
	assert.Empty(t, data)
}

func TestReadFileLimited_NotFound(t *testing.T) {
	_, err := bytelimit.ReadFileLimited("/nonexistent/file.md", 100)
	require.Error(t, err)
}

func TestReadFileLimited_PreSizesBuffer(t *testing.T) {
	// A large in-cap file should land in a single stat-sized buffer
	// rather than io.ReadAll's repeated grow-and-copy. With a 1 MB file
	// ReadAll reallocates ~11 times; a pre-sized read is open+stat+one
	// buffer. Bound the allocations between the two so a regression to
	// ReadAll trips this test.
	dir := t.TempDir()
	p := filepath.Join(dir, "big.md")
	content := make([]byte, 1024*1024)
	for i := range content {
		content[i] = byte('a' + i%26)
	}
	require.NoError(t, os.WriteFile(p, content, 0o644))

	var gotLen int
	var gotErr error
	allocs := testing.AllocsPerRun(20, func() {
		data, err := bytelimit.ReadFileLimited(p, 2*1024*1024)
		gotLen, gotErr = len(data), err
	})
	require.NoError(t, gotErr)
	require.Equal(t, len(content), gotLen)
	assert.LessOrEqualf(t, allocs, float64(6),
		"expected a pre-sized read; got %v allocations (io.ReadAll regression?)", allocs)
}

func TestReadFileLimited_MaxInt64Unlimited(t *testing.T) {
	// MaxInt64 must be treated as unlimited to avoid overflow in max+1.
	dir := t.TempDir()
	p := filepath.Join(dir, "file.md")
	require.NoError(t, os.WriteFile(p, []byte("hello"), 0o644))

	data, err := bytelimit.ReadFileLimited(p, math.MaxInt64)
	require.NoError(t, err)
	assert.Equal(t, []byte("hello"), data)
}

func writeBytes(t *testing.T, dir, name string, n int, b byte) string {
	t.Helper()
	p := filepath.Join(dir, name)
	content := make([]byte, n)
	for i := range content {
		content[i] = b
	}
	require.NoError(t, os.WriteFile(p, content, 0o644))
	return p
}

func TestReadFileLimitedInto_InCap(t *testing.T) {
	// Buffer with ample capacity: no grow, no reallocation; returned
	// slice aliases the caller's backing array.
	dir := t.TempDir()
	p := writeBytes(t, dir, "small.md", 5, 'x')

	buf := make([]byte, 0, 1024)
	orig := buf[:1]
	data, err := bytelimit.ReadFileLimitedInto(p, &buf, 100)
	require.NoError(t, err)
	assert.Equal(t, []byte("xxxxx"), data)
	// No grow happened: buf still points at the original backing array.
	assert.Equal(t, cap(orig), cap(buf))
	assert.Same(t, &orig[:cap(orig)][0], &buf[:cap(buf)][0])
}

func TestReadFileLimitedInto_AtCap(t *testing.T) {
	dir := t.TempDir()
	p := writeBytes(t, dir, "exact.md", 50, 'y')

	buf := make([]byte, 0, 64)
	data, err := bytelimit.ReadFileLimitedInto(p, &buf, 50)
	require.NoError(t, err)
	assert.Len(t, data, 50)
}

func TestReadFileLimitedInto_OverCap(t *testing.T) {
	dir := t.TempDir()
	p := writeBytes(t, dir, "huge.md", 100, 'z')

	buf := make([]byte, 0, 16)
	_, err := bytelimit.ReadFileLimitedInto(p, &buf, 50)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "file too large")
	assert.Contains(t, err.Error(), "100 bytes")
	assert.Contains(t, err.Error(), "max 50")
}

func TestReadFileLimitedInto_GrowNeeded(t *testing.T) {
	// File fits within max but the buffer starts smaller than the file,
	// so the read must grow the buffer. The grown backing array is
	// written back through buf for reuse.
	dir := t.TempDir()
	p := writeBytes(t, dir, "grow.md", 500, 'g')

	buf := make([]byte, 0, 8)
	data, err := bytelimit.ReadFileLimitedInto(p, &buf, 2*1024*1024)
	require.NoError(t, err)
	assert.Len(t, data, 500)
	assert.GreaterOrEqual(t, cap(buf), 500, "buffer should have grown")
}

func TestReadFileLimitedInto_Reuse(t *testing.T) {
	// A second read into the same (now grown) buffer reuses the backing
	// array without reallocating.
	dir := t.TempDir()
	p1 := writeBytes(t, dir, "first.md", 1000, 'a')
	p2 := writeBytes(t, dir, "second.md", 200, 'b')

	buf := make([]byte, 0, 8)
	data1, err := bytelimit.ReadFileLimitedInto(p1, &buf, 2*1024*1024)
	require.NoError(t, err)
	require.Len(t, data1, 1000)
	capAfterFirst := cap(buf)
	require.GreaterOrEqual(t, capAfterFirst, 1000)

	// Second, smaller read fits in the existing capacity → no realloc.
	data2, err := bytelimit.ReadFileLimitedInto(p2, &buf, 2*1024*1024)
	require.NoError(t, err)
	assert.Len(t, data2, 200)
	assert.Equal(t, capAfterFirst, cap(buf), "second read should reuse the buffer")
	for _, c := range data2 {
		assert.Equal(t, byte('b'), c)
	}
}

func TestReadFileLimitedInto_ReuseNoAlloc(t *testing.T) {
	// With a pre-grown buffer, a steady-state read allocates nothing for
	// the source buffer itself (open+stat may allocate, but not the
	// data slice). Pin a low alloc count so a regression to a fresh
	// per-read allocation trips this test.
	dir := t.TempDir()
	p := writeBytes(t, dir, "steady.md", 4096, 'r')

	buf := make([]byte, 0, 8192) // pre-grown past the file size
	var gotLen int
	var gotErr error
	allocs := testing.AllocsPerRun(50, func() {
		data, err := bytelimit.ReadFileLimitedInto(p, &buf, 2*1024*1024)
		gotLen, gotErr = len(data), err
	})
	require.NoError(t, gotErr)
	require.Equal(t, 4096, gotLen)
	// os.Open + Stat have a small fixed allocation cost; the point is
	// that the source data buffer is NOT reallocated each call. A 4 KB
	// fresh-allocation regression would push this well past the bound.
	assert.LessOrEqualf(t, allocs, float64(5),
		"steady-state read should not allocate a fresh source buffer; got %v", allocs)
}

func TestReadFileLimitedInto_Unlimited(t *testing.T) {
	dir := t.TempDir()
	p := writeBytes(t, dir, "big.md", 10000, 'u')

	buf := make([]byte, 0, 8)
	data, err := bytelimit.ReadFileLimitedInto(p, &buf, 0)
	require.NoError(t, err)
	assert.Len(t, data, 10000)
}

func TestReadFileLimitedInto_MaxInt64Unlimited(t *testing.T) {
	dir := t.TempDir()
	p := writeBytes(t, dir, "file.md", 5, 'm')

	buf := make([]byte, 0, 64)
	data, err := bytelimit.ReadFileLimitedInto(p, &buf, math.MaxInt64)
	require.NoError(t, err)
	assert.Len(t, data, 5)
}

func TestReadFileLimitedInto_NotFound(t *testing.T) {
	buf := make([]byte, 0, 8)
	_, err := bytelimit.ReadFileLimitedInto("/nonexistent/file.md", &buf, 100)
	require.Error(t, err)
}

func TestReadFileLimitedInto_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "empty.md")
	require.NoError(t, os.WriteFile(p, nil, 0o644))

	buf := make([]byte, 0, 64)
	data, err := bytelimit.ReadFileLimitedInto(p, &buf, 100)
	require.NoError(t, err)
	assert.Empty(t, data)
}

func TestReadFileLimitedInto_NilBuffer(t *testing.T) {
	// A nil/zero-cap buffer is valid: the read seeds capacity and writes
	// it back through buf.
	dir := t.TempDir()
	p := writeBytes(t, dir, "nil.md", 42, 'n')

	var buf []byte
	data, err := bytelimit.ReadFileLimitedInto(p, &buf, 100)
	require.NoError(t, err)
	assert.Len(t, data, 42)
	assert.GreaterOrEqual(t, cap(buf), 42)
}

func TestReadFSFileLimited_MaxInt64Unlimited(t *testing.T) {
	fsys := fstest.MapFS{
		"test.md": &fstest.MapFile{Data: []byte("hello")},
	}
	data, err := bytelimit.ReadFSFileLimited(fsys, "test.md", math.MaxInt64)
	require.NoError(t, err)
	assert.Equal(t, []byte("hello"), data)
}

func TestReadFSFileLimited_Normal(t *testing.T) {
	fsys := fstest.MapFS{
		"test.md": &fstest.MapFile{Data: []byte("hello")},
	}
	data, err := bytelimit.ReadFSFileLimited(fsys, "test.md", 100)
	require.NoError(t, err)
	assert.Equal(t, []byte("hello"), data)
}

func TestReadFSFileLimited_OverLimit(t *testing.T) {
	content := make([]byte, 100)
	fsys := fstest.MapFS{
		"big.md": &fstest.MapFile{Data: content},
	}
	_, err := bytelimit.ReadFSFileLimited(fsys, "big.md", 50)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "file too large")
}

func TestReadFSFileLimited_ZeroUnlimited(t *testing.T) {
	content := make([]byte, 10000)
	fsys := fstest.MapFS{
		"big.md": &fstest.MapFile{Data: content},
	}
	data, err := bytelimit.ReadFSFileLimited(fsys, "big.md", 0)
	require.NoError(t, err)
	assert.Len(t, data, 10000)
}

func TestReadFSFileLimited_AtLimit(t *testing.T) {
	content := make([]byte, 50)
	for i := range content {
		content[i] = 'a'
	}
	fsys := fstest.MapFS{
		"exact.md": &fstest.MapFile{Data: content},
	}
	data, err := bytelimit.ReadFSFileLimited(fsys, "exact.md", 50)
	require.NoError(t, err)
	assert.Equal(t, content, data)
}

func TestReadFSFileLimited_Nonexistent(t *testing.T) {
	fsys := fstest.MapFS{}
	_, err := bytelimit.ReadFSFileLimited(fsys, "no-such.md", 100)
	require.Error(t, err)
}
