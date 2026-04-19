// Package tocdirective implements MDS035, which flags renderer-specific
// table-of-contents directives that render as literal text on CommonMark
// and goldmark.
package tocdirective

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

func init() {
	rule.Register(&Rule{})
}

// tocVariant pairs a line-level detection regex with the exact directive
// token echoed back in diagnostics.
type tocVariant struct {
	pattern *regexp.Regexp
	token   string
	// isLinkRefCandidate marks `[TOC]`, which is syntactically a valid
	// CommonMark shortcut reference link and must be suppressed when a
	// matching link reference definition exists.
	isLinkRefCandidate bool
}

// variants lists the four renderer-specific TOC directives detected by the
// rule. The regex anchors ensure each directive occupies the entire line
// (trailing whitespace allowed); anything else on the line rules it out.
var variants = []tocVariant{
	{pattern: regexp.MustCompile(`^\[TOC\][ \t]*$`), token: "[TOC]", isLinkRefCandidate: true},
	{pattern: regexp.MustCompile(`^\[\[_TOC_\]\][ \t]*$`), token: "[[_TOC_]]"},
	{pattern: regexp.MustCompile(`^\[\[toc\]\][ \t]*$`), token: "[[toc]]"},
	{pattern: regexp.MustCompile(`^\$\{toc\}[ \t]*$`), token: "${toc}"},
}

// Rule detects renderer-specific TOC directives.
type Rule struct{}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "MDS035" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "toc-directive" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "meta" }

// EnabledByDefault implements rule.Defaultable.
func (r *Rule) EnabledByDefault() bool { return false }

// Check implements rule.Rule.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	if f == nil || f.AST == nil {
		return nil
	}

	hasTOCRef := hasTOCLinkReference(f.Source)

	var diags []lint.Diagnostic
	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		para, ok := n.(*ast.Paragraph)
		if !ok {
			return ast.WalkContinue, nil
		}
		lines := para.Lines()
		for i := 0; i < lines.Len(); i++ {
			seg := lines.At(i)
			lineText := strings.TrimRight(
				string(seg.Value(f.Source)), "\r\n",
			)
			v, matched := matchVariant(lineText)
			if !matched {
				continue
			}
			if v.isLinkRefCandidate && hasTOCRef {
				continue
			}
			diags = append(diags, lint.Diagnostic{
				File:     f.Path,
				Line:     f.LineOfOffset(seg.Start),
				Column:   1,
				RuleID:   r.ID(),
				RuleName: r.Name(),
				Severity: lint.Warning,
				Message:  buildMessage(v.token),
			})
		}
		return ast.WalkContinue, nil
	})
	return diags
}

func matchVariant(line string) (tocVariant, bool) {
	for _, v := range variants {
		if v.pattern.MatchString(line) {
			return v, true
		}
	}
	return tocVariant{}, false
}

func buildMessage(token string) string {
	return fmt.Sprintf(
		"unsupported TOC directive `%s`; "+
			"mdsmith has no heading TOC equivalent; "+
			"use `<?catalog?>` for file indexes (MDS019)",
		token,
	)
}

// hasTOCLinkReference returns true when the document defines a link
// reference with label "TOC" (CommonMark-normalized). It re-parses with
// lint.NewParser so the parser configuration (including mdsmith's PI
// block parser) matches the original lint parse; otherwise content
// absorbed into a processing-instruction block could register as a link
// reference here while being hidden from the rule's AST walk.
func hasTOCLinkReference(source []byte) bool {
	if len(source) == 0 {
		return false
	}
	ctx := parser.NewContext()
	lint.NewParser().Parse(text.NewReader(source), parser.WithContext(ctx))
	_, ok := ctx.Reference("toc")
	return ok
}

var _ rule.Defaultable = (*Rule)(nil)
