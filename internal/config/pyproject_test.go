//go:build !wasm

package config

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
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
h = 2.5
big = 1e21
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
	doc := tomlTableToDoc(tree, nil)
	var got map[string]any
	require.NoError(t, doc.Decode(&got))
	assert.Equal(t, "true", got["s"])
	assert.Equal(t, true, got["b"])
	assert.Equal(t, -3, got["i"])
	assert.Equal(t, 2.0, got["f"])
	assert.Equal(t, 1500.0, got["g"])
	assert.Equal(t, 2.5, got["h"])
	assert.Equal(t, 1e21, got["big"])
	assert.True(t, math.IsInf(got["inf"].(float64), 1))
	assert.True(t, math.IsInf(got["ninf"].(float64), -1))
	assert.True(t, math.IsNaN(got["nan"].(float64)))
	assert.Equal(t, time.Date(1979, 5, 27, 7, 32, 0, 0, time.UTC), got["odt"])
	assert.Equal(t, time.Date(1979, 5, 27, 7, 32, 0, 0, time.UTC), got["ldt"])
	assert.Equal(t, "07:32:00", got["lt"])
	assert.Equal(t, []any{[]any{1, 2}, []any{"a"}}, got["arr"])
	assert.Equal(t, []any{map[string]any{"a": 1}, map[string]any{"a": 2}}, got["tables"])

	n := tomlScalarNode(uint64(7))
	assert.Equal(t, "7", n.Value)
	// go-toml v1 cannot parse a bare local date in every position, so
	// the LocalDate branch is driven directly.
	n = tomlScalarNode(toml.LocalDate{Year: 1979, Month: 5, Day: 27})
	assert.Equal(t, "!!timestamp", n.Tag)
	assert.Equal(t, "1979-05-27", n.Value)
}

func TestTOMLConversionSortsKeys(t *testing.T) {
	tree, err := toml.Load("zeta = 1\nalpha = 2\nmid = { z = 1, a = 2 }\n")
	require.NoError(t, err)
	out, err := yaml.Marshal(tomlTableToDoc(tree, nil))
	require.NoError(t, err)
	assert.Equal(t, "\"alpha\": 2\n\"mid\":\n    \"a\": 2\n    \"z\": 1\n\"zeta\": 1\n", string(out))
}

func TestTOMLConversionStringifiesForeignValues(t *testing.T) {
	// go-toml never yields these; a hand-built tree keeps their text.
	n := tomlConverter{}.value([]any{complex(1, 2)}, toml.Position{})
	require.Len(t, n.Content, 1)
	assert.Equal(t, "!!str", n.Content[0].Tag)
	assert.Equal(t, "(1+2i)", n.Content[0].Value)

	tree, err := toml.Load("[a]\nx = 1\n")
	require.NoError(t, err)
	tree.SetPath([]string{"a", "y"}, struct{}{})
	seq := tomlConverter{}.value([]*toml.Tree{tree}, toml.Position{})
	assert.Equal(t, yaml.SequenceNode, seq.Kind)
	require.Len(t, seq.Content, 1)
}

func TestLoadPyproject_OffsetDatetimeConverts(t *testing.T) {
	dir := t.TempDir()
	p := writeCfg(t, dir, "pyproject.toml", "[tool.mdsmith]\nfiles = [\"a.md\"]\nstamp = 1979-05-27T00:32:00-07:00\n")
	_, err := loadPyproject(p)
	require.NoError(t, err)
}

func TestLoad_DispatchesTOMLToPyproject(t *testing.T) {
	dir := t.TempDir()
	body := "[tool.mdsmith.rules]\nline-length = false\n"
	for _, name := range []string{"pyproject.toml", "foo.toml", "UPPER.TOML"} {
		t.Run(name, func(t *testing.T) {
			cfg, err := Load(writeCfg(t, dir, name, body))
			require.NoError(t, err)
			assert.False(t, cfg.Rules["line-length"].Enabled)
		})
	}
}

func TestLoad_TOMLReadIsSizeCapped(t *testing.T) {
	dir := t.TempDir()
	big := "[tool.mdsmith]\n#" + strings.Repeat("x", int(maxConfigBytes)) + "\n"
	_, err := Load(writeCfg(t, dir, "pyproject.toml", big))
	assert.ErrorContains(t, err, "too large")
}

