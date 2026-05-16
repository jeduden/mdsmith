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
	"fmt"
	"sort"
	"strings"

	"github.com/jeduden/mdsmith/internal/archetype/gensection"
	"github.com/jeduden/mdsmith/internal/lint"
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
// Generated section markers are removed, generated bodies stay as
// plain Markdown, and `<?include?>` content is inlined (recursively,
// when the body is fresh or has been regenerated).
//
// Exactly one of the returned values is populated:
//   - on success, the exported bytes (non-nil) and a nil diagnostic
//     slice — including the no-op case of a directive-free file
//   - on refusal, nil bytes and a non-empty diagnostic slice naming
//     the offending directive(s); the caller should exit non-zero
func Export(f *lint.File, mode Mode) ([]byte, []lint.Diagnostic) {
	directives := selectDirectives(rule.All())

	working := f
	switch mode {
	case Fix:
		regenerated, diags := regenerate(f, directives)
		if len(diags) > 0 {
			return nil, diags
		}
		working = regenerated
	case Check:
		if diags := checkStaleness(f, directives); len(diags) > 0 {
			return nil, diags
		}
	case NoCheck:
		// Skip staleness handling; strip uses the on-disk body verbatim.
	}

	body := stripDirectives(working, directives)
	return working.FullSource(body), nil
}

// directiveRule pairs a fixable rule with its gensection.Directive
// view so we can call both Fix (for regeneration) and the
// directive-level Validate/Generate (for staleness checks).
type directiveRule struct {
	rule      rule.FixableRule
	directive gensection.Directive
}

// selectDirectives picks the rules that implement gensection.Directive
// AND rule.FixableRule, and orders them by directive name so behavior
// is deterministic across calls.
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

// regenerate runs each directive rule's Fix in memory until the source
// stops changing, then returns a freshly parsed *lint.File that
// downstream phases can walk. The input is not mutated.
func regenerate(orig *lint.File, directives []directiveRule) (*lint.File, []lint.Diagnostic) {
	current := append([]byte(nil), orig.Source...)

	const maxPasses = 10
	for pass := 0; pass < maxPasses; pass++ {
		before := current
		for _, d := range directives {
			parsed, err := lint.NewFile(orig.Path, current)
			if err != nil {
				continue
			}
			hydrate(parsed, orig)
			current = d.rule.Fix(parsed)
		}
		if bytes.Equal(before, current) {
			break
		}
	}

	working, err := lint.NewFile(orig.Path, current)
	if err != nil {
		return nil, []lint.Diagnostic{{
			File:     orig.Path,
			Line:     1,
			Column:   1,
			Severity: lint.Error,
			Message:  fmt.Sprintf("re-parsing regenerated source: %v", err),
		}}
	}
	hydrate(working, orig)
	working.FrontMatter = orig.FrontMatter
	working.LineOffset = orig.LineOffset
	working.StripFrontMatter = orig.StripFrontMatter
	return working, nil
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
// by another directive, matching the suppression `engine.CheckRules`
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
// lines around the holes left behind.
//
// Marker-like text the engine treats as literal content — for example
// inner same-type markers nested in an outer directive — survives,
// because such PIs sit inside a pair's body range and are skipped
// here.
func stripDirectives(f *lint.File, directives []directiveRule) []byte {
	stripLines := map[int]bool{}
	bodyLines := map[int]bool{}

	for _, d := range directives {
		pairs, _ := gensection.FindMarkerPairs(f, d.directive.Name(),
			d.directive.RuleID(), d.directive.RuleName())
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
		pi, ok := n.(*lint.ProcessingInstruction)
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

	out := emitLines(f.Lines, stripLines)
	return normalizeBlankLines(out)
}

// piLineRange returns the 1-based start and end source-line numbers
// of a processing-instruction block (including the closing ?>).
func piLineRange(pi *lint.ProcessingInstruction, f *lint.File) (int, int) {
	first := pi.Lines().At(0)
	start := f.LineOfOffset(first.Start)
	end := start
	if pi.HasClosure() && pi.ClosureLine.Start != first.Start {
		end = f.LineOfOffset(pi.ClosureLine.Start)
	} else if pi.Lines().Len() > 1 {
		last := pi.Lines().At(pi.Lines().Len() - 1)
		end = f.LineOfOffset(last.Start)
	}
	return start, end
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
func normalizeBlankLines(src []byte) []byte {
	if len(src) == 0 {
		return src
	}
	lines := strings.Split(string(src), "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}

	var out []string
	blank := false
	for _, l := range lines {
		if strings.TrimSpace(l) == "" {
			if !blank {
				out = append(out, "")
			}
			blank = true
		} else {
			out = append(out, l)
			blank = false
		}
	}
	if len(out) == 0 {
		return nil
	}
	result := strings.Join(out, "\n") + "\n"
	return []byte(result)
}
