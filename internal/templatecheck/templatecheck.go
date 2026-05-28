// Package templatecheck statically analyses Hugo templates for
// front-matter rendering invariants. The single rule today: every
// reference to `.Params.summary` must either be a presence predicate
// or flow through `.RenderString` so backticks in the value render
// as `<code>` rather than ship as literal characters.
//
// Implementation: text/template/parse with parse.SkipFuncCheck — so
// Hugo's funcs (`dict`, `partial`, `printf`, …) parse without
// being declared. The walker descends into every tree the parser
// produced, not just `tree.Root`, because Hugo layouts wrap their
// content in `{{ define "main" }}...{{ end }}` blocks whose bodies
// live in separate trees.
//
// See docs/development/website-config.md for the safe/forbidden
// shape enumeration this package enforces.
package templatecheck

import (
	"fmt"
	"sort"
	"strings"
	"text/template/parse"
)

// Violation records one misuse of `.Params.summary` in a Hugo
// template, located by file path and line within that file.
type Violation struct {
	Path string
	Line int
	Why  string
}

// Scan parses one Hugo template (`content`) and returns every
// `.Params.summary` reference that violates the rule documented
// on the package. `path` is used only to populate Violation.Path
// for diagnostics; the function does not touch the filesystem.
//
// Safe shapes:
//   - `if` / `else if` predicate, including compound forms
//     (`if and ...`, `if or ...`) and subfield access
//     (`if .Params.summary.HTML`).
//   - Argument to a `.RenderString` call — positional, piped, or
//     nested inside a sub-pipeline whose output flows into
//     `.RenderString`. Qualified receivers like `$.RenderString`
//     and `.Page.RenderString` are recognised.
//
// Forbidden shapes:
//   - `with` / `else with .Params.summary` — the body rebinds the
//     dot and emits the value raw.
//   - `range .Params.summary` — iterates the string rune-by-rune
//     and emits each code point as an integer.
//   - `template`/`block` invocations passing the summary as the
//     sub-template's dot.
//   - Variable assignment binding the summary to a name, including
//     the `if $s := .Params.summary` form.
//   - Any value-emitting action whose pipe references the summary
//     without reaching a `.RenderString` call.
//
// Field-name comparisons are case-insensitive to match Hugo's
// case-insensitive Params map.
func Scan(path, content string) ([]Violation, error) {
	tree := parse.New(path)
	tree.Mode = parse.SkipFuncCheck
	treeSet := map[string]*parse.Tree{}
	if _, err := tree.Parse(content, "{{", "}}", treeSet); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	w := &walker{path: path, content: content}
	// Hugo layouts use `{{ define "main" }}...{{ end }}` blocks; the
	// body of each define is parsed into a separate tree in treeSet,
	// so walking only `tree.Root` would miss every define's content.
	// Sort keys so violation order is deterministic across runs —
	// map range is randomised and would otherwise shuffle diagnostics.
	keys := make([]string, 0, len(treeSet))
	for k := range treeSet {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		t := treeSet[k]
		if t != nil && t.Root != nil {
			w.walk(t.Root)
		}
	}
	return w.violations, nil
}

type walker struct {
	path       string
	content    string
	violations []Violation
}

func (w *walker) lineOf(pos parse.Pos) int {
	off := int(pos)
	if off > len(w.content) {
		off = len(w.content)
	}
	return 1 + strings.Count(w.content[:off], "\n")
}

func (w *walker) add(pos parse.Pos, why string) {
	w.violations = append(w.violations, Violation{
		Path: w.path,
		Line: w.lineOf(pos),
		Why:  why,
	})
}

func (w *walker) walk(n parse.Node) {
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
	case *parse.TemplateNode:
		w.checkTemplate(n)
	}
}

func (w *walker) checkAction(n *parse.ActionNode) {
	if pipeAssignsSummary(n.Pipe) {
		w.add(n.Pos, "variable assignment of .Params.summary — pass the value directly to .RenderString")
		return
	}
	if pipeReferencesSummary(n.Pipe) && !pipeOutputsSummaryViaRenderString(n.Pipe) {
		w.add(n.Pos, ".Params.summary referenced in a value-emitting action that does not pass it to .RenderString")
	}
}

func (w *walker) checkBranch(p *parse.PipeNode, pos parse.Pos, keyword string) {
	if pipeAssignsSummary(p) {
		w.add(pos, "variable assignment of .Params.summary in `"+keyword+
			"` predicate — the bound name escapes the per-action check")
	}
}

