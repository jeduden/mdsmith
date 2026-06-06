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
// (e.g. strings.Join).
//
// Scope. Each Render call emits a CUE source file with three
// layers visible to the user expression:
//
//   - The full frontmatter map under the `fm` field. Reference
//     any key via `fm.id` (identifier-safe names) or
//     `fm["my-key"]` (any name, including hyphens and dots).
//   - Top-level aliases for each frontmatter key whose name is
//     a valid CUE identifier and does not collide with a
//     reserved keyword or the `strings` import. So
//     "\(id)" works the same as "\(fm.id)".
//   - The `strings` standard-library package, preimported.
//     A frontmatter key named `strings` is reachable via
//     `fm.strings` only; the bare `strings` identifier always
//     resolves to the import so `strings.Join(...)` keeps
//     working regardless of frontmatter contents.
package cuetemplate

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"cuelang.org/go/cue/parser"
)

// identRE matches a frontmatter key safe to emit as a bare
// CUE identifier alias at the file's top-level scope.
var identRE = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*$`)

// reservedAliases lists names that must not be aliased at
// the file's top-level scope. CUE keywords are syntactically
// reserved as identifiers; `strings` is reserved because it
// is the preimported package and shadowing it would break
// `strings.Join` in user expressions; `fm` and `outField`
// name the renderer's own scaffolding. Frontmatter keys that
// collide with these are still reachable through the `fm`
// struct (e.g. `fm.strings`).
//
// CUE operator keywords (`div`, `mod`, `quo`, `rem`) are
// deliberately omitted — they are operators in expression
// position only and parse legally as labels in the
// `<key>: fm.<key>` alias emission.
var reservedAliases = map[string]bool{
	"package": true, "import": true, "for": true, "in": true,
	"if": true, "let": true, "true": true, "false": true,
	"null": true, "_": true, "strings": true,
	outField: true, fmField: true,
}

// outField is the synthetic field name used to hold the
// compiled expression's result. No leading underscore: hidden
// fields are not reachable via LookupPath. The name is
// deliberately unlikely to collide with a real frontmatter
// key. Snake-case matches the rest of the synthetic
// identifiers in this file (`_strings_used`, `fm`).
const outField = "mdsmith_template_out"

// fmField is the name of the struct that holds the full
// frontmatter map, indexable by any key (including those that
// are not valid CUE identifiers).
const fmField = "fm"

// Template is a syntactically validated CUE expression body,
// ready to evaluate against successive frontmatter maps. The
// CUE context is created once at Compile and reused across
// Render calls — cuelang's *cue.Context is safe for
// concurrent use and reusing it avoids the per-Render
// allocation when the same Template is applied to a catalog
// of many matched files.
type Template struct {
	expr string
	ctx  *cue.Context
}

// Compile parses the expression syntactically and returns a
// Template. Compile only checks CUE syntax; unresolved
// references and non-string results surface from Render
// against a specific frontmatter map.
func Compile(expr string) (*Template, error) {
	if expr == "" {
		return nil, fmt.Errorf("empty cue expression")
	}
	if _, err := parser.ParseFile("expr",
		fmt.Sprintf("%s: %s", outField, expr)); err != nil {
		return nil, fmt.Errorf("invalid cue expression: %w", err)
	}
	return &Template{expr: expr, ctx: cuecontext.New()}, nil
}

// Render evaluates the compiled expression against fm and
// returns the result as a string. fm is exposed both as the
// `fm` struct and as top-level aliases for each
// identifier-safe non-reserved key. The result must be a
// concrete CUE string; any other shape is an error.
func (t *Template) Render(fm map[string]any) (string, error) {
	if fm == nil {
		fm = map[string]any{}
	}
	src := buildSource(fm, t.expr)
	val := t.ctx.CompileString(src)
	if err := val.Err(); err != nil {
		return "", fmt.Errorf("evaluating cue expression: %w", err)
	}
	// out.String errors on both wrong-kind values (int, bool,
	// list, struct) and string-typed-but-non-concrete values
	// (the unevaluated `string` type, an open `"a" | "b"`
	// disjunction). The error message from CUE already names
	// the offending shape; wrap it so row-expr never silently
	// emits a blank cell.
	s, err := val.LookupPath(cue.ParsePath(outField)).String()
	if err != nil {
		return "", fmt.Errorf(
			"cue expression must yield a concrete string: %w", err)
	}
	return s, nil
}

// buildSource assembles the CUE source. The user's expression
// runs in a scope with:
//
//   - `import "strings"` and a sink field so the import is
//     "used".
//   - `fm: { ... }` carrying the full frontmatter as JSON.
//   - One top-level alias `<key>: fm.<key>` per
//     identifier-safe non-reserved frontmatter key, so a
//     row-expr can write `\(id)` instead of `\(fm.id)`.
//   - The synthetic outField holding the user's expression.
//
// JSON marshalling is infallible for the value shapes
// produced by the YAML frontmatter loader, so any encoding
// failure indicates a programming bug upstream and the panic
// is the correct response.
func buildSource(fm map[string]any, expr string) string {
	// Single-pass filter: drop the renderer's synthetic field
	// names from the JSON-emitted map AND collect the sorted
	// alias keys in the same walk. Both surfaces use the same
	// predicate so the JSON struct and the alias list cannot
	// drift apart: a key that shadows `fm` or `outField` is
	// dropped from both; a key that collides with `reservedAliases`
	// is in the JSON but not aliased at top level (reachable
	// via `fm["strings"]` etc.).
	emit := make(map[string]any, len(fm))
	aliasKeys := make([]string, 0, len(fm))
	for k, v := range fm {
		if k == fmField || k == outField {
			continue
		}
		emit[k] = v
		if identRE.MatchString(k) && !reservedAliases[k] {
			aliasKeys = append(aliasKeys, k)
		}
	}
	sort.Strings(aliasKeys)

	fmJSON, err := json.Marshal(emit)
	if err != nil {
		panic(fmt.Errorf("cuetemplate: encoding frontmatter: %w", err))
	}
	src := make([]byte, 0, len(fmJSON)+128)
	src = append(src, []byte(
		"import \"strings\"\n\n"+
			"_strings_used: strings.Join([], \"\")\n")...)
	src = append(src, []byte(fmField+": ")...)
	src = append(src, fmJSON...)
	src = append(src, '\n')
	for _, k := range aliasKeys {
		src = append(src, []byte(fmt.Sprintf("%s: %s.%s\n",
			k, fmField, k))...)
	}
	src = append(src, []byte(fmt.Sprintf("%s: %s\n",
		outField, expr))...)
	return string(src)
}
