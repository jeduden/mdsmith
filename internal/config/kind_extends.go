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
// Merge is structural only. Frontmatter constraints from both
// layers are joined with CUE `&`, but the result is not CUE-eval'd
// — call ValidateKindInlineSchema (or rely on ValidateKinds, which
// runs it at load time) to detect unsatisfiable conjunctions like
// `int & string`. Splitting the two passes lets effectiveRules
// call this resolver per file without re-running the CUE checker.
//
// Returns nil when the kind has no inline schema (KindBody.Schema is
// empty) and no parent declares one either. Returns the kind's own
// schema unchanged when no `extends:` is set, so the existing
// single-kind merge path stays byte-equivalent for non-inheriting
// kinds.
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
	for _, c := range chain[1:] {
		// MergeRawMap is structural-only after the plan-135 split:
		// it always returns a nil error and we never reach the err
		// branch from here, so the merge call doesn't propagate one.
		next, _ := schema.MergeRawMap(merged, c.raw)
		merged = next
	}
	return merged, nil
}

// ValidateKindInlineSchema resolves the extends chain for the named
// kind and CUE-checks the merged frontmatter expressions, returning
// an error if any unified expression is unsatisfiable. The error
// names the conflicting key and both layer expressions. Wrapped
// with the kind name and parent context so the diagnostic carries
// the full extends-chain trail (plan 135 / plan 147 shape).
//
// Returns nil when the kind has no extends chain (validation is a
// no-op for non-inheriting kinds) and when every unified
// expression compiles cleanly.
func ValidateKindInlineSchema(
	kinds map[string]KindBody, name string,
) error {
	resolved, err := ResolveKindInlineSchema(kinds, name)
	if err != nil {
		return err
	}
	if resolved == nil {
		return nil
	}
	if err := schema.ValidateExtendedFrontmatter(resolved); err != nil {
		return fmt.Errorf("kind %q: %w", name, err)
	}
	return nil
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
