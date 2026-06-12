// Package githooks provides utilities shared between the mdsmith CLI
// and the git-hook-sync rule for managing the pre-merge-commit hook,
// merge-driver assignments in .gitattributes, and discovery of files
// that contain generated-section directives.
package githooks

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jeduden/mdsmith/internal/archetype/gensection"
	"github.com/jeduden/mdsmith/internal/bytelimit"
	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/jeduden/mdsmith/internal/setutil"
)

// readFile, atomicWriteFn, and lstatFile are variables so tests can substitute
// failing implementations to exercise error paths in WriteGitattributes.
var readFile = os.ReadFile
var atomicWriteFn = atomicWriteGitattributes
var lstatFile = os.Lstat

// PreMergeCommitMarker is the comment line written into the
// pre-merge-commit hook so that mdsmith (and the git-hook-sync rule)
// can recognise hooks it manages without stomping on user-authored
// hooks of the same name.
const PreMergeCommitMarker = "# mdsmith merge-driver pre-merge-commit hook"

// GitRepoRoot returns the absolute path of the git repository that
// contains dir. The lookup runs `git -C dir rev-parse --show-toplevel`
// so it works correctly when invoked from any subdirectory or when
// linting an absolute path outside the process working directory.
func GitRepoRoot(dir string) (string, error) {
	if dir == "" {
		dir = "."
	}
	out, err := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// ResolveHooksDir returns the directory where git hooks live for the
// repository at repoRoot. It uses `git rev-parse --git-path hooks` so
// that worktrees, submodules, and core.hooksPath all resolve correctly.
// Falls back to <repoRoot>/.git/hooks when git cannot be queried.
func ResolveHooksDir(repoRoot string) string {
	out, err := exec.Command("git", "-C", repoRoot, "rev-parse", "--git-path", "hooks").Output()
	if err == nil {
		p := strings.TrimSpace(string(out))
		if p != "" {
			if !filepath.IsAbs(p) {
				p = filepath.Join(repoRoot, p)
			}
			return filepath.Clean(p)
		}
	}
	return filepath.Join(repoRoot, ".git", "hooks")
}

// DiscoverFiles scans repoRoot for Markdown files containing a
// generated-section directive (catalog, include, toc, …). Returned
// paths are relative to repoRoot and use forward-slash separators on
// every platform so they compare correctly against entries written
// into .gitattributes and the pre-merge-commit hook.
//
// Hidden directories (names starting with ".") are skipped. The
// returned slice is sorted and may be empty: the caller decides
// whether to apply a fallback (the install commands do; the
// git-hook-sync rule does not).
func DiscoverFiles(repoRoot string, maxBytes int64) []string {
	allRules := rule.All()
	directiveNames := make([]string, 0, len(allRules))
	for _, r := range allRules {
		if d, ok := r.(gensection.Directive); ok {
			directiveNames = append(directiveNames, d.Name())
		}
	}

	// Load the project's ignore patterns so discovery does not list
	// files that mdsmith would skip during `mdsmith fix`. Without this
	// the merge driver and pre-merge-commit hook would fire on paths
	// (e.g. fixture files under `internal/rules/*/{good,bad,fixed}/**`)
	// where mdsmith fix is a no-op, leaving real conflicts unresolved.
	// A missing or unparseable config simply means no ignore filtering.
	var ignorePatterns []string
	if cfg, err := config.Load(filepath.Join(repoRoot, configFileName)); err == nil {
		ignorePatterns = cfg.Ignore
	}

	seen := make(map[string]struct{})
	var files []string
	_ = filepath.Walk(repoRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			if path != repoRoot && strings.HasPrefix(info.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		// Only follow regular files. Skip symlinks (consistent with
		// the project's secure-by-default symlink stance) and any
		// other non-regular type (FIFOs, devices, sockets), which
		// would otherwise cause hangs or read outside the repo.
		if !info.Mode().IsRegular() {
			return nil
		}
		name := info.Name()
		if !strings.HasSuffix(name, ".md") && !strings.HasSuffix(name, ".markdown") {
			return nil
		}
		rel, err := filepath.Rel(repoRoot, path)
		if err != nil {
			return nil
		}
		key := filepath.ToSlash(rel)
		if config.IsIgnored(ignorePatterns, key) {
			return nil
		}
		content, err := bytelimit.ReadFileLimited(path, maxBytes)
		if err != nil {
			return nil
		}
		// Detect real directive markers line-by-line via the marker
		// parser so prose/inline-code mentions of `<?catalog?>` do
		// not bloat the discovered set.
		if !hasDirectiveMarker(content, directiveNames) {
			return nil
		}
		if _, dup := seen[key]; dup {
			return nil
		}
		seen[key] = struct{}{}
		files = append(files, key)
		return nil
	})

	// Sort so the file list is stable across platforms and
	// filesystems; the result is printed to users and embedded into
	// the pre-merge-commit hook and .gitattributes, where churn
	// hurts review diffs.
	sort.Strings(files)
	return files
}

// configFileName duplicates the config filename locally so this
// package does not need `internal/config` to export the constant.
const configFileName = ".mdsmith.yml"

// GlobsFromConfig returns the canonical merge-driver glob set for a
// repository: every markdown extension is included, and the project's
// .mdsmith.yml ignore patterns are translated as exclude patterns.
// Last-match-wins in .gitattributes lets the excludes override the
// broader markdown includes. cfg may be nil (no exclusions then).
//
// Patterns that cannot be represented directly in .gitattributes
// are dropped from the exclude set so MDS048's auto-fix never
// produces a broken managed block:
//
//   - .gitattributes splits attribute lines on whitespace, so a
//     pattern containing a space or tab would be parsed as a path
//     plus a stray attribute.
//   - .gitattributes does not support `!`-prefixed negation. A
//     pattern like `!docs/*.md` written verbatim would be silently
//     ignored by git (or treated as a literal path starting with
//     `!`), which is misleading.
//
// The returned `skipped` slice lists any ignore patterns that were
// dropped, in input order. Callers that have an error channel
// (notably the install commands) surface them on stderr; the
// rule's auto-fix path silently discards the list because it runs
// per-file and would otherwise flood diagnostic output.
func GlobsFromConfig(cfg *config.Config) (Globs, []string) {
	g := Globs{Include: DefaultIncludes()}
	if cfg == nil || len(cfg.Ignore) == 0 {
		return g, nil
	}
	g.Exclude = make([]string, 0, len(cfg.Ignore))
	var skipped []string
	for _, p := range cfg.Ignore {
		if !isRepresentableGitattributesPattern(p) {
			skipped = append(skipped, p)
			continue
		}
		g.Exclude = append(g.Exclude, p)
	}
	return g, skipped
}

// isRepresentableGitattributesPattern reports whether pattern can be
// copied directly into a .gitattributes pattern field without
// changing its meaning. Negation (`!pattern`) is unsupported, and
// whitespace would split the generated line into multiple fields.
func isRepresentableGitattributesPattern(pattern string) bool {
	if pattern == "" {
		return false
	}
	if strings.HasPrefix(pattern, "!") {
		return false
	}
	return !strings.ContainsAny(pattern, " \t\r\n")
}

// LoadGlobs reads .mdsmith.yml from repoRoot and returns the merge-
// driver glob set. A missing or unparseable config falls back to the
// default include set with no exclusions. Skipped (unrepresentable)
// ignore patterns are silently discarded — callers that need to
// surface them should use GlobsFromConfig directly.
func LoadGlobs(repoRoot string) Globs {
	cfg, err := config.Load(filepath.Join(repoRoot, configFileName))
	if err != nil {
		g, _ := GlobsFromConfig(nil)
		return g
	}
	g, _ := GlobsFromConfig(cfg)
	return g
}

// DiscoverFilesForInstall is the install-time variant of DiscoverFiles
// that supplies a sensible default file list when the repository has
// no directive-bearing files. It returns ["PLAN.md", "README.md"] in
// that case so a fresh repo still gets a useful hook/.gitattributes
// configuration after `mdsmith merge-driver install` or
// `mdsmith pre-merge-commit install`.
//
// The git-hook-sync rule must not use this variant: when the user
// has no directive-bearing files, the rule should report nothing
// rather than reference fictional PLAN.md/README.md paths.
func DiscoverFilesForInstall(repoRoot string, maxBytes int64) []string {
	files := DiscoverFiles(repoRoot, maxBytes)
	if len(files) == 0 {
		return []string{"PLAN.md", "README.md"}
	}
	return files
}

// hasDirectiveMarker reports whether content contains a real
// processing-instruction start or end marker for any of the named
// directives. It scans line-by-line so a backticked or otherwise
// inline mention like `<?catalog?>` in prose is not treated as a
// directive. Markers that fall inside a fenced code block (lines
// between matching ``` or ~~~ fences, with the closing fence using
// the same character and at least the same length as the opener)
// are also ignored; mdsmith's own parser only honors processing-
// instructions at the document root.
//
// The same indentation gate applied by internal/lint.pi_parser is
// used here: a line that begins with a tab or with more than three
// spaces is an indented code block per CommonMark and cannot host a
// processing-instruction, so any directive-looking text on such a
// line is ignored.
func hasDirectiveMarker(content []byte, names []string) bool {
	var fenceChar byte
	var fenceLen int
	for _, line := range bytes.Split(content, []byte("\n")) {
		if fenceChar == 0 {
			if ch, run := openingFence(line); ch != 0 {
				// Entering a fenced block.
				fenceChar = ch
				fenceLen = run
				continue
			}
		} else {
			if isClosingFence(line, fenceChar, fenceLen) {
				fenceChar = 0
				fenceLen = 0
				continue
			}
			// Inside a fenced block: ignore any directive markers.
			continue
		}
		if isIndentedCodeBlock(line) {
			continue
		}
		for _, n := range names {
			if gensection.IsRawStartMarker(line, n) || gensection.IsRawEndMarker(line, n) {
				return true
			}
		}
	}
	return false
}

// isIndentedCodeBlock reports whether line begins an indented code
// block per CommonMark: four or more spaces of indentation, or a tab
// character within the first four columns (optionally preceded by
// up to three spaces). internal/lint.pi_parser uses the same rule,
// so this keeps discovery aligned with the actual mdsmith parser.
func isIndentedCodeBlock(line []byte) bool {
	if len(line) == 0 {
		return false
	}
	spaces := 0
	for spaces < len(line) && line[spaces] == ' ' {
		spaces++
	}
	if spaces >= 4 {
		return true
	}
	return spaces < len(line) && line[spaces] == '\t'
}

// openingFence reports the fence character and run length of a line
// that begins (after up to 3 spaces of indentation) with a sequence
// of three or more backticks or tildes. Returns (0, 0) if the line
// is not a fence.
func openingFence(line []byte) (byte, int) {
	// Allow up to three spaces of indentation per CommonMark.
	i := 0
	for i < len(line) && i < 3 && line[i] == ' ' {
		i++
	}
	if i >= len(line) {
		return 0, 0
	}
	c := line[i]
	if c != '`' && c != '~' {
		return 0, 0
	}
	run := 0
	for i < len(line) && line[i] == c {
		i++
		run++
	}
	if run < 3 {
		return 0, 0
	}
	return c, run
}

// isClosingFence reports whether line closes an open fenced block
// that uses ch with opener length >= openLen. Per CommonMark, the
// closing fence may be preceded by up to three spaces of indentation
// and may only be followed by whitespace (no info string allowed),
// so a line like "```not-a-closing-fence" is treated as content,
// not as a fence terminator.
func isClosingFence(line []byte, ch byte, openLen int) bool {
	i := 0
	for i < len(line) && i < 3 && line[i] == ' ' {
		i++
	}
	run := 0
	for i < len(line) && line[i] == ch {
		i++
		run++
	}
	if run < openLen {
		return false
	}
	for i < len(line) {
		if line[i] != ' ' && line[i] != '\t' && line[i] != '\r' {
			return false
		}
		i++
	}
	return true
}

// FilesMatch reports whether a and b contain the same set of files,
// ignoring order and duplicates. A repeated entry on either side is
// treated the same as a single occurrence so that a `.gitattributes`
// or hook script that lists the same path twice still compares equal
// to a deduplicated list.
func FilesMatch(a, b []string) bool {
	setA := setutil.FromStrings(a)
	setB := setutil.FromStrings(b)
	if len(setA) != len(setB) {
		return false
	}
	for f := range setA {
		if _, ok := setB[f]; !ok {
			return false
		}
	}
	return true
}

// ExtractHookFiles parses a pre-merge-commit hook script and returns
// the list of files it invokes `mdsmith fix --` on. Files appear in
// the order they occur in the hook. Each `fix --` line contributes at
// most one entry: the first single-quoted token that follows. Comment
// and blank lines are skipped so a commented-out example or note in
// the hook does not produce a false managed-file entry.
func ExtractHookFiles(content string) []string {
	var files []string
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if !strings.Contains(trimmed, "fix --") {
			continue
		}
		if f, ok := firstQuotedAfter(trimmed, "fix --"); ok {
			files = append(files, f)
		}
	}
	return files
}

