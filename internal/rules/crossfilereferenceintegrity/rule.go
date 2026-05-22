package crossfilereferenceintegrity

import (
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/jeduden/mdsmith/internal/globpath"
	"github.com/jeduden/mdsmith/internal/linkgraph"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/placeholders"
	"github.com/jeduden/mdsmith/internal/rule"
)

func init() {
	rule.Register(&Rule{})
}

// LinksConfig holds the per-file link-validation knobs exposed via the
// links: sub-block. Mirrors the shared links: config block described in
// docs/research/links/README.md.
type LinksConfig struct {
	SiteRoot               string // resolved against site root for absolute paths
	ValidateImages         bool   // check *ast.Image targets (default on)
	ValidateReferenceStyle bool   // check reference-style link targets (default on)
}

// Rule checks Markdown links for missing target files and missing heading
// anchors in linked Markdown files.
type Rule struct {
	Include       []string
	Exclude       []string
	Strict        bool
	Placeholders  []string // placeholder tokens to treat as opaque
	Wikilinks     bool     // when true, validate Obsidian-style [[...]] targets
	WikilinkStyle string   // resolution style; only "obsidian" ships today
	Links         LinksConfig
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

	if err := r.validateGlobSettings(); err != nil {
		return []lint.Diagnostic{configDiag(f.Path, r, err)}
	}

	selfAnchors := linkgraph.CollectAnchors(f)
	anchorCache := map[string]map[string]bool{"self": selfAnchors}

	// Precompute the resolved absolute root once for all link checks.
	resolvedRoot := resolveAbsRoot(f.RootDir)
	resolvedSiteRoot := resolveAbsRoot(r.Links.SiteRoot)

	var diags []lint.Diagnostic
	for _, link := range linkgraph.ExtractLinks(f) {
		diags = append(diags, r.checkLink(
			f, link, false,
			r.Include, r.Exclude,
			selfAnchors, resolvedRoot, resolvedSiteRoot,
			anchorCache,
		)...)
	}
	if r.Links.ValidateImages {
		for _, link := range linkgraph.ExtractImages(f) {
			diags = append(diags, r.checkLink(
				f, link, true,
				r.Include, r.Exclude,
				selfAnchors, resolvedRoot, resolvedSiteRoot,
				anchorCache,
			)...)
		}
	}
	if r.Links.ValidateReferenceStyle {
		for _, link := range linkgraph.ExtractRefLinkTargets(f) {
			diags = append(diags, r.checkLink(
				f, link, false,
				r.Include, r.Exclude,
				selfAnchors, resolvedRoot, resolvedSiteRoot,
				anchorCache,
			)...)
		}
	}
	if r.Wikilinks {
		diags = append(diags, r.checkWikilinks(f, anchorCache)...)
	}

	return diags
}

// checkWikilinks resolves every Obsidian-style wikilink against the
// project root and emits one diagnostic per unresolved target or
// missing heading anchor. Wikilink targets pass through the same
// placeholder filter the standard link check uses.
//
// Resolution is cached per (style, target) within one Check: two
// wikilinks pointing at the same page share a single fs walk, so
// runtime stays linear in distinct targets rather than total
// wikilinks.
func (r *Rule) checkWikilinks(
	f *lint.File,
	anchorCache map[string]map[string]bool,
) []lint.Diagnostic {
	// f.FS is guaranteed non-nil by the caller (r.Check returns early
	// otherwise), and wikilinkRoot's last fallback returns f.FS, so
	// root is always populated here.
	root := wikilinkRoot(f)
	resolver := newWikilinkResolver(
		root, workspaceRelativeSource(f), r.effectiveWikilinkStyle(),
		wikilinkIndexForRoot(f, root),
	)

	var diags []lint.Diagnostic
	for _, wl := range linkgraph.ExtractWikiLinks(f) {
		if r.wikilinkSuppressed(wl) {
			continue
		}
		resolved, ok := resolver.resolve(wl.Target)
		if !ok {
			diags = append(diags, wikilinkBrokenTargetDiag(f.Path, wl, r))
			continue
		}
		diags = append(diags, r.checkWikilinkAnchor(f, wl, resolved, root, anchorCache)...)
	}
	return diags
}

