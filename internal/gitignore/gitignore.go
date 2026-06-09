// Package gitignore matches a path against .gitignore patterns. It
// collects the patterns from .gitignore files at every directory level
// (plus ancestors, stopping at the working-tree boundary) and answers
// whether a given path is ignored.
package gitignore

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// Matcher checks whether a given path is ignored according to
// .gitignore rules. It supports multiple .gitignore files at different
// directory levels, including negation patterns.
type Matcher struct {
	// rules ordered from root to leaf; later rules override earlier ones.
	rules []ignoreRule
}

// ignoreRule is a single pattern from a .gitignore file.
type ignoreRule struct {
	// base is the directory containing the .gitignore that defined this rule.
	base string
	// pattern is the gitignore pattern (without leading / or trailing /).
	pattern string
	// negate means this rule re-includes a previously ignored path.
	negate bool
	// dirOnly means the pattern only matches directories.
	dirOnly bool
	// hasSlash means the pattern contains a / (other than trailing), so it
	// should be matched against the full relative path rather than just the
	// base name.
	hasSlash bool
}

// parseGitignoreFile reads a .gitignore file and returns its rules.
func parseGitignoreFile(path string) ([]ignoreRule, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	base := filepath.Dir(path)
	var rules []ignoreRule

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()

		// Strip trailing whitespace (unless escaped with backslash).
		line = trimTrailingWhitespace(line)

		// Skip blank lines and comments.
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		r := ignoreRule{base: base}

		// Handle negation.
		if strings.HasPrefix(line, "!") {
			r.negate = true
			line = line[1:]
		}

		// Handle directory-only pattern (trailing /).
		if strings.HasSuffix(line, "/") {
			r.dirOnly = true
			line = strings.TrimSuffix(line, "/")
		}

		// A leading slash anchors the pattern to the base directory.
		// Remove it but note that the pattern contains a slash.
		if strings.HasPrefix(line, "/") {
			line = line[1:]
			r.hasSlash = true
		} else {
			// If the pattern contains a slash (not just trailing which was
			// already removed), it is also anchored.
			r.hasSlash = strings.Contains(line, "/")
		}

		r.pattern = line
		rules = append(rules, r)
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return rules, nil
}

// trimTrailingWhitespace removes trailing spaces and tabs unless the last
// space is escaped with a backslash.
func trimTrailingWhitespace(s string) string {
	i := len(s)
	for i > 0 && (s[i-1] == ' ' || s[i-1] == '\t') {
		i--
	}
	// If the char before the trimmed spaces is a backslash, keep one space.
	if i < len(s) && i > 0 && s[i-1] == '\\' {
		return s[:i-1] + " "
	}
	return s[:i]
}

// NewMatcher creates a matcher by collecting .gitignore files
// from the given root directory and all its subdirectories.
// It also looks for .gitignore files in ancestor directories up to the
// filesystem root.
func NewMatcher(root string) *Matcher {
	m := &Matcher{}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return m
	}

	// Collect .gitignore files from ancestors (root of tree down to parent of root).
	ancestors := collectAncestorGitignores(absRoot)
	for _, gi := range ancestors {
		rules, err := parseGitignoreFile(gi)
		if err != nil {
			continue
		}
		m.rules = append(m.rules, rules...)
	}

	// Collect .gitignore files within the root directory tree.
	_ = filepath.Walk(absRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() && info.Name() == ".gitignore" {
			rules, parseErr := parseGitignoreFile(path)
			if parseErr != nil {
				return nil
			}
			m.rules = append(m.rules, rules...)
		}
		return nil
	})

	return m
}

