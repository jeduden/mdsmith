// Package directivefiles discovers Markdown files in a workspace that
// contain a generated-section directive marker (catalog, include,
// toc, …), for consumers — the merge-driver and pre-merge-commit
// install commands, and the git-hook-sync rule — that need to know
// which files a repo-wide `mdsmith fix` would touch.
package directivefiles

import (
	"bytes"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jeduden/mdsmith/internal/archetype/gensection"
	"github.com/jeduden/mdsmith/internal/bytelimit"
	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/mdpath"
	"github.com/jeduden/mdsmith/internal/rule"
)

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
	if cfg, err := config.Load(config.DefaultConfigPath(repoRoot)); err == nil {
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
		if !mdpath.IsMarkdownPath(name) {
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
