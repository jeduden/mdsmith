package rootfstest

import (
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jeduden/mdsmith/internal/lint"
)

// TestRecord locks that Record opens through the opener it replaced,
// records each root lint.OpenRootFS hands out, and restores the opener
// at cleanup.
func TestRecord(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.md"), []byte("# A\n"), 0o644))
	t.Run("records", func(t *testing.T) {
		opened := Record(t)
		r := lint.OpenRootFS(dir)
		got := opened()
		require.Len(t, got, 1)
		assert.Same(t, r, got[0])
		_, err := fs.Stat(r, "a.md")
		assert.NoError(t, err, "the root opens through the previous opener")
		assert.NoError(t, r.Close())
	})
	r := lint.OpenRootFS(dir)
	defer func() { _ = r.Close() }()
	_, err := fs.Stat(r, "a.md")
	assert.NoError(t, err, "the restored opener still opens")
}

// TestRecordConcurrentOpens locks that roots opened from several
// goroutines at once, as the engine's parallel workers do, are each
// recorded.
func TestRecordConcurrentOpens(t *testing.T) {
	dir := t.TempDir()
	opened := Record(t)
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = lint.OpenRootFS(dir).Close()
		}()
	}
	wg.Wait()
	assert.Len(t, opened(), 8)
}
