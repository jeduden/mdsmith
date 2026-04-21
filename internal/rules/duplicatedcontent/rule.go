// Package duplicatedcontent implements MDS037, which flags substantial
// paragraphs that also appear verbatim in another Markdown file in the
// project root after whitespace and case normalization.
package duplicatedcontent

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/gobwas/glob"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/jeduden/mdsmith/internal/rules/settings"
	"github.com/yuin/goldmark/ast"
)

// defaultMinChars is the minimum normalized paragraph length (in runes)
// that makes a paragraph large enough to be worth flagging as a duplicate.
// Below this threshold paragraphs like "See [foo](bar)." accumulate too
// many coincidental matches across a documentation corpus.
const defaultMinChars = 200

func init() {
	rule.Register(&Rule{})
}

// Rule detects paragraphs duplicated across Markdown files in the corpus.
type Rule struct {
	Include  []string
	Exclude  []string
	MinChars int
}

// EnabledByDefault implements rule.Defaultable. MDS037 is opt-in: in a
// project that intentionally shares prose across agent files (AGENTS.md,
// CLAUDE.md, .github/copilot-instructions.md, include-expanded docs) the
// default behavior would fire on every boilerplate paragraph. Projects
// that want duplication checks enable it explicitly in `.mdsmith.yml`.
func (r *Rule) EnabledByDefault() bool { return false }

// ID implements rule.Rule.
func (r *Rule) ID() string { return "MDS037" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "duplicated-content" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "meta" }

// Check implements rule.Rule.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	if f.AST == nil {
		return nil
	}

	// Validate config first so bad globs surface even on files that
	// contain no qualifying paragraphs.
	includeMatchers, err := compileMatchers(r.Include)
	if err != nil {
		return []lint.Diagnostic{configDiag(f, r, err)}
	}
	excludeMatchers, err := compileMatchers(r.Exclude)
	if err != nil {
		return []lint.Diagnostic{configDiag(f, r, err)}
	}

	minChars := r.MinChars
	if minChars <= 0 {
		minChars = defaultMinChars
	}

	self := extractParagraphs(f, minChars)
	if len(self) == 0 {
		return nil
	}

	corpus, selfName := resolveCorpus(f)
	if corpus == nil {
		return nil
	}

	index := buildCorpusIndex(
		corpus, selfName, f.MaxInputBytes, minChars,
		includeMatchers, excludeMatchers,
	)

	var diags []lint.Diagnostic
	for _, p := range self {
		matches, ok := index[p.fingerprint]
		if !ok {
			continue
		}
		for _, m := range matches {
			diags = append(diags, lint.Diagnostic{
				File:     f.Path,
				Line:     p.line,
				Column:   1,
				RuleID:   r.ID(),
				RuleName: r.Name(),
				Severity: lint.Warning,
				Message: fmt.Sprintf(
					"paragraph duplicated in %s:%d",
					m.path, m.line,
				),
			})
		}
	}
	return diags
}

// paragraph is a fingerprinted paragraph in a single file.
type paragraph struct {
	fingerprint string
	line        int
}

// externalMatch is a paragraph match found in another file. The line is
// already adjusted for the other file's front-matter offset.
type externalMatch struct {
	path string
	line int
}

// extractParagraphs walks f.AST and returns fingerprints for every
// paragraph whose normalized text is at least minChars runes long.
// Paragraphs are read via Node.Lines so raw markdown text — not rendered
// inline output — feeds the fingerprint.
func extractParagraphs(f *lint.File, minChars int) []paragraph {
	var out []paragraph
	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if n.Kind() != ast.KindParagraph {
			return ast.WalkContinue, nil
		}
		text, startOffset, ok := nodeText(n, f.Source)
		if !ok {
			return ast.WalkSkipChildren, nil
		}
		normalized := normalize(text)
		if runeLen(normalized) < minChars {
			return ast.WalkSkipChildren, nil
		}
		sum := sha256.Sum256([]byte(normalized))
		out = append(out, paragraph{
			fingerprint: hex.EncodeToString(sum[:]),
			line:        f.LineOfOffset(startOffset),
		})
		return ast.WalkSkipChildren, nil
	})
	return out
}

