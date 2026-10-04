//go:build !wasm

package config

import (
	"math"
	"path/filepath"
	"testing"
	"time"

	"github.com/pelletier/go-toml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

const equivPyproject = `[project]
name = "demo"
version = "0.1.0"

[tool.black]
line-length = 88

[tool.mdsmith]
files = ["docs/**/*.md", "README.md"]
convention = "house"
max-input-size = "2MB"

[tool.mdsmith.rules]
no-bare-urls = false
line-length = { max = 100, exclude = ["code-blocks"] }

[tool.mdsmith.rules.heading-style]
style = "atx"

[tool.mdsmith.conventions.house]
flavor = "commonmark"

[tool.mdsmith.conventions.house.rules]
first-line-heading = { level = 1 }

[[tool.mdsmith.overrides]]
glob = ["CHANGELOG.md"]
rules = { no-duplicate-headings = false }

[[tool.mdsmith.overrides]]
glob = ["docs/api/**"]
[tool.mdsmith.overrides.rules.line-length]
max = 120

[tool.mdsmith.kinds.plan]
path-pattern = "plan/*.md"

[tool.mdsmith.kinds.plan.schema.frontmatter]
id = "int"
status = "string"

[[tool.mdsmith.kind-assignment]]
glob = ["plan/*.md"]
kinds = ["plan"]
`

const equivYAML = `files: ["docs/**/*.md", "README.md"]
convention: house
max-input-size: 2MB
rules:
  no-bare-urls: false
  line-length:
    max: 100
    exclude: [code-blocks]
  heading-style:
    style: atx
conventions:
  house:
    flavor: commonmark
    rules:
      first-line-heading:
        level: 1
overrides:
  - glob: ["CHANGELOG.md"]
    rules:
      no-duplicate-headings: false
  - glob: ["docs/api/**"]
    rules:
      line-length:
        max: 120
kinds:
  plan:
    path-pattern: "plan/*.md"
    schema:
      frontmatter:
        id: int
        status: string
kind-assignment:
  - glob: ["plan/*.md"]
    kinds: [plan]
`

// clearSourcePaths blanks the provenance fields that name the file a
// kind or convention came from, so two configs read from different
// files compare equal on everything else.
func clearSourcePaths(cfg *Config) {
	for name, k := range cfg.Kinds {
		k.SourcePath = ""
		k.Schema.SourcePath = ""
		cfg.Kinds[name] = k
	}
	for name, c := range cfg.Conventions {
		c.SourcePath = ""
		cfg.Conventions[name] = c
	}
}

func TestLoadPyproject_MatchesEquivalentYAML(t *testing.T) {
	tomlDir, yamlDir := t.TempDir(), t.TempDir()
	py := writeCfg(t, tomlDir, "pyproject.toml", equivPyproject)
	yml := writeCfg(t, yamlDir, ".mdsmith.yml", equivYAML)

	fromTOML, err := loadPyproject(py)
	require.NoError(t, err)
	fromYAML, err := Load(yml)
	require.NoError(t, err)

	// Provenance names the file each kind came from.
	assert.Equal(t, py, fromTOML.Kinds["plan"].SourcePath)
	clearSourcePaths(fromTOML)
	clearSourcePaths(fromYAML)
	assert.Equal(t, fromYAML, fromTOML)

	// Effective config agrees too, across defaults, convention, kinds,
	// and overrides.
	assert.Equal(t, Merge(Defaults(), fromYAML), Merge(Defaults(), fromTOML))
	assert.False(t, fromTOML.Rules["no-bare-urls"].Enabled)
	assert.Equal(t, 1, fromTOML.ConventionPreset["first-line-heading"].Settings["level"])
}

func TestLoadPyproject_SidecarAnchoredNextToPyproject(t *testing.T) {
	dir := t.TempDir()
	py := writeCfg(t, dir, "pyproject.toml", "[tool.mdsmith]\nfiles = [\"*.md\"]\n")
	kf := writeCfg(t, dir, ".mdsmith/kinds/note.yml", "path-pattern: \"notes/*.md\"\n")
	cfg, err := loadPyproject(py)
	require.NoError(t, err)
	require.Contains(t, cfg.Kinds, "note")
	assert.Equal(t, kf, cfg.Kinds["note"].SourcePath)
}

func TestLoadPyproject_Errors(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		name, body, want string
	}{
		{"no table", "[project]\nname = \"x\"\n", "no [tool.mdsmith] table"},
		{"array of tables", "[[tool.mdsmith]]\nfiles = []\n", "[tool.mdsmith] must be a table"},
		{"syntax", "[tool.mdsmith\n", "parsing"},
		{"bad value", "[tool.mdsmith]\nconvention = \"nope\"\n", "unknown convention"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := writeCfg(t, dir, tc.name+".toml", tc.body)
			_, err := loadPyproject(p)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
			le := requireLoadError(t, err)
			assert.Equal(t, p, le.File)
		})
	}
	_, err := loadPyproject(filepath.Join(dir, "missing.toml"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reading config file")
}

func TestTOMLScalarConversion(t *testing.T) {
	tree, err := toml.Load(`s = "true"
b = true
i = -3
f = 2.0
g = 1.5e3
inf = inf
ninf = -inf
nan = nan
odt = 1979-05-27T07:32:00Z
ldt = 1979-05-27T07:32:00
lt = 07:32:00
arr = [[1, 2], ["a"]]
tables = [{a = 1}, {a = 2}]
`)
	require.NoError(t, err)
	out, err := tomlTableToYAML(tree)
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, yaml.Unmarshal(out, &got))
	assert.Equal(t, "true", got["s"])
	assert.Equal(t, true, got["b"])
	assert.Equal(t, -3, got["i"])
	assert.Equal(t, 2.0, got["f"])
	assert.Equal(t, 1500.0, got["g"])
	assert.True(t, math.IsInf(got["inf"].(float64), 1))
	assert.True(t, math.IsInf(got["ninf"].(float64), -1))
	assert.True(t, math.IsNaN(got["nan"].(float64)))
	assert.Equal(t, time.Date(1979, 5, 27, 7, 32, 0, 0, time.UTC), got["odt"])
	assert.Equal(t, time.Date(1979, 5, 27, 7, 32, 0, 0, time.UTC), got["ldt"])
	assert.Equal(t, "07:32:00", got["lt"])
	assert.Equal(t, []any{[]any{1, 2}, []any{"a"}}, got["arr"])
	assert.Equal(t, []any{map[string]any{"a": 1}, map[string]any{"a": 2}}, got["tables"])

	n, err := tomlScalarNode(uint64(7))
	require.NoError(t, err)
	assert.Equal(t, "7", n.Value)
	// go-toml v1 cannot parse a bare local date in every position, so
	// the LocalDate branch is driven directly.
	n, err = tomlScalarNode(toml.LocalDate{Year: 1979, Month: 5, Day: 27})
	require.NoError(t, err)
	assert.Equal(t, "!!timestamp", n.Tag)
	assert.Equal(t, "1979-05-27", n.Value)
}