func (w *walker) checkWith(n *parse.WithNode) {
	if pipeAssignsSummary(n.Pipe) {
		w.add(n.Pos, "variable assignment of .Params.summary in `with` predicate — "+
			"the bound name escapes the per-action check")
		return
	}
	if pipeReferencesSummary(n.Pipe) {
		w.add(n.Pos, "`with .Params.summary` rebinds the dot and the body emits the value raw")
	}
}

func (w *walker) checkRange(n *parse.RangeNode) {
	if pipeAssignsSummary(n.Pipe) {
		w.add(n.Pos, "variable assignment of .Params.summary in `range` predicate — "+
			"iterating a string rebinds the dot to each rune")
		return
	}
	if pipeReferencesSummary(n.Pipe) {
		w.add(n.Pos, "`range .Params.summary` iterates the string rune-by-rune and emits each code point as an integer")
	}
}

// checkTemplate flags `{{ template "name" pipe }}` (and `{{ block
// "name" pipe }}` shorthand) invocations whose pipe carries the
// summary as the sub-template's dot. The sub-template's body sees
// `.` as the bound value rather than `.Params.summary`; the
// scanner cannot follow that rebinding, the same problem `with`
// has across a tree boundary.
func (w *walker) checkTemplate(n *parse.TemplateNode) {
	if pipeReferencesSummary(n.Pipe) {
		w.add(n.Pos, "`template` invocation passes .Params.summary as the sub-template's dot — "+
			"the sub-template cannot be scanned through the rebinding; render the summary in this template instead")
	}
}

// identsReferenceSummary reports whether an identifier chain
// contains an adjacent `Params, summary` pair. Matches the bare
// `.Params.summary`, qualified `.Page.Params.summary`, dollar-
// context `$.Params.summary`, and subfield `.Params.summary.HTML`.
// Comparison is case-insensitive because Hugo's Params map is.
func identsReferenceSummary(idents []string) bool {
	for i := 0; i+1 < len(idents); i++ {
		if strings.EqualFold(idents[i], "Params") &&
			strings.EqualFold(idents[i+1], "summary") {
			return true
		}
	}
	return false
}

func fieldIsSummary(f *parse.FieldNode) bool       { return identsReferenceSummary(f.Ident) }
func chainIsSummary(c *parse.ChainNode) bool       { return identsReferenceSummary(c.Field) }
func variableIsSummary(v *parse.VariableNode) bool { return identsReferenceSummary(v.Ident) }

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
		// ChainNode is `<receiver>.Field1...`. Summary may live in
		// the Field chain or hide in the parenthesised receiver
		// (`(.Params.summary).Foo` — summary lives in the PipeNode
		// receiver, not the trailing Field).
		if chainIsSummary(n) {
			return true
		}
		return argReferencesSummary(n.Node)
	case *parse.VariableNode:
		return variableIsSummary(n)
	case *parse.PipeNode:
		return pipeReferencesSummary(n)
	}
	return false
}

// pipeAssignsSummary returns true if the pipe — or any sub-pipe
// nested inside one of its command args — declares variables
// whose right-hand value references the summary. The recursion
// catches forms like `{{ .RenderString (dict) ($s := .Params.summary) }}`
// where the binding hides in a sub-pipeline arg and the outer
// pipe's Decl is empty.
func pipeAssignsSummary(p *parse.PipeNode) bool {
	if p == nil {
		return false
	}
	if len(p.Decl) > 0 && pipeReferencesSummary(p) {
		return true
	}
	for _, c := range p.Cmds {
		for _, arg := range c.Args {
			if sub, ok := arg.(*parse.PipeNode); ok {
				if pipeAssignsSummary(sub) {
					return true
				}
			}
		}
	}
	return false
}

// pipeOutputsSummaryViaRenderString walks the pipe stage-by-stage
// and returns true if `.Params.summary` reaches a `.RenderString`
// call somewhere in the chain — as a direct positional argument,
// inside a sub-pipeline argument, or piped in from an earlier
// stage. Subsequent post-render filters (`| plainify`, `| safeHTML`)
// are fine because the Markdown rendering has already happened.
func pipeOutputsSummaryViaRenderString(p *parse.PipeNode) bool {
	if p == nil || len(p.Cmds) == 0 {
		return false
	}
	summaryFlowing := false
	for i, cmd := range p.Cmds {
		if cmdIsRenderString(cmd) {
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
		// First-stage value heads (e.g. `.Params.summary | ...`)
		// have the value in Args[0]; later stages put the function
		// in Args[0] and positional args in Args[1:].
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
// first arg) ends in `RenderString`. Accepts the canonical
// `.RenderString` (FieldNode), qualified `.Page.RenderString`
// (multi-Ident FieldNode), the dollar-context `$.RenderString`
// (VariableNode with Ident `["$", "RenderString"]`), and chain
// receivers (ChainNode).
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
