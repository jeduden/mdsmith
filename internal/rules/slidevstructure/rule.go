// Package slidevstructure implements MDS072 (slide-structure): a
// per-slide structural check for Slidev (https://sli.dev) decks.
//
// Slidev's renderer is deliberately permissive — an unmatched
// ::slot::, a misspelled layout, or an unknown frontmatter key
// produce wrong or empty output with no error. The rule splits a deck
// into slides on `---` separators, parses each slide's own frontmatter
// block, and reports the silent failures Slidev never does.
//
// The rule owns the Markdown/structural layer only: layout names,
// named slots, layout-required fields, and frontmatter key spelling.
// It does not resolve theme packages, render Vue, or validate UnoCSS
// classes. Theme-provided layouts are declared via the custom-layouts
// setting rather than discovered from node_modules.
package slidevstructure

import (
	"bytes"
	"fmt"
	"sort"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	rulesettings "github.com/jeduden/mdsmith/internal/rules/settings"
)

func init() {
	rule.Register(&Rule{})
}

// Rule implements MDS072.
type Rule struct {
	// CustomLayouts names theme/addon layouts that must be treated as
	// known (no unknown-layout diagnostic). Sorted for stable output.
	CustomLayouts []string
}

// ID implements rule.Rule.
func (r *Rule) ID() string { return "MDS072" }

// Name implements rule.Rule.
func (r *Rule) Name() string { return "slide-structure" }

// Category implements rule.Rule.
func (r *Rule) Category() string { return "structural" }

// EnabledByDefault implements rule.Defaultable. MDS072 is opt-in: it
// only makes sense for Slidev decks, so it ships off and is turned on
// by the `slidev` convention.
func (r *Rule) EnabledByDefault() bool { return false }

// builtinLayouts is the set of Slidev's 18 built-in layout names.
var builtinLayouts = map[string]bool{
	"center": true, "cover": true, "default": true, "end": true,
	"fact": true, "full": true, "image": true, "image-left": true,
	"image-right": true, "iframe": true, "iframe-left": true,
	"iframe-right": true, "intro": true, "none": true, "quote": true,
	"section": true, "statement": true, "two-cols": true,
	"two-cols-header": true,
}

// layoutSlots maps a layout to the named slots it exposes. A ::slot::
// for any other name is orphaned — its content will not render. The
// default (unmarked) region is the left column for two-cols and the
// header for two-cols-header, so only the extra slots are listed.
var layoutSlots = map[string][]string{
	"two-cols":        {"right"},
	"two-cols-header": {"left", "right"},
}

// layoutRequiredField maps a layout to the frontmatter field it needs.
var layoutRequiredField = map[string]string{
	"image": "image", "image-left": "image", "image-right": "image",
	"iframe": "url", "iframe-left": "url", "iframe-right": "url",
}

// knownFMKeys is the set of first-class per-slide frontmatter keys
// (from Slidev's Frontmatter type). Unknown keys pass through as data
// in Slidev, so a typo never errors; this set powers the did-you-mean
// check. Pass-through data keys that are within edit distance 2 of a
// real key are the false-positive risk, which is why the check only
// fires on a near miss, never on an arbitrary unrecognized key.
var knownFMKeys = map[string]bool{
	"layout": true, "class": true, "clicks": true, "clicksStart": true,
	"preload": true, "hide": true, "disabled": true, "hideInToc": true,
	"title": true, "level": true, "routeAlias": true, "zoom": true,
	"clickAnimation": true, "dragPos": true, "src": true,
	"transition": true, "background": true, "backgroundSize": true,
	"name": true, "image": true, "url": true, "default": true,
}

var fenceBytes = []byte("---")

