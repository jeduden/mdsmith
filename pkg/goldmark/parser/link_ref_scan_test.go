package parser_test

import (
	"testing"

	"github.com/jeduden/mdsmith/pkg/goldmark/ast"
	"github.com/jeduden/mdsmith/pkg/goldmark/parser"
	"github.com/jeduden/mdsmith/pkg/goldmark/text"
	"github.com/jeduden/mdsmith/pkg/goldmark/util"
)

// refKeySet collects the normalised reference labels in pc.References()
// with their destination and title, so a scan result can be compared to
// a full parse result independent of slice order.
func refKeySet(refs []parser.Reference) map[string][2]string {
	out := map[string][2]string{}
	for _, r := range refs {
		out[util.ToLinkReference(r.Label())] = [2]string{
			string(r.Destination()), string(r.Title()),
		}
	}
	return out
}

// fullParseRefs parses src with the default parser and returns its
// collected reference map.
func fullParseRefs(src string) map[string][2]string {
	ctx := parser.NewContext()
	p := parser.NewParser(
		parser.WithBlockParsers(parser.DefaultBlockParsers()...),
		parser.WithInlineParsers(parser.DefaultInlineParsers()...),
		parser.WithParagraphTransformers(parser.DefaultParagraphTransformers()...),
	)
	p.Parse(text.NewReader([]byte(src)), parser.WithContext(ctx))
	return refKeySet(ctx.References())
}

// docLineSegments splits source into one Segment per line, each ending
// just past its newline, matching goldmark's reader line boundaries.
func docLineSegments(source []byte) *text.Segments {
	segs := text.NewSegments()
	start := 0
	for i := 0; i < len(source); i++ {
		if source[i] == '\n' {
			segs.Append(text.NewSegment(start, i+1))
			start = i + 1
		}
	}
	if start < len(source) {
		segs.Append(text.NewSegment(start, len(source)))
	}
	return segs
}

// scanWholeDocRefs feeds the entire document as a single block to
// ScanReferenceDefinitions. For documents whose definitions all sit at
// the very top this matches the full parse; the per-paragraph splitting
// is exercised by the lint-side equivalence test.
func scanWholeDocRefs(src string) map[string][2]string {
	ctx := parser.NewContext()
	parser.ScanReferenceDefinitions([]byte(src), docLineSegments([]byte(src)), ctx)
	return refKeySet(ctx.References())
}

func TestScanReferenceDefinitions_MatchesFullParse(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"single", "[a]: /a\n"},
		{"multiple", "[a]: /a\n[b]: /b\n[c]: /c\n"},
		{"title-double", "[a]: /a \"title\"\n"},
		{"title-single", "[a]: /a 'title'\n"},
		{"title-paren", "[a]: /a (title)\n"},
		{"angle-dest", "[a]: <http://x/y>\n"},
		{"case-fold", "[FooBar]: /x\n"},
		{"title-next-line", "[a]: /a\n   \"title\"\n"},
		{"dup-first-wins", "[a]: /first\n[a]: /second\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			want := fullParseRefs(tc.src)
			got := scanWholeDocRefs(tc.src)
			if len(got) != len(want) {
				t.Fatalf("ref count: got %d want %d\ngot=%v want=%v",
					len(got), len(want), got, want)
			}
			for k, wv := range want {
				gv, ok := got[k]
				if !ok {
					t.Fatalf("missing label %q", k)
				}
				if gv != wv {
					t.Fatalf("label %q: got %v want %v", k, gv, wv)
				}
			}
		})
	}
}

// TestScanReferenceDefinitions_StopsAtNonDefinition pins that a trailing
// paragraph line after the head definitions is not misread as a label.
func TestScanReferenceDefinitions_StopsAtNonDefinition(t *testing.T) {
	src := "[a]: /a\nThis is prose, not a [definition].\n"
	got := scanWholeDocRefs(src)
	want := map[string][2]string{"a": {"/a", ""}}
	if len(got) != len(want) {
		t.Fatalf("ref count: got %d want %d\ngot=%v want=%v", len(got), len(want), got, want)
	}
	for k, wv := range want {
		gv, ok := got[k]
		if !ok {
			t.Fatalf("missing label %q", k)
		}
		if gv != wv {
			t.Fatalf("label %q: got %v want %v", k, gv, wv)
		}
	}
}

func TestScanReferenceDefinitions_EmptyIsNoop(t *testing.T) {
	ctx := parser.NewContext()
	parser.ScanReferenceDefinitions(nil, nil, ctx)
	if len(ctx.References()) != 0 {
		t.Fatalf("expected no refs from nil segments")
	}

	// Non-nil but empty *text.Segments must also be a no-op. Use a fresh
	// context so a bug in the nil path doesn't shadow the error message.
	ctx2 := parser.NewContext()
	empty := text.NewSegments()
	parser.ScanReferenceDefinitions([]byte("[a]: /a\n"), empty, ctx2)
	if len(ctx2.References()) != 0 {
		t.Fatalf("expected no refs from empty segments")
	}
	_ = ast.KindParagraph // keep ast import meaningful
}