// ExtractGitattributesFiles returns the list of paths assigned to the
// mdsmith merge driver in .gitattributes content. Each entry is the
// pathname token from a line of the form `<pathname> merge=mdsmith`.
// Comment lines (`#`) and lines without a `merge=mdsmith` attribute
// are ignored.
//
// The parser splits on whitespace, so it does not support pathnames
// that themselves contain whitespace. NormalizeManagedPath rejects
// such paths at install time so the installer and the drift checker
// stay consistent.
func ExtractGitattributesFiles(content string) []string {
	var files []string
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) < 2 {
			continue
		}
		hasDriver := false
		for _, f := range fields[1:] {
			if f == "merge=mdsmith" {
				hasDriver = true
				break
			}
		}
		if hasDriver {
			files = append(files, fields[0])
		}
	}
	return files
}

// NormalizeManagedPath converts p (which may be absolute, relative,
// or use OS-specific separators) into the canonical form used in
// .gitattributes and the pre-merge-commit hook: a non-empty
// repo-relative path with forward-slash separators that does not
// escape repoRoot.
//
// Whitespace inside the *resulting* repo-relative path is rejected
// because .gitattributes splits attributes on whitespace and the
// rule's Fields-based parser cannot recover the original token. The
// check runs after Rel/ToSlash so an absolute input rooted at a
// repo whose own path contains whitespace (e.g. a Windows or macOS
// home dir with spaces) is still accepted, as long as the
// repo-relative tail is whitespace-free.
//
// Glob and pathspec metacharacters (`*`, `?`, `[`) are also
// rejected. The install commands write each managed entry into a
// `[ -e <path> ]` guard inside the pre-merge-commit hook script, and
// `[ -e ]` treats its argument as a literal filename rather than a
// glob, so a pattern like `docs/*.md` would always be skipped even
// when files match. The drift checker likewise compares exact paths.
func NormalizeManagedPath(repoRoot, p string) (string, error) {
	if strings.TrimSpace(p) == "" {
		return "", fmt.Errorf("empty path")
	}

	abs := p
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(repoRoot, abs)
	}
	absClean := filepath.Clean(abs)
	rootClean := filepath.Clean(repoRoot)

	rel, err := filepath.Rel(rootClean, absClean)
	if err != nil {
		return "", fmt.Errorf("path %q is not relative to repo root %q: %w", p, repoRoot, err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("path %q escapes repository root", p)
	}
	out := filepath.ToSlash(rel)
	if strings.ContainsAny(out, " \t\n\r") {
		return "", fmt.Errorf("path %q contains whitespace, which is not supported in managed file lists", p)
	}
	if strings.ContainsAny(out, "*?[") {
		return "", fmt.Errorf(
			"path %q contains a glob/pathspec character (*, ?, [); "+
				"managed file lists must be exact paths",
			p,
		)
	}
	return out, nil
}