// Check implements rule.Rule.
func (r *Rule) Check(f *lint.File) []lint.Diagnostic {
	if f == nil || len(f.Lines) == 0 {
		return nil
	}
	// Zero-alloc pre-scan: with no slide separator and no slot marker
	// there is nothing Slidev-specific to check — unless the engine
	// stripped a headmatter block that itself declares a layout. Check
	// f.FrontMatter first so a single-slide deck whose only layout
	// lives in the (stripped) headmatter is still validated.
	if !hasSlidevMarkers(f.Lines) && len(f.FrontMatter) == 0 {
		return nil
	}
	slides := parseSlides(f.Lines)
	// The engine strips the deck's headmatter — the first slide's own
	// frontmatter — into f.FrontMatter. Fold it back onto slide 0 so
	// the first slide's layout, slots, and keys are checked like any
	// other. Its source lines are gone, so headmatter diagnostics
	// anchor at the first body line (startLine).
	if len(f.FrontMatter) > 0 && len(slides) > 0 {
		slides[0].fm = parseFrontMatterBytes(f.FrontMatter)
		slides[0].fmLine = nil
	}
	var diags []lint.Diagnostic
	for i := range slides {
		diags = r.checkSlide(&slides[i], f, diags)
	}
	return diags
}

// parseFrontMatterBytes parses a stripped headmatter block (with or
// without its `---` fences) into key -> raw value. Only the simple
// `key: value` lines a slide's frontmatter uses are read; nested YAML
// is ignored, which is all the layout/slot/field checks need.
func parseFrontMatterBytes(b []byte) map[string]string {
	fm := map[string]string{}
	for _, ln := range bytes.Split(b, []byte("\n")) {
		t := bytes.TrimSpace(ln)
		if len(t) == 0 || bytes.Equal(t, fenceBytes) {
			continue
		}
		c := bytes.IndexByte(t, ':')
		if c <= 0 {
			continue
		}
		// Skip indented (nested) keys — top-level slide keys only.
		if ln[0] == ' ' || ln[0] == '\t' || ln[0] == '-' {
			continue
		}
		fm[string(bytes.TrimSpace(t[:c]))] = string(bytes.TrimSpace(t[c+1:]))
	}
	return fm
}

// hasSlidevMarkers reports whether any line outside a fenced code
// block is a `---` fence or a `::name::` slot separator. Pure byte
// scans, no allocation.
func hasSlidevMarkers(lines [][]byte) bool {
	inCode := false
	for _, ln := range lines {
		if isCodeFence(ln) {
			inCode = !inCode
			continue
		}
		if inCode {
			continue
		}
		if isFence(ln) {
			return true
		}
		if _, ok := slotName(ln); ok {
			return true
		}
	}
	return false
}

// isCodeFence reports whether a line opens or closes a fenced code
// block (``` or ~~~, three or more). A `---` or `::slot::` inside such
// a block is literal content — a slide showing YAML or a diff — not a
// separator, so the scanners skip it.
func isCodeFence(line []byte) bool {
	t := bytes.TrimLeft(line, " ")
	return bytes.HasPrefix(t, []byte("```")) || bytes.HasPrefix(t, []byte("~~~"))
}

// slide is one logical slide with its frontmatter and slot markers.
type slide struct {
	startLine int // 1-based line of the slide's first body content
	fm        map[string]string
	fmLine    map[string]int // key -> 1-based line, for precise anchors
	slots     []slotRef
}

type slotRef struct {
	name string
	line int
}

