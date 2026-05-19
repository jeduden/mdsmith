package nospaceinlinktext

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

func check(t *testing.T, src string, checkImages bool) []lint.Diagnostic {
	t.Helper()
	f, err := lint.NewFile("test.md", []byte(src))
	require.NoError(t, err)
	r := &Rule{CheckImages: checkImages}
	return r.Check(f)
}

func fix(t *testing.T, src string, checkImages bool) string {
	t.Helper()
	f, err := lint.NewFile("test.md", []byte(src))
	require.NoError(t, err)
	r := &Rule{CheckImages: checkImages}
	return string(r.Fix(f))
}

func TestCleanLink(t *testing.T) {
	diags := check(t, "# T\n\n[text](url)\n", true)
	assert.Empty(t, diags)
}

func TestLeadingSpace(t *testing.T) {
	diags := check(t, "# T\n\n[ text](url)\n", true)
	require.Len(t, diags, 1)
	assert.Equal(t, "link text has leading whitespace", diags[0].Message)
}

func TestTrailingSpace(t *testing.T) {
	diags := check(t, "# T\n\n[text ](url)\n", true)
	require.Len(t, diags, 1)
	assert.Equal(t, "link text has trailing whitespace", diags[0].Message)
}

func TestLeadingAndTrailingSpace(t *testing.T) {
	diags := check(t, "# T\n\n[ text ](url)\n", true)
	require.Len(t, diags, 2)
	msgs := []string{diags[0].Message, diags[1].Message}
	assert.Contains(t, msgs, "link text has leading whitespace")
	assert.Contains(t, msgs, "link text has trailing whitespace")
}

func TestFixLeadingAndTrailing(t *testing.T) {
	result := fix(t, "# T\n\n[ text ](url)\n", true)
	assert.Equal(t, "# T\n\n[text](url)\n", result)
}

func TestImageLeadingSpace(t *testing.T) {
	diags := check(t, "# T\n\n![ alt](img.png)\n", true)
	require.Len(t, diags, 1)
	assert.Equal(t, "image alt text has leading whitespace", diags[0].Message)
}

func TestImageTrailingSpace(t *testing.T) {
	diags := check(t, "# T\n\n![alt ](img.png)\n", true)
	require.Len(t, diags, 1)
	assert.Equal(t, "image alt text has trailing whitespace", diags[0].Message)
}

func TestImageBothSpaces(t *testing.T) {
	diags := check(t, "# T\n\n![ alt ](img.png)\n", true)
	require.Len(t, diags, 2)
	msgs := []string{diags[0].Message, diags[1].Message}
	assert.Contains(t, msgs, "image alt text has leading whitespace")
	assert.Contains(t, msgs, "image alt text has trailing whitespace")
}

func TestImageFixBothSpaces(t *testing.T) {
	result := fix(t, "# T\n\n![ alt ](img.png)\n", true)
	assert.Equal(t, "# T\n\n![alt](img.png)\n", result)
}

func TestCheckImagesDisabled(t *testing.T) {
	diags := check(t, "# T\n\n![ alt ](img.png)\n", false)
	assert.Empty(t, diags)
}

// TestFixImagesDisabled_LeavesImageAltUntouched pins the
// CheckImages=false branch on the Fix path (collectSpans): images
// must be skipped so their alt text is not stripped of intentional
// padding. The Check path is already covered by
// TestCheckImagesDisabled; this exercises the same predicate in
// the collectSpans loop used by Fix.
func TestFixImagesDisabled_LeavesImageAltUntouched(t *testing.T) {
	src := "# T\n\n![ alt ](img.png)\n"
	result := fix(t, src, false)
	assert.Equal(t, src, result, "Fix must leave image alt untouched when CheckImages=false")
}

// TestEmptyLinkText_NoDiagnostic pins the `len(inner) == 0`
// short-circuit in diagsForSpan: a link with empty brackets
// `[](url)` has zero inner bytes, so neither leading- nor
// trailing-space checks make sense — return without flagging.
// MDS049 leaves empty link text to MDS063 (descriptive-link-text)
// since "empty" is a content concern, not a whitespace concern.
func TestEmptyLinkText_NoDiagnostic(t *testing.T) {
	diags := check(t, "# T\n\n[](https://example.com)\n", true)
	assert.Empty(t, diags, "empty link text must not trigger a leading/trailing-space diagnostic")
}

