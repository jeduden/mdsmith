package markdownflavor

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/yuin/goldmark/ast"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
)

func init() {
	rule.Register(&Rule{})
}

// Rule implements MDS034, validating Markdown against a declared
// target flavor and flagging syntax the renderer will reject.
type Rule struct {
	Flavor Flavor
}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "MDS034" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "markdown-flavor" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "meta" }

// EnabledByDefault implements rule.Defaultable. MDS034 is opt-in.
func (r *Rule) EnabledByDefault() bool { return false }

// ApplySettings implements rule.Configurable.
func (r *Rule) ApplySettings(settings map[string]any) error {
	for k, v := range settings {
		switch k {
		case "flavor":
			s, ok := v.(string)
			if !ok {
				return fmt.Errorf("markdown-flavor: flavor must be a string, got %T", v)
			}
			if s == "" {
				r.Flavor = flavorInvalid
				continue
			}
			fl, ok := ParseFlavor(s)
			if !ok {
				return fmt.Errorf(
					"markdown-flavor: unknown flavor %q (expected one of: "+
						"any, commonmark, gfm, goldmark, multimarkdown, myst, pandoc, phpextra)",
					s,
				)
			}
			r.Flavor = fl
		default:
			return fmt.Errorf("markdown-flavor: unknown setting %q", k)
		}
	}
	return nil
}

// DefaultSettings implements rule.Configurable.
func (r *Rule) DefaultSettings() map[string]any {
	return map[string]any{
		"flavor": "",
	}
}

// Check implements rule.Rule.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	if r.Flavor == flavorInvalid {
		return nil
	}
	// Only ask detectors about features this flavor rejects. Detectors
	// like the bare-URL regex scan then skip large files entirely when
	// the flavor (gfm, goldmark) accepts them.
	unsupported := func(feat Feature) bool {
		return !r.Flavor.Supports(feat)
	}
	var diags []lint.Diagnostic
	for _, found := range DetectFiltered(f, unsupported) {
		diags = append(diags, lint.Diagnostic{
			File:     f.Path,
			Line:     found.Line,
			Column:   found.Column,
			RuleID:   r.ID(),
			RuleName: r.Name(),
			Severity: lint.Warning,
			Message: fmt.Sprintf("%s %s not supported by %s",
				found.Feature.Name(), found.Feature.Verb(), r.Flavor),
		})
	}
	return diags
}

// Fix implements rule.FixableRule. It first removes the [!TOKEN]
// marker line from GitHub Alert blockquotes (line-level edit, with
// lazy-continuation handling), then runs the byte-range fix pipeline
// over the result for heading IDs, strikethrough, task lists,
// superscript, subscript, and bare-URL autolinks. Each feature is
// fixed only when the configured flavor does not support it. When
// alerts are stripped the byte-range pass re-parses the rewritten
// source so AST offsets match the new bytes.
func (r *Rule) Fix(f *lint.File) []byte {
	if r.Flavor == flavorInvalid {
		return f.Source
	}
	current := f
	if !r.Flavor.Supports(FeatureGitHubAlerts) {
		stripped := r.fixGitHubAlerts(f)
		if !bytes.Equal(stripped, f.Source) {
			reparsed, err := lint.NewFile(f.Path, stripped)
			if err != nil {
				return stripped
			}
			current = reparsed
		}
	}
	return r.fixByteRangeFeatures(current)
}

// fixGitHubAlerts strips [!TOKEN] alert markers from blockquotes,
// re-adding "> " on lazy-continuation lines so the blockquote stays
// well-formed after the marker line goes away. If the marker is the
// only line in the blockquote, the whole blockquote is removed.
func (r *Rule) fixGitHubAlerts(f *lint.File) []byte {
	skip := map[int]bool{}
	addPrefix := map[int]bool{} // lazy-continuation lines that lose blockquote context
	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		bq, ok := n.(*ast.Blockquote)
		if !ok {
			return ast.WalkContinue, nil
		}
		if !isGitHubAlert(bq, f.Source) {
			return ast.WalkContinue, nil
		}
		para := bq.FirstChild().(*ast.Paragraph)
		lines := para.Lines()
		seg := lines.At(0)
		markerLine, _ := lineCol(f.Source, seg.Start)
		skip[markerLine] = true

		// Remaining lines of the first paragraph may use lazy continuation
		// (no "> " prefix in the raw source). After removing the marker they
		// would no longer be inside a blockquote, so re-add the prefix.
		for i := 1; i < lines.Len(); i++ {
			contSeg := lines.At(i)
			contLine, _ := lineCol(f.Source, contSeg.Start)
			raw := strings.TrimLeft(string(f.Lines[contLine-1]), " \t")
			if !strings.HasPrefix(raw, ">") {
				addPrefix[contLine] = true
			}
		}
		return ast.WalkContinue, nil
	})

	if len(skip) == 0 {
		return f.Source
	}

	var out []string
	for i, line := range f.Lines {
		lineNum := i + 1
		if skip[lineNum] {
			continue
		}
		s := string(line)
		if addPrefix[lineNum] {
			trimmed := strings.TrimLeft(s, " \t")
			s = s[:len(s)-len(trimmed)] + "> " + trimmed
		}
		out = append(out, s)
	}
	return []byte(strings.Join(out, "\n"))
}

var (
	_ rule.Configurable = (*Rule)(nil)
	_ rule.Defaultable  = (*Rule)(nil)
	_ rule.FixableRule  = (*Rule)(nil)
)
