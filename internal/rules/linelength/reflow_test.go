package linelength

import (
	"reflect"
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
)

func TestIsAbbrev_TrainedModel(t *testing.T) {
	r := &Rule{}
	cases := []struct {
		tok  string
		want bool
	}{
		{"Dr.", true},   // trained honorific
		{"vs.", true},   // trained reference form
		{"Mr.", true},   // trained honorific
		{"e.g.", true},  // dotted pattern
		{"i.e.", true},  // dotted pattern
		{"J.", true},    // initial
		{"e.g.,", true}, // trailing comma trimmed
		{"etc.", false}, // not in the trained model (needs config)
		{"cat.", false}, // ordinary word ending a sentence
		{"plain", false},
		{",", false}, // trims to empty
	}
	for _, c := range cases {
		if got := r.isAbbrev(c.tok); got != c.want {
			t.Errorf("isAbbrev(%q) = %v, want %v", c.tok, got, c.want)
		}
	}
}

func TestIsAbbrev_ConfiguredExtension(t *testing.T) {
	// "etc." and "approx." are not in the trained model, so they prove
	// the configured list extends detection.
	r := &Rule{Abbreviations: []string{"etc.", "approx."}}
	if !r.isAbbrev("etc.") {
		t.Errorf("configured abbreviation etc. should glue")
	}
	if !r.isAbbrev("approx.") {
		t.Errorf("configured abbreviation approx. should glue")
	}
	if !r.isAbbrev("etc.,") {
		t.Errorf("configured abbreviation should match after trimming punctuation")
	}
	// Trained entries still glue alongside a configured extension list.
	if !r.isAbbrev("Dr.") {
		t.Errorf("trained Dr. should still glue alongside extensions")
	}
}

func TestTokenizeParagraph_Plain(t *testing.T) {
	src := []byte("foo bar\nbaz qux")
	got := tokenizeParagraph(src, 0, len(src), nil)
	want := []string{"foo", "bar", "baz", "qux"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("tokenizeParagraph = %q, want %q", got, want)
	}
}

func TestTokenizeParagraph_CodeSpanAtomicPreservesSpaces(t *testing.T) {
	src := []byte("a `b  c` d")
	spans := []lint.Range{{Start: 2, End: 8}} // "`b  c`"
	got := tokenizeParagraph(src, 0, len(src), spans)
	want := []string{"a", "`b  c`", "d"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("tokenizeParagraph = %q, want %q", got, want)
	}
}

func TestTokenizeParagraph_CodeSpanGluedToText(t *testing.T) {
	src := []byte("pre`code`post tail")
	spans := []lint.Range{{Start: 3, End: 9}} // "`code`"
	got := tokenizeParagraph(src, 0, len(src), spans)
	want := []string{"pre`code`post", "tail"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("tokenizeParagraph = %q, want %q", got, want)
	}
}

func TestTokenizeParagraph_CodeSpanNewlineBecomesSpace(t *testing.T) {
	src := []byte("x `a\nb` y")
	spans := []lint.Range{{Start: 2, End: 7}} // "`a\nb`"
	got := tokenizeParagraph(src, 0, len(src), spans)
	want := []string{"x", "`a b`", "y"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("tokenizeParagraph = %q, want %q", got, want)
	}
}

func TestTokenizeParagraph_SkipsSpansBeforeStart(t *testing.T) {
	// A code span that ends before the paragraph start is skipped by the
	// initial advance loop; tokenizing begins cleanly at start.
	src := []byte("xx foo bar")
	got := tokenizeParagraph(src, 3, len(src), []lint.Range{{Start: 0, End: 2}})
	want := []string{"foo", "bar"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("tokenizeParagraph = %q, want %q", got, want)
	}
}

func TestWrapTokens_GreedyNoGlue(t *testing.T) {
	tokens := []string{"The", "quick", "brown", "fox", "jumps"}
	noGlue := func(string) bool { return false }
	got := wrapTokens(tokens, "", 11, noGlue)
	want := []string{"The quick", "brown fox", "jumps"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("wrapTokens = %q, want %q", got, want)
	}
}

func TestWrapTokens_GluePreventsBreakAfterAbbrev(t *testing.T) {
	tokens := []string{"aaaa", "e.g.", "bbbb"}
	noGlue := func(string) bool { return false }
	glue := func(prev string) bool { return prev == "e.g." }

	gotNo := wrapTokens(tokens, "", 9, noGlue)
	wantNo := []string{"aaaa e.g.", "bbbb"} // line ends with the abbreviation
	if !reflect.DeepEqual(gotNo, wantNo) {
		t.Fatalf("wrapTokens(noGlue) = %q, want %q", gotNo, wantNo)
	}

	gotGlue := wrapTokens(tokens, "", 9, glue)
	// "e.g. bbbb" is one unit; it does not fit after "aaaa", so it wraps
	// whole — the abbreviation is never left at a line end, and the line
	// never overflows width.
	wantGlue := []string{"aaaa", "e.g. bbbb"}
	if !reflect.DeepEqual(gotGlue, wantGlue) {
		t.Errorf("wrapTokens(glue) = %q, want %q", gotGlue, wantGlue)
	}
}

func TestWrapTokens_GluedUnitWrapsWholeNoOverflow(t *testing.T) {
	// A run of initials glued to the following word forms one unit that
	// wraps as a whole rather than overflowing the line.
	tokens := []string{"shelved", "J.", "R.", "R.", "Tolkien", "today"}
	initialGlue := func(prev string) bool { return prev == "J." || prev == "R." }
	got := wrapTokens(tokens, "", 16, initialGlue)
	want := []string{"shelved", "J. R. R. Tolkien", "today"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("wrapTokens = %q, want %q", got, want)
	}
}

func TestWrapTokens_LongTokenOwnsLine(t *testing.T) {
	tokens := []string{"short", "thisisaverylongunbreakabletoken", "tail"}
	got := wrapTokens(tokens, "", 10, func(string) bool { return false })
	want := []string{"short", "thisisaverylongunbreakabletoken", "tail"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("wrapTokens = %q, want %q", got, want)
	}
}

func TestWrapTokens_Indent(t *testing.T) {
	tokens := []string{"alpha", "beta", "gamma"}
	got := wrapTokens(tokens, "  ", 12, func(string) bool { return false })
	want := []string{"  alpha beta", "  gamma"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("wrapTokens = %q, want %q", got, want)
	}
}

func TestWrapTokens_Empty(t *testing.T) {
	if got := wrapTokens(nil, "", 80, func(string) bool { return false }); got != nil {
		t.Errorf("wrapTokens(nil) = %q, want nil", got)
	}
}

func TestHasHardLineBreak(t *testing.T) {
	cases := []struct {
		line string
		want bool
	}{
		{"plain text", false},
		{"break here  ", true},      // two trailing spaces
		{"backslash break\\", true}, // trailing backslash
		{"one space ", false},       // a single trailing space is not a hard break
		{"text\\ ", false},          // backslash not at the very end
	}
	for _, c := range cases {
		if got := hasHardLineBreak([]byte(c.line)); got != c.want {
			t.Errorf("hasHardLineBreak(%q) = %v, want %v", c.line, got, c.want)
		}
	}
}

func TestLeadingWhitespace(t *testing.T) {
	if got := leadingWhitespace([]byte("  text")); got != "  " {
		t.Errorf("leadingWhitespace = %q, want two spaces", got)
	}
	if got := leadingWhitespace([]byte("text")); got != "" {
		t.Errorf("leadingWhitespace = %q, want empty", got)
	}
}
