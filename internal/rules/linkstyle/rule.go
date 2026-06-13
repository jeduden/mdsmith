// Package linkstyle implements MDS068, an opt-in rule that flags
// links whose path style, extension policy, or inline-vs-reference
// form deviates from the project's declared `links.style` policy.
// See docs/research/links/README.md gap G8 for the design.
package linkstyle

import (
	"bytes"
	"fmt"
	"path"
	"strings"

	"github.com/jeduden/mdsmith/pkg/goldmark/ast"

	"github.com/jeduden/mdsmith/internal/linkgraph"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
)

func init() {
	rule.Register(&Rule{})
}

// LinksConfig holds the `links:` sub-block this rule reads. The same
// block name is used by MDS027; the two rules each parse the keys
// they care about. The future external-link-check rule (issue #47)
// reads ExternalSkip from this same block.
type LinksConfig struct {
	Style        StyleConfig
	ExternalSkip []string
}

// StyleConfig captures the policy axes the rule enforces.
// Empty strings mean "no check" for the string axes; LinkImageStyle.Active
// false means the MD054 axis is inactive. Users can enable one axis
// without committing to all others.
type StyleConfig struct {
	Path           string // "relative" | "absolute" | ""
	Extension      string // "keep" | "strip" | ""
	Form           string // "inline" | "reference" | "any" | ""
	LinkImageStyle LinkImageStyleConfig
}

// LinkImageStyleConfig holds the six MD054 link/image style toggles.
// Active is set to true when the user explicitly configures this axis
// via links.style.link-image-style; false means the axis is inactive
// so no diagnostics are emitted regardless of toggle values.
//
// Each toggle is true (allow) or false (forbid). When Active is true
// and a toggle is false the corresponding link/image form is forbidden.
// Default for all six toggles when Active is true is true (allow), so
// an enabled-but-unconfigured axis is a no-op, matching markdownlint.
//
// Design choice: `form` (the legacy three-value string) and this axis
// are kept independent. Both can be active simultaneously. `form` was
// the original coarse-grained axis; link-image-style is the full MD054
// replacement. Users who have already configured `form` are not forced
// to migrate — both checks run and produce separate diagnostics.
type LinkImageStyleConfig struct {
	Active      bool
	Autolink    bool // <https://x>
	Inline      bool // [t](u)
	Full        bool // [t][label]
	Collapsed   bool // [t][]
	Shortcut    bool // [t]
	InlineImage bool // ![alt](src)
}

// Rule flags links whose style deviates from the declared policy.
type Rule struct {
	Links LinksConfig
}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "MDS068" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "link-style" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "link" }

// EnabledByDefault implements rule.Defaultable.
func (r *Rule) EnabledByDefault() bool { return false }

const (
	msgPathRelative  = "link target is absolute; style.path=relative requires a relative path"
	msgPathAbsolute  = "link target is relative; style.path=absolute requires an absolute path"
	msgExtensionKeep = "link target has no markdown extension; style.extension=keep requires .md or .markdown"
	msgExtStripFmt   = "link target has a markdown extension; style.extension=strip forbids .md and .markdown"
	msgFormInline    = "reference-style link; style.form=inline requires inline form [text](url)"
	msgFormReference = "inline link; style.form=reference requires reference form [text][label]"

	msgLISAutolink    = "autolink style forbidden; link-image-style.autolink=false"
	msgLISInline      = "inline style forbidden; link-image-style.inline=false"
	msgLISFull        = "full reference style forbidden; link-image-style.full=false"
	msgLISCollapsed   = "collapsed reference style forbidden; link-image-style.collapsed=false"
	msgLISShortcut    = "shortcut reference style forbidden; link-image-style.shortcut=false"
	msgLISInlineImage = "inline-image style forbidden; link-image-style.inline-image=false"
)

// Check implements rule.Rule.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	style := r.Links.Style
	if style.Path == "" && style.Extension == "" && style.Form == "" && !style.LinkImageStyle.Active {
		return nil
	}

	var diags []lint.Diagnostic
	for _, link := range linkgraph.Links(f) {
		diags = append(diags, r.checkOne(f, link, false)...)
	}
	for _, link := range linkgraph.RefLinkTargets(f) {
		diags = append(diags, r.checkOne(f, link, true)...)
	}

	// The link-image-style axis needs direct AST walking for nodes
	// not covered by the linkgraph extractors:
	//   - autolinks (ast.AutoLink): not emitted by ExtractLinks
	//   - reference sub-forms (full/collapsed/shortcut): RefLinkTargets
	//     resolves the destination but drops the Reference sub-form
	//   - inline images (ast.Image): not included in link checks above
	if style.LinkImageStyle.Active {
		diags = append(diags, r.checkLinkImageStyle(f)...)
	}
	return diags
}

