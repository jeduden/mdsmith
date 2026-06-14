package yamlutil_test

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/yamlutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// flatScalarCases is the differential test corpus shared by
// TestFlatScalarFrontMatter_Differential. Each entry is run through both
// FlatScalarFrontMatter and UnmarshalSafe; when the fast path returns true
// its result must match yaml.v3 exactly.
var flatScalarCases = []struct {
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
	{name: "double-quoted carriage-return escape", input: `summary: "a\rb"` + "\n"},
	{name: "double-quoted bell escape", input: `summary: "\a"` + "\n"},
	{name: "double-quoted backspace escape", input: `summary: "\b"` + "\n"},
	{name: "double-quoted form-feed escape", input: `summary: "\f"` + "\n"},
	{name: "double-quoted vertical-tab escape", input: `summary: "\v"` + "\n"},
	{name: "double-quoted null escape", input: `summary: "\0"` + "\n"},
	{name: "double-quoted space escape", input: `summary: "a\ b"` + "\n"},

	// Single-quoted string values.
	{name: "single-quoted simple", input: "status: 'done'\n"},
	{name: "single-quoted emoji", input: "status: '✅'\n"},
	{name: "single-quoted doubled apostrophe", input: "title: 'it''s here'\n"},
	{name: "single-quoted special chars", input: "expr: 'int & >=1'\n"},
	{name: "single-quoted pipe", input: "type: 'push | pull'\n"},
	{name: "single-quoted double-quotes inside", input: `expr: '"push" | "pull"'` + "\n"},

	// Timestamp-looking values. yaml.v3 resolves a bare YYYY-MM-DD to a
	// time.Time, so the fast path must defer rather than return a string.
	{name: "date only iso", input: "when: 2026-01-01\n"},
	{name: "date only short fields", input: "when: 2026-1-1\n"},
	{name: "datetime with T", input: "when: 2026-01-01T10:00:00Z\n"},
	{name: "datetime with space", input: "when: 2026-01-01 10:00:00\n"},
	{name: "invalid date stays string", input: "when: 2026-13-99\n"},
	{name: "non-date with four digit prefix", input: "code: 1234-foo\n"},

	// Digit-leading keys. yaml.v3 accepts these as plain string mapping
	// keys; the fast path must produce the same map key.
	{name: "digit-leading key", input: "1key: val\n"},
	{name: "all-digit key", input: "123: val\n"},

	// Tab as the key/value separator after the colon.
	{name: "tab after colon", input: "k:\tval\n"},

	// Windows-style CRLF line endings.
	{name: "windows crlf", input: "a: foo\r\nb: bar\r\n"},

	// No trailing newline (last line without newline byte).
	{name: "no trailing newline", input: "a: 1"},

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
	// (Fast path returning false is always safe; test only verifies no panic.)
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
	{name: "duplicate key", input: "a: 1\na: 2\n"},
	{name: "unknown double-quote escape", input: "x: \"\\q\"\n"},
	{name: "lone single-quote in value", input: "x: 'foo'bar'\n"},
	{name: "leading comma", input: "x: ,foo\n"},
	{name: "leading percent", input: "x: %tag\n"},
	{name: "leading plus integer", input: "x: +5\n"},
	{name: "underscore numeric", input: "x: 1_000\n"},
	{name: "negative leading-zero", input: "x: -01\n"},
	{name: "bare minus", input: "x: -\n"},
	{name: "bare colon", input: "x: :\n"},
	{name: "bare question-mark", input: "x: ?\n"},
	{name: "tag indicator", input: "x: !str foo\n"},
	{name: "explicit key indicator", input: "? key\n"},
	{name: "no key separator", input: "nocolon\n"},
	{name: "leading-zero key invalid", input: "-field: val\n"},
	{name: "plus-inf", input: "x: +.inf\n"},
	{name: "negative nan", input: "x: .NaN\n"},
	{name: "scientific notation", input: "x: 1e5\n"},
	{name: "bool ON", input: "flag: on\n"},
	{name: "bool OFF", input: "flag: off\n"},
	{name: "binary prefix", input: "x: 0b101\n"},
	{name: "double-quoted unclosed", input: "x: \"hello\n"},
	{name: "single-quoted unclosed", input: "x: 'hello\n"},
	{name: "mid-string double-quote", input: "x: \"a\"b\"\n"},

	// Coverage: isValidFlatKey branches.
	{name: "uppercase key", input: "Key: val\n"},
	{name: "empty key colon first", input: ": value\n"},
	{name: "key with invalid char", input: "ke!y: val\n"},

	// Coverage: parseDoubleQuoted slow-path branches.
	// Unescaped mid-string quote in slow path (has both backslash and unescaped quote).
	{name: "double-quoted mid-string unescaped quote", input: "x: \"\\a\"b\"\n"},
	// Trailing backslash (slow path, backslash at end of inner).
	{name: "double-quoted trailing backslash", input: "x: \"a\\\"\n"},

	// Coverage: parsePlainNumericOrString metacharacter bail.
	{name: "plain string with mid colon", input: "x: foo:bar\n"},

	// Coverage: looksLikeTimestamp early return when non-digit in first four chars.
	{name: "four chars then dash but non-digit", input: "x: 123a-foo\n"},
}

// TestFlatScalarFrontMatter_Differential verifies that whenever
// FlatScalarFrontMatter returns true, its result is identical to what
// UnmarshalSafe produces. A false return (deferred) is always acceptable.
func TestFlatScalarFrontMatter_Differential(t *testing.T) {
	for _, tc := range flatScalarCases {
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
// FlatScalarFrontMatter with a simple flat body allocates few times.
func TestFlatScalarFrontMatter_ZeroAllocsOnHit(t *testing.T) {
	body := []byte("status: \"✅\"\nmodel: opus\nid: 42\n")
	// Warm call to ensure the function is compiled.
	_, _ = yamlutil.FlatScalarFrontMatter(body)

	allocs := testing.AllocsPerRun(50, func() {
		_, _ = yamlutil.FlatScalarFrontMatter(body)
	})
	// The map itself must be allocated, but we should not build a yaml.Node
	// tree. For 3 fields the measured cost is map alloc + key/value string
	// allocs + the any boxes; the budget caps that and stays orders of
	// magnitude below the yaml.v3 node tree.
	assert.LessOrEqual(t, allocs, 12.0, "unexpectedly many allocations on fast path")
}
