// Package markdownflavor implements MDS034, which validates Markdown
// against a declared target flavor (commonmark, gfm, goldmark,
// pandoc, phpextra, multimarkdown, myst, or any) and flags syntax
// the target renderer will not understand. The Flavor identity, the
// Feature support model, the dual-parser configuration, and the
// public detection entry point all live in pkg/markdown/flavor; this
// package is the rule adapter that maps config and convention into a
// flavor.Detect call and then maps flavor.Finding into engine
// diagnostics and fix bytes.
package markdownflavor

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"github.com/yuin/goldmark/ast"

	"github.com/jeduden/mdsmith/internal/convention"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/jeduden/mdsmith/pkg/markdown"
	"github.com/jeduden/mdsmith/pkg/markdown/flavor"
)

func init() {
	rule.Register(&Rule{})
}

// Rule implements MDS034, validating Markdown against a declared
// target flavor and flagging syntax the renderer does not interpret
// as a feature. The rule reads only the flavor; project-level
// convention selection (which can preset this rule's flavor and
// other rules' settings) is handled at config load — see
// internal/config/convention.go.
type Rule struct {
	Flavor convention.Flavor
}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "MDS034" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "markdown-flavor" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "structural" }

// EnabledByDefault implements rule.Defaultable. MDS034 is opt-in.
func (r *Rule) EnabledByDefault() bool { return false }

// ApplySettings implements rule.Configurable. Keys are processed in
// sorted order so the error reported for multiple unknown settings is
// deterministic across runs (Go's map iteration order is randomised,
// which would otherwise produce flaky fixture goldens).
func (r *Rule) ApplySettings(settings map[string]any) error {
	keys := make([]string, 0, len(settings))
	for k := range settings {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := settings[k]
		switch k {
		case "flavor":
			s, ok := v.(string)
			if !ok {
				return fmt.Errorf("markdown-flavor: flavor must be a string, got %T", v)
			}
			if s == "" {
				r.Flavor = convention.Flavor(0)
				continue
			}
			fl, ok := convention.ParseFlavor(s)
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

// Check implements rule.Rule. It runs flavor.Detect with an accept
// predicate that admits only features the configured flavor rejects,
// then maps each resulting Finding into one engine diagnostic.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	if !r.Flavor.IsValid() {
		return nil
	}
	unsupported := func(feat flavor.Feature) bool {
		return !flavor.Supports(r.Flavor, feat)
	}
	doc := &markdown.Document{Body: f.Source, AST: f.AST}
	findings := flavor.Detect(doc, unsupported)
	if len(findings) == 0 {
		return nil
	}
	diags := make([]lint.Diagnostic, 0, len(findings))
	for _, found := range findings {
		diags = append(diags, lint.Diagnostic{
			File:     f.Path,
			Line:     found.Line,
			Column:   found.Column,
			RuleID:   r.ID(),
			RuleName: r.Name(),
			Severity: lint.Warning,
			Message: fmt.Sprintf("%s does not interpret %s as a feature",
				r.Flavor, found.Feature.Name()),
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
	if !r.Flavor.IsValid() {
		return f.Source
	}
	current := f
	if !flavor.Supports(r.Flavor, flavor.FeatureGitHubAlerts) {
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
		if !flavor.IsGitHubAlert(bq, f.Source) {
			return ast.WalkContinue, nil
		}
		// flavor.IsGitHubAlert is the only authority on whether the
		// (Paragraph, non-empty Lines) invariants hold; if it returns
		// true the assertion + At(0) below cannot panic. A defensive
		// local re-check would only mask a future contract break by
		// silently skipping the fix while Check still flagged the
		// alert — worse than a clear panic. The contract is locked by
		// the rule's existing fix tests.
		para := bq.FirstChild().(*ast.Paragraph)
		lines := para.Lines()
		seg := lines.At(0)
		markerLine, _ := flavor.LineCol(f.Source, seg.Start)
		skip[markerLine] = true

		// Remaining lines of the first paragraph may use lazy continuation
		// (no "> " prefix in the raw source). After removing the marker they
		// would no longer be inside a blockquote, so re-add the prefix.
		for i := 1; i < lines.Len(); i++ {
			contSeg := lines.At(i)
			contLine, _ := flavor.LineCol(f.Source, contSeg.Start)
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

// FixTitle implements rule.QuickFixTitler.
func (r *Rule) FixTitle() string { return "Replace syntax the flavor can't render" }