// checkOne returns 0..3 diagnostics for one link occurrence.
// External links (scheme/host) are excluded earlier by
// linkgraph.ParseTarget; local-anchor-only destinations
// (`#section`) reach this function with LocalAnchor=true and are
// filtered out here because they carry no path or form to judge.
func (r *Rule) checkOne(f *lint.File, link linkgraph.Link, isRef bool) []lint.Diagnostic {
	target := link.Target
	if target.LocalAnchor {
		return nil
	}
	style := r.Links.Style
	var diags []lint.Diagnostic
	if msg := checkPath(style.Path, target.Path); msg != "" {
		diags = append(diags, diag(f, link, r, msg))
	}
	if msg := checkExtension(style.Extension, target.Path); msg != "" {
		diags = append(diags, diag(f, link, r, msg))
	}
	if msg := checkForm(style.Form, isRef); msg != "" {
		diags = append(diags, diag(f, link, r, msg))
	}
	return diags
}

// checkLinkImageStyle walks the AST and emits a diagnostic for every
// link or image whose style is forbidden by the link-image-style axis.
// It covers autolinks, all three reference sub-forms, inline links,
// and inline images — the full set of MD054 styles.
func (r *Rule) checkLinkImageStyle(f *lint.File) []lint.Diagnostic {
	if f == nil || f.AST == nil {
		return nil
	}
	lis := r.Links.Style.LinkImageStyle
	var diags []lint.Diagnostic
	add := func(line, col int, msg string) {
		diags = append(diags, lint.Diagnostic{
			File:     f.Path,
			Line:     line,
			Column:   col,
			RuleID:   r.ID(),
			RuleName: r.Name(),
			Severity: lint.Warning,
			Message:  msg,
		})
	}
	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch node := n.(type) {
		case *ast.AutoLink:
			if !lis.Autolink {
				line, col := autolinkPosition(f, node)
				add(line, col, msgLISAutolink)
			}
		case *ast.Link:
			if msg := linkImageStyleMsg(lis, node.Reference, false); msg != "" {
				line, col := linkNodePosition(f, node)
				add(line, col, msg)
			}
		case *ast.Image:
			// Images carry the same Reference sub-form as links, so a
			// reference-style image (![alt][label], ![alt][], ![alt])
			// is checked against full/collapsed/shortcut; only an
			// inline image (![alt](src)) uses the inline-image toggle.
			if msg := linkImageStyleMsg(lis, node.Reference, true); msg != "" {
				line, col := linkNodePosition(f, node)
				add(line, col, msg)
			}
		}
		return ast.WalkContinue, nil
	})
	return diags
}

// linkImageStyleMsg returns the forbidden-style message for a link or
// image node given its reference (nil for the inline form), or "" if
// the form is allowed. isImage selects the inline-image vs inline
// message for the non-reference case; the three reference sub-forms
// (full/collapsed/shortcut) share their messages across links and
// images, matching MD054.
func linkImageStyleMsg(lis LinkImageStyleConfig, ref *ast.ReferenceLink, isImage bool) string {
	if ref == nil {
		// Inline form: [text](url) or ![alt](src).
		if isImage {
			if !lis.InlineImage {
				return msgLISInlineImage
			}
			return ""
		}
		if !lis.Inline {
			return msgLISInline
		}
		return ""
	}
	switch ref.Type {
	case ast.ReferenceLinkFull:
		if !lis.Full {
			return msgLISFull
		}
	case ast.ReferenceLinkCollapsed:
		if !lis.Collapsed {
			return msgLISCollapsed
		}
	case ast.ReferenceLinkShortcut:
		if !lis.Shortcut {
			return msgLISShortcut
		}
	}
	return ""
}

