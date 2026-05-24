package release

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// Patcher reads or rewrites one specific field in a structured
// file (JSON, TOML, YAML frontmatter, or a generated Markdown
// fragment). Implementations are pure byte transforms: no IO,
// no side effects. Caller wires reads and writes around them.
//
// ReadValue / PatchValue together support both the apply path
// (PatchValue + write-if-changed) and the drift check
// (ReadValue + compare against source).
type Patcher interface {
	// ReadValue returns the field's current string value.
	// Returns a "field not found" error when the field is
	// missing from body.
	ReadValue(body []byte) (string, error)
	// PatchValue rewrites the field to value and returns the
	// new bytes. The shape of the file is otherwise preserved.
	PatchValue(body []byte, value string) ([]byte, error)
}

// JSONStringField patches a JSON object's top-level string
// field by name. Suitable for npm `package.json` and Claude
// Code `plugin.json` files where `description` is a root-level
// key. The regex matches `"key": "..."` anywhere in the body,
// which is fine for the targets we patch (none embeds the
// patched key inside a string value); the apply path's
// idempotence check catches any false positive after the fact.
type JSONStringField struct{ Key string }

func (f JSONStringField) re() *regexp.Regexp {
	// Group 1 captures the quoted value including its outer
	// quotes so the replacement can swap the whole literal.
	return regexp.MustCompile(`"` + regexp.QuoteMeta(f.Key) +
		`"[ \t]*:[ \t]*("(?:[^"\\]|\\.)*")`)
}

// ReadValue returns the decoded current value of the JSON field.
// Decoding unescapes \" / \\ / \uXXXX so the returned string
// matches what a json.Decoder would produce.
func (f JSONStringField) ReadValue(body []byte) (string, error) {
	sub := f.re().FindSubmatch(body)
	if sub == nil {
		return "", fmt.Errorf("JSON field %q not found", f.Key)
	}
	var s string
	if err := json.Unmarshal(sub[1], &s); err != nil {
		return "", fmt.Errorf("decode JSON field %q: %w", f.Key, err)
	}
	return s, nil
}

// PatchValue rewrites the field to value (JSON-encoded). Returns
// the original bytes unchanged when the field already holds
// value.
func (f JSONStringField) PatchValue(body []byte, value string) ([]byte, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("encode value for JSON field %q: %w", f.Key, err)
	}
	idx := f.re().FindSubmatchIndex(body)
	if idx == nil {
		return nil, fmt.Errorf("JSON field %q not found", f.Key)
	}
	// idx[2]:idx[3] is the quoted value literal, including
	// outer quotes — swap it for the encoded replacement,
	// which carries its own quotes.
	return concat(body[:idx[2]], encoded, body[idx[3]:]), nil
}

// TOMLStringField patches a TOML basic-string value at
// Table/Key. Table is the bracketed header path (`["params"]`
// for `[params]`); nil targets the root table. The patch is
// scoped: a key inside another `[table]` is not affected. Only
// basic strings ("...") are recognized — literal ('...') or
// multiline ("""...""") values are not patched.
type TOMLStringField struct {
	Table []string
	Key   string
}

func (f TOMLStringField) keyRE() *regexp.Regexp {
	return regexp.MustCompile(`(?m)^([ \t]*` +
		regexp.QuoteMeta(f.Key) + `[ \t]*=[ \t]*)` +
		`"((?:[^"\\]|\\.)*)"`)
}

// ReadValue locates Table, then the first Key inside it, and
// returns the decoded value.
func (f TOMLStringField) ReadValue(body []byte) (string, error) {
	scope, err := f.scope(body)
	if err != nil {
		return "", err
	}
	sub := f.keyRE().FindSubmatch(scope)
	if sub == nil {
		return "", fmt.Errorf("TOML key %q not found in %s",
			f.Key, f.tablePath())
	}
	return decodeTOMLBasicString(string(sub[2]))
}

// PatchValue rewrites the first Key inside Table to value.
// value is encoded as a TOML basic string (\, ", and control
// chars escaped).
func (f TOMLStringField) PatchValue(body []byte, value string) ([]byte, error) {
	start, end, err := f.scopeBounds(body)
	if err != nil {
		return nil, err
	}
	scope := body[start:end]
	idx := f.keyRE().FindSubmatchIndex(scope)
	if idx == nil {
		return nil, fmt.Errorf("TOML key %q not found in %s",
			f.Key, f.tablePath())
	}
	encoded := encodeTOMLBasicString(value)
	// idx[2]:idx[3] is the prefix (whitespace + key + `=` +
	// spaces). The opening quote sits at idx[3]; the closing
	// quote at idx[5]. Replace [idx[3] .. idx[5]+1) — the
	// whole `"..."` literal — with the encoded value wrapped
	// in fresh quotes.
	patched := concat(
		scope[:idx[3]],
		[]byte(`"`+encoded+`"`),
		scope[idx[5]+1:],
	)
	return concat(body[:start], patched, body[end:]), nil
}

// scope returns the byte slice of body that belongs to f.Table.
// Returns the whole body when f.Table is empty.
func (f TOMLStringField) scope(body []byte) ([]byte, error) {
	start, end, err := f.scopeBounds(body)
	if err != nil {
		return nil, err
	}
	return body[start:end], nil
}

