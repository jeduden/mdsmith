package include

import "testing"

func TestAdjustLinks_SameDir(t *testing.T) {
	tests := []struct {
		name          string
		content       string
		includedFile  string
		includingFile string
		want          string
	}{
		{
			name:          "same directory no-op both in root",
			content:       "[link](foo.md)",
			includedFile:  "DEVELOPMENT.md",
			includingFile: "README.md",
			want:          "[link](foo.md)",
		},
		{
			name:          "same subdirectory no-op",
			content:       "[link](foo.md)",
			includedFile:  "docs/a.md",
			includingFile: "docs/b.md",
			want:          "[link](foo.md)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := adjustLinks(tt.content, tt.includedFile, tt.includingFile)
			if got != tt.want {
				t.Errorf("adjustLinks() =\n  %q\nwant:\n  %q", got, tt.want)
			}
		})
	}
}

func TestAdjustLinks_Rewrite(t *testing.T) {
	tests := []struct {
		name          string
		content       string
		includedFile  string
		includingFile string
		want          string
	}{
		{
			name:          "different directories rewrite",
			content:       "[link](internal/rules/foo.go)",
			includedFile:  "DEVELOPMENT.md",
			includingFile: "docs/guide.md",
			want:          "[link](../internal/rules/foo.go)",
		},
		{
			name:          "fragment preserved after rewrite",
			content:       "[link](foo.md#section)",
			includedFile:  "DEVELOPMENT.md",
			includingFile: "docs/guide.md",
			want:          "[link](../foo.md#section)",
		},
		{
			name:          "query string preserved",
			content:       "[link](foo.md?v=1#section)",
			includedFile:  "DEVELOPMENT.md",
			includingFile: "docs/guide.md",
			want:          "[link](../foo.md?v=1#section)",
		},
		{
			name:          "image link rewritten",
			content:       "![alt](images/pic.png)",
			includedFile:  "DEVELOPMENT.md",
			includingFile: "docs/guide.md",
			want:          "![alt](../images/pic.png)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := adjustLinks(tt.content, tt.includedFile, tt.includingFile)
			if got != tt.want {
				t.Errorf("adjustLinks() =\n  %q\nwant:\n  %q", got, tt.want)
			}
		})
	}
}

func TestAdjustLinks_Complex(t *testing.T) {
	tests := []struct {
		name          string
		content       string
		includedFile  string
		includingFile string
		want          string
	}{
		{
			name:          "multiple links in same content",
			content:       "[a](foo.md) and [b](bar.md)",
			includedFile:  "DEVELOPMENT.md",
			includingFile: "docs/guide.md",
			want:          "[a](../foo.md) and [b](../bar.md)",
		},
		{
			name:          "nested brackets in link text",
			content:       "[text [inner]](foo.md)",
			includedFile:  "DEVELOPMENT.md",
			includingFile: "docs/guide.md",
			want:          "[text [inner]](../foo.md)",
		},
		{
			name:          "deeper nesting rewrite",
			content:       "[link](../shared/util.go)",
			includedFile:  "internal/rules/include/rule.go",
			includingFile: "docs/design/include.md",
			want:          "[link](../../internal/rules/shared/util.go)",
		},
		{
			name:          "including from subdirectory into root",
			content:       "[link](../README.md)",
			includedFile:  "docs/guide.md",
			includingFile: "README.md",
			want:          "[link](README.md)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := adjustLinks(tt.content, tt.includedFile, tt.includingFile)
			if got != tt.want {
				t.Errorf("adjustLinks() =\n  %q\nwant:\n  %q", got, tt.want)
			}
		})
	}
}

func TestAdjustLinks_CodeSkip(t *testing.T) {
	tests := []struct {
		name          string
		content       string
		includedFile  string
		includingFile string
		want          string
	}{
		{
			name:          "link inside fenced code block not rewritten",
			content:       "## Heading\n\n```md\n[link](foo.md)\n```\n",
			includedFile:  "DEVELOPMENT.md",
			includingFile: "docs/guide.md",
			want:          "## Heading\n\n```md\n[link](foo.md)\n```\n",
		},
		{
			name:          "link inside inline code not rewritten",
			content:       "Use `[link](foo.md)` syntax.\n",
			includedFile:  "DEVELOPMENT.md",
			includingFile: "docs/guide.md",
			want:          "Use `[link](foo.md)` syntax.\n",
		},
		{
			name:          "link outside code rewritten, inside code preserved",
			content:       "[real](foo.md) and `[fake](bar.md)`\n",
			includedFile:  "DEVELOPMENT.md",
			includingFile: "docs/guide.md",
			want:          "[real](../foo.md) and `[fake](bar.md)`\n",
		},
		{
			name:          "multi-backtick inline code preserved",
			content:       "Use ``[link](foo.md)`` syntax.\n",
			includedFile:  "DEVELOPMENT.md",
			includingFile: "docs/guide.md",
			want:          "Use ``[link](foo.md)`` syntax.\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := adjustLinks(tt.content, tt.includedFile, tt.includingFile)
			if got != tt.want {
				t.Errorf("adjustLinks() =\n  %q\nwant:\n  %q", got, tt.want)
			}
		})
	}
}

func TestAdjustLinks_Skip(t *testing.T) {
	tests := []struct {
		name          string
		content       string
		includedFile  string
		includingFile string
		want          string
	}{
		{
			name:          "anchor-only link untouched",
			content:       "[section](#foo)",
			includedFile:  "DEVELOPMENT.md",
			includingFile: "docs/guide.md",
			want:          "[section](#foo)",
		},
		{
			name:          "absolute URL http untouched",
			content:       "[link](http://example.com)",
			includedFile:  "DEVELOPMENT.md",
			includingFile: "docs/guide.md",
			want:          "[link](http://example.com)",
		},
		{
			name:          "absolute URL https untouched",
			content:       "[link](https://example.com/path)",
			includedFile:  "DEVELOPMENT.md",
			includingFile: "docs/guide.md",
			want:          "[link](https://example.com/path)",
		},
		{
			name:          "absolute path untouched",
			content:       "[link](/foo/bar)",
			includedFile:  "DEVELOPMENT.md",
			includingFile: "docs/guide.md",
			want:          "[link](/foo/bar)",
		},
		{
			name:          "mailto link untouched",
			content:       "[email](mailto:a@b.com)",
			includedFile:  "DEVELOPMENT.md",
			includingFile: "docs/guide.md",
			want:          "[email](mailto:a@b.com)",
		},
		{
			name:          "empty target left as-is",
			content:       "[link]()",
			includedFile:  "DEVELOPMENT.md",
			includingFile: "docs/guide.md",
			want:          "[link]()",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := adjustLinks(tt.content, tt.includedFile, tt.includingFile)
			if got != tt.want {
				t.Errorf("adjustLinks() =\n  %q\nwant:\n  %q", got, tt.want)
			}
		})
	}
}
