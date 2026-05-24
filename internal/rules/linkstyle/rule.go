// Package linkstyle implements MDS068, an opt-in rule that flags
// links whose path style, extension policy, or inline-vs-reference
// form deviates from the project's declared `links.style` policy.
// See docs/research/links/README.md gap G8 for the design.
package linkstyle

import (
	"fmt"
	"path"
	"strings"

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

// StyleConfig captures the three policy axes the rule enforces.
// Empty strings mean "no check" so users can enable one axis without
// committing to all three.
type StyleConfig struct {
	Path      string // "relative" | "absolute" | ""
	Extension string // "keep" | "strip" | ""
	Form      string // "inline" | "reference" | "any" | ""
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
)

// Check implements rule.Rule.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	style := r.Links.Style
	if style.Path == "" && style.Extension == "" && style.Form == "" {
		return nil
	}

	var diags []lint.Diagnostic
	for _, link := range linkgraph.ExtractLinks(f) {
		diags = append(diags, r.checkOne(f, link, false)...)
	}
	for _, link := range linkgraph.ExtractRefLinkTargets(f) {
		diags = append(diags, r.checkOne(f, link, true)...)
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
func checkExtension(policy, targetPath string) string {
	if policy == "" || targetPath == "" {
		return ""
	}
	base := path.Base(targetPath)
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
		s, ok := v.(string)
		if !ok {
			return fmt.Errorf("link-style: links.style.%s must be a string, got %T", k, v)
		}
		switch k {
		case "path":
			if err := validatePathStyle(s); err != nil {
				return err
			}
			r.Links.Style.Path = s
		case "extension":
			if err := validateExtensionStyle(s); err != nil {
				return err
			}
			r.Links.Style.Extension = s
		case "form":
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
		default:
			return fmt.Errorf("link-style: unknown links.style setting %q", k)
		}
	}
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
