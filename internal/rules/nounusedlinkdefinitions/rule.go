// Package nounusedlinkdefinitions implements MDS053, which flags link
// reference definitions that are never used by any reference-style link or
// image, and definitions that duplicate an existing label.
package nounusedlinkdefinitions

import (
	"bytes"
	"fmt"
	"sort"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/jeduden/mdsmith/internal/setutil"
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
	ignoredLabels map[string]struct{}
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
	r.ignoredLabels = map[string]struct{}{}
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
				r.ignoredLabels[normalizeLabel(s)] = struct{}{}
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
	if len(defs) == 1 {
		return r.checkSingleDef(f, defs[0])
	}
	return r.checkMultiDefs(f, defs)
}

// checkSingleDef is the hot path for files with exactly one link
// reference definition (the universal case). It skips the
// usedLabels map and short-circuits the AST walk via
// isLabelUsedInAST, which returns on the first matching
// reference. The map + entry + `collectUsedLabelsInto`'s recursive
// helper that the multi-def path pays each become a single
// allocation-free walk here — plan 195 task 6.
func (r *Rule) checkSingleDef(f *lint.File, d referenceDefinition) []lint.Diagnostic {
	norm := util.ToLinkReference(d.rawLabel)
	if setutil.Contains(r.ignoredLabels, norm) {
		return nil
	}
	if isLabelUsedInAST(f.AST, norm) {
		return nil
	}
	return []lint.Diagnostic{{
		File:     f.Path,
		Line:     d.line,
		Column:   d.col,
		RuleID:   r.ID(),
		RuleName: r.Name(),
		Severity: lint.Warning,
		Message:  fmt.Sprintf(msgUnused, d.labelString()),
	}}
}

// checkMultiDefs walks defs once, flagging the first duplicate of
// each label and the unused-survivors. usedLabels is computed
// lazily so a file where every def is duplicated or ignored skips
// the AST walk.
func (r *Rule) checkMultiDefs(f *lint.File, defs []referenceDefinition) []lint.Diagnostic {
	ignored := r.ignoredLabels
	seen := make(map[string]int, len(defs))
	var (
		usedLabels     map[string]struct{}
		usedLabelsDone bool
	)
	var diags []lint.Diagnostic
	for _, d := range defs {
		norm := util.ToLinkReference(d.rawLabel)
		if setutil.Contains(ignored, norm) {
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
				Message:  fmt.Sprintf(msgDuplicate, d.labelString(), firstLine),
			})
			continue
		}
		seen[norm] = d.line
		if !usedLabelsDone {
			usedLabels = collectUsedLabels(f)
			usedLabelsDone = true
		}
		if !setutil.Contains(usedLabels, norm) {
			diags = append(diags, lint.Diagnostic{
				File:     f.Path,
				Line:     d.line,
				Column:   d.col,
				RuleID:   r.ID(),
				RuleName: r.Name(),
				Severity: lint.Warning,
				Message:  fmt.Sprintf(msgUnused, d.labelString()),
			})
		}
	}
	return diags
}

