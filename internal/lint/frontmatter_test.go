package lint

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/yamlutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// StripFrontMatter / CountLines are thin forwards to pkg/markdown;
// their behavior tests (including the block-scalar fence regression)
// live there as TestStripFrontMatter / TestCountLines. The lint-owned
// YAML decoders ParseFrontMatterKinds / ParseFrontMatterFields are
// tested below.

func TestFrontMatterYAML(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"nil", "", ""},
		{"typical block", "---\ntitle: hi\n---\n", "title: hi\n"},
		{"empty block", "---\n---\n", ""},
		{
			"inner fence in block scalar kept",
			"---\nnotes: |\n  a\n  ---\n  b\n---\n",
			"notes: |\n  a\n  ---\n  b\n",
		},
		{"no delimiters passes through", "title: hi\n", "title: hi\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, string(FrontMatterYAML([]byte(tt.input))))
		})
	}
}

func TestParseFrontMatterKinds(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "single kind",
			input: "---\nkinds: [plan]\nid: 1\n---\n",
			want:  []string{"plan"},
		},
		{
			name:  "multiple kinds",
			input: "---\nkinds: [tip, worksheet]\ntitle: hello\n---\n",
			want:  []string{"tip", "worksheet"},
		},
		{
			name:  "no kinds field",
			input: "---\ntitle: hello\n---\n",
			want:  nil,
		},
		{
			name:  "nil input",
			input: "",
			want:  nil,
		},
		{
			name:  "empty kinds list",
			input: "---\nkinds: []\n---\n",
			want:  []string{},
		},
	}

	// Invalid YAML returns an error.
	t.Run("invalid yaml returns error", func(t *testing.T) {
		got, err := ParseFrontMatterKinds([]byte("---\nkinds: [[[invalid\n---\n"))
		assert.Nil(t, got)
		assert.Error(t, err)
	})

	// YAML aliases are rejected.
	t.Run("yaml aliases rejected", func(t *testing.T) {
		got, err := ParseFrontMatterKinds([]byte("---\nbase: &a [plan]\nkinds: *a\n---\n"))
		assert.Nil(t, got)
		assert.Error(t, err)
	})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var fm []byte
			if tt.input != "" {
				prefix, _ := StripFrontMatter([]byte(tt.input))
				require.NotNil(t, prefix, "expected front matter in input")
				fm = prefix
			}
			got, err := ParseFrontMatterKinds(fm)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestParseFrontMatterKinds_KeySpellings: every YAML spelling of
