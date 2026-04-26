package crossfilereferenceintegrity

import (
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/gobwas/glob"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/mdtext"
	"github.com/jeduden/mdsmith/internal/placeholders"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/yuin/goldmark/ast"
)

func init() {
	rule.Register(&Rule{})
}

// Rule checks Markdown links for missing target files and missing heading
// anchors in linked Markdown files.
type Rule struct {
	Include      []string
	Exclude      []string
	Strict       bool
	Placeholders []string // placeholder tokens to treat as opaque
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

	// Precompute the resolved absolute root once for all link checks.
	resolvedRoot := resolveAbsRoot(f.RootDir)

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
			resolvedRoot,
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
	resolvedRoot string,
	anchorCache map[string]map[string]bool,
) []lint.Diagnostic {
	target, ok := parseTarget(string(linkNode.Destination))
	if !ok {
		return nil
	}

	if placeholders.ContainsBodyToken(target.Raw, r.Placeholders) {
		return nil
	}

	line, col := linkPosition(f, linkNode)

	if target.LocalAnchor {
		return checkLocalAnchor(f.Path, line, col, r, target, selfAnchors)
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

	targetFile, ok := resolveTargetFile(f, linkPath, resolvedRoot)
	if !ok {
		// If the link escapes the project root, silently skip it.
		if resolvedRoot != "" && linkEscapesRoot(f, linkPath, resolvedRoot) {
			return nil
		}
		return []lint.Diagnostic{brokenFileDiag(f.Path, line, col, r, target.Raw)}
	}

	if target.Anchor == "" || !isMarkdownPath(linkPath) {
		return nil
	}

	targetAnchors, err := anchorsForFile(targetFile, anchorCache)
	if err != nil {
		return []lint.Diagnostic{unreadableTargetDiag(f.Path, line, col, r, target.Raw, err)}
	}
	if targetAnchors[normalizeAnchor(target.Anchor)] {
		return nil
	}
	return []lint.Diagnostic{brokenHeadingDiag(f.Path, line, col, r, target.Raw)}
}

func checkLocalAnchor(
	path string, line, col int, r *Rule, target linkTarget, selfAnchors map[string]bool,
) []lint.Diagnostic {
	if selfAnchors[normalizeAnchor(target.Anchor)] {
		return nil
	}
	return []lint.Diagnostic{brokenHeadingDiag(path, line, col, r, target.Raw)}
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
		case "placeholders":
			toks, ok := toStringSlice(v)
			if !ok {
				return fmt.Errorf(
					"cross-file-reference-integrity: placeholders must be a list of strings, got %T",
					v,
				)
			}
			if err := placeholders.Validate(toks); err != nil {
				return fmt.Errorf("cross-file-reference-integrity: %w", err)
			}
			r.Placeholders = toks
		default:
			return fmt.Errorf(
				"cross-file-reference-integrity: unknown setting %q",
				k,
			)
		}
	}
	return r.validateGlobSettings()
}

func (r *Rule) validateGlobSettings() error {
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
		"include":      []string{},
		"exclude":      []string{},
		"strict":       false,
		"placeholders": []string{},
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

func resolveTargetFile(f *lint.File, linkPath, resolvedRoot string) (targetFile, bool) {
	maxBytes := f.MaxInputBytes
	if path, ok := resolveTargetOSPath(f.Path, linkPath); ok {
		if _, err := os.Stat(path); err == nil {
			// Reject links that resolve outside the project root,
			// evaluating symlinks to prevent bypass via symlinked dirs.
			if resolvedRoot != "" && !isWithinRoot(resolvedRoot, path) {
				return targetFile{}, false
			}
			return targetFile{
				cacheKey: "os:" + path,
				read: func() ([]byte, error) {
					return lint.ReadFileLimited(path, maxBytes)
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
			return lint.ReadFSFileLimited(f.FS, fsPath, maxBytes)
		},
	}, true
}

// resolveAbsRoot computes the absolute, symlink-resolved root directory
// path once per rule check. Returns "" if rootDir is empty.
func resolveAbsRoot(rootDir string) string {
	if rootDir == "" {
		return ""
	}
	realRoot, err := filepath.EvalSymlinks(rootDir)
	if err != nil {
		realRoot = rootDir
	}
	abs, err := filepath.Abs(realRoot)
	if err != nil {
		return filepath.Clean(realRoot)
	}
	return abs
}

// isWithinRoot checks whether target is inside the pre-resolved absolute
// root, resolving symlinks on the target to prevent symlink-based traversal.
func isWithinRoot(resolvedRoot, target string) bool {
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return false
	}
	realTarget, err := filepath.EvalSymlinks(absTarget)
	if err != nil {
		// Symlink resolution failed (e.g. dangling link); fall back to
		// the cleaned absolute path so the root comparison still works.
		realTarget = filepath.Clean(absTarget)
	}
	rel, err := filepath.Rel(resolvedRoot, realTarget)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// linkEscapesRoot checks whether resolving linkPath from f.Path would land
// outside f.RootDir. Used to silently skip links that traverse above the
// project root.
func linkEscapesRoot(f *lint.File, linkPath, resolvedRoot string) bool {
	resolved, ok := resolveTargetOSPath(f.Path, linkPath)
	if !ok {
		return false
	}
	return !isWithinRoot(resolvedRoot, resolved)
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

		slug := mdtext.Slugify(extractText(h, f.Source))
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

func normalizeAnchor(anchor string) string {
	decoded, err := url.PathUnescape(anchor)
	if err == nil {
		anchor = decoded
	}
	return mdtext.Slugify(anchor)
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

// unreadableTargetDiag reports a link whose target exists on the
// filesystem but cannot be read (e.g. exceeds the configured
// max-input-size). The underlying error is surfaced so users can
// distinguish these from genuinely missing targets.
func unreadableTargetDiag(path string, line, col int, r *Rule, target string, err error) lint.Diagnostic {
	return lint.Diagnostic{
		File:     path,
		Line:     line,
		Column:   col,
		RuleID:   r.ID(),
		RuleName: r.Name(),
		Severity: lint.Warning,
		Message:  fmt.Sprintf("cannot read link target %q: %v", target, err),
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
