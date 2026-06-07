//go:build wasm

package githooks

import "os"

// chmodFn is a no-op under WASM: tinygo's wasm runtime omits os.Chmod,
// and a WASM host never installs git hooks (those are an on-disk,
// native-CLI operation). See docs/background/concepts/engine-api.md.
var chmodFn = func(_ string, _ os.FileMode) error { return nil }

// sameFile is a no-op under WASM that reports the two FileInfos as the
// same file. tinygo's wasm runtime omits os.SameFile; the
// atomicWriteGitattributes path this guards is never reached on a WASM
// host. See docs/background/concepts/engine-api.md.
func sameFile(_, _ os.FileInfo) bool { return true }
