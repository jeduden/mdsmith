package linkgraph

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolveRelTarget(t *testing.T) {
	cases := []struct {
		name string
		src  string
		link string
		want string
	}{
		{"sibling", "docs/a.md", "b.md", "docs/b.md"},
		{"parent", "docs/sub/a.md", "../top.md", "docs/top.md"},
		{"workspace-root sibling", "a.md", "b.md", "b.md"},
		{"absolute link rejected", "a.md", "/etc/passwd", ""},
		{"absolute src rejected", "/abs/a.md", "b.md", ""},
		{"drive-letter src rejected", `C:/work/a.md`, "b.md", ""},
		{"unc src rejected", "//srv/share/a.md", "b.md", ""},
		{"escape via .. rejected", "a.md", "../up.md", ""},
		{"deep escape via .. rejected", "docs/a.md", "../../way-up.md", ""},
		{"backslash translated", "docs/a.md", `sub\x.md`, "docs/sub/x.md"},
		{"cleaned-abs result rejected", "a/../../b.md", "x.md", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, ResolveRelTarget(tc.src, tc.link))
		})
	}
}
