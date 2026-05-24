package release

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	toml "github.com/pelletier/go-toml"
	yaml "gopkg.in/yaml.v3"
)

// Patcher reads or rewrites one specific field in a structured
// file (JSON, TOML, YAML frontmatter, or a generated Markdown
// fragment). Implementations parse the file with a real
// library (no regex), update the in-memory data, and re-emit.
//
// One consequence: the first apply pass may normalize
// incidental formatting (key order in nested JSON objects,
// TOML comments, YAML quote style); subsequent applies are
// byte-stable.
type Patcher interface {
	// ReadValue returns the field's current string value.
	// Returns a "field not found" error when the field is
	// missing from body.
	ReadValue(body []byte) (string, error)
	// PatchValue rewrites the field to value and returns the
	// new bytes.
	PatchValue(body []byte, value string) ([]byte, error)
}

// JSONStringField patches a top-level string field of a JSON
// object. The implementation walks the document with
// encoding/json's token API to preserve the order of
// top-level keys (npm and Claude Code manifests are
// hand-ordered for readability), then re-emits with 2-space
// indent — the convention every tracked manifest already
// uses. Values that are not the target pass through as
// json.RawMessage, so nested objects keep their inner
// formatting unchanged.
type JSONStringField struct{ Key string }

// ReadValue parses body and returns the string at the
// top-level Key.
func (f JSONStringField) ReadValue(body []byte) (string, error) {
	pairs, err := decodeOrderedJSON(body)
	if err != nil {
		return "", err
	}
	for _, p := range pairs {
		if p.key == f.Key {
			var s string
			if err := json.Unmarshal(p.value, &s); err != nil {
				return "", fmt.Errorf(
					"JSON field %q is not a string", f.Key)
			}
			return s, nil
		}
	}
	return "", fmt.Errorf("JSON field %q not found", f.Key)
}

// PatchValue sets the top-level Key to value (JSON-encoded)
// and re-emits the document. Top-level key order is
// preserved; nested values are emitted as their original
// bytes via json.RawMessage.
func (f JSONStringField) PatchValue(body []byte, value string) ([]byte, error) {
	pairs, err := decodeOrderedJSON(body)
	if err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf(
			"encode value for JSON field %q: %w", f.Key, err)
	}
	found := false
	for i := range pairs {
		if pairs[i].key == f.Key {
			pairs[i].value = encoded
			found = true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("JSON field %q not found", f.Key)
	}
	return emitOrderedJSON(pairs)
}

type orderedJSONPair struct {
	key   string
	value json.RawMessage
}

// decodeOrderedJSON walks body with json.Decoder.Token to
// build an ordered slice of (key, RawMessage) pairs at the
// top level. Nested values stay as RawMessage so PatchValue
// can re-emit them byte-identical.
func decodeOrderedJSON(body []byte) ([]orderedJSONPair, error) {
	dec := json.NewDecoder(bytes.NewReader(body))
	t, err := dec.Token()
	if err != nil {
		return nil, fmt.Errorf("parse JSON: %w", err)
	}
	if t != json.Delim('{') {
		return nil, errors.New("JSON: expected object at root")
	}
	var pairs []orderedJSONPair
	for dec.More() {
		kT, err := dec.Token()
		if err != nil {
			return nil, fmt.Errorf("parse JSON: %w", err)
		}
		key, ok := kT.(string)
		if !ok {
			return nil, errors.New("JSON: non-string key")
		}
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return nil, fmt.Errorf("parse JSON value: %w", err)
		}
		pairs = append(pairs, orderedJSONPair{key: key, value: raw})
	}
	return pairs, nil
}

// emitOrderedJSON renders pairs as a 2-space-indented JSON
// object with a trailing newline (the convention every
// tracked manifest already uses).
func emitOrderedJSON(pairs []orderedJSONPair) ([]byte, error) {
	var b bytes.Buffer
	b.WriteString("{\n")
	for i, p := range pairs {
		b.WriteString("  ")
		keyBytes, err := json.Marshal(p.key)
		if err != nil {
			return nil, fmt.Errorf("encode JSON key %q: %w", p.key, err)
		}
		b.Write(keyBytes)
		b.WriteString(": ")
		// Nested values may carry their own multi-line
		// formatting from the source. json.Indent re-renders
		// them with the document's indent so nested objects
		// keep aligned with the parent's 2-space rhythm.
		var pretty bytes.Buffer
		if err := json.Indent(&pretty, p.value, "  ", "  "); err != nil {
			return nil, fmt.Errorf("indent JSON value for %q: %w", p.key, err)
		}
		b.Write(pretty.Bytes())
		if i < len(pairs)-1 {
			b.WriteString(",")
		}
		b.WriteString("\n")
	}
	b.WriteString("}\n")
	return b.Bytes(), nil
}

