package crossfilereferenceintegrity

import (
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/gobwas/glob"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/yuin/goldmark/ast"
)

func init() {
	rule.Register(&Rule{})
}

// Rule checks Markdown links for missing target files and missing heading
// anchors in linked Markdown files.
type Rule struct {
	Include []string
	Exclude []string
	Strict  bool
}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "MDS027" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "cross-file-reference-integrity" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "link" }

// Check implements rule.Rule.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	// Stdin/source-only checks have no stable filesystem context.
	if f.FS == nil {
		return nil
	}

	includeMatchers, err := compileMatchers(r.Include)
	if err != nil {
		return []lint.Diagnostic{configDiag(f.Path, r, err)}
	}
	excludeMatchers, err := compileMatchers(r.Exclude)
	if err != nil {
		return []lint.Diagnostic{configDiag(f.Path, r, err)}
	}

	selfAnchors := collectHeadingAnchors(f)
	anchorCache := map[string]map[string]bool{"self": selfAnchors}

	var diags []lint.Diagnostic
	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		linkNode, ok := n.(*ast.Link)
		if !ok {
			return ast.WalkContinue, nil
		}

		diags = append(diags, r.checkLink(
			f,
			linkNode,
			includeMatchers,
			excludeMatchers,
			selfAnchors,
			anchorCache,
		)...)

		return ast.WalkContinue, nil
	})

	return diags
}

func (r *Rule) checkLink(
	f *lint.File,
	linkNode *ast.Link,
	includeMatchers []glob.Glob,
	excludeMatchers []glob.Glob,
	selfAnchors map[string]bool,
	anchorCache map[string]map[string]bool,
) []lint.Diagnostic {
	target, ok := parseTarget(string(linkNode.Destination))
	if !ok {
		return nil
	}

	line, col := linkPosition(f, linkNode)

	if target.LocalAnchor {
		if target.Anchor == "" {
			return nil
		}
		if selfAnchors[normalizeAnchor(target.Anchor)] {
			return nil
		}
		return []lint.Diagnostic{brokenHeadingDiag(f.Path, line, col, r, target.Raw)}
	}

	linkPath := normalizeLinkPath(target.Path)
	if linkPath == "" || filepath.IsAbs(linkPath) {
		return nil
	}

	if !r.Strict && !isMarkdownPath(linkPath) {
		return nil
	}

	if !matchesPathFilters(linkPath, includeMatchers, excludeMatchers) {
		return nil
	}

	targetFile, ok := resolveTargetFile(f, linkPath)
	if !ok {
		return []lint.Diagnostic{brokenFileDiag(f.Path, line, col, r, target.Raw)}
	}

	if target.Anchor == "" || !isMarkdownPath(linkPath) {
		return nil
	}

	targetAnchors, err := anchorsForFile(targetFile, anchorCache)
	if err != nil {
		return []lint.Diagnostic{brokenFileDiag(f.Path, line, col, r, target.Raw)}
	}
	if targetAnchors[normalizeAnchor(target.Anchor)] {
		return nil
	}
	return []lint.Diagnostic{brokenHeadingDiag(f.Path, line, col, r, target.Raw)}
}

// ApplySettings implements rule.Configurable.
func (r *Rule) ApplySettings(settings map[string]any) error {
	for k, v := range settings {
		switch k {
		case "include":
			list, ok := toStringSlice(v)
			if !ok {
				return fmt.Errorf(
					"cross-file-reference-integrity: include must be a list of strings, got %T",
					v,
				)
			}
			r.Include = list
		case "exclude":
			list, ok := toStringSlice(v)
			if !ok {
				return fmt.Errorf(
					"cross-file-reference-integrity: exclude must be a list of strings, got %T",
					v,
				)
			}
			r.Exclude = list
		case "strict":
			b, ok := v.(bool)
			if !ok {
				return fmt.Errorf(
					"cross-file-reference-integrity: strict must be a bool, got %T",
					v,
				)
			}
			r.Strict = b
		default:
			return fmt.Errorf(
				"cross-file-reference-integrity: unknown setting %q",
				k,
			)
		}
	}

	if _, err := compileMatchers(r.Include); err != nil {
		return fmt.Errorf(
			"cross-file-reference-integrity: include has invalid glob pattern: %w",
			err,
		)
	}
	if _, err := compileMatchers(r.Exclude); err != nil {
		return fmt.Errorf(
			"cross-file-reference-integrity: exclude has invalid glob pattern: %w",
			err,
		)
	}

	return nil
}

