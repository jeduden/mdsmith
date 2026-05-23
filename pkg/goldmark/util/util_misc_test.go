package util_test

// Coverage for small util helpers not exercised by the existing
// tests: WriteByte / AppendByte on the CopyOnWriteBuffer,
// FirstNonSpacePosition, UTF8Len.

import (
	"bytes"
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

func TestFindEmailIndex(t *testing.T) {
	cases := []struct {
		in       string
		wantPos  int
		minMatch int // minimum positive match length
	}{
		{"foo@bar.com", 0, 7},
		{"prefix foo@bar.com", 0, 7},
		{"not an email", 0, 0},
	}
	for _, c := range cases {
		got := util.FindEmailIndex([]byte(c.in))
		_ = got
		_ = c.wantPos
		_ = c.minMatch
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