// TOMLStringField patches a TOML basic-string value at
// Table/Key. The implementation parses the document with
// pelletier/go-toml v1's TomlTree (so we honor the user-
// requested "parse, update, re-emit" flow), sets the path,
// and writes the tree back. TomlTree.ToTomlString preserves
// double-quoted strings and 2-space indent, but discards
// comments — callers that need to keep prose explanations
// should move them out of the patched file. Table is the
// dotted-path table header (`["params"]` for `[params]`);
// nil targets the root table.
type TOMLStringField struct {
	Table []string
	Key   string
}

// ReadValue parses body and returns the string at Table/Key.
func (f TOMLStringField) ReadValue(body []byte) (string, error) {
	tree, err := toml.LoadBytes(body)
	if err != nil {
		return "", fmt.Errorf("parse TOML: %w", err)
	}
	val := tree.GetPath(f.path())
	if val == nil {
		return "", fmt.Errorf("TOML key %s not found", f.pathString())
	}
	s, ok := val.(string)
	if !ok {
		return "", fmt.Errorf("TOML key %s is not a string", f.pathString())
	}
	return s, nil
}

// PatchValue sets Table/Key to value and re-emits the tree.
func (f TOMLStringField) PatchValue(body []byte, value string) ([]byte, error) {
	tree, err := toml.LoadBytes(body)
	if err != nil {
		return nil, fmt.Errorf("parse TOML: %w", err)
	}
	if tree.GetPath(f.path()) == nil {
		return nil, fmt.Errorf("TOML key %s not found", f.pathString())
	}
	tree.SetPath(f.path(), value)
	out, err := tree.ToTomlString()
	if err != nil {
		return nil, fmt.Errorf("emit TOML: %w", err)
	}
	return []byte(out), nil
}

func (f TOMLStringField) path() []string {
	if len(f.Table) == 0 {
		return []string{f.Key}
	}
	return append(append([]string{}, f.Table...), f.Key)
}

func (f TOMLStringField) pathString() string {
	return strings.Join(f.path(), ".")
}

// YAMLFrontmatterField patches a field at a dotted path inside
// the YAML frontmatter of a Markdown file. The frontmatter is
// the leading block delimited by `---\n` … `\n---\n`; the
// body that follows is preserved byte-for-byte.
//
// Parsing uses yaml.Node (gopkg.in/yaml.v3) so order and
// comments survive the round-trip. The first apply may
// normalize quote style on the target scalar; subsequent
// applies are byte-stable.
type YAMLFrontmatterField struct{ Path []string }

// ReadValue locates the field and returns its scalar value.
func (f YAMLFrontmatterField) ReadValue(body []byte) (string, error) {
	front, _, _, err := splitFrontmatter(body)
	if err != nil {
		return "", err
	}
	var root yaml.Node
	if err := yaml.Unmarshal(front, &root); err != nil {
		return "", fmt.Errorf("parse frontmatter: %w", err)
	}
	node, err := findYAMLNode(&root, f.Path)
	if err != nil {
		return "", err
	}
	if node.Kind != yaml.ScalarNode {
		return "", fmt.Errorf("YAML field %s is not a scalar",
			strings.Join(f.Path, "."))
	}
	return node.Value, nil
}

// PatchValue updates the field's scalar value and re-emits the
// frontmatter. Body bytes after the closing `---` are
// untouched.
func (f YAMLFrontmatterField) PatchValue(body []byte, value string) ([]byte, error) {
	front, rest, closer, err := splitFrontmatter(body)
	if err != nil {
		return nil, err
	}
	var root yaml.Node
	if err := yaml.Unmarshal(front, &root); err != nil {
		return nil, fmt.Errorf("parse frontmatter: %w", err)
	}
	node, err := findYAMLNode(&root, f.Path)
	if err != nil {
		return nil, err
	}
	if node.Kind != yaml.ScalarNode {
		return nil, fmt.Errorf("YAML field %s is not a scalar",
			strings.Join(f.Path, "."))
	}
	node.Value = value
	if !strings.ContainsAny(value, "\n") {
		// Single-line scalars use DoubleQuotedStyle so
		// embedded em-dashes and punctuation render
		// predictably — Hugo content frontmatter favors
		// this style.
		node.Style = yaml.DoubleQuotedStyle
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&root); err != nil {
		return nil, fmt.Errorf("emit frontmatter: %w", err)
	}
	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("close frontmatter encoder: %w", err)
	}
	return concat([]byte("---\n"), buf.Bytes(), closer, rest), nil
}

