package yamlutil_test

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/yamlutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

var aliasTests = []struct {
	name    string
	input   string
	wantErr bool
}{
	{"clean YAML", "title: Hello\nauthor: World\n", false},
	{"anchor definition", "base: &base\n  name: foo\n", true},
	{"alias reference", "child:\n  <<: *base\n", true},
	{"ampersand in quoted string", "title: \"Q&A Session\"\n", false},
	{"ampersand in single quoted string", "title: 'Q&A'\n", false},
	{"asterisk in quoted string", "note: \"use *bold* text\"\n", false},
	{"ampersand in unquoted value", "title: Q&A\n", false},
	{"billion laughs chain", "a: &a [\"lol\"]\nb: &b [*a,*a]\nc: &c [*b,*b]\n", true},
	{"empty input", "", false},
	{"asterisk not followed by identifier", "note: 5 * 3 = 15\n", false},
	{"anchor at start of line", "&anchor value\n", true},
	{"alias at start of value", "key: *alias\n", true},
	{"block scalar with ampersand", "key: |\n  &name in block\n", false},
	{"block scalar with asterisk", "key: >\n  *name in folded\n", false},
	{"comment with ampersand", "key: val # &anchor\n", false},
	{"comment with asterisk", "key: val # *alias\n", false},
	{"escaped quote in double string", "key: \"she said \\\"&hello\\\"\"\n", false},
	{"doubled single quote", "key: 'it''s &here'\n", false},
	{"syntax error ignored", "key: [unclosed\n", false},
	{"multi-document clean", "title: a\n---\ntitle: b\n", false},
}