// collectAncestorGitignores finds .gitignore files in directories above
// the given root, ordered from the top of root's own working tree down
// to root's parent — the walk stops at the working-tree boundary, as
// detailed below.
//
// The walk stops at a Git working-tree boundary. Git applies a
// .gitignore only to paths within the working tree that contains it, so
// rules from an enclosing repository do not cross into a nested working
// tree. Concretely:
//
//   - If root is itself a working-tree root (it contains a .git entry,
//     which is a file for a linked worktree and a directory for the main
//     checkout), no ancestor .gitignore is collected at all.
//   - Otherwise the walk climbs through ancestors and stops after the
//     first ancestor that is a working-tree root — that ancestor is the
//     top of root's own working tree.
//
// Without this boundary, a worktree nested under a path its superproject
// ignores (e.g. ".claude/worktrees/agent-x") would inherit that ignore
// rule and classify every file inside the worktree as ignored, so a
// catalog glob would resolve to zero files and `fix` would empty the
// section.
func collectAncestorGitignores(root string) []string {
	// A matcher rooted at its own working tree must not inherit the
	// superproject's ignore rules.
	if isWorktreeRoot(root) {
		return nil
	}

	var ancestors []string
	dir := filepath.Dir(root)
	for {
		gi := filepath.Join(dir, ".gitignore")
		if _, err := os.Stat(gi); err == nil {
			ancestors = append([]string{gi}, ancestors...)
		}
		// Stop after the working-tree root that root belongs to; rules
		// above it are in an enclosing repository and do not apply.
		if isWorktreeRoot(dir) {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	// Reverse so they go from root-of-tree down to immediate parent.
	// They are already collected root-first due to prepending, so no reversal needed.
	return ancestors
}

// isWorktreeRoot reports whether dir is the root of a Git working tree,
// i.e. it directly contains a ".git" entry. For the main checkout this
// is a directory; for a linked worktree (git worktree add) it is a file
// holding a "gitdir:" pointer. Either form marks a working-tree boundary
// that ancestor .gitignore collection must not cross.
func isWorktreeRoot(dir string) bool {
	_, err := os.Lstat(filepath.Join(dir, ".git"))
	return err == nil
}

// IsIgnored returns true if the given absolute path should be ignored.
// isDir indicates whether the path is a directory.
func (m *Matcher) IsIgnored(absPath string, isDir bool) bool {
	ignored := false
	for _, r := range m.rules {
		if r.dirOnly && !isDir {
			continue
		}
		if matchRule(r, absPath) {
			ignored = !r.negate
		}
	}
	return ignored
}

// matchRule checks whether a single rule matches the given absolute path.
func matchRule(r ignoreRule, absPath string) bool {
	// Compute the path relative to the rule's base directory.
	rel, err := filepath.Rel(r.base, absPath)
	if err != nil {
		return false
	}
	// Normalize to forward slashes for matching.
	rel = filepath.ToSlash(rel)

	// Paths outside the rule's base should not match.
	if strings.HasPrefix(rel, "..") {
		return false
	}

	if r.hasSlash {
		// Pattern contains a slash: match against the full relative path.
		return matchGitignorePattern(r.pattern, rel)
	}

	// No slash: match against just the basename, or any path component.
	// Per git spec, a pattern without a slash matches the basename of any
	// file at any depth.
	base := filepath.Base(absPath)
	if matchGitignorePattern(r.pattern, base) {
		return true
	}
	// Also try matching against the full relative path, since patterns like
	// "*.md" should match "sub/file.md" via basename matching above, but
	// patterns like "dir" should match "dir" at any level.
	return matchGitignorePattern(r.pattern, rel)
}

// matchGitignorePattern matches a gitignore pattern against a path string.
// It supports *, ?, [...], and ** (which matches zero or more directories).
func matchGitignorePattern(pattern, path string) bool {
	// Handle ** patterns by expanding them.
	if strings.Contains(pattern, "**") {
		return matchDoublestar(pattern, path)
	}
	// Use filepath.Match for simple patterns.
	matched, _ := filepath.Match(pattern, path)
	return matched
}

// matchDoublestar handles patterns containing **.
func matchDoublestar(pattern, path string) bool {
	// Find the single "**" without allocating. Multiple "**" fall back to
	// the filepath.Match path at the bottom.
	idx := strings.Index(pattern, "**")
	if idx >= 0 && !strings.Contains(pattern[idx+2:], "**") {
		prefix := pattern[:idx]
		suffix := pattern[idx+2:]
		return matchSingleDoublestar(prefix, suffix, path)
	}
	// Multiple ** in one pattern — fall back to simple matching.
	matched, _ := filepath.Match(strings.ReplaceAll(pattern, "**", "*"), path)
	return matched
}

// matchSingleDoublestar handles the three structural cases for a pattern
// with exactly one "**": leading, trailing, and middle.
// All string slicing is zero-copy (no allocations from splitting).
func matchSingleDoublestar(prefix, suffix, path string) bool {
	// Leading ** — matches any path prefix.
	if prefix == "" || prefix == "/" {
		return matchLeadingDoublestar(strings.TrimPrefix(suffix, "/"), path)
	}
	// Trailing ** — matches any path suffix.
	if suffix == "" || suffix == "/" {
		prefix = strings.TrimSuffix(prefix, "/")
		return strings.HasPrefix(path, prefix+"/") || path == prefix
	}
	// Middle ** — e.g., "a/**/b".
	return matchMiddleDoublestar(
		strings.TrimSuffix(prefix, "/"),
		strings.TrimPrefix(suffix, "/"),
		path,
	)
}

// matchLeadingDoublestar handles "**/suffix" style patterns. It tries the
// suffix against every trailing sub-path of path, advancing one directory
// component at a time. String slicing shares the backing array with path.
func matchLeadingDoublestar(suffix, path string) bool {
	if suffix == "" {
		return true // pattern is just "**"
	}
	tail := path
	for {
		if matchGitignorePattern(suffix, tail) {
			return true
		}
		j := strings.Index(tail, "/")
		if j < 0 {
			break
		}
		tail = tail[j+1:]
	}
	return false
}

// matchMiddleDoublestar handles "prefix/**/suffix" style patterns. It tries
// every split position of path without allocating (substrings share the
// backing array with path).
func matchMiddleDoublestar(prefix, suffix, path string) bool {
	// Position 0: no prefix constraint; check suffix against the whole path.
	if matchGitignorePattern(suffix, path) {
		return true
	}
	remaining := path
	for {
		j := strings.Index(remaining, "/")
		if j < 0 {
			break
		}
		prefixPart := path[:len(path)-len(remaining)+j]
		suffixPart := remaining[j+1:]
		if matchGitignorePattern(prefix, prefixPart) &&
			matchGitignorePattern(suffix, suffixPart) {
			return true
		}
		remaining = remaining[j+1:]
	}
	return false
}