func (r *Rule) checkWikilinkAnchor(
	f *lint.File,
	wl linkgraph.WikiLink,
	resolved string,
	root fs.FS,
	anchorCache map[string]map[string]bool,
) []lint.Diagnostic {
	if wl.Anchor == "" || !isMarkdownPath(resolved) {
		return nil
	}
	anchors, err := wikilinkAnchorsForTarget(f, root, resolved, anchorCache)
	if err != nil {
		return []lint.Diagnostic{wikilinkUnreadableTargetDiag(f.Path, wl, resolved, err, r)}
	}
	if anchors[linkgraph.NormalizeAnchor(wl.Anchor)] {
		return nil
	}
	return []lint.Diagnostic{wikilinkBrokenAnchorDiag(f.Path, wl, resolved, r)}
}

func (r *Rule) wikilinkSuppressed(wl linkgraph.WikiLink) bool {
	if len(r.Placeholders) == 0 {
		return false
	}
	// MDS027's placeholder filter only applies to link destinations,
	// not link text. For a wikilink the destination is target plus
	// optional "#anchor"; the alias is display text and must not be
	// scanned for placeholder tokens.
	dest := wl.Target
	if wl.Anchor != "" {
		dest += "#" + wl.Anchor
	}
	return placeholders.ContainsBodyToken(dest, r.Placeholders)
}

func (r *Rule) effectiveWikilinkStyle() string {
	if r.WikilinkStyle == "" {
		return "obsidian"
	}
	return r.WikilinkStyle
}

// wikilinkIndexForRoot returns a *linkgraph.WikilinkIndex cached on
// f.RunCache, keyed by the workspace's absolute root directory. The
// index is built lazily by the first Check that needs it and shared
// by every later host file in the same run. Returns nil when no
// RunCache is installed (struct-literal Files in unit tests) so the
// resolver falls back to the per-Check fs walk.
func wikilinkIndexForRoot(f *lint.File, root fs.FS) *linkgraph.WikilinkIndex {
	if f.RunCache == nil || root == nil {
		return nil
	}
	key := wikilinkCacheKey(f)
	if key == "" {
		return nil
	}
	v := f.RunCache.Wikilinks(key, func() any {
		return linkgraph.NewWikilinkIndex(root)
	})
	idx, _ := v.(*linkgraph.WikilinkIndex)
	return idx
}

// wikilinkCacheKey returns the per-root cache key for the wikilink
// index. RootDir's absolute form is preferred so two host files at
// different relative paths under the same root share one index.
//
// filepath.Abs only errors when os.Getwd fails — an OS-level
// catastrophe the rest of the linter does not survive either —
// so the rare error is swallowed in favour of the raw RootDir.
func wikilinkCacheKey(f *lint.File) string {
	if f.RootDir == "" {
		return ""
	}
	abs, _ := filepath.Abs(f.RootDir) //nolint:errcheck
	return abs
}

func wikilinkRoot(f *lint.File) fs.FS {
	if f.RootFS != nil {
		return f.RootFS
	}
	if f.RootDir != "" {
		return os.DirFS(f.RootDir)
	}
	return f.FS
}

// wikilinkResolver caches workspace-walk results so a doc with many
// references to the same target does a single fs walk per target.
// When a per-run WikilinkIndex is available (built once and shared
// via f.RunCache), the resolver serves every lookup from that
// index — turning N files × M targets × workspace-walk into one
// walk per workspace.
type wikilinkResolver struct {
	root   fs.FS
	from   string
	style  string
	index  *linkgraph.WikilinkIndex
	memory map[string]wikilinkResolveResult
}

