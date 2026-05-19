package headingwhitespace

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newFile(t *testing.T, src string) *lint.File {
	t.Helper()
	f, err := lint.NewFile("test.md", []byte(src))
	require.NoError(t, err)
	return f
}

func check(t *testing.T, src string) []lint.Diagnostic {
	t.Helper()
	return (&Rule{}).Check(newFile(t, src))
}

func fix(t *testing.T, src string) string {
	t.Helper()
	return string((&Rule{}).Fix(newFile(t, src)))
}

// --- Check: no violations ---

func TestCheck_Clean(t *testing.T) {
	diags := check(t, "# Title\n\n## Section\n\n### Sub\n")
	assert.Empty(t, diags)
}

func TestCheck_EmptyHeading(t *testing.T) {
	diags := check(t, "# Title\n\n##\n")
	assert.Empty(t, diags)
}

func TestCheck_ClosedATX_CorrectSpacing_Flagged(t *testing.T) {
	diags := check(t, "# Title\n\n# Heading #\n")
	require.Len(t, diags, 1)
	assert.Equal(t, "heading has closing # marker", diags[0].Message)
	assert.Equal(t, 3, diags[0].Line)
	assert.Equal(t, 10, diags[0].Column)
}

// --- Check: MD018 missing space ---

func TestCheck_MissingSpace(t *testing.T) {
	diags := check(t, "# Title\n\n#Heading\n")
	require.Len(t, diags, 1)
	assert.Equal(t, "missing space after # in heading", diags[0].Message)
	assert.Equal(t, 3, diags[0].Line)
	assert.Equal(t, 2, diags[0].Column)
}

func TestCheck_MissingSpace_Level2(t *testing.T) {
	diags := check(t, "# Title\n\n##Heading\n")
	require.Len(t, diags, 1)
	assert.Equal(t, "missing space after # in heading", diags[0].Message)
	assert.Equal(t, 3, diags[0].Column)
}

// --- Check: MD019 multiple spaces ---

func TestCheck_MultipleSpaces(t *testing.T) {
	diags := check(t, "# Title\n\n#  Heading\n")
	require.Len(t, diags, 1)
	assert.Equal(t, "multiple spaces or tabs after # in heading", diags[0].Message)
	assert.Equal(t, 3, diags[0].Line)
	assert.Equal(t, 3, diags[0].Column)
}

func TestCheck_SpaceThenTab(t *testing.T) {
	// "# \tHeading": first char is space so MD018 doesn't fire, but
	// leadingSpaces counts the tab too → fires "multiple spaces or tabs".
	diags := check(t, "# Title\n\n# \tHeading\n")
	require.Len(t, diags, 1)
	assert.Equal(t, "multiple spaces or tabs after # in heading", diags[0].Message)
	assert.Equal(t, 3, diags[0].Line)
	assert.Equal(t, 3, diags[0].Column)
}

func TestFix_SpaceThenTab(t *testing.T) {
	assert.Equal(t, "# Title\n\n# Heading\n", fix(t, "# Title\n\n# \tHeading\n"))
}

// --- Check: MD023 indented heading ---

func TestCheck_Indented(t *testing.T) {
	diags := check(t, "# Title\n\n   # Heading\n")
	require.Len(t, diags, 1)
	assert.Equal(t, "heading must start at column 1", diags[0].Message)
	assert.Equal(t, 3, diags[0].Line)
	assert.Equal(t, 1, diags[0].Column)
}

// --- Check: #Heading# — trailing # without preceding space is content, not a closing marker ---

func TestCheck_ClosedATX_NoSpace(t *testing.T) {
	// Per CommonMark, a closing # sequence requires preceding whitespace.
	// "#Heading#" has no space before the trailing "#", so only MD018 fires.
	diags := check(t, "# Title\n\n#Heading#\n")
	require.Len(t, diags, 1)
	assert.Equal(t, "missing space after # in heading", diags[0].Message)
}

func TestCheck_ClosedATX_NoSpace_Column(t *testing.T) {
	diags := check(t, "#Heading#\n")
	require.Len(t, diags, 1)
	assert.Equal(t, 2, diags[0].Column)
}

// --- Check: content ending with # is not a false positive ---

func TestCheck_ContentEndingWithHash(t *testing.T) {
	// "# C#" — trailing # has no preceding space; it is content, not a closing marker.
	diags := check(t, "# Title\n\n# C#\n")
	assert.Empty(t, diags)
}

// --- Check: tab after # is flagged (normalise to single space) ---

func TestCheck_TabAfterHash(t *testing.T) {
	diags := check(t, "# Title\n\n#\tHeading\n")
	require.Len(t, diags, 1)
	assert.Equal(t, "missing space after # in heading", diags[0].Message)
}

// --- Check: CRLF line endings don't produce false positives ---

