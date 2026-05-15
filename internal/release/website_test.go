package release

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildWebsite_RunsFixThenSync(t *testing.T) {
	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "out")
	writeFile(t, filepath.Join(src, "top.md"), "# top\n")
	rec := &recordingRunner{}

	require.NoError(t, NewWithDeps(osFS{}, rec).BuildWebsite(src, dst, true))

	require.Len(t, rec.calls, 1)
	assert.Equal(t, "go", rec.calls[0].name)
	assert.Equal(t, []string{"run", "./cmd/mdsmith", "fix", src}, rec.calls[0].args)
	assertFile(t, filepath.Join(dst, "top.md"), "# top\n")
}

func TestBuildWebsite_NoFixSkipsRunner(t *testing.T) {
	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "out")
	writeFile(t, filepath.Join(src, "top.md"), "# top\n")
	rec := &recordingRunner{}

	require.NoError(t, NewWithDeps(osFS{}, rec).BuildWebsite(src, dst, false))

	assert.Empty(t, rec.calls, "no-fix must not invoke the runner")
	assertFile(t, filepath.Join(dst, "top.md"), "# top\n")
}

func TestBuildWebsite_FixFailureWraps(t *testing.T) {
	src := t.TempDir()
	writeFile(t, filepath.Join(src, "top.md"), "# top\n")

	err := NewWithDeps(osFS{}, &fakeRunner{failOnCall: 1}).
		BuildWebsite(src, filepath.Join(t.TempDir(), "out"), true)

	require.Error(t, err)
	assert.ErrorIs(t, err, errInjected)
	assert.Contains(t, err.Error(), "mdsmith fix")
}

func TestBuildWebsite_SyncFailureWraps(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "x.md"), "x\n")

	// recordingRunner succeeds on fix; src==dst trips the
	// SyncDocs overlap guard so the wrapping path is exercised.
	err := NewWithDeps(osFS{}, &recordingRunner{}).BuildWebsite(dir, dir, true)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "sync")
	assert.Contains(t, err.Error(), "same path")
}
