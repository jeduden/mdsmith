package release

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"text/template/parse"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// summaryViolation records one misuse of `.Params.summary` in a
// Hugo template, located by file path and line.
type summaryViolation struct {
	Path string
	Line int
	Why  string
}

// scanSummaryViolations parses Hugo template content via Go's
// text/template/parse package (in SkipFuncCheck mode so undefined
// Hugo helpers like `dict`, `partial`, `printf` do not error) and
// walks the AST to find any reference to `.Params.summary` outside
// a safe context.
//
// Safe:
//   - `if` predicate, including compound forms (`if and ...`,
//     `if or ...`) and subfield access (`if .Params.summary.HTML`).
//   - Argument to a `.RenderString` call — positional, piped, or
//     nested inside a sub-pipeline whose output flows into
//     `.RenderString`. Qualified receivers like `$.RenderString`
//     and `.Page.RenderString` are recognised.
//   - The classifier treats `.Params.summary` and any case variant
//     (`.params.summary`, `.Params.Summary`) the same way, matching
//     Hugo's case-insensitive Params map.
//
// Forbidden:
//   - `with` / `else with .Params.summary` — the body rebinds the
//     dot and emits the value raw.
//   - `range .Params.summary` — iterates the string rune-by-rune,
//     emitting each code point as an integer.
//   - Variable assignment that binds the summary value to a name,
//     including the `if $s := .Params.summary` form; the bound name
//     escapes the per-action check.
//   - Any value-emitting action whose pipe references the summary
//     without reaching a `.RenderString` call.
func scanSummaryViolations(path, content string) ([]summaryViolation, error) {
	tree := parse.New(path)
	tree.Mode = parse.SkipFuncCheck
	if _, err := tree.Parse(content, "{{", "}}", map[string]*parse.Tree{}); err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	w := &summaryWalker{path: path, content: content}
	w.walk(tree.Root)
	return w.violations, nil
}

type summaryWalker struct {
	path       string
	content    string
	violations []summaryViolation
}

func (w *summaryWalker) lineOf(pos parse.Pos) int {
	off := int(pos)
	if off > len(w.content) {
		off = len(w.content)
	}
	return 1 + strings.Count(w.content[:off], "\n")
}

func (w *summaryWalker) add(pos parse.Pos, why string) {
	w.violations = append(w.violations, summaryViolation{
		Path: w.path,
		Line: w.lineOf(pos),
		Why:  why,
	})
}

func (w *summaryWalker) walk(n parse.Node) {
	if n == nil {
		return
	}
	switch n := n.(type) {
	case *parse.ListNode:
		if n == nil {
			return
		}
		for _, child := range n.Nodes {
			w.walk(child)
		}
	case *parse.ActionNode:
		w.checkAction(n)
	case *parse.IfNode:
		w.checkBranch(n.Pipe, n.Pos, "if")
		w.walk(n.List)
		w.walk(n.ElseList)
	case *parse.WithNode:
		w.checkWith(n)
		w.walk(n.List)
		w.walk(n.ElseList)
	case *parse.RangeNode:
		w.checkRange(n)
		w.walk(n.List)
		w.walk(n.ElseList)
	}
}

func (w *summaryWalker) checkAction(n *parse.ActionNode) {
	if pipeAssignsSummary(n.Pipe) {
		w.add(n.Pos, "variable assignment of .Params.summary — pass the value directly to .RenderString")
		return
	}
	if pipeReferencesSummary(n.Pipe) && !pipeOutputsSummaryViaRenderString(n.Pipe) {
		w.add(n.Pos, ".Params.summary referenced in a value-emitting action that does not pass it to .RenderString")
	}
}

func (w *summaryWalker) checkBranch(p *parse.PipeNode, pos parse.Pos, keyword string) {
	if pipeAssignsSummary(p) {
		w.add(pos, "variable assignment of .Params.summary in `"+keyword+
			"` predicate — the bound name escapes the per-action check")
	}
}

func (w *summaryWalker) checkWith(n *parse.WithNode) {
	if pipeAssignsSummary(n.Pipe) {
		w.add(n.Pos, "variable assignment of .Params.summary in `with` predicate — "+
			"the bound name escapes the per-action check")
		return
	}
	if pipeReferencesSummary(n.Pipe) {
		w.add(n.Pos, "`with .Params.summary` rebinds the dot and the body emits the value raw")
	}
}

