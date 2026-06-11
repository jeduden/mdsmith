package schema

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/mdtext"
	"github.com/jeduden/mdsmith/pkg/goldmark/ast"
)

// compilePattern returns the pre-compiled Pattern regexp when available,
// falling back to regexp.Compile for CrossRef values built outside the
// parser (e.g., in tests).
func (cr CrossRef) compilePattern() (*regexp.Regexp, error) {
	if cr.compiled != nil {
		return cr.compiled, nil
	}
	return regexp.Compile(cr.Pattern)
}

// compileSkip returns the pre-compiled SkipLinesMatching regexp when
// available, nil when the field is empty, and a fresh compile otherwise.
func (cr CrossRef) compileSkip() (*regexp.Regexp, error) {
	if cr.SkipLinesMatching == "" {
		return nil, nil
	}
	if cr.compiledSkip != nil {
		return cr.compiledSkip, nil
	}
	return regexp.Compile(cr.SkipLinesMatching)
}

// ValidateCrossReferences walks the document's inline text nodes and,
// for each cross-reference pattern, checks that every match resolves
// to an existing heading slug. Unresolved references produce a
// diagnostic at the source position of the match. Lines whose raw
// text matches SkipLinesMatching are exempt.
func ValidateCrossReferences(
	f *lint.File, sch *Schema, mkDiag MakeDiag,
) []lint.Diagnostic {
	if sch == nil || len(sch.CrossReferences) == 0 {
		return nil
	}
	slugs := documentSlugSet(f)
	texts := collectTextNodes(f)
	var diags []lint.Diagnostic
	for _, cr := range sch.CrossReferences {
		re, err := cr.compilePattern()
		if err != nil {
			diags = append(diags, mkDiag(f.Path, 1,
				fmt.Sprintf(
					"cross-references: invalid pattern %q: %v",
					cr.Pattern, err)))
			continue
		}
		skipRE, err := cr.compileSkip()
		if err != nil {
			diags = append(diags, mkDiag(f.Path, 1,
				fmt.Sprintf(
					"cross-references: invalid skip-lines-matching %q: %v",
					cr.SkipLinesMatching, err)))
			continue
		}
		diags = append(diags, checkCrossRef(f, cr, re, skipRE, slugs, texts, mkDiag)...)
	}
	return diags
}

// textNode is a captured text segment with its source line.
type textNode struct {
	Text string
	Line int
}

// collectTextNodes walks the AST for all ast.Text nodes and records
// their content with the source line. The list is in document order.
// Headings are skipped because cross-references aren't expected to
// resolve against the section title that contains them.
func collectTextNodes(f *lint.File) []textNode {
	var out []textNode
	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if _, isHeading := n.(*ast.Heading); isHeading {
			return ast.WalkSkipChildren, nil
		}
		t, ok := n.(*ast.Text)
		if !ok {
			return ast.WalkContinue, nil
		}
		seg := t.Segment
		text := string(seg.Value(f.Source))
		out = append(out, textNode{Text: text, Line: f.LineOfOffset(seg.Start)})
		return ast.WalkContinue, nil
	})
	return out
}

// documentSlugSet returns the set of heading slugs present in f. The
// slugifier matches mdtext.Slugify; duplicates collapse to one entry
// because the cross-reference resolver only needs membership.
func documentSlugSet(f *lint.File) map[string]struct{} {
	out := map[string]struct{}{}
	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		h, ok := n.(*ast.Heading)
		if !ok {
			return ast.WalkContinue, nil
		}
		text := mdtext.ExtractPlainText(h, f.Source)
		slug := mdtext.Slugify(text)
		if slug != "" {
			out[slug] = struct{}{}
		}
		return ast.WalkContinue, nil
	})
	return out
}

func checkCrossRef(
	f *lint.File, cr CrossRef, re, skipRE *regexp.Regexp,
	slugs map[string]struct{}, texts []textNode, mkDiag MakeDiag,
) []lint.Diagnostic {
	var diags []lint.Diagnostic
	groupNames := re.SubexpNames()
	for _, tn := range texts {
		if skipRE != nil && lineMatches(f, tn.Line, skipRE) {
			continue
		}
		matches := re.FindAllStringSubmatch(tn.Text, -1)
		for _, m := range matches {
			target, err := fillTemplate(cr.MustMatch, m, groupNames)
			if err != nil {
				diags = append(diags, mkDiag(f.Path, tn.Line,
					fmt.Sprintf(
						"cross-references: cannot resolve template %q: %v",
						cr.MustMatch, err)))
				continue
			}
			slug := mdtext.Slugify(target)
			_, ok := slugs[slug]
			if slug == "" || !ok {
				diags = append(diags, mkDiag(f.Path, tn.Line,
					fmt.Sprintf(
						"cross-reference %q does not resolve to a heading (looked for %q)",
						m[0], target)))
			}
		}
	}
	return diags
}

// lineMatches reports whether the raw source line at line (1-based)
// matches skipRE. Out-of-range lines return false.
func lineMatches(f *lint.File, line int, skipRE *regexp.Regexp) bool {
	idx := line - 1
	if idx < 0 || idx >= len(f.Lines) {
		return false
	}
	return skipRE.Match(f.Lines[idx])
}

// fillTemplate substitutes {n} / {name} placeholders in must-match
// with captured groups. Numeric placeholders ({1}, {2}, …) refer to
// regex submatch indices; the special {n} alias maps to {1} for the
// common "first capture" case. Named placeholders look up by regex
// group name.
func fillTemplate(tmpl string, match []string, groupNames []string) (string, error) {
	var b strings.Builder
	i := 0
	for i < len(tmpl) {
		c := tmpl[i]
		if c != '{' {
			b.WriteByte(c)
			i++
			continue
		}
		end := strings.IndexByte(tmpl[i:], '}')
		if end < 0 {
			return "", fmt.Errorf("unterminated placeholder")
		}
		name := tmpl[i+1 : i+end]
		val, err := resolveCrossRefPlaceholder(name, match, groupNames)
		if err != nil {
			return "", err
		}
		b.WriteString(val)
		i += end + 1
	}
	return b.String(), nil
}

func resolveCrossRefPlaceholder(
	name string, match []string, groupNames []string,
) (string, error) {
	if name == "" {
		return "", fmt.Errorf("empty placeholder")
	}
	if name == "n" {
		if len(match) < 2 {
			return "", fmt.Errorf("{n} but pattern has no capture group")
		}
		return match[1], nil
	}
	if idx, ok := tryParseIndex(name); ok {
		if idx < 1 || idx >= len(match) {
			return "", fmt.Errorf("{%s} out of range (pattern has %d captures)",
				name, len(match)-1)
		}
		return match[idx], nil
	}
	for i, gn := range groupNames {
		if gn == name {
			return match[i], nil
		}
	}
	return "", fmt.Errorf("unknown placeholder %q", name)
}

func tryParseIndex(s string) (int, bool) {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, false
		}
		n = n*10 + int(r-'0')
	}
	if s == "" {
		return 0, false
	}
	return n, true
}
