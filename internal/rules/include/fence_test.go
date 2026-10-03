package include

import (
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jeduden/mdsmith/pkg/goldmark/util"
)

// steps runs a fresh fenceScan over lines with no paragraph open and
// returns the per-line verdicts.
func steps(lines ...string) []bool {
	var s fenceScan
	got := make([]bool, len(lines))
	for i, ln := range lines {
		got[i] = s.step([]byte(ln), false)
	}
	return got
}

func TestFenceScan_Step(t *testing.T) {
	assert.Equal(t, []bool{true, true, true, false},
		steps("```", "## x", "```", "## y"), "opens, content, closes")
	assert.Equal(t, []bool{true, true, true, false},
		steps("   ```", "x", "  ````  ", "y"), "up to three spaces open and close")
	assert.Equal(t, []bool{false, false, false},
		steps("    ```", "\t```", "x"), "four columns or a tab is no fence")
	assert.Equal(t, []bool{true, true, true, true, false},
		steps("```", "    ```", "\t```", "```", "y"), "an indented-code-deep line is no closer")
	assert.Equal(t, []bool{true, true, true, false},
		steps("```", "x", "```\r", "y"), "a CRLF closer closes")
	assert.Equal(t, []bool{true, true, true, false},
		steps("```\n", "x\n", "```\n", "y\n"), "lines keeping their LF")
	assert.Equal(t, []bool{true, true, true, true},
		steps("```", "``", "```go", "~~~"), "short, info, and other-char lines do not close")
	assert.Equal(t, []bool{false, false}, steps("```x``` is code.", "## h"),
		"a backtick in a backtick info string is not a fence")
	assert.Equal(t, []bool{true, true, true, true, false},
		steps("- ```", "  # c", "", "  ```", "  x"), "a list-marker fence closes at the item's column")
	assert.Equal(t, []bool{true, true, false},
		steps("1. ~~~", "   x", "# out"), "a line left of the item's column ends its fence")
	assert.Equal(t, []bool{false}, steps("-     ```"), "five columns after a marker is indented code")
}

func TestFenceScan_StepInParagraph(t *testing.T) {
	var s fenceScan
	assert.False(t, s.step([]byte("2. ```"), true), "an ordered start other than 1 cannot interrupt")
	assert.True(t, s.step([]byte("- ```"), true), "a bullet with content interrupts")
	s = fenceScan{}
	assert.True(t, s.step([]byte("```"), true), "a fence interrupts a paragraph")
}

func TestFenceScan_LeavesItem(t *testing.T) {
	s := fenceScan{base: 2}
	assert.True(t, s.leavesItem([]byte("x")))
	assert.True(t, s.leavesItem([]byte(" x")))
	assert.False(t, s.leavesItem([]byte("  x")))
	assert.False(t, s.leavesItem([]byte("\tx")))
	assert.False(t, s.leavesItem([]byte("")))
	assert.False(t, s.leavesItem([]byte(" \r")))
	assert.False(t, (&fenceScan{}).leavesItem([]byte("x")), "top-level fence")
}

// TestFenceScan_ZeroAllocs pins that the string-to-bytes view does not
// copy: a line longer than the compiler's 32-byte stack buffer would
// otherwise allocate on every call.
func TestFenceScan_ZeroAllocs(t *testing.T) {
	const long = "  ```go title=\"a fairly long info string here\""
	allocs := testing.AllocsPerRun(100, func() {
		var s fenceScan
		_ = s.step(util.StringToReadOnlyBytes(long), false)
	})
	assert.Zero(t, allocs)
}

func TestRewriteSkippingCode(t *testing.T) {
	up := func(s string) string { return strings.ToUpper(s) }
	in := "para\n    ```\nx\n\n```\ny\n```\n- ```\n  z\n"
	want := "PARA\n    ```\nX\n\n```\ny\n```\n- ```\n  z\n"
	assert.Equal(t, want, rewriteSkippingCode(in, up))
	assert.Equal(t, "PARA\n2. ```\nX\n", rewriteSkippingCode("para\n2. ```\nx\n", up),
		"a marker line that cannot interrupt the paragraph opens no fence")
}

// TestFenceScan_ScalarOnly pins that the scan cannot retain a line: the
// lines it reads are zero-copy views of strings, so no field at any
// depth may hold a pointer, slice, string, map, or interface.
func TestFenceScan_ScalarOnly(t *testing.T) {
	assertScalarFields(t, reflect.TypeOf(fenceScan{}))
}

// assertScalarFields fails when typ holds a non-scalar field at any
// depth, recursing into nested structs such as mdfence.Fence.
func assertScalarFields(t *testing.T, typ reflect.Type) {
	t.Helper()
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		switch f.Type.Kind() {
		case reflect.Struct:
			assertScalarFields(t, f.Type)
		case reflect.Bool, reflect.Int, reflect.Uint8:
		default:
			t.Errorf("%s.%s is %s, not a scalar", typ.Name(), f.Name, f.Type.Kind())
		}
	}
}

// TestFenceScan_StepLeavesLineIntact pins that step reads a line
// without writing to it, which the read-only string view relies on.
func TestFenceScan_StepLeavesLineIntact(t *testing.T) {
	lines := []string{"- ```go", "  # c", "\t```", "x", "  ````  \r", "1. ~~~"}
	var s fenceScan
	for _, l := range lines {
		b := []byte(l)
		_ = s.step(b, false)
		_ = s.step(b, true)
		_ = s.leavesItem(b)
		_, _, _ = listContent(b)
		assert.Equal(t, l, string(b), "input mutated")
	}
}

// TestRewriteSkippingCode_SiblingItemFence pins that a list marker line
// after a list item's paragraph opens its fence: the paragraph-interrupt
// rule binds only a document-level paragraph, so "2. ```" there starts a
// sibling item rather than continuing the text.
func TestRewriteSkippingCode_SiblingItemFence(t *testing.T) {
	up := func(s string) string { return strings.ToUpper(s) }
	assert.Equal(t, "1. A\n2. ```\n   x\n   ```\nY\n",
		rewriteSkippingCode("1. a\n2. ```\n   x\n   ```\ny\n", up))
	assert.Equal(t, "> Q\n2. ```\n   x\n   ```\nY\n",
		rewriteSkippingCode("> q\n2. ```\n   x\n   ```\ny\n", up))
}