func TestDiscover_Pyproject(t *testing.T) {
	const withTable = "[project]\nname = \"x\"\n\n[tool.mdsmith]\nfiles = [\"*.md\"]\n"
	const withoutTable = "[project]\nname = \"x\"\n\n[tool.black]\nline-length = 88\n"

	t.Run("pyproject with table is found", func(t *testing.T) {
		dir := t.TempDir()
		py := writeCfg(t, dir, "pyproject.toml", withTable)
		got := Discover(dir)
		assert.Equal(t, py, got)
	})
	t.Run("mdsmith.yml wins in the same directory", func(t *testing.T) {
		dir := t.TempDir()
		writeCfg(t, dir, "pyproject.toml", withTable)
		yml := writeCfg(t, dir, ".mdsmith.yml", "rules: {}\n")
		got := Discover(dir)
		assert.Equal(t, yml, got)
	})
	t.Run("nearest file wins across directories", func(t *testing.T) {
		root := t.TempDir()
		writeCfg(t, root, ".mdsmith.yml", "rules: {}\n")
		py := writeCfg(t, root, "sub/pyproject.toml", withTable)
		got := Discover(filepath.Join(root, "sub"))
		assert.Equal(t, py, got)
	})
	t.Run("pyproject without table is skipped", func(t *testing.T) {
		root := t.TempDir()
		yml := writeCfg(t, root, ".mdsmith.yml", "rules: {}\n")
		writeCfg(t, root, "sub/pyproject.toml", withoutTable)
		got := Discover(filepath.Join(root, "sub"))
		assert.Equal(t, yml, got)
	})
	t.Run("git boundary still stops the walk", func(t *testing.T) {
		root := t.TempDir()
		writeCfg(t, root, "pyproject.toml", withTable)
		require.NoError(t, os.MkdirAll(filepath.Join(root, "repo", ".git"), 0o755))
		got := Discover(filepath.Join(root, "repo"))
		assert.Equal(t, "", got)
	})
	t.Run("malformed pyproject naming the table is a config source", func(t *testing.T) {
		dir := t.TempDir()
		py := writeCfg(t, dir, "pyproject.toml", "[tool.mdsmith]\nfiles = [\n")
		got := Discover(dir)
		assert.Equal(t, py, got, "so the load reports the syntax error")
	})
	t.Run("malformed pyproject without the table is skipped", func(t *testing.T) {
		dir := t.TempDir()
		writeCfg(t, dir, "pyproject.toml", "[project\n")
		got := Discover(dir)
		assert.Equal(t, "", got)
	})
}

func TestProbePyproject_Source(t *testing.T) {
	dir := t.TempDir()
	cases := map[string]bool{
		"[tool.mdsmith]\n":                        true,
		"[tool.mdsmith.rules]\nx = false\n":       true,
		"[tool]\nmdsmith = { files = [] }\n":      true,
		"tool.mdsmith.files = []\n":               true,
		"[tool.mdsmithx]\n":                       false,
		"[tools.mdsmith]\n":                       false,
		"[tool.mdsmith\n":                         true,
		"  [ tool.mdsmith.kinds ] # broken = \n=": true,
		"# [tool.mdsmith]\n[x\n":                  false,
	}
	i := 0
	for body, want := range cases {
		i++
		p := writeCfg(t, dir, fmt.Sprintf("p%d.toml", i), body)
		source, _ := probePyproject(p)
		assert.Equal(t, want, source, "body %q", body)
	}
	source, hint := probePyproject(filepath.Join(dir, "missing.toml"))
	assert.False(t, source)
	assert.Equal(t, "", hint)
}

// deeplyNested returns a TOML value nested n arrays deep.
func deeplyNested(n int) string {
	return strings.Repeat("[", n) + strings.Repeat("]", n)
}

