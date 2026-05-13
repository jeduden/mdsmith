package lint

import (
	"bytes"
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/jeduden/mdsmith/internal/yamlutil"
)

// StripFrontMatter removes YAML front matter delimited by "---\n"
// from the beginning of source. It returns the front matter block
// (including delimiters) and the remaining content. If no front
// matter is found, prefix is nil and content equals source.
func StripFrontMatter(source []byte) (prefix, content []byte) {
	delim := []byte("---\n")
	if !bytes.HasPrefix(source, delim) {
		return nil, source
	}
	rest := source[len(delim):]
	idx := bytes.Index(rest, delim)
	if idx < 0 {
		return nil, source
	}
	end := len(delim) + idx + len(delim)
	return source[:end], source[end:]
}

// CountLines returns the number of newline-terminated lines in b.
func CountLines(b []byte) int {
	return bytes.Count(b, []byte("\n"))
}

// ParseFrontMatterKinds extracts the kinds: list from a YAML front-matter
// block (including its --- delimiters). Returns nil kinds and nil error if
// the block is nil or the kinds key is absent. Returns an error if the
// YAML contains anchors/aliases or cannot be parsed.
func ParseFrontMatterKinds(fm []byte) ([]string, error) {
	if len(fm) == 0 {
		return nil, nil
	}
	// Strip the leading and trailing --- delimiters to get raw YAML.
	delim := []byte("---\n")
	body := bytes.TrimPrefix(fm, delim)
	body = bytes.TrimSuffix(body, delim)

	// Fast path: skip full YAML decode when no "kinds:" key is present.
	if !bytes.Contains(body, []byte("kinds:")) {
		return nil, nil
	}

	var parsed struct {
		Kinds []string `yaml:"kinds"`
	}
	if err := yamlutil.UnmarshalSafe(body, &parsed); err != nil {
		return nil, err
	}
	return parsed.Kinds, nil
}

// ParseFrontMatterFields decodes a YAML front-matter block (including its
// --- delimiters) into a map of top-level keys to raw values. Returns
// (nil, nil) when fm is empty or whitespace-only, or when the payload
// decodes to YAML null (yaml.v3 surfaces a null document as a scalar
// node with no decode error). Returns an error when the payload is a
// non-null scalar or a sequence — both reject because the
// field-presence selector requires named keys — or when the YAML is
// otherwise invalid. Used by the kind-assignment field-presence
// selector; a field is considered present when its value is non-null.
func ParseFrontMatterFields(fm []byte) (map[string]any, error) {
	if len(fm) == 0 {
		return nil, nil
	}
	delim := []byte("---\n")
	body := bytes.TrimPrefix(fm, delim)
	body = bytes.TrimSuffix(body, delim)
	if len(bytes.TrimSpace(body)) == 0 {
		return nil, nil
	}
	node, err := yamlutil.UnmarshalNodeSafe(body)
	if err != nil {
		return nil, err
	}
	if node.Kind == 0 || len(node.Content) == 0 {
		return nil, nil
	}
	root := node.Content[0]
	switch root.Kind {
	case yaml.MappingNode:
		var parsed map[string]any
		if err := root.Decode(&parsed); err != nil {
			return nil, err
		}
		return parsed, nil
	case yaml.ScalarNode:
		if root.Tag == "!!null" {
			return nil, nil
		}
		return nil, fmt.Errorf("front matter must be a mapping, got scalar")
	case yaml.SequenceNode:
		return nil, fmt.Errorf("front matter must be a mapping, got sequence")
	default:
		return nil, fmt.Errorf("front matter must be a mapping")
	}
}
