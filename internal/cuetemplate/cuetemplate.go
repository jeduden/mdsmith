// Package cuetemplate evaluates a CUE expression against a
// frontmatter map and returns the rendered string. It is the
// templating sibling of internal/query: query.Match unifies a
// CUE struct-literal constraint with frontmatter to produce a
// boolean; cuetemplate.Render evaluates a CUE expression in a
// scope that exposes every frontmatter field by name and
// produces the string value at the result selector.
//
// The expression source uses CUE syntax — string interpolation
// (\(x)), list comprehension ([for m in xs {...}]),
// list-comprehension conditionals
// ([if cond {x}, if !cond {y}][0]), and the row-expression
// builtins strings.Join and len.
//
// Evaluation runs on the in-house cue/cuelite engine (plan 239),
// not cuelang.org/go. Scope:
//
//   - Every frontmatter key binds by its bare name, so a row-expr
//     can write `\(id)`.
//   - The full frontmatter map also binds under the `fm` field,
//     so a key whose name is not a valid CUE identifier is
//     reachable as `fm["my-key"]`, and any key is reachable as
//     `fm.id`.
//   - `strings.Join` and `len` are builtins the evaluator
//     provides; no preimport is needed. A frontmatter key named
//     `strings` is reachable as `fm.strings` — the bare `strings`
//     identifier is the builtin namespace.
package cuetemplate

import (
	"fmt"

	"github.com/jeduden/mdsmith/cue/cuelite"
)

// Template is a syntactically validated row expression, ready to
// evaluate against successive frontmatter maps. The parsed
// expression is cached at Compile and reused across Render calls;
// the in-house engine is context-free, so a Template is safe to
// reuse across a catalog of many matched files with no
// per-Render allocation of a fresh evaluation context.
type Template struct {
	row *cuelite.RowTemplate
}

// Compile parses the expression syntactically and returns a
// Template. Compile only checks CUE syntax; unresolved
// references and non-string results surface from Render against a
// specific frontmatter map.
func Compile(expr string) (*Template, error) {
	if expr == "" {
		return nil, fmt.Errorf("empty cue expression")
	}
	row, err := cuelite.CompileRow(expr)
	if err != nil {
		return nil, fmt.Errorf("invalid cue expression: %w", err)
	}
	return &Template{row: row}, nil
}

// Render evaluates the compiled expression against fm and
// returns the result as a string. fm is exposed both as the
// `fm` struct and as top-level bindings for each key. The result
// must be a concrete CUE string; any other shape — a number, a
// bool, a list, a struct, or a missing-field reference — is an
// error so a row-expr never silently emits a blank cell. A nil fm
// is treated as an empty map.
func (t *Template) Render(fm map[string]any) (string, error) {
	return t.row.Render(fm)
}
