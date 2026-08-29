// Package githooks generates and validates the mdsmith
// pre-merge-commit hook script — the marker that identifies a
// mdsmith-managed hook, the canonical script content, and drift
// detection against that canonical form.
package githooks

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/jeduden/mdsmith/internal/setutil"
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
