//go:build !wasm

package config

import (
	"bytes"
	"regexp"
	"sort"
	"strconv"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/pelletier/go-toml"
	"gopkg.in/yaml.v3"
)

// tomlTableToDoc renders the `[tool.mdsmith]` table as a YAML document
// node with the same structure, for loadFromNode. No YAML text is
// generated, so nothing larger than the capped TOML read is ever
// materialised. Mapping keys are sorted by name (key order carries no
// meaning in mdsmith config: every mapping decodes into a Go map).
//
// Each node carries the 1-based line and column of its key in src, so
// a decoder issue (which reads its node's position) and a key-path
// issue (resolved by pyprojectResolver over this tree) both land in the
// pyproject.toml. go-toml v1 records no position inside an inline
// table, so those keys are found by scanning src forward from the
// enclosing key; a key that cannot be found inherits its parent's
// position.
func tomlTableToDoc(table *toml.Tree, src []byte) *yaml.Node {
	c := tomlConverter{lines: bytes.Split(src, []byte("\n"))}
	root := c.tree(table, table.Position())
	return &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{root}}
}

// tomlConverter converts go-toml values to positioned yaml.Nodes.
type tomlConverter struct {
	lines [][]byte
}

// value converts v, whose key sits at at.
func (c tomlConverter) value(v any, at toml.Position) *yaml.Node {
	switch t := v.(type) {
	case *toml.Tree:
		return c.tree(t, at)
	case []*toml.Tree:
		seq := placed(&yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}, at)
		for i, sub := range t {
			p := sub.Position()
			if p.Invalid() {
				// An array of inline tables: element i opens at the
				// (i+1)-th `{` after the key.
				p = c.nthBrace(at, i)
			}
			seq.Content = append(seq.Content, c.tree(sub, p))
		}
		return seq
	case []any:
		seq := placed(&yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}, at)
		for _, el := range t {
			seq.Content = append(seq.Content, c.value(el, at))
		}
		return seq
	}
	return placed(tomlScalarNode(v), at)
}

// tree converts a table whose key (or `[header]`) sits at at.
func (c tomlConverter) tree(t *toml.Tree, at toml.Position) *yaml.Node {
	m := placed(&yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}, at)
	keys := t.Keys()
	sort.Strings(keys)
	for _, k := range keys {
		v := t.Get(k)
		p := t.GetPosition(k)
		if aot, ok := v.([]*toml.Tree); ok && len(aot) > 0 && !aot[0].Position().Invalid() {
			// go-toml reports an array of tables at its last
			// `[[header]]`; the key belongs at the first.
			p = aot[0].Position()
		}
		if p.Invalid() {
			p = c.findKey(at, k)
		}
		m.Content = append(m.Content, placed(strNode(k), p), c.value(v, p))
	}
	return m
}

// placed stamps n with p when p is a known position.
func placed(n *yaml.Node, p toml.Position) *yaml.Node {
	if !p.Invalid() {
		n.Line, n.Column = p.Line, p.Col
	}
	return n
}

// findKey scans the source from position from (inclusive) for key k
// written as a bare or quoted key followed by `=` or a `.` (dotted
// key), stopping at the next `[table]` header. It returns from when
// the key is not found.
func (c tomlConverter) findKey(from toml.Position, k string) toml.Position {
	if from.Invalid() {
		return from
	}
	for ln := from.Line; ln <= len(c.lines); ln++ {
		line := c.lines[ln-1]
		start := 0
		if ln == from.Line {
			start = min(from.Col-1, len(line))
		} else if t := bytes.TrimLeft(line, " \t"); len(t) > 0 && t[0] == '[' {
			break
		}
		if col := keyColumn(line, start, k); col > 0 {
			return toml.Position{Line: ln, Col: col}
		}
	}
	return from
}

// keyColumn returns the 1-based column of key k in line at or after
// byte offset start, or 0. A match must start the line or follow
// whitespace, `{`, `,`, or `.` (a dotted-key segment), may be quoted, and must be followed
// (after optional spaces) by `=` or `.`.
func keyColumn(line []byte, start int, k string) int {
	for i := start; i < len(line); {
		j := bytes.Index(line[i:], []byte(k))
		if j < 0 {
			return 0
		}
		at := i + j
		begin, end := at, at+len(k)
		if begin > 0 && end < len(line) && isQuote(line[begin-1]) && line[end] == line[begin-1] {
			begin--
			end++
		}
		if keyBoundaryBefore(line, begin) && keyFollowedByAssign(line, end) {
			return begin + 1
		}
		i = at + 1
	}
	return 0
}

// isQuote reports whether b opens a quoted TOML key.
func isQuote(b byte) bool { return b == '"' || b == '\'' }

// keyBoundaryBefore reports whether a key may start at offset i.
func keyBoundaryBefore(line []byte, i int) bool {
	if i == 0 {
		return true
	}
	switch line[i-1] {
	case ' ', '\t', '{', ',', '.':
		return true
	}
	return false
}

// keyFollowedByAssign reports whether line[i:] continues with optional
// spaces and then `=` or `.`.
func keyFollowedByAssign(line []byte, i int) bool {
	for ; i < len(line); i++ {
		switch line[i] {
		case ' ', '\t':
			continue
		case '=', '.':
			return true
		}
		return false
	}
	return false
}

// nthBrace returns the position of the (n+1)-th `{` at or after from,
// or from when there are fewer.
func (c tomlConverter) nthBrace(from toml.Position, n int) toml.Position {
	if from.Invalid() {
		return from
	}
	for ln := from.Line; ln <= len(c.lines); ln++ {
		line := c.lines[ln-1]
		start := 0
		if ln == from.Line {
			start = min(from.Col-1, len(line))
		}
		for i := start; i < len(line); i++ {
			if line[i] != '{' {
				continue
			}
			if n == 0 {
				return toml.Position{Line: ln, Col: i + 1}
			}
			n--
		}
	}
	return from
}

// pyprojectResolver resolves key paths (relative to `[tool.mdsmith]`)
// over the positioned node tree tomlTableToDoc built. A path that does
// not resolve at all anchors on the table's own header.
func pyprojectResolver(doc *yaml.Node) PositionResolver {
	return tomlResolver{root: doc.Content[0]}
}

// tomlResolver is the PositionResolver for a pyproject table.
type tomlResolver struct {
	root *yaml.Node
}

// Resolve implements PositionResolver.
func (r tomlResolver) Resolve(path KeyPath) (line, col int, ok bool) {
	if line, col, ok := yamlResolver(r).Resolve(path); ok {
		return line, col, true
	}
	if r.root.Line > 0 {
		return r.root.Line, r.root.Column, true
	}
	return 0, 0, false
}

// tomlPosRe matches the `(line, col): ` prefix go-toml v1 puts on a
// parse error.
var tomlPosRe = regexp.MustCompile(`^\((\d+), (\d+)\)`)

// tomlErrorIssue turns a go-toml parse error into an Issue at the line
// and column the parser reported; an error without one passes through.
func tomlErrorIssue(err error) error {
	m := tomlPosRe.FindStringSubmatch(err.Error())
	if m == nil {
		return err
	}
	line, _ := strconv.Atoi(m[1]) // \d+ always parses
	col, _ := strconv.Atoi(m[2])
	return &Issue{Message: err.Error(), Severity: lint.Error, Err: err, Line: line, Column: col}
}
