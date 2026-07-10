// Package mdpath provides path predicates shared across rule packages.
package mdpath

import "strings"

// IsMarkdownPath reports whether p has a Markdown extension (.md or
// .markdown, case-insensitively).
//
// Callers pass paths of two different shapes: fs.WalkDir-sourced paths
// are always forward-slash regardless of host OS, while a link target
// run through filepath.FromSlash carries the host's native separator
// (backslash on Windows). Neither path/filepath.Ext nor path.Ext alone
// is correct for both: filepath.Ext treats '\' as a separator only on
// Windows, so a Linux build would search past a backslash into an
// earlier path segment; path.Ext never treats '\' as a separator at
// all, so a Windows-native path with a dot in an earlier directory
// segment (e.g. `some.dir\README`) would misdetect that segment's dot
// as the extension. ext finds the last '.' after the last occurrence
// of either separator, independent of the host OS.
func IsMarkdownPath(p string) bool {
	base := p
	if i := strings.LastIndexAny(p, `/\`); i >= 0 {
		base = p[i+1:]
	}
	dot := strings.LastIndexByte(base, '.')
	if dot < 0 {
		return false
	}
	ext := base[dot:]
	return strings.EqualFold(ext, ".md") || strings.EqualFold(ext, ".markdown")
}
