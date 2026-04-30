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
// Hidden directories (names starting with ".") are skipped, matching
// the behavior of the original CLI helper. Falls back to
// ["PLAN.md", "README.md"] when discovery yields no files.
func DiscoverFiles(repoRoot string, maxBytes int64) []string {
	directiveNames := make([]string, 0)
	for _, r := range rule.All() {
		if d, ok := r.(gensection.Directive); ok {
			directiveNames = append(directiveNames, d.Name())
		}
	}

	seen := make(map[string]struct{})
	var files []string
	err := filepath.Walk(repoRoot, func(path string, info os.FileInfo, err error) error {
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

	if err != nil || len(files) == 0 {
		return []string{"PLAN.md", "README.md"}
	}
	// Sort so the file list is stable across platforms and
	// filesystems; the result is printed to users and embedded into
	// the pre-merge-commit hook and .gitattributes, where churn
	// hurts review diffs.
	sort.Strings(files)
	return files
}

// hasDirectiveMarker reports whether content contains a real
// processing-instruction start or end marker for any of the named
// directives. It scans line-by-line so a backticked or otherwise
// inline mention like `<?catalog?>` in prose is not treated as a
// directive.
func hasDirectiveMarker(content []byte, names []string) bool {
	for _, line := range bytes.Split(content, []byte("\n")) {
		for _, n := range names {
			if gensection.IsRawStartMarker(line, n) || gensection.IsRawEndMarker(line, n) {
				return true
			}
		}
	}
	return false
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
// most one entry: the first single-quoted token that follows.
func ExtractHookFiles(content string) []string {
	var files []string
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
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

// HasMdsmithMergeDriver reports whether the repository's local git
// config defines `merge.mdsmith.driver` (i.e. the merge driver itself
// has been registered). A missing driver is reported as false rather
// than as an error so callers can treat the merge-driver setup as
// "not installed".
func HasMdsmithMergeDriver(repoRoot string) bool {
	out, err := exec.Command("git", "-C", repoRoot, "config", "--get", "merge.mdsmith.driver").Output()
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