// NormalizeManagedPaths normalizes each entry via NormalizeManagedPath.
// It returns the first error encountered, so callers can surface a
// single clear message rather than a list of failures.
func NormalizeManagedPaths(repoRoot string, paths []string) ([]string, error) {
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		norm, err := NormalizeManagedPath(repoRoot, p)
		if err != nil {
			return nil, err
		}
		out = append(out, norm)
	}
	return out, nil
}

// Marker comments for the managed block in .gitattributes
const (
	gitattributesManagedBlockStart = "# BEGIN mdsmith merge-driver"
	gitattributesManagedBlockEnd   = "# END mdsmith merge-driver"
)

// stripStaleMergeMdsmithLines drops any non-comment line that assigns
// the mdsmith merge driver outside the managed block. The match logic
// mirrors ExtractGitattributesFiles: blank/comment lines are ignored,
// and a line is considered a merge-driver assignment when any field
// after the path equals `merge=mdsmith`. Without this, leftover
// entries from older append-only installs (or hand-edits) would make
// .gitattributes appear out of sync immediately after a fix, and
// could leave the resulting file with duplicate path assignments.
func stripStaleMergeMdsmithLines(content string) string {
	lines := strings.Split(content, "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			kept = append(kept, line)
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) >= 2 {
			hasDriver := false
			for _, f := range fields[1:] {
				if f == "merge=mdsmith" {
					hasDriver = true
					break
				}
			}
			if hasDriver {
				continue
			}
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}

// findManagedBlockLines returns the half-open line range
// [startLine, endLineExclusive) covering the managed block in lines.
// The BEGIN and END markers are matched only as standalone trimmed
// lines (not embedded in another comment).
//
// When BEGIN is present but END is missing — for example, after a
// partial edit or an aborted merge that left half a managed block
// behind — the range runs from BEGIN to EOF. The writer then replaces
// the incomplete block instead of appending a duplicate one and
// leaving the stray BEGIN behind. Returns (-1, -1) only when no
// BEGIN marker exists.
func findManagedBlockLines(lines []string) (int, int) {
	startLine := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == gitattributesManagedBlockStart {
			startLine = i
			break
		}
	}
	if startLine == -1 {
		return -1, -1
	}
	for i := startLine; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == gitattributesManagedBlockEnd {
			return startLine, i + 1
		}
	}
	return startLine, len(lines)
}

