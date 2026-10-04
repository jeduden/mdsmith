//go:build !unix

package lsp

import "io/fs"

// registryDirOwned is the non-unix half of the registry ownership
// check. Windows, plan9 and js/wasm expose no portable owner uid on a
// FileInfo, so only the symlink and directory checks in
// registryDirSafe apply there; the registry dir sits in the per-user
// cache or temp dir on those platforms.
func registryDirOwned(fs.FileInfo) bool { return true }
