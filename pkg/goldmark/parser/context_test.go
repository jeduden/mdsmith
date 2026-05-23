package parser_test

// Coverage for the parser.Context surface that mdsmith's tests
// otherwise wouldn't touch: ID generation, IDs accessor,
// ComputeIfAbsent, References list, IsInLinkLabel, WithIDs
// option, plus the WithEscapedSpace and WithOption parser
// options.

import (
	"testing"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

func TestContext_IDs_GenerateAndPut(t *testing.T) {
	ctx := parser.NewContext()
	ids := ctx.IDs()
	if ids == nil {
		t.Fatal("Context.IDs() must return a non-nil IDs")
	}
	// Generate two distinct slugs for the same label.
	a := string(ids.Generate([]byte("Heading"), ast.KindHeading))
	b := string(ids.Generate([]byte("Heading"), ast.KindHeading))
	if a == "" || b == "" {
		t.Fatal("Generate returned empty string")
	}
	if a == b {
		t.Errorf("two Generate calls with same input must disambiguate: %q == %q", a, b)
	}
	// Put claims a slug so it doesn't get handed out again.
	ids.Put([]byte("used"))
	got := string(ids.Generate([]byte("Used"), ast.KindHeading))
	if got == "used" {
		t.Errorf("Generate should not return a pre-claimed slug, got %q", got)
	}
}

func TestContext_WithIDs(t *testing.T) {
	// Custom IDs implementation via WithIDs.
	custom := &recordingIDs{}
	ctx := parser.NewContext(parser.WithIDs(custom))
	got := ctx.IDs().Generate([]byte("X"), ast.KindHeading)
	if string(got) != "custom-X" {
		t.Errorf("WithIDs should install the custom IDs; got %q", got)
	}
	if custom.generateCalls != 1 {
		t.Errorf("Generate was not routed to custom IDs (calls=%d)", custom.generateCalls)
	}
}

// computeIfAbsentKey is allocated at package init time so it lives
// in the slice-backed store of any Context created in the tests
// below. ContextKeyMax grows on each NewContextKey call but the
// store is sized at NewContext time, so this must run first.
var computeIfAbsentKey = parser.NewContextKey()

func TestContext_ComputeIfAbsent(t *testing.T) {
	ctx := parser.NewContext()
	// First call computes; second call returns cached.
	v1 := ctx.ComputeIfAbsent(computeIfAbsentKey, func() any { return 42 })
	v2 := ctx.ComputeIfAbsent(computeIfAbsentKey, func() any { return 99 })
	if v1 != 42 {
		t.Errorf("first ComputeIfAbsent = %v, want 42", v1)
	}
	if v2 != 42 {
		t.Errorf("second ComputeIfAbsent must return cached 42, got %v", v2)
	}
}

func TestContext_References(t *testing.T) {
	// Parse a doc with two link references and verify the
	// References() accessor returns them.
	src := `[a]: /a
[b]: /b
[a] and [b]
`
	p := parser.NewParser(
		parser.WithBlockParsers(parser.DefaultBlockParsers()...),
		parser.WithInlineParsers(parser.DefaultInlineParsers()...),
		parser.WithParagraphTransformers(parser.DefaultParagraphTransformers()...),
	)
	ctx := parser.NewContext()
	p.Parse(text.NewReader([]byte(src)), parser.WithContext(ctx))
	refs := ctx.References()
	if len(refs) < 2 {
		t.Errorf("References() = %d, want >= 2", len(refs))
	}
}

func TestParser_WithEscapedSpace(t *testing.T) {
	p := parser.NewParser(
		parser.WithBlockParsers(parser.DefaultBlockParsers()...),
		parser.WithInlineParsers(parser.DefaultInlineParsers()...),
		parser.WithParagraphTransformers(parser.DefaultParagraphTransformers()...),
		parser.WithEscapedSpace(),
	)
	root := p.Parse(text.NewReader([]byte(`a\ b`+"\n")), parser.WithContext(parser.NewContext()))
	if root == nil {
		t.Fatal("Parse returned nil root")
	}
}

func TestParser_WithOption(t *testing.T) {
	// WithOption sets an arbitrary option by name.
	p := parser.NewParser(
		parser.WithBlockParsers(parser.DefaultBlockParsers()...),
		parser.WithInlineParsers(parser.DefaultInlineParsers()...),
		parser.WithParagraphTransformers(parser.DefaultParagraphTransformers()...),
		parser.WithOption(parser.OptionName("AutoHeadingID"), true),
	)
	root := p.Parse(text.NewReader([]byte("# Heading\n")), parser.WithContext(parser.NewContext()))
	if root == nil {
		t.Fatal("Parse returned nil root")
	}
}

// recordingIDs is a custom IDs implementation that records call
// counts and returns deterministic slugs prefixed with "custom-".
type recordingIDs struct {
	generateCalls int
	putCalls      int
}

func (r *recordingIDs) Generate(value []byte, kind ast.NodeKind) []byte {
	r.generateCalls++
	return append([]byte("custom-"), value...)
}

func (r *recordingIDs) Put(value []byte) {
	r.putCalls++
}
