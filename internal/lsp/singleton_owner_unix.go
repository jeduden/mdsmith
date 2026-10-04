//go:build unix

package lsp

import (
	"io/fs"
	"os"
	"syscall"
)

// registryDirOwned reports whether info, an Lstat of a registry
// directory or its parent, belongs to the current user and is not
// world-writable. On the shared temp-dir fallback another local user
// could otherwise own (or be able to rewrite) the path prune walks.
func registryDirOwned(info fs.FileInfo) bool {
	return dirOwnedBy(info, os.Getuid()) && info.Mode().Perm()&0o002 == 0
}

// dirOwnedBy reports whether info's owner is uid. A FileInfo without
// a *syscall.Stat_t cannot prove ownership, so it reports false.
func dirOwnedBy(info fs.FileInfo, uid int) bool {
	st, ok := info.Sys().(*syscall.Stat_t)
	return ok && int(st.Uid) == uid
}
