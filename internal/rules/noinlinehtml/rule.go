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
	// Register with AllowComments=true so that enabling the rule with
	// the bare boolean form (`no-inline-html: true`) matches what
	// DefaultSettings documents. ConfigureRule does not clone or apply
	// settings when cfg.Settings is nil, so the registered instance
	// must already reflect the documented default.
	rule.Register(&Rule{AllowComments: true})
}

// Rule implements MDS041, flagging raw HTML in Markdown documents.
//
// The lookup form of Allow is memoised on the per-Check *lint.File
// via File.Memo (see cachedAllowSet) rather than on the rule
// instance. Rule instances are shared across concurrent LSP calls
// (cmd/mdsmith/lsp.go reuses rule.All(), and ConfigureRule does
// not clone when cfg.Settings is nil), so any mutable state on the
// rule itself would race; *lint.File is created fresh per Check
// and File.Memo is sync.Map + sync.Once protected.
type Rule struct {
	Allow         []string
	AllowComments bool
}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "MDS041" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "no-inline-html" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "structural" }

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

// Check implements rule.Rule. The per-HTML-node logic is pure and
// stateless, so it is expressed as CheckNode and the engine can fold
// this rule into one shared AST walk; a direct call still works via
// rule.WalkNodes.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	return rule.WalkNodes(r, f)
}

// CheckNode implements rule.NodeChecker.
func (r *Rule) CheckNode(n ast.Node, entering bool, f *lint.File) []lint.Diagnostic {
	if !entering {
		return nil
	}
	// Build the allow-set lazily and only when we actually see an HTML
	// node. Most documents have many AST nodes and very few HTML ones,
	// so computing it unconditionally allocated a map per visit under
	// the multiplexed walk.
	switch node := n.(type) {
	case *ast.HTMLBlock:
		seg := node.Lines().At(0)
		raw := seg.Value(f.Source)
		offset := seg.Start
		if i := bytes.IndexByte(raw, '<'); i >= 0 {
			offset += i
		}
		if d, ok := r.checkRaw(f, r.cachedAllowSet(f), raw, offset); ok {
			return []lint.Diagnostic{d}
		}
	case *ast.RawHTML:
		seg := node.Segments.At(0)
		raw := rawHTMLBytes(node, f.Source)
		if d, ok := r.checkRaw(f, r.cachedAllowSet(f), raw, seg.Start); ok {
			return []lint.Diagnostic{d}
		}
	}
	return nil
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

// cachedAllowSet returns the lookup form of r.Allow, memoised on
// the per-Check *lint.File. File.Memo is sync.Map + sync.Once
// protected so the build runs at most once per File even under
// the LSP's concurrent reader pattern, where the same rule
// instance is shared across goroutines (config defaults set
// cfg.Settings=nil, which makes ConfigureRule a no-op).
func (r *Rule) cachedAllowSet(f *lint.File) map[string]bool {
	v := f.Memo("MDS041.allowSet", func() any { return r.allowSet() })
	return v.(map[string]bool)
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
	_ rule.NodeChecker  = (*Rule)(nil)
)
