package include

import "testing"

func TestAdjustHeadings_ATX(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		parentLevel int
		want        string
	}{
		{
			name:        "shift up: parent=2, source h2 and h3 become h3 and h4",
			content:     "## Title\n\nSome text.\n\n### Sub\n",
			parentLevel: 2,
			want:        "### Title\n\nSome text.\n\n#### Sub\n",
		},
		{
			name:        "no headings in content returns unchanged",
			content:     "Just some text.\n\nAnother paragraph.\n",
			parentLevel: 3,
			want:        "Just some text.\n\nAnother paragraph.\n",
		},
		{
			name:        "parentLevel=0 returns unchanged",
			content:     "## Title\n\n### Sub\n",
			parentLevel: 0,
			want:        "## Title\n\n### Sub\n",
		},
		{
			name:        "cap at level 6",
			content:     "# One\n\n## Two\n\n### Three\n",
			parentLevel: 5,
			want:        "###### One\n\n###### Two\n\n###### Three\n",
		},
		{
			name:        "ATX headings with closing hashes",
			content:     "## Heading ##\n\n### Sub ###\n",
			parentLevel: 2,
			want:        "### Heading ##\n\n#### Sub ###\n",
		},
		{
			name:        "shift=0 returns unchanged: parent=1, source min=h2",
			content:     "## Title\n\n### Sub\n",
			parentLevel: 1,
			want:        "## Title\n\n### Sub\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := adjustHeadings(tt.content, tt.parentLevel)
			if got != tt.want {
				t.Errorf("adjustHeadings() =\n%q\nwant:\n%q", got, tt.want)
			}
		})
	}
}

func TestAdjustHeadings_Setext(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		parentLevel int
		want        string
	}{
		{
			name:        "setext h1 converted to ATX when shifted",
			content:     "Title\n=====\n\nBody text.\n",
			parentLevel: 2,
			want:        "### Title\n\nBody text.\n",
		},
		{
			name:        "setext h2 converted to ATX when shifted",
			content:     "Title\n-----\n\nBody text.\n",
			parentLevel: 2,
			want:        "### Title\n\nBody text.\n",
		},
		{
			name:        "mixed ATX and setext headings",
			content:     "Top\n===\n\n## Sub ATX\n\nAnother\n---\n",
			parentLevel: 2,
			want:        "### Top\n\n#### Sub ATX\n\n#### Another\n",
		},
		{
			name:        "multiple setext h1 and h2",
			content:     "First\n=====\n\nSecond\n-----\n",
			parentLevel: 1,
			want:        "## First\n\n### Second\n",
		},
		{
			name:        "setext h2 is min level, parent=3",
			content:     "Only H2\n-------\n",
			parentLevel: 3,
			want:        "#### Only H2\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := adjustHeadings(tt.content, tt.parentLevel)
			if got != tt.want {
				t.Errorf("adjustHeadings() =\n%q\nwant:\n%q", got, tt.want)
			}
		})
	}
}

func TestAdjustHeadings_CodeBlocks(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		parentLevel int
		want        string
	}{
		{
			name:        "code blocks with hash lines not modified",
			content:     "## Real Heading\n\n```bash\n# this is a comment\n## another comment\n```\n",
			parentLevel: 2,
			want:        "### Real Heading\n\n```bash\n# this is a comment\n## another comment\n```\n",
		},
		{
			name:        "setext inside code block not modified",
			content:     "## Heading\n\n```\nFake Title\n=====\n```\n",
			parentLevel: 2,
			want:        "### Heading\n\n```\nFake Title\n=====\n```\n",
		},
		{
			name:        "indented code fence (up to 3 spaces) skipped",
			content:     "## Heading\n\n   ```\n## fake heading\n   ```\n",
			parentLevel: 2,
			want:        "### Heading\n\n   ```\n## fake heading\n   ```\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := adjustHeadings(tt.content, tt.parentLevel)
			if got != tt.want {
				t.Errorf("adjustHeadings() =\n%q\nwant:\n%q", got, tt.want)
			}
		})
	}
}
