package fencepos

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/pkg/goldmark/ast"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// firstFence parses src and returns its first fenced code block.
func firstFence(t *testing.T, src string) (*lint.File, *ast.FencedCodeBlock) {
	t.Helper()
	f, err := lint.NewFile("test.md", []byte(src))
	require.NoError(t, err)
	var fcb *ast.FencedCodeBlock
	_ = ast.Walk(f.AST, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if b, ok := n.(*ast.FencedCodeBlock); ok && entering && fcb == nil {
			fcb = b
		}
		return ast.WalkContinue, nil
	})
	require.NotNil(t, fcb, src)
	return f, fcb
}

// TestOpenRun pins the opener's fence run behind container markers:
// the run starts at the parser's node position, not at the line start.
func TestOpenRun(t *testing.T) {
	cases := []struct {
		src      string
		start, n int
		ch       byte
	}{
		{"```\nx\n```\n", 0, 3, '`'},
		{"   ~~~~\nx\n~~~~\n", 3, 4, '~'},
		{"- ```\n  x\n  ```\n", 2, 3, '`'},
		{"1. ~~~go\n   x\n   ~~~\n", 3, 3, '~'},
		{"> ```\n> x\n> ```\n", 2, 3, '`'},
		{"> - ````\n>   x\n>   ````\n", 4, 4, '`'},
		{">\t```\n>\tx\n>\t```\n", 2, 3, '`'},
	}
	for _, tc := range cases {
		f, fcb := firstFence(t, tc.src)
		start, n, ch := OpenRun(f.Source, fcb)
		assert.Equal(t, tc.start, start, tc.src)
		assert.Equal(t, tc.n, n, tc.src)
		assert.Equal(t, tc.ch, ch, tc.src)
	}
}

// TestOpenRun_NoRun returns no run for a node without a position and
// for a positioned line that does not open with a fence character.
func TestOpenRun_NoRun(t *testing.T) {
	start, n, ch := OpenRun([]byte("  ~~~go\nx\n~~~\n"), ast.NewFencedCodeBlock(nil))
	assert.Equal(t, 0, n)
	assert.Equal(t, byte(0), ch)
	assert.Equal(t, 0, start)

	fcb := ast.NewFencedCodeBlock(nil)
	fcb.SetPos(0)
	_, n, ch = OpenRun([]byte("text\n"), fcb)
	assert.Equal(t, 0, n)
	assert.Equal(t, byte(0), ch)
}

// TestCloseRun pins whether a block has a closing fence and where its
// run starts. The closer line is read through the block's containers
// (block-quote markers, list-item indent), so a line that ends the
// container — and so belongs to another block — never closes it.
func TestCloseRun(t *testing.T) {
	cases := []struct {
		name  string
		src   string
		start int // -1 for not closed
	}{
		{"top level", "```\nx\n```\n", 6},
		{"top level indented closer", "```\nx\n   ```\n", 9},
		{"top level CRLF", "```\r\nx\r\n```\r\n", 8},
		{"top level unclosed", "```\nx\n", -1},
		{"empty closed", "```\n```\n", 4},
		{"list closed", "- ```\n  x\n  ```\n", 12},
		{"list unclosed", "- ```\n  x\n", -1},
		{"list ended by new fence", "- ```\n  x\n```\n", -1},
		{"list continuation ended by new fence", "- a\n\n  ```\n  x\n```\n", -1},
		{"ordered list closed", "1. ~~~\n   x\n   ~~~\n", 15},
		{"quote closed", "> ```\n> x\n> ```\n", 12},
		{"quote closed no space", ">```\n>x\n>```\n", 9},
		{"quote unclosed", "> ```\n> x\n", -1},
		{"quote ended by new fence", "> ```\n> x\n```\n", -1},
		{"quote ended by indented code", "> ```\n> x\n    ```\n", -1},
		{"nested quote closed", "> > ~~~\n> > x\n> > ~~~\n", 18},
		{"nested quote ended by outer quote", "> > ~~~\n> > x\n> ~~~\n", -1},
		{"list in quote closed", "> - ```\n>   x\n>   ```\n", 18},
		{"quote in list closed", "- > ```\n  > x\n  > ```\n", 18},
		{"tab after quote", ">\t```\n>\tx\n>\t```\n", 12},
		{"shorter closer", "````\nx\n```\n", -1},
		{"closer with trailing text", "- ```\n  x\n  ``` y\n", -1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f, fcb := firstFence(t, tc.src)
			start, n, ok := CloseRun(f.Source, fcb)
			if tc.start < 0 {
				assert.False(t, ok)
				return
			}
			require.True(t, ok)
			assert.Equal(t, tc.start, start)
			_, openN, _ := OpenRun(f.Source, fcb)
			assert.GreaterOrEqual(t, n, openN)
		})
	}
}
