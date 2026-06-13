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
	"sync"

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

// validPatterns caches doublestar.ValidatePattern verdicts. Match runs
// once per pattern per file on the check hot path (ignore lists, kind
// path-patterns, overrides), and doublestar.Match re-validates its
// pattern on every call — the validation walk showed up as ~4% of check
// CPU. Patterns reaching this package come from bounded config
// surfaces, so the cache stays small for the life of the process.
var validPatterns sync.Map // pattern string -> bool

// patternValid reports whether pattern is a valid doublestar pattern,
// memoizing the verdict. LoadOrStore collapses the miss path to one
// map op and keeps concurrent first-touch callers from each storing
// their own (identical) verdict.
func patternValid(pattern string) bool {
	if v, ok := validPatterns.Load(pattern); ok {
		return v.(bool)
	}
	actual, _ := validPatterns.LoadOrStore(pattern, doublestar.ValidatePattern(pattern))
	return actual.(bool)
}

// Match reports whether path matches pattern using the doublestar matcher.
// It checks the raw path, the cleaned path, and the base name so that
// patterns without path separators (e.g. "slides.md") match files in any
// directory.
// Invalid patterns return false.
func Match(pattern, path string) bool {
	pat := filepath.ToSlash(pattern)
	if !patternValid(pat) {
		return false
	}
	if doublestar.MatchUnvalidated(pat, filepath.ToSlash(path)) {
		return true
	}
	// Identical candidates cannot change the verdict, so the cleaned
	// path and base name only run when they differ from the raw path.
	if cleanPath := filepath.Clean(path); cleanPath != path &&
		doublestar.MatchUnvalidated(pat, filepath.ToSlash(cleanPath)) {
		return true
	}
	if base := filepath.Base(path); base != path &&
		doublestar.MatchUnvalidated(pat, filepath.ToSlash(base)) {
		return true
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