func TestTOMLNestingExceeds(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want bool
	}{
		{"at the limit", "a = " + deeplyNested(3), false},
		{"past the limit", "a = " + deeplyNested(4), true},
		{"inline tables count", "a = { b = { c = { d = { e = 1 } } } }", true},
		{"table headers close", "[a]\n[[b]]\n[[b]]\n[c.d]\n", false},
		{"brackets in a basic string", `a = "[[[[[[" `, false},
		{"escaped quote stays in string", `a = "\"[[[[[" `, false},
		{"brackets in a literal string", "a = '[[[[[['", false},
		{"brackets in a multi-line string", "a = \"\"\"\n[[[[[\n\"\"\"\n", false},
		{"brackets in a multi-line literal", "a = '''\n[[[[[\n'''\n", false},
		{"brackets in a comment", "# [[[[[[\na = 1\n", false},
		{"unterminated string ends at newline", "a = \"[[\nb = " + deeplyNested(4), true},
		{"string closers do not hide depth", `a = [ "]", [ "]", [ "]", [ "]", 1 ] ] ] ]`, true},
		// Each dot of a dotted key or table header opens one more table.
		{"dotted key at the limit", "a.b.c.d = 1", false},
		{"dotted key past the limit", "a.b.c.d.e = 1", true},
		{"dotted header past the limit", "[a.b.c.d]\n", true},
		{"dotted array-of-tables header", "[[a.b.c.d]]\n", true},
		{"quoted dotted segments", `"a"."b".'c'.d.e = 1`, true},
		{"spaced dotted key", "a . b . c . d . e = 1", true},
		{"dotted key in an inline table", "a = { b.c.d.e = 1 }", true},
		{"dotted key in an inline table at the limit", "a = { b.c.d = 1 }", false},
		// A dotted key's tables stay open inside the inline table or
		// array that is its value, so its dots add to the depth there.
		{"dotted key opening an inline table", "a.b = { c.d.e = 1 }", true},
		{"dotted key opening an inline table at the limit", "a.b = { c.d = 1 }", false},
		{"dotted keys of nested inline tables add up", "a.b = { c.d = { e = 1 } }", true},
		{"dotted key opening an array", "a.b = [[[1]]]", true},
		{"a closed inline table gives its depth back", "a.b = { c = 1 }\nd = [[[1]]]\n", false},
		{"a stray closer is ignored", "] }\na = [[[1]]]\n", false},
		{"floats in an array do not add up", "a = [1.5, 2.5, 3.5, 4.5, 5.5]", false},
		{"dotted keys on separate lines", "a.b.c = 1\na.b.d = 2\nx.y.z = 3\n", false},
		{"date-time fraction", "a = 1979-05-27T07:32:00.999999", false},
		{"dots in a string", `a = "x.y.z.w.v.u"`, false},
		{"dots in a comment", "a = 1 # x.y.z.w.v.u\n", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tomlNestingExceeds([]byte(tc.src), 3))
		})
	}
}

func TestTOMLStringEnd(t *testing.T) {
	assert.Equal(t, 3, tomlStringEnd([]byte(`"ab" = 1`), 0))
	assert.Equal(t, 1, tomlStringEnd([]byte(`""`), 0))
	assert.Equal(t, 4, tomlStringEnd([]byte(`"\"a"`), 0))
	assert.Equal(t, 2, tomlStringEnd([]byte(`'\'`), 0), "no escapes in a literal string")
	assert.Equal(t, 7, tomlStringEnd([]byte("\"\"\"a\n\"\"\""), 0))
	assert.Equal(t, 7, tomlStringEnd([]byte("'''a\n'''"), 0))
	assert.Equal(t, 7, tomlStringEnd([]byte(`"""\""""`), 0), "escaped quote in a multi-line string")
	assert.Equal(t, 2, tomlStringEnd([]byte("\"a\nb\""), 0), "a single-line string ends at a newline")
	assert.Equal(t, 3, tomlStringEnd([]byte(`"ab`), 0), "unterminated")
	assert.Equal(t, 5, tomlStringEnd([]byte(`"""ab`), 0), "unterminated multi-line")
}

