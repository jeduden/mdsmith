// Package mdpath is the single source of truth for the set of file
// extensions mdsmith treats as Markdown, together with the path
// predicates and glob builders derived from that set.
//
// Default file discovery (config.DefaultFiles), the CLI directory
// walk, the merge-driver include set (githooks.DefaultIncludes), the
// merge-driver discovery walk, wikilink resolution, the link-style
// rule, and the LSP document selector all derive from Extensions here,
// so teaching mdsmith a new extension (say ".mdx") is a one-line change
// in this file rather than a hunt across a dozen packages.
package mdpath

import (
	"slices"
	"strings"
)

// extensions is the canonical, lowercased Markdown extension set — each
// element including the leading dot. It is unexported so callers cannot
// mutate the shared slice; read a copy through Extensions, or test
// membership through HasMarkdownExt / IsMarkdownPath.
var extensions = []string{".md", ".markdown"}

// Extensions returns a fresh copy of the canonical Markdown extension
// list (each element includes the leading dot, lowercased), so callers
// may sort or append without disturbing the shared source of truth.
func Extensions() []string {
	return slices.Clone(extensions)
}

// FileGlobs returns a basename glob per Markdown extension — "*.md",
// "*.markdown". Used for the merge-driver include set, where each
// pattern is matched against a file's name at any depth.
func FileGlobs() []string {
	out := make([]string, len(extensions))
	for i, ext := range extensions {
		out[i] = "*" + ext
	}
	return out
}

// RecursiveGlobs returns a recursive doublestar glob per Markdown
// extension — "**/*.md", "**/*.markdown". Used for default file
// discovery (config.DefaultFiles) and the LSP document selector.
func RecursiveGlobs() []string {
	out := make([]string, len(extensions))
	for i, ext := range extensions {
		out[i] = "**/*" + ext
	}
	return out
}

// HasMarkdownExt reports whether ext — a file extension including the
// leading dot, as returned by path.Ext or filepath.Ext — is one of the
// recognised Markdown extensions, compared case-insensitively. It does
// not allocate: strings.EqualFold folds case without the copy that
// strings.ToLower would make.
func HasMarkdownExt(ext string) bool {
	for _, e := range extensions {
		if strings.EqualFold(ext, e) {
			return true
		}
	}
	return false
}

// IsMarkdownPath reports whether p has a Markdown extension (one of
// Extensions, case-insensitively).
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
// as the extension. The scan below finds the last '.' after the last
// occurrence of either separator, independent of the host OS.
func IsMarkdownPath(p string) bool {
	base := p
	if i := strings.LastIndexAny(p, `/\`); i >= 0 {
		base = p[i+1:]
	}
	dot := strings.LastIndexByte(base, '.')
	if dot < 0 {
		return false
	}
	return HasMarkdownExt(base[dot:])
}
