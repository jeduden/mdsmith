// Package pathutil holds small, dependency-free predicates over
// workspace-relative path strings. It exists so a generic string
// check like "is this path absolute" doesn't have to live inside a
// domain package (internal/backlinks, internal/linkgraph,
// cmd/mdsmith) just because that package happened to need it first.
package pathutil

// IsAbsOrDriveOrUNC reports whether p is anchored somewhere other
// than the workspace root under any scheme mdsmith targets,
// whichever host it runs on:
//
//   - a leading `/` or `\` — a POSIX absolute path, a Windows
//     root-relative path, or a UNC prefix (`//host` or `\\host`);
//   - a Windows drive letter — `C:/`, `C:\`, or the drive-relative
//     `C:x`, which resolves against drive C's working directory.
//
// Backslashes count as separators on every host, matching
// index.NormalizePath and linkgraph.ResolveRelTarget, so callers need
// not normalize p first. `path.IsAbs` and `filepath.IsAbs` both miss
// the Windows forms on a POSIX host.
func IsAbsOrDriveOrUNC(p string) bool {
	if p == "" {
		return false
	}
	if p[0] == '/' || p[0] == '\\' {
		return true
	}
	if len(p) < 2 || p[1] != ':' {
		return false
	}
	c := p[0]
	return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')
}