func (w *summaryWalker) checkRange(n *parse.RangeNode) {
	if pipeAssignsSummary(n.Pipe) {
		w.add(n.Pos, "variable assignment of .Params.summary in `range` predicate — "+
			"iterating a string rebinds the dot to each rune")
		return
	}
	if pipeReferencesSummary(n.Pipe) {
		w.add(n.Pos, "`range .Params.summary` iterates the string rune-by-rune and emits each code point as an integer")
	}
}

// fieldIsSummary reports whether a FieldNode references
// `.Params.summary` (or a subfield like `.Params.summary.HTML`).
// Case-insensitive on `Params` and `summary` because Hugo's Params
// is a case-insensitive map.
func fieldIsSummary(f *parse.FieldNode) bool {
	if len(f.Ident) < 2 {
		return false
	}
	return strings.EqualFold(f.Ident[0], "Params") &&
		strings.EqualFold(f.Ident[1], "summary")
}

// chainIsSummary reports whether a ChainNode references
// `.Params.summary` via a dollar-context base, e.g. `$.Params.summary`.
func chainIsSummary(c *parse.ChainNode) bool {
	if len(c.Field) < 2 {
		return false
	}
	return strings.EqualFold(c.Field[0], "Params") &&
		strings.EqualFold(c.Field[1], "summary")
}

// pipeReferencesSummary returns true if any FieldNode/ChainNode
// anywhere in the pipe (including sub-pipes inside command args)
// references `.Params.summary`.
func pipeReferencesSummary(p *parse.PipeNode) bool {
	if p == nil {
		return false
	}
	for _, c := range p.Cmds {
		if cmdReferencesSummary(c) {
			return true
		}
	}
	return false
}

func cmdReferencesSummary(c *parse.CommandNode) bool {
	for _, arg := range c.Args {
		if argReferencesSummary(arg) {
			return true
		}
	}
	return false
}

func argReferencesSummary(arg parse.Node) bool {
	switch n := arg.(type) {
	case *parse.FieldNode:
		return fieldIsSummary(n)
	case *parse.ChainNode:
		return chainIsSummary(n)
	case *parse.PipeNode:
		return pipeReferencesSummary(n)
	}
	return false
}

// pipeAssignsSummary returns true if the pipe declares variables
// (a `:=` or `=`) and the right-hand value references the summary.
// Pipes with declarations have non-nil Decl.
func pipeAssignsSummary(p *parse.PipeNode) bool {
	if p == nil || len(p.Decl) == 0 {
		return false
	}
	return pipeReferencesSummary(p)
}

// pipeOutputsSummaryViaRenderString walks the pipe stage-by-stage
// and returns true if `.Params.summary` reaches a `.RenderString`
// call somewhere in the chain — either as a direct positional
// argument to that call, inside a sub-pipeline argument, or piped
// in from an earlier stage. Once the value has been through
// `.RenderString`, any subsequent filter stage (e.g. `| plainify`,
// `| safeHTML`) is fine — the Markdown rendering has already
// happened.
func pipeOutputsSummaryViaRenderString(p *parse.PipeNode) bool {
	if p == nil || len(p.Cmds) == 0 {
		return false
	}
	// summaryFlowing tracks whether the running pipe input carries
	// the summary value (i.e., derives from .Params.summary).
	summaryFlowing := false
	for i, cmd := range p.Cmds {
		if cmdIsRenderString(cmd) {
			// Summary flows in via a positional arg or the piped input.
			for _, arg := range cmd.Args[1:] {
				if argReferencesSummary(arg) {
					return true
				}
			}
			if summaryFlowing {
				return true
			}
		}
		// Track whether this stage's output carries the summary.
		// For the first stage, the function position (Args[0]) is
		// the value head when it's not a function call; we check
		// every arg. For later stages, the piped input flows in
		// implicitly, so summaryFlowing carries over.
		if i == 0 {
			for _, arg := range cmd.Args {
				if argReferencesSummary(arg) {
					summaryFlowing = true
					break
				}
			}
		} else {
			for _, arg := range cmd.Args[1:] {
				if argReferencesSummary(arg) {
					summaryFlowing = true
					break
				}
			}
		}
	}
	return false
}

