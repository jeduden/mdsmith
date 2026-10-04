//go:build !wasm

package config

import (
	"bytes"
	"os"
	"testing"

	"github.com/pelletier/go-toml"
	"gopkg.in/yaml.v3"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadPyproject_PositionedErrors(t *testing.T) {
	cases := []struct {
		name      string
		body      string
		line, col int
	}{
		{"semantic value", `[project]
name = "x"

[tool.mdsmith]
files = ["*.md"]
convention = "nope"
`, 6, 1},
		{"decoder value in sub-table", `[tool.mdsmith.rules]
no-bare-urls = true
line-length = 42
`, 3, 1},
		{"array of tables entry", `[tool.mdsmith]
files = ["*.md"]

[[tool.mdsmith.overrides]]
glob = ["a.md"]

[[tool.mdsmith.overrides]]
glob = ["b.md"]
foreign-regions = [{ start = "<!-- x -->", end = "" }]
`, 9, 44},
		{"kind key", `[tool.mdsmith.kinds.plan]
rules = {}
path-pattern = "plan/[a"
`, 3, 1},
		{"inline table key", `[tool.mdsmith]
rules = { no-bare-urls = true, line-length = 42 }
`, 2, 32},
		{"nested inline table key", `[tool.mdsmith]
kinds = { plan = { extends = "ghost" } }
`, 2, 20},
		{"dotted key", `[tool.mdsmith]
kinds.plan.extends = "ghost"
`, 2, 1},
		{"syntax error", `[tool.mdsmith]
files = ["a.md"]
convention = = "x"
`, 3, 14},
		{"preset issue anchors on convention key", `[tool.mdsmith]

convention = "nope"
`, 3, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := writeCfg(t, t.TempDir(), "pyproject.toml", tc.body)
			_, err := Load(p)
			le := requireLoadError(t, err)
			assert.Equal(t, p, le.File)
			assert.Equal(t, tc.line, le.Line, "err: %v", err)
			assert.Equal(t, tc.col, le.Column, "err: %v", err)
		})
	}
}

func TestLoadPyproject_UnresolvablePathAnchorsOnTableHeader(t *testing.T) {
	// The issue path names a key absent from the table (the wordlist
	// check addresses the convention preset under `convention`, which
	// this config does not set), so the header line is reported.
	p := writeCfg(t, t.TempDir(), "pyproject.toml", "[project]\nname = \"x\"\n\n[tool.mdsmith]\nfiles = []\n")
	r := pyprojectResolver(mustParseTOMLDoc(t, p))
	line, col, ok := r.Resolve(KeyPath{"nope"})
	require.True(t, ok)
	assert.Equal(t, 4, line)
	assert.Equal(t, 1, col)
}

func TestTOMLErrorIssue(t *testing.T) {
	iss := tomlErrorIssue(errorString("(7, 3): boom"), nil)
	var got *Issue
	require.ErrorAs(t, iss, &got)
	assert.Equal(t, 7, got.Line)
	assert.Equal(t, 3, got.Column)
	// The parser's own `(line, col)` prefix counts characters; the
	// diagnostic carries the byte position, so the message drops it.
	assert.Equal(t, "boom", got.Message)
	assert.EqualError(t, got.Unwrap(), "(7, 3): boom", "the cause keeps the parser text")
	plain := errorString("no position")
	assert.Equal(t, plain, tomlErrorIssue(plain, nil))
}

// mustParseTOMLDoc builds the positioned document node for the
// [tool.mdsmith] table of the pyproject.toml at p.
func mustParseTOMLDoc(t *testing.T, p string) *yaml.Node {
	t.Helper()
	data, err := os.ReadFile(p)
	require.NoError(t, err)
	tree, err := toml.LoadBytes(data)
	require.NoError(t, err)
	table, err := mdsmithTable(tree, p)
	require.NoError(t, err)
	return tomlTableToDoc(table, data)
}

type errorString string

func (e errorString) Error() string { return string(e) }

func TestKeyColumn(t *testing.T) {
	cases := []struct {
		line  string
		start int
		key   string
		want  int
	}{
		{"a = 1", 0, "a", 1},
		{`x = { "a" = 1 }`, 0, "a", 7},
		{"x = { 'a' = 1 }", 0, "a", 7},
		{"ab = 1, b = 2", 0, "b", 9}, // "ab" is not a boundary match
		{"b2 = 1, b = 2", 0, "b", 9}, // "b2" is not followed by =
		{"x.b.c = 1", 0, "b", 3},     // dotted key segment
		{"x = { b }", 0, "b", 0},     // no = after the key
		{"b", 0, "b", 0},             // end of line
		{"a = 1, a = 2", 2, "a", 8},  // start offset skips the first
		{"x = \"b\"", 0, "b", 0},     // a quoted value, not a key
		// Text inside a string value or a comment is never a key.
		{`x = "see b.md", b = 1`, 0, "b", 17},
		{"x = 'b = 1', b = 2", 0, "b", 14},
		{"x = 1 # b = 2", 0, "b", 0},
		{`"" = 1`, 0, "", 1}, // the empty key is always quoted
		{"x = 1", 0, "", 0},
		{`x = "b`, 0, "b", 0},    // an unterminated string is no key
		{`x="b" = 1`, 0, "b", 0}, // a quoted key must follow a boundary
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, keyColumn([]byte(tc.line), tc.start, tc.key), "%q", tc.line)
	}
}

