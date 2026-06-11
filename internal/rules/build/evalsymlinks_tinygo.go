//go:build tinygo

package build

// evalSymlinks is a no-op identity function on tinygo/wasm builds. The wasm
// sandbox has no symlinks; returning the input path unchanged is correct there.
func evalSymlinks(path string) (string, error) {
	return path, nil
}
