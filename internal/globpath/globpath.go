// Package globpath provides glob matching and pattern utilities for mdsmith
// config surfaces: ignore:, overrides:, kind-assignment:, and rule settings
// (allowed:, include:, exclude:, budgets[].glob).
//
// The catalog directive uses SplitIncludeExclude from this package to split
// !-prefixed exclusion patterns; include resolution and exclude matching in
// the catalog use doublestar directly with full-path semantics.
//
// CLI argument expansion uses doublestar.FilepathGlob directly and does not
// route through this package; !-prefix exclusion is not available on the CLI.
package globpath

import (
	"path"
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

// ResolveAgainstRoot resolves p against baseDir (both slash-separated and
// relative to a project root) and reports whether the cleaned result
// escapes the project root via ".." segments.
//
// baseDir == "" or "." is interpreted as the project root. The returned
// path is also slash-separated and relative to the project root; it is
// "" for the root itself.
//
// Callers use this to check that ".." segments in a path stay within the
// project root before resolving the path against a project-rooted fs.FS.
func ResolveAgainstRoot(baseDir, p2 string) (resolved string, escapes bool) {
	if baseDir == "" {
		baseDir = "."
	}
	cleaned := path.Clean(path.Join(baseDir, p2))
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return cleaned, true
	}
	if cleaned == "." {
		cleaned = ""
	}
	return cleaned, false
}

// ContainsDotDotSegment reports whether p contains a ".." path element
// when split on "/". Filenames like "..foo" do not match.
// Uses zero-allocation string checks instead of strings.Split.
func ContainsDotDotSegment(p string) bool {
	return p == ".." ||
		strings.HasPrefix(p, "../") ||
		strings.HasSuffix(p, "/..") ||
		strings.Contains(p, "/../")
}

// Match reports whether path matches pattern using the doublestar matcher.
// It checks the raw path, the cleaned path, and the base name so that
// patterns without path separators (e.g. "slides.md") match files in any
// directory.
// Invalid patterns return false.
func Match(pattern, path string) bool {
	cleanPath := filepath.Clean(path)
	base := filepath.Base(path)

	for _, candidate := range []string{path, cleanPath, base} {
		if ok, err := doublestar.Match(
			filepath.ToSlash(pattern),
			filepath.ToSlash(candidate),
		); err == nil && ok {
			return true
		}
	}
	return false
}

// MatchAny reports whether path matches any of the given patterns.
// A pattern prefixed with "!" is an exclusion pattern. The path matches
// when at least one non-negated pattern matches and no exclusion pattern
// matches; an exclusion pattern always wins over an inclusion pattern,
// regardless of list order. A list containing only exclusion patterns
// matches nothing.
func MatchAny(patterns []string, path string) bool {
	matchedInclude := false
	for _, pattern := range patterns {
		isExclude := strings.HasPrefix(pattern, "!")
		if isExclude {
			pattern = pattern[1:]
		}
		if !Match(pattern, path) {
			continue
		}
		if isExclude {
			return false
		}
		matchedInclude = true
	}
	return matchedInclude
}

// SplitIncludeExclude separates patterns into include and exclude lists.
// Patterns prefixed with "!" are exclusion patterns (the prefix is stripped).
func SplitIncludeExclude(patterns []string) (include, exclude []string) {
	for _, p := range patterns {
		if strings.HasPrefix(p, "!") {
			exclude = append(exclude, p[1:])
		} else {
			include = append(include, p)
		}
	}
	return include, exclude
}
