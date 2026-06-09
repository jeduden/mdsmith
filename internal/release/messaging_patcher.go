package release

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	toml "github.com/pelletier/go-toml"
	yaml "gopkg.in/yaml.v3"

	"github.com/jeduden/mdsmith/internal/yamlutil"
)

// must panics if err is non-nil. Used at call sites where the
// upstream library guarantees `err == nil` for the inputs we
// pass (json.Marshal of a string, json.Indent of a RawMessage
// that already round-tripped through Decode, yaml.Encoder
// writing to a bytes.Buffer, toml.Tree.ToTomlString) — the
// panic guards against a future library change that would make
// those paths fallible without forcing an `if err != nil`
// branch in every call site.
func must[T any](v T, err error) T {
	if err != nil {
		panic(fmt.Errorf("impossible error: %w", err))
	}
	return v
}

// mustErr is the unit-typed companion of must, for library
// calls that return only an error (e.g. yaml.Encoder.Encode).
func mustErr(err error) {
	if err != nil {
		panic(fmt.Errorf("impossible error: %w", err))
	}
}

// Patcher reads or rewrites one specific field in a structured
// file (JSON, TOML, or YAML frontmatter). Implementations parse
// the file with a real library (no regex), update the in-memory
// data, and re-emit.
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
// uses. Top-level key order is preserved; nested object/array
// formatting is normalized by `json.Indent` on re-emit.
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
				// Surface the underlying type/parse error so a
				// debugger can see which token shape the value
				// actually was (number, bool, object, …).
				return "", fmt.Errorf(
					"json field %q is not a string: %w", f.Key, err)
			}
			return s, nil
		}
	}
	return "", fmt.Errorf("json field %q not found", f.Key)
}

