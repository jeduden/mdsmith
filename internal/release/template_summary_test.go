package release

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// summaryCommentRe strips Hugo comment blocks (`{{/* ... */}}` and
// `{{- /* ... */ -}}`) so the scanner does not see `.Params.summary`
// mentions inside comments as live references. (?s) makes . match
// newlines so multi-line comments are caught.
var summaryCommentRe = regexp.MustCompile(`(?s)\{\{-?\s*/\*.*?\*/\s*-?\}\}`)

// summaryRefRe matches `.Params.summary` (possibly followed by a
// subfield such as `.X`) at a word boundary.
var summaryRefRe = regexp.MustCompile(`\.Params\.summary\b`)

// summaryAssignRe matches a variable assignment whose right-hand side
// references `.Params.summary` — e.g. `{{ $s := .Params.summary }}`.
// The check forbids this form because the bound name escapes the
// per-action scan; if the variable is later emitted via `{{ $s }}`,
// the value ships raw with no way for the scanner to know.
var summaryAssignRe = regexp.MustCompile(`^\$\w+\s*:?=`)

// summaryViolation records one misuse of `.Params.summary` in a Hugo
// template, located by file path and line. Body carries the action
// text so the failure message points the reader at the exact form.
type summaryViolation struct {
	Path string
	Line int
	Body string
	Why  string
}

// action is one `{{...}}` block located in source. outerStart/End span
// the delimiters; bodyStart/End span the contents between them.
type action struct {
	outerStart, outerEnd int
	bodyStart, bodyEnd   int
}

// findActions returns every `{{...}}` block in content, respecting
// double-quoted strings and backtick-delimited raw strings so that
// braces inside string literals do not terminate an action early.
// (Naive regex-based scanning misses `{{ printf "{%s}" .X }}`.)
func findActions(content string) []action {
	var actions []action
	n := len(content)
	i := 0
	for i+1 < n {
		if content[i] != '{' || content[i+1] != '{' {
			i++
			continue
		}
		a, next, ok := scanOneAction(content, i)
		if !ok {
			break
		}
		actions = append(actions, a)
		i = next
	}
	return actions
}

// scanOneAction reads one `{{...}}` action starting at the `{{` at
// position start. It returns the action, the index past the closing
// `}}`, and ok=true on success. ok=false means the action was not
// terminated before end of input — the caller treats this as the
// end of the content.
func scanOneAction(content string, start int) (a action, next int, ok bool) {
	n := len(content)
	bodyStart := start + 2
	j := bodyStart
	for j+1 < n {
		c := content[j]
		switch c {
		case '"':
			j = skipDoubleQuoted(content, j)
		case '`':
			j = skipBacktickQuoted(content, j)
		case '}':
			if j+1 < n && content[j+1] == '}' {
				return action{
					outerStart: start,
					outerEnd:   j + 2,
					bodyStart:  bodyStart,
					bodyEnd:    j,
				}, j + 2, true
			}
			j++
		default:
			j++
		}
	}
	return action{}, n, false
}

// skipDoubleQuoted advances past a double-quoted Go template string
// starting at content[i] == '"', honoring backslash escapes.
func skipDoubleQuoted(content string, i int) int {
	n := len(content)
	i++
	for i < n && content[i] != '"' {
		if content[i] == '\\' && i+1 < n {
			i += 2
			continue
		}
		i++
	}
	if i < n {
		i++
	}
	return i
}

// skipBacktickQuoted advances past a raw backtick-delimited string
// starting at content[i] == '`'. Backtick strings do not honor escapes.
func skipBacktickQuoted(content string, i int) int {
	n := len(content)
	i++
	for i < n && content[i] != '`' {
		i++
	}
	if i < n {
		i++
	}
	return i
}

// scanSummaryViolations finds every misuse of `.Params.summary` in
// the given Hugo template content. See classifyAction for the per-
// action contract. path is used only to populate violation.Path.
func scanSummaryViolations(path, content string) []summaryViolation {
	// Strip comments first, replacing each with newlines so line
	// numbers reported later still align with the source.
	stripped := summaryCommentRe.ReplaceAllStringFunc(content, func(c string) string {
		return strings.Repeat("\n", strings.Count(c, "\n"))
	})

	var out []summaryViolation
	for _, a := range findActions(stripped) {
		raw := stripped[a.bodyStart:a.bodyEnd]
		body := strings.TrimSpace(raw)
		body = strings.TrimPrefix(body, "-")
		body = strings.TrimSuffix(body, "-")
		body = strings.TrimSpace(body)
		if !summaryRefRe.MatchString(body) {
			continue
		}
		if ok, _ := classifyAction(body); ok {
			continue
		}
		_, why := classifyAction(body)
		out = append(out, summaryViolation{
			Path: path,
			Line: 1 + strings.Count(stripped[:a.outerStart], "\n"),
			Body: strings.TrimSpace(raw),
			Why:  why,
		})
	}
	return out
}