type wikilinkResolveResult struct {
	path string
	ok   bool
}

func newWikilinkResolver(root fs.FS, from, style string, index *linkgraph.WikilinkIndex) *wikilinkResolver {
	return &wikilinkResolver{
		root:   root,
		from:   from,
		style:  style,
		index:  index,
		memory: map[string]wikilinkResolveResult{},
	}
}

func (rv *wikilinkResolver) resolve(target string) (string, bool) {
	if cached, ok := rv.memory[target]; ok {
		return cached.path, cached.ok
	}
	var out wikilinkResolveResult
	switch rv.style {
	case "obsidian":
		if rv.index != nil {
			out.path, out.ok = rv.index.Resolve(target)
		} else {
			out.path, out.ok = linkgraph.ResolveWikiLink(rv.root, rv.from, target)
		}
	default:
		// Settings parsing already rejects unsupported values; this
		// branch is a defensive no-op so a manually-constructed Rule
		// with a non-empty unknown style cannot silently fall back.
	}
	rv.memory[target] = out
	return out.path, out.ok
}

// wikilinkAnchorsForTarget memoizes anchor lookup per workspace-relative
// target path so two wikilinks pointing at the same file share one parse.
func wikilinkAnchorsForTarget(
	f *lint.File,
	root fs.FS,
	resolved string,
	cache map[string]map[string]bool,
) (map[string]bool, error) {
	key := "wikilink:" + resolved
	if anchors, ok := cache[key]; ok {
		return anchors, nil
	}
	data, err := lint.ReadFSFileLimited(root, resolved, f.MaxInputBytes)
	if err != nil {
		return nil, err
	}
	target, _ := lint.NewFileFromSource(resolved, data, true) //nolint:errcheck
	anchors := linkgraph.CollectAnchors(target)
	cache[key] = anchors
	return anchors, nil
}

// workspaceRelativeSource returns f.Path expressed relative to its
// project root, using forward slashes. The LSP and the CLI hand the
// rule different shapes of f.Path — the CLI walks files by their
// absolute path, the LSP threads the workspace-relative form — so
// the helper has to anchor a relative f.Path to RootDir before
// re-relativising, or filepath.Abs would land it under the process
// cwd instead.
//
// When the path cannot be made relative (a struct-literal test
// File without RootDir, or a cross-volume pair on Windows) it
// returns the original path with separators normalised.
func workspaceRelativeSource(f *lint.File) string {
	if f.RootDir == "" {
		return filepath.ToSlash(f.Path)
	}
	sourcePath := f.Path
	if !filepath.IsAbs(sourcePath) {
		sourcePath = filepath.Join(f.RootDir, sourcePath)
	}
	abs, _ := filepath.Abs(sourcePath)    //nolint:errcheck
	absRoot, _ := filepath.Abs(f.RootDir) //nolint:errcheck
	rel, err := filepath.Rel(absRoot, abs)
	if err != nil {
		return filepath.ToSlash(f.Path)
	}
	return filepath.ToSlash(rel)
}

// wikilinkRaw renders wl back to its source form for diagnostic
// messages — broken-anchor diagnostics quote the full bracket span
// (target + #anchor + |alias) so the user can grep the source for
// the exact token. Placeholder suppression does NOT call this; see
// wikilinkSuppressed for the destination-only check.
func wikilinkRaw(wl linkgraph.WikiLink) string {
	var sb strings.Builder
	if wl.Embed {
		sb.WriteByte('!')
	}
	sb.WriteString("[[")
	sb.WriteString(wl.Target)
	if wl.Anchor != "" {
		sb.WriteByte('#')
		sb.WriteString(wl.Anchor)
	}
	if wl.Alias != "" {
		sb.WriteByte('|')
		sb.WriteString(wl.Alias)
	}
	sb.WriteString("]]")
	return sb.String()
}

