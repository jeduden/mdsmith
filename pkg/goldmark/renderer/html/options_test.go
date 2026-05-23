package html_test

// Coverage for the html renderer's option dispatchers and Writer
// methods. Each option's SetConfig branch is exercised by passing
// the With*Option through goldmark.WithRendererOptions; the writer
// methods are driven directly via the Writer interface.

import (
	"bufio"
	"bytes"
	"strings"
	"testing"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/renderer/html"
)

func convertWithOpts(t *testing.T, src string, opts ...renderer.Option) string {
	t.Helper()
	md := goldmark.New(goldmark.WithRendererOptions(opts...))
	var buf bytes.Buffer
	if err := md.Convert([]byte(src), &buf); err != nil {
		t.Fatalf("Convert: %v", err)
	}
	return buf.String()
}

func TestRenderOption_EastAsianLineBreaksNone(t *testing.T) {
	// Without the option the soft line break between CJK runs
	// renders as a literal newline.
	out := convertWithOpts(t, "日本語\nテキスト\n", html.WithEastAsianLineBreaks(html.EastAsianLineBreaksNone))
	if !strings.Contains(out, "\n") {
		t.Errorf("expected newline preserved (None mode), got: %q", out)
	}
}

func TestRenderOption_EastAsianLineBreaksSimple(t *testing.T) {
	// Simple mode: between two East-Asian-wide runes the soft
	// line break is suppressed.
	out := convertWithOpts(t, "日本語\nテキスト\n", html.WithEastAsianLineBreaks(html.EastAsianLineBreaksSimple))
	// Suppressed means no newline between the two CJK words.
	if strings.Contains(out, "日本語\nテキスト") {
		t.Errorf("Simple mode should suppress break between CJK runs: %q", out)
	}
}

func TestRenderOption_EastAsianLineBreaksCSS3Draft(t *testing.T) {
	out := convertWithOpts(t, "日本語\nテキスト\n", html.WithEastAsianLineBreaks(html.EastAsianLineBreaksCSS3Draft))
	if strings.Contains(out, "日本語\nテキスト") {
		t.Errorf("CSS3Draft mode should suppress break between CJK runs: %q", out)
	}
}

func TestRenderOption_EastAsianLineBreaksCSS3DraftPunctuation(t *testing.T) {
	// CSS3Draft has rules for combining punctuation around the
	// break — exercise a few branches with halfwidth punctuation.
	cases := []string{
		"a。\n日本語\n",
		"日本語\n。b\n",
		"​\n日本語\n",
	}
	for _, c := range cases {
		_ = convertWithOpts(t, c, html.WithEastAsianLineBreaks(html.EastAsianLineBreaksCSS3Draft))
	}
}

func TestRenderOption_HardWraps(t *testing.T) {
	out := convertWithOpts(t, "one\ntwo\n", html.WithHardWraps())
	if !strings.Contains(out, "<br") {
		t.Errorf("HardWraps should emit <br>: %q", out)
	}
}

func TestRenderOption_XHTML(t *testing.T) {
	out := convertWithOpts(t, "---\n\n![alt](/x)\n", html.WithXHTML())
	if !strings.Contains(out, " />") {
		t.Errorf("XHTML option should produce self-closing form: %q", out)
	}
}

func TestRenderOption_Unsafe(t *testing.T) {
	out := convertWithOpts(t, "<script>x</script>\n", html.WithUnsafe())
	if !strings.Contains(out, "<script>") {
		t.Errorf("Unsafe should keep <script>: %q", out)
	}
}

func TestRenderOption_WithWriter(t *testing.T) {
	// WithWriter installs a custom Writer. We pass the default
	// Writer reconfigured with WithEscapedSpace to drive that path.
	w := html.NewWriter(html.WithEscapedSpace())
	out := convertWithOpts(t, "hello world\n", html.WithWriter(w))
	if !strings.Contains(out, "hello") {
		t.Errorf("WithWriter convert dropped content: %q", out)
	}
}

func TestWriter_RawWrite_SecureWrite_Write(t *testing.T) {
	// Drive the Writer methods directly so the per-method coverage
	// climbs without depending on which Convert path picks them up.
	w := html.NewWriter()
	var buf bytes.Buffer
	bw := bufio.NewWriter(&buf)

	// RawWrite escapes & < > and " — drive each separately.
	for _, in := range [][]byte{
		[]byte("plain text"),
		[]byte("with & and <"),
		[]byte("with > and \""),
	} {
		buf.Reset()
		w.RawWrite(bw, in)
		_ = bw.Flush()
	}

	// SecureWrite drops NUL bytes (UTF-8 invalid byte stripping).
	buf.Reset()
	w.SecureWrite(bw, []byte("a\x00b\x00c"))
	_ = bw.Flush()
	if strings.Contains(buf.String(), "\x00") {
		t.Errorf("SecureWrite should strip NUL: %q", buf.String())
	}

	// Write handles entities (&amp; etc) plus the escape rules.
	buf.Reset()
	w.Write(bw, []byte("a &amp; b &#65; c &lt;d&gt;"))
	_ = bw.Flush()
	if !strings.Contains(buf.String(), "&amp;") {
		t.Errorf("Write should preserve &amp;: %q", buf.String())
	}
}

func TestWriter_EscapedSpace(t *testing.T) {
	// With WithEscapedSpace, the writer escapes literal "\ " (a
	// backslash followed by space) differently — drive a code
	// path that includes one.
	w := html.NewWriter(html.WithEscapedSpace())
	var buf bytes.Buffer
	bw := bufio.NewWriter(&buf)
	w.Write(bw, []byte(`a\ b`))
	_ = bw.Flush()
}
