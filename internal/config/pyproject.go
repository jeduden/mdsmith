//go:build !wasm

package config

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/pelletier/go-toml"
	"gopkg.in/yaml.v3"
)

// pyprojectTable is the key path of mdsmith's table in pyproject.toml.
var pyprojectTable = []string{"tool", "mdsmith"}

// pluralTable is the common misspelling `[tools.mdsmith]`. It is never
// read as config; its presence only earns a hint.
var pluralTable = []string{"tools", "mdsmith"}

// loadPyproject reads the `[tool.mdsmith]` table of a pyproject.toml
// (or any TOML file) and loads it as config. The table has the same
// shape as `.mdsmith.yml`: it is converted to a YAML node tree and run
// through the shared loadFromNode pipeline, so the custom YAML
// decoders, sidecar discovery (anchored next to path), and validation
// all apply unchanged. Every generated node carries the line and column
// of its key in the TOML source, so a failure is positioned in the
// pyproject.toml.
func loadPyproject(path string) (*Config, error) {
	data, err := readLimitedConfig(path)
	if err != nil {
		return nil, positionError(fmt.Errorf("reading config file: %w", err), path, nil)
	}
	tree, err := loadTOML(data)
	if err != nil {
		return nil, positionError(fmt.Errorf("parsing %s: %w", path, tomlErrorIssue(err, data)), path, nil)
	}
	table, err := mdsmithTable(tree, path)
	if err != nil {
		return nil, positionError(err, path, nil)
	}
	doc := tomlTableToDoc(table, data)
	cfg, err := loadFromNode(doc, path, true)
	if err != nil {
		return nil, positionError(err, path, func() PositionResolver { return pyprojectResolver(doc) })
	}
	return cfg, nil
}

// maxTOMLNesting caps how deeply arrays, inline tables and dotted keys
// may nest in a TOML file before go-toml parses it. go-toml v1's
// recursive-descent parser has no depth limit: a pyproject.toml holding
// a few hundred thousand nested `[` (well inside the maxConfigBytes read
// cap) overflows the goroutine stack, a fatal error no recover catches.
// A dotted key or table header with as many segments nests as many
// tables, which the TOML-to-YAML conversion and the decoder then recurse
// through one frame per level. Discovery parses every pyproject.toml on
// its walk, so without the cap any Python project file could crash or
// stall the CLI and the language server. Real config nests a handful of
// levels.
const maxTOMLNesting = 1000

// loadTOML parses data with go-toml after rejecting input whose arrays,
// inline tables and dotted keys nest deeper than maxTOMLNesting.
func loadTOML(data []byte) (*toml.Tree, error) {
	if tomlNestingExceeds(data, maxTOMLNesting) {
		return nil, fmt.Errorf("toml: arrays, inline tables and dotted keys nest deeper than %d levels",
			maxTOMLNesting)
	}
	return toml.LoadBytes(data)
}

// tomlNestingExceeds reports whether nesting outside strings and
// comments in data goes deeper than limit. The depth at a point is the
// count of open `[`/`{` plus the dots of the dotted run it is in: key
// segments — bare, quoted, or space-padded — joined by `.`, each dot one
// more table. Any other byte (`=`, `,`, a bracket, a newline) ends the
// run, so the one dot of each float in an array never adds up. Table
// headers count as brackets and a float's dot as a segment, so the bound
// is conservative.
func tomlNestingExceeds(data []byte, limit int) bool {
	depth, run := 0, 0
	for i := 0; i < len(data); i++ {
		switch c := data[i]; {
		case c == '"' || c == '\'':
			i = tomlStringEnd(data, i)
		case c == '.':
			run++
			if depth+run > limit {
				return true
			}
		case tomlKeyByte(c):
		case c == '#':
			run = 0
			for i < len(data) && data[i] != '\n' {
				i++
			}
		case c == '[' || c == '{':
			run = 0
			depth++
			if depth > limit {
				return true
			}
		case c == ']' || c == '}':
			run = 0
			if depth > 0 {
				depth--
			}
		default:
			run = 0
		}
	}
	return false
}

// tomlKeyByte reports whether c can sit inside a dotted run without
// ending it: a bare-key byte or the blank TOML allows around a dot.
func tomlKeyByte(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' ||
		c == '_' || c == '-' || c == ' ' || c == '\t'
}