func wikilinkBrokenTargetDiag(
	path string, wl linkgraph.WikiLink, r *Rule,
) lint.Diagnostic {
	return lint.Diagnostic{
		File:     path,
		Line:     wl.Line,
		Column:   wl.Column,
		RuleID:   r.ID(),
		RuleName: r.Name(),
		Severity: lint.Warning,
		Message:  fmt.Sprintf("wikilink target %q not found in workspace", wl.Target),
	}
}

func wikilinkBrokenAnchorDiag(
	path string, wl linkgraph.WikiLink, resolved string, r *Rule,
) lint.Diagnostic {
	return lint.Diagnostic{
		File:     path,
		Line:     wl.Line,
		Column:   wl.Column,
		RuleID:   r.ID(),
		RuleName: r.Name(),
		Severity: lint.Warning,
		Message: fmt.Sprintf(
			"wikilink %q: anchor %q not found in %s",
			wikilinkRaw(wl), wl.Anchor, resolved,
		),
	}
}

func wikilinkUnreadableTargetDiag(
	path string, wl linkgraph.WikiLink, resolved string, err error, r *Rule,
) lint.Diagnostic {
	return lint.Diagnostic{
		File:     path,
		Line:     wl.Line,
		Column:   wl.Column,
		RuleID:   r.ID(),
		RuleName: r.Name(),
		Severity: lint.Warning,
		Message: fmt.Sprintf(
			"cannot read wikilink target %q (%s): %v",
			wl.Target, resolved, err,
		),
	}
}

func (r *Rule) checkLink(
	f *lint.File,
	link linkgraph.Link,
	isImage bool,
	includePatterns []string,
	excludePatterns []string,
	selfAnchors map[string]bool,
	resolvedRoot string,
	resolvedSiteRoot string,
	anchorCache map[string]map[string]bool,
) []lint.Diagnostic {
	target := link.Target

	if placeholders.ContainsBodyToken(target.Raw, r.Placeholders) {
		return nil
	}

	line, col := link.Line, link.Column

	if target.LocalAnchor {
		return checkLocalAnchor(f.Path, line, col, r, target, selfAnchors)
	}

	linkPath := normalizeLinkPath(target.Path)
	if linkPath == "" {
		return nil
	}

	if filepath.IsAbs(linkPath) {
		if resolvedSiteRoot == "" {
			return nil
		}
		return r.checkSiteAbsoluteLink(f, link, linkPath, resolvedSiteRoot)
	}

	if !isImage && !r.Strict && !isMarkdownPath(linkPath) {
		return nil
	}

	if !matchesPathFilters(linkPath, includePatterns, excludePatterns) {
		return nil
	}

	return r.checkRelativeTarget(f, line, col, target, linkPath, resolvedRoot, anchorCache)
}

// checkRelativeTarget verifies a relative link path exists and, for
// Markdown targets with an anchor, that the anchor resolves to a heading.
func (r *Rule) checkRelativeTarget(
	f *lint.File,
	line, col int,
	target linkgraph.Target,
	linkPath string,
	resolvedRoot string,
	anchorCache map[string]map[string]bool,
) []lint.Diagnostic {
	targetFile, ok := resolveTargetFile(f, linkPath, resolvedRoot)
	if !ok {
		// Silently skip links that escape the project root.
		if resolvedRoot != "" && linkEscapesRoot(f, linkPath, resolvedRoot) {
			return nil
		}
		return []lint.Diagnostic{brokenFileDiag(f.Path, line, col, r, target.Raw)}
	}

	if target.Anchor == "" || !isMarkdownPath(linkPath) {
		return nil
	}

	targetAnchors, err := anchorsForFile(f, targetFile, anchorCache)
	if err != nil {
		return []lint.Diagnostic{unreadableTargetDiag(f.Path, line, col, r, target.Raw, err)}
	}
	if targetAnchors[linkgraph.NormalizeAnchor(target.Anchor)] {
		return nil
	}
	return []lint.Diagnostic{brokenHeadingDiag(f.Path, line, col, r, target.Raw)}
}

