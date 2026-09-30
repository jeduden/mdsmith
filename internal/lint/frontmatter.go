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
// value is never mistaken for the fence. It is the one home for the
// delimiter trim every front-matter decoder needs.
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
// cannot be parsed, or if kinds: is not a list of scalars.
func ParseFrontMatterKinds(fm []byte) ([]string, error) {
	if len(fm) == 0 {
		return nil, nil
	}
	body := FrontMatterYAML(fm)

	// Fast path: skip the YAML decode when the word "kinds" appears
	// nowhere. Any spelling of the key ("kinds":, kinds :, a merge
	// key's value) contains it, so the check never drops a real key.
	if !bytes.Contains(body, []byte("kinds")) {
		return nil, nil
	}

	doc, err := yamlutil.UnmarshalNodeSafe(body)
	if err != nil {
		return nil, err
	}
	head, err := DecodeFrontMatterHead(&doc)
	if err != nil {
		return nil, err
	}
	return head.KindList()
}

// FrontMatterHead holds the raw value nodes of the top-level
// `title:` and `kinds:` front-matter keys. A key that is absent
// leaves its node at the zero value (Kind 0).
type FrontMatterHead struct {
	Title yaml.Node `yaml:"title"`
	Kinds yaml.Node `yaml:"kinds"`
}

// DecodeFrontMatterHead decodes the title and kinds nodes from a
// parsed front-matter document (as returned by
// yamlutil.UnmarshalNodeSafe). yaml.v3 applies its usual rules: a
// duplicate top-level key or a non-mapping document is an error, and
// merge keys contribute their values. The engine's kinds parser and
// the workspace index both decode through here, so they agree on
// every spelling. An empty document yields a zero head.
func DecodeFrontMatterHead(doc *yaml.Node) (FrontMatterHead, error) {
	var head FrontMatterHead
	if doc.Kind == 0 {
		return head, nil
	}
	if err := doc.Decode(&head); err != nil {
		return FrontMatterHead{}, err
	}
	return head, nil
}

// KindList decodes the kinds node into a []string. An absent or
// null value yields nil. Scalars of any type keep their source text
// (`- 42` is "42"). A scalar or mapping value, or a list with a
// non-scalar entry, is an error.
func (h *FrontMatterHead) KindList() ([]string, error) {
	if h.Kinds.Kind == 0 {
		return nil, nil
	}
	var kinds []string
	if err := h.Kinds.Decode(&kinds); err != nil {
		return nil, err
	}
	return kinds, nil
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
