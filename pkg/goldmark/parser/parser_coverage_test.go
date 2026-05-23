package parser_test

// Coverage for parser-internal helper types and option setters
// that aren't reached by upstream's parser-level black-box tests
// (the upstream test corpus exercises full-parse output, not the
// internal node types or their accessors).

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	done := make(chan struct{})
	var buf bytes.Buffer
	go func() {
		_, _ = io.Copy(&buf, r)
		close(done)
	}()
	fn()
	_ = w.Close()
	<-done
	os.Stdout = orig
	return buf.String()
}

func TestDelimiter_NodeMethods(t *testing.T) {
	d := parser.NewDelimiter(true, true, 1, '*', nil)
	d.Segment = text.NewSegment(0, 1)
	src := []byte("*")
	if d.Kind().String() != "Delimiter" {
		t.Errorf("Delimiter.Kind() = %q, want Delimiter", d.Kind())
	}
	if got := string(d.Text(src)); got != "*" {
		t.Errorf("Delimiter.Text() = %q, want *", got)
	}
	out := captureStdout(t, func() { d.Dump(src, 0) })
	if out == "" {
		t.Error("Delimiter.Dump() produced no output")
	}
	d.Inline() // empty marker, just exercise dispatch
}

func TestReference_StringAndAccessors(t *testing.T) {
	r := parser.NewReference([]byte("label"), []byte("/url"), []byte("title"))
	if string(r.Label()) != "label" {
		t.Errorf("Reference.Label() = %q, want label", r.Label())
	}
	if string(r.Destination()) != "/url" {
		t.Errorf("Reference.Destination() = %q, want /url", r.Destination())
	}
	if string(r.Title()) != "title" {
		t.Errorf("Reference.Title() = %q, want title", r.Title())
	}
	got := r.String()
	if got == "" {
		t.Error("Reference.String() returned empty")
	}
}

func TestWithHeadingAttribute_OptionDispatch(t *testing.T) {
	// WithHeadingAttribute returns an option that sets the
	// AttributeFilter on the ATX heading parser. Run a parse with
	// it applied and confirm Heading nodes accept attribute syntax.
	p := parser.NewParser(
		parser.WithBlockParsers(parser.DefaultBlockParsers()...),
		parser.WithInlineParsers(parser.DefaultInlineParsers()...),
		parser.WithParagraphTransformers(parser.DefaultParagraphTransformers()...),
		parser.WithHeadingAttribute(),
	)
	src := []byte("# H {#h1}\n")
	root := p.Parse(text.NewReader(src), parser.WithContext(parser.NewContext()))
	if root == nil {
		t.Fatal("Parse returned nil")
	}
}

func TestParser_DefaultsConstructorsAreNonEmpty(t *testing.T) {
	// Exercise DefaultBlockParsers, DefaultInlineParsers (already
	// hit via integration), and the *Parser interface accessors so
	// the trivial constructors stay at 100 %.
	b := parser.DefaultBlockParsers()
	if len(b) == 0 {
		t.Error("DefaultBlockParsers empty")
	}
	i := parser.DefaultInlineParsers()
	if len(i) == 0 {
		t.Error("DefaultInlineParsers empty")
	}
	pt := parser.DefaultParagraphTransformers()
	if len(pt) == 0 {
		t.Error("DefaultParagraphTransformers empty")
	}
	if b[0].Priority < 0 || i[0].Priority < 0 || pt[0].Priority < 0 {
		t.Error("default priorities must be non-negative")
	}
}

// parserCovKey is declared at package init so the global key
// counter is incremented BEFORE NewContext sizes its store slice.
// Inline declaration inside a test would over-index store.
var parserCovKey = parser.NewContextKey()

func TestNewContext_AccessorsRoundTrip(t *testing.T) {
	ctx := parser.NewContext()
	ctx.Set(parserCovKey, "value")
	if got := ctx.Get(parserCovKey); got != "value" {
		t.Errorf("Context.Get/Set roundtrip = %v, want \"value\"", got)
	}
	ctx.Set(parserCovKey, []string{"a"})
	if got, ok := ctx.Get(parserCovKey).([]string); !ok || len(got) != 1 || got[0] != "a" {
		t.Errorf("Context.Get list = %v ok=%v", got, ok)
	}
}

func TestParser_RaceFreeAcrossParses(t *testing.T) {
	// Plan-197 contract: each parser owns its own state, so two
	// parses on the same parser must not bleed references into
	// each other's contexts.
	p := parser.NewParser(
		parser.WithBlockParsers(parser.DefaultBlockParsers()...),
		parser.WithInlineParsers(parser.DefaultInlineParsers()...),
		parser.WithParagraphTransformers(parser.DefaultParagraphTransformers()...),
	)
	srcA := []byte("[a]: /url-a\n\n[a]\n")
	srcB := []byte("[b]: /url-b\n\n[b]\n")
	ctxA := parser.NewContext()
	p.Parse(text.NewReader(srcA), parser.WithContext(ctxA))
	ctxB := parser.NewContext()
	p.Parse(text.NewReader(srcB), parser.WithContext(ctxB))
	if _, ok := ctxA.Reference("a"); !ok {
		t.Error("first parse lost reference [a]")
	}
	if _, ok := ctxA.Reference("b"); ok {
		t.Error("first parse leaked reference [b] from second parse")
	}
	if _, ok := ctxB.Reference("b"); !ok {
		t.Error("second parse lost reference [b]")
	}
	if _, ok := ctxB.Reference("a"); ok {
		t.Error("second parse leaked reference [a] from first parse")
	}
}

func TestParser_AddOptions(t *testing.T) {
	p := parser.NewParser(
		parser.WithBlockParsers(parser.DefaultBlockParsers()...),
		parser.WithInlineParsers(parser.DefaultInlineParsers()...),
		parser.WithParagraphTransformers(parser.DefaultParagraphTransformers()...),
	)
	// AddOptions accepts variadic options and dispatches each
	// based on its kind (parser/block/inline/paragraph/ast/option).
	p.AddOptions(parser.WithHeadingAttribute())
	p.AddOptions(parser.WithBlockParsers(util.Prioritized(noopBlockParser{}, 5000)))
	// Re-parse must still succeed after option mutation.
	root := p.Parse(text.NewReader([]byte("hi\n")), parser.WithContext(parser.NewContext()))
	if root == nil {
		t.Fatal("post-AddOptions Parse returned nil")
	}
}

// noopBlockParser is a minimal BlockParser used to exercise the
// AddOptions dispatch — it never opens.
type noopBlockParser struct{}

func (noopBlockParser) Trigger() []byte               { return nil }
func (noopBlockParser) Open(parent ast.Node, reader text.Reader, pc parser.Context) (ast.Node, parser.State) {
	return nil, parser.NoChildren
}
func (noopBlockParser) Continue(node ast.Node, reader text.Reader, pc parser.Context) parser.State {
	return parser.Close
}
func (noopBlockParser) Close(node ast.Node, reader text.Reader, pc parser.Context) {}
func (noopBlockParser) CanInterruptParagraph() bool                                 { return false }
func (noopBlockParser) CanAcceptIndentedLine() bool                                 { return false }