// cmdIsRenderString reports whether the command's function (its
// first argument) ends in `RenderString`. Accepts the canonical
// `.RenderString` (FieldNode), qualified receivers like
// `.Page.RenderString` (FieldNode with multiple Idents), the
// dollar-context form `$.RenderString` (VariableNode with Ident
// `["$", "RenderString"]`), and bare chains via ChainNode.
func cmdIsRenderString(c *parse.CommandNode) bool {
	if len(c.Args) == 0 {
		return false
	}
	switch fn := c.Args[0].(type) {
	case *parse.FieldNode:
		if len(fn.Ident) == 0 {
			return false
		}
		return fn.Ident[len(fn.Ident)-1] == "RenderString"
	case *parse.ChainNode:
		if len(fn.Field) == 0 {
			return false
		}
		return fn.Field[len(fn.Field)-1] == "RenderString"
	case *parse.VariableNode:
		if len(fn.Ident) == 0 {
			return false
		}
		return fn.Ident[len(fn.Ident)-1] == "RenderString"
	}
	return false
}

// TestSummaryFrontMatterRenderedThroughRenderString walks every
// `.html` file under `website/layouts/` and asserts none uses
// `.Params.summary` in a forbidden context. No template is
// exempt — baseof.html's meta-description fallback uses
// `{{ if .Params.summary }}{{ $.RenderString ... .Params.summary | plainify }}`
// (no `with`-rebinding) so the scanner can verify it natively.
func TestSummaryFrontMatterRenderedThroughRenderString(t *testing.T) {
	layoutsDir := filepath.Join(repoRoot(t), "website", "layouts")

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
		rel, relErr := filepath.Rel(layoutsDir, path)
		if relErr != nil {
			ioErrors = append(ioErrors, fmt.Sprintf("rel %s: %v", path, relErr))
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			ioErrors = append(ioErrors, fmt.Sprintf("read %s: %v", path, readErr))
			return nil
		}
		got, scanErr := scanSummaryViolations(rel, string(data))
		if scanErr != nil {
			ioErrors = append(ioErrors, fmt.Sprintf("scan %s: %v", path, scanErr))
			return nil
		}
		violations = append(violations, got...)
		return nil
	}))

	formatted := make([]string, 0, len(violations))
	for _, v := range violations {
		formatted = append(formatted, fmt.Sprintf("%s:%d: %s", v.Path, v.Line, v.Why))
	}
	assert.Empty(t, formatted)
	assert.Empty(t, ioErrors, "filesystem errors during scan")
}

