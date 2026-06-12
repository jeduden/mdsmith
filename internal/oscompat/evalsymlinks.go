//go:build !tinygo

package oscompat

import "path/filepath"

// EvalSymlinks resolves symlinks in path. It wraps filepath.EvalSymlinks on
// standard builds.
func EvalSymlinks(path string) (string, error) {
	return filepath.EvalSymlinks(path)
}
