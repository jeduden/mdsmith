package crossfilereferenceintegrity

import (
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/jeduden/mdsmith/internal/bytelimit"
	"github.com/jeduden/mdsmith/internal/globpath"
	"github.com/jeduden/mdsmith/internal/linkgraph"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/placeholders"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/jeduden/mdsmith/internal/setutil"
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

// checkCtx holds per-Check lazy state for the link walk. The
// anchor-bearing fields are nil until the first link that
// actually needs them: selfAnchors stays nil unless a link is a
// local anchor or a cross-file Markdown link with an anchor;
// anchorCache stays nil unless a cross-file anchor is resolved.
// resolvedRoot and resolvedSiteRoot are eager because every
// relative-link check needs them and the cache is package-scope
// (cachedAbsRoot) so the cost is paid once across all Files in
// one run. Plan 195 task 5.
type checkCtx struct {
	f                *lint.File
	rule             *Rule
	resolvedRoot     string
	resolvedSiteRoot string

	selfAnchors      map[string]struct{}
	selfAnchorsBuilt bool
	anchorCache      map[string]map[string]struct{}
}

// ensureSelfAnchors lazily builds the heading-anchor set for f.
// The fixture's link `[other](other.md)` has no anchor and no
// link is local-anchor, so this path stays cold; the per-Check
// allocation collapsed from "always paid" to "only when needed".
func (c *checkCtx) ensureSelfAnchors() map[string]struct{} {
	if !c.selfAnchorsBuilt {
		c.selfAnchors = linkgraph.CollectAnchors(c.f)
		c.selfAnchorsBuilt = true
	}
	return c.selfAnchors
}

// ensureAnchorCache lazily initialises the per-Check map of
// already-resolved target-file anchor sets. The cache is only
// queried for cross-file Markdown links with a fragment; for the
// gate fixture the link has no fragment so the map stays nil.
func (c *checkCtx) ensureAnchorCache() map[string]map[string]struct{} {
	if c.anchorCache == nil {
		c.anchorCache = make(map[string]map[string]struct{}, 2)
		// Seed with the self-anchor entry the original eager
		// initialisation used — preserves the cache shape for any
		// future caller that asks via the "self" key.
		c.anchorCache["self"] = c.ensureSelfAnchors()
	}
	return c.anchorCache
}

// Check implements rule.Rule.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	// Stdin/source-only checks have no stable filesystem context.
	if f.FS == nil {
		return nil
	}

	if err := r.validateGlobSettings(); err != nil {
		return []lint.Diagnostic{configDiag(f.Path, r, err)}
	}

	ctx := checkCtx{
		f:                f,
		rule:             r,
		resolvedRoot:     resolveAbsRoot(f.RootDir),
		resolvedSiteRoot: resolveAbsRoot(r.Links.SiteRoot),
	}

	var diags []lint.Diagnostic
	for _, link := range linkgraph.ExtractLinks(f) {
		diags = append(diags, r.checkLink(&ctx, link, false)...)
	}
	if r.Links.ValidateImages {
		for _, link := range linkgraph.ExtractImages(f) {
			diags = append(diags, r.checkLink(&ctx, link, true)...)
		}
	}
	if r.Links.ValidateReferenceStyle {
		for _, link := range linkgraph.ExtractRefLinkTargets(f) {
			diags = append(diags, r.checkLink(&ctx, link, false)...)
		}
	}
	if r.Wikilinks {
		diags = append(diags, r.checkWikilinks(f, ctx.ensureAnchorCache())...)
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
	anchorCache map[string]map[string]struct{},
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
	anchorCache map[string]map[string]struct{},
) []lint.Diagnostic {
	if wl.Anchor == "" || !isMarkdownPath(resolved) {
		return nil
	}
	anchors, err := wikilinkAnchorsForTarget(f, root, resolved, anchorCache)
	if err != nil {
		return []lint.Diagnostic{wikilinkUnreadableTargetDiag(f.Path, wl, resolved, err, r)}
	}
	if setutil.Contains(anchors, linkgraph.NormalizeAnchor(wl.Anchor)) {
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
//
// Delegates to linkgraph.WikilinkIndexFor so MDS027 and `mdsmith
// list backlinks` share one build/cache implementation.
func wikilinkIndexForRoot(f *lint.File, root fs.FS) *linkgraph.WikilinkIndex {
	if f.RunCache == nil || root == nil {
		return nil
	}
	key := wikilinkCacheKey(f)
	if key == "" {
		return nil
	}
	return linkgraph.WikilinkIndexFor(f.RunCache, key, root)
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
	cache map[string]map[string]struct{},
) (map[string]struct{}, error) {
	key := "wikilink:" + resolved
	if anchors, ok := cache[key]; ok {
		return anchors, nil
	}
	data, err := bytelimit.ReadFSFileLimited(root, resolved, f.MaxInputBytes)
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
	ctx *checkCtx,
	link linkgraph.Link,
	isImage bool,
) []lint.Diagnostic {
	target := link.Target

	if placeholders.ContainsBodyToken(target.Raw, r.Placeholders) {
		return nil
	}

	line, col := link.Line, link.Column

	if target.LocalAnchor {
		return checkLocalAnchor(ctx, line, col, target)
	}

	linkPath := normalizeLinkPath(target.Path)
	if linkPath == "" {
		return nil
	}

	if filepath.IsAbs(linkPath) {
		if ctx.resolvedSiteRoot == "" {
			return nil
		}
		return r.checkSiteAbsoluteLink(ctx.f, link, linkPath, ctx.resolvedSiteRoot)
	}

	if !isImage && !r.Strict && !isMarkdownPath(linkPath) {
		return nil
	}

	if !matchesPathFilters(linkPath, r.Include, r.Exclude) {
		return nil
	}

	return r.checkRelativeTarget(ctx, line, col, target, linkPath)
}

// checkRelativeTarget verifies a relative link path exists and, for
// Markdown targets with an anchor, that the anchor resolves to a heading.
func (r *Rule) checkRelativeTarget(
	ctx *checkCtx,
	line, col int,
	target linkgraph.Target,
	linkPath string,
) []lint.Diagnostic {
	// Split the "does the target exist" check from the "build a
	// readable targetFile" step so we only pay for the read
	// closure (which would otherwise escape to the heap) when an
	// anchor actually has to be resolved. Plan 195 task 5.
	if target.Anchor == "" || !isMarkdownPath(linkPath) {
		if !targetExists(ctx.f, linkPath, ctx.resolvedRoot) {
			if ctx.resolvedRoot != "" && linkEscapesRoot(ctx.f, linkPath, ctx.resolvedRoot) {
				return nil
			}
			return []lint.Diagnostic{brokenFileDiag(ctx.f.Path, line, col, r, target.Raw)}
		}
		return nil
	}

	targetFile, ok := resolveTargetFile(ctx.f, linkPath, ctx.resolvedRoot)
	if !ok {
		if ctx.resolvedRoot != "" && linkEscapesRoot(ctx.f, linkPath, ctx.resolvedRoot) {
			return nil
		}
		return []lint.Diagnostic{brokenFileDiag(ctx.f.Path, line, col, r, target.Raw)}
	}

	targetAnchors, err := anchorsForFile(ctx.f, targetFile, ctx.ensureAnchorCache())
	if err != nil {
		return []lint.Diagnostic{unreadableTargetDiag(ctx.f.Path, line, col, r, target.Raw, err)}
	}
	if setutil.Contains(targetAnchors, linkgraph.NormalizeAnchor(target.Anchor)) {
		return nil
	}
	return []lint.Diagnostic{brokenHeadingDiag(ctx.f.Path, line, col, r, target.Raw)}
}

// targetExists reports whether linkPath resolves to a file the
// rule treats as existing — on-disk via cachedStatExists, or
// inside f.FS via fs.Stat. The OS branch matches
// resolveTargetFile's exact precedence so callers see identical
// "exists" answers. Used to short-circuit the closure-allocating
// resolveTargetFile when only file existence (not the read
// helper) matters.
func targetExists(f *lint.File, linkPath, resolvedRoot string) bool {
	if osPath, ok := resolveTargetOSPath(f.Path, linkPath); ok && cachedStatExists(osPath) {
		if resolvedRoot == "" || isWithinRoot(resolvedRoot, osPath) {
			return true
		}
		return false
	}
	// In-memory / workspace-relative resolution: the WASM and LSP engines
	// have no OS disk for the branch above. Resolve the link against the
	// source file's directory within the project-root FS, collapsing ".."
	// so io/fs — which rejects paths containing ".." — can stat an
	// up-and-over target (e.g. docs/x.md -> ../../internal/y.md).
	if f.RootFS != nil && !filepath.IsAbs(f.Path) {
		if rel, ok := resolveWorkspaceRelTarget(f.Path, linkPath); ok {
			if _, err := fs.Stat(f.RootFS, rel); err == nil {
				return true
			}
		}
	}
	fsPath := filepath.ToSlash(linkPath)
	fsPath = strings.TrimPrefix(fsPath, "./")
	if fsPath == "" || strings.HasPrefix(fsPath, "/") {
		return false
	}
	_, err := fs.Stat(f.FS, fsPath)
	return err == nil
}

func checkLocalAnchor(
	ctx *checkCtx, line, col int, target linkgraph.Target,
) []lint.Diagnostic {
	if setutil.Contains(ctx.ensureSelfAnchors(), linkgraph.NormalizeAnchor(target.Anchor)) {
		return nil
	}
	return []lint.Diagnostic{brokenHeadingDiag(ctx.f.Path, line, col, ctx.rule, target.Raw)}
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
		// MDS068's keys are tolerated so a single shared `links:`
		// block can configure both rules without forcing the user
		// to split the YAML. MDS068 reads the values from its own
		// settings map; here they are no-ops.
		case "style", "external-skip":
			// no-op for cross-file-reference-integrity
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

func anchorsForFile(
	host *lint.File, target targetFile, cache map[string]map[string]struct{},
) (map[string]struct{}, error) {
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
	var anchors map[string]struct{}
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

func (b anchorBuilder) build() (map[string]struct{}, error) {
	return buildAnchorsForTarget(b.target)
}

// buildAnchorsForTarget reads the target file's bytes, parses it,
// and collects the heading anchor set. Extracted so anchorsForFile
// can call it without going through the build closure that would
// otherwise capture `err` and escape to the heap.
func buildAnchorsForTarget(target targetFile) (map[string]struct{}, error) {
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
	if osPath, ok := resolveTargetOSPath(f.Path, linkPath); ok {
		if cachedStatExists(osPath) {
			// Reject links that resolve outside the project root,
			// evaluating symlinks to prevent bypass via symlinked dirs.
			if resolvedRoot != "" && !isWithinRoot(resolvedRoot, osPath) {
				return targetFile{}, false
			}
			return targetFile{
				cacheKey:    "os:" + osPath,
				runCacheKey: osPath,
				read: func() ([]byte, error) {
					return bytelimit.ReadFileLimited(osPath, maxBytes)
				},
			}, true
		}
	}

	// In-memory / workspace-relative resolution (see targetExists): resolve
	// the link within the project-root FS so an up-and-over ".." target,
	// which io/fs rejects as a raw path, still reads on the WASM/LSP engines.
	if f.RootFS != nil && !filepath.IsAbs(f.Path) {
		if rel, ok := resolveWorkspaceRelTarget(f.Path, linkPath); ok {
			if _, err := fs.Stat(f.RootFS, rel); err == nil {
				rootFS := f.RootFS
				return targetFile{
					cacheKey: "fs:" + rel,
					read: func() ([]byte, error) {
						return bytelimit.ReadFSFileLimited(rootFS, rel, maxBytes)
					},
				}, true
			}
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
			return bytelimit.ReadFSFileLimited(f.FS, fsPath, maxBytes)
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
	// filepath.Abs only errors when os.Getwd() fails, an OS-level
	// catastrophe; the cached form swallows that error too. Caching
	// at package scope is safe because both inputs are stable for
	// the process lifetime.
	return cachedAbs(realRoot)
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

// resolveWorkspaceRelTarget maps a workspace-relative source path and a
// file-relative link to a slash path valid for fs.Stat against the
// project-root FS (f.RootFS). It joins the link onto the source file's
// directory and cleans ".." away — io/fs rejects any path containing
// ".." — and returns ("", false) when the result escapes the workspace
// root, is empty, or is absolute, none of which name a file inside the
// in-memory workspace.
func resolveWorkspaceRelTarget(sourcePath, linkPath string) (string, bool) {
	lp := filepath.ToSlash(linkPath)
	if lp == "" || strings.HasPrefix(lp, "/") {
		return "", false
	}
	dir := path.Dir(filepath.ToSlash(sourcePath))
	rel := path.Clean(path.Join(dir, lp))
	if rel == "." || rel == ".." || strings.HasPrefix(rel, "../") {
		return "", false
	}
	return rel, true
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