// tomlStringEnd returns the index of the byte that closes the string
// opening at data[i] — a basic or literal string, single- or
// multi-line. A single-line string left open at a newline ends there
// (go-toml rejects it), and one left open at end of input ends at
// len(data).
func tomlStringEnd(data []byte, i int) int {
	q := data[i]
	if i+2 < len(data) && data[i+1] == q && data[i+2] == q {
		for j := i + 3; j+2 < len(data); j++ {
			if q == '"' && data[j] == '\\' {
				j++
				continue
			}
			if data[j] == q && data[j+1] == q && data[j+2] == q {
				return j + 2
			}
		}
		return len(data)
	}
	for j := i + 1; j < len(data); j++ {
		switch data[j] {
		case '\\':
			if q == '"' {
				j++
			}
		case q, '\n':
			return j
		}
	}
	return len(data)
}

// probePyproject reads and parses the TOML file at path once and
// reports whether it is an mdsmith config source and, when it is not,
// a one-line hint for a plural `[tools.mdsmith]` table ("" when there
// is none). A file that parses is a source when it has a
// `tool.mdsmith` entry. A file that does not parse is a source when a
// line opens a `[tool.mdsmith` header or sets a `tool.mdsmith.` dotted
// key, so loading it reports the syntax error instead of the walk
// skipping a broken config; it earns no hint. An unreadable file is
// neither.
func probePyproject(path string) (source bool, hint string) {
	data, err := readLimitedConfig(path)
	if err != nil {
		return false, ""
	}
	tree, err := loadTOML(data)
	if err != nil {
		return mdsmithHeaderRe.Match(data), ""
	}
	if tree.GetPath(pyprojectTable) != nil {
		return true, ""
	}
	if tree.GetPath(pluralTable) != nil {
		return false, path + ": [tools.mdsmith] is not read; rename the table to [tool.mdsmith]"
	}
	return false, ""
}

// mdsmithHeaderRe matches a line that opens a `[tool.mdsmith]` or
// `[tool.mdsmith.<sub>]` header (array-of-tables and unclosed forms
// included) or a `tool.mdsmith.<key> =` dotted key.
var mdsmithHeaderRe = regexp.MustCompile(
	`(?m)^[ \t]*(?:\[\[?[ \t]*tool\.mdsmith[ \t]*(?:[\].]|$)|tool\.mdsmith\.)`)

// mdsmithTable returns the `[tool.mdsmith]` table of tree.
func mdsmithTable(tree *toml.Tree, path string) (*toml.Tree, error) {
	switch v := tree.GetPath(pyprojectTable).(type) {
	case *toml.Tree:
		return v, nil
	case nil:
		if tree.GetPath(pluralTable) != nil {
			return nil, fmt.Errorf(
				"%s: no [tool.mdsmith] table; found [tools.mdsmith] — rename it to [tool.mdsmith]", path)
		}
		return nil, fmt.Errorf("%s: no [tool.mdsmith] table", path)
	default:
		return nil, fmt.Errorf("%s: [tool.mdsmith] must be a table, got %T", path, v)
	}
}

// strNode is a double-quoted YAML string, so a value such as "true" or
// "1" keeps its string type through the round trip.
func strNode(s string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: s, Style: yaml.DoubleQuotedStyle}
}

// tomlScalarNode converts a TOML scalar to the YAML scalar that decodes
// to the same Go value a `.mdsmith.yml` author would get.
func tomlScalarNode(v any) *yaml.Node {
	scalar := func(tag, val string) *yaml.Node {
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: tag, Value: val}
	}
	switch t := v.(type) {
	case string:
		return strNode(t)
	case bool:
		return scalar("!!bool", strconv.FormatBool(t))
	case int64:
		return scalar("!!int", strconv.FormatInt(t, 10))
	case uint64:
		return scalar("!!int", strconv.FormatUint(t, 10))
	case float64:
		return scalar("!!float", yamlFloat(t))
	case time.Time:
		return scalar("!!timestamp", t.Format(time.RFC3339Nano))
	case toml.LocalDate:
		return scalar("!!timestamp", t.String())
	case toml.LocalDateTime:
		// yaml.v3 accepts a zone-less timestamp only with a space
		// between date and time; it decodes as UTC.
		return scalar("!!timestamp", t.Date.String()+" "+t.Time.String())
	case toml.LocalTime:
		return strNode(t.String())
	}
	// go-toml yields only the types above; anything else (reachable
	// only through a hand-built tree) keeps its string form.
	return strNode(fmt.Sprint(v))
}

// yamlFloat renders f in YAML float syntax.
func yamlFloat(f float64) string {
	switch {
	case math.IsInf(f, 1):
		return ".inf"
	case math.IsInf(f, -1):
		return "-.inf"
	case math.IsNaN(f):
		return ".nan"
	}
	s := strconv.FormatFloat(f, 'g', -1, 64)
	if !strings.ContainsAny(s, ".e") {
		s += ".0"
	}
	return s
}
