//go:build !tinygo

package lint_test

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/require"
)

func TestOpenRootFS_NonExistentDir(t *testing.T) {
	// When os.OpenRoot fails (dir does not exist), OpenRootFS returns an
	// openRootErrFS that propagates the error on every Open call rather
	// than panicking at construction time.
	fsys := lint.OpenRootFS(t.TempDir() + "/does-not-exist")
	_, err := fsys.Open("any.md")
	require.Error(t, err, "Open on an openRootErrFS must return the construction error")
}
