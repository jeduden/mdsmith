//go:build !windows

package githooks

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteGitattributes_ReturnsErrorForUnreadableExistingFile(t *testing.T) {
	// Mode 0000 only blocks reads for non-root users; root bypasses
	// file permission bits, so this assertion can't hold under uid 0.
	if os.Geteuid() == 0 {
		t.Skip("file permission bits don't restrict root")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, ".gitattributes")

	err := os.WriteFile(path, []byte("test"), 0000)
	require.NoError(t, err)

	err = WriteGitattributes(path, Globs{Include: []string{"a.md"}})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "reading")
}
