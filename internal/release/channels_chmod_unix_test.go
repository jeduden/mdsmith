//go:build !windows

package release

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests drive WriteChannelsData's MkdirAll and WriteFile
// error branches by chmod-ing intermediate paths read-only. They
// mirror coverage_chmod_unix_test.go and live in a Unix-tagged
// file because the chmod permission semantics are not portable to
// Windows and root bypasses them.

func TestWriteChannelsData_MkdirError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("chmod-based readonly test is unreliable as root")
	}
	root := t.TempDir()
	// Make website/ read-only so MkdirAll(website/data) fails while
	// ReadFile of the absent leaf still reports NotExist.
	web := filepath.Join(root, "website")
	require.NoError(t, os.MkdirAll(web, 0o755))
	require.NoError(t, os.Chmod(web, 0o555))
	t.Cleanup(func() { _ = os.Chmod(web, 0o755) })
	_, err := WriteChannelsData(root, fixtureChannels())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mkdir website data")
}

func TestWriteChannelsData_WriteFileError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("chmod-based readonly test is unreliable as root")
	}
	root := t.TempDir()
	path := filepath.Join(root, filepath.FromSlash(ChannelsDataFile))
	// Pre-create the data file read-only so ReadFile succeeds with
	// stale content (≠ the generated output) and the WriteFile
	// below cannot overwrite it.
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte("stale\n"), 0o444))
	_, err := WriteChannelsData(root, fixtureChannels())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "write")
}