// nodeText concatenates a block node's line segments into the raw text
// that the source contains between the node's first and last line. It
// returns the first line's byte offset so callers can compute its line.
func nodeText(n ast.Node, source []byte) (string, int, bool) {
	lines := n.Lines()
	if lines.Len() == 0 {
		return "", 0, false
	}
	var b strings.Builder
	for i := 0; i < lines.Len(); i++ {
		seg := lines.At(i)
		b.Write(seg.Value(source))
	}
	return b.String(), lines.At(0).Start, true
}

// normalize collapses runs of whitespace to single spaces, lowercases
// letters, and trims leading/trailing space. The goal is to treat
// paragraphs that differ only by reflow or case as duplicates.
func normalize(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	inSpace := false
	for _, r := range s {
		if unicode.IsSpace(r) {
			if !inSpace && b.Len() > 0 {
				b.WriteRune(' ')
			}
			inSpace = true
			continue
		}
		b.WriteRune(unicode.ToLower(r))
		inSpace = false
	}
	return strings.TrimSpace(b.String())
}

func runeLen(s string) int {
	n := 0
	for range s {
		n++
	}
	return n
}

// resolveCorpus picks the filesystem to scan and the path of the current
// file within it. RootFS (the project root) is preferred; otherwise the
// file's own directory is used. The returned selfName is forward-slash,
// fs.FS-style so it can be compared to fs.WalkDir's path argument.
//
// f.Path may be absolute (CLI runs with a discovered root) or relative
// to the project root (ResolveFiles returns things like "./docs/a.md").
// Absolute paths go through filepath.Rel; relative paths are cleaned
// and slashed in place. Either way, a self-path that escapes RootDir
// (starts with "..") falls through to the FS scope rather than
// walking the whole project root behind the user's back.
func resolveCorpus(f *lint.File) (fs.FS, string) {
	if f.RootFS != nil && f.RootDir != "" {
		if selfName, ok := rootRelative(f.RootDir, f.Path); ok {
			return f.RootFS, selfName
		}
	}
	if f.FS != nil {
		return f.FS, filepath.Base(f.Path)
	}
	return nil, ""
}

// rootRelative returns path expressed relative to rootDir using forward
// slashes, or ok=false when path escapes rootDir. Already-relative paths
// are assumed to be rooted at rootDir and only cleaned; absolute paths
// go through filepath.Rel.
func rootRelative(rootDir, path string) (string, bool) {
	var rel string
	if filepath.IsAbs(path) {
		r, err := filepath.Rel(rootDir, path)
		if err != nil {
			return "", false
		}
		rel = r
	} else {
		rel = filepath.Clean(path)
	}
	slash := filepath.ToSlash(rel)
	slash = strings.TrimPrefix(slash, "./")
	if slash == ".." || strings.HasPrefix(slash, "../") {
		return "", false
	}
	return slash, true
}

// buildCorpusIndex walks corpus for .md files (excluding selfName) and
// returns a map from paragraph fingerprint to every occurrence found.
// Files that can't be read or parsed are silently skipped — this rule is
// advisory and should never fail a run because a sibling file is
// malformed or oversize.
func buildCorpusIndex(
	corpus fs.FS,
	selfName string,
	maxBytes int64,
	minChars int,
	include, exclude []glob.Glob,
) map[string][]externalMatch {
	index := make(map[string][]externalMatch)
	_ = fs.WalkDir(corpus, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			// Prune excluded subtrees so a walk that would otherwise
			// recurse into `.git/`, `node_modules/`, or any user-
			// configured `exclude` entry bails out at the directory
			// boundary instead of scanning every file inside it.
			if path != "." && shouldSkipDir(path, exclude) {
				return fs.SkipDir
			}
			return nil
		}
		if !isMarkdownPath(path) {
			return nil
		}
		if path == selfName {
			return nil
		}
		if !matchesFilters(path, include, exclude) {
			return nil
		}
		data, err := lint.ReadFSFileLimited(corpus, path, maxBytes)
		if err != nil {
			return nil
		}
		other, err := lint.NewFileFromSource(path, data, true)
		if err != nil {
			return nil
		}
		for _, p := range extractParagraphs(other, minChars) {
			index[p.fingerprint] = append(index[p.fingerprint], externalMatch{
				path: path,
				line: p.line + other.LineOffset,
			})
		}
		return nil
	})

	// Sort each fingerprint's matches so diagnostics are deterministic.
	for fp, matches := range index {
		sort.Slice(matches, func(i, j int) bool {
			if matches[i].path != matches[j].path {
				return matches[i].path < matches[j].path
			}
			return matches[i].line < matches[j].line
		})
		index[fp] = matches
	}
	return index
}