func TestNewlineInLinkTextNotFlagged(t *testing.T) {
	// Newline between words inside brackets must not be flagged.
	diags := check(t, "# T\n\n[long text that\nwraps](url)\n", true)
	assert.Empty(t, diags)
}

func TestNoChange(t *testing.T) {
	src := "# T\n\n[text](url)\n"
	result := fix(t, src, true)
	assert.Equal(t, src, result)
}

func TestApplySettings_CheckImages(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"check-images": false})
	require.NoError(t, err)
	assert.False(t, r.CheckImages)
}

func TestApplySettings_Unknown(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"unknown": true})
	require.Error(t, err)
}

func TestDefaultSettings(t *testing.T) {
	r := &Rule{}
	s := r.DefaultSettings()
	assert.Equal(t, true, s["check-images"])
}

func TestEnabledByDefault(t *testing.T) {
	r := &Rule{}
	assert.False(t, r.EnabledByDefault())
}

func TestCategory(t *testing.T) {
	r := &Rule{}
	assert.Equal(t, "link", r.Category())
}

func TestApplySettings_CheckImagesWrongType(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"check-images": "yes"})
	require.Error(t, err)
}

func TestEmptyLinkText(t *testing.T) {
	// [](url) has no text child; bracketSpan returns (-1,-1) and is skipped.
	diags := check(t, "# T\n\n[](url)\n", true)
	assert.Empty(t, diags)
}

func TestEmptyLinkTextFix(t *testing.T) {
	src := "# T\n\n[](url)\n"
	result := fix(t, src, true)
	assert.Equal(t, src, result)
}

func TestLinkWithImageChild(t *testing.T) {
	// [![img](img.png)](url) — outer link wraps an image with clean alt text.
	// Outer link inner text is "![img](img.png)" — first byte '!', not space.
	// The inner image has no whitespace in alt text.
	diags := check(t, "# T\n\n[![img](img.png)](url)\n", true)
	assert.Empty(t, diags)
}

func TestLinkWithImageChildSpacedAlt(t *testing.T) {
	// [![ alt ](img.png)](url) — inner image has leading/trailing space in alt.
	// Outer link inner text is "![ alt ](img.png)" — first byte '!', not space.
	// The inner image produces two diagnostics.
	diags := check(t, "# T\n\n[![ alt ](img.png)](url)\n", true)
	require.Len(t, diags, 2)
	msgs := []string{diags[0].Message, diags[1].Message}
	assert.Contains(t, msgs, "image alt text has leading whitespace")
	assert.Contains(t, msgs, "image alt text has trailing whitespace")
}

func TestLinkWithImageChildFixSpacedAlt(t *testing.T) {
	result := fix(t, "# T\n\n[![ alt ](img.png)](url)\n", true)
	assert.Equal(t, "# T\n\n[![alt](img.png)](url)\n", result)
}

func TestBracketInLinkText(t *testing.T) {
	// [foo [bar] baz](url) — nested brackets exercise depth tracking.
	diags := check(t, "# T\n\n[foo [bar] baz](url)\n", true)
	assert.Empty(t, diags)
}

func TestEmphasisFirstChildTrailingSpace(t *testing.T) {
	// [*x* ](url) — emphasis is the first child, trailing space follows.
	diags := check(t, "# T\n\n[*x* ](url)\n", true)
	require.Len(t, diags, 1)
	assert.Equal(t, "link text has trailing whitespace", diags[0].Message)
}

func TestEmphasisFirstChildFix(t *testing.T) {
	result := fix(t, "# T\n\n[*x* ](url)\n", true)
	assert.Equal(t, "# T\n\n[*x*](url)\n", result)
}

func TestImageEmphasisFirstChild(t *testing.T) {
	// ![ *alt* ](img.png) — emphasis inside image alt text.
	diags := check(t, "# T\n\n![ *alt* ](img.png)\n", true)
	require.Len(t, diags, 2)
	msgs := []string{diags[0].Message, diags[1].Message}
	assert.Contains(t, msgs, "image alt text has leading whitespace")
	assert.Contains(t, msgs, "image alt text has trailing whitespace")
}