// Globs describes the set of paths the mdsmith merge driver applies
// to. Each Include pattern is written as `<pattern> merge=mdsmith`
// and each Exclude pattern is written after them as `<pattern>
// -merge`. .gitattributes uses last-match-wins, so an exclude line
// after the include lines effectively removes the merge driver from
// any path the include patterns matched.
//
// `.gitattributes` itself does not support negative patterns (`!*.md`
// is a syntax error there). Order-sensitive override via -merge is the
// supported way to express exclusions, which is why Globs keeps
// Include and Exclude as separate ordered slices.
type Globs struct {
	Include []string
	Exclude []string
}

// DefaultIncludes is the canonical include pattern set: every
// markdown extension mdsmith processes. Kept as a function so callers
// always get a fresh slice rather than sharing a package-level value.
func DefaultIncludes() []string {
	return []string{"*.md", "*.markdown"}
}

// RenderManagedBlock returns the .gitattributes managed block content
// for globs, including the BEGIN/END markers and a trailing newline.
// Output is deterministic so drift detection compares it byte-for-byte
// against the installed block.
func RenderManagedBlock(globs Globs) string {
	var b strings.Builder
	b.WriteString(gitattributesManagedBlockStart)
	b.WriteString("\n")
	for _, p := range globs.Include {
		fmt.Fprintf(&b, "%s merge=mdsmith\n", p)
	}
	for _, p := range globs.Exclude {
		fmt.Fprintf(&b, "%s -merge\n", p)
	}
	b.WriteString(gitattributesManagedBlockEnd)
	b.WriteString("\n")
	return b.String()
}

// ExtractGlobs parses the managed block from .gitattributes content
// and returns the include and exclude patterns. The second return is
// true when a managed block was found. Content outside the BEGIN/END
// markers is ignored — stale `merge=mdsmith` lines outside the block
// are handled by stripStaleMergeMdsmithLines at write time.
func ExtractGlobs(content string) (Globs, bool) {
	lines := strings.Split(content, "\n")
	if strings.HasSuffix(content, "\n") {
		lines = lines[:len(lines)-1]
	}
	startLine, endLine := findManagedBlockLines(lines)
	if startLine == -1 {
		return Globs{}, false
	}
	var globs Globs
	for i := startLine + 1; i < endLine; i++ {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) < 2 {
			continue
		}
		pattern := fields[0]
		for _, attr := range fields[1:] {
			switch attr {
			case "merge=mdsmith":
				globs.Include = append(globs.Include, pattern)
			case "-merge":
				globs.Exclude = append(globs.Exclude, pattern)
			default:
				continue
			}
			break
		}
	}
	return globs, true
}

// GlobsEqual reports whether two glob sets are identical. Comparison
// is order-sensitive because .gitattributes uses last-match-wins:
// reordering Include vs Exclude (or shuffling Exclude entries that
// might overlap) changes which paths the merge driver applies to.
func GlobsEqual(a, b Globs) bool {
	if len(a.Include) != len(b.Include) || len(a.Exclude) != len(b.Exclude) {
		return false
	}
	for i, p := range a.Include {
		if b.Include[i] != p {
			return false
		}
	}
	for i, p := range a.Exclude {
		if b.Exclude[i] != p {
			return false
		}
	}
	return true
}