// parseSlides splits body lines into slides. A line equal to `---` is
// a boundary; if the block after it is YAML closed by another `---`,
// that block is the next slide's frontmatter. A leading fence is
// treated as the first slide's frontmatter (headmatter, when the
// engine has not already stripped it).
func parseSlides(lines [][]byte) []slide {
	var slides []slide
	i := 0
	cur := slide{startLine: 1}
	if isFence(lines[0]) {
		cur.fm, cur.fmLine, i = readFrontmatter(lines, 0)
		cur.startLine = i + 1
	}
	inCode := false
	for i < len(lines) {
		if isCodeFence(lines[i]) {
			inCode = !inCode
			i++
			continue
		}
		if inCode {
			i++
			continue
		}
		if isFence(lines[i]) {
			slides = append(slides, cur)
			if hasFrontmatterAfter(lines, i) {
				fm, fmLine, next := readFrontmatter(lines, i)
				cur = slide{startLine: next + 1, fm: fm, fmLine: fmLine}
				i = next
				continue
			}
			cur = slide{startLine: i + 2}
			i++
			continue
		}
		if name, ok := slotName(lines[i]); ok {
			cur.slots = append(cur.slots, slotRef{name: name, line: i + 1})
		}
		i++
	}
	slides = append(slides, cur)
	return slides
}

func isFence(line []byte) bool {
	return bytes.Equal(bytes.TrimRight(line, "\r"), fenceBytes)
}

// hasFrontmatterAfter reports whether the block after a boundary at
// index sep is a YAML frontmatter block: key/list lines closed by a
// second `---` with no blank line breaking into body first.
func hasFrontmatterAfter(lines [][]byte, sep int) bool {
	sawKey := false
	for j := sep + 1; j < len(lines); j++ {
		if isFence(lines[j]) {
			return sawKey
		}
		t := bytes.TrimSpace(lines[j])
		if len(t) == 0 {
			return false
		}
		if bytes.IndexByte(t, ':') >= 0 || bytes.HasPrefix(t, []byte("- ")) {
			sawKey = true
			continue
		}
		return false
	}
	return false
}

// readFrontmatter parses a `---`\nYAML\n`---` block opening at index
// open and returns the keys, their 1-based lines, and the index past
// the closing fence.
func readFrontmatter(lines [][]byte, open int) (map[string]string, map[string]int, int) {
	fm := map[string]string{}
	fmLine := map[string]int{}
	j := open + 1
	for ; j < len(lines) && !isFence(lines[j]); j++ {
		t := bytes.TrimSpace(lines[j])
		c := bytes.IndexByte(t, ':')
		if c <= 0 {
			continue
		}
		key := string(bytes.TrimSpace(t[:c]))
		val := string(bytes.TrimSpace(t[c+1:]))
		fm[key] = val
		fmLine[key] = j + 1
	}
	return fm, fmLine, j + 1
}

// slotName returns the name in a `::name::` slot line and whether the
// line is one. A bare `::` or an empty name does not count.
func slotName(line []byte) (string, bool) {
	t := bytes.TrimSpace(line)
	if len(t) >= 5 && bytes.HasPrefix(t, []byte("::")) && bytes.HasSuffix(t, []byte("::")) {
		inner := t[2 : len(t)-2]
		if len(inner) == 0 {
			return "", false
		}
		return string(inner), true
	}
	return "", false
}

