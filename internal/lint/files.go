package lint

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gobwas/glob"
)

// isMarkdown returns true if the file extension is .md or .markdown.
func isMarkdown(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".md" || ext == ".markdown"
}

// matchesGlob returns true if path matches any of the given glob patterns.
func matchesGlob(patterns []string, path string) bool {
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

// hasGlobChars returns true if the string contains glob meta-characters.
func hasGlobChars(s string) bool {
	return strings.ContainsAny(s, "*?[")
}

// ResolveOpts controls how file resolution behaves.
type ResolveOpts struct {
	// UseGitignore enables filtering of walked directories by .gitignore
	// rules. When true (the default), files matched by .gitignore patterns
	// are skipped during directory walking. Explicitly named file paths are
	// never filtered by gitignore. Defaults to true when the zero value is
	// used (see DefaultResolveOpts).
	UseGitignore *bool

	// NoFollowSymlinks is a list of glob patterns. Symbolic links
	// whose path matches any pattern are skipped during directory
	// walking and glob expansion.
	NoFollowSymlinks []string
}

// DefaultResolveOpts returns options with defaults applied.
func DefaultResolveOpts() ResolveOpts {
	t := true
	return ResolveOpts{UseGitignore: &t}
}

// useGitignore returns whether gitignore filtering is enabled.
func (o ResolveOpts) useGitignore() bool {
	if o.UseGitignore == nil {
		return true // default
	}
	return *o.UseGitignore
}

// ResolveFiles takes positional arguments and returns deduplicated, sorted
// markdown file paths. It supports individual files, directories (recursive
// *.md and *.markdown), and glob patterns. Returns an error for nonexistent
// paths (that are not glob patterns).
// By default, directory walking respects .gitignore files.
func ResolveFiles(args []string) ([]string, error) {
	return ResolveFilesWithOpts(args, DefaultResolveOpts())
}

// ResolveFilesWithOpts is like ResolveFiles but accepts options to control
// behavior such as gitignore filtering.
func ResolveFilesWithOpts(args []string, opts ResolveOpts) ([]string, error) {
	seen := make(map[string]bool)
	var result []string

	addFile := func(path string) {
		abs, err := filepath.Abs(path)
		if err != nil {
			abs = path
		}
		if !seen[abs] {
			seen[abs] = true
			result = append(result, path)
		}
	}

	for _, arg := range args {
		if err := resolveArg(arg, opts, addFile); err != nil {
			return nil, err
		}
	}

	sort.Strings(result)
	return result, nil
}

// resolveArg resolves a single argument (glob, directory, or file) and calls
// addFile for each markdown file found.
func resolveArg(arg string, opts ResolveOpts, addFile func(string)) error {
	if hasGlobChars(arg) {
		return resolveGlob(arg, opts, addFile)
	}

	info, err := os.Stat(arg)
	if err != nil {
		return fmt.Errorf("cannot access %q: %w", arg, err)
	}

	if info.IsDir() {
		return addDirFiles(arg, opts, addFile)
	}

	// Explicitly named files are never filtered by gitignore.
	addFile(arg)
	return nil
}

// resolveGlob expands a glob pattern and adds matching markdown files.
func resolveGlob(pattern string, opts ResolveOpts, addFile func(string)) error {
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return fmt.Errorf("invalid glob pattern %q: %w", pattern, err)
	}
	for _, m := range matches {
		// Skip symlinks matching no-follow-symlinks patterns.
		if len(opts.NoFollowSymlinks) > 0 {
			linfo, lerr := os.Lstat(m)
			if lerr == nil && linfo.Mode()&os.ModeSymlink != 0 {
				if matchesGlob(opts.NoFollowSymlinks, m) {
					continue
				}
			}
		}
		info, err := os.Stat(m)
		if err != nil {
			continue
		}
		if info.IsDir() {
			if err := addDirFiles(m, opts, addFile); err != nil {
				return err
			}
		} else if isMarkdown(m) {
			addFile(m)
		}
	}
	return nil
}

// addDirFiles walks a directory and adds all markdown files found.
func addDirFiles(dir string, opts ResolveOpts, addFile func(string)) error {
	dirFiles, err := walkDir(dir, opts.useGitignore(), opts.NoFollowSymlinks)
	if err != nil {
		return err
	}
	for _, f := range dirFiles {
		addFile(f)
	}
	return nil
}

// isSkippedSymlink reports whether path should be skipped because it is
// a symlink matching one of the no-follow-symlinks patterns.
func isSkippedSymlink(info os.FileInfo, path string, patterns []string) bool {
	if len(patterns) == 0 {
		return false
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return false
	}
	return matchesGlob(patterns, path)
}

// walkDir recursively walks a directory and returns all markdown files.
// When useGitignore is true, files matched by .gitignore patterns are skipped.
// Symlinks whose path matches a noFollowSymlinks pattern are skipped.
func walkDir(dir string, useGitignore bool, noFollowSymlinks []string) ([]string, error) {
	var matcher *GitignoreMatcher
	if useGitignore {
		matcher = NewGitignoreMatcher(dir)
	}

	var files []string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if isSkippedSymlink(info, path, noFollowSymlinks) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if matcher != nil && isGitignored(matcher, path, info.IsDir()) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if !info.IsDir() && isMarkdown(path) {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walking directory %q: %w", dir, err)
	}
	return files, nil
}

// isGitignored checks if a path is ignored by gitignore rules.
func isGitignored(matcher *GitignoreMatcher, path string, isDir bool) bool {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	return matcher.IsIgnored(absPath, isDir)
}