// WriteGitattributes updates .gitattributes to assign the mdsmith
// merge driver to the patterns described by globs. It preserves all
// non-mdsmith entries and replaces only the BEGIN/END managed block.
// Stray `merge=mdsmith` lines outside the managed block (left behind
// by older append-only installs or hand-edited files) are removed so
// the resulting file matches globs exactly.
//
// If the file does not exist, it is created with only the managed
// block. If the file exists but has no managed block, one is
// appended. If a managed block exists, it is replaced.
//
// This approach ensures that other .gitattributes entries (e.g.
// text, eol=lf, linguist settings, other merge drivers) are never
// dropped.
func WriteGitattributes(path string, globs Globs) error {
	// Reject symlinks and non-regular files before any I/O to reduce the
	// risk of following a link to a path outside the repository.
	// A narrow TOCTOU window remains between this check and the I/O calls.
	if info, err := lstatFile(path); err == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("writing %s: not a regular file", path)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("writing %s: lstat: %w", path, err)
	}

	existing, err := readFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("reading %s: %w", path, err)
	}

	managedBlock := RenderManagedBlock(globs)

	var newContent strings.Builder

	if len(existing) == 0 {
		// New file: just write the managed block
		newContent.WriteString(managedBlock)
	} else {
		// Existing file: preserve non-mdsmith content, replace managed
		// block. Strip stale merge=mdsmith lines from the surrounding
		// text independently so the original ordering of unrelated
		// entries (text, eol=lf, linguist settings) is preserved.
		// Block boundaries are matched against full trimmed lines, not
		// substrings, so a comment like
		// `# update via mdsmith merge-driver install` cannot be
		// mistaken for the managed-block start marker.
		content := string(existing)
		// strings.Split on a trailing newline produces an empty last
		// element. Trim it so each element is a real line; the writer
		// always appends a final newline below (managedBlock and
		// joinLines both emit one), normalising the file to end with
		// a newline regardless of the input's prior state.
		lines := strings.Split(content, "\n")
		if strings.HasSuffix(content, "\n") {
			lines = lines[:len(lines)-1]
		}
		startLine, endLine := findManagedBlockLines(lines)

		joinLines := func(ls []string) string {
			if len(ls) == 0 {
				return ""
			}
			return strings.Join(ls, "\n") + "\n"
		}

		if startLine == -1 {
			// No valid managed block: everything is "before"; the new
			// block is appended at the end after the preserved content.
			before := stripStaleMergeMdsmithLines(joinLines(lines))
			before = strings.TrimSuffix(before, "\n")
			newContent.WriteString(before)
			if before != "" {
				newContent.WriteString("\n")
			}
			newContent.WriteString(managedBlock)
		} else {
			before := stripStaleMergeMdsmithLines(joinLines(lines[:startLine]))
			after := stripStaleMergeMdsmithLines(joinLines(lines[endLine:]))
			newContent.WriteString(before)
			newContent.WriteString(managedBlock)
			newContent.WriteString(after)
		}
	}

	return writeGitattributesFile(path, newContent.String())
}

// writeGitattributesFile writes content to path using a temp-then-rename
// strategy. Even if path is swapped to a symlink between the lstat guard in
// WriteGitattributes and this call, os.Rename replaces the directory entry
// rather than following the link, so the write cannot escape the repository.
func writeGitattributesFile(path, content string) error {
	mode := os.FileMode(0o644)
	if info, err := lstatFile(path); err == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("writing %s: not a regular file", path)
		}
		mode = info.Mode() &^ os.ModeType
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("writing %s: lstat: %w", path, err)
	}
	if err := atomicWriteFn(path, []byte(content), mode); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}

// createTempFn, syncTempFn, closeTempFn, chmodFn, and fstatFn are variables
// so tests can inject failures into atomicWriteGitattributes without OS tricks.
// chmodFn is declared in compat_notinygo.go / compat_tinygo.go so the
// tinygo/wasm build can substitute a no-op without pulling in os.Chmod.
var createTempFn = os.CreateTemp
var syncTempFn = (*os.File).Sync
var closeTempFn = (*os.File).Close
var fstatFn = (*os.File).Stat

