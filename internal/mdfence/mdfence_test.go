package mdfence

import (
	"testing"
	"unsafe"

	"github.com/stretchr/testify/assert"
)

func TestOpen(t *testing.T) {
	tests := []struct {
		name string
		line string
		want Fence
		ok   bool
	}{
		{"bare backtick", "```", Fence{Char: '`', Len: 3}, true},
		{"bare tilde", "~~~", Fence{Char: '~', Len: 3}, true},
		{"long run", "`````", Fence{Char: '`', Len: 5}, true},
		{"info string", "```go", Fence{Char: '`', Len: 3, HasInfo: true}, true},
		{"info after space", "~~~~ yaml title", Fence{Char: '~', Len: 4, HasInfo: true}, true},
		{"one space indent", " ```", Fence{Char: '`', Len: 3, Indent: 1}, true},
		{"three space indent", "   ~~~", Fence{Char: '~', Len: 3, Indent: 3}, true},
		{"trailing whitespace is no info", "```  \t", Fence{Char: '`', Len: 3}, true},
		{"trailing CR is no info", "```\r", Fence{Char: '`', Len: 3}, true},
		{"trailing LF is no info", "```\n", Fence{Char: '`', Len: 3}, true},
		{"CRLF after info", "```go\r", Fence{Char: '`', Len: 3, HasInfo: true}, true},
		{"vertical tab is info", "```\v", Fence{Char: '`', Len: 3, HasInfo: true}, true},
		{"form feed is info", "```\f", Fence{Char: '`', Len: 3, HasInfo: true}, true},
		{"nbsp is info", "```\u00a0", Fence{Char: '`', Len: 3, HasInfo: true}, true},
		{"tilde info holds backtick", "~~~ `x`", Fence{Char: '~', Len: 3, HasInfo: true}, true},
		{"mixed run stops at other char", "``~", Fence{}, false},
		{"tilde run then backticks", "~~~```", Fence{Char: '~', Len: 3, HasInfo: true}, true},
		{"backtick in backtick info", "```x```", Fence{}, false},
		{"backtick after info word", "``` js `x`", Fence{}, false},
		{"trailing backtick in info", "```go`", Fence{}, false},
		{"two backticks", "``", Fence{}, false},
		{"two tildes then text", "~~ x", Fence{}, false},
		{"four space indent", "    ```", Fence{}, false},
		{"tab indent", "\t```", Fence{}, false},
		{"spaces then tab", "  \t```", Fence{}, false},
		{"empty", "", Fence{}, false},
		{"spaces only", "   ", Fence{}, false},
		{"text", "text", Fence{}, false},
		{"leading nbsp", "\u00a0```", Fence{}, false},
		{"leading form feed", "\f```", Fence{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := Open([]byte(tt.line))
			assert.Equal(t, tt.ok, ok)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestClose(t *testing.T) {
	bt3 := Fence{Char: '`', Len: 3}
	bt4 := Fence{Char: '`', Len: 4}
	td3 := Fence{Char: '~', Len: 3}
	tests := []struct {
		name  string
		line  string
		fence Fence
		want  bool
	}{
		{"exact", "```", bt3, true},
		{"tilde exact", "~~~", td3, true},
		{"longer run", "`````", bt3, true},
		{"indented 3", "   ```", bt3, true},
		{"indent independent of opener", "```", Fence{Char: '`', Len: 3, Indent: 2}, true},
		{"trailing spaces and tabs", "```  \t", bt3, true},
		{"trailing CR", "```\r", bt3, true},
		{"trailing CRLF", "```\r\n", bt3, true},
		{"trailing LF", "```\n", bt3, true},
		{"shorter run", "```", bt4, false},
		{"two of three", "``", bt3, false},
		{"other char", "~~~", bt3, false},
		{"backticks for tilde", "```", td3, false},
		{"info string", "```go", bt3, false},
		{"text after space", "``` x", bt3, false},
		{"mixed chars", "```~", bt3, false},
		{"four space indent", "    ```", bt3, false},
		{"tab indent", "\t```", bt3, false},
		{"spaces then tab", "  \t```", bt3, false},
		{"trailing vertical tab", "```\v", bt3, false},
		{"trailing nbsp", "```\u00a0", bt3, false},
		{"blank", "", bt3, false},
		{"spaces only", "   ", bt3, false},
		{"zero fence never closes", "", Fence{}, false},
		{"zero fence with run", "```", Fence{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, Close([]byte(tt.line), tt.fence))
		})
	}
}

func TestTracker_Step(t *testing.T) {
	steps := func(lines ...string) []bool {
		var tr Tracker
		got := make([]bool, len(lines))
		for i, ln := range lines {
			got[i] = tr.Step([]byte(ln))
		}
		return got
	}
	tests := []struct {
		name  string
		lines []string
		want  []bool
	}{
		{"opens and closes", []string{"```ts", "---", "```", "---"}, []bool{true, true, true, false}},
		{"tilde info holds backtick", []string{"~~~ `x`", "~~~", "x"}, []bool{true, true, false}},
		{"backtick info is inline code", []string{"```ts``` is the language", "---"}, []bool{false, false}},
		{
			"shorter inner fence is content",
			[]string{"````md", "```js", "---", "```", "````", "---"},
			[]bool{true, true, true, true, true, false},
		},
		{
			"other char or info does not close; CR does",
			[]string{"```", "~~~", "```js", "```\r", "x"},
			[]bool{true, true, true, true, false},
		},
		{"two backticks", []string{"``", "x"}, []bool{false, false}},
		{"unclosed runs to end", []string{"text", "~~~", "a", "b"}, []bool{false, true, true, true}},
		{"reopens after close", []string{"```", "```", "~~~", "x"}, []bool{true, true, true, true}},
		{"over-indented close is content", []string{"```", "    ```", "```", "x"}, []bool{true, true, true, false}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, steps(tt.lines...))
		})
	}
}

func TestIsSpace(t *testing.T) {
	for _, c := range []byte{' ', '\t', '\n', '\r'} {
		assert.True(t, isSpace(c), "%q", c)
	}
	for _, c := range []byte{'\v', '\f', 0xA0, 'x', '`', 0} {
		assert.False(t, isSpace(c), "%q", c)
	}
}

// TestFence_Size pins Fence at 24 bytes: callers return it by value on
// per-line scan paths, so its two ints come before the two single-byte
// fields (docs/development/high-performance-go.md "Struct layout").
func TestFence_Size(t *testing.T) {
	assert.LessOrEqual(t, unsafe.Sizeof(Fence{}), uintptr(24))
}

// TestZeroAllocs pins the package's zero-allocation contract: every
// helper runs on per-line hot paths inside rule Check calls.
func TestZeroAllocs(t *testing.T) {
	open := []byte("   ```go title")
	body := []byte("x := 1")
	closeLine := []byte("````  \r")
	allocs := testing.AllocsPerRun(100, func() {
		f, ok := Open(open)
		if !ok || !Close(closeLine, f) {
			panic("unexpected fence result")
		}
		if f, ok = OpenFinal(open, true); !ok || !f.HasInfo {
			panic("unexpected final fence result")
		}
		if f, ok = OpenFinal(open, true); !ok || !f.HasInfo {
			panic("unexpected final fence result")
		}
		var tr Tracker
		tr.Step(open)
		tr.Step(body)
		tr.Step(closeLine)
	})
	assert.Zero(t, allocs)
}
