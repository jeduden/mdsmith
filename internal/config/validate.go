package config

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

// ValidateKinds returns an error if any kind named in a kind-assignment
// entry is not declared in cfg.Kinds, or if any declared kind sets a
// schema both inline (KindBody.Schema) and via the legacy
// rules.required-structure.schema: path. Front-matter kinds are
// validated at lint time via ValidateFrontMatterKinds (see engine).
// It also rejects an `extends:` chain that references an undeclared
// kind or forms a cycle (plan 135).
func ValidateKinds(cfg *Config) error {
	if len(cfg.Kinds) == 0 && len(cfg.KindAssignment) == 0 {
		return nil
	}
	names := make([]string, 0, len(cfg.Kinds))
	for name := range cfg.Kinds {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		body := cfg.Kinds[name]
		if err := validateKindSchemaSources(name, body); err != nil {
			return err
		}
		if err := validateKindPathPattern(name, body); err != nil {
			return err
		}
		if err := validateKindExtends(cfg.Kinds, name); err != nil {
			return err
		}
	}
	// Walk extends chains a second time, now that every chain is
	// known to be well-formed, to catch unsatisfiable frontmatter
	// expressions (e.g. parent says `int`, child says `string`).
	// Running ValidateKindInlineSchema here surfaces those at
	// config-load time rather than as a per-file MDS020 diagnostic
	// later, so users see the conflict immediately on `mdsmith
	// check`. The per-file merge path calls ResolveKindInlineSchema
	// without re-running the CUE check, since we know the chain is
	// already valid by the time effectiveRules runs.
	for _, name := range names {
		body := cfg.Kinds[name]
		if body.Extends == "" {
			continue
		}
		if err := ValidateKindInlineSchema(cfg.Kinds, name); err != nil {
			return err
		}
	}
	for i, entry := range cfg.KindAssignment {
		for _, name := range entry.Kinds {
			if _, ok := cfg.Kinds[name]; !ok {
				return fmt.Errorf(
					"kind-assignment[%d]: references undeclared kind %q", i, name,
				)
			}
		}
	}
	return nil
}

// validateKindExtends walks a kind's `extends:` chain, rejecting an
// undeclared parent and any cycle (single- or multi-hop) by naming
// the cycle path. The check runs per kind so a cycle reported for
// `a` does not silently re-fire for `b` and `c` on the same cycle —
// each kind sees its own canonical entry point. The kind iteration
// order in ValidateKinds is sorted, so the diagnostic stays
// deterministic across runs.
func validateKindExtends(kinds map[string]KindBody, name string) error {
	visited := map[string]bool{}
	chain := []string{}
	current := name
	for current != "" {
		if visited[current] {
			chain = append(chain, current)
			return fmt.Errorf(
				"kind %q: extends cycle detected: %s",
				name, strings.Join(chain, " -> "))
		}
		visited[current] = true
		chain = append(chain, current)
		body, ok := kinds[current]
		if !ok {
			return fmt.Errorf(
				"kind %q: extends references undeclared kind %q",
				name, current)
		}
		current = body.Extends
	}
	return nil
}

// validateKindPathPattern rejects a kind whose top-level
// `path-pattern:` is not a valid doublestar glob. Without this
// check, commands that load config but do not run the
// required-structure rule (e.g. `mdsmith kinds show`) would
// silently accept and display a malformed pattern, and the
// problem would only surface as a per-file rule-configuration
// error at lint time. The matcher is shared with overrides:,
// ignore:, and kind-assignment:; mirroring their syntax keeps
// the user-facing config surface uniform.
func validateKindPathPattern(name string, body KindBody) error {
	if body.PathPattern == "" {
		return nil
	}
	if !doublestar.ValidatePattern(filepath.ToSlash(body.PathPattern)) {
		return fmt.Errorf(
			"kind %q: path-pattern %q is not a valid doublestar glob",
			name, body.PathPattern)
	}
	return nil
}

// validateKindSchemaSources rejects a kind that declares more than
// one schema source. The three forms that conflict pairwise:
//
//   - `kinds.<name>.schema:` (inline block on KindBody.Schema)
//   - `kinds.<name>.rules.required-structure.schema:` (file path)
//   - `kinds.<name>.rules.required-structure.inline-schema:`
//     (inline map under the rule settings)
//
// Any two of these on the same kind make the effective schema
// ambiguous; the validator surfaces the conflict at load time with
// a message naming both sources.
func validateKindSchemaSources(name string, body KindBody) error {
	rsCfg, hasRS := body.Rules["required-structure"]
	pathSet, pathSetting := schemaPathSetting(rsCfg, hasRS)
	inlineSet, _ := schemaInlineSetting(rsCfg, hasRS)

	// The top-level `schema:` is filled in two ways: an inline mapping
	// (Schema.Name empty) or a named registry reference resolved to a
	// body (Schema.Name set). Describe whichever the user wrote so the
	// conflict message points at the right line. Both forms conflict
	// with a file-path or inline-schema source under required-structure.
	if len(body.Schema.Map()) > 0 && pathSet {
		return fmt.Errorf(
			"kind %q: schema is declared both via %s "+
				"and as a file (kinds.%s.rules.required-structure.schema: %q); "+
				"pick one source",
			name, schemaSourceDescription(name, body), name, pathSetting)
	}
	if len(body.Schema.Map()) > 0 && inlineSet {
		return fmt.Errorf(
			"kind %q: schema is declared both via %s "+
				"and under kinds.%s.rules.required-structure.inline-schema:; "+
				"pick one source — keep the top-level kinds.%s.schema: declaration",
			name, schemaSourceDescription(name, body), name, name)
	}
	if pathSet && inlineSet {
		return fmt.Errorf(
			"kind %q: required-structure has both `schema:` (%q) and "+
				"`inline-schema:` set under kinds.%s.rules.required-structure; "+
				"pick one source",
			name, pathSetting, name)
	}
	return nil
}

// schemaSourceDescription renders the kind's top-level `schema:` source
// for a conflict message: a named reference reports the registry name
// it resolved from, an inline mapping reports the bare key. The name
// form lets a user trace a dual-source error back to the `schema: foo`
// line rather than mistaking a registry reference for an inline block.
func schemaSourceDescription(name string, body KindBody) string {
	if body.Schema.Name != "" {
		return fmt.Sprintf("a named reference (kinds.%s.schema: %s)",
			name, body.Schema.Name)
	}
	return fmt.Sprintf("an inline block (kinds.%s.schema:)", name)
}

func schemaPathSetting(rs RuleCfg, hasRS bool) (bool, string) {
	if !hasRS || rs.Settings == nil {
		return false, ""
	}
	v, ok := rs.Settings["schema"]
	if !ok {
		return false, ""
	}
	s, ok := v.(string)
	if !ok || s == "" {
		return false, ""
	}
	return true, s
}

func schemaInlineSetting(rs RuleCfg, hasRS bool) (bool, map[string]any) {
	if !hasRS || rs.Settings == nil {
		return false, nil
	}
	m, ok := rs.Settings["inline-schema"].(map[string]any)
	if !ok || len(m) == 0 {
		return false, nil
	}
	return true, m
}

// ValidateFrontMatterKinds returns an error if any of the supplied front-matter
// kind names is not declared in cfg.Kinds. filePath is used in the message.
func ValidateFrontMatterKinds(cfg *Config, filePath string, kinds []string) error {
	for _, name := range kinds {
		if _, ok := cfg.Kinds[name]; !ok {
			return fmt.Errorf(
				"%s: front matter references undeclared kind %q", filePath, name,
			)
		}
	}
	return nil
}
