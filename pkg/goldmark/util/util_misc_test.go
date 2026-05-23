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