// A pyproject.toml nested deep enough to overflow go-toml's recursive
// parser is rejected before parsing, both by discovery and by Load,
// instead of crashing the process.
func TestPyproject_DeepNestingIsRejectedNotFatal(t *testing.T) {
	dir := t.TempDir()
	// 500k levels stays under the maxConfigBytes read cap yet is deep
	// enough to overflow go-toml's parser stack.
	deep := deeplyNested(500000)
	plain := writeCfg(t, dir, "plain.toml", "[project]\nx = "+deep+"\n")
	source, hint := probePyproject(plain)
	assert.False(t, source)
	assert.Equal(t, "", hint)

	cfg := writeCfg(t, dir, "pyproject.toml", "[tool.mdsmith.rules.line-length]\nx = "+deep+"\n")
	source, _ = probePyproject(cfg)
	assert.True(t, source, "a file naming the table is still a source, so Load reports why")
	_, err := Load(cfg)
	assert.ErrorContains(t, err, "nest deeper than")
}

// A dotted key or table header with more segments than maxTOMLNesting
// is rejected before parsing: each segment is one more nested table
// for the TOML-to-YAML conversion and the decoder to recurse through.
func TestPyproject_DeepDottedKeyIsRejected(t *testing.T) {
	dir := t.TempDir()
	segs := strings.Repeat(".a", maxTOMLNesting+1)
	header := writeCfg(t, dir, "h/pyproject.toml", "[tool.mdsmith.rules.line-length"+segs+"]\n")
	_, err := Load(header)
	assert.ErrorContains(t, err, "nest deeper than")
	key := writeCfg(t, dir, "k/pyproject.toml", "[tool.mdsmith]\nrules"+segs+" = 1\n")
	_, err = Load(key)
	assert.ErrorContains(t, err, "nest deeper than")
}

// Short dotted keys that each open an inline table nest as deep as one
// long key: a few hundred levels of nine-dot keys build a table chain
// far past maxTOMLNesting although no single key or bracket run does.
func TestPyproject_DottedInlineTableChainIsRejected(t *testing.T) {
	const levels = maxTOMLNesting/10 + 1
	body := strings.Repeat("a.a.a.a.a.a.a.a.a.a = { ", levels) + "x = 1" + strings.Repeat(" }", levels)
	p := writeCfg(t, t.TempDir(), "pyproject.toml", "[tool.mdsmith.rules]\nr = "+"{ "+body+" }\n")
	_, err := Load(p)
	assert.ErrorContains(t, err, "nest deeper than")
}

// Quoted keys reach the config verbatim: `Get` would split `"a.b"` on
// its dot and lose the value, and return the table itself for `""`,
// recursing without end.
func TestTOMLConversionKeepsQuotedKeysVerbatim(t *testing.T) {
	src := "[x]\n\"a.b\" = 1\n\"\" = 2\nplain = { \"c.d\" = 3 }\n"
	tree, err := toml.Load(src)
	require.NoError(t, err)
	doc := tomlTableToDoc(tree.GetPath([]string{"x"}).(*toml.Tree), []byte(src))
	var got map[string]any
	require.NoError(t, doc.Decode(&got))
	assert.Equal(t, map[string]any{"a.b": 1, "": 2, "plain": map[string]any{"c.d": 3}}, got)

	line, col, ok := pyprojectResolver(doc).Resolve(KeyPath{"a.b"})
	require.True(t, ok)
	assert.Equal(t, 2, line)
	assert.Equal(t, 1, col)
}

func TestLoadPyproject_EmptyKeyLoads(t *testing.T) {
	p := writeCfg(t, t.TempDir(), "pyproject.toml", "[tool.mdsmith]\n\"\" = 1\nfiles = [\"*.md\"]\n")
	cfg, err := Load(p)
	require.NoError(t, err)
	assert.Equal(t, []string{"*.md"}, cfg.Files)
}

func TestFileIn(t *testing.T) {
	dir := t.TempDir()
	assert.Equal(t, "", FileIn(dir))
	py := writeCfg(t, dir, "pyproject.toml", "[tool.mdsmith]\n")
	assert.Equal(t, py, FileIn(dir))
	yml := writeCfg(t, dir, ".mdsmith.yml", "rules: {}\n")
	assert.Equal(t, yml, FileIn(dir))
}

