package config

import (
	"fmt"
	"strings"

	"github.com/jeduden/mdsmith/internal/schema"
)

// ResolveKindInlineSchema walks the `extends:` chain for the named
// kind and returns a single inline schema map that is the merge of
// every layer, parent-first. The kinds map and entry name are
// assumed to have passed ValidateKinds — cycles and undeclared
// parents are detected there and the resolver re-raises them
// defensively when it spots them so callers never silently see a
// truncated chain.
//
// Returns nil when the kind has no inline schema (KindBody.Schema is
// empty) and no parent declares one either. Returns the kind's own
// schema unchanged when no `extends:` is set, so the existing
// single-kind merge path stays byte-equivalent for non-inheriting
// kinds.
//
// Conflicts in the unified frontmatter (a child whose CUE expression
// cannot unify with the parent's) surface as an
// extendsChainConflictError naming the offending child's kind, the
// parent's kind, and both expressions. The caller (config validate
// or merge) wraps it with the source location.
func ResolveKindInlineSchema(
	kinds map[string]KindBody, name string,
) (map[string]any, error) {
	chain, err := extendsChainSchemas(kinds, name)
	if err != nil {
		return nil, err
	}
	if len(chain) == 0 {
		return nil, nil
	}
	if len(chain) == 1 {
		return chain[0].raw, nil
	}
	merged := chain[0].raw
	parentName := chain[0].kind
	for _, c := range chain[1:] {
		next, err := schema.MergeRawMap(merged, c.raw)
		if err != nil {
			return nil, fmt.Errorf(
				"kind %q (extends %s): %w",
				c.kind, parentName, err)
		}
		merged = next
		parentName = c.kind
	}
	return merged, nil
}

// schemaChainEntry pairs a kind name with the raw inline schema it
// declared. The chain is the resolved sequence of layers used for
// extending, in parent-to-child order. Kinds with no inline schema
// drop out of the chain so they don't contribute an empty layer.
type schemaChainEntry struct {
	kind string
	raw  map[string]any
}

// extendsChainSchemas walks the chain from child up to root, then
// reverses it so the caller can fold parent → child. Each layer's
// inline schema is reported verbatim; layers without an inline
// schema are filtered out so they never produce empty intermediate
// merges. Cycles and undeclared parents are re-detected
// defensively — ValidateKinds is the authoritative gate, but a
// caller that mutates `kinds` between validate and resolve would
// otherwise silently hang.
func extendsChainSchemas(
	kinds map[string]KindBody, name string,
) ([]schemaChainEntry, error) {
	visited := map[string]bool{}
	chain := []string{}
	current := name
	for current != "" {
		if visited[current] {
			chain = append(chain, current)
			return nil, fmt.Errorf(
				"kind %q: extends cycle detected: %s",
				name, strings.Join(chain, " -> "))
		}
		visited[current] = true
		chain = append(chain, current)
		body, ok := kinds[current]
		if !ok {
			return nil, fmt.Errorf(
				"kind %q: extends references undeclared kind %q",
				name, current)
		}
		current = body.Extends
	}
	// Walk root → child so MergeRawMap sees parent then child.
	out := make([]schemaChainEntry, 0, len(chain))
	for i := len(chain) - 1; i >= 0; i-- {
		kind := chain[i]
		body := kinds[kind]
		if len(body.Schema) == 0 {
			continue
		}
		out = append(out, schemaChainEntry{kind: kind, raw: body.Schema})
	}
	return out, nil
}

// KindExtendsChain returns the chain of kind names from `name` up
// to its furthest ancestor, child-first. A kind without `extends:`
// returns a one-element slice containing just its own name. The
// chain is the inheritance audit list `mdsmith kinds show` prints.
func KindExtendsChain(kinds map[string]KindBody, name string) []string {
	visited := map[string]bool{}
	out := []string{}
	current := name
	for current != "" {
		if visited[current] {
			return out
		}
		visited[current] = true
		out = append(out, current)
		body, ok := kinds[current]
		if !ok {
			return out
		}
		current = body.Extends
	}
	return out
}