func checkLocalAnchor(
	path string, line, col int, r *Rule, target linkgraph.Target, selfAnchors map[string]bool,
) []lint.Diagnostic {
	if selfAnchors[linkgraph.NormalizeAnchor(target.Anchor)] {
		return nil
	}
	return []lint.Diagnostic{brokenHeadingDiag(path, line, col, r, target.Raw)}
}

// checkSiteAbsoluteLink resolves an absolute-path link (e.g.
// /docs/rules/MDS027/) against the configured site root and checks
// whether the resulting on-disk path exists. Anchor checking is
// skipped for site-absolute paths: the target is a rendered page
// directory, not a Markdown source file.
func (r *Rule) checkSiteAbsoluteLink(
	f *lint.File,
	link linkgraph.Link,
	absPath string,
	resolvedSiteRoot string,
) []lint.Diagnostic {
	// Strip the leading path separator and re-express as a
	// platform-native relative path before joining with siteRoot.
	rel := strings.TrimPrefix(filepath.ToSlash(absPath), "/")
	rel = filepath.FromSlash(rel)
	if rel == "" {
		return nil
	}
	target := filepath.Join(resolvedSiteRoot, rel)
	if cachedStatExists(target) {
		return nil
	}
	return []lint.Diagnostic{brokenFileDiag(f.Path, link.Line, link.Column, r, link.Target.Raw)}
}

// ApplySettings implements rule.Configurable.
func (r *Rule) ApplySettings(settings map[string]any) error {
	for k, v := range settings {
		if err := r.applyOneSetting(k, v); err != nil {
			return err
		}
	}
	return r.validateGlobSettings()
}

func (r *Rule) applyOneSetting(key string, v any) error {
	switch key {
	case "include":
		return r.applyListSetting(&r.Include, "include", v)
	case "exclude":
		return r.applyListSetting(&r.Exclude, "exclude", v)
	case "strict":
		b, ok := v.(bool)
		if !ok {
			return fmt.Errorf(
				"cross-file-reference-integrity: strict must be a bool, got %T",
				v,
			)
		}
		r.Strict = b
		return nil
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
		return nil
	case "links":
		linksMap, ok := v.(map[string]any)
		if !ok {
			return fmt.Errorf(
				"cross-file-reference-integrity: links must be a map, got %T",
				v,
			)
		}
		return r.applyLinksSettings(linksMap)
	case "wikilinks", "wikilink-style":
		return r.applyWikilinkSetting(key, v)
	}
	return fmt.Errorf("cross-file-reference-integrity: unknown setting %q", key)
}

func (r *Rule) applyListSetting(target *[]string, name string, v any) error {
	list, ok := toStringSlice(v)
	if !ok {
		return fmt.Errorf(
			"cross-file-reference-integrity: %s must be a list of strings, got %T",
			name, v,
		)
	}
	*target = list
	return nil
}

func (r *Rule) applyWikilinkSetting(key string, v any) error {
	switch key {
	case "wikilinks":
		b, ok := v.(bool)
		if !ok {
			return fmt.Errorf(
				"cross-file-reference-integrity: wikilinks must be a bool, got %T",
				v,
			)
		}
		r.Wikilinks = b
	case "wikilink-style":
		s, ok := v.(string)
		if !ok {
			return fmt.Errorf(
				"cross-file-reference-integrity: wikilink-style must be a string, got %T",
				v,
			)
		}
		if s != "" && s != "obsidian" {
			return fmt.Errorf(
				"cross-file-reference-integrity: wikilink-style %q not supported; only \"obsidian\" ships today",
				s,
			)
		}
		r.WikilinkStyle = s
	}
	return nil
}

