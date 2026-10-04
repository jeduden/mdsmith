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
	tree, err := toml.LoadBytes(data)
	if err != nil {
		return nil, positionError(fmt.Errorf("parsing %s: %w", path, tomlErrorIssue(err)), path, nil)
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

// pyprojectHasMdsmithTable reports whether the TOML file at path is an
// mdsmith config source: it parses and has a `tool.mdsmith` entry. A
// file that does not parse counts when a line opens a `[tool.mdsmith`
// header or sets a `tool.mdsmith.` dotted key, so loading it reports
// the syntax error instead of the walk skipping a broken config. An
// unreadable file is not a source.
func pyprojectHasMdsmithTable(path string) bool {
	data, err := readLimitedConfig(path)
	if err != nil {
		return false
	}
	tree, err := toml.LoadBytes(data)
	if err != nil {
		return mdsmithHeaderRe.Match(data)
	}
	return tree.GetPath(pyprojectTable) != nil
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
