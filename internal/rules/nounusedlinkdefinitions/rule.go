// Package nounusedlinkdefinitions implements MDS053, which flags link
// reference definitions that are never used by any reference-style link or
// image, and definitions that duplicate an existing label.
package nounusedlinkdefinitions

import (
	"bytes"
	"fmt"
	"regexp"
	"sort"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/util"
)

func init() {
	rule.Register(&Rule{})
}

// Rule flags unused and duplicate link reference definitions.
type Rule struct {
	// ignoredLabels is the CommonMark-normalized set of labels that are
	// never flagged as unused or duplicate, regardless of whether they are consumed.
	ignoredLabels map[string]bool
}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "MDS053" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "no-unused-link-definitions" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "link" }

// EnabledByDefault implements rule.Defaultable.
func (r *Rule) EnabledByDefault() bool { return true }

// ApplySettings implements rule.Configurable.
func (r *Rule) ApplySettings(settings map[string]any) error {
	r.ignoredLabels = map[string]bool{}
	for k, v := range settings {
		switch k {
		case "ignored-labels":
			list, ok := toStringSlice(v)
			if !ok {
				return fmt.Errorf(
					"no-unused-link-definitions: ignored-labels must be a list of strings, got %T",
					v,
				)
			}
			for _, s := range list {
				r.ignoredLabels[normalizeLabel(s)] = true
			}
		default:
			return fmt.Errorf("no-unused-link-definitions: unknown setting %q", k)
		}
	}
	return nil
}

// DefaultSettings implements rule.Configurable.
func (r *Rule) DefaultSettings() map[string]any {
	return map[string]any{
		"ignored-labels": []string{},
	}
}

// SettingMergeMode implements rule.ListMerger.
// ignored-labels uses replace mode: a later config layer's list replaces the
// earlier layer's list wholesale (not appended). Unknown keys fall through to
// the default MergeReplace per the rule.ListMerger contract.
func (r *Rule) SettingMergeMode(key string) rule.MergeMode {
	switch key {
	case "ignored-labels":
		return rule.MergeReplace
	default:
		return rule.MergeReplace
	}
}

const (
	msgUnused    = "unused link reference definition %q"
	msgDuplicate = "duplicate link reference definition %q; first defined on line %d"
)

// Check implements rule.Rule.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	defs := collectDefinitions(f)
	if len(defs) == 0 {
		return nil
	}
	usedLabels := collectUsedLabels(f)
	ignored := r.ignoredLabels

	seen := map[string]int{} // normalized label → first definition line
	var diags []lint.Diagnostic
	for _, d := range defs {
		norm := normalizeLabel(d.label)
		if ignored[norm] {
			continue
		}
		if firstLine, exists := seen[norm]; exists {
			diags = append(diags, lint.Diagnostic{
				File:     f.Path,
				Line:     d.line,
				Column:   d.col,
				RuleID:   r.ID(),
				RuleName: r.Name(),
				Severity: lint.Warning,
				Message:  fmt.Sprintf(msgDuplicate, d.label, firstLine),
			})
		} else {
			seen[norm] = d.line
			if !usedLabels[norm] {
				diags = append(diags, lint.Diagnostic{
					File:     f.Path,
					Line:     d.line,
					Column:   d.col,
					RuleID:   r.ID(),
					RuleName: r.Name(),
					Severity: lint.Warning,
					Message:  fmt.Sprintf(msgUnused, d.label),
				})
			}
		}
	}
	return diags
}

// Fix implements rule.FixableRule. It removes unused and duplicate definition
// lines, collapsing any blank line left behind so the file's blank-line policy
// is preserved.
func (r *Rule) Fix(f *lint.File) []byte {
	defs := collectDefinitions(f)
	if len(defs) == 0 {
		out := make([]byte, len(f.Source))
		copy(out, f.Source)
		return out
	}
	usedLabels := collectUsedLabels(f)
	ignored := r.ignoredLabels
	source := f.Source

	seen := map[string]bool{}
	var cuts []fixCut
	for _, d := range defs {
		norm := normalizeLabel(d.label)
		if ignored[norm] {
			continue
		}
		isDuplicate := seen[norm]
		seen[norm] = true
		if !isDuplicate && usedLabels[norm] {
			continue
		}
		start := d.start
		// Consume the blank line before the definition only when a blank line
		// also follows (or the definition ends the file). This preserves the
		// paragraph separator when the definition sat between two block elements
		// with only a single blank line on each side.
		if start >= 2 && source[start-1] == '\n' && source[start-2] == '\n' {
			afterDefIsBlankOrEOF := d.end >= len(source) || source[d.end] == '\n'
			if afterDefIsBlankOrEOF {
				start--
			}
		}
		cuts = append(cuts, fixCut{start: start, end: d.end})
	}
	if len(cuts) == 0 {
		out := make([]byte, len(source))
		copy(out, source)
		return out
	}
	result := applyCuts(source, cuts)
	// If the original file ended with exactly one newline and the result ends
	// with two, a run of consecutive definitions at EOF was removed without
	// consuming the preceding blank line.  Trim the extra newline so the file
	// still ends with exactly one newline (MDS009 territory, but do it here to
	// keep Fix() idempotent).
	if bytes.HasSuffix(source, []byte{'\n'}) &&
		!bytes.HasSuffix(source, []byte{'\n', '\n'}) &&
		bytes.HasSuffix(result, []byte{'\n', '\n'}) {
		result = result[:len(result)-1]
	}
	return result
}

