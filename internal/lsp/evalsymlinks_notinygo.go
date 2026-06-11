//go:build !tinygo

package lsp

import "path/filepath"

// evalSymlinks resolves symlinks in path. On non-tinygo builds this calls
// filepath.EvalSymlinks.
func evalSymlinks(path string) (string, error) {
	return filepath.EvalSymlinks(path)
}
