package lint

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRejectYAMLAliases(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{
			name:    "clean YAML",
			input:   "title: Hello\nauthor: World\n",
			wantErr: false,
		},
		{
			name:    "anchor definition",
			input:   "base: &base\n  name: foo\n",
			wantErr: true,
		},
		{
			name:    "alias reference",
			input:   "child:\n  <<: *base\n",
			wantErr: true,
		},
		{
			name:    "ampersand in quoted string value",
			input:   "title: \"Q&A Session\"\n",
			wantErr: false,
		},
		{
			name:    "ampersand in single quoted string",
			input:   "title: 'Q&A'\n",
			wantErr: false,
		},
		{
			name:    "asterisk in quoted string value",
			input:   "note: \"use *bold* text\"\n",
			wantErr: false,
		},
		{
			name:    "ampersand in unquoted value",
			input:   "title: Q&A\n",
			wantErr: false,
		},
		{
			name:    "billion laughs anchor chain",
			input:   "a: &a [\"lol\"]\nb: &b [*a,*a]\nc: &c [*b,*b]\n",
			wantErr: true,
		},
		{
			name:    "empty input",
			input:   "",
			wantErr: false,
		},
		{
			name:    "asterisk not followed by identifier",
			input:   "note: 5 * 3 = 15\n",
			wantErr: false,
		},
		{
			name:    "anchor at start of line",
			input:   "&anchor value\n",
			wantErr: true,
		},
		{
			name:    "alias at start of value",
			input:   "key: *alias\n",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := RejectYAMLAliases([]byte(tt.input))
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "anchors/aliases are not permitted")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestRejectYAMLAliases_FrontMatter(t *testing.T) {
	// Simulate the flow: strip front matter, then check YAML before unmarshal.
	t.Run("anchor in front matter is rejected", func(t *testing.T) {
		doc := []byte("---\na: &a [\"lol\"]\nb: &b [*a,*a]\nc: &c [*b,*b]\n---\n# Title\n")
		prefix, content := StripFrontMatter(doc)
		require.NotNil(t, prefix)
		assert.Contains(t, string(content), "# Title")

		// Extract YAML between --- delimiters.
		delim := []byte("---\n")
		yamlBytes := prefix[len(delim) : len(prefix)-len(delim)]

		err := RejectYAMLAliases(yamlBytes)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "anchors/aliases are not permitted")
	})

	t.Run("clean front matter is accepted", func(t *testing.T) {
		doc := []byte("---\ntitle: \"Q&A Guide\"\nstatus: draft\n---\n# Title\n")
		prefix, _ := StripFrontMatter(doc)
		require.NotNil(t, prefix)

		delim := []byte("---\n")
		yamlBytes := prefix[len(delim) : len(prefix)-len(delim)]

		err := RejectYAMLAliases(yamlBytes)
		assert.NoError(t, err)
	})
}