func (r *Rule) applyLinksSettings(m map[string]any) error {
	for k, v := range m {
		switch k {
		case "site-root":
			s, ok := v.(string)
			if !ok {
				return fmt.Errorf(
					"cross-file-reference-integrity: links.site-root must be a string, got %T",
					v,
				)
			}
			r.Links.SiteRoot = s
		case "validate-images":
			b, ok := v.(bool)
			if !ok {
				return fmt.Errorf(
					"cross-file-reference-integrity: links.validate-images must be a bool, got %T",
					v,
				)
			}
			r.Links.ValidateImages = b
		case "validate-reference-style":
			b, ok := v.(bool)
			if !ok {
				return fmt.Errorf(
					"cross-file-reference-integrity: links.validate-reference-style must be a bool, got %T",
					v,
				)
			}
			r.Links.ValidateReferenceStyle = b
		default:
			return fmt.Errorf(
				"cross-file-reference-integrity: unknown links setting %q",
				k,
			)
		}
	}
	return nil
}

func (r *Rule) validateGlobSettings() error {
	if err := validatePatterns(r.Include); err != nil {
		return fmt.Errorf(
			"cross-file-reference-integrity: include has invalid glob pattern: %w",
			err,
		)
	}
	if err := validatePatterns(r.Exclude); err != nil {
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
		"include":        []string{},
		"exclude":        []string{},
		"strict":         false,
		"placeholders":   []string{},
		"wikilinks":      false,
		"wikilink-style": "obsidian",
		"links": map[string]any{
			"site-root":                "",
			"validate-images":          true,
			"validate-reference-style": true,
		},
	}
}

// SettingMergeMode implements rule.ListMerger.
func (r *Rule) SettingMergeMode(key string) rule.MergeMode {
	if key == "placeholders" {
		return rule.MergeAppend
	}
	return rule.MergeReplace
}

type targetFile struct {
	// cacheKey is the per-Check cache key (the `cache` map in
	// anchorsForFile). Prefixed with "os:" or "fs:" so OS and FS
	// resolutions of the same path do not collide within one
	// Check call.
	cacheKey string
	// runCacheKey is the engine-wide RunCache.Anchors key — an
	// absolute on-disk path. Left empty for FS-only resolutions
	// (in-memory FSes do not have a stable on-disk anchor that
	// the LSP's Invalidate(absPath) would call with). An empty
	// runCacheKey signals "skip the RunCache slot, use the
	// per-Check cache only".
	runCacheKey string
	read        func() ([]byte, error)
}

func anchorsForFile(host *lint.File, target targetFile, cache map[string]map[string]bool) (map[string]bool, error) {
	if anchors, ok := cache[target.cacheKey]; ok {
		return anchors, nil
	}

	// Engine-shared cache: the target file's anchor set is a function
	// of the file's bytes alone, so memoizing on the engine.Runner's
	// RunCache collapses the per-host-file walk to one walk per
	// (Run, target). On link-heavy real corpora (e.g. plan files all
	// linking to PLAN.md) this collapses N parses to one.
	//
	// Read errors stay outside the cache: the cache stores `nil` for
	// "build returned no anchors" and the read is retried on the next
	// host file. That matches the previous per-Check cache's
	// semantics, where an unreadable target produced a diagnostic on
	// the host but did not poison the cache for siblings.
	var anchors map[string]bool
	var err error
	if host != nil && host.RunCache != nil && target.runCacheKey != "" {
		// The build closure escapes to heap (the RunCache stores it
		// behind a sync.Mutex slot), but it captures only `target`
		// (a small value) — buildAnchorsForTarget receives it by
		// value, so no err pointer leaks into the closure box.
		//
		// The key is the on-disk absolute path so RunCache.Invalidate
		// (called by the LSP on document edits) hits the same slot
		// and the next cross-file check re-reads from disk.
		builder := anchorBuilder{target: target}
		anchors, err = host.RunCache.Anchors(target.runCacheKey, builder.build)
	} else {
		// In-memory FS or no RunCache (struct-literal File in tests):
		// fall back to the per-Check cache only.
		anchors, err = buildAnchorsForTarget(target)
	}
	if err != nil {
		return nil, err
	}
	cache[target.cacheKey] = anchors
	return anchors, nil
}

