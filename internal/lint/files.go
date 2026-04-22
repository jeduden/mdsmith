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

	// FollowSymlinks opts in to following symbolic links that resolve
	// to files. The zero value skips all symlinks, which is the secure
	// default.
	//
	// Symlinked directories are always skipped, regardless of this
	// flag. `filepath.Walk` is Lstat-based and does not descend into
	// a symlink root, so supporting symlinked-directory traversal
	// would require explicit EvalSymlinks resolution plus atomic
	// writes against an unknown path — out of scope for plan 84.
	FollowSymlinks bool
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

	// Reject any path that traverses a symlinked directory. Intermediate
	// symlinked components would otherwise let a user-supplied path
	// like `linked/dirty.md` reach an external target, since Lstat
	// on the leaf follows the intermediate symlink during name
	// resolution. Symlinked directories are always skipped regardless
	// of FollowSymlinks, per the Option doc.
	if hasSymlinkAncestor(arg) {
		return nil
	}

	// Default-deny symlinks even for explicit, non-glob paths. Lstat
	// (not Stat) avoids following the link so that an attacker who
	// plants `evil.md -> /etc/cron.d/jobs` can't trick a user into
	// processing the target with `mdsmith check ./evil.md`.
	linfo, lerr := os.Lstat(arg)
	if lerr != nil {
		return fmt.Errorf("cannot access %q: %w", arg, lerr)
	}
	isSymlink := linfo.Mode()&os.ModeSymlink != 0
	if isSymlink && !opts.FollowSymlinks {
		return nil
	}

	info, err := os.Stat(arg)
	if err != nil {
		return fmt.Errorf("cannot access %q: %w", arg, err)
	}

	// Symlinks to directories are always skipped. filepath.Walk is
	// Lstat-based and cannot recurse into a symlink root, so walking
	// a symlinked dir would silently yield no files and confuse
	// callers. `--follow-symlinks` applies to file symlinks only.
	if isSymlink && info.IsDir() {
		return nil
	}

	if info.IsDir() {
		return addDirFiles(arg, opts, addFile)
	}

	// Explicitly named files are never filtered by gitignore.
	addFile(arg)
	return nil
}

// hasSymlinkAncestor reports whether any ancestor directory named
// by the relative components of path is a symbolic link. It rejects
// paths like `linked/dirty.md` where `linked` is a symlinked dir —
// the filesystem would resolve the link during `os.Stat(path)` and
// let the external target slip past a leaf-only Lstat check.
//
// Only relative path components are walked. Absolute paths and
// paths that escape via `..` are treated as the user's explicit
// choice: we do not probe above the invocation directory (that
// would misclassify environments where temp dirs or project roots
// live behind a system-wide symlink, e.g. `/tmp` on macOS).
func hasSymlinkAncestor(path string) bool {
	clean := filepath.Clean(path)
	if filepath.IsAbs(clean) || strings.HasPrefix(clean, "..") {
		return false
	}
	dir := filepath.Dir(clean)
	for dir != "." && dir != "/" {
		if strings.HasPrefix(dir, "..") {
			return false
		}
		info, err := os.Lstat(dir)
		if err == nil && info.Mode()&os.ModeSymlink != 0 {
			return true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return false
		}
		dir = parent
	}
	return false
}

// resolveGlob expands a glob pattern and adds matching markdown files.
func resolveGlob(pattern string, opts ResolveOpts, addFile func(string)) error {
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return fmt.Errorf("invalid glob pattern %q: %w", pattern, err)
	}
	for _, m := range matches {
		// filepath.Glob follows symlinked directory components
		// during expansion, so a pattern like `linked/*.md` will
		// return paths rooted under a symlinked dir. Reject any
		// match whose ancestor chain contains a symlink: symlinked
		// directories are always skipped.
		if hasSymlinkAncestor(m) {
			continue
		}
		linfo, lerr := os.Lstat(m)
		if lerr != nil {
			continue
		}
		isSymlink := linfo.Mode()&os.ModeSymlink != 0
		if isSymlink && !opts.FollowSymlinks {
			continue
		}
		info, err := os.Stat(m)
		if err != nil {
			continue
		}
		// Symlinks to directories are always skipped (see resolveArg).
		if isSymlink && info.IsDir() {
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
	dirFiles, err := walkDir(dir, opts.useGitignore(), opts.FollowSymlinks)
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
// Symlinks are skipped unless followSymlinks is true. filepath.Walk is
// Lstat-based, so symlinked directories encountered during the walk are
// never descended into either way.
func walkDir(dir string, useGitignore, followSymlinks bool) ([]string, error) {
	var matcher *GitignoreMatcher
	if useGitignore {
		matcher = NewGitignoreMatcher(dir)
	}

	var files []string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Symlink entries always have Lstat-based info with
		// IsDir()==false under filepath.Walk, so a plain return nil
		// here also means Walk won't try to descend.
		if info.Mode()&os.ModeSymlink != 0 {
			if !followSymlinks {
				return nil
			}
			// In opt-in mode, follow the link only if it resolves to
			// a regular file. A symlink-to-dir named "evil.md" would
			// otherwise be treated as a markdown file and fail on
			// read; this also matches the FollowSymlinks doc that
			// says symlinked directories are always skipped.
			if tgt, statErr := os.Stat(path); statErr != nil || tgt.IsDir() {
				return nil
			}
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
