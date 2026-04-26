package lint

import (
	"bytes"

	"gopkg.in/yaml.v3"
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

	if err := RejectYAMLAliases(body); err != nil {
		return nil, err
	}

	var parsed struct {
		Kinds []string `yaml:"kinds"`
	}
	if err := yaml.Unmarshal(body, &parsed); err != nil {
		return nil, err
	}
	return parsed.Kinds, nil
}