// PatchValue sets the top-level Key to value (JSON-encoded)
// and re-emits the document. Top-level key order is
// preserved; nested object/array formatting is normalized
// by json.Indent on re-emit.
func (f JSONStringField) PatchValue(body []byte, value string) ([]byte, error) {
	pairs, err := decodeOrderedJSON(body)
	if err != nil {
		return nil, err
	}
	encoded := must(json.Marshal(value))
	found := false
	for i := range pairs {
		if pairs[i].key == f.Key {
			pairs[i].value = encoded
			found = true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("json field %q not found", f.Key)
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
// can re-emit them in source order; emitOrderedJSON normalizes
// inner whitespace via json.Indent, so nested formatting is
// rewritten to the document's 2-space indent (top-level order
// is preserved, inner formatting is not).
func decodeOrderedJSON(body []byte) ([]orderedJSONPair, error) {
	dec := json.NewDecoder(bytes.NewReader(body))
	t, err := dec.Token()
	if err != nil {
		return nil, fmt.Errorf("parse json: %w", err)
	}
	if t != json.Delim('{') {
		return nil, errors.New("json: expected object at root")
	}
	var pairs []orderedJSONPair
	for dec.More() {
		kT, err := dec.Token()
		if err != nil {
			return nil, fmt.Errorf("parse json: %w", err)
		}
		// json.Decoder guarantees that the token in object-key
		// position is a string; a non-string would have surfaced
		// as a tokenization error on the line above, never as a
		// successful non-string token. The assertion is a static
		// invariant, not a runtime check.
		key := kT.(string)
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return nil, fmt.Errorf("parse json value: %w", err)
		}
		pairs = append(pairs, orderedJSONPair{key: key, value: raw})
	}
	// Once dec.More() returns false, the next Token is either
	// `}` (well-formed input) or an EOF / parse error
	// (truncated input). Consume it: a parse error becomes a
	// failure here; a successful read leaves us positioned for
	// the trailing-content guard below.
	if _, err := dec.Token(); err != nil {
		return nil, fmt.Errorf("parse json: %w", err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, errors.New("json: unexpected trailing content after root object")
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
		b.Write(must(json.Marshal(p.key)))
		b.WriteString(": ")
		// Nested values may carry their own multi-line
		// formatting from the source. json.Indent re-renders
		// them with the document's indent so nested objects
		// keep aligned with the parent's 2-space rhythm.
		var pretty bytes.Buffer
		mustErr(json.Indent(&pretty, p.value, "  ", "  "))
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
		return "", fmt.Errorf("parse toml: %w", err)
	}
	val := tree.GetPath(f.path())
	if val == nil {
		return "", fmt.Errorf("toml key %s not found", f.pathString())
	}
	s, ok := val.(string)
	if !ok {
		return "", fmt.Errorf("toml key %s is not a string", f.pathString())
	}
	return s, nil
}

// PatchValue sets Table/Key to value and re-emits the tree.
func (f TOMLStringField) PatchValue(body []byte, value string) ([]byte, error) {
	tree, err := toml.LoadBytes(body)
	if err != nil {
		return nil, fmt.Errorf("parse toml: %w", err)
	}
	if tree.GetPath(f.path()) == nil {
		return nil, fmt.Errorf("toml key %s not found", f.pathString())
	}
	tree.SetPath(f.path(), value)
	// pelletier/go-toml v1's ToTomlString prefixes a leading
	// newline for documents whose first element is a table
	// header. Trim it so the re-emitted file starts at the
	// table on line 1 (the original pyproject.toml shape).
	out := strings.TrimLeft(must(tree.ToTomlString()), "\n")
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
	front, _, _, _, err := splitFrontmatter(body)
	if err != nil {
		return "", err
	}
	root, err := yamlutil.UnmarshalNodeSafe(front)
	if err != nil {
		return "", fmt.Errorf("parse frontmatter: %w", err)
	}
	node, err := findYAMLNode(&root, f.Path)
	if err != nil {
		return "", err
	}
	if node.Kind != yaml.ScalarNode {
		return "", fmt.Errorf("yaml field %s is not a scalar",
			strings.Join(f.Path, "."))
	}
	return node.Value, nil
}

// PatchValue updates the field's scalar value and re-emits the
// frontmatter. The original opener (`---\n` or `---\r\n`) is
// reused so a CRLF source stays CRLF on the opener; body bytes
// after the closing `---` are untouched.
func (f YAMLFrontmatterField) PatchValue(body []byte, value string) ([]byte, error) {
	front, rest, opener, closer, err := splitFrontmatter(body)
	if err != nil {
		return nil, err
	}
	root, err := yamlutil.UnmarshalNodeSafe(front)
	if err != nil {
		return nil, fmt.Errorf("parse frontmatter: %w", err)
	}
	node, err := findYAMLNode(&root, f.Path)
	if err != nil {
		return nil, err
	}
	if node.Kind != yaml.ScalarNode {
		return nil, fmt.Errorf("yaml field %s is not a scalar",
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
	mustErr(enc.Encode(&root))
	mustErr(enc.Close())
	return concat(opener, buf.Bytes(), closer, rest), nil
}

// splitFrontmatter returns the YAML frontmatter (no delimiters)
// and the body that follows, together with the opening and
// closing delimiters as they appear in the original. Accepts
// both `---\n` and `---\r\n` openers/closers, and tolerates a
// closer at EOF with no trailing newline. Returning the opener
// lets PatchValue re-emit the same line-ending style so CRLF
// inputs stay CRLF on the re-write.
func splitFrontmatter(body []byte) (front, rest, opener, closer []byte, err error) {
	openerLen := 0
	switch {
	case bytes.HasPrefix(body, []byte("---\n")):
		openerLen = 4
	case bytes.HasPrefix(body, []byte("---\r\n")):
		openerLen = 5
	default:
		return nil, nil, nil, nil,
			errors.New("file does not start with a yaml frontmatter delimiter")
	}
	opener = body[:openerLen]
	rest = body[openerLen:]
	idx := indexFrontmatterClose(rest)
	if idx < 0 {
		return nil, nil, nil, nil,
			errors.New("yaml frontmatter has no closing delimiter")
	}
	// Closer is `---` plus an optional `\r\n`, `\n`, or EOF.
	// `idx+3` may equal len(rest) when the file ends exactly at
	// `---`; clamp closerEnd before slicing to avoid an
	// out-of-bounds panic on the EOF case.
	closerEnd := idx + 3
	if closerEnd < len(rest) && rest[closerEnd] == '\r' {
		closerEnd++
	}
	if closerEnd < len(rest) && rest[closerEnd] == '\n' {
		closerEnd++
	}
	return rest[:idx], rest[closerEnd:], opener, rest[idx:closerEnd], nil
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
// and returns the leaf scalar. A yaml.v3 DocumentNode normally
// carries one content child (the root mapping); the empty
// Content case is theoretical for our inputs (an
// alias-rejected, well-formed frontmatter that survives
// splitFrontmatter is never empty) but the guard is cheap and
// keeps the walker total instead of panicking on a future
// yaml.v3 behavior change.
func findYAMLNode(root *yaml.Node, path []string) (*yaml.Node, error) {
	cur := root
	if cur.Kind == yaml.DocumentNode {
		if len(cur.Content) == 0 {
			return nil, errors.New("empty yaml frontmatter")
		}
		cur = cur.Content[0]
	}
	for _, seg := range path {
		if cur.Kind != yaml.MappingNode {
			return nil, fmt.Errorf("yaml path %s: parent is not a map",
				strings.Join(path, "."))
		}
		next, err := mappingChild(cur, seg)
		if err != nil {
			return nil, fmt.Errorf("yaml path %s: %w",
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