// anchorBuilder pairs a targetFile with a method-value `build` so
// RunCache.Anchors receives a function that does not capture an
// `err` variable in a closure box. Method values on a small value
// receiver are stack-allocatable when the receiver lifetime is
// known; the call site (anchorsForFile) keeps the receiver on the
// stack for the duration of the RunCache call.
type anchorBuilder struct {
	target targetFile
}

func (b anchorBuilder) build() (map[string]bool, error) {
	return buildAnchorsForTarget(b.target)
}

// buildAnchorsForTarget reads the target file's bytes, parses it,
// and collects the heading anchor set. Extracted so anchorsForFile
// can call it without going through the build closure that would
// otherwise capture `err` and escape to the heap.
func buildAnchorsForTarget(target targetFile) (map[string]bool, error) {
	data, err := target.read()
	if err != nil {
		return nil, err
	}
	// lint.NewFile never errors; goldmark always produces an AST.
	file, _ := lint.NewFileFromSource(target.cacheKey, data, true) //nolint:errcheck
	return linkgraph.CollectAnchors(file), nil
}

func resolveTargetFile(f *lint.File, linkPath, resolvedRoot string) (targetFile, bool) {
	maxBytes := f.MaxInputBytes
	if path, ok := resolveTargetOSPath(f.Path, linkPath); ok {
		if cachedStatExists(path) {
			// Reject links that resolve outside the project root,
			// evaluating symlinks to prevent bypass via symlinked dirs.
			if resolvedRoot != "" && !isWithinRoot(resolvedRoot, path) {
				return targetFile{}, false
			}
			return targetFile{
				cacheKey:    "os:" + path,
				runCacheKey: path,
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
	realRoot, ok := cachedEvalSymlinks(rootDir)
	if !ok {
		realRoot = rootDir
	}
	// filepath.Abs only errors when os.Getwd() fails, an OS-level catastrophe.
	// Returning "" lets the caller treat the root as unset (safe fallback).
	abs, _ := filepath.Abs(realRoot) //nolint:errcheck
	return abs
}

// isWithinRoot checks whether target is inside the pre-resolved absolute
// root, resolving symlinks on the target to prevent symlink-based traversal.
func isWithinRoot(resolvedRoot, target string) bool {
	// filepath.Abs only errors when os.Getwd() fails (OS-level catastrophe);
	// "" as absTarget degrades gracefully through the rest of the function.
	absTarget, _ := filepath.Abs(target) //nolint:errcheck
	realTarget, ok := cachedEvalSymlinks(absTarget)
	if !ok {
		// Symlink resolution failed (e.g. dangling link); fall back to
		// the cleaned absolute path so the root comparison still works.
		realTarget = filepath.Clean(absTarget)
	}
	// filepath.Rel only errors on mismatched volumes (Windows); both paths
	// are absolute on Linux so this never errors here.
	rel, _ := filepath.Rel(resolvedRoot, realTarget) //nolint:errcheck
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

func isMarkdownPath(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".md" || ext == ".markdown"
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

// validatePatterns checks that all patterns are valid doublestar patterns.
func validatePatterns(patterns []string) error {
	for _, p := range patterns {
		if _, err := doublestar.Match(p, ""); err != nil {
			return fmt.Errorf("invalid pattern %q: %w", p, err)
		}
	}
	return nil
}

func matchesPathFilters(path string, include, exclude []string) bool {
	if len(include) > 0 {
		matched := false
		for _, pattern := range include {
			if globpath.Match(pattern, path) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	for _, pattern := range exclude {
		if globpath.Match(pattern, path) {
			return false
		}
	}

	return true
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

var (
	_ rule.Configurable = (*Rule)(nil)
	_ rule.ListMerger   = (*Rule)(nil)
)