func isMarkdownPath(p string) bool {
	lower := strings.ToLower(p)
	return strings.HasSuffix(lower, ".md") ||
		strings.HasSuffix(lower, ".markdown")
}

// shouldSkipDir reports whether a directory path matches one of the
// exclude globs and should be pruned from the walk. Matching the
// slash path lets patterns like "vendor/**" hit at the directory
// boundary; matching basename lets ".git" or "node_modules" skip
// wherever they appear in the tree. Include globs are intentionally
// not consulted here: excluding a subtree is safe, but a missing
// include match at the directory level would skip subtree entries
// that individual include globs could still allow.
func shouldSkipDir(path string, exclude []glob.Glob) bool {
	if len(exclude) == 0 {
		return false
	}
	slashPath := filepath.ToSlash(path)
	base := filepath.Base(path)
	for _, g := range exclude {
		if g.Match(slashPath) || g.Match(base) {
			return true
		}
	}
	return false
}

// matchesFilters reports whether path is allowed by include/exclude.
// To stay consistent with MDS027 cross-file-reference-integrity,
// patterns are matched against both the full forward-slash path and
// the basename, so `"draft.md"` excludes a file regardless of which
// directory it sits in.
func matchesFilters(path string, include, exclude []glob.Glob) bool {
	slashPath := filepath.ToSlash(path)
	base := filepath.Base(path)
	for _, g := range exclude {
		if g.Match(slashPath) || g.Match(base) {
			return false
		}
	}
	if len(include) == 0 {
		return true
	}
	for _, g := range include {
		if g.Match(slashPath) || g.Match(base) {
			return true
		}
	}
	return false
}

func compileMatchers(patterns []string) ([]glob.Glob, error) {
	out := make([]glob.Glob, 0, len(patterns))
	for _, pat := range patterns {
		g, err := glob.Compile(pat, '/')
		if err != nil {
			return nil, fmt.Errorf("invalid glob pattern %q: %w", pat, err)
		}
		out = append(out, g)
	}
	return out, nil
}

func configDiag(f *lint.File, r *Rule, err error) lint.Diagnostic {
	return lint.Diagnostic{
		File:     f.Path,
		Line:     1,
		Column:   1,
		RuleID:   r.ID(),
		RuleName: r.Name(),
		Severity: lint.Error,
		Message:  "duplicated-content: " + err.Error(),
	}
}

// ApplySettings implements rule.Configurable.
func (r *Rule) ApplySettings(cfg map[string]any) error {
	for k, v := range cfg {
		switch k {
		case "include":
			list, ok := settings.ToStringSlice(v)
			if !ok {
				return fmt.Errorf(
					"duplicated-content: include must be a list of strings, got %T",
					v,
				)
			}
			r.Include = list
		case "exclude":
			list, ok := settings.ToStringSlice(v)
			if !ok {
				return fmt.Errorf(
					"duplicated-content: exclude must be a list of strings, got %T",
					v,
				)
			}
			r.Exclude = list
		case "min-chars":
			n, ok := settings.ToInt(v)
			if !ok {
				return fmt.Errorf(
					"duplicated-content: min-chars must be an integer, got %T",
					v,
				)
			}
			if n <= 0 {
				// Check treats a zero MinChars as "unset" and applies
				// defaultMinChars, so an explicit 0 in config would be
				// silently ignored; reject it at validation time to
				// keep ApplySettings and Check consistent.
				return fmt.Errorf(
					"duplicated-content: min-chars must be > 0, got %d",
					n,
				)
			}
			r.MinChars = n
		default:
			return fmt.Errorf("duplicated-content: unknown setting %q", k)
		}
	}

	if _, err := compileMatchers(r.Include); err != nil {
		return fmt.Errorf(
			"duplicated-content: include has invalid glob pattern: %w",
			err,
		)
	}
	if _, err := compileMatchers(r.Exclude); err != nil {
		return fmt.Errorf(
			"duplicated-content: exclude has invalid glob pattern: %w",
			err,
		)
	}
	return nil
}

// DefaultSettings implements rule.Configurable.
func (r *Rule) DefaultSettings() map[string]any {
	return map[string]any{
		"include":   []string{},
		"exclude":   []string{},
		"min-chars": defaultMinChars,
	}
}

var _ rule.Configurable = (*Rule)(nil)
