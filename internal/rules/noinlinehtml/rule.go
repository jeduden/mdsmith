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
			seg := node.Lines().At(0)
			if d, ok := r.checkRaw(f, allowed, seg.Value(f.Source), seg.Start); ok {
				diags = append(diags, d)
			}

		case *ast.RawHTML:
			seg := node.Segments.At(0)
			raw := rawHTMLBytes(node, f.Source)
			if d, ok := r.checkRaw(f, allowed, raw, seg.Start); ok {
				diags = append(diags, d)
			}
		}

		return ast.WalkContinue, nil
	})

	return diags
}

// checkRaw inspects raw HTML bytes and returns a diagnostic if the HTML
// should be flagged. offset is the byte position of the HTML in f.Source.
func (r *Rule) checkRaw(f *lint.File, allowed map[string]bool, raw []byte, offset int) (lint.Diagnostic, bool) {
	tag := extractTag(raw)
	switch {
	case tag == "":
		// PI directive or unrecognised — skip
		return lint.Diagnostic{}, false
	case tag == "<!--":
		if r.AllowComments {
			return lint.Diagnostic{}, false
		}
		return r.diag(f, offset, "<!--"), true
	case isClosingTag(raw):
		// Closing tags produce no extra diagnostic
		return lint.Diagnostic{}, false
	case allowed[tag]:
		return lint.Diagnostic{}, false
	default:
		return r.diag(f, offset, "<"+tag+">"), true
	}
}

func (r *Rule) allowSet() map[string]bool {
	m := make(map[string]bool, len(r.Allow))
	for _, t := range r.Allow {
		m[strings.ToLower(t)] = true
	}
	return m
}

func (r *Rule) diag(f *lint.File, offset int, display string) lint.Diagnostic {
	line, col := lineColOfOffset(f.Source, offset)
	return lint.Diagnostic{
		File:     f.Path,
		Line:     line,
		Column:   col,
		RuleID:   r.ID(),
		RuleName: r.Name(),
		Severity: lint.Warning,
		Message:  fmt.Sprintf("inline HTML %s is not allowed", display),
	}
}

// lineColOfOffset converts a byte offset in source to 1-based line and column numbers.
func lineColOfOffset(source []byte, offset int) (line, col int) {
	line = 1
	lineStart := 0
	for i := 0; i < offset && i < len(source); i++ {
		if source[i] == '\n' {
			line++
			lineStart = i + 1
		}
	}
	col = offset - lineStart + 1
	return
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

func rawHTMLBytes(n *ast.RawHTML, source []byte) []byte {
	var b []byte
	for i := 0; i < n.Segments.Len(); i++ {
		seg := n.Segments.At(i)
		b = append(b, seg.Value(source)...)
	}
	return b
}

var (
	_ rule.Defaultable  = (*Rule)(nil)
	_ rule.Configurable = (*Rule)(nil)
)
