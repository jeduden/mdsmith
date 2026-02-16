package emptysectionbody

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/mdtext"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/yuin/goldmark/ast"
)

const (
	defaultMinLevel    = 2
	defaultMaxLevel    = 6
	defaultAllowMarker = "allow-empty-section"
)

var htmlCommentPattern = regexp.MustCompile(`(?s)<!--.*?-->`)

func init() {
	rule.Register(&Rule{
		MinLevel:    defaultMinLevel,
		MaxLevel:    defaultMaxLevel,
		AllowMarker: defaultAllowMarker,
	})
}

// Rule reports headings whose section body has no meaningful content.
type Rule struct {
	MinLevel    int
	MaxLevel    int
	AllowMarker string
}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "MDS030" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "empty-section-body" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "heading" }

// Check implements rule.Rule.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	minLevel, maxLevel, allowMarker := r.effectiveSettings()
	if f.AST == nil {
		return nil
	}

	nodes := topLevelNodes(f.AST)
	if len(nodes) == 0 {
		return nil
	}

	var diags []lint.Diagnostic
	for i, node := range nodes {
		heading, ok := node.(*ast.Heading)
		if !ok {
			continue
		}
		if heading.Level < minLevel || heading.Level > maxLevel {
			continue
		}

		end := len(nodes)
		for j := i + 1; j < len(nodes); j++ {
			nextHeading, ok := nodes[j].(*ast.Heading)
			if !ok {
				continue
			}
			if nextHeading.Level <= heading.Level {
				end = j
				break
			}
		}

		sectionNodes := nodes[i+1 : end]
		if hasAllowMarker(sectionNodes, f.Source, allowMarker) {
			continue
		}
		if hasMeaningfulContent(sectionNodes, f.Source) {
			continue
		}

		message := fmt.Sprintf(
			"section %q has no meaningful body content; "+
				"add paragraph, list, table, or code content, "+
				"or add %q for an intentional empty section",
			headingLabel(heading, f.Source),
			fmt.Sprintf("<!-- %s -->", allowMarker),
		)
		diags = append(diags, lint.Diagnostic{
			File:     f.Path,
			Line:     headingLine(heading, f),
			Column:   1,
			RuleID:   r.ID(),
			RuleName: r.Name(),
			Severity: lint.Warning,
			Message:  message,
		})
	}

	return diags
}

// ApplySettings implements rule.Configurable.
func (r *Rule) ApplySettings(settings map[string]any) error {
	minLevel, maxLevel, allowMarker := r.effectiveSettings()

	for key, value := range settings {
		if err := applySetting(
			&minLevel,
			&maxLevel,
			&allowMarker,
			key,
			value,
		); err != nil {
			return err
		}
	}

	if err := validateLevels(minLevel, maxLevel); err != nil {
		return err
	}

	r.MinLevel = minLevel
	r.MaxLevel = maxLevel
	r.AllowMarker = allowMarker
	return nil
}

func applySetting(
	minLevel *int,
	maxLevel *int,
	allowMarker *string,
	key string,
	value any,
) error {
	switch key {
	case "min-level":
		n, ok := toInt(value)
		if !ok {
			return fmt.Errorf(
				"empty-section-body: min-level must be an integer, got %T",
				value,
			)
		}
		*minLevel = n
	case "max-level":
		n, ok := toInt(value)
		if !ok {
			return fmt.Errorf(
				"empty-section-body: max-level must be an integer, got %T",
				value,
			)
		}
		*maxLevel = n
	case "allow-marker":
		s, ok := value.(string)
		if !ok {
			return fmt.Errorf(
				"empty-section-body: allow-marker must be a string, got %T",
				value,
			)
		}
		s = strings.TrimSpace(s)
		if s == "" {
			return fmt.Errorf(
				"empty-section-body: allow-marker cannot be empty or whitespace-only",
			)
		}
		*allowMarker = s
	default:
		return fmt.Errorf("empty-section-body: unknown setting %q", key)
	}

	return nil
}

func validateLevels(minLevel, maxLevel int) error {
	if minLevel < 1 || minLevel > 6 {
		return fmt.Errorf(
			"empty-section-body: min-level must be between 1 and 6, got %d",
			minLevel,
		)
	}
	if maxLevel < 1 || maxLevel > 6 {
		return fmt.Errorf(
			"empty-section-body: max-level must be between 1 and 6, got %d",
			maxLevel,
		)
	}
	if minLevel > maxLevel {
		return fmt.Errorf(
			"empty-section-body: min-level (%d) cannot be greater than max-level (%d)",
			minLevel, maxLevel,
		)
	}
	return nil
}

