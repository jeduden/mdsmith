// Package gitattributes reads and writes the mdsmith-managed
// merge-driver block in a repository's .gitattributes file: deriving
// the include/exclude glob set from .mdsmith.yml, rendering and
// parsing the managed block, and staging the file with git after a
// rewrite.
package gitattributes

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/mdpath"
	"github.com/jeduden/mdsmith/internal/oscompat"
)

// readFile, atomicWriteFn, and lstatFile are variables so tests can substitute
// failing implementations to exercise error paths in WriteGitattributes.
var readFile = os.ReadFile
var atomicWriteFn = atomicWriteGitattributes
var lstatFile = os.Lstat

// GlobsFromConfig returns the canonical merge-driver glob set for a
// repository: every markdown extension is included, and the project's
// .mdsmith.yml ignore patterns are translated into markdown-scoped
// exclude patterns. Last-match-wins in .gitattributes lets the excludes
// override the broader markdown includes. cfg may be nil (no exclusions
// then).
//
// Each representable ignore pattern is intersected with the include
// set's markdown extensions before it becomes an exclude line, so a
// coarse directory ignore like `demo/**` emits
//
//	demo/**/*.md merge=text
//	demo/**/*.markdown merge=text
//
// rather than a bare `demo/** merge=text`. The `ignore:` list scopes
// the markdown linter (files: is *.md / *.markdown), so an ignore
// pattern is only ever meant to affect markdown; a bare exclude line,
// by contrast, is not extension-scoped and would change git's merge
// behaviour for every file in the tree — including source code that
// nobody asked to grandfather (issue #750). scopeExcludeToMarkdown
// guarantees every emitted exclude ends in a markdown extension.
//
// The exclude lines carry `merge=text` (git's built-in 3-way text
// merge), not `-merge` (merge unset). Both take the mdsmith driver out
// of the picture for the path, but `-merge` also forfeits git's text
// merge and leaves the file binary-conflicting; the ignore-derived
// paths are Markdown with no generated sections, so `merge=text` is
// the correct, conflict-avoiding fallback (issue #755).
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
	exts := mdpath.Extensions()
	g.Exclude = make([]string, 0, len(cfg.Ignore)*len(exts))
	var skipped []string
	for _, p := range cfg.Ignore {
		if !isRepresentableGitattributesPattern(p) {
			skipped = append(skipped, p)
			continue
		}
		g.Exclude = append(g.Exclude, scopeExcludeToMarkdown(p, exts)...)
	}
	return g, skipped
}

// scopeExcludeToMarkdown rewrites one .mdsmith.yml ignore pattern into
// the .gitattributes exclude patterns that fall back to git's built-in
// text merge (merge=text) for the Markdown files the pattern
// grandfathers — and only those files (issue #750). The returned
// patterns are the path field; RenderManagedBlock appends the
// `merge=text` attribute.
//
// A pattern that already ends in one of exts targets a specific
// Markdown extension, so its exclude line can only affect Markdown; it
// is returned unchanged. Otherwise the pattern is treated as a path
// scope and one exclude is emitted per extension, keyed on the final
// path segment:
//
//   - `dir/**` -> `dir/**/*.md`, `dir/**/*.markdown` (recursive tree)
//   - `dir/`   -> `dir/**/*.md`, `dir/**/*.markdown` (trailing slash)
//   - `dir/*`  -> `dir/*.md`,    `dir/*.markdown`    (one level)
//   - `dir`    -> `dir/**/*.md`, `dir/**/*.markdown` (name as a tree)
//
// Every branch appends a Markdown extension to the emitted pattern, so
// the invariant "a derived exclude line never matches a non-Markdown
// file" holds for any input.
func scopeExcludeToMarkdown(pattern string, exts []string) []string {
	for _, ext := range exts {
		if strings.HasSuffix(pattern, ext) {
			return []string{pattern}
		}
	}
	dir, last := splitLastSegment(pattern)
	var base string
	switch last {
	case "**", "":
		// Recursive tree (`dir/**`) or a bare directory with a trailing
		// slash (`dir/`, where the final segment is empty): all Markdown
		// under dir at any depth. Folding the empty segment in here also
		// avoids the `dir//**` double slash the default branch would
		// produce for a trailing-slash pattern.
		base = dir + "**/*"
	case "*":
		// Single segment (`dir/*`): Markdown directly under dir.
		base = dir + "*"
	default:
		// A plain directory name (or any other shape) is treated as a
		// directory scope covering all Markdown beneath it.
		base = pattern + "/**/*"
	}
	out := make([]string, 0, len(exts))
	for _, ext := range exts {
		out = append(out, base+ext)
	}
	return out
}