// isLabelUsedInAST reports whether n or any of its descendants is a
// reference-style link or image whose normalised label matches target.
// Short-circuits on the first match so the single-def Check path
// pays one walk, not a full map build.
func isLabelUsedInAST(n ast.Node, target string) bool {
	if n == nil {
		return false
	}
	switch v := n.(type) {
	case *ast.Link:
		if v.Reference != nil &&
			util.ToLinkReference(v.Reference.Value) == target {
			return true
		}
	case *ast.Image:
		if v.Reference != nil &&
			util.ToLinkReference(v.Reference.Value) == target {
			return true
		}
	}
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		if isLabelUsedInAST(c, target) {
			return true
		}
	}
	return false
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

	seen := map[string]struct{}{}
	var cuts []fixCut
	for _, d := range defs {
		norm := util.ToLinkReference(d.rawLabel)
		if setutil.Contains(ignored, norm) {
			continue
		}
		alreadySeen := setutil.Contains(seen, norm)
		seen[norm] = struct{}{}
		if !alreadySeen && setutil.Contains(usedLabels, norm) {
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
// The label is stored as a byte slice pointing into f.Source rather
// than a copied string so collectDefinitions adds no per-def
// allocation on the hot path. Diagnostic-message formatting and
// label normalisation accept the byte slice directly; only the
// `%q` Sprintf paths actually materialise a string. Plan 195
// task 6.
type referenceDefinition struct {
	rawLabel []byte
	line     int
	col      int
	start    int
	end      int
}

// labelString returns the label as a fresh string, used only when
// emitting a diagnostic message or when normalisation routes through
// `string` for symmetry with goldmark's API. Callers on the hot
// path read `rawLabel` directly.
func (d referenceDefinition) labelString() string {
	return string(d.rawLabel)
}

// collectDefinitions returns all link reference definitions in the file,
// including duplicates, in document order. Lines inside code blocks, PI blocks,
// or generated sections (f.GeneratedRanges) are excluded so that Fix() never
// deletes content that belongs to those regions. The PI-block filter is
// load-bearing: when a label is defined both inside a PI block and outside, the
// wanted-label check passes for both matches, so an explicit line-range
// exclusion is required to avoid treating the PI-block occurrence as a
// duplicate definition.
//
// The source scan walks lines manually rather than calling
// regexp.FindAllSubmatchIndex; the regex form paid ~4 allocs per
// Check on the alloc-budget gate fixture (result slice plus a
// per-match `[]int`), which the hand-rolled byte scanner sheds.
// The scan is inlined here so no callback closure is needed — a
// visit-style helper would have allocated 3 extra closure-box words
// for the captured locals (refs, codeLines, etc.). Plan 195 task 6.
func collectDefinitions(f *lint.File) []referenceDefinition {
	source := f.Source

	refs := f.LinkReferences()
	if len(refs) == 0 {
		return nil
	}

	var (
		codeLines, piLines map[int]struct{}
		blockLinesReady    bool
		out                []referenceDefinition
	)

	lineNum := 1
	lineStart := 0
	for lineStart <= len(source) {
		eol := lineStart
		for eol < len(source) && source[eol] != '\n' {
			eol++
		}
		labelStart, labelEnd, ok := scanRefDefLine(source, lineStart, eol)
		if ok {
			rawLabel := source[labelStart:labelEnd]
			if labelInRefs(rawLabel, refs) {
				if !blockLinesReady {
					codeLines = lint.CollectCodeBlockLines(f)
					piLines = lint.CollectPIBlockLines(f)
					blockLinesReady = true
				}
				if !lint.InCodeOrPI(codeLines, piLines, lineNum) &&
					!lineInGeneratedRanges(lineNum, f.GeneratedRanges) {
					bracketAbs := labelStart - 1
					end := eol
					if end < len(source) && source[end] == '\n' {
						end++
					}
					out = append(out, referenceDefinition{
						rawLabel: rawLabel,
						line:     lineNum,
						col:      f.ColumnOfOffset(bracketAbs),
						start:    lineStart,
						end:      end,
					})
				}
			}
		}
		if eol >= len(source) {
			break
		}
		lineStart = eol + 1
		lineNum++
	}
	return out
}

// labelInRefs reports whether the normalised form of rawLabel matches
// any label in refs. The caller iterates LinkReferences (already
// normalised by goldmark); we normalise rawLabel once and compare
// each ref's Label() byte-for-byte via stringEqualsBytes — Go's
// `s == string(b)` form does not generally elide the conversion
// allocation when the left side is a variable (only the
// `m[string(b)]` and `string(b) == "literal"` forms are special-
// cased), so the open-coded compare is the alloc-free option.
// For the typical case of one or a few refs per file this is
// cheaper than building the wanted-map literal the original code
// allocated.
func labelInRefs(rawLabel []byte, refs []lint.Reference) bool {
	normalised := util.ToLinkReference(rawLabel)
	for _, ref := range refs {
		if stringEqualsBytes(normalised, ref.Label()) {
			return true
		}
	}
	return false
}

// stringEqualsBytes compares a string to a byte slice without
// allocating a string from the slice. Used on hot paths where
// `s == string(b)` would force a heap copy of b.
func stringEqualsBytes(s string, b []byte) bool {
	if len(s) != len(b) {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] != b[i] {
			return false
		}
	}
	return true
}

// scanRefDefLine examines source[lineStart:lineEnd] for the
// CommonMark reference definition pattern (0-3 leading spaces,
// `[label]:`, optional space/tab, a non-whitespace destination).
// Returns the absolute byte offsets of the bracket contents and ok=true
// on a hit. Mirrors the previous regex
// `(?m)^[ ]{0,3}\[([^\]\n]+)\]:[ \t]*\S+.*$` byte-for-byte.
func scanRefDefLine(source []byte, lineStart, lineEnd int) (labelStart, labelEnd int, ok bool) {
	j := lineStart
	spaces := 0
	for j < lineEnd && source[j] == ' ' && spaces < 3 {
		j++
		spaces++
	}
	if j >= lineEnd || source[j] != '[' {
		return -1, -1, false
	}
	labelStart = j + 1
	k := labelStart
	for k < lineEnd && source[k] != ']' {
		k++
	}
	if k >= lineEnd || k == labelStart {
		// Missing `]` on the line, or empty label (matches the
		// regex's `[^\]\n]+` which requires ≥ 1 char).
		return -1, -1, false
	}
	labelEnd = k
	colon := labelEnd + 1
	if colon >= lineEnd || source[colon] != ':' {
		return -1, -1, false
	}
	after := colon + 1
	for after < lineEnd && (source[after] == ' ' || source[after] == '\t') {
		after++
	}
	if after >= lineEnd {
		return -1, -1, false
	}
	// `\S` rejects ASCII whitespace; the trim loop above already
	// consumed ' ' and '\t', so this rejects \r and the other
	// whitespace runes the regex form also rejected.
	if isASCIIWhitespace(source[after]) {
		return -1, -1, false
	}
	return labelStart, labelEnd, true
}

// isASCIIWhitespace mirrors Go's `\s` character class for the
// destination-prefix guard: the regex `\S+` rejects ' ', '\t', '\n',
// '\r', '\f', and '\v'.
func isASCIIWhitespace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r' || b == '\f' || b == '\v'
}