// DefaultSettings implements rule.Configurable.
func (r *Rule) DefaultSettings() map[string]any {
	return map[string]any{
		"include": []string{},
		"exclude": []string{},
		"strict":  false,
	}
}

type targetFile struct {
	cacheKey string
	read     func() ([]byte, error)
}

func anchorsForFile(target targetFile, cache map[string]map[string]bool) (map[string]bool, error) {
	if anchors, ok := cache[target.cacheKey]; ok {
		return anchors, nil
	}

	data, err := target.read()
	if err != nil {
		return nil, err
	}

	file, err := lint.NewFileFromSource(target.cacheKey, data, true)
	if err != nil {
		return nil, err
	}

	anchors := collectHeadingAnchors(file)
	cache[target.cacheKey] = anchors
	return anchors, nil
}

func resolveTargetFile(f *lint.File, linkPath string) (targetFile, bool) {
	if path, ok := resolveTargetOSPath(f.Path, linkPath); ok {
		if _, err := os.Stat(path); err == nil {
			return targetFile{
				cacheKey: "os:" + path,
				read: func() ([]byte, error) {
					return os.ReadFile(path)
				},
			}, true
		}
	}

	fsPath := filepath.ToSlash(linkPath)
	fsPath = strings.TrimPrefix(fsPath, "./")
	if fsPath == "" || strings.HasPrefix(fsPath, "/") {
		return targetFile{}, false
	}
	if _, err := fs.Stat(f.FS, fsPath); err != nil {
		return targetFile{}, false
	}
	return targetFile{
		cacheKey: "fs:" + fsPath,
		read: func() ([]byte, error) {
			return fs.ReadFile(f.FS, fsPath)
		},
	}, true
}

func resolveTargetOSPath(sourcePath, linkPath string) (string, bool) {
	if sourcePath == "" || sourcePath == "." {
		return "", false
	}

	sep := string(filepath.Separator)
	hasDir := filepath.IsAbs(sourcePath) || strings.Contains(sourcePath, sep)
	if !hasDir {
		return "", false
	}

	return filepath.Clean(filepath.Join(filepath.Dir(sourcePath), linkPath)), true
}

func collectHeadingAnchors(f *lint.File) map[string]bool {
	anchors := make(map[string]bool)
	seen := make(map[string]int)

	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		h, ok := n.(*ast.Heading)
		if !ok {
			return ast.WalkContinue, nil
		}

		slug := slugify(extractText(h, f.Source))
		if slug == "" {
			return ast.WalkContinue, nil
		}

		count := seen[slug]
		anchor := slug
		if count > 0 {
			anchor = fmt.Sprintf("%s-%d", slug, count)
		}
		seen[slug] = count + 1
		anchors[anchor] = true

		return ast.WalkContinue, nil
	})

	return anchors
}

func extractText(node ast.Node, source []byte) string {
	var b strings.Builder
	appendNodeText(&b, node, source)
	return b.String()
}

func appendNodeText(b *strings.Builder, node ast.Node, source []byte) {
	switch n := node.(type) {
	case *ast.Text:
		b.Write(n.Segment.Value(source))
		return
	case *ast.String:
		b.Write(n.Value)
		return
	}

	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		appendNodeText(b, child, source)
	}
}

func slugify(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return ""
	}

	var b strings.Builder
	prevDash := false
	for _, r := range s {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			prevDash = false
		case unicode.IsSpace(r) || r == '-' || r == '_':
			if b.Len() > 0 && !prevDash {
				b.WriteByte('-')
				prevDash = true
			}
		}
	}

	return strings.Trim(b.String(), "-")
}

func normalizeAnchor(anchor string) string {
	decoded, err := url.PathUnescape(anchor)
	if err == nil {
		anchor = decoded
	}
	return slugify(anchor)
}

func isMarkdownPath(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".md" || ext == ".markdown"
}

type linkTarget struct {
	Raw         string
	Path        string
	Anchor      string
	LocalAnchor bool
}