// classifyAction decides whether one Hugo action body (delimiters
// and trim markers already stripped) uses `.Params.summary` safely.
//
// Safe forms:
//   - Presence predicates `if [not] .Params.summary[...]` and their
//     `else if` variant. Compound forms (`if and .Params.summary $x`)
//     and subfield access (`if .Params.summary.X`) are all accepted —
//     the rule cares only that the action does not produce output.
//   - A `.RenderString` call that takes `.Params.summary` as a top-
//     level positional argument, or a pipeline whose terminal stage
//     is `.RenderString` and whose head emits `.Params.summary`.
//
// Forbidden:
//   - `with` / `else with .Params.summary` — these rebind `.` to the
//     summary string and the body typically emits raw.
//   - Variable assignment `$s := .Params.summary` — the bound name
//     escapes this per-action check.
//   - Any other action that mentions `.Params.summary` — bare output,
//     nested inside a non-RenderString call, or piped to a function
//     other than `.RenderString`.
func classifyAction(body string) (safe bool, reason string) {
	if hasLeadingWord(body, "if") || hasLeadingPhrase(body, "else", "if") {
		return true, ""
	}
	if hasLeadingWord(body, "with") || hasLeadingPhrase(body, "else", "with") {
		return false, "`with` / `else with .Params.summary` rebinds the dot and emits the value raw"
	}
	if hasLeadingWord(body, "range") {
		return true, ""
	}
	if summaryAssignRe.MatchString(body) {
		return false, "variable assignment of .Params.summary — pass the value directly " +
			"to .RenderString instead of binding a name"
	}
	return pipelineRendersSummary(body)
}

// hasLeadingWord reports whether body starts with `word` followed by
// a whitespace boundary (or the end of the string). Skips trim markers
// and surrounding whitespace already stripped by the caller.
func hasLeadingWord(body, word string) bool {
	if !strings.HasPrefix(body, word) {
		return false
	}
	rest := body[len(word):]
	if rest == "" {
		return true
	}
	return rest[0] == ' ' || rest[0] == '\t' || rest[0] == '\n'
}

// hasLeadingPhrase reports whether body starts with `first` then
// whitespace then `second`. Used for two-keyword openers like
// "else if" and "else with".
func hasLeadingPhrase(body, first, second string) bool {
	if !hasLeadingWord(body, first) {
		return false
	}
	rest := strings.TrimLeft(body[len(first):], " \t\n")
	return hasLeadingWord(rest, second)
}

// pipelineRendersSummary checks that `.Params.summary` in a value-
// producing pipeline reaches `.RenderString`, either as a direct
// positional argument or as the pipe input to a terminating
// `.RenderString` stage.
func pipelineRendersSummary(body string) (bool, string) {
	stages := splitPipeStages(body)

	// Locate the stage whose first command is `.RenderString` (if any).
	renderStageIdx := -1
	for i, s := range stages {
		if strings.HasPrefix(strings.TrimSpace(s), ".RenderString") {
			head := firstToken(strings.TrimSpace(s))
			if head == ".RenderString" {
				renderStageIdx = i
				break
			}
		}
	}

	for i, s := range stages {
		ts := strings.TrimSpace(s)
		// Where does .Params.summary appear in this stage?
		hits := summaryHits(ts)
		if len(hits) == 0 {
			continue
		}

		// Any hit nested inside parens? That means .Params.summary is
		// an argument to a sub-call, not a top-level argument to the
		// stage's head function — unsafe regardless of head.
		for _, h := range hits {
			if h.depth > 0 {
				return false, "`.Params.summary` appears nested inside a non-RenderString call " +
					"(e.g. `printf` or another helper)"
			}
		}

		head := firstToken(ts)

		if head == ".RenderString" && i == renderStageIdx {
			// Top-level positional argument to .RenderString. Safe.
			continue
		}

		// Stage doesn't start with .RenderString. Safe only if .Params.summary
		// is the head of this stage AND a later stage's head is .RenderString
		// (i.e. the summary value is the pipe input that flows through).
		if head == ".Params.summary" && renderStageIdx > i {
			// Verify intermediate stages don't transform the value into
			// something else of the same shape — we cannot reason about
			// arbitrary functions, so we require the chain has no
			// .Params.summary hits beyond the head (and the head was
			// already checked above).
			continue
		}

		return false, "`.Params.summary` is referenced outside an `if` predicate and is not passed to `.RenderString`"
	}

	return true, ""
}

