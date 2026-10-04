package config

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/yamlutil"
	"gopkg.in/yaml.v3"
)

// KeyPath is a format-neutral address of a value inside a config
// document. Each element is a mapping key (string) or a sequence index
// (int), so `["overrides", 0, "foreign-regions"]` names the
// foreign-regions list of the first override. Validation attaches a
// KeyPath to an Issue; a PositionResolver maps it to a line and column
// in whichever source format the config was read from.
type KeyPath []any

// String renders the path in the dotted form error messages use:
// `overrides[0].foreign-regions`.
func (p KeyPath) String() string {
	var b strings.Builder
	for _, el := range p {
		switch v := el.(type) {
		case int:
			b.WriteByte('[')
			b.WriteString(strconv.Itoa(v))
			b.WriteByte(']')
		default:
			if b.Len() > 0 {
				b.WriteByte('.')
			}
			fmt.Fprint(&b, v)
		}
	}
	return b.String()
}

// Issue is a typed config error. It carries the human-readable
// message, a severity, and where the offending value lives: either a
// structured key Path (from semantic validation) or a pre-resolved
// Line/Column into the YAML text (from the YAML parser or a custom
// node decoder, which know the node position but not its path).
//
// Issue implements error, so validation sites return it through the
// usual `fmt.Errorf("...: %w", ...)` wrapping; the load boundary finds
// it again with errors.As and resolves its position.
type Issue struct {
	Message  string
	Severity lint.Severity
	Path     KeyPath

	// Line and Column, when Line > 0, are a 1-based position in the
	// YAML text that was decoded. They take precedence over Path.
	Line   int
	Column int
}

// Error returns the issue message.
func (i *Issue) Error() string { return i.Message }

// issueAt builds an error-severity Issue at path with a formatted
// message.
func issueAt(path KeyPath, format string, args ...any) *Issue {
	return &Issue{
		Message:  fmt.Sprintf(format, args...),
		Severity: lint.Error,
		Path:     path,
	}
}

// PositionResolver maps a KeyPath to a 1-based line and column in a
// config source. ok is false when not even the first path element is
// present in the source; otherwise the position is that of the deepest
// element that resolved, so a value missing from the source (say, one
// filled in by a convention) still points at its nearest ancestor.
type PositionResolver interface {
	Resolve(path KeyPath) (line, col int, ok bool)
}

// yamlResolver resolves key paths against a parsed yaml.v3 node tree.
// A mapping key resolves to the key node's position; a sequence index
// resolves to the item node's position.
type yamlResolver struct {
	root *yaml.Node
}

// newYAMLResolver parses data into a node tree for position lookups.
// Input that does not parse to a mapping yields a resolver that
// resolves nothing.
func newYAMLResolver(data []byte) PositionResolver {
	node, err := yamlutil.UnmarshalNodeSafe(data)
	if err != nil || node.Kind != yaml.DocumentNode || len(node.Content) == 0 {
		return yamlResolver{}
	}
	return yamlResolver{root: node.Content[0]}
}

// Resolve implements PositionResolver.
func (r yamlResolver) Resolve(path KeyPath) (line, col int, ok bool) {
	last := yamlWalk(r.root, path)
	if last == nil {
		return 0, 0, false
	}
	return last.Line, last.Column, true
}

// yamlWalk follows path from node and returns the node holding the
// deepest element that resolved (a key node for a mapping key, the
// item node for a sequence index), or nil when none did.
func yamlWalk(node *yaml.Node, path KeyPath) *yaml.Node {
	var last *yaml.Node
	for _, el := range path {
		if node == nil {
			break
		}
		key, val := yamlStep(node, el)
		if key == nil {
			break
		}
		last, node = key, val
	}
	return last
}

// yamlStep descends one path element from node. It returns the node to
// report a position for and the node to continue from, or nils when el
// is absent.
func yamlStep(node *yaml.Node, el any) (pos, next *yaml.Node) {
	switch k := el.(type) {
	case string:
		if node.Kind != yaml.MappingNode {
			return nil, nil
		}
		for i := 0; i+1 < len(node.Content); i += 2 {
			if node.Content[i].Value == k {
				return node.Content[i], node.Content[i+1]
			}
		}
	case int:
		if node.Kind != yaml.SequenceNode || k < 0 || k >= len(node.Content) {
			return nil, nil
		}
		return node.Content[k], node.Content[k]
	}
	return nil, nil
}
