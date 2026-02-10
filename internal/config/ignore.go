package config

import (
	"path/filepath"

	"github.com/gobwas/glob"
)

// IsIgnored returns true if the file path matches any of the given
// ignore patterns. It checks the raw path, the cleaned path, and
// the base name.
func IsIgnored(patterns []string, path string) bool {
	cleanPath := filepath.Clean(path)

	for _, pattern := range patterns {
		g, err := glob.Compile(pattern)
		if err != nil {
			continue
		}
		if g.Match(path) || g.Match(cleanPath) || g.Match(filepath.Base(path)) {
			return true
		}
	}
	return false
}