// splitLastSegment splits p at the final "/" into the directory prefix
// (including the trailing slash; empty when p has no slash) and the
// final path segment.
func splitLastSegment(p string) (dir, last string) {
	if i := strings.LastIndexByte(p, '/'); i >= 0 {
		return p[:i+1], p[i+1:]
	}
	return "", p
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
	cfg, err := config.Load(config.DefaultConfigPath(repoRoot))
	if err != nil {
		g, _ := GlobsFromConfig(nil)
		return g
	}
	g, _ := GlobsFromConfig(cfg)
	return g
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
// merge=text`. .gitattributes uses last-match-wins, so an exclude line
// after the include lines effectively removes the mdsmith merge driver
// from any path the include patterns matched, falling back to git's
// built-in 3-way text merge.
//
// Exclude patterns derived from .mdsmith.yml ignore entries are
// markdown-scoped (see GlobsFromConfig): a `merge=text` line only ever
// matches a markdown file, so it takes the mdsmith driver off
// grandfathered markdown without touching git's default merge for
// non-markdown files in the same tree (issue #750). Emitting
// `merge=text` rather than `-merge` also keeps git's text merge for
// those files instead of declaring them binary-conflicting (#755).
//
// ExtractGlobs still reads the legacy `-merge` exclude form so a
// `.gitattributes` written by an older mdsmith round-trips unchanged.
//
// `.gitattributes` itself does not support negative patterns (`!*.md`
// is a syntax error there). Order-sensitive override via a trailing
// exclude line is the supported way to express exclusions, which is
// why Globs keeps Include and Exclude as separate ordered slices.
type Globs struct {
	Include []string
	Exclude []string
}

// DefaultIncludes is the canonical merge-driver include pattern set:
// one basename glob per markdown extension mdsmith processes. It is
// derived from mdpath, the single source of truth for the extension
// set, and returns a fresh slice each call so callers may mutate it
// without sharing a package-level value.
func DefaultIncludes() []string {
	return mdpath.FileGlobs()
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
		// Emit `merge=text`, not `-merge`. Both turn the mdsmith driver
		// off for the path (last-match-wins over the include lines), but
		// `-merge` also unsets git's built-in merge — declaring the file
		// has no well-defined merge semantics, which is only correct for
		// binaries and leaves these Markdown files binary-conflicting.
		// `merge=text` selects git's built-in 3-way text merge instead.
		// The excludes are ignore-derived Markdown paths (see
		// GlobsFromConfig): they are never linted, so they carry no
		// generated sections for the driver to reconcile, and the text
		// merge is strictly safe (issue #755).
		fmt.Fprintf(&b, "%s merge=text\n", p)
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
			case "merge=text", "-merge":
				// `merge=text` is the current exclude form; `-merge` is
				// the legacy form still found in blocks written by an
				// older mdsmith (or the pinned CI baseline). Both mean
				// "turn the mdsmith driver off for this path", so both
				// map to an exclude — an unmigrated `-merge` block then
				// extracts to the same glob set as a freshly rendered
				// `merge=text` one and does not read as drift (#755).
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

// createTempFn, syncTempFn, closeTempFn, chmodFile, and fstatFn are variables
// so tests can inject failures into atomicWriteGitattributes without OS tricks.
// chmodFile delegates to oscompat.Chmod so the tinygo/wasm build uses a no-op
// without pulling in os.Chmod directly.
var chmodFile = oscompat.Chmod
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
	if err := chmodFile(tmpName, mode); err != nil {
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