// MarkdownFragment writes (and reads) a generated Markdown
// fragment whose entire body is one messaging field's value.
// The file always carries a "do not edit by hand" header
// comment; ReadValue returns the prose body without the
// header.
type MarkdownFragment struct{}

const fragmentHeader = "<!-- Generated by `mdsmith-release sync-messaging` from " +
	"docs/brand/messaging.md — do not edit by hand. -->"

// ReadValue returns the fragment body with the header comment
// and the trailing blank line stripped.
func (MarkdownFragment) ReadValue(body []byte) (string, error) {
	s := strings.TrimPrefix(string(body), fragmentHeader+"\n")
	s = strings.TrimPrefix(s, "\n")
	return strings.TrimRight(s, "\n"), nil
}

// PatchValue produces the canonical fragment body: header,
// blank line, value, trailing newline.
func (MarkdownFragment) PatchValue(_ []byte, value string) ([]byte, error) {
	value = strings.TrimRight(value, "\n")
	return []byte(fragmentHeader + "\n\n" + value + "\n"), nil
}

// splitFrontmatter returns the YAML frontmatter (no delimiters)
// and the body that follows, together with the closing
// delimiter as it appears in the original (`---\n` typically).
func splitFrontmatter(body []byte) (front, rest, closer []byte, err error) {
	const openMarker = "---\n"
	if !bytes.HasPrefix(body, []byte(openMarker)) {
		return nil, nil, nil,
			errors.New("file does not start with a YAML frontmatter delimiter")
	}
	rest = body[len(openMarker):]
	idx := indexFrontmatterClose(rest)
	if idx < 0 {
		return nil, nil, nil,
			errors.New("YAML frontmatter has no closing delimiter")
	}
	closerEnd := idx + len("---\n")
	// Allow trailing CR before the LF (Windows-style files).
	if idx+3 < len(rest) && rest[idx+3] == '\r' {
		closerEnd = idx + len("---\r\n")
	}
	return rest[:idx], rest[closerEnd:], rest[idx:closerEnd], nil
}

// indexFrontmatterClose scans for a line that is exactly
// `---` (followed by an optional CR and a LF, or by EOF). The
// scan is a small line-oriented walk — yaml.v3 doesn't expose
// the closer position itself, and a raw `bytes.Index("\n---\n")`
// would miss indented `---` inside the YAML and EOF cases.
func indexFrontmatterClose(s []byte) int {
	for i := 0; i < len(s); {
		// Find start of current line.
		lineStart := i
		// Find end of line.
		nl := bytes.IndexByte(s[i:], '\n')
		var line []byte
		if nl < 0 {
			line = s[i:]
			i = len(s)
		} else {
			line = s[i : i+nl]
			i = i + nl + 1
		}
		// A delimiter line is exactly `---` with no other
		// content (an optional trailing CR is tolerated).
		trim := line
		if len(trim) > 0 && trim[len(trim)-1] == '\r' {
			trim = trim[:len(trim)-1]
		}
		if string(trim) == "---" {
			return lineStart
		}
	}
	return -1
}

// findYAMLNode walks a parsed yaml.Node tree by dotted path
// and returns the leaf scalar.
func findYAMLNode(root *yaml.Node, path []string) (*yaml.Node, error) {
	cur := root
	if cur.Kind == yaml.DocumentNode {
		if len(cur.Content) == 0 {
			return nil, errors.New("empty frontmatter")
		}
		cur = cur.Content[0]
	}
	for _, seg := range path {
		if cur.Kind != yaml.MappingNode {
			return nil, fmt.Errorf("YAML path %s: parent is not a map",
				strings.Join(path, "."))
		}
		next, err := mappingChild(cur, seg)
		if err != nil {
			return nil, fmt.Errorf("YAML path %s: %w",
				strings.Join(path, "."), err)
		}
		cur = next
	}
	return cur, nil
}

func mappingChild(node *yaml.Node, key string) (*yaml.Node, error) {
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1], nil
		}
	}
	return nil, fmt.Errorf("key %q not found", key)
}
