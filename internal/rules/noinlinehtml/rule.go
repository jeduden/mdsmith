// Package noinlinehtml implements MDS041, which flags raw HTML in Markdown
// documents (block and inline). An allowlist lets teams permit tags that
// have no direct Markdown equivalent, and a toggle controls whether HTML
// comments are also flagged.
package noinlinehtml

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/jeduden/mdsmith/internal/rules/settings"
	"github.com/yuin/goldmark/ast"
)

func init() {
	rule.Register(&Rule{AllowComments: true})
}

// Rule flags raw HTML nodes in parsed Markdown ASTs.
type Rule struct {
	// Allow is the list of tag names that are permitted (case-insensitive).
	// Replace-mode: a later config layer replaces the list wholesale.
	Allow []string

	// AllowComments controls whether HTML comments (<!-- ... -->) are
	// allowed. Defaults to true so existing docs are not broken on
	// opt-in.
	AllowComments bool
}

// tagPattern extracts the tag name from raw HTML bytes.
// Matches both opening (<tag>) and closing (</tag>) tags.
var tagPattern = regexp.MustCompile(`(?i)<\/?([a-zA-Z][a-zA-Z0-9-]*)`)

// extractTagName returns the lowercase tag name from raw HTML bytes, or
// the empty string if no valid tag name is found.
// For comments (bytes starting with "<!--"), returns "<!--".
func extractTagName(raw []byte) string {
	if bytes.HasPrefix(raw, []byte("<!--")) {
		return "<!--"
	}
	m := tagPattern.FindSubmatch(raw)
	if m == nil {
		return ""
	}
	return strings.ToLower(string(m[1]))
}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "MDS041" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "no-inline-html" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "meta" }

// EnabledByDefault implements rule.Defaultable.
func (r *Rule) EnabledByDefault() bool { return false }

// Check implements rule.Rule.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	var diags []lint.Diagnostic
	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		raw, startOffset, ok := nodeRawBytes(n, f.Source)
		if !ok {
			return ast.WalkContinue, nil
		}
		if d := r.checkRaw(f, raw, startOffset); d != nil {
			diags = append(diags, *d)
		}
		return ast.WalkContinue, nil
	})
	return diags
}

// nodeRawBytes extracts the first-line raw bytes and start offset for
// HTMLBlock and RawHTML nodes. Returns ok=false for all other node types.
func nodeRawBytes(n ast.Node, src []byte) ([]byte, int, bool) {
	switch node := n.(type) {
	case *ast.HTMLBlock:
		lines := node.Lines()
		if lines.Len() == 0 {
			return nil, 0, false
		}
		seg := lines.At(0)
		return src[seg.Start:seg.Stop], seg.Start, true
	case *ast.RawHTML:
		if node.Segments.Len() == 0 {
			return nil, 0, false
		}
		seg := node.Segments.At(0)
		return seg.Value(src), seg.Start, true
	}
	return nil, 0, false
}

// checkRaw inspects the raw bytes of one HTML node and returns a diagnostic
// if the node should be flagged, or nil if it should be skipped.
func (r *Rule) checkRaw(f *lint.File, raw []byte, startOffset int) *lint.Diagnostic {
	trimmed := bytes.TrimSpace(raw)

	// Skip mdsmith directives (inline <?...?> forms not converted by the
	// block PI parser) and closing tags (the opening tag flags the element).
	if bytes.HasPrefix(trimmed, []byte("<?")) || bytes.HasPrefix(trimmed, []byte("</")) {
		return nil
	}

	tag := extractTagName(trimmed)
	if tag == "" || (tag == "<!--" && r.AllowComments) || r.isAllowed(tag) {
		return nil
	}

	return &lint.Diagnostic{
		File:     f.Path,
		Line:     f.LineOfOffset(startOffset),
		Column:   columnOfOffset(f.Source, startOffset),
		RuleID:   r.ID(),
		RuleName: r.Name(),
		Severity: lint.Warning,
		Message:  buildMessage(tag),
	}
}

// buildMessage returns the diagnostic message for the given tag name.
// Comments are reported as "<!--" to keep the message distinct.
func buildMessage(tag string) string {
	if tag == "<!--" {
		return "inline HTML <!-- is not allowed"
	}
	return fmt.Sprintf("inline HTML <%s> is not allowed", tag)
}

// isAllowed reports whether the given (already lowercased) tag name is in
// the allowlist.
func (r *Rule) isAllowed(tag string) bool {
	for _, a := range r.Allow {
		if strings.ToLower(a) == tag {
			return true
		}
	}
	return false
}

// columnOfOffset returns the 1-based column of the given byte offset in src.
func columnOfOffset(src []byte, offset int) int {
	lineStart := 0
	for i := 0; i < offset && i < len(src); i++ {
		if src[i] == '\n' {
			lineStart = i + 1
		}
	}
	return offset - lineStart + 1
}

// ApplySettings implements rule.Configurable.
func (r *Rule) ApplySettings(s map[string]any) error {
	for k, v := range s {
		switch k {
		case "allow":
			tags, ok := settings.ToStringSlice(v)
			if !ok {
				return fmt.Errorf("no-inline-html: allow must be a list of strings, got %T", v)
			}
			r.Allow = tags
		case "allow-comments":
			b, ok := v.(bool)
			if !ok {
				return fmt.Errorf("no-inline-html: allow-comments must be a bool, got %T", v)
			}
			r.AllowComments = b
		default:
			return fmt.Errorf("no-inline-html: unknown setting %q", k)
		}
	}
	return nil
}

// DefaultSettings implements rule.Configurable.
func (r *Rule) DefaultSettings() map[string]any {
	return map[string]any{
		"allow":          []string{},
		"allow-comments": true,
	}
}

// SettingMergeMode implements rule.ListMerger.
// Both settings use replace-mode: allow replaces the list wholesale
// (teams that want to extend the defaults must restate all tags), and
// allow-comments is a scalar replaced by the later layer.
func (r *Rule) SettingMergeMode(_ string) rule.MergeMode {
	return rule.MergeReplace
}

var (
	_ rule.Configurable = (*Rule)(nil)
	_ rule.ListMerger   = (*Rule)(nil)
	_ rule.Defaultable  = (*Rule)(nil)
)