func TestRejectYAMLAliases(t *testing.T) {
	for _, tt := range aliasTests {
		t.Run(tt.name, func(t *testing.T) {
			err := yamlutil.RejectYAMLAliases([]byte(tt.input))
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "anchors/aliases are not permitted")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestUnmarshalSafe(t *testing.T) {
	t.Run("unmarshals clean YAML into struct", func(t *testing.T) {
		var out struct {
			Title string `yaml:"title"`
		}
		err := yamlutil.UnmarshalSafe([]byte("title: Hello\n"), &out)
		require.NoError(t, err)
		assert.Equal(t, "Hello", out.Title)
	})

	t.Run("rejects anchor/alias", func(t *testing.T) {
		var out any
		err := yamlutil.UnmarshalSafe([]byte("a: &a [\"lol\"]\nb: &b [*a,*a]\n"), &out)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "anchors/aliases are not permitted")
	})

	t.Run("returns error on invalid YAML", func(t *testing.T) {
		var out struct{ A int }
		err := yamlutil.UnmarshalSafe([]byte("a: not-a-number\n"), &out)
		require.Error(t, err)
	})

	t.Run("empty input produces zero value", func(t *testing.T) {
		var out struct{ Title string }
		err := yamlutil.UnmarshalSafe([]byte(""), &out)
		require.NoError(t, err)
		assert.Equal(t, "", out.Title)
	})
}

func TestUnmarshalNodeSafe(t *testing.T) {
	t.Run("returns document node for clean YAML", func(t *testing.T) {
		node, err := yamlutil.UnmarshalNodeSafe([]byte("title: Hello\n"))
		require.NoError(t, err)
		assert.Equal(t, yaml.DocumentNode, node.Kind)
		require.NotEmpty(t, node.Content)
		assert.Equal(t, yaml.MappingNode, node.Content[0].Kind)
	})

	t.Run("rejects anchor/alias", func(t *testing.T) {
		_, err := yamlutil.UnmarshalNodeSafe([]byte("a: &a [\"lol\"]\nb: &b [*a,*a]\n"))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "anchors/aliases are not permitted")
	})

	t.Run("returns error on invalid YAML", func(t *testing.T) {
		_, err := yamlutil.UnmarshalNodeSafe([]byte("key: {unclosed\n"))
		require.Error(t, err)
	})

	t.Run("empty input returns empty node", func(t *testing.T) {
		node, err := yamlutil.UnmarshalNodeSafe([]byte(""))
		require.NoError(t, err)
		assert.Equal(t, yaml.Node{}, node)
	})
}

func TestMarshal(t *testing.T) {
	t.Run("marshals struct to YAML", func(t *testing.T) {
		v := struct {
			Title string `yaml:"title"`
		}{Title: "Hello"}
		data, err := yamlutil.Marshal(v)
		require.NoError(t, err)
		assert.Contains(t, string(data), "title: Hello")
	})

	t.Run("marshals nil to null YAML", func(t *testing.T) {
		data, err := yamlutil.Marshal(nil)
		require.NoError(t, err)
		assert.Equal(t, "null\n", string(data))
	})
}

func TestTopLevelMappingLines(t *testing.T) {
	t.Run("maps top-level keys to source lines", func(t *testing.T) {
		node, err := yamlutil.UnmarshalNodeSafe([]byte("id: 1\nname: foo\n"))
		require.NoError(t, err)
		lines := yamlutil.TopLevelMappingLines(&node, 0)
		assert.Equal(t, map[string]int{"id": 1, "name": 2}, lines)
	})

	t.Run("applies lineOffset", func(t *testing.T) {
		node, err := yamlutil.UnmarshalNodeSafe([]byte("id: 1\n"))
		require.NoError(t, err)
		// Offset of 1 covers the opening "---" delimiter when
		// parsing a stripped front-matter body.
		lines := yamlutil.TopLevelMappingLines(&node, 1)
		assert.Equal(t, map[string]int{"id": 2}, lines)
	})

	t.Run("returns nil for empty document", func(t *testing.T) {
		var node yaml.Node
		assert.Nil(t, yamlutil.TopLevelMappingLines(&node, 0))
	})

	t.Run("returns nil for non-mapping root", func(t *testing.T) {
		node, err := yamlutil.UnmarshalNodeSafe([]byte("- a\n- b\n"))
		require.NoError(t, err)
		assert.Nil(t, yamlutil.TopLevelMappingLines(&node, 0))
	})

	t.Run("skips non-scalar keys, keeps scalar siblings", func(t *testing.T) {
		// YAML's explicit `?` syntax allows non-scalar mapping
		// keys (a sequence or a mapping as the key). The helper
		// silently skips them because diagnostics only reference
		// scalar keys; remaining scalar siblings are still
		// returned with their source line.
		src := "" +
			"? [a, b]\n" +
			": list-key-value\n" +
			"id: 1\n"
		node, err := yamlutil.UnmarshalNodeSafe([]byte(src))
		require.NoError(t, err)
		lines := yamlutil.TopLevelMappingLines(&node, 0)
		// id is on the third source line; the non-scalar key on
		// lines 1-2 produces no entry.
		assert.Equal(t, map[string]int{"id": 3}, lines)
	})
}

func TestTopLevelScalarField(t *testing.T) {
	doc, err := yamlutil.UnmarshalNodeSafe([]byte(
		"id: 7\ntitle: \"7\"\nempty:\nlist:\n  - a\n"))
	require.NoError(t, err)

	v, line, ok := yamlutil.TopLevelScalarField(&doc, "id", 1)
	require.True(t, ok)
	assert.Equal(t, "7", v)
	assert.Equal(t, 2, line, "body line 1 shifts by the offset")

	v, _, ok = yamlutil.TopLevelScalarField(&doc, "title", 1)
	require.True(t, ok, "quoted scalar is a scalar")
	assert.Equal(t, "7", v, "quoted and bare ints share text")

	_, _, ok = yamlutil.TopLevelScalarField(&doc, "empty", 1)
	assert.False(t, ok, "null value carries no scalar text")

	_, _, ok = yamlutil.TopLevelScalarField(&doc, "list", 1)
	assert.False(t, ok, "sequence value carries no scalar text")

	_, _, ok = yamlutil.TopLevelScalarField(&doc, "absent", 1)
	assert.False(t, ok)

	_, _, ok = yamlutil.TopLevelScalarField(nil, "id", 1)
	assert.False(t, ok, "nil document degrades to not-found")
}

func TestTopLevelScalarField_CanonicalSpellings(t *testing.T) {
	doc, err := yamlutil.UnmarshalNodeSafe([]byte(
		"hex: 0x10\nunders: 1_000\nflag: True\nf: 7.0\nbig: 99999999999999999999999\n"))
	require.NoError(t, err)

	v, _, ok := yamlutil.TopLevelScalarField(&doc, "hex", 1)
	require.True(t, ok)
	assert.Equal(t, "16", v, "hex ints canonicalize to decimal")

	v, _, ok = yamlutil.TopLevelScalarField(&doc, "unders", 1)
	require.True(t, ok)
	assert.Equal(t, "1000", v, "underscore ints canonicalize")

	v, _, ok = yamlutil.TopLevelScalarField(&doc, "flag", 1)
	require.True(t, ok)
	assert.Equal(t, "true", v, "bool spellings canonicalize")

	v, _, ok = yamlutil.TopLevelScalarField(&doc, "f", 1)
	require.True(t, ok)
	assert.Equal(t, "7", v, "floats canonicalize via %g")

	v, _, ok = yamlutil.TopLevelScalarField(&doc, "big", 1)
	require.True(t, ok)
	assert.Equal(t, "1e+23", v,
		"yaml.v3 resolves over-int64 literals as !!float; both "+
			"spellings of such a value canonicalize the same way")
}

func TestTopLevelScalarField_NonMappingDocuments(t *testing.T) {
	doc, err := yamlutil.UnmarshalNodeSafe([]byte("- just\n- a list\n"))
	require.NoError(t, err)
	_, _, ok := yamlutil.TopLevelScalarField(&doc, "id", 1)
	assert.False(t, ok, "sequence documents carry no top-level mapping")

	nilRoot := yaml.Node{Content: []*yaml.Node{nil}}
	_, _, ok = yamlutil.TopLevelScalarField(&nilRoot, "id", 1)
	assert.False(t, ok, "a nil root degrades to not-found")
}