func TestEscapedBracketInLinkText(t *testing.T) {
	// [ text \] ](url) — escaped ] inside link text should not terminate scan.
	// Goldmark parses \] as an escaped bracket; the real closing ] is later.
	diags := check(t, "# T\n\n[ text \\] ](url)\n", true)
	// Leading space and trailing space around the escaped bracket content.
	msgs := make([]string, len(diags))
	for i, d := range diags {
		msgs[i] = d.Message
	}
	assert.Contains(t, msgs, "link text has leading whitespace")
	assert.Contains(t, msgs, "link text has trailing whitespace")
}

func TestLinkWithTextAndNestedImage(t *testing.T) {
	// [ text ![ alt ](img.png) ](url) — outer link has boundary spaces,
	// inner image also has spaces in alt text.
	// Check reports all four diagnostics.
	diags := check(t, "# T\n\n[ text ![ alt ](img.png) ](url)\n", true)
	msgs := make([]string, len(diags))
	for i, d := range diags {
		msgs[i] = d.Message
	}
	assert.Contains(t, msgs, "link text has leading whitespace")
	assert.Contains(t, msgs, "link text has trailing whitespace")
	assert.Contains(t, msgs, "image alt text has leading whitespace")
	assert.Contains(t, msgs, "image alt text has trailing whitespace")
}

func TestFixNestedSpanSinglePass(t *testing.T) {
	// Fix must trim both outer link and inner image whitespace in a single call.
	src := "# T\n\n[ text ![ alt ](img.png) ](url)\n"
	result := fix(t, src, true)
	assert.Equal(t, "# T\n\n[text ![alt](img.png)](url)\n", result)
}

func TestLinkWithTextAndNestedImageClean(t *testing.T) {
	// [ text ![clean](img.png) ](url) — outer link has boundary spaces but inner image has clean alt.
	// Only link text diagnostics; inner image has no alt whitespace.
	diags := check(t, "# T\n\n[ text ![clean](img.png) ](url)\n", true)
	msgs := make([]string, len(diags))
	for i, d := range diags {
		msgs[i] = d.Message
	}
	assert.Contains(t, msgs, "link text has leading whitespace")
	assert.Contains(t, msgs, "link text has trailing whitespace")
}

func TestLinkWithImageThenTrailingSpace(t *testing.T) {
	// [ ![img](img.png) tail ](url) — link has image child followed by text;
	// both boundary spaces must be detected.
	diags := check(t, "# T\n\n[ ![img](img.png) tail ](url)\n", true)
	msgs := make([]string, len(diags))
	for i, d := range diags {
		msgs[i] = d.Message
	}
	assert.Contains(t, msgs, "link text has leading whitespace")
	assert.Contains(t, msgs, "link text has trailing whitespace")
}

func TestLinkStartingWithImageTrailingSpace(t *testing.T) {
	// [![img](img.png) tail ](url) — no space between [ and ![, so minStart is
	// inside the image subtree. The backward scan hits the image's [ first and
	// must skip it (imageOpener=true, img=false) to find the outer [.
	diags := check(t, "# T\n\n[![img](img.png) tail ](url)\n", true)
	msgs := make([]string, len(diags))
	for i, d := range diags {
		msgs[i] = d.Message
	}
	assert.Contains(t, msgs, "link text has trailing whitespace")
}

func TestLinkWithOnlyImageAndBoundarySpaces(t *testing.T) {
	// [ ![alt](img.png) ](url) — link content is only an image with surrounding spaces.
	// The outer link's inner text starts with space; leading and trailing are detected.
	diags := check(t, "# T\n\n[ ![alt](img.png) ](url)\n", true)
	msgs := make([]string, len(diags))
	for i, d := range diags {
		msgs[i] = d.Message
	}
	assert.Contains(t, msgs, "link text has leading whitespace")
	assert.Contains(t, msgs, "link text has trailing whitespace")
}

func TestFixLinkWithOnlyImageAndBoundarySpaces(t *testing.T) {
	result := fix(t, "# T\n\n[ ![alt](img.png) ](url)\n", true)
	assert.Equal(t, "# T\n\n[![alt](img.png)](url)\n", result)
}

func TestEscapedExclamationBeforeLink(t *testing.T) {
	// \![ text ](url) — escaped '!' before '['; goldmark parses this as a link,
	// not an image. imageOpener checks the backslash count and returns false,
	// so the [ is correctly identified as the link opener.
	diags := check(t, "# T\n\n\\![ text ](url)\n", true)
	msgs := make([]string, len(diags))
	for i, d := range diags {
		msgs[i] = d.Message
	}
	assert.Contains(t, msgs, "link text has leading whitespace")
	assert.Contains(t, msgs, "link text has trailing whitespace")
}

