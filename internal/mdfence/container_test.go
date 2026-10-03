package mdfence

import (
	"reflect"
	"testing"

	"github.com/jeduden/mdsmith/pkg/goldmark"
	"github.com/jeduden/mdsmith/pkg/goldmark/ast"
	"github.com/jeduden/mdsmith/pkg/goldmark/text"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIndent(t *testing.T) {
	tests := []struct {
		line          string
		col, width, n int
	}{
		{"```", 0, 0, 0},
		{"  ```", 0, 2, 2},
		{"\t```", 0, 4, 1},
		{" \t```", 0, 4, 2},
		{"   \t```", 0, 4, 4},
		{"\t ```", 0, 5, 2},
		{"\t```", 2, 2, 1},
		{"\t```", 3, 1, 1},
		{"\t\t", 0, 8, 2},
		{"", 0, 0, 0},
	}
	for _, tt := range tests {
		w, n := Indent([]byte(tt.line), tt.col)
		assert.Equal(t, [2]int{tt.width, tt.n}, [2]int{w, n}, "%q col=%d", tt.line, tt.col)
	}
}

// itemLines are lines placed inside a list item whose content starts at
// column two ("- "), mixing spaces and tabs in the indentation.
var itemLines = []string{
	"  ```", "   ```", "     ```", "      ```", "\t```", " \t```", "  \t```",
	"   \t```", "\t ```", "\t  ```", " \t ```", "\t\t```", "  \t~~~",
}

// itemFence parses src and returns the first fenced code block inside
// a list item, or nil.
func itemFence(t *testing.T, src string) *ast.FencedCodeBlock {
	t.Helper()
	root := goldmark.DefaultParser().Parse(text.NewReader([]byte(src)))
	var fcb *ast.FencedCodeBlock
	_ = ast.Walk(root, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if f, ok := n.(*ast.FencedCodeBlock); ok && entering {
			if _, in := f.Parent().(*ast.ListItem); in {
				fcb = f
				return ast.WalkStop, nil
			}
		}
		return ast.WalkContinue, nil
	})
	return fcb
}

// TestOpenIn_MatchesParser checks OpenIn against goldmark for lines in a
// list item: a tab reaches the next multiple of four, measured from
// column zero, and the indent budget counts from the item's content
// column.
//
// One layout diverges, and OpenIn follows CommonMark there: when the
// item's content column falls between spaces and a tab follows ("  \t"),
// goldmark's opener reads the fence character at a byte offset equal to
// the indent's column width, lands inside the run, and finds it too
// short. CommonMark (and goldmark's closer) measure the tab correctly.
func TestOpenIn_MatchesParser(t *testing.T) {
	forkQuirk := map[string]bool{"  \t```": true, "  \t~~~": true}
	for _, line := range itemLines {
		want := itemFence(t, "- a\n\n"+line+"\n") != nil
		_, ok := OpenIn([]byte(line), 0, 2)
		if forkQuirk[line] {
			assert.False(t, want, "goldmark quirk gone for %q; drop it from forkQuirk", line)
			assert.True(t, ok, "%q", line)
			continue
		}
		assert.Equal(t, want, ok, "%q", line)
	}
}

// TestCloseIn_MatchesParser checks CloseIn against goldmark: the line
// closes the item's fence exactly when the parser gives the fence no
// content line.
func TestCloseIn_MatchesParser(t *testing.T) {
	open := Fence{Char: '`', Len: 3}
	for _, line := range itemLines {
		fcb := itemFence(t, "- a\n\n  ```\n"+line+"\n")
		require.NotNil(t, fcb, "%q", line)
		want := fcb.Lines().Len() == 0
		assert.Equal(t, want, CloseIn([]byte(line), open, 0, 2), "%q", line)
	}
}

func TestOpenIn(t *testing.T) {
	f, ok := OpenIn([]byte("```go"), 4, 4)
	assert.True(t, ok)
	assert.Equal(t, Fence{Char: '`', Len: 3, HasInfo: true}, f)
	_, ok = OpenIn([]byte("```"), 0, 2)
	assert.True(t, ok, "a line left of the content column opens at indent 0")
	_, ok = OpenIn([]byte("\t```"), 2, 2)
	assert.True(t, ok, "a tab from column 2 reaches column 4: indent 2")
	_, ok = OpenIn([]byte("\t```"), 0, 0)
	assert.False(t, ok)
}

func TestCloseIn(t *testing.T) {
	bt := Fence{Char: '`', Len: 3}
	assert.True(t, CloseIn([]byte("\t```"), bt, 0, 2))
	assert.False(t, CloseIn([]byte("\t```"), bt, 0, 0))
	assert.False(t, CloseIn([]byte("```"), Fence{}, 0, 0))
}

// TestNoRetention pins what include's zero-copy string view relies on:
// the helpers never write to the line they are given, and a Tracker
// holds only scalars, so it cannot keep a reference to a line.
func TestNoRetention(t *testing.T) {
	lines := []string{"  ```go x", "body", "\t```", "  ````  \r", "~~~", "```x```"}
	var tr Tracker
	for _, l := range lines {
		b := []byte(l)
		_, _ = Open(b)
		_, _ = OpenFinal(b, true)
		_, _ = OpenIn(b, 1, 2)
		_ = Close(b, Fence{Char: '`', Len: 3})
		_ = CloseIn(b, Fence{Char: '~', Len: 3}, 0, 2)
		_, _ = Indent(b, 0)
		_ = tr.Step(b)
		assert.Equal(t, l, string(b), "input mutated")
	}
	assertScalarOnly(t, reflect.TypeOf(Tracker{}))
}

// assertScalarOnly fails when typ holds a pointer-bearing field at any
// depth.
func assertScalarOnly(t *testing.T, typ reflect.Type) {
	t.Helper()
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		switch f.Type.Kind() {
		case reflect.Struct:
			assertScalarOnly(t, f.Type)
		case reflect.Bool, reflect.Int, reflect.Uint8:
		default:
			t.Errorf("%s.%s is %s, not a scalar", typ.Name(), f.Name, f.Type.Kind())
		}
	}
}