func TestCheck_NoCRLFFalsePositive(t *testing.T) {
	// An empty ATX heading with CRLF ("##\r\n") must not emit "missing space";
	// the \r is trailing whitespace, not missing content.
	diags := check(t, "# Title\r\n\r\n##\r\n")
	assert.Empty(t, diags)
}

// --- Check: MD021 closed ATX multiple spaces ---

func TestCheck_ClosedATX_MultipleSpaces(t *testing.T) {
	diags := check(t, "# Title\n\n# Heading  #\n")
	require.Len(t, diags, 1)
	assert.Equal(t, "multiple spaces before closing # in heading", diags[0].Message)
	assert.Equal(t, 3, diags[0].Line)
	assert.Equal(t, 10, diags[0].Column)
}

// --- Check: code block and PI body skipped ---

func TestCheck_SkipsCodeBlock(t *testing.T) {
	src := "# Title\n\n```\n#Heading\n```\n"
	diags := check(t, src)
	assert.Empty(t, diags)
}

func TestCheck_SkipsIndentedCode(t *testing.T) {
	src := "# Title\n\nParagraph.\n\n    #Heading\n"
	diags := check(t, src)
	assert.Empty(t, diags)
}

func TestCheck_SkipsPIBlock(t *testing.T) {
	src := "# Title\n\n<?foo\n#Heading\n?>\n"
	diags := check(t, src)
	assert.Empty(t, diags)
}

func TestFix_PreservesPIBlock(t *testing.T) {
	src := "# Title\n\n<?foo\n#Heading\n?>\n"
	assert.Equal(t, src, fix(t, src))
}

// --- Check: 7+ hashes not flagged ---

func TestCheck_TooManyHashes(t *testing.T) {
	diags := check(t, "# Title\n\n####### not a heading\n")
	assert.Empty(t, diags)
}

// --- Fix ---

func TestFix_MissingSpace(t *testing.T) {
	assert.Equal(t, "# Title\n\n# Heading\n", fix(t, "# Title\n\n#Heading\n"))
}

func TestFix_MultipleSpaces(t *testing.T) {
	assert.Equal(t, "# Title\n\n# Heading\n", fix(t, "# Title\n\n#  Heading\n"))
}

func TestFix_Indented(t *testing.T) {
	assert.Equal(t, "# Title\n\n# Heading\n", fix(t, "# Title\n\n   # Heading\n"))
}

func TestFix_ClosedATX_NoSpace(t *testing.T) {
	// Trailing # without preceding space is content; fix only adds the opening space.
	assert.Equal(t, "# Title\n\n# Heading#\n", fix(t, "# Title\n\n#Heading#\n"))
}

func TestFix_TabAfterHash(t *testing.T) {
	assert.Equal(t, "# Title\n\n# Heading\n", fix(t, "# Title\n\n#\tHeading\n"))
}

func TestFix_ClosedATX_MultipleSpaces(t *testing.T) {
	assert.Equal(t, "# Title\n\n# Heading\n", fix(t, "# Title\n\n# Heading  #\n"))
}

func TestFix_ClosedATX_SingleSpace(t *testing.T) {
	assert.Equal(t, "# Title\n\n# Heading\n", fix(t, "# Title\n\n# Heading #\n"))
}

func TestFix_Stable(t *testing.T) {
	src := "# Title\n\n## Section\n"
	assert.Equal(t, src, fix(t, src))
}

func TestFix_PreservesCodeBlock(t *testing.T) {
	src := "# Title\n\n```\n#Heading\n```\n"
	assert.Equal(t, src, fix(t, src))
}

func TestFix_EmptyHeading(t *testing.T) {
	// "# ###" has closing hashes; fix to "#"
	assert.Equal(t, "# Title\n\n#\n", fix(t, "# Title\n\n# ###\n"))
}

func TestFix_ClosingHashesAllHashes(t *testing.T) {
	// "## ##" has all-hash content that is the closing suffix: fix to "##"
	assert.Equal(t, "# Title\n\n##\n", fix(t, "# Title\n\n## ##\n"))
}

func TestFix_PreservesCRLFOnRewrittenLines(t *testing.T) {
	// On a CRLF file, a rewritten line must keep its \r so the result is uniformly
	// CRLF rather than mixed LF/CRLF.
	src := "# Title\r\n\r\n#Heading\r\n"
	got := fix(t, src)
	assert.Equal(t, "# Title\r\n\r\n# Heading\r\n", got)
}

// --- ID/Name/Category ---

func TestID(t *testing.T) {
	assert.Equal(t, "MDS064", (&Rule{}).ID())
}

func TestName(t *testing.T) {
	assert.Equal(t, "atx-heading-whitespace", (&Rule{}).Name())
}

func TestCategory(t *testing.T) {
	assert.Equal(t, "heading", (&Rule{}).Category())
}
