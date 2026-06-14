// Package export implements the source-to-source transform behind
// `mdsmith export`. It strips every directive start/end marker from a
// Markdown file while keeping the directive bodies as plain Markdown,
// so the result renders on any tool without mdsmith knowledge.
//
// The package operates purely on an in-memory *lint.File. File reads
// and disk writes are the CLI layer's responsibility.
package export

import (
	"bytes"
	"sort"
	"strings"

	"github.com/jeduden/mdsmith/internal/archetype/gensection"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/piparser"
	"github.com/jeduden/mdsmith/internal/rule"
)

// Mode controls how Export handles directive staleness.
type Mode int

const (
	// Check is the default mode. A directive body that disagrees with
	// the engine's regenerated output is a refusal — Export returns nil
	// bytes and a diagnostic naming the stale directive.
	Check Mode = iota
	// Fix regenerates stale bodies in memory before stripping. The
	// source file is never written.
	Fix
	// NoCheck skips the staleness check entirely. Bodies are exported
	// exactly as they appear on disk.
	NoCheck
)

// Export returns a portable, directive-free copy of f's source.
//
// rules carries the caller's effective ruleset (already cloned and
// configured via checker.ConfigureRule, and filtered to enabled rules
// only — like fix.Fixer.fixableRules). Staleness checks (Check mode)
// and regeneration (Fix mode) only consult rules in this slice, so a
// directive disabled in `.mdsmith.yml` neither produces a stale-body
// refusal nor gets regenerated on `--fix`.
//
// Marker stripping is independent of `rules`: every directive
// registered in the global rule registry has its start/end markers
// stripped from the output, so a disabled directive's markers still
// disappear even though its body is untouched. Callers that want
// stripping but no staleness behavior can pass a nil rules slice.
//
// Generated section markers are removed, generated bodies stay as
// plain Markdown, and `<?include?>` content is inlined (recursively,
// when the body is fresh or has been regenerated).
//
// Exactly one of the returned values is populated:
//   - on success, the exported bytes (non-nil) and a nil diagnostic
//     slice — including the no-op case of a directive-free file
//   - on Check-mode refusal, nil bytes and a non-empty diagnostic
//     slice naming the offending directive(s); the caller should
//     exit non-zero
func Export(f *lint.File, mode Mode, rules []rule.Rule) ([]byte, []lint.Diagnostic) {
	active := selectDirectives(rules)
	stripDirs := allDirectiveNames()

	working := f
	switch mode {
	case Fix:
		working = regenerate(f, active)
	case Check:
		if diags := checkStaleness(f, active); len(diags) > 0 {
			return nil, diags
		}
	case NoCheck:
		// Skip staleness handling; strip uses the on-disk body verbatim.
	}

	body := stripDirectives(working, stripDirs)
	return working.FullSource(body), nil
}

// directiveRule pairs a fixable rule with its gensection.Directive
// view so we can call both Fix (for regeneration) and the
// directive-level Check (for staleness).
type directiveRule struct {
	rule      rule.FixableRule
	directive gensection.Directive
}

// selectDirectives picks the rules that implement gensection.Directive
// AND rule.FixableRule, and orders them by directive name so behavior
// is deterministic across calls. Returns nil for a nil/empty input.
func selectDirectives(rules []rule.Rule) []directiveRule {
	var out []directiveRule
	for _, r := range rules {
		fr, fok := r.(rule.FixableRule)
		if !fok {
			continue
		}
		d, dok := r.(gensection.Directive)
		if !dok {
			continue
		}
		out = append(out, directiveRule{rule: fr, directive: d})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].directive.Name() < out[j].directive.Name()
	})
	return out
}

// directiveStrip carries the minimal context Export needs to remove a
// directive's start/end markers and locate its body range. It is
// populated from the unconfigured registry so stripping recognises a
// directive even when its rule is disabled in `.mdsmith.yml`.
type directiveStrip struct {
	name     string
	ruleID   string
	ruleName string
}

// allDirectiveNames returns the strip-only descriptor for every
// directive registered at package-init time. Stripping is independent
// of the file's effective config — a disabled rule's markers should
// still vanish — so this list comes straight from `rule.All()` without
// any kind/override merge.
func allDirectiveNames() []directiveStrip {
	all := rule.All()
	var out []directiveStrip
	for _, r := range all {
		d, ok := r.(gensection.Directive)
		if !ok {
			continue
		}
		out = append(out, directiveStrip{
			name:     d.Name(),
			ruleID:   d.RuleID(),
			ruleName: d.RuleName(),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].name < out[j].name })
	return out
}

