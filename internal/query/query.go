package query

import (
	"fmt"

	"github.com/jeduden/mdsmith/cue/cuelite"
)

// Matcher holds a pre-compiled CUE expression for matching
// against front matter maps.
type Matcher struct {
	schema cuelite.Value
	paths  []cuelite.Path // leaf field paths required by the expression
}

// Compile parses a CUE struct literal body and returns a
// Matcher. Returns an error if the expression is invalid.
func Compile(expr string) (*Matcher, error) {
	// Wrap the expression body in braces to form a struct literal.
	val, err := cuelite.Compile("{" + expr + "}")
	if err != nil {
		return nil, fmt.Errorf("invalid CUE expression: %w", err)
	}
	paths := collectPaths(val, nil)
	return &Matcher{schema: val, paths: paths}, nil
}

// collectPaths recursively collects all leaf field paths from a CUE
// value, so Match can verify they exist in front matter data before
// unification. This handles nested struct expressions like
// `meta: { status: "✅" }`.
//
// Paths are built with cuelite.MakePath from the raw Fields() selectors,
// never ParsePath: a selector is a data key (it may be dotted, hyphenated,
// or otherwise unparseable as a path expression), and MakePath stores it
// verbatim so the later LookupPath finds it.
func collectPaths(v cuelite.Value, prefix []string) []cuelite.Path {
	var paths []cuelite.Path
	for _, f := range v.Fields() {
		cur := append(append([]string{}, prefix...), f.Selector)
		// If the child is a struct with fields, recurse into it.
		if len(f.Value.Fields()) > 0 {
			paths = append(paths, collectPaths(f.Value, cur)...)
		} else {
			paths = append(paths, cuelite.MakePath(cur...))
		}
	}
	return paths
}

// Match reports whether fm satisfies the compiled CUE expression.
// A nil or empty map never matches an expression that requires fields.
func (m *Matcher) Match(fm map[string]any) bool {
	if fm == nil {
		fm = map[string]any{}
	}
	// Lift the front-matter map directly into a data Value, no JSON
	// round-trip (plan 218). A bottom (an unsupported value type) never
	// matches.
	dataVal := cuelite.LiftMap(fm)
	if dataVal.Err() != nil {
		return false
	}
	// Require that every field path in the expression exists in the
	// data. CUE structs are open by default, so unification alone
	// would accept data missing a field by filling it from the schema.
	for _, p := range m.paths {
		if _, ok := dataVal.LookupPath(p); !ok {
			return false
		}
	}
	// A context-free Value unifies in either order; the shared compiled
	// schema is the operand and is never mutated.
	return dataVal.Unify(m.schema).Validate() == nil
}

// Match is a convenience function that compiles expr and tests fm
// in a single call. Returns an error only if the CUE expression is
// invalid.
func Match(expr string, fm map[string]any) (bool, error) {
	m, err := Compile(expr)
	if err != nil {
		return false, err
	}
	return m.Match(fm), nil
}
