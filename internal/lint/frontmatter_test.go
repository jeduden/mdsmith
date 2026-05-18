package lint

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// StripFrontMatter / CountLines are thin forwards to pkg/markdown;
// their behavior tests (including the block-scalar fence regression)
// live there as TestStripFrontMatter / TestCountLines. The lint-owned
// YAML decoders ParseFrontMatterKinds / ParseFrontMatterFields are
// tested below.

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
