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
// ([if cond {x}, if !cond {y}][0]), and the standard library
// (e.g. strings.Join). All frontmatter fields are visible at
// the top-level scope, so an expression body like
//
//	"\(id) - \(name)"
//
// resolves \(id) and \(name) against the corresponding
// frontmatter keys.
package cuetemplate

import (
	"encoding/json"
	"fmt"
	"regexp"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/parser"
)

// identRE matches a frontmatter key safe to emit as a bare
// CUE identifier. Keys that fail the match are emitted in
// quoted form ("my-key": ...); the user can still reach them
// via CUE's quoted-label reference syntax (\("my-key")).
var identRE = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*$`)

// outField is the synthetic field name used to hold the
// compiled expression's result. No leading underscore: hidden
// fields are not reachable via LookupPath. The name is
// deliberately unlikely to collide with a real frontmatter
// key.
const outField = "mdsmithTemplateOut"

// Template is a syntactically validated CUE expression body,
// ready to evaluate against successive frontmatter maps.
type Template struct {
	expr string
}

// Compile parses the expression syntactically and returns a
// Template. Compile only checks CUE syntax; unresolved
// references and non-string results surface from Render
// against a specific frontmatter map.
func Compile(expr string) (*Template, error) {
	if expr == "" {
		return nil, fmt.Errorf("empty CUE expression")
	}
	if _, err := parser.ParseFile("expr",
		fmt.Sprintf("%s: %s", outField, expr)); err != nil {
		return nil, fmt.Errorf("invalid CUE expression: %w", err)
	}
	return &Template{expr: expr}, nil
}

// Render evaluates the compiled expression with fm exposed at
// the top-level scope and returns the result as a string. The
// result must be a CUE string; any other concrete type is an
// error.
func (t *Template) Render(fm map[string]any) (string, error) {
	if fm == nil {
		fm = map[string]any{}
	}
	src := buildSource(fm, t.expr)
	val := cuecontext.New().CompileString(src)
	if err := val.Err(); err != nil {
		return "", fmt.Errorf("evaluating CUE expression: %w", err)
	}
	out := val.LookupPath(cue.ParsePath(outField))
	if out.Kind() != cue.StringKind {
		return "", fmt.Errorf(
			"CUE expression must evaluate to a string, got %s",
			out.Kind())
	}
	s, _ := out.String()
	return s, nil
}

// buildSource assembles the CUE source: a strings import with
// a sink field that satisfies "imported and not used", one
// top-level field per frontmatter key, and the synthetic
// outField holding the user's expression. Frontmatter values
// are encoded via JSON (a syntactic subset of CUE) so nested
// lists and maps reach the expression scope unchanged.
//
// JSON marshalling is infallible for the value shapes
// produced by the YAML frontmatter loader (string, bool,
// int, float, nil, slices, and maps of those), so any
// encoding failure here would indicate a programming bug
// upstream and the panic is the correct response.
func buildSource(fm map[string]any, expr string) string {
	var src []byte
	src = append(src, []byte(
		"import \"strings\"\n\n"+
			"_strings_used: strings.Join([], \"\")\n")...)
	for k, v := range fm {
		jb, err := json.Marshal(v)
		if err != nil {
			panic(fmt.Errorf("cuetemplate: encoding frontmatter %q: %w", k, err))
		}
		var label string
		if identRE.MatchString(k) {
			label = k
		} else {
			label = fmt.Sprintf("%q", k)
		}
		src = append(src, []byte(label+": ")...)
		src = append(src, jb...)
		src = append(src, '\n')
	}
	src = append(src, []byte(fmt.Sprintf("%s: %s\n",
		outField, expr))...)
	return string(src)
}
