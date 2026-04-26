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
// block (including its --- delimiters). Returns nil if the block is nil,
// the key is absent, or it cannot be parsed.
func ParseFrontMatterKinds(fm []byte) []string {
	if len(fm) == 0 {
		return nil
	}
	// Strip the leading and trailing --- delimiters to get raw YAML.
	delim := []byte("---\n")
	body := bytes.TrimPrefix(fm, delim)
	body = bytes.TrimSuffix(body, delim)

	var parsed struct {
		Kinds []string `yaml:"kinds"`
	}
	if err := yaml.Unmarshal(body, &parsed); err != nil {
		return nil
	}
	return parsed.Kinds
}