func TestIsConfigFile(t *testing.T) {
	for path, want := range map[string]bool{
		"/p/.mdsmith.yml":      true,
		"/p/pyproject.toml":    true,
		"pyproject.toml":       true,
		"/p/my-pyproject.toml": false,
		"/p/foo.toml":          false,
		"/p/doc.md":            false,
		"/p/notes.mdsmith.yml": false,
	} {
		assert.Equal(t, want, IsConfigFile(path), path)
	}
}

// TestSidecarOwnerDir pins which files Load reads beside a config —
// the YAML files directly under .mdsmith/{kinds,conventions,schemas,
// wordlists}/ — and the directory whose config reads them.
func TestSidecarOwnerDir(t *testing.T) {
	p := filepath.Join(string(filepath.Separator), "p")
	for _, sub := range []string{"kinds", "conventions", "schemas", "wordlists"} {
		// The loaders match the extension in any case, so a `.YAML`
		// sidecar is read and must count too.
		for _, ext := range []string{".yml", ".yaml", ".YML", ".Yaml"} {
			dir, ok := SidecarOwnerDir(filepath.Join(p, ".mdsmith", sub, "x"+ext))
			assert.True(t, ok, sub+ext)
			assert.Equal(t, p, dir, sub+ext)
		}
	}
	for _, path := range []string{
		filepath.Join(p, ".mdsmith", "kinds", "x.md"),
		filepath.Join(p, ".mdsmith", "other", "x.yml"),
		filepath.Join(p, "mdsmith", "kinds", "x.yml"),
		filepath.Join(p, ".mdsmith", "x.yml"),
		filepath.Join(p, ".mdsmith", "kinds", "deep", "x.yml"),
	} {
		_, ok := SidecarOwnerDir(path)
		assert.False(t, ok, path)
	}
}

// discoverHints returns only the hints of a DiscoverWithHints walk.
func discoverHints(start string) []string {
	_, hints := DiscoverWithHints(start)
	return hints
}

func TestDiscover_PluralToolsTableIsHintedNotUsed(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(root, ".git"), 0o755))
	py := writeCfg(t, root, "sub/pyproject.toml", "[tools.mdsmith]\nfiles = [\"*.md\"]\n")
	start := filepath.Join(root, "sub")

	got := Discover(start)
	assert.Equal(t, "", got, "a plural table is not a config source")
	assert.Equal(t, []string{
		py + ": [tools.mdsmith] is not read; rename the table to [tool.mdsmith]",
	}, discoverHints(start))

	// A singular table alongside the plural one is used, with no hint.
	writeCfg(t, root, "sub/pyproject.toml", "[tools.mdsmith]\n[tool.mdsmith]\nfiles = []\n")
	got = Discover(start)
	assert.Equal(t, py, got)
	assert.Empty(t, discoverHints(start))
}

func TestProbePyproject_PluralHint(t *testing.T) {
	dir := t.TempDir()
	for name, body := range map[string]string{
		"bad.toml":  "[tools.mdsmith\n",
		"none.toml": "[project]\n",
		"both.toml": "[tools.mdsmith]\n[tool.mdsmith]\n",
	} {
		_, hint := probePyproject(writeCfg(t, dir, name, body))
		assert.Equal(t, "", hint, name)
	}
	plural := writeCfg(t, dir, "plural.toml", "[tools.mdsmith]\n")
	source, hint := probePyproject(plural)
	assert.False(t, source)
	assert.Equal(t, plural+": [tools.mdsmith] is not read; rename the table to [tool.mdsmith]", hint)
}

// A pyproject.toml over the size cap that opens [tool.mdsmith] within
// the cap is still selected, so Load fails loudly with the size error
// as an oversized .mdsmith.yml does; one that never names the table
// there is skipped like any pyproject.toml without it.
func TestProbePyproject_OversizedFile(t *testing.T) {
	dir := t.TempDir()
	pad := "# " + strings.Repeat("x", int(maxConfigBytes)) + "\n"
	withTable := writeCfg(t, dir, "with.toml", "[tool.mdsmith]\nfiles = []\n"+pad)
	source, hint := probePyproject(withTable)
	assert.True(t, source)
	assert.Equal(t, "", hint)
	_, err := Load(withTable)
	assert.ErrorContains(t, err, "too large")

	without := writeCfg(t, dir, "without.toml", "[project]\n"+pad+"[tool.mdsmith]\n")
	source, hint = probePyproject(without)
	assert.False(t, source, "the table past the cap is not seen")
	assert.Equal(t, "", hint)
}

