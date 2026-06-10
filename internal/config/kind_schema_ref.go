package config

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// schemaNameRE is the pattern a named `schema:` reference must match.
// It is the same shape a schema file's basename carries (a lowercase
// identifier usable as a registry key), so an inline `schema: foo`
// reference and a `.mdsmith/schemas/foo.yaml` file name the same
// entry. Kept identical to kindFileBasenameRE /
// conventionFileBasenameRE on purpose.
var schemaNameRE = kindFileBasenameRE

// KindSchemaRef is a kind's `schema:` value. It is polymorphic: a
// scalar names an entry in the top-level `schemas:` registry (Name is
// set, the body is resolved later at load time), while a mapping is an
// inline schema body carried directly on the kind. A null value (or a
// kind that omits `schema:`) decodes to the zero ref — neither named
// nor inline.
//
// The resolved body is read through Map(); never touch the unexported
// field directly outside this file so the zero value stays safe to
// query. SourcePath records where the schema itself was defined (a
// `.mdsmith/schemas/<name>.yaml` path for a file entry, `.mdsmith.yml`
// for an inline-registry entry, empty for an inline-on-kind body — the
// kind's own file then applies). It is filled by resolveNamedSchemas
// for named refs and left empty for inline bodies. See plan 241.
type KindSchemaRef struct {
	// Name is the registry key when the kind referenced a schema by
	// name (`schema: rfc-v1`). Empty for an inline body.
	Name string

	// SourcePath is the workspace-absolute path of the file that
	// defined the resolved schema. Set at resolution for a named ref;
	// empty for an inline-on-kind body (the kind's file applies).
	SourcePath string

	// inline holds the schema body — either an inline map decoded
	// straight off the kind or the registry body filled in by
	// resolveNamedSchemas. Read it via Map().
	inline map[string]any
}

// Map returns the resolved inline schema body, or nil when the ref
// carries none yet (an unresolved named ref, a null value, or the zero
// ref). Safe to call on a zero-value ref.
func (r KindSchemaRef) Map() map[string]any {
	return r.inline
}

// inlineSchemaRef builds a KindSchemaRef that carries an inline body
// directly (no registry name). It is the constructor the merge layer
// and in-package tests use; UnmarshalYAML is the decode-time
// counterpart.
func inlineSchemaRef(m map[string]any) KindSchemaRef {
	return KindSchemaRef{inline: m}
}

// InlineSchema builds a KindSchemaRef carrying an inline body, for
// callers outside this package that construct a KindBody directly
// (chiefly tests in internal/kindsout and friends, which cannot reach
// the unexported field). Equivalent to a kind whose `schema:` is a
// YAML mapping.
func InlineSchema(m map[string]any) KindSchemaRef {
	return KindSchemaRef{inline: m}
}

// InlineSchemaWithSource builds a KindSchemaRef as if a named reference
// had resolved to a body from a registry entry at sourcePath, for
// out-of-package callers (chiefly kindsout tests exercising the
// schema-source-path surface). It mirrors the internal resolvedSchemaRef
// produced by resolveNamedSchemas.
func InlineSchemaWithSource(name string, m map[string]any, sourcePath string) KindSchemaRef {
	return KindSchemaRef{Name: name, SourcePath: sourcePath, inline: m}
}

// resolvedSchemaRef builds a KindSchemaRef for a named reference whose
// body has been looked up in the registry. The original Name is kept
// so audit surfaces can still report the reference, and SourcePath
// records where the schema was defined.
func resolvedSchemaRef(name string, body map[string]any, sourcePath string) KindSchemaRef {
	return KindSchemaRef{Name: name, SourcePath: sourcePath, inline: body}
}

// UnmarshalYAML dispatches on the YAML node kind so a kind's `schema:`
// value can be either a registry name (scalar) or an inline body
// (mapping):
//
//   - ScalarNode → a named reference. A null scalar leaves the ref
//     empty (the kind declares no schema). Any other scalar must match
//     schemaNameRE; an empty or malformed name is rejected so a typo
//     surfaces at load rather than silently resolving to nothing.
//   - MappingNode → an inline schema body decoded into the map.
//   - anything else (a sequence, most notably) → an error: a schema is
//     a name or a mapping, never a list.
func (r *KindSchemaRef) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		// A null scalar (`schema:` with no value, or `schema: null`)
		// is not a reference and not a body — the kind simply declares
		// no schema. Leave the ref zero.
		if node.Tag == "!!null" {
			return nil
		}
		var name string
		if err := node.Decode(&name); err != nil {
			return fmt.Errorf("schema: %w", err)
		}
		if name == "" {
			return fmt.Errorf("schema: name must not be empty")
		}
		if !schemaNameRE.MatchString(name) {
			return fmt.Errorf(
				"schema: name %q must match %s",
				name, schemaNameRE.String())
		}
		r.Name = name
		return nil
	case yaml.MappingNode:
		var m map[string]any
		if err := node.Decode(&m); err != nil {
			return fmt.Errorf("schema: %w", err)
		}
		r.inline = m
		return nil
	default:
		return fmt.Errorf(
			"schema: must be a registry name or an inline mapping, "+
				"not a %s", nodeKindName(node.Kind))
	}
}

// nodeKindName renders a yaml.Kind for an error message.
func nodeKindName(k yaml.Kind) string {
	switch k {
	case yaml.SequenceNode:
		return "sequence"
	case yaml.MappingNode:
		return "mapping"
	case yaml.ScalarNode:
		return "scalar"
	case yaml.AliasNode:
		return "alias"
	case yaml.DocumentNode:
		return "document"
	default:
		return "value"
	}
}