// DefaultSettings implements rule.Configurable.
func (r *Rule) DefaultSettings() map[string]any {
	return map[string]any{
		"min-level":    defaultMinLevel,
		"max-level":    defaultMaxLevel,
		"allow-marker": defaultAllowMarker,
	}
}

func (r *Rule) effectiveSettings() (int, int, string) {
	minLevel := r.MinLevel
	if minLevel == 0 {
		minLevel = defaultMinLevel
	}

	maxLevel := r.MaxLevel
	if maxLevel == 0 {
		maxLevel = defaultMaxLevel
	}

	allowMarker := r.AllowMarker
	if allowMarker == "" {
		allowMarker = defaultAllowMarker
	}

	if minLevel < 1 || minLevel > 6 || maxLevel < 1 || maxLevel > 6 || minLevel > maxLevel {
		return defaultMinLevel, defaultMaxLevel, defaultAllowMarker
	}

	return minLevel, maxLevel, allowMarker
}

func topLevelNodes(root ast.Node) []ast.Node {
	var nodes []ast.Node
	for n := root.FirstChild(); n != nil; n = n.NextSibling() {
		nodes = append(nodes, n)
	}
	return nodes
}

func hasAllowMarker(nodes []ast.Node, source []byte, marker string) bool {
	if marker == "" {
		return false
	}
	trimmed := strings.TrimSpace(marker)
	if trimmed == "" {
		return false
	}
	pattern := regexp.MustCompile(
		`(?is)<!--\s*` + regexp.QuoteMeta(trimmed) + `\s*-->`,
	)

	for _, node := range nodes {
		block, ok := node.(*ast.HTMLBlock)
		if !ok {
			continue
		}
		if pattern.MatchString(nodeLinesText(block, source)) {
			return true
		}
	}
	return false
}

func hasMeaningfulContent(nodes []ast.Node, source []byte) bool {
	for _, node := range nodes {
		switch n := node.(type) {
		case *ast.Heading:
			continue
		case *ast.HTMLBlock:
			raw := stripHTMLComments(nodeLinesText(n, source))
			if strings.TrimSpace(raw) == "" {
				continue
			}
			return true
		case *ast.CodeBlock, *ast.FencedCodeBlock:
			if hasNonBlankLines(node, source) {
				return true
			}
		default:
			if nodeHasText(node, source) {
				return true
			}
		}
	}
	return false
}

func nodeHasText(node ast.Node, source []byte) bool {
	if strings.TrimSpace(mdtext.ExtractPlainText(node, source)) != "" {
		return true
	}
	return strings.TrimSpace(nodeLinesText(node, source)) != ""
}

func hasNonBlankLines(node ast.Node, source []byte) bool {
	lines := node.Lines()
	for i := 0; i < lines.Len(); i++ {
		seg := lines.At(i)
		if strings.TrimSpace(string(seg.Value(source))) != "" {
			return true
		}
	}
	return false
}

func nodeLinesText(node ast.Node, source []byte) string {
	lines := node.Lines()
	if lines.Len() == 0 {
		return ""
	}
	var b strings.Builder
	for i := 0; i < lines.Len(); i++ {
		seg := lines.At(i)
		b.Write(seg.Value(source))
	}
	return b.String()
}

func stripHTMLComments(s string) string {
	return htmlCommentPattern.ReplaceAllString(s, "")
}

func headingLabel(heading *ast.Heading, source []byte) string {
	text := strings.TrimSpace(mdtext.ExtractPlainText(heading, source))
	if text == "" {
		text = "(empty heading)"
	}
	return strings.TrimSpace(strings.Repeat("#", heading.Level) + " " + text)
}

func headingLine(heading *ast.Heading, f *lint.File) int {
	lines := heading.Lines()
	if lines.Len() > 0 {
		return f.LineOfOffset(lines.At(0).Start)
	}
	for c := heading.FirstChild(); c != nil; c = c.NextSibling() {
		if text, ok := c.(*ast.Text); ok {
			return f.LineOfOffset(text.Segment.Start)
		}
	}
	return 1
}

func toInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int64:
		return int(n), true
	case float64:
		if n != float64(int(n)) {
			return 0, false
		}
		return int(n), true
	default:
		return 0, false
	}
}

var _ rule.Configurable = (*Rule)(nil)