// A pyproject.toml that exists but cannot be read is skipped with a
// hint, so a config it may hold is not dropped silently.
func TestProbePyproject_UnreadableFileHints(t *testing.T) {
	p := filepath.Join(t.TempDir(), "pyproject.toml")
	require.NoError(t, os.Mkdir(p, 0o755)) // opens, but reading fails
	source, hint := probePyproject(p)
	assert.False(t, source)
	assert.Contains(t, hint, p+": cannot read; a [tool.mdsmith] table in it is not used: ")
}

func TestDiscoverWithHints(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(root, ".git"), 0o755))
	yml := writeCfg(t, root, ".mdsmith.yml", "rules: {}\n")
	py := writeCfg(t, root, "sub/pyproject.toml", "[tools.mdsmith]\n")
	found, hints := DiscoverWithHints(filepath.Join(root, "sub"))
	assert.Equal(t, yml, found)
	assert.Equal(t, []string{py + ": [tools.mdsmith] is not read; rename the table to [tool.mdsmith]"}, hints)
}

func TestLoadPyproject_PluralOnlyErrorCarriesHint(t *testing.T) {
	p := writeCfg(t, t.TempDir(), "pyproject.toml", "[tools.mdsmith]\nfiles = []\n")
	_, err := Load(p)
	assert.ErrorContains(t, err, "no [tool.mdsmith] table; found [tools.mdsmith] — rename it to [tool.mdsmith]")
}

// countTOMLParses swaps the go-toml parse for one that counts its calls.
// It empties the loadTOML cache first, so a parse an earlier test left
// there for the same bytes cannot hide a parse from the count.
func countTOMLParses(t *testing.T) *int {
	t.Helper()
	resetTOMLCache()
	n := 0
	orig := parseTOMLBytes
	parseTOMLBytes = func(b []byte) (*toml.Tree, error) {
		n++
		return orig(b)
	}
	t.Cleanup(func() {
		parseTOMLBytes = orig
		resetTOMLCache()
	})
	return &n
}

// resetTOMLCache empties the single-entry loadTOML cache.
func resetTOMLCache() {
	lastTOML.Lock()
	defer lastTOML.Unlock()
	lastTOML.data, lastTOML.tree, lastTOML.err = nil, nil, nil
}

// Discovery's probe and the Load that follows parse the chosen
// pyproject.toml once between them.
func TestPyproject_DiscoverThenLoadParsesOnce(t *testing.T) {
	dir := t.TempDir()
	py := writeCfg(t, dir, "pyproject.toml", "[tool.mdsmith.rules]\nline-length = false\n")
	n := countTOMLParses(t)
	found, _ := DiscoverWithHints(dir)
	require.Equal(t, py, found)
	cfg, err := Load(found)
	require.NoError(t, err)
	assert.False(t, cfg.Rules["line-length"].Enabled)
	assert.Equal(t, 1, *n)

	// Changed bytes are parsed afresh.
	writeCfg(t, dir, "pyproject.toml", "[tool.mdsmith.rules]\nline-length = true\n")
	cfg, err = Load(py)
	require.NoError(t, err)
	assert.True(t, cfg.Rules["line-length"].Enabled)
	assert.Equal(t, 2, *n)
}

// A pyproject.toml that never names mdsmith can be neither a config
// source nor earn the plural-table hint, so the probe skips the parse.
func TestProbePyproject_SkipsParseWithoutMdsmith(t *testing.T) {
	dir := t.TempDir()
	p := writeCfg(t, dir, "pyproject.toml", "[project]\nname = \"x\"\n")
	n := countTOMLParses(t)
	source, hint := probePyproject(p)
	assert.False(t, source)
	assert.Equal(t, "", hint)
	assert.Equal(t, 0, *n)
}
