// Package discovery finds Markdown files by expanding glob patterns from config.
package discovery

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/jeduden/mdsmith/internal/gitignore"
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

	// FollowSymlinks opts in to including symlinks that resolve
	// to regular files. The zero value skips all symlinks, which
	// is the secure default.
	//
	// Symlinks resolving to anything other than a regular file
	// (directories, FIFOs, devices, sockets) are always skipped.
	// filepath.WalkDir reports symlinks via Lstat, so symlinked
	// directories are never descended into regardless of this flag.
	FollowSymlinks bool
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

	validPatterns := validatePatterns(opts.Patterns)
	if len(validPatterns) == 0 {
		return nil, nil
	}

	var gitMatcher *gitignore.Matcher
	if opts.UseGitignore {
		gitMatcher = gitignore.NewMatcher(baseDir)
	}

	w := &walker{
		absBase:        absBase,
		patterns:       validPatterns,
		git:            gitMatcher,
		followSymlinks: opts.FollowSymlinks,
		seen:           make(map[string]struct{}),
	}

	if err := filepath.WalkDir(absBase, w.visit); err != nil {
		return nil, err
	}

	sort.Strings(w.result)
	return w.result, nil
}

// validatePatterns returns patterns that are syntactically valid.
func validatePatterns(patterns []string) []string {
	valid := make([]string, 0, len(patterns))
	for _, p := range patterns {
		if doublestar.ValidatePattern(p) {
			valid = append(valid, p)
		}
	}
	return valid
}

// walker holds state for the directory walk.
type walker struct {
	absBase        string
	patterns       []string
	git            *gitignore.Matcher
	followSymlinks bool
	seen           map[string]struct{}
	result         []string
}

// visit is the fs.WalkDirFunc callback. filepath.WalkDir (unlike the
// deprecated filepath.Walk) hands us the DirEntry ReadDir already
// produced instead of Lstat-ing every entry itself — one syscall per
// entry instead of two, and d.Type()/d.IsDir() below never Lstat again.
func (w *walker) visit(path string, d fs.DirEntry, walkErr error) error {
	if walkErr != nil {
		return walkErr
	}

	rel, err := filepath.Rel(w.absBase, path)
	if err != nil || rel == "." {
		return nil
	}
	rel = filepath.ToSlash(rel)

	// Symlink entries always report IsDir()==false under WalkDir (same
	// as Walk's Lstat-based info), so returning nil also means WalkDir
	// won't try to descend.
	if d.Type()&os.ModeSymlink != 0 {
		if !w.followSymlinks {
			return nil
		}
		// In opt-in mode, include the entry only if it resolves to
		// a regular file. Directory targets are skipped (Options
		// doc); FIFO/device/socket targets are skipped to avoid
		// blocking reads during linting.
		if tgt, statErr := os.Stat(path); statErr != nil ||
			!tgt.Mode().IsRegular() {
			return nil
		}
	}

	if w.isGitignored(path, d.IsDir()) {
		if d.IsDir() {
			return filepath.SkipDir
		}
		return nil
	}

	if d.IsDir() {
		return nil
	}
	// Only include regular files and opted-in symlinks whose
	// target is regular (already verified in the symlink branch
	// above). FIFO, device, and socket entries are skipped to
	// avoid blocking reads during linting.
	isSymlink := d.Type()&os.ModeSymlink != 0
	if !isSymlink && !d.Type().IsRegular() {
		return nil
	}

	if w.matchesAny(rel) {
		w.addFile(rel, path)
	}
	return nil
}

// isGitignored returns true if the path should be skipped by .gitignore rules.
func (w *walker) isGitignored(path string, isDir bool) bool {
	if w.git == nil {
		return false
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	return w.git.IsIgnored(absPath, isDir)
}

// matchesAny returns true if rel matches any of the configured patterns.
// w.patterns has already passed doublestar.ValidatePattern once, in
// validatePatterns; doublestar.Match re-validates its pattern argument
// on every call, so matching through MatchUnvalidated here skips paying
// that cost again for every file the walk visits (see
// internal/globpath's validPatterns cache for the same fix applied to
// the config-driven glob surfaces).
//
// A pattern already passing ValidatePattern does not guarantee Match
// and MatchUnvalidated agree: a brace alternative (e.g.
// "{[!mdb[],docs/**/*.md}") can contain a syntax error in a non-final
// alternative that Match's internal validation aborts on before trying
// later alternatives, while MatchUnvalidated tries them all — a
// divergence confirmed directly against the vendored doublestar
// source, not assumed. That divergence requires brace syntax: a 1.3M+
// -case differential fuzz over brace-free, ValidatePattern-accepted
// patterns found zero disagreement. Patterns without "{" take the fast
// MatchUnvalidated path; a pattern using brace expansion falls back to
// the safe (slower, but provably correct) Match.
func (w *walker) matchesAny(rel string) bool {
	for _, p := range w.patterns {
		if strings.IndexByte(p, '{') < 0 {
			if doublestar.MatchUnvalidated(p, rel) {
				return true
			}
			continue
		}
		if matched, err := doublestar.Match(p, rel); err == nil && matched {
			return true
		}
	}
	return false
}

// addFile adds a file to the result set if not already seen. The rel path
// (relative to BaseDir, using forward slashes) is stored in the result so
// that config override patterns match consistently regardless of discovery
// method. The absPath is used only for deduplication.
func (w *walker) addFile(rel, absPath string) {
	if _, ok := w.seen[absPath]; !ok {
		w.seen[absPath] = struct{}{}
		w.result = append(w.result, rel)
	}
}