func TestTOMLConversionSortsKeys(t *testing.T) {
	tree, err := toml.Load("zeta = 1\nalpha = 2\nmid = { z = 1, a = 2 }\n")
	require.NoError(t, err)
	out, err := tomlTableToYAML(tree)
	require.NoError(t, err)
	assert.Equal(t, "\"alpha\": 2\n\"mid\":\n    \"a\": 2\n    \"z\": 1\n\"zeta\": 1\n", string(out))
}

func TestTOMLConversionRejectsUnsupportedValues(t *testing.T) {
	_, err := tomlToNode([]any{complex(1, 2)})
	assert.ErrorContains(t, err, "unsupported TOML value type complex128")

	tree, err := toml.Load("[a]\nx = 1\n")
	require.NoError(t, err)
	tree.SetPath([]string{"a", "bad"}, struct{}{})
	_, err = tomlTableToYAML(tree)
	assert.ErrorContains(t, err, "a: bad: unsupported TOML value type")

	trees := []*toml.Tree{tree}
	_, err = tomlToNode(trees)
	assert.Error(t, err)
}

func TestLoadPyproject_OffsetDatetimeConverts(t *testing.T) {
	dir := t.TempDir()
	p := writeCfg(t, dir, "pyproject.toml", "[tool.mdsmith]\nfiles = [\"a.md\"]\nstamp = 1979-05-27T00:32:00-07:00\n")
	_, err := loadPyproject(p)
	require.NoError(t, err)
}
