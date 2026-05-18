package markdown

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

// TestStripFrontMatter_BlockScalarFenceSequence regresses a Copilot
// review observation: a YAML block-scalar value (e.g. `notes: |`) can
// contain the literal `---\n` sequence inside its body. The closing
// fence must still be matched at the start of a line, not at the first
// `---\n` substring anywhere in the YAML body.
func TestStripFrontMatter_BlockScalarFenceSequence(t *testing.T) {
	src := "---\n" +
		"id: 1\n" +
		"notes: |\n" +
		"  ---\n" +
		"  more text\n" +
		"status: open\n" +
		"---\n" +
		"# Body\n"
	wantPrefix := "---\n" +
		"id: 1\n" +
		"notes: |\n" +
		"  ---\n" +
		"  more text\n" +
		"status: open\n" +
		"---\n"
	prefix, content := StripFrontMatter([]byte(src))
	assert.Equal(t, wantPrefix, string(prefix))
	assert.Equal(t, "# Body\n", string(content))
}

func TestCountLines(t *testing.T) {
	assert.Equal(t, 0, CountLines([]byte("")))
	assert.Equal(t, 0, CountLines([]byte("no newline")))
	assert.Equal(t, 1, CountLines([]byte("one\n")))
	assert.Equal(t, 3, CountLines([]byte("---\nid: 1\n---\n")))
}