// referenceDefinition records a single `[label]: url` line in source.
type referenceDefinition struct {
	label string
	line  int
	col   int
	start int
	end   int
}

// refDefRE matches a CommonMark reference definition at the start of a line:
// optional 0-3 spaces, [label]: dest (with optional title). Used only for
// locating definitions after goldmark confirmed they exist; a permissive
// regex is safe.
var refDefRE = regexp.MustCompile(`(?m)^[ ]{0,3}\[([^\]\n]+)\]:[ \t]*\S+.*$`)

// collectDefinitions returns all link reference definitions in the file,
// including duplicates, in document order. Lines inside code blocks, PI blocks,
// or generated sections (f.GeneratedRanges) are excluded so that Fix() never
// deletes content that belongs to those regions. The PI-block filter is
// load-bearing: when a label is defined both inside a PI block and outside, the
// wanted-label check passes for both matches, so an explicit line-range
// exclusion is required to avoid treating the PI-block occurrence as a
// duplicate definition.
func collectDefinitions(f *lint.File) []referenceDefinition {
	source := f.Source

	// wanted holds normalized labels that goldmark found as valid definitions
	// (first-wins, non-code-block, non-PI-block), read from the file's
	// single parse rather than a second re-parse.
	wanted := map[string]bool{}
	for _, ref := range f.LinkReferences() {
		wanted[string(ref.Label())] = true
	}
	if len(wanted) == 0 {
		return nil
	}

	codeLines := lint.CollectCodeBlockLines(f)
	piLines := lint.CollectPIBlockLines(f)
	var out []referenceDefinition
	for _, m := range refDefRE.FindAllSubmatchIndex(source, -1) {
		raw := source[m[2]:m[3]]
		if !wanted[util.ToLinkReference(raw)] {
			continue
		}
		bracketAbs := m[2] - 1
		matchLine := f.LineOfOffset(bracketAbs)
		if codeLines[matchLine] || piLines[matchLine] {
			continue
		}
		if lineInGeneratedRanges(matchLine, f.GeneratedRanges) {
			continue
		}
		end := m[1]
		if end < len(source) && source[end] == '\n' {
			end++
		}
		out = append(out, referenceDefinition{
			label: string(raw),
			line:  matchLine,
			col:   f.ColumnOfOffset(bracketAbs),
			start: m[0],
			end:   end,
		})
	}
	return out
}

// collectUsedLabels walks the AST and returns the set of normalized labels
// referenced by at least one reference-style link or image.
func collectUsedLabels(f *lint.File) map[string]bool {
	used := map[string]bool{}
	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch v := n.(type) {
		case *ast.Link:
			if v.Reference != nil {
				used[util.ToLinkReference(v.Reference.Value)] = true
			}
		case *ast.Image:
			if v.Reference != nil {
				used[util.ToLinkReference(v.Reference.Value)] = true
			}
		}
		return ast.WalkContinue, nil
	})
	return used
}

// normalizeLabel applies CommonMark label normalization (collapse whitespace,
// lowercase) via goldmark's util.ToLinkReference.
func normalizeLabel(s string) string {
	return util.ToLinkReference([]byte(s))
}

// fixCut is a byte-range deletion in source.
type fixCut struct {
	start, end int
}

func applyCuts(source []byte, cuts []fixCut) []byte {
	sort.Slice(cuts, func(i, j int) bool {
		return cuts[i].start < cuts[j].start
	})
	var out bytes.Buffer
	prev := 0
	for _, c := range cuts {
		if c.start < prev {
			continue
		}
		out.Write(source[prev:c.start])
		prev = c.end
	}
	out.Write(source[prev:])
	return out.Bytes()
}

func lineInGeneratedRanges(line int, ranges []lint.LineRange) bool {
	for _, r := range ranges {
		if r.Contains(line) {
			return true
		}
	}
	return false
}

func toStringSlice(v any) ([]string, bool) {
	switch list := v.(type) {
	case []string:
		out := make([]string, len(list))
		copy(out, list)
		return out, true
	case []any:
		out := make([]string, 0, len(list))
		for _, item := range list {
			s, ok := item.(string)
			if !ok {
				return nil, false
			}
			out = append(out, s)
		}
		return out, true
	default:
		return nil, false
	}
}
