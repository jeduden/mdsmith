//go:build tinygo

package oscompat

// EvalSymlinks is a no-op identity function on tinygo/wasm builds. The wasm
// sandbox has no symlinks; returning the input path unchanged is correct there.
func EvalSymlinks(path string) (string, error) {
	return path, nil
}
