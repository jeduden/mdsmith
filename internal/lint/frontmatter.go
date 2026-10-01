package lint

import (
	"bytes"
	"fmt"

	"github.com/jeduden/mdsmith/internal/yamlutil"
	"github.com/jeduden/mdsmith/pkg/markdown"
	"gopkg.in/yaml.v3"
)

// StripFrontMatter removes YAML front matter delimited by "---\n"
// from the beginning of source, forwarding to pkg/markdown so the
// front-matter split lives in one place. It returns the front matter
// block (including delimiters) and the remaining content; if no front
// matter is found, prefix is nil and content equals source.
func StripFrontMatter(source []byte) (prefix, content []byte) {
	return markdown.StripFrontMatter(source)
}

// FrontMatterYAML returns the YAML body of a front-matter block
// (as returned by StripFrontMatter) with its opening and closing
// "---\n" fences removed. The closing fence is removed with a
// suffix trim, not a search, so a "---" line inside a block-scalar
// value is never mistaken for the fence. Input without fences
// passes through unchanged. Decoders that take a StripFrontMatter
// prefix call this instead of repeating the trim.
func FrontMatterYAML(fm []byte) []byte {
	delim := []byte("---\n")
	return bytes.TrimSuffix(bytes.TrimPrefix(fm, delim), delim)
}

// UnmarshalFrontMatter strips the leading YAML front matter block off
// source, decodes it into v via yamlutil.UnmarshalSafe, and returns
// the body with the block removed. hadFrontMatter reports whether
// source had a front-matter block; it is false (and v is left
// untouched) when source had none, true otherwise. Callers that need
// to distinguish "no front matter" from "front matter with no
// recognised keys" (typos, schema mismatch) use hadFrontMatter rather
// than inspecting v's zero state, which conflates the two.
func UnmarshalFrontMatter(source []byte, v any) (body []byte, hadFrontMatter bool, err error) {
	prefix, content := markdown.StripFrontMatter(source)
	if prefix == nil {
		return content, false, nil
	}
	yamlBody := FrontMatterYAML(prefix)
	if err := yamlutil.UnmarshalSafe(yamlBody, v); err != nil {
		return content, true, err
	}
	return content, true, nil
}

// CountLines returns the number of newline-terminated lines in b,
// forwarded from pkg/markdown.
func CountLines(b []byte) int {
	return markdown.CountLines(b)
}

// ParseFrontMatterKinds extracts the kinds: list from a YAML front-matter
// block (including its --- delimiters). Returns nil kinds and nil error if
// the block is nil or the kinds key is absent. Returns an error if the
// YAML contains anchors/aliases, has a duplicate top-level key, or
// cannot be parsed, or if kinds: is not a list of scalars. Entries
// decode as yaml.v3 decodes a []string: `- 42` becomes "42".
//
// The key is read only when the block contains the bytes `kinds:`.
// This fast path is part of the contract: a block that merely
// mentions the word elsewhere is never decoded, so its YAML errors
// do not abort the file. Spellings without those bytes (`"kinds":`,
// `kinds :`, escapes) are not read. The workspace index calls this
// function, so it applies the same rule.
func ParseFrontMatterKinds(fm []byte) ([]string, error) {
	if len(fm) == 0 {
		return nil, nil
	}
	body := FrontMatterYAML(fm)
	if !bytes.Contains(body, kindsKey) {
		return nil, nil
	}
	doc, err := yamlutil.UnmarshalNodeSafe(body)
	if err != nil {
		return nil, err
	}
	return FrontMatterKindsFromNode(body, &doc)
}

// kindsKey is the byte gate ParseFrontMatterKinds documents.
var kindsKey = []byte("kinds:")

// FrontMatterKindsFromNode is ParseFrontMatterKinds for a caller
// that has already parsed body, the FrontMatterYAML of the block,
// into doc with yamlutil.UnmarshalNodeSafe. It applies the same
// `kinds:` byte gate and the same decode without parsing body a
// second time. A nil or empty doc has no kinds.
func FrontMatterKindsFromNode(body []byte, doc *yaml.Node) ([]string, error) {
	if doc == nil || len(doc.Content) == 0 || !bytes.Contains(body, kindsKey) {
		return nil, nil
	}
	var parsed struct {
		Kinds []string `yaml:"kinds"`
	}
	if err := yamlutil.DecodeNodeSafe(doc, &parsed); err != nil {
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
	body := FrontMatterYAML(fm)
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
