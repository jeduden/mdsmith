package lint

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStripFrontMatter(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantPrefix  string
		wantContent string
	}{
		{
			name:        "with front matter",
			input:       "---\ntitle: hello\n---\n# Heading\n",
			wantPrefix:  "---\ntitle: hello\n---\n",
			wantContent: "# Heading\n",
		},
		{
			name:        "no front matter",
			input:       "# Heading\nSome text\n",
			wantPrefix:  "",
			wantContent: "# Heading\nSome text\n",
		},
		{
			name:        "unclosed front matter",
			input:       "---\ntitle: hello\n# Heading\n",
			wantPrefix:  "",
			wantContent: "---\ntitle: hello\n# Heading\n",
		},
		{
			name:        "empty front matter",
			input:       "---\n---\n# Heading\n",
			wantPrefix:  "---\n---\n",
			wantContent: "# Heading\n",
		},
		{
			name:        "dashes not at start",
			input:       "# Heading\n---\nfoo\n---\n",
			wantPrefix:  "",
			wantContent: "# Heading\n---\nfoo\n---\n",
		},
		{
			name:        "empty input",
			input:       "",
			wantPrefix:  "",
			wantContent: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prefix, content := StripFrontMatter([]byte(tt.input))
			assert.Equal(t, tt.wantPrefix, string(prefix))
			assert.Equal(t, tt.wantContent, string(content))
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

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var fm []byte
			if tt.input != "" {
				prefix, _ := StripFrontMatter([]byte(tt.input))
				require.NotNil(t, prefix, "expected front matter in input")
				fm = prefix
			}
			got := ParseFrontMatterKinds(fm)
			assert.Equal(t, tt.want, got)
		})
	}
}
