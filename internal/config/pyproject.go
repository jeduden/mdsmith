//go:build !wasm

package config

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/pelletier/go-toml"
	"gopkg.in/yaml.v3"
)

// pyprojectTable is the key path of mdsmith's table in pyproject.toml.
var pyprojectTable = []string{"tool", "mdsmith"}

// loadPyproject reads the `[tool.mdsmith]` table of a pyproject.toml
// (or any TOML file) and loads it as config. The table has the same
// shape as `.mdsmith.yml`: it is converted to YAML and run through the
// shared loadFromBytes pipeline, so the custom YAML decoders, sidecar
// discovery (anchored next to path), and validation all apply
// unchanged.
func loadPyproject(path string) (*Config, error) {
	data, err := readLimitedConfig(path)
	if err != nil {
		return nil, positionError(fmt.Errorf("reading config file: %w", err), path, nil)
	}
	tree, err := toml.LoadBytes(data)
	if err != nil {
		return nil, positionError(fmt.Errorf("parsing %s: %w", path, err), path, nil)
	}
	table, err := mdsmithTable(tree, path)
	if err != nil {
		return nil, positionError(err, path, nil)
	}
	yamlData, err := tomlTableToYAML(table)
	if err != nil {
		return nil, positionError(fmt.Errorf("converting [tool.mdsmith] in %s: %w", path, err), path, nil)
	}
	cfg, err := loadFromBytes(yamlData, path, true)
	if err != nil {
		// Positions found while decoding refer to the generated YAML,
		// not the TOML source, so they are dropped here.
		return nil, &LoadError{File: path, Message: err.Error(), Severity: lint.Error, Err: err}
	}
	return cfg, nil
}

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

// tomlTableToYAML renders a TOML table as a YAML document with the
// same structure.
func tomlTableToYAML(table *toml.Tree) ([]byte, error) {
	node, err := tomlToNode(table)
	if err != nil {
		return nil, err
	}
	return yaml.Marshal(&yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{node}})
}

// tomlToNode converts one go-toml value to a yaml.Node.
func tomlToNode(v any) (*yaml.Node, error) {
	switch t := v.(type) {
	case *toml.Tree:
		return tomlTreeToNode(t)
	case []*toml.Tree:
		seq := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		for _, sub := range t {
			n, err := tomlTreeToNode(sub)
			if err != nil {
				return nil, err
			}
			seq.Content = append(seq.Content, n)
		}
		return seq, nil
	case []any:
		seq := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		for _, el := range t {
			n, err := tomlToNode(el)
			if err != nil {
				return nil, err
			}
			seq.Content = append(seq.Content, n)
		}
		return seq, nil
	}
	return tomlScalarNode(v)
}

// tomlTreeToNode converts a TOML table to a YAML mapping with its keys
// sorted by name. Key order carries no meaning in mdsmith config —
// every mapping decodes into a Go map — and go-toml v1 records no
// source position inside inline tables to recover it from anyway.
func tomlTreeToNode(t *toml.Tree) (*yaml.Node, error) {
	keys := t.Keys()
	sort.Strings(keys)
	m := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	for _, k := range keys {
		val, err := tomlToNode(t.Get(k))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", k, err)
		}
		m.Content = append(m.Content, strNode(k), val)
	}
	return m, nil
}

// strNode is a double-quoted YAML string, so a value such as "true" or
// "1" keeps its string type through the round trip.
func strNode(s string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: s, Style: yaml.DoubleQuotedStyle}
}

// tomlScalarNode converts a TOML scalar to the YAML scalar that decodes
// to the same Go value a `.mdsmith.yml` author would get.
func tomlScalarNode(v any) (*yaml.Node, error) {
	scalar := func(tag, val string) *yaml.Node {
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: tag, Value: val}
	}
	switch t := v.(type) {
	case string:
		return strNode(t), nil
	case bool:
		return scalar("!!bool", strconv.FormatBool(t)), nil
	case int64:
		return scalar("!!int", strconv.FormatInt(t, 10)), nil
	case uint64:
		return scalar("!!int", strconv.FormatUint(t, 10)), nil
	case float64:
		return scalar("!!float", yamlFloat(t)), nil
	case time.Time:
		return scalar("!!timestamp", t.Format(time.RFC3339Nano)), nil
	case toml.LocalDate:
		return scalar("!!timestamp", t.String()), nil
	case toml.LocalDateTime:
		// yaml.v3 accepts a zone-less timestamp only with a space
		// between date and time; it decodes as UTC.
		return scalar("!!timestamp", t.Date.String()+" "+t.Time.String()), nil
	case toml.LocalTime:
		return strNode(t.String()), nil
	}
	return nil, errors.New("unsupported TOML value type " + fmt.Sprintf("%T", v))
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
