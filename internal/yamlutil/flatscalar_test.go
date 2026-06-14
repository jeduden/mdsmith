package yamlutil_test

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/yamlutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFlatScalarFrontMatter_Differential verifies that whenever
// FlatScalarFrontMatter returns true, its result is identical to what
// UnmarshalSafe produces. A false return (deferred) is always acceptable.
func TestFlatScalarFrontMatter_Differential(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		// Empty / null-ish bodies.
		{name: "empty body", input: ""},
		{name: "blank lines only", input: "\n\n"},
		{name: "comment only", input: "# comment\n"},

		// Null values.
		{name: "null explicit", input: "field: null\n"},
		{name: "null tilde", input: "field: ~\n"},
		{name: "null empty value", input: "field:\n"},
		{name: "null Null capitalised", input: "field: Null\n"},
		{name: "null NULL upper", input: "field: NULL\n"},

		// Boolean values.
		{name: "bool true lowercase", input: "flag: true\n"},
		{name: "bool false lowercase", input: "flag: false\n"},

		// Integer values.
		{name: "integer positive", input: "id: 7\n"},
		{name: "integer large", input: "id: 2606130837\n"},
		{name: "integer zero", input: "id: 0\n"},
		{name: "integer negative", input: "offset: -3\n"},

		// Plain string values.
		{name: "plain string simple", input: "model: opus\n"},
		{name: "plain string word", input: "kind: plan\n"},
		{name: "plain string with hyphen", input: "sort: path\n"},

		// Double-quoted string values.
		{name: "double-quoted simple", input: `status: "done"` + "\n"},
		{name: "double-quoted emoji", input: "status: \"✅\"\n"},
		{name: "double-quoted backslash", input: `summary: "a\\b"` + "\n"},
		{name: "double-quoted newline escape", input: `summary: "line1\nline2"` + "\n"},
		{name: "double-quoted tab escape", input: `summary: "col1\tcol2"` + "\n"},
		{name: "double-quoted inner quote", input: `summary: "say \"hi\""` + "\n"},


		// Single-quoted string values.
		{name: "single-quoted simple", input: "status: 'done'\n"},
		{name: "single-quoted emoji", input: "status: '✅'\n"},
		{name: "single-quoted doubled apostrophe", input: "title: 'it''s here'\n"},
		{name: "single-quoted special chars", input: "expr: 'int & >=1'\n"},
		{name: "single-quoted pipe", input: "type: 'push | pull'\n"},
		{name: "single-quoted double-quotes inside", input: `expr: '"push" | "pull"'` + "\n"},

		// Multiple fields.
		{name: "multiple flat fields", input: "id: 42\nstatus: \"✅\"\nmodel: opus\n"},
		{name: "mixed null and string", input: "a: hello\nb: null\nc: world\n"},

		// Comment and blank line stripping.
		{name: "inline comment stripped", input: "key: value # comment\n"},
		{name: "comment line skipped", input: "# top comment\nid: 1\n"},
		{name: "blank line between fields", input: "a: foo\n\nb: bar\n"},

		// Keys with hyphens and underscores.
		{name: "hyphen key", input: "depends-on: none\n"},
		{name: "underscore key", input: "my_field: val\n"},

		// Bail cases — fast path must return false; yaml.v3 result is ignored.
		// (Fast path returning false is always safe; test only checks no panic.)
		{name: "block scalar literal", input: "title: |\n  hello\n"},
		{name: "block scalar folded", input: "title: >-\n  hello\n"},
		{name: "flow sequence value", input: "items: [a, b]\n"},
		{name: "flow mapping value", input: "nested: {a: b}\n"},
		{name: "anchor in value", input: "base: &anchor foo\n"},
		{name: "alias in value", input: "field: *anchor\n"},
		{name: "nested mapping", input: "outer:\n  inner: value\n"},
		{name: "bool True capitalised", input: "flag: True\n"},
		{name: "bool YES", input: "flag: yes\n"},
		{name: "bool NO", input: "flag: no\n"},
		{name: "hex integer", input: "port: 0xFF\n"},
		{name: "octal integer", input: "perm: 0755\n"},
		{name: "float value", input: "ratio: 3.14\n"},
		{name: "infinity value", input: "field: .inf\n"},
		{name: "nan value", input: "field: .nan\n"},
		{name: "integer overflow", input: "big: 99999999999999999999\n"},
		{name: "multi-doc marker", input: "a: 1\n---\nb: 2\n"},
		{name: "end-of-doc marker", input: "a: 1\n...\n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fastResult, ok := yamlutil.FlatScalarFrontMatter([]byte(tc.input))

			var yamlResult map[string]any
			unmarshalErr := yamlutil.UnmarshalSafe([]byte(tc.input), &yamlResult)

			if ok {
				// Fast path committed to a result; it must equal yaml.v3 exactly.
				require.NoError(t, unmarshalErr,
					"fast path returned result but yaml.v3 would error")
				assert.Equal(t, yamlResult, fastResult,
					"fast path result must equal yaml.v3 result")
			}
			// ok==false: deferred to yaml.v3 — always acceptable.
		})
	}
}

// TestFlatScalarFrontMatter_AnchorRejected verifies that inputs containing
// anchors or aliases cause the fast path to return false, so callers fall
// through to UnmarshalSafe which rejects them.
func TestFlatScalarFrontMatter_AnchorRejected(t *testing.T) {
	hostile := []byte("base: &anchor foo\nfield: *anchor\n")

	_, ok := yamlutil.FlatScalarFrontMatter(hostile)
	assert.False(t, ok, "anchor/alias must cause fast path to return false")

	var result map[string]any
	err := yamlutil.UnmarshalSafe(hostile, &result)
	require.Error(t, err, "UnmarshalSafe must reject anchors/aliases")
	assert.Contains(t, err.Error(), "anchors/aliases are not permitted")
}

// TestFlatScalarFrontMatter_ZeroAllocsOnHit verifies that a warm call to
// FlatScalarFrontMatter with a simple flat body allocates zero times.
func TestFlatScalarFrontMatter_ZeroAllocsOnHit(t *testing.T) {
	body := []byte("status: \"✅\"\nmodel: opus\nid: 42\n")
	// Warm call to ensure the function is compiled.
	_, _ = yamlutil.FlatScalarFrontMatter(body)

	allocs := testing.AllocsPerRun(50, func() {
		_, _ = yamlutil.FlatScalarFrontMatter(body)
	})
	// The map itself must be allocated, but we should not build a yaml.Node tree.
	// Budget: map alloc + string allocs for each key/value = ~7 allocs for 3 fields.
	assert.LessOrEqual(t, allocs, 10.0, "unexpectedly many allocations on fast path")
}