func TestCodeSpanWithCloseBracketNotFlaggedAsClose(t *testing.T) {
	// [ text `]` ](url) — `]` inside code span (forward scan) must not terminate
	// the forward scan early. The trailing space before the real `]` is detected.
	diags := check(t, "# T\n\n[ text `]` ](url)\n", true)
	msgs := make([]string, len(diags))
	for i, d := range diags {
		msgs[i] = d.Message
	}
	assert.Contains(t, msgs, "link text has leading whitespace")
	assert.Contains(t, msgs, "link text has trailing whitespace")
}

func TestCodeSpanWithOpenBracketNotFlaggedAsNested(t *testing.T) {
	// [ `[` text ](url) — `[` inside code span (forward scan) must not increment
	// bracket depth. The leading space is detected.
	diags := check(t, "# T\n\n[ `[` text](url)\n", true)
	msgs := make([]string, len(diags))
	for i, d := range diags {
		msgs[i] = d.Message
	}
	assert.Contains(t, msgs, "link text has leading whitespace")
}

func TestCodeSpanWithOpenBracketBackwardScan(t *testing.T) {
	// [`[code]` text ](url) — no space between [ and the code span, so the backward
	// scan starts after the code span and must skip it via skipCodeSpanBackward.
	// The trailing space is detected.
	diags := check(t, "# T\n\n[`[code]` text ](url)\n", true)
	msgs := make([]string, len(diags))
	for i, d := range diags {
		msgs[i] = d.Message
	}
	assert.Contains(t, msgs, "link text has trailing whitespace")
}

func TestDoubleBacktickCodeSpanBackwardScan(t *testing.T) {
	// [``x`y`` text ](url) — double-backtick code span with a single backtick
	// inside. skipCodeSpanBackward must skip the wrong-length single-backtick
	// sequence before finding the double-backtick opener. Trailing space detected.
	diags := check(t, "# T\n\n[``x`y`` text ](url)\n", true)
	msgs := make([]string, len(diags))
	for i, d := range diags {
		msgs[i] = d.Message
	}
	assert.Contains(t, msgs, "link text has trailing whitespace")
}

func TestDoubleBacktickCodeSpanForwardScan(t *testing.T) {
	// [ text ``x`y`` ](url) — double-backtick code span (forward scan) with
	// single backtick inside; skipCodeSpan must skip the wrong-length sequence.
	diags := check(t, "# T\n\n[ text ``x`y`` ](url)\n", true)
	msgs := make([]string, len(diags))
	for i, d := range diags {
		msgs[i] = d.Message
	}
	assert.Contains(t, msgs, "link text has leading whitespace")
	assert.Contains(t, msgs, "link text has trailing whitespace")
}

func TestSkipCodeSpanBackwardUnit(t *testing.T) {
	// Unit test for skipCodeSpanBackward directly.
	// Source: abc``x`y``def where double-backtick code span ends at position 10.
	src := []byte("abc``x`y``def")
	// Closing `` is at positions 8-9. Call with i=9 (last char of closer).
	result := skipCodeSpanBackward(src, 9)
	// Should return position before the opening ``, which starts at position 3.
	// j=3 at end, return j-1=2.
	assert.Equal(t, 2, result)
}

func TestSkipCodeSpanBackwardWrongLength(t *testing.T) {
	// skipCodeSpanBackward with n=2, encountering single-backtick first.
	// Source: abc`d``ef``gh where double-backtick closer ends at 9.
	// Single backtick at 3 has wrong length; opener is at 5-6.
	src := []byte("abc`d``ef``gh")
	// Closer `` is at positions 9-10. Call with i=10 (last ` of closer).
	result := skipCodeSpanBackward(src, 10)
	// Opener `` is at 5-6. return j-1 = 4.
	assert.Equal(t, 4, result)
}

func TestFindOpenBracketSkipsCodeSpan(t *testing.T) {
	// Direct unit test for findOpenBracket: source has a code span with [ inside.
	// The backward scan must call skipCodeSpanBackward and use the returned position.
	// Source bytes: [ `[code]` text (representing a link with code span content).
	src := []byte("[ `[code]` text")
	// Text "text" starts at position 11; scan backward from 10.
	result := findOpenBracket(src, false, 11)
	assert.Equal(t, 0, result)
}

