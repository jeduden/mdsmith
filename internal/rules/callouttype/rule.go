// Package callouttype implements MDS067, which validates the
// `[!type]` token at the start of an Obsidian callout blockquote
// against the convention's allowed type set.
package callouttype

import (
	"bytes"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/jeduden/mdsmith/pkg/goldmark/ast"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/jeduden/mdsmith/internal/rules/settings"
)

func init() {
	rule.Register(&Rule{})
}

// Rule flags blockquote callouts whose `[!type]` token is not in the
// effective allow set. The set is the Obsidian-flavor base types plus
// any user-configured `allow:` entries; `allow-unknown: true` turns
// validation off entirely.
type Rule struct {
	Allow        []string
	AllowUnknown bool
}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "MDS067" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "callout-type" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "structural" }

// EnabledByDefault implements rule.Defaultable. Disabled by default
// so non-Obsidian projects do not see diagnostics for blockquote
// lines that happen to start with `[!something]`.
func (r *Rule) EnabledByDefault() bool { return false }

// builtInTypes lists every base Obsidian callout type and its
// aliases. Lowercased; lookup uses strings.ToLower on the captured
// token. Keep this map in sync with Obsidian's published
// vocabulary; the diagnostic message orders names via
// validTypeOrder below, so map iteration order does not affect
// output stability.
var builtInTypes = map[string]bool{
	"note":      true,
	"abstract":  true,
	"summary":   true,
	"tldr":      true,
	"info":      true,
	"todo":      true,
	"tip":       true,
	"hint":      true,
	"important": true,
	"success":   true,
	"check":     true,
	"done":      true,
	"question":  true,
	"help":      true,
	"faq":       true,
	"warning":   true,
	"caution":   true,
	"attention": true,
	"failure":   true,
	"fail":      true,
	"missing":   true,
	"danger":    true,
	"error":     true,
	"bug":       true,
	"example":   true,
	"quote":     true,
	"cite":      true,
}

// validTypeOrder is the message-facing list of base type names. The
// order matches the convention's documented vocabulary so users
// quoting the diagnostic see the official names first; aliases stay
// out of the message to keep it readable.
var validTypeOrder = []string{
	"note", "abstract", "info", "tip", "success", "question",
	"warning", "failure", "danger", "bug", "example", "quote",
}

// calloutRE matches the `[!type]` token at the start of a callout
// line. The capture group is the type body (letters, digits, dash,
// underscore).
var calloutRE = regexp.MustCompile(`^\[!([A-Za-z0-9_-]+)\]`)

// Check implements rule.Rule. The callout token lives on a blockquote's
// first content line — read from the AST's first paragraph on a parsed
// File and from the Layer 0 BlockQuote span's first line on a parse-
// skipped File (f.AST nil). Both resolve the same token, line, and
// column, so the diagnostics are identical.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	if r.AllowUnknown {
		return nil
	}
	if f == nil {
		return nil
	}
	if f.AST == nil {
		return r.checkLayer0(f)
	}
	allowed := r.cachedAllowSet(f)
	var diags []lint.Diagnostic
	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		bq, ok := n.(*ast.Blockquote)
		if !ok {
			return ast.WalkContinue, nil
		}
		token, line, col, ok := calloutToken(bq, f)
		if !ok {
			return ast.WalkContinue, nil
		}
		if allowed[strings.ToLower(token)] {
			return ast.WalkContinue, nil
		}
		diags = append(diags, r.unknownTypeDiag(f.Path, line, col, token))
		return ast.WalkContinue, nil
	})
	return diags
}

// checkLayer0 is the nil-AST counterpart of the AST walk. It scans
// source lines directly, tracking blockquote nesting depth. A line
// whose depth d exceeds the previous non-blank line's depth marks the
// opening line of a nested blockquote at depth d; calloutRE is applied
// to its content, matching what the AST path extracts from each
// blockquote node's first paragraph. Code-block lines are skipped.
//
// This line-by-line approach is necessary because Layer0 emits only one
// BlockQuote span per outermost blockquote run; inner nested spans live
// only in the throwaway recursive scan and are never surfaced.
func (r *Rule) checkLayer0(f *lint.File) []lint.Diagnostic {
	allowed := r.cachedAllowSet(f)
	codeLines := lint.CollectCodeBlockLines(f)
	var diags []lint.Diagnostic
	prevDepth := 0
	for lineNum := 1; lineNum <= len(f.Lines); lineNum++ {
		if _, inCode := codeLines[lineNum]; inCode {
			prevDepth = 0
			continue
		}
		d := lineQuoteDepth(f.Lines[lineNum-1])
		if d == 0 {
			prevDepth = 0
			continue
		}
		if d <= prevDepth {
			prevDepth = d
			continue
		}
		// d > prevDepth: first content line at this nesting depth.
		token, line, col, ok := calloutTokenFromLine(f, lineNum, d)
		if !ok || allowed[strings.ToLower(token)] {
			prevDepth = d
			continue
		}
		diags = append(diags, r.unknownTypeDiag(f.Path, line, col, token))
		prevDepth = d
	}
	return diags
}

// lineQuoteDepth counts the number of consecutive blockquote marker
// levels (stripped one at a time by quoteMarkerLen) on a raw line.
func lineQuoteDepth(line []byte) int {
	d, offset := 0, 0
	for {
		n := quoteMarkerLen(line[offset:])
		if n == 0 {
			return d
		}
		d++
		offset += n
	}
}

