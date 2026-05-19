package descriptivelinktext

import (
	"fmt"
	"strings"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/jeduden/mdsmith/internal/rules/settings"
	"github.com/yuin/goldmark/ast"
)

var defaultBanned = []string{"click here", "here", "link", "more"}

func init() {
	rule.Register(&Rule{Banned: append([]string(nil), defaultBanned...)})
}

// Rule flags links whose visible text is a non-descriptive phrase such as
// "click here", "here", "link", or "more".
type Rule struct {
	Banned []string
}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "MDS063" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "descriptive-link-text" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "prose" }

// EnabledByDefault implements rule.Defaultable. MDS063 is opt-in.
func (r *Rule) EnabledByDefault() bool { return false }

// ApplySettings implements rule.Configurable.
// banned replaces (not appends to) the default phrase list.
func (r *Rule) ApplySettings(s map[string]any) error {
	for k, v := range s {
		switch k {
		case "banned":
			ss, ok := settings.ToStringSlice(v)
			if !ok {
				return fmt.Errorf("descriptive-link-text: banned must be a list of strings, got %T", v)
			}
			r.Banned = ss
		default:
			return fmt.Errorf("descriptive-link-text: unknown setting %q", k)
		}
	}
	return nil
}

// DefaultSettings implements rule.Configurable.
func (r *Rule) DefaultSettings() map[string]any {
	return map[string]any{
		"banned": append([]string(nil), defaultBanned...),
	}
}

// Check implements rule.Rule. The per-link logic is pure and
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
	if len(r.Banned) == 0 {
		return nil
	}
	link, ok := n.(*ast.Link)
	if !ok {
		return nil
	}
	if isOnlyImageChild(link) || isOnlyCodeSpanChild(link) {
		return nil
	}

	// Banned-set construction is cheap, but doing it per node is wasteful
	// when there are many links. The set is per-rule-config (Banned),
	// so memoise on r.banned only when r.Banned hasn't changed. For
	// simplicity and correctness — and because rule clones are per
	// Check (so a fresh slice maps to a fresh check pass) — just build
	// it inline per call. The work is bounded by len(Banned) ~ <10.
	bannedSet := make(map[string]bool, len(r.Banned))
	for _, b := range r.Banned {
		bannedSet[normalizeText(b)] = true
	}

	text := collectLinkText(link, f.Source)
	if !bannedSet[normalizeText(text)] {
		return nil
	}
	line := linkLine(link, f)
	return []lint.Diagnostic{{
		File:     f.Path,
		Line:     line,
		Column:   1,
		RuleID:   r.ID(),
		RuleName: r.Name(),
		Severity: lint.Warning,
		Message:  fmt.Sprintf("link text %q is not descriptive", text),
	}}
}

// normalizeText trims, lowercases, and collapses internal whitespace.
func normalizeText(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(s), " "))
}

// isOnlyImageChild reports whether link's sole child is an image node.
func isOnlyImageChild(link *ast.Link) bool {
	c := link.FirstChild()
	return c != nil && c.NextSibling() == nil && c.Kind() == ast.KindImage
}

// isOnlyCodeSpanChild reports whether link's sole child is a code span.
func isOnlyCodeSpanChild(link *ast.Link) bool {
	c := link.FirstChild()
	return c != nil && c.NextSibling() == nil && c.Kind() == ast.KindCodeSpan
}

// collectLinkText returns all plain text within the link node, including
// text nested inside emphasis or other inline formatting.
func collectLinkText(n ast.Node, source []byte) string {
	var b strings.Builder
	collectText(&b, n, source)
	return b.String()
}

func collectText(b *strings.Builder, n ast.Node, source []byte) {
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		if t, ok := c.(*ast.Text); ok {
			b.Write(t.Segment.Value(source))
			if t.SoftLineBreak() || t.HardLineBreak() {
				b.WriteByte(' ')
			}
		} else {
			collectText(b, c, source)
		}
	}
}

// linkLine returns the 1-based source line of the first text node inside
// the link, falling back to 1 if none exists.
func linkLine(link *ast.Link, f *lint.File) int {
	line := 1
	_ = ast.Walk(link, func(n ast.Node, _ bool) (ast.WalkStatus, error) {
		t, ok := n.(*ast.Text)
		if !ok {
			return ast.WalkContinue, nil
		}
		line = f.LineOfOffset(t.Segment.Start)
		return ast.WalkStop, nil
	})
	return line
}

var (
	_ rule.Configurable = (*Rule)(nil)
	_ rule.Defaultable  = (*Rule)(nil)
	_ rule.NodeChecker  = (*Rule)(nil)
)