// TestScanSummaryViolations_TableDriven enumerates every safe and
// unsafe shape the AST classifier recognises. Each entry is the
// full template body (delimiters included) — the scanner parses
// it the same way Hugo would.
func TestScanSummaryViolations_TableDriven(t *testing.T) {
	cases := []struct {
		name      string
		template  string
		wantCount int
	}{
		// Safe: presence predicates.
		{"if presence",
			`{{ if .Params.summary }}<p>{{ .RenderString (dict "display" "inline") .Params.summary }}</p>{{ end }}`, 0},
		{"if not", `{{ if not .Params.summary }}x{{ end }}`, 0},
		{"if and compound", `{{ if and .Params.summary .X }}{{ .RenderString (dict) .Params.summary }}{{ end }}`, 0},
		{"if or compound", `{{ if or .Params.summary .Other }}x{{ end }}`, 0},
		{"else if", `{{ if .X }}{{ else if .Params.summary }}{{ .RenderString (dict) .Params.summary }}{{ end }}`, 0},
		{"if subfield", `{{ if .Params.summary.HTML }}x{{ end }}`, 0},
		{"eq comparison in if predicate", `{{ if eq .Params.summary "default" }}x{{ end }}`, 0},

		// Safe: .RenderString call shapes.
		{"positional with dict", `{{ .RenderString (dict "display" "inline") .Params.summary }}`, 0},
		{"positional bare", `{{ .RenderString .Params.summary }}`, 0},
		{"piped one stage", `{{ .Params.summary | .RenderString }}`, 0},
		{"piped two stages", `{{ .Params.summary | strings.TrimSpace | .RenderString }}`, 0},
		{"subfield positional", `{{ .RenderString (dict) .Params.summary.HTML }}`, 0},
		{"subfield piped", `{{ .Params.summary.HTML | .RenderString }}`, 0},
		{"qualified $.RenderString", `{{ $.RenderString (dict "display" "inline") .Params.summary }}`, 0},
		{"sub-pipeline arg to RenderString", `{{ .RenderString (dict) (printf "wrapper: %s" .Params.summary) }}`, 0},

		// Forbidden: rebinding.
		{"with rebind", `{{ with .Params.summary }}{{ . }}{{ end }}`, 1},
		{"else with rebind", `{{ with .Y }}{{ else with .Params.summary }}{{ . }}{{ end }}`, 1},
		{"range string", `{{ range .Params.summary }}{{ . }}{{ end }}`, 1},

		// Forbidden: variable assignment.
		{"var assign action", `{{ $s := .Params.summary }}`, 1},
		{"var assign in if", `{{ if $s := .Params.summary }}{{ $s }}{{ end }}`, 1},
		{"var assign in with", `{{ with $s := .Params.summary }}{{ $s }}{{ end }}`, 1},
		{"var assign in range", `{{ range $i, $v := .Params.summary }}{{ $v }}{{ end }}`, 1},

		// Forbidden: bare output, no RenderString.
		{"bare action", `{{ .Params.summary }}`, 1},
		{"printf no render", `{{ printf "x is %s" .Params.summary }}`, 1},

		// Forbidden: piped to non-RenderString.
		{"piped to print", `{{ .Params.summary | print "x is" .Page.RenderString }}`, 1},

		// Forbidden: comparison fed to non-render.
		{"co-occurrence in printf without render", `{{ printf "%v %v" (.RenderString "foo") .Params.summary }}`, 1},

		// Case-insensitive: lowercase `.params.summary` is still the same field.
		{"lowercase params", `{{ .params.summary }}`, 1},

		// Comments are stripped by the parser (no ParseComments).
		{"comment mentioning field", `{{/* renders .Params.summary via .RenderString */}}`, 0},

		// String literals are not field references — never flagged.
		{"summary literal inside string", `{{ printf "Warning: .Params.summary missing" .X }}`, 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := scanSummaryViolations("test.html", tc.template)
			require.NoError(t, err)
			if tc.wantCount != len(got) {
				lines := make([]string, len(got))
				for i, v := range got {
					lines[i] = v.Why
				}
				t.Fatalf("template %q: want %d violations, got %d:\n  %s",
					tc.template, tc.wantCount, len(got), strings.Join(lines, "\n  "))
			}
		})
	}
}

// TestScanSummaryViolations_MultiLineWith pins multi-line action
// detection. The parser handles newlines inside actions natively;
// no special tokenizer support is needed.
func TestScanSummaryViolations_MultiLineWith(t *testing.T) {
	content := "<p>\n{{ with\n  .Params.summary }}\n  <span>{{ . }}</span>\n{{ end }}\n</p>"
	got, err := scanSummaryViolations("file.html", content)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Contains(t, got[0].Why, "with")
}

// TestScanSummaryViolations_CRLFAction pins that CRLF line endings
// inside a multi-line action body don't misclassify a presence
// predicate. The text/template lexer accepts any whitespace,
// including \r, so the AST is the same whether the file uses LF
// or CRLF.
func TestScanSummaryViolations_CRLFAction(t *testing.T) {
	content := "{{ if\r\n.Params.summary }}{{ .RenderString (dict) .Params.summary }}{{ end }}"
	got, err := scanSummaryViolations("file.html", content)
	require.NoError(t, err)
	assert.Empty(t, got, "CRLF inside a multi-line `if` predicate must not be flagged")
}

// TestScanSummaryViolations_UnterminatedActionErrors pins that an
// unterminated `{{` returns a parse error (not silent acceptance).
// Hugo's own build would also fail on such input; the scanner
// surfaces the error explicitly rather than swallowing actions.
func TestScanSummaryViolations_UnterminatedActionErrors(t *testing.T) {
	_, err := scanSummaryViolations("file.html", `<p>{{ unterminated`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse")
}
