// Package pathutil holds small, dependency-free predicates over
// forward-slash-normalized workspace-relative path strings. It exists
// so a generic string check like "is this path absolute" doesn't have
// to live inside a domain package (internal/backlinks, cmd/mdsmith)
// just because that package happened to need it first.
package pathutil

import (
	"path"
	"strings"
)

// IsAbsOrDriveOrUNC reports whether p is absolute under any of the
// schemes mdsmith targets: POSIX-style leading `/`, Windows drive
// letters like `C:/`, or UNC prefixes like `//host`. `path.IsAbs`
// alone misses the Windows forms because the path package is Unix-only.
//
// p must already have backslashes normalized to forward slashes (e.g.
// via filepath.ToSlash or strings.ReplaceAll(p, `\`, "/")) — this
// predicate only recognizes the forward-slash UNC prefix `//host`, not
// a raw `\\host`. Callers working with un-normalized, potentially
// native-OS paths (such as internal/lsp, which receives paths as-is
// from RPC clients) need their own check against the backslash form.
func IsAbsOrDriveOrUNC(p string) bool {
	if path.IsAbs(p) {
		return true
	}
	if len(p) >= 2 && p[1] == ':' {
		c := p[0]
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') {
			return true
		}
	}
	return strings.HasPrefix(p, "//")
}