func parseTarget(dest string) (linkTarget, bool) {
	dest = strings.TrimSpace(dest)
	if dest == "" || strings.HasPrefix(dest, "//") {
		return linkTarget{}, false
	}

	u, err := url.Parse(dest)
	if err != nil {
		return linkTarget{}, false
	}
	if u.Scheme != "" || u.Host != "" {
		return linkTarget{}, false
	}

	path := u.Path
	if path == "" && u.Opaque != "" {
		path = u.Opaque
	}

	if path == "" && u.Fragment != "" {
		return linkTarget{
			Raw:         dest,
			Anchor:      u.Fragment,
			LocalAnchor: true,
		}, true
	}

	if path == "" {
		return linkTarget{}, false
	}

	return linkTarget{
		Raw:    dest,
		Path:   path,
		Anchor: u.Fragment,
	}, true
}

func normalizeLinkPath(linkPath string) string {
	decoded, err := url.PathUnescape(linkPath)
	if err == nil {
		linkPath = decoded
	}
	linkPath = filepath.FromSlash(linkPath)
	linkPath = filepath.Clean(linkPath)
	if linkPath == "." {
		return ""
	}
	return linkPath
}

func compileMatchers(patterns []string) ([]glob.Glob, error) {
	matchers := make([]glob.Glob, 0, len(patterns))
	for _, pattern := range patterns {
		g, err := glob.Compile(pattern)
		if err != nil {
			return nil, err
		}
		matchers = append(matchers, g)
	}
	return matchers, nil
}

func matchesPathFilters(path string, include, exclude []glob.Glob) bool {
	slashPath := filepath.ToSlash(path)
	base := filepath.Base(path)

	if len(include) > 0 {
		matched := false
		for _, g := range include {
			if g.Match(slashPath) || g.Match(base) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	for _, g := range exclude {
		if g.Match(slashPath) || g.Match(base) {
			return false
		}
	}

	return true
}

func linkPosition(f *lint.File, n ast.Node) (int, int) {
	offset := firstTextOffset(n)
	if offset < 0 {
		return 1, 1
	}
	line := f.LineOfOffset(offset)
	lineStart := 0
	for i := 0; i < offset && i < len(f.Source); i++ {
		if f.Source[i] == '\n' {
			lineStart = i + 1
		}
	}
	return line, offset - lineStart + 1
}

func firstTextOffset(n ast.Node) int {
	offset := -1
	_ = ast.Walk(n, func(cur ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		text, ok := cur.(*ast.Text)
		if !ok {
			return ast.WalkContinue, nil
		}
		if offset == -1 || text.Segment.Start < offset {
			offset = text.Segment.Start
		}
		return ast.WalkContinue, nil
	})
	return offset
}

func configDiag(path string, r *Rule, err error) lint.Diagnostic {
	return lint.Diagnostic{
		File:     path,
		Line:     1,
		Column:   1,
		RuleID:   r.ID(),
		RuleName: r.Name(),
		Severity: lint.Warning,
		Message:  fmt.Sprintf("invalid rule settings: %v", err),
	}
}

func brokenFileDiag(path string, line, col int, r *Rule, target string) lint.Diagnostic {
	return lint.Diagnostic{
		File:     path,
		Line:     line,
		Column:   col,
		RuleID:   r.ID(),
		RuleName: r.Name(),
		Severity: lint.Warning,
		Message:  fmt.Sprintf("broken link target %q not found", target),
	}
}

func brokenHeadingDiag(path string, line, col int, r *Rule, target string) lint.Diagnostic {
	return lint.Diagnostic{
		File:     path,
		Line:     line,
		Column:   col,
		RuleID:   r.ID(),
		RuleName: r.Name(),
		Severity: lint.Warning,
		Message:  fmt.Sprintf("broken link target %q has no matching heading anchor", target),
	}
}

func toStringSlice(v any) ([]string, bool) {
	switch list := v.(type) {
	case []string:
		out := make([]string, len(list))
		copy(out, list)
		return out, true
	case []any:
		out := make([]string, 0, len(list))
		for _, item := range list {
			s, ok := item.(string)
			if !ok {
				return nil, false
			}
			out = append(out, s)
		}
		return out, true
	default:
		return nil, false
	}
}

var _ rule.Configurable = (*Rule)(nil)