// atomicWriteGitattributes writes data to a temp file in the same directory
// as path, sets its permissions, then renames it over path. The rename
// replaces the directory entry atomically, so it cannot follow a symlink
// that might have been introduced between an earlier lstat check and the write.
func atomicWriteGitattributes(path string, data []byte, mode os.FileMode) error {
	// Verify an existing target is writable and has not been swapped to a
	// symlink. os.Rename can replace read-only files when the directory is
	// writable, so we check writability explicitly. We then compare lstat and
	// fstat to detect a TOCTOU swap between the lstat and the open.
	if lstatInfo, err := lstatFile(path); err == nil {
		f, err := os.OpenFile(path, os.O_WRONLY, 0)
		if err != nil {
			return err
		}
		fdInfo, statErr := fstatFn(f)
		_ = f.Close()
		if statErr != nil {
			return statErr
		}
		if !sameFile(lstatInfo, fdInfo) {
			return fmt.Errorf("%s: file changed since lstat", path)
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	dir := filepath.Dir(path)
	tmp, err := createTempFn(dir, ".mdsmith-gitattributes-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := syncTempFn(tmp); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := closeTempFn(tmp); err != nil {
		return err
	}
	if err := chmodFn(tmpName, mode); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

// gitAddGitattributes runs `git add -- .gitattributes` against
// repoRoot and returns git's combined output plus the exit error. It
// is a package-level variable so tests can substitute a fake git that
// fails with a synthetic index.lock message a fixed number of times
// before succeeding, exercising the transient- and persistent-lock
// retry paths deterministically without racing a real lock file.
//
// CombinedOutput is used so git's stderr (e.g. `fatal: Unable to
// create '/.../.git/index.lock': File exists.`) is preserved — both
// for the lock detector below and for MDS048's "staging failed"
// diagnostic, which would otherwise carry only an `exit status N`
// and nothing actionable.
var gitAddGitattributes = func(repoRoot string) ([]byte, error) {
	return exec.Command(
		"git", "-C", repoRoot, "add", "--", ".gitattributes",
	).CombinedOutput()
}

// stageRetryBackoff is the wait schedule between `git add` attempts in
// StageGitattributes. Its length sets the retry budget: one initial
// attempt plus one retry per entry. The waits ramp so a lock held for
// a few hundred milliseconds clears without a tight spin, while a
// genuinely stuck lock still fails in well under a second. It is a
// variable so tests can shorten it to zero-length waits.
var stageRetryBackoff = []time.Duration{
	10 * time.Millisecond,
	20 * time.Millisecond,
	40 * time.Millisecond,
	80 * time.Millisecond,
	160 * time.Millisecond,
}

// isIndexLockError reports whether git's combined output describes a
// failure to acquire .git/index.lock. git prints a stable
// "Unable to create '<path>/index.lock': File exists" line in that
// case; matching both fragments avoids treating an unrelated mention
// of a lock file as a retryable condition.
func isIndexLockError(output []byte) bool {
	s := string(output)
	return strings.Contains(s, "index.lock") && strings.Contains(s, "File exists")
}

// StageGitattributes runs `git add -- .gitattributes` against repoRoot
// so updates written by Fix end up in the index. Without this, the
// pre-merge-commit hook flow stages only the markdown file passed to
// `mdsmith fix`, leaving the regenerated .gitattributes in the working
// tree but absent from the resulting merge commit. Errors are surfaced
// so callers can decide whether to roll back; the working-tree write
// itself is already done at the point this is called.
//
// A `git add` that fails because `.git/index.lock` already exists is
// retried with bounded backoff (stageRetryBackoff): a transient lock
// held by a concurrent git invocation usually clears within a few
// tens of milliseconds, and retrying turns a queue-bouncing hard
// failure into a brief wait. The retry never deletes the lock — it
// only waits for the holder to release it — so a lock this process
// did not create is left untouched. When the lock persists past the
// retry budget, StageGitattributes returns a clear "index locked"
// error rather than a bare exit status. Non-lock failures are
// returned immediately, since they will not clear on retry.
func StageGitattributes(repoRoot string) error {
	var out []byte
	var err error
	for attempt := 0; ; attempt++ {
		out, err = gitAddGitattributes(repoRoot)
		if err == nil {
			return nil
		}
		if !isIndexLockError(out) || attempt >= len(stageRetryBackoff) {
			break
		}
		time.Sleep(stageRetryBackoff[attempt])
	}

	msg := strings.TrimSpace(string(out))
	if isIndexLockError(out) {
		// The lock outlasted every retry. Report it as locked and keep
		// git's own message so the operator sees which lock file and the
		// "remove the file manually" hint, without mdsmith ever removing
		// a lock it did not create. isIndexLockError only returns true
		// when git's output contains the lock message, so msg is
		// non-empty here.
		return fmt.Errorf("stage .gitattributes: index locked: %w: %s", err, msg)
	}
	if msg == "" {
		return fmt.Errorf("stage .gitattributes: %w", err)
	}
	return fmt.Errorf("stage .gitattributes: %w: %s", err, msg)
}

// HasMdsmithMergeDriver reports whether the repository's local git
// config defines `merge.mdsmith.driver` (i.e. the merge driver itself
// has been registered for this repo). The lookup is scoped to the
// repo's local config (`--local`), not global/system config, so a
// user with a personal merge driver elsewhere cannot accidentally
// opt every clone into MDS048's drift checks. A missing driver is
// reported as false rather than as an error so callers can treat
// the merge-driver setup as "not installed".
func HasMdsmithMergeDriver(repoRoot string) bool {
	out, err := exec.Command(
		"git", "-C", repoRoot, "config", "--local", "--get", "merge.mdsmith.driver",
	).Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) != ""
}

// stagingHelperShellFunc is the POSIX `mdsmith_git_add` function the
// hook uses to stage one path with index.lock-aware retry. It is kept
// as a package-level constant so BuildHookScript stays short and so
// the retry policy lives in one readable block. The function backs off
// with `sleep 0.1 2>/dev/null || sleep 1` (fast on coreutils that
// honor fractional sleep, portable elsewhere), bounds the retries, and
// on a persistent lock prints `index locked` and exits the hook
// non-zero. It never deletes `.git/index.lock` — only waits for the
// holder to release it.
const stagingHelperShellFunc = "# Stage one path, retrying a transient .git/index.lock with\n" +
	"# bounded backoff. Never removes a lock it did not create; a\n" +
	"# persistent lock exits non-zero with a clear message.\n" +
	"mdsmith_git_add() {\n" +
	"  _attempt=0\n" +
	"  while :; do\n" +
	"    _err=$(git add -- \"$1\" 2>&1)\n" +
	"    _status=$?\n" +
	"    [ \"$_status\" -eq 0 ] && return 0\n" +
	"    case \"$_err\" in\n" +
	"      *index.lock*\"File exists\"*)\n" +
	"        if [ \"$_attempt\" -ge 5 ]; then\n" +
	"          echo \"mdsmith pre-merge-commit hook: index locked: $_err\" >&2\n" +
	"          exit 1\n" +
	"        fi\n" +
	"        _attempt=$((_attempt + 1))\n" +
	"        sleep 0.1 2>/dev/null || sleep 1\n" +
	"        ;;\n" +
	"      *)\n" +
	"        echo \"$_err\" >&2\n" +
	"        exit \"$_status\"\n" +
	"        ;;\n" +
	"    esac\n" +
	"  done\n" +
	"}\n"

// BuildHookScript returns the canonical pre-merge-commit hook
// content. The script runs `mdsmith fix` once on the entire repo
// after git resolves every per-file merge, so generated sections
// reflect the final merged state. mdsmith fix walks the worktree
// respecting `.mdsmith.yml` ignore patterns, matching the same set
// of files marked with `merge=mdsmith` in `.gitattributes`. Modified
// markdown files are then staged so the merge commit captures them.
//
// The script embeds the absolute path of the mdsmith binary, so one
// line is machine-specific. The rule's drift detection therefore
// re-renders the canonical template and validates the stable hook
// lines (chdir, fix invocation, staging) rather than requiring a
// full byte-for-byte match.
//
// `mdsmith fix` exit code 1 means unfixed diagnostics remain — the
// hook still allows the merge to proceed in that case so reviewers
// can resolve the remaining issues in a follow-up commit. Any other
// non-zero exit (e.g. config errors, panics, exit 2) is propagated
// out of the hook so the merge commit aborts on genuine errors.
//
// The staging loop reads `git diff --name-only` newline-by-newline
// inside a POSIX `while read` loop. `xargs -r` is a GNU extension
// (BSD xargs on macOS does not support it), so an empty pipeline
// would otherwise invoke `git add --` with no arguments and abort
// the merge. The loop also avoids splitting on filename whitespace
// (read uses IFS= -r) at the cost of mishandling the rare filename
// that contains literal newlines — an acceptable trade for
// portability.
//
// Each `git add` is wrapped in mdsmith_git_add, a bounded
// retry-with-backoff that absorbs a transient `.git/index.lock`. If a
// concurrent git invocation briefly holds the lock, `git add` fails
// with `Unable to create '.../index.lock': File exists`; retrying
// after a short wait turns a queue-bouncing hard failure into a brief
// pause. The retry only waits for the holder to release the lock — it
// never deletes `.git/index.lock`, so a lock the hook did not create
// is left untouched. When the lock outlasts the retry budget the hook
// prints `index locked` and exits non-zero so the merge aborts
// loudly rather than committing a partially staged tree. A non-lock
// `git add` failure is propagated immediately, since it will not
// clear on retry.
//
// The backoff uses `sleep 0.1 2>/dev/null || sleep 1`: fractional
// sleep is honored on GNU/BSD/macOS coreutils (fast), and the 1s
// fallback keeps the script correct on any `sleep` that accepts only
// integer seconds.
//
// The staging phase runs under `set +e` so the retry helper can
// inspect each `git add` exit status itself; an assignment from a
// failing command substitution would otherwise trip `set -e` before
// the helper could classify the failure.
func BuildHookScript(exe string) string {
	return "#!/bin/sh\n" +
		PreMergeCommitMarker + "\n" +
		"# Re-runs mdsmith fix once git has resolved every per-file\n" +
		"# merge, so generated sections reflect the final merged\n" +
		"# state of every source file. mdsmith fix walks the worktree\n" +
		"# respecting .mdsmith.yml ignore patterns — the same set\n" +
		"# marked with merge=mdsmith in .gitattributes.\n" +
		"set -e\n" +
		"cd \"$(git rev-parse --show-toplevel)\"\n" +
		stagingHelperShellFunc +
		"# `set +e` around the fix invocation so we can capture its\n" +
		"# raw exit code. `if ! cmd; then status=$?; ...` looks\n" +
		"# tempting, but POSIX `! cmd` returns the logical NOT of\n" +
		"# cmd's exit status, so `$?` immediately after is 0 when\n" +
		"# cmd exited 1 — and the `[ \"$status\" -ne 1 ]` guard\n" +
		"# would then exit before the staging loop ever runs.\n" +
		"set +e\n" +
		shellQuote(exe) + " fix --no-build .\n" +
		"status=$?\n" +
		"if [ \"$status\" -ne 0 ] && [ \"$status\" -ne 1 ]; then\n" +
		"  exit \"$status\"\n" +
		"fi\n" +
		"# Stay under `set +e`: mdsmith_git_add captures each `git add`\n" +
		"# exit status to classify a lock failure, and exits on a hard\n" +
		"# error. The `while` loop runs in the pipeline's subshell, so a\n" +
		"# `mdsmith_git_add` exit there ends only the subshell; capture\n" +
		"# the pipeline status afterward and re-raise it so a persistent\n" +
		"# lock (or other hard error) aborts the whole hook.\n" +
		"#\n" +
		"# Capture the changed-file list first and check `git diff`'s own\n" +
		"# exit status. Piping `git diff` straight into the loop would tie\n" +
		"# $? to the `while` (which exits 0 on empty input), masking a\n" +
		"# hard `git diff` failure and committing without staging fixes.\n" +
		"changed_md=$(git diff --name-only -- '*.md' '*.markdown')\n" +
		"diff_status=$?\n" +
		"if [ \"$diff_status\" -ne 0 ]; then\n" +
		"  exit \"$diff_status\"\n" +
		"fi\n" +
		"printf '%s\\n' \"$changed_md\" | " +
		"while IFS= read -r f; do\n" +
		"  if [ -n \"$f\" ]; then\n" +
		"    mdsmith_git_add \"$f\"\n" +
		"  fi\n" +
		"done\n" +
		"stage_status=$?\n" +
		"if [ \"$stage_status\" -ne 0 ]; then\n" +
		"  exit \"$stage_status\"\n" +
		"fi\n"
}

// HookMatchesCanonical reports whether hook content looks like the
// current glob-based pre-merge-commit template. The mdsmith binary
// path is repo-specific, so canonical comparison checks for the
// stable lines that carry the runtime behaviour: cd to the repo
// root, run `mdsmith fix .` inside the exit-1-tolerant guard, and
// stage modified markdown files via the POSIX `while read` loop.
// Both the CLI status output and the git-hook-sync rule call this
// so they cannot disagree on what counts as in-sync.
//
// Required fragments are matched only on non-comment lines so a
// drifted hook with the canonical commands sitting in a comment
// (or otherwise inert text) is reliably detected as drift.
func HookMatchesCanonical(hook string) bool {
	required := []string{
		`cd "$(git rev-parse --show-toplevel)"`,
		"set +e",
		" fix --no-build .",
		"status=$?",
		`if [ "$status" -ne 0 ] && [ "$status" -ne 1 ]; then`,
		`changed_md=$(git diff --name-only -- '*.md' '*.markdown')`,
		`while IFS= read -r f; do`,
		// Staging goes through the index.lock-aware retry helper. A
		// hook that drifted back to a bare `git add -- "$f"` loop
		// lacks this call and must be flagged so the lock hardening is
		// not silently lost on an out-of-date hook.
		`mdsmith_git_add "$f"`,
		// Require the helper definition itself, not just the call, so a
		// hook that dropped the `mdsmith_git_add()` function (and would
		// fail at runtime) is flagged as drift.
		`mdsmith_git_add() {`,
		// The capture lines alone are not enough: require the guards that
		// act on them, or a drifted hook could keep `stage_status=$?` yet
		// drop the exit and silently swallow a staging failure. The
		// diff-failure guard likewise keeps a hard `git diff` error from
		// being masked by the pipeline.
		"stage_status=$?",
		`if [ "$stage_status" -ne 0 ]; then`,
		`if [ "$diff_status" -ne 0 ]; then`,
	}
	for _, frag := range required {
		if !hookHasNonCommentLineContaining(hook, frag) {
			return false
		}
	}
	return true
}

// hookHasNonCommentLineContaining reports whether hook contains
// fragment on at least one line that is not blank and does not
// start with a `#` shell comment marker. Substring matching alone
// would treat a documentation comment ("# example: fix .; then")
// as canonical, masking real drift.
func hookHasNonCommentLineContaining(hook, fragment string) bool {
	for _, line := range strings.Split(hook, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if strings.Contains(line, fragment) {
			return true
		}
	}
	return false
}

// shellQuote single-quotes s for use in a POSIX shell. An embedded
// single quote is encoded as the four-byte sequence U+0027 U+005C
// U+0027 U+0027 (close-quote, backslash-escaped quote, reopen-quote)
// so the result round-trips through the shell's quoting rules. The
// sequence is spelled out by codepoint here because gofmt's godoc
// smart-quote substitution rewrites two adjacent ASCII apostrophes
// into a curly close-quote and corrupts the literal example.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// EnableRuleSnippet returns the YAML the user can paste into
// .mdsmith.yml to enable the given rule. mdsmith never rewrites the
// user's config file automatically; the snippet is printed instead.
func EnableRuleSnippet(ruleName string) string {
	return fmt.Sprintf("rules:\n  %s: true\n", ruleName)
}

// firstQuotedAfter returns the first POSIX single-quoted token that
// appears after marker in line, decoding shell-quote escapes so a
// filename containing a single quote round-trips correctly. The
// installer encodes a literal single quote inside a single-quoted
// string by closing the quote, emitting a backslash-escaped quote,
// and reopening the quote. The decoder reverses that pattern when
// it sees an unmatched continuation immediately after a closing
// quote.
//
// Returns ok=false if the marker is missing or no quoted token
// follows it.
func firstQuotedAfter(line, marker string) (string, bool) {
	idx := strings.Index(line, marker)
	if idx == -1 {
		return "", false
	}
	rest := strings.TrimSpace(line[idx+len(marker):])
	if rest == "" || rest[0] != '\'' {
		return "", false
	}
	rest = rest[1:]

	var b strings.Builder
	for {
		end := strings.IndexByte(rest, '\'')
		if end == -1 {
			return "", false
		}
		b.WriteString(rest[:end])
		rest = rest[end+1:]
		// Continuation: `\''` after a closing quote means a literal
		// single quote followed by a re-opened quoted segment.
		if strings.HasPrefix(rest, `\''`) {
			b.WriteByte('\'')
			rest = rest[3:]
			continue
		}
		break
	}
	tok := b.String()
	if tok == "" {
		return "", false
	}
	return tok, true
}
