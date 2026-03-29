package query

import (
	"encoding/json"
	"fmt"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
)

// Matcher holds a pre-compiled CUE expression for matching
// against front matter maps.
type Matcher struct {
	schema cue.Value
	paths  []cue.Path // leaf field paths required by the expression
}

// Compile parses a CUE struct literal body and returns a
// Matcher. Returns an error if the expression is invalid.
func Compile(expr string) (*Matcher, error) {
	ctx := cuecontext.New()
	// Wrap the expression body in braces to form a struct literal.
	val := ctx.CompileString("{" + expr + "}")
	if err := val.Err(); err != nil {
		return nil, fmt.Errorf("invalid CUE expression: %w", err)
	}
	paths := collectPaths(val, nil)
	return &Matcher{schema: val, paths: paths}, nil
}

// collectPaths recursively collects all leaf field paths from a CUE
// value, so Match can verify they exist in front matter data before
// unification. This handles nested struct expressions like
// `meta: { status: "✅" }`.
func collectPaths(v cue.Value, prefix []cue.Selector) []cue.Path {
	var paths []cue.Path
	iter, err := v.Fields()
	if err != nil {
		return nil
	}
	for iter.Next() {
		cur := append(append([]cue.Selector{}, prefix...), iter.Selector())
		child := iter.Value()
		// If the child is a struct with fields, recurse into it.
		childIter, err := child.Fields()
		if err == nil && childIter.Next() {
			paths = append(paths, collectPaths(child, cur)...)
		} else {
			paths = append(paths, cue.MakePath(cur...))
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
	data, err := json.Marshal(fm)
	if err != nil {
		return false
	}
	dataVal := m.schema.Context().CompileBytes(data)
	if dataVal.Err() != nil {
		return false
	}
	// Require that every field path in the expression exists in the
	// data. CUE structs are open by default, so unification alone
	// would accept data missing a field by filling it from the schema.
	for _, p := range m.paths {
		if !dataVal.LookupPath(p).Exists() {
			return false
		}
	}
	merged := m.schema.Unify(dataVal)
	return merged.Validate(cue.Concrete(true)) == nil
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
