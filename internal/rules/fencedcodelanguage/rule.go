package fencedcodelanguage

import (
	"bytes"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/jeduden/mdsmith/internal/rules/fencepos"
	"github.com/jeduden/mdsmith/pkg/goldmark/ast"
)

func init() {
	rule.Register(&Rule{})
}

// Rule checks that fenced code blocks have a language tag.
type Rule struct{}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "MDS011" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "fenced-code-language" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "code" }

// Check implements rule.Rule. The per-block verdict depends only on the
// info string on a fenced block's opening line, never the inline tree,
// so the rule is a rule.BlockChecker: on a parsed File it folds into the
// engine's shared AST walk (rule.WalkNodes); on a parse-skipped File
// (f.AST nil) it reads the Layer 0 block scan (rule.WalkBlocks).
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	if f != nil && f.AST == nil {
		return rule.WalkBlocks(r, f)
	}
	return rule.WalkNodes(r, f)
}

// CheckNode implements rule.NodeChecker.
func (r *Rule) CheckNode(n ast.Node, entering bool, f *lint.File) []lint.Diagnostic {
	if !entering {
		return nil
	}
	fcb, ok := n.(*ast.FencedCodeBlock)
	if !ok {
		return nil
	}

	hasLanguage := false
	if fcb.Info != nil {
		info := fcb.Info.Segment
		if info.Stop > info.Start {
			lang := f.Source[info.Start:info.Stop]
			if len(lang) > 0 {
				hasLanguage = true
			}
		}
	}
	return r.verdict(f, hasLanguage, fencepos.OpenLine(f, fcb))
}

// CheckBlock implements rule.BlockChecker. A BlockFencedCode span's Start
// is the opening fence line, and the info string is the text after the
// fence run on that line — goldmark's info segment is that same text
// trimmed — so the language-presence verdict is byte-identical.
func (r *Rule) CheckBlock(span lint.BlockSpan, f *lint.File) []lint.Diagnostic {
	return r.verdict(f, fenceLineHasInfo(f.Lines[span.Start-1]), span.Start)
}

// blockKinds is the static block-kind interest CheckBlock declares via
// rule.BlockChecker; package-level so BlockKinds returns it without
// allocating.
var blockKinds = []lint.BlockKind{lint.BlockFencedCode}

// BlockKinds implements rule.BlockChecker.
func (r *Rule) BlockKinds() []lint.BlockKind { return blockKinds }

var (
	_ rule.NodeChecker  = (*Rule)(nil)
	_ rule.BlockChecker = (*Rule)(nil)
)

// verdict is the shared per-block check both paths drive: hasLanguage is
// whether the opening fence carries an info string, line its 1-based
// opening line.
func (r *Rule) verdict(f *lint.File, hasLanguage bool, line int) []lint.Diagnostic {
	if hasLanguage {
		return nil
	}
	return []lint.Diagnostic{{
		File:     f.Path,
		Line:     line,
		Column:   1,
		RuleID:   r.ID(),
		RuleName: r.Name(),
		Severity: lint.Warning,
		Message:  "fenced code block should have a language tag",
	}}
}

// fenceLineHasInfo reports whether a BlockFencedCode opening line carries
// a non-empty info string: any non-whitespace after the leading spaces
// and the backtick/tilde fence run. This mirrors goldmark's info segment,
// which is that trailing text trimmed of surrounding whitespace.
func fenceLineHasInfo(line []byte) bool {
	i := 0
	for i < len(line) && line[i] == ' ' {
		i++
	}
	if i >= len(line) {
		return false
	}
	fc := line[i]
	if fc != '`' && fc != '~' {
		return false
	}
	for i < len(line) && line[i] == fc {
		i++
	}
	return len(bytes.TrimSpace(line[i:])) > 0
}

// enteringKinds is the static node-kind interest CheckNode declares
// via rule.KindScopedChecker; package-level so EnteringKinds returns
// it without allocating.
var enteringKinds = []ast.NodeKind{ast.KindFencedCodeBlock}

// EnteringKinds implements rule.KindScopedChecker: CheckNode only
// reacts to these node kinds, entering visits only.
func (r *Rule) EnteringKinds() []ast.NodeKind { return enteringKinds }

var _ rule.KindScopedChecker = (*Rule)(nil)