func TestFindKeyAndNthBrace(t *testing.T) {
	src := "[t]\nx = 1\n  y = 2\n[u]\nz = 3\nw = [\n  { a = 1 },\n  { a = 2 },\n]\n"
	c := tomlConverter{lines: bytes.Split([]byte(src), []byte("\n"))}
	from := toml.Position{Line: 1, Col: 1}

	assert.Equal(t, toml.Position{Line: 3, Col: 3}, c.findKey(from, "y"))
	assert.Equal(t, from, c.findKey(from, "z"), "stops at the next [header]")
	assert.Equal(t, toml.Position{}, c.findKey(toml.Position{}, "x"), "unknown start stays unknown")

	w := toml.Position{Line: 6, Col: 1}
	assert.Equal(t, toml.Position{Line: 7, Col: 3}, c.nthBrace(w, 0))
	assert.Equal(t, toml.Position{Line: 8, Col: 3}, c.nthBrace(w, 1))
	assert.Equal(t, w, c.nthBrace(w, 5), "fewer braces keeps the key position")
	assert.Equal(t, toml.Position{}, c.nthBrace(toml.Position{}, 0))
	// A start column past the line end clamps rather than panicking.
	assert.Equal(t, toml.Position{Line: 3, Col: 3}, c.findKey(toml.Position{Line: 2, Col: 99}, "y"))
	assert.Equal(t, toml.Position{Line: 7, Col: 3}, c.nthBrace(toml.Position{Line: 6, Col: 99}, 0))
}

func TestTOMLResolverWithoutHeaderPosition(t *testing.T) {
	r := tomlResolver{root: &yaml.Node{Kind: yaml.MappingNode}}
	_, _, ok := r.Resolve(KeyPath{"x"})
	assert.False(t, ok)
}

func TestNthBraceSkipsStringsAndNestedTables(t *testing.T) {
	src := `x = [{ s = "{", t = '{', u = { v = 1 } }, { w = 2 }] # {
y = [{ a = 1 }]
z = { inner = [{ b = 1 }], after = { c = 2 } }
`
	c := tomlConverter{lines: bytes.Split([]byte(src), []byte("\n"))}
	x := toml.Position{Line: 1, Col: 1}
	assert.Equal(t, toml.Position{Line: 1, Col: 6}, c.nthBrace(x, 0))
	assert.Equal(t, toml.Position{Line: 1, Col: 43}, c.nthBrace(x, 1),
		"braces in strings and in a nested table are not elements")
	assert.Equal(t, x, c.nthBrace(x, 2),
		"the scan stops at the array's `]`; the comment and the next line are not elements")
	inner := toml.Position{Line: 3, Col: 7}
	assert.Equal(t, toml.Position{Line: 3, Col: 16}, c.nthBrace(inner, 0))
	assert.Equal(t, inner, c.nthBrace(inner, 1),
		"a table after the array's `]` is not an element")
	// A `}` that closes a table opened before from ends the scan.
	after := toml.Position{Line: 3, Col: 28}
	assert.Equal(t, after, c.nthBrace(after, 1))
}

func TestBraceScanScanLine(t *testing.T) {
	s := braceScan{n: 1}
	_, found, done := s.scanLine([]byte(`a = [{ x = "{" }, # {`), 0)
	assert.False(t, found)
	assert.False(t, done, "the array continues past the comment")
	col, found, _ := s.scanLine([]byte(`  { y = 1 }]`), 0)
	assert.True(t, found, "state carries across lines")
	assert.Equal(t, 2, col)

	_, found, done = (&braceScan{}).scanLine([]byte(`a = []`), 0)
	assert.False(t, found)
	assert.True(t, done, "the array closed with no element")
	_, _, done = (&braceScan{}).scanLine([]byte(`x }`), 0)
	assert.True(t, done, "an enclosing table closed")
}

// A foreign-region marker that contains a brace (a Jinja or Hugo
// delimiter) does not shift the error onto the wrong list entry.
func TestLoadPyproject_BraceInMarkerKeepsEntryPosition(t *testing.T) {
	p := writeCfg(t, t.TempDir(), "pyproject.toml", `[tool.mdsmith]
foreign-regions = [{ start = "{% raw %}", end = "{% endraw %}" }, { start = "{{<", end = "" }]
`)
	_, err := Load(p)
	le := requireLoadError(t, err)
	assert.Equal(t, 2, le.Line)
	assert.Equal(t, 84, le.Column, "the `end` key of the second entry")
}

func TestLoadPyproject_ArrayOfInlineTablesPositions(t *testing.T) {
	p := writeCfg(t, t.TempDir(), "pyproject.toml", `[tool.mdsmith]
foreign-regions = [
  { start = "<!-- a -->", end = "<!-- /a -->" },
  { start = "<!-- b -->", end = "" },
]
`)
	_, err := Load(p)
	le := requireLoadError(t, err)
	assert.Equal(t, 4, le.Line)
	assert.Equal(t, 27, le.Column)
}

// go-toml counts a parse error's column in characters; the reported
// column must be the byte offset the CLI and LSP expect.
func TestLoadPyproject_ParseErrorColumnIsByteOffset(t *testing.T) {
	body := "[tool.mdsmith]\nx = \"éé\" y\n"
	p := writeCfg(t, t.TempDir(), "pyproject.toml", body)
	_, err := Load(p)
	le := requireLoadError(t, err)
	assert.Equal(t, 2, le.Line)
	assert.Equal(t, byteColOf(t, body, 2, "y"), le.Column)
}
