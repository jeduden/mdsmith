// Package yamlutil provides safe YAML parsing and marshaling helpers.
//
// All user-supplied content (config files, front matter, directive parameters)
// must pass through [UnmarshalSafe] or [UnmarshalNodeSafe] rather than calling
// yaml.Unmarshal directly. These wrappers call [RejectYAMLAliases] first, which
// prevents billion-laughs denial-of-service attacks by refusing any YAML that
// contains anchors or aliases before the alias expansion happens.
//
// When to use each function:
//   - [UnmarshalSafe] — unmarshal user content into a Go struct or map.
//   - [UnmarshalNodeSafe] — unmarshal user content into a raw yaml.Node tree
//     (needed when inspecting YAML structure before decoding into typed values).
//   - [Marshal] — thin wrapper around yaml.Marshal for consistency; safe for
//     output marshaling where data originates from trusted Go values.
//
// One escape hatch is allowed: call [RejectYAMLAliases] directly, followed by
// a raw decode, when the wrappers cannot express the decode — a strict
// KnownFields decoder (kind and convention files), per-error-type diagnostics
// (required-structure front matter), or parse errors that must defer to a
// later [UnmarshalSafe] on the same bytes (the config convention pre-check).
// Every such site keeps the pre-check directly above its decode.
//
// See docs/security/2026-04-05-adversarial-markdown.md for threat model context.
package yamlutil

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"gopkg.in/yaml.v3"
)

// RejectYAMLAliases decodes YAML into a node tree and returns an error if any
// anchor or alias is found. Decoding into yaml.Node does not expand aliases,
// so this is safe even for billion-laughs payloads. Non-anchor syntax errors
// return nil (handled by the caller's yaml.Unmarshal). This check must be
// called before yaml.Unmarshal on user-supplied content.
func RejectYAMLAliases(data []byte) error {
	if len(data) == 0 {
		return nil
	}

	dec := yaml.NewDecoder(bytes.NewReader(data))
	for {
		var doc yaml.Node
		err := dec.Decode(&doc)
		if err == io.EOF {
			return nil
		}
		if err != nil {
			// An undefined alias causes a parse error containing "unknown anchor".
			// Reject this as evidence of alias usage.
			if strings.Contains(err.Error(), "unknown anchor") {
				return fmt.Errorf("yaml anchors/aliases are not permitted")
			}
			// Other syntax errors are handled by the caller's yaml.Unmarshal.
			return nil
		}

		if hasYAMLAnchorOrAlias(&doc) {
			return fmt.Errorf("yaml anchors/aliases are not permitted")
		}
	}
}

// UnmarshalSafe rejects YAML anchors/aliases then unmarshals data into v.
// Use this for all user-supplied YAML content (config files, front matter,
// directive parameters).
func UnmarshalSafe(data []byte, v any) error {
	if err := RejectYAMLAliases(data); err != nil {
		return err
	}
	return yaml.Unmarshal(data, v)
}

// UnmarshalNodeSafe rejects YAML anchors/aliases then unmarshals data into a
// yaml.Node document node. Use this when raw node inspection is needed before
// typed decoding (e.g. checking top-level key presence or tag types).
func UnmarshalNodeSafe(data []byte) (yaml.Node, error) {
	if err := RejectYAMLAliases(data); err != nil {
		return yaml.Node{}, err
	}
	var node yaml.Node
	if err := yaml.Unmarshal(data, &node); err != nil {
		return yaml.Node{}, err
	}
	return node, nil
}

// Marshal is a thin wrapper around yaml.Marshal for consistency with
// UnmarshalSafe. Safe for output marshaling where data originates from
// trusted Go values.
func Marshal(v any) ([]byte, error) {
	return yaml.Marshal(v)
}

// TopLevelMappingLines walks the yaml.Node document produced by
// UnmarshalNodeSafe (or a direct yaml.Unmarshal into a yaml.Node)
// and returns a map from each top-level scalar mapping key to its
// source line, shifted by lineOffset.
//
// yaml.v3 nests the user's mapping inside a single-document node;
// the helper skips past that layer to the content of interest.
// A nil node, missing content, or a non-mapping root all return
// nil so callers degrade to a "no per-key line known" fallback
// rather than panicking. Non-scalar mapping keys (YAML's
// explicit `?` syntax with a sequence or mapping as the key)
// are skipped silently — the remaining scalar keys still appear
// in the returned map, which may therefore be empty if every
// key is non-scalar.
//
// Two callers want this: internal/schema parses a proto.md
// frontmatter and the requiredstructure rule parses its legacy
// schema frontmatter. Centralising the walk here means future
// YAML edge cases (documents, anchors, etc.) only need fixing in
// one place — see the Copilot review on PR #284.
func TopLevelMappingLines(doc *yaml.Node, lineOffset int) map[string]int {
	if doc == nil || len(doc.Content) == 0 {
		return nil
	}
	root := doc.Content[0]
	if root == nil || root.Kind != yaml.MappingNode {
		return nil
	}
	out := make(map[string]int, len(root.Content)/2)
	for i := 0; i+1 < len(root.Content); i += 2 {
		k := root.Content[i]
		if k.Kind != yaml.ScalarNode {
			continue
		}
		out[k.Value] = k.Line + lineOffset
	}
	return out
}

func hasYAMLAnchorOrAlias(node *yaml.Node) bool {
	if node.Anchor != "" || node.Kind == yaml.AliasNode {
		return true
	}
	for _, child := range node.Content {
		if hasYAMLAnchorOrAlias(child) {
			return true
		}
	}
	return false
}
