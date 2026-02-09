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

// ResolveFiles takes positional arguments and returns deduplicated, sorted
// markdown file paths. It supports individual files, directories (recursive
// *.md and *.markdown), and glob patterns. Returns an error for nonexistent
// paths (that are not glob patterns).
func ResolveFiles(args []string) ([]string, error) {
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
		if hasGlobChars(arg) {
			// Expand glob pattern.
			matches, err := filepath.Glob(arg)
			if err != nil {
				return nil, fmt.Errorf("invalid glob pattern %q: %w", arg, err)
			}
			for _, m := range matches {
				info, err := os.Stat(m)
				if err != nil {
					continue
				}
				if info.IsDir() {
					dirFiles, err := walkDir(m)
					if err != nil {
						return nil, err
					}
					for _, f := range dirFiles {
						addFile(f)
					}
				} else if isMarkdown(m) {
					addFile(m)
				}
			}
			continue
		}

		info, err := os.Stat(arg)
		if err != nil {
			return nil, fmt.Errorf("cannot access %q: %w", arg, err)
		}

		if info.IsDir() {
			dirFiles, err := walkDir(arg)
			if err != nil {
				return nil, err
			}
			for _, f := range dirFiles {
				addFile(f)
			}
		} else {
			addFile(arg)
		}
	}

	sort.Strings(result)
	return result, nil
}

// walkDir recursively walks a directory and returns all markdown files.
func walkDir(dir string) ([]string, error) {
	var files []string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
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
