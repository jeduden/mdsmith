package lint

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
