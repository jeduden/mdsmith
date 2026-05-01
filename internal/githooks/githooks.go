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

	"github.com/jeduden/mdsmith/internal/archetype/gensection"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
)

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
	directiveNames := make([]string, 0)
	for _, r := range rule.All() {
		if d, ok := r.(gensection.Directive); ok {
			directiveNames = append(directiveNames, d.Name())
		}
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
		content, err := lint.ReadFileLimited(path, maxBytes)
		if err != nil {
			return nil
		}
		// Detect real directive markers line-by-line via the marker
		// parser so prose/inline-code mentions of `<?catalog?>` do
		// not bloat the discovered set.
		if !hasDirectiveMarker(content, directiveNames) {
			return nil
		}
		rel, err := filepath.Rel(repoRoot, path)
		if err != nil {
			return nil
		}
		key := filepath.ToSlash(rel)
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
	setA := toSet(a)
	setB := toSet(b)
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

func toSet(s []string) map[string]struct{} {
	m := make(map[string]struct{}, len(s))
	for _, v := range s {
		m[v] = struct{}{}
	}
	return m
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

// stripStaleMergeMdsmithLines drops any line that assigns merge=mdsmith
// outside the managed block. ExtractGitattributesFiles treats every
// merge=mdsmith line as installed, so leaving stray entries would make
// the file appear out of sync immediately after a fix and could leave
// duplicates from older append-only installs.
func stripStaleMergeMdsmithLines(content string) string {
	lines := strings.Split(content, "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[len(fields)-1] == "merge=mdsmith" {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}

// WriteGitattributes updates .gitattributes to register the given files
// with the mdsmith merge driver. It preserves all non-mdsmith entries and
// updates only the mdsmith-managed block (delimited by BEGIN/END markers).
// Stray `merge=mdsmith` lines outside the managed block (left behind by
// older append-only installs or hand-edited files) are removed so the
// resulting file matches the provided files exactly.
//
// If the file does not exist, it is created with only the managed block.
// If the file exists but has no managed block, one is appended.
// If a managed block exists, it is replaced with the new file list.
//
// This approach ensures that other .gitattributes entries (e.g., text,
// eol=lf, linguist settings, other merge drivers) are never dropped.
func WriteGitattributes(path string, files []string) error {
	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("reading %s: %w", path, err)
	}

	// Build the new managed block
	var managedBlock strings.Builder
	managedBlock.WriteString(gitattributesManagedBlockStart)
	managedBlock.WriteString("\n")
	for _, f := range files {
		fmt.Fprintf(&managedBlock, "%s merge=mdsmith\n", f)
	}
	managedBlock.WriteString(gitattributesManagedBlockEnd)
	managedBlock.WriteString("\n")

	var newContent strings.Builder

	if len(existing) == 0 {
		// New file: just write the managed block
		newContent.WriteString(managedBlock.String())
	} else {
		// Existing file: preserve non-mdsmith content, replace managed
		// block. Strip stale merge=mdsmith lines from the surrounding
		// text independently so the original ordering of unrelated
		// entries (text, eol=lf, linguist settings) is preserved.
		content := string(existing)
		startIdx := strings.Index(content, gitattributesManagedBlockStart)
		endIdx := strings.Index(content, gitattributesManagedBlockEnd)

		var before, after string
		if startIdx == -1 || endIdx == -1 || endIdx < startIdx {
			// No valid managed block: everything is "before"; the new
			// block is appended at the end after the preserved content.
			before = stripStaleMergeMdsmithLines(content)
			newContent.WriteString(before)
			if before != "" && !strings.HasSuffix(before, "\n") {
				newContent.WriteString("\n")
			}
			newContent.WriteString(managedBlock.String())
		} else {
			endLineEnd := strings.Index(content[endIdx:], "\n")
			if endLineEnd == -1 {
				endLineEnd = len(content)
			} else {
				endLineEnd += endIdx + 1 // Include the newline
			}
			before = stripStaleMergeMdsmithLines(content[:startIdx])
			after = stripStaleMergeMdsmithLines(content[endLineEnd:])
			newContent.WriteString(before)
			newContent.WriteString(managedBlock.String())
			newContent.WriteString(after)
		}
	}

	return os.WriteFile(path, []byte(newContent.String()), 0644)
}

// StageGitattributes runs `git add -- .gitattributes` against repoRoot
// so updates written by Fix end up in the index. Without this, the
// pre-merge-commit hook flow stages only the markdown file passed to
// `mdsmith fix`, leaving the regenerated .gitattributes in the working
// tree but absent from the resulting merge commit. Errors are surfaced
// so callers can decide whether to roll back; the working-tree write
// itself is already done at the point this is called.
func StageGitattributes(repoRoot string) error {
	return exec.Command("git", "-C", repoRoot, "add", "--", ".gitattributes").Run()
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