func (r *Rule) checkSlide(s *slide, f *lint.File, diags []lint.Diagnostic) []lint.Diagnostic {
	layout, hasLayout := "", false
	if s.fm != nil {
		layout, hasLayout = s.fm["layout"]
	}
	anchor := s.startLine
	if ln, ok := s.fmLine["layout"]; ok {
		anchor = ln
	}

	// 1. Unknown layout name.
	if hasLayout && !builtinLayouts[layout] && !r.isCustomLayout(layout) {
		msg := fmt.Sprintf("unknown Slidev layout %q", layout)
		if sug := nearest(layout, r.layoutCandidates()); sug != "" {
			msg += fmt.Sprintf(" — did you mean %q?", sug)
		}
		diags = append(diags, r.diag(f, anchor, msg))
	}

	effLayout := layout
	if !hasLayout {
		effLayout = "default"
	}

	// 2. Missing required slot(s).
	for _, need := range layoutSlots[effLayout] {
		if !slotProvided(s.slots, need) {
			diags = append(diags, r.diag(f, anchor, fmt.Sprintf(
				"layout %q requires a ::%s:: slot — that column will be empty",
				effLayout, need)))
		}
	}

	// 3. Orphaned slot(s): content that will not render.
	accepted := layoutSlots[effLayout]
	for _, sl := range s.slots {
		if !contains(accepted, sl.name) {
			diags = append(diags, r.diag(f, sl.line, fmt.Sprintf(
				"::%s:: has no matching slot in layout %q — this content will not render",
				sl.name, effLayout)))
		}
	}

	// 4. Missing layout-required field.
	if field, ok := layoutRequiredField[effLayout]; ok {
		if _, present := s.fm[field]; !present {
			diags = append(diags, r.diag(f, anchor, fmt.Sprintf(
				"layout %q requires the %q frontmatter field", effLayout, field)))
		}
	}

	// 5. Unknown frontmatter key (typo — passes through silently).
	for _, k := range sortedKeys(s.fm) {
		if knownFMKeys[k] {
			continue
		}
		if sug := nearest(k, knownKeyList()); sug != "" {
			diags = append(diags, r.diag(f, s.fmLine[k], fmt.Sprintf(
				"unknown Slidev frontmatter key %q — did you mean %q?", k, sug)))
		}
	}

	return diags
}

func (r *Rule) isCustomLayout(name string) bool {
	for _, c := range r.CustomLayouts {
		if c == name {
			return true
		}
	}
	return false
}

func (r *Rule) diag(f *lint.File, line int, msg string) lint.Diagnostic {
	return lint.Diagnostic{
		File:     f.Path,
		Line:     line,
		Column:   1,
		RuleID:   r.ID(),
		RuleName: r.Name(),
		Severity: lint.Warning,
		Message:  msg,
	}
}

func slotProvided(slots []slotRef, name string) bool {
	for _, s := range slots {
		if s.name == name {
			return true
		}
	}
	return false
}

func contains(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}

func sortedKeys(m map[string]string) []string {
	if len(m) == 0 {
		return nil
	}
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// layoutCandidates returns builtin plus custom layout names for
// did-you-mean matching.
func (r *Rule) layoutCandidates() []string {
	out := make([]string, 0, len(builtinLayouts)+len(r.CustomLayouts))
	for k := range builtinLayouts {
		out = append(out, k)
	}
	out = append(out, r.CustomLayouts...)
	return out
}

func knownKeyList() []string {
	out := make([]string, 0, len(knownFMKeys))
	for k := range knownFMKeys {
		out = append(out, k)
	}
	return out
}

// nearest returns the closest candidate within edit distance 2, or "".
func nearest(s string, cands []string) string {
	best, bestD := "", 3
	for _, c := range cands {
		if d := editDistance(s, c); d < bestD {
			best, bestD = c, d
		}
	}
	return best
}

func editDistance(a, b string) int {
	la, lb := len(a), len(b)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}
	prev := make([]int, lb+1)
	cur := make([]int, lb+1)
	for j := 0; j <= lb; j++ {
		prev[j] = j
	}
	for i := 1; i <= la; i++ {
		cur[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min3(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[lb]
}

func min3(a, b, c int) int {
	if b < a {
		a = b
	}
	if c < a {
		a = c
	}
	return a
}

// ApplySettings implements rule.Configurable.
func (r *Rule) ApplySettings(settings map[string]any) error {
	for k, v := range settings {
		switch k {
		case "custom-layouts":
			names, ok := rulesettings.ToStringSlice(v)
			if !ok {
				return fmt.Errorf("slide-structure: custom-layouts must be a list of strings, got %T", v)
			}
			sort.Strings(names)
			r.CustomLayouts = names
		default:
			return fmt.Errorf("slide-structure: unknown setting %q", k)
		}
	}
	return nil
}

// DefaultSettings implements rule.Configurable.
func (r *Rule) DefaultSettings() map[string]any {
	return map[string]any{
		"custom-layouts": []string{},
	}
}
