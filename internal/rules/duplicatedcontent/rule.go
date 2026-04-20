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

// ID implements rule.Rule.
func (r *Rule) ID() string { return "MDS037" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "duplicated-content" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "content" }

// Check implements rule.Rule.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	if f.AST == nil {
		return nil
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

	includeMatchers, err := compileMatchers(r.Include)
	if err != nil {
		return []lint.Diagnostic{configDiag(f, r, err)}
	}
	excludeMatchers, err := compileMatchers(r.Exclude)
	if err != nil {
		return []lint.Diagnostic{configDiag(f, r, err)}
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
func resolveCorpus(f *lint.File) (fs.FS, string) {
	if f.RootFS != nil && f.RootDir != "" {
		rel, err := filepath.Rel(f.RootDir, f.Path)
		if err == nil && !strings.HasPrefix(rel, "..") {
			return f.RootFS, filepath.ToSlash(rel)
		}
	}
	if f.FS != nil {
		return f.FS, filepath.Base(f.Path)
	}
	return nil, ""
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
	return strings.HasSuffix(strings.ToLower(p), ".md")
}

func matchesFilters(path string, include, exclude []glob.Glob) bool {
	for _, g := range exclude {
		if g.Match(path) {
			return false
		}
	}
	if len(include) == 0 {
		return true
	}
	for _, g := range include {
		if g.Match(path) {
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
