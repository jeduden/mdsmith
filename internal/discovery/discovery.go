// Package discovery finds Markdown files by expanding glob patterns from config.
package discovery

import (
	"os"
	"path/filepath"
	"sort"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/jeduden/tidymark/internal/lint"
)

// Options controls how file discovery behaves.
type Options struct {
	// Patterns is the list of glob patterns to match files against.
	// An empty or nil list means no files are discovered.
	Patterns []string

	// BaseDir is the directory to walk from. Defaults to "." if empty.
	BaseDir string

	// UseGitignore enables filtering by .gitignore rules.
	UseGitignore bool
}

// Discover walks BaseDir and returns files matching any of the configured
// glob patterns. Results are deduplicated and sorted.
func Discover(opts Options) ([]string, error) {
	if len(opts.Patterns) == 0 {
		return nil, nil
	}

	baseDir := opts.BaseDir
	if baseDir == "" {
		baseDir = "."
	}

	absBase, err := filepath.Abs(baseDir)
	if err != nil {
		return nil, err
	}

	// Validate patterns.
	validPatterns := make([]string, 0, len(opts.Patterns))
	for _, p := range opts.Patterns {
		if doublestar.ValidatePattern(p) {
			validPatterns = append(validPatterns, p)
		}
	}
	if len(validPatterns) == 0 {
		return nil, nil
	}

	// Set up gitignore matcher if enabled.
	var gitMatcher *lint.GitignoreMatcher
	if opts.UseGitignore {
		gitMatcher = lint.NewGitignoreMatcher(baseDir)
	}

	seen := make(map[string]bool)
	var result []string

	err = filepath.Walk(absBase, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		// Compute path relative to base for pattern matching.
		rel, relErr := filepath.Rel(absBase, path)
		if relErr != nil {
			return nil
		}
		// Normalize to forward slashes for glob matching.
		rel = filepath.ToSlash(rel)

		// Skip the root directory itself.
		if rel == "." {
			return nil
		}

		// Apply gitignore filtering.
		if gitMatcher != nil {
			absPath, absErr := filepath.Abs(path)
			if absErr == nil && gitMatcher.IsIgnored(absPath, info.IsDir()) {
				if info.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
		}

		// Only match regular files.
		if info.IsDir() {
			return nil
		}

		// Check if the file matches any pattern.
		for _, p := range validPatterns {
			matched, matchErr := doublestar.Match(p, rel)
			if matchErr != nil {
				continue
			}
			if matched {
				absPath, absErr := filepath.Abs(path)
				if absErr != nil {
					absPath = path
				}
				if !seen[absPath] {
					seen[absPath] = true
					result = append(result, path)
				}
				break
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Strings(result)
	return result, nil
}
