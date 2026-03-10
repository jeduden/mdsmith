package config

import (
	"path/filepath"

	"github.com/gobwas/glob"
)

// globMatchAny returns true if filePath matches any of the given glob
// patterns. It checks the raw path, the cleaned path, and the base name
// so that patterns without path separators (e.g. "slides.md") match files
// in any directory.
func globMatchAny(patterns []string, filePath string) bool {
	cleanPath := filepath.Clean(filePath)
	base := filepath.Base(filePath)

	for _, pattern := range patterns {
		g, err := glob.Compile(pattern)
		if err != nil {
			continue
		}
		if g.Match(filePath) || g.Match(cleanPath) || g.Match(base) {
			return true
		}
	}
	return false
}

// IsIgnored returns true if the file path matches any of the given
// ignore patterns. It checks the raw path, the cleaned path, and
// the base name.
func IsIgnored(patterns []string, path string) bool {
	return globMatchAny(patterns, path)
}
