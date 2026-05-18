package lint

import (
	"bytes"
	"fmt"

	"github.com/jeduden/mdsmith/internal/yamlutil"
	"github.com/jeduden/mdsmith/pkg/markdown"
)

// StripFrontMatter removes YAML front matter delimited by "---\n"
// from the beginning of source, forwarding to pkg/markdown so the
// front-matter split lives in one place. It returns the front matter
// block (including delimiters) and the remaining content; if no front
// matter is found, prefix is nil and content equals source.
func StripFrontMatter(source []byte) (prefix, content []byte) {
	return markdown.StripFrontMatter(source)
}

// CountLines returns the number of newline-terminated lines in b,
// forwarded from pkg/markdown.
func CountLines(b []byte) int {
	return markdown.CountLines(b)
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
// (nil, nil) when fm is empty, whitespace-only, or decodes to YAML null.
// Returns an error when the payload is a non-null scalar or a sequence
// — both reject because the field-presence selector requires named
// keys — or when the YAML is otherwise invalid. Used by the
// kind-assignment field-presence selector; a field is considered
// present when its value is non-null.
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
	var raw any
	if err := yamlutil.UnmarshalSafe(body, &raw); err != nil {
		return nil, err
	}
	switch v := raw.(type) {
	case nil:
		return nil, nil
	case map[string]any:
		return v, nil
	case map[any]any:
		return nil, fmt.Errorf("front matter mapping keys must be strings")
	default:
		return nil, fmt.Errorf("front matter must be a mapping, got %T", raw)
	}
}