// regenerate runs each directive rule's Fix in memory until the
// source stops changing, then returns a freshly parsed *lint.File
// that downstream phases can walk. The input is not mutated.
//
// Mirrors fix.Fixer.applyFixPasses: each rule's Fix only runs when
// its Check fires — so up-to-date directives aren't regenerated
// gratuitously. lint.NewFile never errors with the current goldmark
// configuration (same invariant fix.Fixer relies on at
// buildPostFixFile), so re-parses here cannot fail.
func regenerate(orig *lint.File, directives []directiveRule) *lint.File {
	current := append([]byte(nil), orig.Source...)

	const maxPasses = 10
	for pass := 0; pass < maxPasses; pass++ {
		before := current
		for _, d := range directives {
			parsed, _ := lint.NewFile(orig.Path, current) // never errors today
			hydrate(parsed, orig)

			if len(d.rule.Check(parsed)) == 0 {
				continue
			}
			current = d.rule.Fix(parsed)
		}
		if bytes.Equal(before, current) {
			break
		}
	}

	working, _ := lint.NewFile(orig.Path, current) // never errors today
	hydrate(working, orig)
	working.FrontMatter = orig.FrontMatter
	working.LineOffset = orig.LineOffset
	working.StripFrontMatter = orig.StripFrontMatter
	return working
}

// hydrate copies the per-file context the directive engines rely on
// (FS, RootFS/RootDir, MaxInputBytes, gitignore factory) from orig
// onto parsed so a freshly parsed buffer behaves like the original.
func hydrate(parsed, orig *lint.File) {
	parsed.FS = orig.FS
	parsed.RootFS = orig.RootFS
	parsed.RootDir = orig.RootDir
	parsed.MaxInputBytes = orig.MaxInputBytes
	parsed.GitignoreFunc = orig.GitignoreFunc
	parsed.GeneratedRanges = gensection.FindAllGeneratedRanges(parsed)
}

// checkStaleness runs each directive's rule.Check and keeps only
// error-severity diagnostics, so blocking problems (stale body,
// invalid YAML, missing include file) cause a refusal while
// non-blocking hints (catalog case-mismatch, injection warnings)
// don't.
//
// Diagnostics whose line falls inside the host file's
// GeneratedRanges (i.e. inside an outer include/catalog body) are
// dropped: the host file is not responsible for content pulled in
// by another directive, matching the suppression `checker.CheckRules`
// applies on the regular check path.
//
// Returned diagnostics carry file-relative line numbers (front
// matter included) so the CLI prints positions a user can navigate
// to directly.
func checkStaleness(f *lint.File, directives []directiveRule) []lint.Diagnostic {
	var diags []lint.Diagnostic
	for _, d := range directives {
		for _, rd := range d.rule.Check(f) {
			if rd.Severity != lint.Error {
				continue
			}
			if inGeneratedRange(rd.Line, f.GeneratedRanges) {
				continue
			}
			diags = append(diags, rd)
		}
	}
	f.AdjustDiagnostics(diags)
	return diags
}

func inGeneratedRange(line int, ranges []lint.LineRange) bool {
	for _, r := range ranges {
		if r.Contains(line) {
			return true
		}
	}
	return false
}