// autolinkPosition returns the 1-based line and column of an AutoLink
// node in body-relative coordinates.
//
// AutoLink stores its URL text in a private field (not a child node),
// so ast.Walk cannot find its position. Instead we locate the nearest
// block ancestor with a Lines() set and search its source bytes for
// the `<url>` pattern to find the `<` offset.
func autolinkPosition(f *lint.File, n *ast.AutoLink) (int, int) {
	url := n.URL(f.Source)
	if len(url) == 0 {
		return 1, 1
	}
	// Match the literal `<url>` including the closing `>`, so a short
	// autolink whose URL is a prefix of a neighbour's (e.g. `<a.com>`
	// beside `<a.com/x>` on one line) does not match the longer one.
	pat := make([]byte, 0, len(url)+2)
	pat = append(pat, '<')
	pat = append(pat, url...)
	pat = append(pat, '>')
	// Walk up to the nearest block ancestor for a source range to
	// search. Lines() panics on inline nodes, so skip any ancestor
	// that is not a block (emphasis, link text, the document root).
	for p := n.Parent(); p != nil; p = p.Parent() {
		if p.Type() != ast.TypeBlock {
			continue
		}
		// Search each source line for the literal `<url>`. If no block
		// line contains it, fall through to the (1,1) fallback below.
		lines := p.Lines()
		for i := range lines.Len() {
			seg := lines.At(i)
			if idx := bytes.Index(seg.Value(f.Source), pat); idx >= 0 {
				off := seg.Start + idx
				return f.LineOfOffset(off), f.ColumnOfOffset(off)
			}
		}
		break
	}
	return 1, 1
}

// linkNodePosition returns the 1-based line and column of a link or
// image node by locating its first text child, in body-relative
// coordinates.
func linkNodePosition(f *lint.File, n ast.Node) (int, int) {
	offset := -1
	_ = ast.Walk(n, func(cur ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if t, ok := cur.(*ast.Text); ok {
			if offset == -1 || t.Segment.Start < offset {
				offset = t.Segment.Start
			}
		}
		return ast.WalkContinue, nil
	})
	if offset < 0 {
		return 1, 1
	}
	return f.LineOfOffset(offset), f.ColumnOfOffset(offset)
}

// checkPath returns a diagnostic message when the configured path
// style does not match this target, or "" to pass.
func checkPath(policy, targetPath string) string {
	if policy == "" || targetPath == "" {
		return ""
	}
	isAbs := strings.HasPrefix(targetPath, "/")
	switch policy {
	case "relative":
		if isAbs {
			return msgPathRelative
		}
	case "absolute":
		if !isAbs {
			return msgPathAbsolute
		}
	}
	return ""
}

// checkExtension returns a diagnostic message when the configured
// extension policy does not match this target's path. Only Markdown-
// shaped targets are considered: a `.md` suffix counts under keep,
// no extension at all counts under strip. Other suffixes (`.png`,
// `.css`, …) are not Markdown links and are silently ignored.
// Directory-style targets (trailing `/`, `.`, `..`) reference a
// rendered page directory rather than a file and are also skipped.
func checkExtension(policy, targetPath string) string {
	if policy == "" || targetPath == "" {
		return ""
	}
	if strings.HasSuffix(targetPath, "/") {
		return ""
	}
	base := path.Base(targetPath)
	if base == "." || base == ".." {
		return ""
	}
	ext := strings.ToLower(path.Ext(base))
	isMD := ext == ".md" || ext == ".markdown"
	hasOtherExt := ext != "" && !isMD
	if hasOtherExt {
		return ""
	}
	switch policy {
	case "keep":
		if !isMD {
			return msgExtensionKeep
		}
	case "strip":
		if isMD {
			return msgExtStripFmt
		}
	}
	return ""
}

// checkForm returns a diagnostic message when the link form does
// not match the configured policy. "any" and "" are permissive.
func checkForm(policy string, isRef bool) string {
	switch policy {
	case "inline":
		if isRef {
			return msgFormInline
		}
	case "reference":
		if !isRef {
			return msgFormReference
		}
	}
	return ""
}

func diag(f *lint.File, link linkgraph.Link, r *Rule, msg string) lint.Diagnostic {
	return lint.Diagnostic{
		File:     f.Path,
		Line:     link.Line,
		Column:   link.Column,
		RuleID:   r.ID(),
		RuleName: r.Name(),
		Severity: lint.Warning,
		Message:  msg,
	}
}

// ApplySettings implements rule.Configurable.
func (r *Rule) ApplySettings(settings map[string]any) error {
	for k, v := range settings {
		if err := r.applyOne(k, v); err != nil {
			return err
		}
	}
	return nil
}

func (r *Rule) applyOne(key string, v any) error {
	switch key {
	case "links":
		m, ok := v.(map[string]any)
		if !ok {
			return fmt.Errorf("link-style: links must be a map, got %T", v)
		}
		return r.applyLinks(m)
	}
	return fmt.Errorf("link-style: unknown setting %q", key)
}

