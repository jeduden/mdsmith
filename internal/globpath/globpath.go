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
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

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
