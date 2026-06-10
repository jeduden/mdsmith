package markdownlint

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var parseTests = []struct {
	name    string
	input   string
	want    map[string]any
	wantErr string
}{
	{
		name:  "yaml config",
		input: "default: true\nMD013:\n  line_length: 100\nMD033: false\n",
		want: map[string]any{
			"default": true,
			"MD013":   map[string]any{"line_length": 100},
			"MD033":   false,
		},
	},
	{
		name:  "json config",
		input: `{"default": true, "MD013": {"line_length": 100}}`,
		want: map[string]any{
			"default": true,
			"MD013":   map[string]any{"line_length": 100},
		},
	},
	{
		name: "jsonc line and block comments",
		input: `{
  // enable everything
  "default": true,
  /* tighten lines */
  "MD013": {"line_length": 120}
}`,
		want: map[string]any{
			"default": true,
			"MD013":   map[string]any{"line_length": 120},
		},
	},
	{
		name: "jsonc trailing commas",
		input: `{
  "MD024": {"siblings_only": true,},
  "MD033": false,
}`,
		want: map[string]any{
			"MD024": map[string]any{"siblings_only": true},
			"MD033": false,
		},
	},
	{
		name:  "comment markers inside strings survive",
		input: `{"MD044": {"names": ["http://example.com", "a/*b*/c"]}}`,
		want: map[string]any{
			"MD044": map[string]any{
				"names": []any{"http://example.com", "a/*b*/c"},
			},
		},
	},
	{
		name:  "utf8 bom is stripped",
		input: "\xef\xbb\xbf{\"MD033\": false}",
		want:  map[string]any{"MD033": false},
	},
	{
		name:    "empty input",
		input:   "",
		wantErr: "empty",
	},
	{
		name:    "yaml scalar root",
		input:   "just a string\n",
		wantErr: "mapping",
	},
	{
		name:    "yaml alias rejected",
		input:   "a: &x true\nMD013: *x\n",
		wantErr: "aliases",
	},
	{
		name:    "malformed json",
		input:   `{"MD013" true}`,
		wantErr: "parsing",
	},
}

func TestParse(t *testing.T) {
	for _, tt := range parseTests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse([]byte(tt.input))
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

var stripJSONCTests = []struct {
	name  string
	input string
	want  string
}{
	{
		name:  "line comment to eol",
		input: "{\n\"a\": 1 // tail\n}",
		want:  "{\n\"a\": 1 \n}",
	},
	{
		name:  "block comment",
		input: `{"a": /* x */ 1}`,
		want:  `{"a":  1}`,
	},
	{
		name:  "block comment spanning lines keeps newlines",
		input: "{\"a\": 1 /* one\ntwo */, \"b\": 2}",
		want:  "{\"a\": 1 \n, \"b\": 2}",
	},
	{
		name:  "slashes inside string kept",
		input: `{"a": "http://x//y"}`,
		want:  `{"a": "http://x//y"}`,
	},
	{
		name:  "escaped quote does not end string",
		input: `{"a": "q\" // not a comment"}`,
		want:  `{"a": "q\" // not a comment"}`,
	},
	{
		name:  "trailing comma before brace",
		input: `{"a": 1,}`,
		want:  `{"a": 1}`,
	},
	{
		name:  "trailing comma before bracket with space",
		input: `{"a": [1, 2, ]}`,
		want:  `{"a": [1, 2 ]}`,
	},
	{
		name:  "trailing comma before comment then brace",
		input: "{\"a\": 1, // last\n}",
		want:  "{\"a\": 1 \n}",
	},
	{
		name:  "comma inside string kept",
		input: `{"a": "x,}"}`,
		want:  `{"a": "x,}"}`,
	},
	{
		name:  "unterminated block comment dropped",
		input: `{"a": 1} /* open`,
		want:  `{"a": 1} `,
	},
}

func TestStripJSONC(t *testing.T) {
	for _, tt := range stripJSONCTests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, string(stripJSONC([]byte(tt.input))))
		})
	}
}

func TestStripComments(t *testing.T) {
	assert.Equal(t, "{\"a\": 1 \n}", string(stripComments([]byte("{\"a\": 1 // x\n}"))))
	assert.Equal(t, `{"a":  1}`, string(stripComments([]byte(`{"a": /* x */ 1}`))))
	assert.Equal(t, `{"a": "//"}`, string(stripComments([]byte(`{"a": "//"}`))))
}

func TestStripTrailingCommas(t *testing.T) {
	assert.Equal(t, `{"a": 1}`, string(stripTrailingCommas([]byte(`{"a": 1,}`))))
	assert.Equal(t, `{"a": [1,2]}`, string(stripTrailingCommas([]byte(`{"a": [1,2]}`))))
	assert.Equal(t, `{"a": ",}"}`, string(stripTrailingCommas([]byte(`{"a": ",}"}`))))
}

func TestCopyString(t *testing.T) {
	out, i := copyString(nil, []byte(`"a\"b" rest`), 0)
	assert.Equal(t, `"a\"b"`, string(out))
	assert.Equal(t, 6, i)

	out, i = copyString(nil, []byte(`"open`), 0)
	assert.Equal(t, `"open`, string(out))
	assert.Equal(t, 5, i)
}

func TestDiscover(t *testing.T) {
	t.Run("prefers jsonc over yaml", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, dir, ".markdownlint.yaml", "default: true\n")
		writeFile(t, dir, ".markdownlint.jsonc", `{"default": true}`)

		got, err := Discover(dir)
		require.NoError(t, err)
		assert.Equal(t, filepath.Join(dir, ".markdownlint.jsonc"), got)
	})

	t.Run("finds markdownlintrc", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, dir, ".markdownlintrc", `{"default": true}`)

		got, err := Discover(dir)
		require.NoError(t, err)
		assert.Equal(t, filepath.Join(dir, ".markdownlintrc"), got)
	})

	t.Run("nothing found", func(t *testing.T) {
		dir := t.TempDir()
		_, err := Discover(dir)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no markdownlint config found")
	})
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644))
}