func TestFindOpenBracketNoOpenerFound(t *testing.T) {
	// findOpenBracket returns -1 when no suitable [ is found.
	src := []byte("no bracket here")
	result := findOpenBracket(src, false, len(src))
	assert.Equal(t, -1, result)
}

func TestFindOpenBracketEscapedBracket(t *testing.T) {
	// findOpenBracket skips \[ (escaped open bracket) and finds the real opener.
	// Bytes: [ (0) space (1) \ (2) [ (3) t (4) e (5) x (6) t (7)
	// Scanning backward from from=5 encounters [ at 3 (escaped, bs=1 → skip),
	// then finds the real [ at 0.
	src := []byte("[ \\[text")
	result := findOpenBracket(src, false, 5)
	assert.Equal(t, 0, result)
}

func TestSkipCodeSpanNoCloser(t *testing.T) {
	// skipCodeSpan returns len(source) when no matching closer exists.
	src := []byte("`abc")
	result := skipCodeSpan(src, 0)
	assert.Equal(t, len(src), result)
}

func TestFindCloseBracketUnmatched(t *testing.T) {
	// findCloseBracket returns -1 when the opening [ has no matching ].
	src := []byte("[unclosed")
	result := findCloseBracket(src, 0)
	assert.Equal(t, -1, result)
}

func TestBracketSpanOpenBracketNotFound(t *testing.T) {
	// bracketSpan returns (-1,-1) when no [ precedes the text in source.
	// Crafted: a Link node with a Text child at segment [5,10), but source has no [ before 5.
	src := []byte("hello world text")
	link := ast.NewLink()
	link.AppendChild(link, ast.NewTextSegment(text.NewSegment(5, 10)))
	open, close := bracketSpan(link, src)
	assert.Equal(t, -1, open)
	assert.Equal(t, -1, close)
}

func TestUnmatchedBacktickInLinkText(t *testing.T) {
	// [ text `broken ](url) — unmatched backtick inside link text.
	// goldmark parses this as a valid link; the rule must detect the trailing space.
	diags := check(t, "# T\n\n[ text `broken ](url)\n", true)
	msgs := make([]string, len(diags))
	for i, d := range diags {
		msgs[i] = d.Message
	}
	assert.Contains(t, msgs, "link text has leading whitespace")
}

func TestBracketSpanCloseBracketNotFound(t *testing.T) {
	// bracketSpan returns (-1,-1) when [ is found but has no matching ].
	// Crafted: source "abc [hello", [ at 4, text at 5, no ] anywhere.
	src := []byte("abc [hello")
	link := ast.NewLink()
	link.AppendChild(link, ast.NewTextSegment(text.NewSegment(5, 10)))
	open, close := bracketSpan(link, src)
	assert.Equal(t, -1, open)
	assert.Equal(t, -1, close)
}

func checkWithGeneratedRanges(t *testing.T, src string, ranges []lint.LineRange) []lint.Diagnostic {
	t.Helper()
	f, err := lint.NewFile("test.md", []byte(src))
	require.NoError(t, err)
	f.GeneratedRanges = ranges
	r := &Rule{CheckImages: true}
	return r.Check(f)
}

func fixWithGeneratedRanges(t *testing.T, src string, ranges []lint.LineRange) string {
	t.Helper()
	f, err := lint.NewFile("test.md", []byte(src))
	require.NoError(t, err)
	f.GeneratedRanges = ranges
	r := &Rule{CheckImages: true}
	return string(r.Fix(f))
}

func TestCheckSkipsGeneratedRange(t *testing.T) {
	// Link with leading whitespace on line 3, which is declared as a generated range.
	// Check must produce no diagnostics for that link.
	src := "# T\n\n[ text ](url)\n"
	diags := checkWithGeneratedRanges(t, src, []lint.LineRange{{From: 3, To: 3}})
	assert.Empty(t, diags)
}

func TestCheckOutsideGeneratedRangeStillFires(t *testing.T) {
	// Line 3 is clean; line 5 has leading whitespace and is outside the generated range (line 3).
	src := "# T\n\n[clean](url)\n\n[ text ](url2)\n"
	diags := checkWithGeneratedRanges(t, src, []lint.LineRange{{From: 3, To: 3}})
	msgs := make([]string, len(diags))
	for i, d := range diags {
		msgs[i] = d.Message
	}
	assert.Contains(t, msgs, "link text has leading whitespace")
}