// collectUsedLabels walks the AST and returns the set of normalized labels
// referenced by at least one reference-style link or image. The walk is
// open-coded with a recursive helper (collectUsedLabelsInto) rather than
// ast.Walk so the per-Check closure box that ast.Walk would otherwise
// require does not heap-allocate — plan 195 task 6.
func collectUsedLabels(f *lint.File) map[string]struct{} {
	used := map[string]struct{}{}
	collectUsedLabelsInto(f.AST, used)
	return used
}

// collectUsedLabelsInto recursively descends node `n` and inserts the
// normalised label of every reference-style link or image into used.
// A package-level recursion sidesteps the ast.Walk callback's
// heap-allocated closure: the helper closes over nothing, so each
// frame is plain stack work.
func collectUsedLabelsInto(n ast.Node, used map[string]struct{}) {
	if n == nil {
		return
	}
	switch v := n.(type) {
	case *ast.Link:
		if v.Reference != nil {
			used[util.ToLinkReference(v.Reference.Value)] = struct{}{}
		}
	case *ast.Image:
		if v.Reference != nil {
			used[util.ToLinkReference(v.Reference.Value)] = struct{}{}
		}
	}
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		collectUsedLabelsInto(c, used)
	}
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

// FixTitle implements rule.QuickFixTitler.
func (r *Rule) FixTitle() string { return "Remove unused link definitions" }
