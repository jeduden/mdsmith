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

	// FollowSymlinks opts in to following symbolic links that
	// resolve to regular files. The zero value skips all symlinks,
	// which is the secure default.
	//
	// Symlinks that resolve to anything other than a regular file
	// are always skipped, regardless of this flag: directories
	// (filepath.Walk is Lstat-based and does not descend a symlink
	// root) as well as FIFOs, devices, and sockets (reading them
	// during linting could block or fail unexpectedly).
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
		// Broken or inaccessible symlink targets are silently
		// skipped to match walkDir / resolveGlob / discovery
		// behavior and the FollowSymlinks contract. For a
		// non-symlink, the missing target is a real error.
		if isSymlink {
			return nil
		}
		return fmt.Errorf("cannot access %q: %w", arg, err)
	}

	// Symlinks are followed only when they resolve to a regular
	// file. Directory targets are skipped (filepath.Walk is
	// Lstat-based and cannot recurse into a symlink root); device,
	// FIFO, and socket targets are skipped to avoid blocking reads
	// later. `--follow-symlinks` applies to file symlinks only.
	if isSymlink && !info.Mode().IsRegular() {
		return nil
	}

	if info.IsDir() {
		return addDirFiles(arg, opts, addFile)
	}

	// Non-directory, non-symlink path: reject FIFO, device, and
	// socket entries even when explicitly named. Reading them via
	// the lint pipeline could block or error, and nothing markdown
	// ever lives behind them.
	if !isSymlink && !info.Mode().IsRegular() {
		return nil
	}

	// Explicitly named files are never filtered by gitignore.
	addFile(arg)
	return nil
}

// hasSymlinkAncestor reports whether any ancestor directory of
// path (up to, but excluding, the project boundary) is a symbolic
// link. It rejects paths like `linked/dirty.md` where `linked` is
// a symlinked dir — the filesystem would resolve the link during
// `os.Stat(path)` and let the external target slip past a
// leaf-only Lstat check.
//
// The project boundary is picked in this order:
//
//   - cwd, if the path is under it (so `mdsmith check .` from the
//     project root catches any symlinked dir inside);
//   - otherwise, the nearest ancestor of the path that contains a
//     `.git` entry (so an absolute path run from a sibling shell
//     is still scanned within the target project);
//   - otherwise, "" (trust the user — no scan).
//
// This keeps system-level symlinks above the boundary (e.g. `/tmp`
// on macOS) out of the probe.
func hasSymlinkAncestor(path string) bool {
	return hasSymlinkAncestorCached(path, make(map[string]bool))
}

// hasSymlinkAncestorCached is the memoized form of
// hasSymlinkAncestor. Callers that run the check repeatedly (e.g.
// per glob match) should share a `cache` across invocations to
// avoid repeated `os.Lstat` calls on the same ancestor directories.
func hasSymlinkAncestorCached(path string, cache map[string]bool) bool {
	cwd, _ := os.Getwd() // "" on error; handled downstream
	return hasSymlinkAncestorWithCwd(path, cwd, cache)
}

// hasSymlinkAncestorWithCwd is like hasSymlinkAncestorCached but
// takes a precomputed cwd. Per-glob-expansion callers should read
// `os.Getwd` once and pass it here for every match to avoid the
// syscall per entry.
func hasSymlinkAncestorWithCwd(path, cwd string, cache map[string]bool) bool {
	abs := absWithCwd(path, cwd)
	if abs == "" {
		return false
	}
	stop := ancestorStopBoundary(abs, cwd)
	if stop == "" {
		return false
	}
	return ancestorChainHasSymlink(filepath.Dir(abs), stop, cache)
}

// absWithCwd resolves path to an absolute, cleaned form using the
// caller-supplied cwd to avoid the per-call `os.Getwd` that
// `filepath.Abs` performs for relative paths. When cwd is empty
// (e.g. a Getwd error upstream), it falls back to `filepath.Abs`.
// Returns "" only on the unreachable `filepath.Abs` failure path.
func absWithCwd(path, cwd string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	if cwd != "" {
		return filepath.Clean(filepath.Join(cwd, path))
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return ""
	}
	return filepath.Clean(abs)
}