// calloutTokenFromLine reads the `[!type]` token from the 1-based source
// line lineNum, stripping depth blockquote marker levels to reach the
// paragraph content goldmark feeds calloutRE on the AST path. The column
// is the 1-based source column of the matched `[`, matching the AST path's
// ColumnOfOffset(seg.Start + m[0]).
func calloutTokenFromLine(f *lint.File, lineNum int, depth int) (token string, line, col int, ok bool) {
	idx := lineNum - 1
	if idx < 0 || idx >= len(f.Lines) {
		return "", 0, 0, false
	}
	raw := f.Lines[idx]
	contentStart := 0
	for i := 0; i < depth; i++ {
		n := quoteMarkerLen(raw[contentStart:])
		if n == 0 {
			return "", 0, 0, false
		}
		contentStart += n
	}
	content := bytes.TrimRight(raw[contentStart:], "\r\n")
	m := calloutRE.FindSubmatchIndex(content)
	if m == nil {
		return "", 0, 0, false
	}
	return string(content[m[2]:m[3]]), lineNum, contentStart + m[0] + 1, true
}

// quoteMarkerLen returns the byte length of the leading blockquote marker
// on line: up to 3 spaces of indent, one `>`, and one optional following
// space. It mirrors goldmark's blockquote-marker consumption so the
// paragraph content begins at the returned offset. A line with no `>`
// returns 0.
func quoteMarkerLen(line []byte) int {
	i := 0
	for i < len(line) && i < 3 && line[i] == ' ' {
		i++
	}
	if i >= len(line) || line[i] != '>' {
		return 0
	}
	i++
	if i < len(line) && line[i] == ' ' {
		i++
	}
	return i
}

// buildAllowSet constructs the union of the built-in Obsidian types and the
// user-configured Allow entries. It is called from cachedAllowSet via f.Memo
// so it runs at most once per file. Rule instances are shared across concurrent
// LSP calls, so mutable caching on the Rule struct itself would race; the
// per-File memo is sync.Map + sync.Once protected and avoids that entirely.
func (r *Rule) buildAllowSet() map[string]bool {
	out := make(map[string]bool, len(builtInTypes)+len(r.Allow))
	for k := range builtInTypes {
		out[k] = true
	}
	for _, name := range r.Allow {
		out[strings.ToLower(name)] = true
	}
	return out
}

// cachedAllowSet returns the effective allow set memoised on the per-Check
// *lint.File, so the map is built at most once per file regardless of how
// many callout blockquotes the file contains.
func (r *Rule) cachedAllowSet(f *lint.File) map[string]bool {
	v := f.Memo("MDS067.allowSet", func() any { return r.buildAllowSet() })
	return v.(map[string]bool)
}

// calloutToken returns the `[!type]` token, body-relative line, and
// column of the bq blockquote when its first paragraph begins with
// the Obsidian callout marker. ok=false means the blockquote is not
// a callout.
func calloutToken(bq *ast.Blockquote, f *lint.File) (token string, line, col int, ok bool) {
	para, ok := bq.FirstChild().(*ast.Paragraph)
	if !ok || para.Lines().Len() == 0 {
		return "", 0, 0, false
	}
	seg := para.Lines().At(0)
	firstLine := bytes.TrimRight(f.Source[seg.Start:seg.Stop], "\r\n")
	m := calloutRE.FindSubmatchIndex(firstLine)
	if m == nil {
		return "", 0, 0, false
	}
	startOffset := seg.Start + m[0]
	return string(firstLine[m[2]:m[3]]), f.LineOfOffset(startOffset), f.ColumnOfOffset(startOffset), true
}

func (r *Rule) unknownTypeDiag(path string, line, col int, token string) lint.Diagnostic {
	valid := strings.Join(validTypeOrder, ", ")
	extra := ""
	if len(r.Allow) > 0 {
		extras := make([]string, len(r.Allow))
		copy(extras, r.Allow)
		sort.Strings(extras)
		extra = " (plus " + strings.Join(extras, ", ") + ")"
	}
	msg := fmt.Sprintf(
		"unknown callout type %q; valid base types: %s%s "+
			"(aliases such as summary, tldr, todo also resolve; "+
			"or configure allow-unknown: true)",
		token, valid, extra,
	)
	return lint.Diagnostic{
		File:     path,
		Line:     line,
		Column:   col,
		RuleID:   r.ID(),
		RuleName: r.Name(),
		Severity: lint.Warning,
		Message:  msg,
	}
}

// ApplySettings implements rule.Configurable.
func (r *Rule) ApplySettings(s map[string]any) error {
	for k, v := range s {
		switch k {
		case "allow":
			list, ok := settings.ToStringSlice(v)
			if !ok {
				return fmt.Errorf("callout-type: allow must be a list of strings, got %T", v)
			}
			r.Allow = list
		case "allow-unknown":
			b, ok := v.(bool)
			if !ok {
				return fmt.Errorf("callout-type: allow-unknown must be a bool, got %T", v)
			}
			r.AllowUnknown = b
		default:
			return fmt.Errorf("callout-type: unknown setting %q", k)
		}
	}
	return nil
}

// DefaultSettings implements rule.Configurable.
func (r *Rule) DefaultSettings() map[string]any {
	return map[string]any{
		"allow":         []string{},
		"allow-unknown": false,
	}
}

// SettingMergeMode implements rule.ListMerger. `allow` appends across
// layers so a user override can extend (not replace) a convention's
// list of custom callout names.
func (r *Rule) SettingMergeMode(key string) rule.MergeMode {
	if key == "allow" {
		return rule.MergeAppend
	}
	return rule.MergeReplace
}

var (
	_ rule.Configurable = (*Rule)(nil)
	_ rule.ListMerger   = (*Rule)(nil)
	_ rule.Defaultable  = (*Rule)(nil)
)