// splitPipeStages splits a Hugo action body on `|` tokens at paren-
// depth 0, respecting double-quoted and backtick string literals.
func splitPipeStages(body string) []string {
	var stages []string
	depth := 0
	start := 0
	i := 0
	for i < len(body) {
		c := body[i]
		switch c {
		case '"':
			i++
			for i < len(body) && body[i] != '"' {
				if body[i] == '\\' && i+1 < len(body) {
					i += 2
					continue
				}
				i++
			}
			if i < len(body) {
				i++
			}
		case '`':
			i++
			for i < len(body) && body[i] != '`' {
				i++
			}
			if i < len(body) {
				i++
			}
		case '(':
			depth++
			i++
		case ')':
			if depth > 0 {
				depth--
			}
			i++
		case '|':
			if depth == 0 {
				stages = append(stages, body[start:i])
				start = i + 1
			}
			i++
		default:
			i++
		}
	}
	stages = append(stages, body[start:])
	return stages
}

// firstToken returns the first whitespace-delimited token of body
// (with surrounding whitespace already stripped by the caller).
func firstToken(body string) string {
	for i, c := range body {
		if c == ' ' || c == '\t' || c == '\n' || c == '(' {
			return body[:i]
		}
	}
	return body
}

// summaryHit records one occurrence of .Params.summary in a stage,
// with the parenthesis depth at the point of reference. depth > 0
// means the reference is nested inside `(...)` — i.e. passed as an
// argument to a function other than the stage's head.
type summaryHit struct {
	offset int
	depth  int
}

// summaryHits returns every occurrence of `.Params.summary` in stage,
// tagged with the parenthesis depth at which it appears.
func summaryHits(stage string) []summaryHit {
	locs := summaryRefRe.FindAllStringIndex(stage, -1)
	if len(locs) == 0 {
		return nil
	}

	depths := computeDepths(stage)
	hits := make([]summaryHit, 0, len(locs))
	for _, loc := range locs {
		hits = append(hits, summaryHit{offset: loc[0], depth: depths[loc[0]]})
	}
	return hits
}

// computeDepths walks stage and returns, for each byte offset, the
// parenthesis depth at that offset. String literals are skipped so
// `(` and `)` inside quotes don't perturb the depth.
func computeDepths(stage string) []int {
	depths := make([]int, len(stage)+1)
	depth := 0
	i := 0
	for i < len(stage) {
		depths[i] = depth
		c := stage[i]
		switch c {
		case '"':
			i++
			for i < len(stage) && stage[i] != '"' {
				depths[i] = depth
				if stage[i] == '\\' && i+1 < len(stage) {
					depths[i+1] = depth
					i += 2
					continue
				}
				i++
			}
			if i < len(stage) {
				depths[i] = depth
				i++
			}
		case '`':
			i++
			for i < len(stage) && stage[i] != '`' {
				depths[i] = depth
				i++
			}
			if i < len(stage) {
				depths[i] = depth
				i++
			}
		case '(':
			depth++
			i++
		case ')':
			if depth > 0 {
				depth--
			}
			i++
		default:
			i++
		}
	}
	depths[len(stage)] = depth
	return depths
}

// TestSummaryFrontMatterRenderedThroughRenderString pins the
// invariant that every reference to `.Params.summary` in Hugo
// templates either checks presence or renders through
// `.RenderString`. The regression this guards against is
// `{{ with .Params.summary }}<p>{{ . }}</p>{{ end }}`: `with`
// rebinds `.` to the summary string and `{{ . }}` then emits the
// value raw — so a summary like "Use `<?catalog?>`..." ships with
// literal backticks instead of `<code>` tags.
//
// Exempt: website/layouts/_default/baseof.html. Its meta-description
// fallback emits the summary as plain text on purpose (after a
// `| plainify` pass to strip any Markdown rendering); `<meta>`
// content cannot contain HTML.
func TestSummaryFrontMatterRenderedThroughRenderString(t *testing.T) {
	layoutsDir := filepath.Join(repoRoot(t), "website", "layouts")
	exemptRel := filepath.Join("_default", "baseof.html")

	var violations []summaryViolation
	var ioErrors []string
	require.NoError(t, filepath.Walk(layoutsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			ioErrors = append(ioErrors, fmt.Sprintf("walk %s: %v", path, err))
			return nil
		}
		if info.IsDir() || filepath.Ext(path) != ".html" {
			return nil
		}
		rel, _ := filepath.Rel(layoutsDir, path)
		if rel == exemptRel {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			ioErrors = append(ioErrors, fmt.Sprintf("read %s: %v", path, readErr))
			return nil
		}
		violations = append(violations, scanSummaryViolations(rel, string(data))...)
		return nil
	}))

	formatted := make([]string, 0, len(violations))
	for _, v := range violations {
		formatted = append(formatted, fmt.Sprintf("%s:%d: %s — %s", v.Path, v.Line, v.Body, v.Why))
	}
	assert.Empty(t, formatted,
		"every .Params.summary reference outside _default/baseof.html must be "+
			"a presence predicate (`if`/`else if`) or render through `.RenderString`")
	assert.Empty(t, ioErrors, "filesystem errors during scan")
}