func (f TOMLStringField) scopeBounds(body []byte) (int, int, error) {
	if len(f.Table) == 0 {
		// Root table runs from byte 0 to the first `[`
		// header line.
		end := findNextTableHeader(body, 0)
		return 0, end, nil
	}
	headerRE := regexp.MustCompile(`(?m)^[ \t]*\[[ \t]*` +
		regexp.QuoteMeta(f.tablePath()) + `[ \t]*\][ \t]*$`)
	loc := headerRE.FindIndex(body)
	if loc == nil {
		return 0, 0, fmt.Errorf("TOML table %s not found", f.tablePath())
	}
	start := loc[1]
	end := findNextTableHeader(body, start)
	return start, end, nil
}

// findNextTableHeader returns the byte index of the next
// `[table]` header line at or after start, or len(body) when
// no further header exists.
func findNextTableHeader(body []byte, start int) int {
	re := regexp.MustCompile(`(?m)^[ \t]*\[`)
	loc := re.FindIndex(body[start:])
	if loc == nil {
		return len(body)
	}
	return start + loc[0]
}

func (f TOMLStringField) tablePath() string {
	return strings.Join(f.Table, ".")
}

// decodeTOMLBasicString reverses TOML basic-string escapes for
// the small subset (`\\`, `\"`, `\n`, `\t`, `\r`) that may
// appear in the fields we patch. Any other escape passes
// through verbatim — sufficient for descriptions but not a
// general-purpose TOML decoder.
func decodeTOMLBasicString(s string) (string, error) {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' {
			b.WriteByte(s[i])
			continue
		}
		if i+1 >= len(s) {
			return "", errors.New("trailing backslash in TOML string")
		}
		switch s[i+1] {
		case '\\':
			b.WriteByte('\\')
		case '"':
			b.WriteByte('"')
		case 'n':
			b.WriteByte('\n')
		case 't':
			b.WriteByte('\t')
		case 'r':
			b.WriteByte('\r')
		default:
			b.WriteByte(s[i])
			b.WriteByte(s[i+1])
		}
		i++
	}
	return b.String(), nil
}

// encodeTOMLBasicString escapes the characters TOML requires
// inside a basic string. UTF-8 bytes other than control chars
// pass through unchanged.
func encodeTOMLBasicString(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// YAMLFrontmatterField patches a field at a dotted path inside
// the YAML frontmatter of a Markdown file. The frontmatter is
// the leading block delimited by `---\n` … `\n---\n`; the
// body that follows is preserved byte-for-byte.
//
// The implementation parses the frontmatter to a yaml.Node,
// updates the target scalar, and re-emits the frontmatter. The
// first run may normalize incidental formatting (quote style,
// indentation); subsequent runs are byte-stable. Block scalar
// style (e.g. `>-`) is preserved when present on the target
// node, so a multi-line `lead:` keeps its folded style.
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
	// Decide quote/style for the new value. Multi-line values
	// reuse the existing folded/literal style when present; a
	// scalar that fits on one line uses double-quoted style so
	// embedded em-dashes and punctuation render without
	// surprises.
	node.Value = value
	if !strings.ContainsAny(value, "\n") {
		// Single-line: drop folded/literal style so the emitted
		// form is a plain quoted scalar. yaml.v3 picks the
		// narrowest safe form; we hint at double-quoted to keep
		// the file diffs aligned with how Hugo content
		// frontmatter is conventionally written.
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
	newFront := buf.Bytes()
	return concat([]byte("---\n"), newFront, closer, rest), nil
}

// MarkdownFragment writes (and reads) a generated Markdown
// fragment whose entire body is one messaging field's value.
// The file always carries a "do not edit by hand" header
// comment; ReadValue returns the prose body without the
// header.
type MarkdownFragment struct{}

// fragmentHeader is the comment that precedes every generated
// fragment body. mdsmith fix-of-the-future could promote this
// to a `<?build?>` recipe; the comment is the human signal in
// the meantime.
const fragmentHeader = "<!-- Generated by `mdsmith-release sync-messaging` from " +
	"docs/brand/messaging.md — do not edit by hand. -->"

// ReadValue returns the fragment body with the header comment
// and the trailing blank line stripped.
func (MarkdownFragment) ReadValue(body []byte) (string, error) {
	s := strings.TrimPrefix(string(body), fragmentHeader+"\n")
	// PatchValue always inserts a blank line between the header
	// and the body; drop it on the way back so the round-trip
	// preserves the value caller passed in.
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
// A file without a leading `---` line returns an error so
// callers cannot accidentally trample a non-frontmatter file.
func splitFrontmatter(body []byte) (front, rest, closer []byte, err error) {
	const openMarker = "---\n"
	if !bytes.HasPrefix(body, []byte(openMarker)) {
		return nil, nil, nil,
			errors.New("file does not start with a YAML frontmatter delimiter")
	}
	rest = body[len(openMarker):]
	// Find the closing `\n---\n` (or `\n---\r\n` / `\n---` at
	// EOF).
	closeRE := regexp.MustCompile(`(?m)^---[ \t]*\r?\n`)
	loc := closeRE.FindIndex(rest)
	if loc == nil {
		return nil, nil, nil,
			errors.New("YAML frontmatter has no closing delimiter")
	}
	return rest[:loc[0]], rest[loc[1]:], rest[loc[0]:loc[1]], nil
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

// mappingChild returns the value node for key in a mapping
// node. Yaml.v3 stores mapping children as alternating
// key/value scalars in Content; this helper walks them in
// pairs.
func mappingChild(node *yaml.Node, key string) (*yaml.Node, error) {
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1], nil
		}
	}
	return nil, fmt.Errorf("key %q not found", key)
}
