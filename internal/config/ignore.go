package config

import (
	"path/filepath"
	"strings"

	"github.com/gobwas/glob"
)

// globMatchAny returns true if filePath matches any of the given glob
// patterns. It checks the raw path, the cleaned path, and the base name
// so that patterns without path separators (e.g. "slides.md") match files
// in any directory.
//
// A pattern prefixed with "!" is an exclusion pattern. The path matches
// the list when at least one non-negated pattern matches and no
// exclusion pattern matches; an exclusion pattern always wins over an
// inclusion pattern, regardless of list order. A list containing only
// exclusion patterns matches nothing.
func globMatchAny(patterns []string, filePath string) bool {
	cleanPath := filepath.Clean(filePath)
	base := filepath.Base(filePath)

	matchedInclude := false
	for _, pattern := range patterns {
		isExclude := strings.HasPrefix(pattern, "!")
		if isExclude {
			pattern = pattern[1:]
		}
		g, err := glob.Compile(pattern)
		if err != nil {
			continue
		}
		if !g.Match(filePath) && !g.Match(cleanPath) && !g.Match(base) {
			continue
		}
		if isExclude {
			return false
		}
		matchedInclude = true
	}
	return matchedInclude
}

// IsIgnored returns true if the file path matches any of the given
// ignore patterns. It checks the raw path, the cleaned path, and
// the base name. A pattern prefixed with "!" excludes a path even if
// another ignore pattern also matches, regardless of list order.
func IsIgnored(patterns []string, path string) bool {
	return globMatchAny(patterns, path)
}