// TestClassifyAction_TableDriven exercises classifyAction against
// every safe and unsafe shape the scanner is supposed to recognise.
// New cases land here when the rule changes or a corner case appears.
func TestClassifyAction_TableDriven(t *testing.T) {
	cases := []struct {
		name      string
		body      string
		violation bool
	}{
		// Safe — presence predicates.
		{"if presence", `if .Params.summary`, false},
		{"if not", `if not .Params.summary`, false},
		{"if compound and", `if and .Params.summary $cond`, false},
		{"if compound or", `if or .Params.summary $other`, false},
		{"else if", `else if .Params.summary`, false},
		{"if subfield", `if .Params.summary.HTML`, false},
		{"range", `range .Params.summary`, false},

		// Safe — RenderString calls.
		{"positional with options", `.RenderString (dict "display" "inline") .Params.summary`, false},
		{"positional bare", `.RenderString .Params.summary`, false},
		{"piped one stage", `.Params.summary | .RenderString`, false},
		{"piped through transform", `.Params.summary | strings.TrimSpace | .RenderString`, false},

		// Unsafe — rebinding forms.
		{"with rebind", `with .Params.summary`, true},
		{"else with", `else with .Params.summary`, true},

		// Unsafe — bare output and assignment.
		{"bare output", `.Params.summary`, true},
		{"var assign", `$s := .Params.summary`, true},
		{"var declare", `$s = .Params.summary`, true},

		// Unsafe — nested inside non-RenderString call.
		{"nested in printf", `.RenderString (printf "wrapper: %s" .Params.summary)`, true},
		{"printf outside RenderString", `printf "%v %v" (.RenderString "foo") .Params.summary`, true},

		// Unsafe — piped to wrong function.
		{"piped to print not render", `.Params.summary | print "x is" .Page.RenderString`, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			safe, reason := classifyAction(tc.body)
			if tc.violation {
				assert.False(t, safe, "expected violation; got safe (reason: %q) for: %s", reason, tc.body)
			} else {
				assert.True(t, safe, "expected safe; got violation (reason: %q) for: %s", reason, tc.body)
			}
		})
	}
}

// TestFindActions_BalancedStrings pins the tokenizer's awareness of
// braces inside string literals. The previous regex-only scanner
// silently skipped any action whose body contained `{` or `}` in a
// quoted string.
func TestFindActions_BalancedStrings(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    int
	}{
		{"brace in double-quoted string", `<p>{{ printf "{%s}" .Params.summary }}</p>`, 1},
		{"brace in backtick string", "<p>{{ printf `{%s}` .Params.summary }}</p>", 1},
		{"escaped quote", `<p>{{ printf "a\"b" .X }}</p>`, 1},
		{"adjacent actions", `{{ .A }}{{ .B }}`, 2},
		{"no actions", `<p>plain text</p>`, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := findActions(tc.content)
			assert.Len(t, got, tc.want)
		})
	}
}

// TestScanSummaryViolations_CommentsIgnored pins that Hugo comment
// blocks mentioning `.Params.summary` are not flagged. The previous
// regex scanner saw comments as ordinary actions and reported them.
func TestScanSummaryViolations_CommentsIgnored(t *testing.T) {
	content := `<p>
{{- /* renders .Params.summary via .RenderString */ -}}
{{ if .Params.summary }}<p>{{ .RenderString (dict "display" "inline") .Params.summary }}</p>{{ end }}
</p>`
	got := scanSummaryViolations("file.html", content)
	assert.Empty(t, got, "comments referencing .Params.summary must not be flagged")
}

// TestScanSummaryViolations_MultiLineWith pins the multi-line action
// case: an opening `{{ with .Params.summary }}` that wraps across
// newlines must still be flagged.
func TestScanSummaryViolations_MultiLineWith(t *testing.T) {
	content := `<p>
{{ with
  .Params.summary }}
  <span>{{ . }}</span>
{{ end }}
</p>`
	got := scanSummaryViolations("file.html", content)
	require.Len(t, got, 1)
	assert.Contains(t, got[0].Why, "with")
	assert.Equal(t, 2, got[0].Line)
}

// TestScanSummaryViolations_BraceInString pins that a summary
// reference inside an action whose body contains brace characters
// in a string literal is still scanned (the tokenizer respects
// quote boundaries).
func TestScanSummaryViolations_BraceInString(t *testing.T) {
	content := `<p>{{ printf "{%s}" .Params.summary }}</p>`
	got := scanSummaryViolations("file.html", content)
	require.Len(t, got, 1, "summary inside printf with brace-bearing string must still be flagged")
}