func TestFixSkipsGeneratedRange(t *testing.T) {
	// Fix must not rewrite the link on line 3 when it falls inside a generated range.
	src := "# T\n\n[ text ](url)\n"
	result := fixWithGeneratedRanges(t, src, []lint.LineRange{{From: 3, To: 3}})
	assert.Equal(t, src, result)
}

func TestTabLeadingWhitespace(t *testing.T) {
	// [\ttext](url) — tab as leading whitespace inside link brackets.
	diags := check(t, "# T\n\n[\ttext](url)\n", true)
	require.Len(t, diags, 1)
	assert.Equal(t, "link text has leading whitespace", diags[0].Message)
}

func TestTabTrailingWhitespace(t *testing.T) {
	// [text\t](url) — tab as trailing whitespace inside link brackets.
	diags := check(t, "# T\n\n[text\t](url)\n", true)
	require.Len(t, diags, 1)
	assert.Equal(t, "link text has trailing whitespace", diags[0].Message)
}

func TestFixTabWhitespace(t *testing.T) {
	// Fix must trim leading and trailing tabs as well as spaces.
	result := fix(t, "# T\n\n[\ttext\t](url)\n", true)
	assert.Equal(t, "# T\n\n[text](url)\n", result)
}

func TestWhitespaceOnlyImageAltNotFlagged(t *testing.T) {
	// ![ ](img.png) — alt text is only whitespace; trimming would produce
	// empty alt, which is an accessibility problem (MDS032), not a formatting
	// problem. MDS049 must not flag it.
	diags := check(t, "# T\n\n![ ](img.png)\n", true)
	assert.Empty(t, diags)
}

func TestWhitespaceOnlyImageAltNotFixed(t *testing.T) {
	// Fix must not trim ![ ](img.png) to ![](img.png) when a clean link is also
	// present — both Check (skip image-only-whitespace) and fixSpans
	// (img && bytes.TrimSpace empty → leave as-is) must be exercised.
	src := "# T\n\n[ text ](url)\n\n![ ](img.png)\n"
	result := fix(t, src, true)
	assert.Equal(t, "# T\n\n[text](url)\n\n![ ](img.png)\n", result)
}

func TestWhitespaceOnlyImageAltWithNewlineNotFlagged(t *testing.T) {
	// ![ \n ](img.png) — alt text is spaces+newline+spaces; TrimSpace → empty,
	// so MDS032 already covers this. MDS049 must not flag the edge spaces.
	diags := check(t, "# T\n\n![ \n ](img.png)\n", true)
	assert.Empty(t, diags)
}

func TestWhitespaceOnlyLinkTextIsFlagged(t *testing.T) {
	// [ ](url) — link text is only whitespace. Unlike images there is no
	// separate empty-label rule for links, so MDS049 must still flag this.
	diags := check(t, "# T\n\n[ ](url)\n", true)
	msgs := make([]string, len(diags))
	for i, d := range diags {
		msgs[i] = d.Message
	}
	assert.Contains(t, msgs, "link text has leading whitespace")
	assert.Contains(t, msgs, "link text has trailing whitespace")
}

func TestWhitespaceOnlyLinkTextIsFixed(t *testing.T) {
	// [ ](url) — Fix trims the whitespace-only link label to empty.
	// No separate empty-link-text rule exists, so trimming is the right action.
	result := fix(t, "# T\n\n[ ](url)\n", true)
	assert.Equal(t, "# T\n\n[](url)\n", result)
}

func TestReferenceImageLeadingSpace(t *testing.T) {
	// ![ alt ][ref] — reference-style image with leading/trailing space in alt text.
	src := "# T\n\n![ alt ][ref]\n\n[ref]: img.png\n"
	diags := check(t, src, true)
	msgs := make([]string, len(diags))
	for i, d := range diags {
		msgs[i] = d.Message
	}
	assert.Contains(t, msgs, "image alt text has leading whitespace")
	assert.Contains(t, msgs, "image alt text has trailing whitespace")
}

func TestFixReferenceImage(t *testing.T) {
	// Fix must trim alt text whitespace for reference-style images.
	src := "# T\n\n![ alt ][ref]\n\n[ref]: img.png\n"
	result := fix(t, src, true)
	assert.Equal(t, "# T\n\n![alt][ref]\n\n[ref]: img.png\n", result)
}