func (r *Rule) applyLinks(m map[string]any) error {
	for k, v := range m {
		switch k {
		case "style":
			sm, ok := v.(map[string]any)
			if !ok {
				return fmt.Errorf("link-style: links.style must be a map, got %T", v)
			}
			if err := r.applyStyle(sm); err != nil {
				return err
			}
		case "external-skip":
			list, ok := toStringSlice(v)
			if !ok {
				return fmt.Errorf("link-style: links.external-skip must be a list of strings, got %T", v)
			}
			r.Links.ExternalSkip = list
		// MDS027's keys are tolerated so a single shared `links:`
		// block can configure both rules without forcing the user
		// to split the YAML. The values are ignored here — MDS027
		// reads them from its own settings map.
		case "site-root", "validate-images", "validate-reference-style":
			// no-op for link-style
		default:
			return fmt.Errorf("link-style: unknown links setting %q", k)
		}
	}
	return nil
}

func (r *Rule) applyStyle(m map[string]any) error {
	for k, v := range m {
		switch k {
		case "path":
			s, ok := v.(string)
			if !ok {
				return fmt.Errorf("link-style: links.style.path must be a string, got %T", v)
			}
			if err := validatePathStyle(s); err != nil {
				return err
			}
			r.Links.Style.Path = s
		case "extension":
			s, ok := v.(string)
			if !ok {
				return fmt.Errorf("link-style: links.style.extension must be a string, got %T", v)
			}
			if err := validateExtensionStyle(s); err != nil {
				return err
			}
			r.Links.Style.Extension = s
		case "form":
			s, ok := v.(string)
			if !ok {
				return fmt.Errorf("link-style: links.style.form must be a string, got %T", v)
			}
			if err := validateFormStyle(s); err != nil {
				return err
			}
			// "any" and "" both mean "no form check"; normalize to
			// "" so Check's no-op fast path stays a single
			// three-way empty-string comparison.
			if s == "any" {
				s = ""
			}
			r.Links.Style.Form = s
		case "link-image-style":
			lism, ok := v.(map[string]any)
			if !ok {
				return fmt.Errorf("link-style: links.style.link-image-style must be a map, got %T", v)
			}
			if err := r.applyLinkImageStyle(lism); err != nil {
				return err
			}
		default:
			return fmt.Errorf("link-style: unknown links.style setting %q", k)
		}
	}
	return nil
}

// applyLinkImageStyle parses the six MD054 boolean toggles. Calling
// this method even with an empty map marks the axis Active so the
// defaults (all allowed) take effect — matching markdownlint which
// emits nothing when MD054 is enabled with defaults.
func (r *Rule) applyLinkImageStyle(m map[string]any) error {
	// Start with all-allowed defaults; mark the axis active.
	lis := LinkImageStyleConfig{
		Active:      true,
		Autolink:    true,
		Inline:      true,
		Full:        true,
		Collapsed:   true,
		Shortcut:    true,
		InlineImage: true,
	}
	for k, v := range m {
		b, ok := v.(bool)
		if !ok {
			return fmt.Errorf(
				"link-style: links.style.link-image-style.%s must be a bool, got %T", k, v)
		}
		switch k {
		case "autolink":
			lis.Autolink = b
		case "inline":
			lis.Inline = b
		case "full":
			lis.Full = b
		case "collapsed":
			lis.Collapsed = b
		case "shortcut":
			lis.Shortcut = b
		case "inline-image":
			lis.InlineImage = b
		default:
			return fmt.Errorf(
				"link-style: unknown links.style.link-image-style key %q", k)
		}
	}
	r.Links.Style.LinkImageStyle = lis
	return nil
}

func validatePathStyle(s string) error {
	switch s {
	case "", "relative", "absolute":
		return nil
	}
	return fmt.Errorf(
		"link-style: links.style.path %q not supported; "+
			"want \"relative\", \"absolute\", or \"\" (disables the check)", s)
}

func validateExtensionStyle(s string) error {
	switch s {
	case "", "keep", "strip":
		return nil
	}
	return fmt.Errorf(
		"link-style: links.style.extension %q not supported; "+
			"want \"keep\", \"strip\", or \"\" (disables the check)", s)
}

func validateFormStyle(s string) error {
	switch s {
	case "", "any", "inline", "reference":
		return nil
	}
	return fmt.Errorf(
		"link-style: links.style.form %q not supported; "+
			"want \"inline\", \"reference\", \"any\", or \"\" (disables the check)", s)
}

// DefaultSettings implements rule.Configurable. All policy strings
// default to "" so an enabled rule with no further config does
// nothing — every check is explicitly opted into.
func (r *Rule) DefaultSettings() map[string]any {
	return map[string]any{
		"links": map[string]any{
			"style": map[string]any{
				"path":      "",
				"extension": "",
				"form":      "",
			},
			"external-skip": []string{},
		},
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
	}
	return nil, false
}

var (
	_ rule.Configurable = (*Rule)(nil)
	_ rule.Defaultable  = (*Rule)(nil)
)
