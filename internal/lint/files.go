package lint

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// isMarkdown returns true if the file extension is .md or .markdown.
func isMarkdown(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".md" || ext == ".markdown"
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
	dirFiles, err := walkDir(dir, opts.useGitignore())
	if err != nil {
		return err
	}
	for _, f := range dirFiles {
		addFile(f)
	}
	return nil
}

// walkDir recursively walks a directory and returns all markdown files.
// When useGitignore is true, files matched by .gitignore patterns are skipped.
func walkDir(dir string, useGitignore bool) ([]string, error) {
	var matcher *gitignoreMatcher
	if useGitignore {
		matcher = newGitignoreMatcher(dir)
	}

	var files []string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Check gitignore rules.
		if matcher != nil {
			absPath, absErr := filepath.Abs(path)
			if absErr == nil {
				if matcher.isIgnored(absPath, info.IsDir()) {
					if info.IsDir() {
						return filepath.SkipDir
					}
					return nil
				}
			}
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