// ancestorStopBoundary returns the directory at which ancestor
// probing should stop (exclusive), or "" if the path should not be
// scanned. Both relative and absolute forms are resolved to
// absolute paths before calling this, so the boundary logic is
// uniform: prefer cwd if the path is under it (cheapest and
// matches user intent), otherwise walk upward for the nearest
// `.git` project root (handles absolute paths to sibling projects
// and `../...` relative paths that escape cwd). An empty cwd
// falls through to the .git walk.
func ancestorStopBoundary(abs, cwd string) string {
	if cwd != "" {
		if rel, err := filepath.Rel(cwd, abs); err == nil &&
			rel != "." && rel != ".." &&
			!strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return cwd
		}
	}
	return gitProjectRoot(filepath.Dir(abs))
}

// gitProjectRoot walks upward from start and returns the first
// directory that contains a `.git` entry (file or directory), or
// "" if none is reached before the filesystem root.
func gitProjectRoot(start string) string {
	dir := start
	for {
		if _, err := os.Lstat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// ancestorChainHasSymlink walks upward from dir up to (but not
// including) stop and reports whether any step in the chain is a
// symbolic link. Results are memoised per directory so sibling
// paths under a shared ancestor do not re-Lstat the same entries.
func ancestorChainHasSymlink(dir, stop string, cache map[string]bool) bool {
	if dir == stop || dir == "." || dir == "/" {
		return false
	}
	if v, ok := cache[dir]; ok {
		return v
	}
	result := false
	if info, err := os.Lstat(dir); err == nil &&
		info.Mode()&os.ModeSymlink != 0 {
		result = true
	} else {
		parent := filepath.Dir(dir)
		if parent != dir {
			result = ancestorChainHasSymlink(parent, stop, cache)
		}
	}
	cache[dir] = result
	return result
}

// resolveGlob expands a glob pattern and adds matching markdown files.
func resolveGlob(pattern string, opts ResolveOpts, addFile func(string)) error {
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return fmt.Errorf("invalid glob pattern %q: %w", pattern, err)
	}
	// Precompute cwd once: it's the dominant input for
	// ancestorStopBoundary and doesn't change across matches. The
	// per-directory Lstat cache is shared across every match too,
	// so each ancestor dir is Lstat'd at most once per expansion.
	cwd, _ := os.Getwd()
	ancestorCache := make(map[string]bool)
	for _, m := range matches {
		// filepath.Glob follows symlinked directory components
		// during expansion, so a pattern like `linked/*.md` will
		// return paths rooted under a symlinked dir. Reject any
		// match whose ancestor chain contains a symlink: symlinked
		// directories are always skipped.
		if hasSymlinkAncestorWithCwd(m, cwd, ancestorCache) {
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
		// Symlinks are followed only when they resolve to a regular
		// file; see resolveArg for rationale.
		if isSymlink && !info.Mode().IsRegular() {
			continue
		}
		if info.IsDir() {
			if err := addDirFiles(m, opts, addFile); err != nil {
				return err
			}
			continue
		}
		// Skip FIFO, device, and socket entries even when their
		// name ends in .md; only regular files (and symlinks to
		// regular files, which passed the check above) are linted.
		if !isSymlink && !info.Mode().IsRegular() {
			continue
		}
		if isMarkdown(m) {
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
			// a regular file. Directory targets would be silently
			// treated as an empty walk; FIFO/device/socket targets
			// would block or fail later on read. `--follow-symlinks`
			// applies to file symlinks only.
			if tgt, statErr := os.Stat(path); statErr != nil ||
				!tgt.Mode().IsRegular() {
				return nil
			}
		}

		if matcher != nil && isGitignored(matcher, path, info.IsDir()) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if info.IsDir() {
			return nil
		}
		// Only regular files and opted-in symlinks (target regular,
		// verified above) are markdown candidates. Skip FIFOs,
		// devices, and sockets to avoid blocking reads later.
		isSymlink := info.Mode()&os.ModeSymlink != 0
		if !isSymlink && !info.Mode().IsRegular() {
			return nil
		}
		if isMarkdown(path) {
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
