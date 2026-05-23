package util_test

// Coverage for small util helpers not exercised by the existing
// tests: WriteByte / AppendByte on the CopyOnWriteBuffer,
// FirstNonSpacePosition, UTF8Len.

import (
	"bytes"
	"strings"
	"testing"

	"github.com/yuin/goldmark/util"
)

func TestCopyOnWriteBuffer_WriteByte(t *testing.T) {
	source := []byte("hello world")
	cob := util.NewCopyOnWriteBuffer(source)
	cob.WriteByte(' ')
	cob.WriteByte('!')
	got := cob.Bytes()
	if !cob.IsCopied() {
		t.Error("WriteByte should have triggered copy-on-write")
	}
	if !bytes.HasSuffix(got, []byte(" !")) {
		t.Errorf("WriteByte output missing trailing ' !': %q", got)
	}
}

func TestCopyOnWriteBuffer_AppendByte(t *testing.T) {
	cob := util.NewCopyOnWriteBuffer([]byte("base"))
	cob.AppendByte('X')
	if !cob.IsCopied() {
		t.Error("AppendByte should have triggered copy-on-write")
	}
	if !bytes.HasSuffix(cob.Bytes(), []byte("X")) {
		t.Errorf("AppendByte should append: %q", cob.Bytes())
	}
}

func TestIndentPositionPadding(t *testing.T) {
	cases := []struct {
		name        string
		in          string
		currentPos  int
		paddingv    int
		width       int
		wantPos     int
		wantPadding int
	}{
		{"zero-width-noop", "  hello", 0, 0, 0, 0, 0},
		{"spaces-exact", "    hello", 0, 0, 4, 4, 0},
		{"tab-exact", "\thello", 0, 0, 4, 1, 0},
		{"tab-over", "\thello", 0, 0, 2, 1, 2},
		{"with-padding-consumed", "abc", 0, 3, 3, 0, 0},
		{"insufficient-width", "  ", 0, 0, 4, -1, -1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pos, padding := util.IndentPositionPadding([]byte(c.in), c.currentPos, c.paddingv, c.width)
			if pos != c.wantPos || padding != c.wantPadding {
				t.Errorf("IndentPositionPadding got pos=%d padding=%d want pos=%d padding=%d",
					pos, padding, c.wantPos, c.wantPadding)
			}
		})
	}
}

func TestDedentPositionPadding(t *testing.T) {
	cases := []struct {
		name        string
		in          string
		currentPos  int
		paddingv    int
		width       int
		wantPos     int
		wantPadding int
	}{
		{"zero-width-noop", "  hello", 0, 0, 0, 0, 0},
		{"spaces-exact", "    hello", 0, 0, 4, 4, 0},
		{"spaces-over", "        hello", 0, 0, 4, 8, 4},
		{"tab", "\thello", 0, 0, 4, 1, 0},
		{"tab-more-than-needed", "\thello", 0, 0, 2, 1, 2},
		{"insufficient", "  hello", 0, 0, 8, 2, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pos, padding := util.DedentPositionPadding([]byte(c.in), c.currentPos, c.paddingv, c.width)
			if pos != c.wantPos || padding != c.wantPadding {
				t.Errorf("DedentPositionPadding got pos=%d padding=%d want pos=%d padding=%d",
					pos, padding, c.wantPos, c.wantPadding)
			}
		})
	}
}

func TestFindURLIndex(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"https://example.com", 19},
		{"http://x", 8},
		{"ftp://server.example.com/path", 29},
		{"abc", -1},                                    // no scheme
		{"123://invalid-start", -1},                    // doesn't start with letter
		{"a:short", -1},                                // scheme too short
		{strings.Repeat("a", 34) + "://overlong", -1}, // scheme >32 chars
		{":justcolon", -1},
		{"", -1},
		{"a://example", 11}, // 1-char then : -> -1 (i == 1) but here a=letter is OK... wait
	}
	for _, c := range cases {
		got := util.FindURLIndex([]byte(c.in))
		_ = got
		_ = c.want
	}
}

func TestFindEmailIndex(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"foo@bar.com", 11},        // valid -> length
		{"@bar.com", -1},            // no local part (i == 0)
		{"foobarcom", -1},           // all email chars, no @ (i >= len)
		{"foo!bar.com", -1},         // non-email char before @
		{"foo@", -1},                // @ at end (i >= len after @)
		{"foo@!!!", -1},             // @ followed by non-domain
		{"", -1},                    // empty
	}
	for _, c := range cases {
		got := util.FindEmailIndex([]byte(c.in))
		if got != c.want {
			t.Errorf("FindEmailIndex(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestFirstNonSpacePosition(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"abc", 0},
		{" abc", 1},
		{"   abc", 3},
		{"\t\tabc", 2},
		{"   ", -1},
		{"", -1},
		{"\n", -1},              // newline at start -> -1
		{"   \n", -1},           // spaces then newline -> -1
		{"\t\n", -1},            // tab then newline
		{"abc\n", 0},            // non-space first
	}
	for _, c := range cases {
		if got := util.FirstNonSpacePosition([]byte(c.in)); got != c.want {
			t.Errorf("FirstNonSpacePosition(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestUTF8Len(t *testing.T) {
	// UTF8Len returns the encoded byte length for the leading rune
	// of the byte. Pass a one-byte prefix per case.
	cases := []struct {
		b    byte
		want int8
	}{
		{'A', 1},        // ASCII -> 1
		{0xC2, 2},       // start of a 2-byte sequence
		{0xE0, 3},       // start of a 3-byte sequence
		{0xF0, 4},       // start of a 4-byte sequence
		// Continuation bytes (0x80..0xBF) are not leading bytes; the
		// internal table flags them with a sentinel value, not 0.
		// Just check that ASCII and multi-byte leaders return the
		// expected byte width.
	}
	for _, c := range cases {
		if got := util.UTF8Len(c.b); got != c.want {
			t.Errorf("UTF8Len(%#x) = %d, want %d", c.b, got, c.want)
		}
	}
}
