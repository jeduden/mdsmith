package noinlinehtml

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"

	"github.com/yuin/goldmark/ast"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	rulesettings "github.com/jeduden/mdsmith/internal/rules/settings"
)

func init() {
	rule.Register(&Rule{})
}

// Rule implements MDS041, flagging raw HTML in Markdown documents.
type Rule struct {
	Allow         []string
	AllowComments bool
}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "MDS041" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "no-inline-html" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "meta" }

// EnabledByDefault implements rule.Defaultable. MDS041 is opt-in.
func (r *Rule) EnabledByDefault() bool { return false }

// DefaultSettings implements rule.Configurable.
func (r *Rule) DefaultSettings() map[string]any {
	return map[string]any{
		"allow":          []string{},
		"allow-comments": true,
	}
}

// ApplySettings implements rule.Configurable.
func (r *Rule) ApplySettings(settings map[string]any) error {
	for k, v := range settings {
		switch k {
		case "allow":
			tags, ok := rulesettings.ToStringSlice(v)
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

// Check implements rule.Rule.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	allowed := r.allowSet()
	var diags []lint.Diagnostic

	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		switch node := n.(type) {
		case *ast.HTMLBlock:
			raw := htmlBlockBytes(node, f.Source)
			if d, ok := r.checkRaw(f, allowed, raw, func() int { return blockLine(node, f) }); ok {
				diags = append(diags, d)
			}

		case *ast.RawHTML:
			raw := rawHTMLBytes(node, f.Source)
			if d, ok := r.checkRaw(f, allowed, raw, func() int { return inlineLine(node, f) }); ok {
				diags = append(diags, d)
			}
		}

		return ast.WalkContinue, nil
	})

	return diags
}

// checkRaw inspects raw HTML bytes and returns a diagnostic if the HTML
// should be flagged. lineFn is called lazily only when a diagnostic is emitted.
func (r *Rule) checkRaw(f *lint.File, allowed map[string]bool, raw []byte, lineFn func() int) (lint.Diagnostic, bool) {
	tag := extractTag(raw)
	switch {
	case tag == "":
		// PI directive or unrecognised — skip
		return lint.Diagnostic{}, false
	case tag == "<!--":
		if r.AllowComments {
			return lint.Diagnostic{}, false
		}
		return r.diag(f, lineFn(), "<!--"), true
	case isClosingTag(raw):
		// Closing tags produce no extra diagnostic
		return lint.Diagnostic{}, false
	case allowed[tag]:
		return lint.Diagnostic{}, false
	default:
		return r.diag(f, lineFn(), tag), true
	}
}

func (r *Rule) allowSet() map[string]bool {
	m := make(map[string]bool, len(r.Allow))
	for _, t := range r.Allow {
		m[strings.ToLower(t)] = true
	}
	return m
}

func (r *Rule) diag(f *lint.File, line int, tag string) lint.Diagnostic {
	return lint.Diagnostic{
		File:     f.Path,
		Line:     line,
		Column:   1,
		RuleID:   r.ID(),
		RuleName: r.Name(),
		Severity: lint.Warning,
		Message:  fmt.Sprintf("inline HTML <%s> is not allowed", tag),
	}
}

var tagNameRe = regexp.MustCompile(`(?i)</?([a-zA-Z][a-zA-Z0-9-]*)`)
var closingTagRe = regexp.MustCompile(`(?i)^<\s*/[a-zA-Z]`)

// extractTag returns the lowercase tag name from raw HTML bytes.
// Returns "<!--" for HTML comments, "" for PI directives or unrecognised input.
func extractTag(raw []byte) string {
	trimmed := bytes.TrimSpace(raw)
	if bytes.HasPrefix(trimmed, []byte("<?")) {
		return ""
	}
	if bytes.HasPrefix(trimmed, []byte("<!--")) {
		return "<!--"
	}
	m := tagNameRe.FindSubmatch(trimmed)
	if m == nil {
		return ""
	}
	return strings.ToLower(string(m[1]))
}

func isClosingTag(raw []byte) bool {
	return closingTagRe.Match(bytes.TrimSpace(raw))
}

func htmlBlockBytes(n *ast.HTMLBlock, source []byte) []byte {
	lines := n.Lines()
	if lines == nil || lines.Len() == 0 {
		return nil
	}
	seg := lines.At(0)
	return seg.Value(source)
}

func rawHTMLBytes(n *ast.RawHTML, source []byte) []byte {
	var b []byte
	for i := 0; i < n.Segments.Len(); i++ {
		seg := n.Segments.At(i)
		b = append(b, seg.Value(source)...)
	}
	return b
}

func blockLine(n *ast.HTMLBlock, f *lint.File) int {
	lines := n.Lines()
	if lines == nil || lines.Len() == 0 {
		return 1
	}
	seg := lines.At(0)
	return f.LineOfOffset(seg.Start)
}

func inlineLine(n *ast.RawHTML, f *lint.File) int {
	if n.Segments.Len() > 0 {
		seg := n.Segments.At(0)
		return f.LineOfOffset(seg.Start)
	}
	for p := n.Parent(); p != nil; p = p.Parent() {
		lines := p.Lines()
		if lines != nil && lines.Len() > 0 {
			seg := lines.At(0)
			return f.LineOfOffset(seg.Start)
		}
	}
	return 1
}

var (
	_ rule.Defaultable  = (*Rule)(nil)
	_ rule.Configurable = (*Rule)(nil)
)