// stripDirectives removes every line that the engine recognises as a
// real directive start or end marker, plus every markerless PI
// (e.g. <?allow-empty-section?>, <?require?>), and normalises blank
// lines around the holes left behind. A directive-free file is
// returned byte-for-byte unchanged — `export` is a no-op when there
// are no markers to remove.
//
// Marker-like text the engine treats as literal content — for example
// inner same-type markers nested in an outer directive — survives,
// because such PIs sit inside a pair's body range and are skipped
// here.
func stripDirectives(f *lint.File, directives []directiveStrip) []byte {
	stripLines := map[int]bool{}
	bodyLines := map[int]bool{}

	for _, d := range directives {
		pairs, _ := gensection.FindMarkerPairs(f, d.name, d.ruleID, d.ruleName)
		for _, p := range pairs {
			for line := p.StartLine; line < p.ContentFrom; line++ {
				stripLines[line] = true
			}
			stripLines[p.EndLine] = true
			for line := p.ContentFrom; line <= p.ContentTo; line++ {
				bodyLines[line] = true
			}
		}
	}

	// Markerless directives: every top-level PI whose lines fall
	// outside a known pair's strip range and body range.
	for n := f.AST.FirstChild(); n != nil; n = n.NextSibling() {
		pi, ok := n.(*piparser.ProcessingInstruction)
		if !ok {
			continue
		}
		startLine, endLine := piLineRange(pi, f)

		if overlapsAny(startLine, endLine, stripLines) {
			continue
		}
		if overlapsAny(startLine, endLine, bodyLines) {
			continue
		}
		for line := startLine; line <= endLine; line++ {
			stripLines[line] = true
		}
	}

	// No directive markers found — pass the source through verbatim
	// so the plan's "export of a directive-free file equals the input"
	// promise holds and code-block blank lines aren't disturbed.
	if len(stripLines) == 0 {
		return f.Source
	}

	out := emitLines(f.Lines, stripLines)
	// Re-parse so the code-block line set lines up with `out`'s
	// (shifted) line numbers. Without this the normalisation pass
	// would still see code-block lines at their pre-strip positions
	// and could collapse intentional blank lines inside fenced/
	// indented code blocks.
	parsed, _ := lint.NewFile(f.Path, out) // never errors today
	return normalizeBlankLines(out, lint.CollectCodeBlockLines(parsed))
}

// piLineRange returns the 1-based start and end source-line numbers
// of a processing-instruction block (including the closing ?>).
// Markerless PIs come in two shapes the goldmark PI parser can
// produce: a single-line PI (`<?name?>`) where the closure is on the
// opening line, and a multi-line PI where the closure sits on its
// own line; the latter is the only shape that extends `end` past
// `start`.
func piLineRange(pi *piparser.ProcessingInstruction, f *lint.File) (int, int) {
	first := pi.Lines().At(0)
	start := f.LineOfOffset(first.Start)
	if !pi.HasClosure() {
		return start, start
	}
	if pi.ClosureLine.Start == first.Start {
		return start, start
	}
	return start, f.LineOfOffset(pi.ClosureLine.Start)
}

func overlapsAny(from, to int, set map[int]bool) bool {
	for line := from; line <= to; line++ {
		if set[line] {
			return true
		}
	}
	return false
}

func emitLines(srcLines [][]byte, strip map[int]bool) []byte {
	var b bytes.Buffer
	for i, line := range srcLines {
		lineNum := i + 1
		if strip[lineNum] {
			continue
		}
		b.Write(line)
		if i < len(srcLines)-1 {
			b.WriteByte('\n')
		}
	}
	return b.Bytes()
}

// normalizeBlankLines collapses runs of consecutive blank lines to a
// single blank line, drops leading/trailing blanks, and ensures the
// output ends with exactly one newline (unless the result is empty).
//
// Lines whose 1-based index appears in codeBlockLines are treated as
// non-blank: they pass through unchanged, end any run of collapsing,
// and anchor leading/trailing trims. That preserves intentional
// blank lines inside fenced and indented code blocks, matching how
// MDS008 (blank-line rule) leaves code-block whitespace alone.
func normalizeBlankLines(src []byte, codeBlockLines map[int]struct{}) []byte {
	if len(src) == 0 {
		return src
	}
	rawLines := strings.Split(string(src), "\n")
	// strings.Split on a non-empty input always returns at least one
	// element, so the guard reduces to a check on the final element.
	if rawLines[len(rawLines)-1] == "" {
		rawLines = rawLines[:len(rawLines)-1]
	}

	type srcLine struct {
		text   string
		inCode bool
	}
	lines := make([]srcLine, len(rawLines))
	for i, l := range rawLines {
		_, inCode := codeBlockLines[i+1]
		lines[i] = srcLine{text: l, inCode: inCode}
	}

	isBlank := func(l srcLine) bool {
		return !l.inCode && strings.TrimSpace(l.text) == ""
	}

	for len(lines) > 0 && isBlank(lines[0]) {
		lines = lines[1:]
	}
	for len(lines) > 0 && isBlank(lines[len(lines)-1]) {
		lines = lines[:len(lines)-1]
	}

	var out []string
	blank := false
	for _, l := range lines {
		if isBlank(l) {
			if !blank {
				out = append(out, "")
			}
			blank = true
			continue
		}
		out = append(out, l.text)
		blank = false
	}
	if len(out) == 0 {
		return nil
	}
	result := strings.Join(out, "\n") + "\n"
	return []byte(result)
}
