package lint

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var yamlAliasTests = []struct {
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
}

func TestRejectYAMLAliases(t *testing.T) {
	for _, tt := range yamlAliasTests {
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
	t.Run("anchor in front matter is rejected", func(t *testing.T) {
		doc := []byte("---\na: &a [\"lol\"]\nb: &b [*a,*a]\nc: &c [*b,*b]\n---\n# Title\n")
		prefix, content := StripFrontMatter(doc)
		require.NotNil(t, prefix)
		assert.Contains(t, string(content), "# Title")

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