// the kinds key reaches the decoder (the fast path must not skip
// them), and typed entries keep their source text.
func TestParseFrontMatterKinds_KeySpellings(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name, input string
		want        []string
	}{
		{"quoted key", "---\n\"kinds\": [a]\n---\n", []string{"a"}},
		{"space before colon", "---\nkinds : [a]\n---\n", []string{"a"}},
		{"merge key", "---\n<<: {kinds: [m]}\n---\n", []string{"m"}},
		{"typed scalars kept as text", "---\nkinds: [a, 42]\n---\n", []string{"a", "42"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := ParseFrontMatterKinds([]byte(tt.input))
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestDecodeFrontMatterHead(t *testing.T) {
	t.Parallel()
	decode := func(t *testing.T, src string) (FrontMatterHead, error) {
		t.Helper()
		doc, err := yamlutil.UnmarshalNodeSafe([]byte(src))
		require.NoError(t, err)
		return DecodeFrontMatterHead(&doc)
	}

	t.Run("empty document", func(t *testing.T) {
		t.Parallel()
		head, err := DecodeFrontMatterHead(&yaml.Node{})
		require.NoError(t, err)
		assert.Zero(t, head.Title.Kind)
		assert.Zero(t, head.Kinds.Kind)
	})

	t.Run("title and kinds", func(t *testing.T) {
		t.Parallel()
		head, err := decode(t, "title: T\nkinds: [a]\nother: 1\n")
		require.NoError(t, err)
		assert.Equal(t, "T", head.Title.Value)
		assert.Equal(t, yaml.SequenceNode, head.Kinds.Kind)
	})

	t.Run("duplicate top-level key errors", func(t *testing.T) {
		t.Parallel()
		_, err := decode(t, "\"\": x\n\"\": y\ntitle: T\n")
		assert.Error(t, err)
	})

	t.Run("non-mapping document errors", func(t *testing.T) {
		t.Parallel()
		_, err := decode(t, "- a\n")
		assert.Error(t, err)
	})
}

func TestFrontMatterHead_KindList(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name, src string
		want      []string
		wantErr   bool
	}{
		{"absent", "title: T\n", nil, false},
		{"null", "kinds: ~\n", nil, false},
		{"list", "kinds: [a, 42]\n", []string{"a", "42"}, false},
		{"scalar errors", "kinds: a\n", nil, true},
		{"mapping entry errors", "kinds: [a, {x: y}]\n", nil, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			doc, err := yamlutil.UnmarshalNodeSafe([]byte(tt.src))
			require.NoError(t, err)
			head, err := DecodeFrontMatterHead(&doc)
			require.NoError(t, err)
			got, err := head.KindList()
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, got)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestUnmarshalFrontMatter(t *testing.T) {
	type fixture struct {
		Title       string         `yaml:"title"`
		Diagnostics []string       `yaml:"diagnostics"`
		Settings    map[string]any `yaml:"settings"`
	}

	t.Run("populates v and signals hadFrontMatter for a valid block", func(t *testing.T) {
		src := []byte("---\ntitle: hi\n---\n\nbody\n")
		var fm fixture
		body, hadFM, err := UnmarshalFrontMatter(src, &fm)
		require.NoError(t, err)
		assert.True(t, hadFM, "valid front matter must report hadFrontMatter=true")
		assert.Equal(t, "hi", fm.Title)
		assert.Equal(t, "\nbody\n", string(body))
	})

	t.Run("hadFrontMatter is false when source has no front matter", func(t *testing.T) {
		src := []byte("body only\n")
		var fm fixture
		body, hadFM, err := UnmarshalFrontMatter(src, &fm)
		require.NoError(t, err)
		assert.False(t, hadFM)
		assert.Equal(t, "", fm.Title)
		assert.Equal(t, "body only\n", string(body))
	})

	t.Run("hadFrontMatter is true for an empty fences-only block", func(t *testing.T) {
		// Distinguishing "no FM" from "empty FM" matters for callers
		// that want to enforce schema (the integration fixture loader
		// uses this to refuse silently-malformed bad fixtures).
		src := []byte("---\n---\n\nbody\n")
		var fm fixture
		_, hadFM, err := UnmarshalFrontMatter(src, &fm)
		require.NoError(t, err)
		assert.True(t, hadFM, "empty fences are still a front-matter block")
	})

	t.Run("hadFrontMatter is true when FM has unrecognised keys", func(t *testing.T) {
		// An unknown key (e.g. a misspelling of "diagnostics" or a
		// schema-mismatched field) decodes into nothing on the target
		// struct, but the FM block is still present. Callers that
		// confuse "v is zero" with "no FM" would silently accept a
		// malformed fixture; hadFrontMatter prevents that.
		src := []byte("---\nunknown_key: oops\n---\n\nbody\n")
		var fm fixture
		_, hadFM, err := UnmarshalFrontMatter(src, &fm)
		require.NoError(t, err)
		assert.True(t, hadFM)
		assert.Nil(t, fm.Diagnostics, "unknown key does not populate the field")
	})

	t.Run("propagates yaml errors and still returns hadFrontMatter=true", func(t *testing.T) {
		// Anchors are rejected by yamlutil.UnmarshalSafe.
		src := []byte("---\nx: &a foo\ny: *a\n---\n\nbody\n")
		var fm fixture
		_, hadFM, err := UnmarshalFrontMatter(src, &fm)
		require.Error(t, err)
		assert.True(t, hadFM, "decode failure still saw a FM block")
	})
}

func TestParseFrontMatterFields(t *testing.T) {
	t.Run("returns parsed mapping", func(t *testing.T) {
		prefix, _ := StripFrontMatter([]byte("---\nstatus: open\nid: 7\n---\n# H\n"))
		got, err := ParseFrontMatterFields(prefix)
		require.NoError(t, err)
		assert.Equal(t, "open", got["status"])
		assert.Equal(t, 7, got["id"])
	})

	t.Run("null value preserved", func(t *testing.T) {
		prefix, _ := StripFrontMatter([]byte("---\nstatus: null\n---\n"))
		got, err := ParseFrontMatterFields(prefix)
		require.NoError(t, err)
		v, ok := got["status"]
		require.True(t, ok, "key should be present")
		assert.Nil(t, v, "null YAML value decodes to nil")
	})

	t.Run("nil-result inputs return nil,nil", func(t *testing.T) {
		cases := map[string]string{
			"no front matter":    "",
			"empty front matter": "---\n---\n# H\n",
			"explicit null":      "---\nnull\n---\n",
		}
		for name, src := range cases {
			t.Run(name, func(t *testing.T) {
				prefix, _ := StripFrontMatter([]byte(src))
				got, err := ParseFrontMatterFields(prefix)
				require.NoError(t, err)
				assert.Nil(t, got)
			})
		}
	})

	t.Run("rejects invalid payloads", func(t *testing.T) {
		cases := map[string]struct {
			src     string
			wantMsg string
		}{
			"yaml aliases":     {"---\nbase: &a x\nkey: *a\n---\n", ""},
			"scalar payload":   {"---\nfoo\n---\n", "mapping"},
			"sequence payload": {"---\n- a\n- b\n---\n", "mapping"},
			"non-string keys":  {"---\n1: foo\n---\n", "keys must be strings"},
		}
		for name, tc := range cases {
			t.Run(name, func(t *testing.T) {
				prefix, _ := StripFrontMatter([]byte(tc.src))
				_, err := ParseFrontMatterFields(prefix)
				require.Error(t, err)
				if tc.wantMsg != "" {
					assert.Contains(t, err.Error(), tc.wantMsg)
				}
			})
		}
	})
}
