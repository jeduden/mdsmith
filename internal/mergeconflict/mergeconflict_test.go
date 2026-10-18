package mergeconflict

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStripSectionConflicts_Diff3CatalogConflict(t *testing.T) {
	// diff3-style conflict markers include a ||||||| base section
	// between <<<<<<< and =======. The merge driver must strip all
	// four marker types inside regenerable sections.
	input := "# Doc\n\n" +
		"<?catalog\nglob: \"plans/*.md\"\nsort: title\n" +
		"header: |\n  | Title |\n  |-------|\nrow: \"| [{title}]({filename}) |\"\n?>\n" +
		"<<<<<<< ours\n" +
		"| [Alpha](plans/alpha.md) |\n" +
		"| [Beta](plans/beta.md) |\n" +
		"||||||| base\n" +
		"| [Alpha](plans/alpha.md) |\n" +
		"=======\n" +
		"| [Alpha](plans/alpha.md) |\n" +
		"| [Gamma](plans/gamma.md) |\n" +
		">>>>>>> theirs\n" +
		"<?/catalog?>\n"

	result := string(StripSectionConflicts([]byte(input), []string{"catalog", "include"}))

	assert.NotContains(t, result, "<<<<<<<", "expected <<<<<<< marker stripped")
	assert.NotContains(t, result, "|||||||", "expected ||||||| base marker stripped")
	assert.NotContains(t, result, "=======", "expected ======= separator stripped")
	assert.NotContains(t, result, ">>>>>>>", "expected >>>>>>> marker stripped")
}

func TestStripSectionConflicts_Diff3OutsideSection_Preserved(t *testing.T) {
	// diff3 conflict markers outside regenerable sections must be
	// preserved so the user can resolve them manually.
	input := "# Doc\n\n" +
		"<<<<<<< ours\n" +
		"ours text\n" +
		"||||||| base\n" +
		"base text\n" +
		"=======\n" +
		"theirs text\n" +
		">>>>>>> theirs\n"

	result := string(StripSectionConflicts([]byte(input), []string{"catalog", "include"}))

	assert.Contains(t, result, "<<<<<<<", "expected <<<<<<< marker preserved outside section")
	assert.Contains(t, result, "|||||||", "expected ||||||| marker preserved outside section")
	assert.Contains(t, result, "=======", "expected ======= separator preserved outside section")
	assert.Contains(t, result, ">>>>>>>", "expected >>>>>>> marker preserved outside section")
}

// --- isOpen ---

func TestIsOpen_True(t *testing.T) {
	assert.True(t, isOpen([]byte("<<<<<<< HEAD")))
	assert.True(t, isOpen([]byte("<<<<<<<extra")))
}

func TestIsOpen_False(t *testing.T) {
	assert.False(t, isOpen([]byte("normal text")))
	assert.False(t, isOpen([]byte("<<<<<< only six")))
}

// --- isBase ---

func TestIsBase_True(t *testing.T) {
	assert.True(t, isBase([]byte("||||||| base")))
	assert.True(t, isBase([]byte("|||||||")))
}

func TestIsBase_False(t *testing.T) {
	assert.False(t, isBase([]byte("normal")))
	assert.False(t, isBase([]byte("<<<<<<< HEAD")))
}

// --- isSeparator ---

func TestIsSeparator_True(t *testing.T) {
	assert.True(t, isSeparator([]byte("=======")))
	assert.True(t, isSeparator([]byte("======= extra")))
}

func TestIsSeparator_False(t *testing.T) {
	assert.False(t, isSeparator([]byte("<<<<<<< HEAD")))
	assert.False(t, isSeparator([]byte("======")))
}

// --- isClose ---

func TestIsClose_True(t *testing.T) {
	assert.True(t, isClose([]byte(">>>>>>> theirs")))
	assert.True(t, isClose([]byte(">>>>>>>")))
}

func TestIsClose_False(t *testing.T) {
	assert.False(t, isClose([]byte("normal")))
	assert.False(t, isClose([]byte("<<<<<<< HEAD")))
}

// --- HasMarkers ---

func TestHasMarkers_WithOpenClose(t *testing.T) {
	content := []byte("line one\n<<<<<<< HEAD\nours\n=======\ntheirs\n>>>>>>> theirs\n")
	assert.True(t, HasMarkers(content))
}

func TestHasMarkers_OpenOnly(t *testing.T) {
	content := []byte("<<<<<<< HEAD\nours\n")
	assert.True(t, HasMarkers(content))
}

func TestHasMarkers_CloseOnly(t *testing.T) {
	content := []byte("some text\n>>>>>>> theirs\n")
	assert.True(t, HasMarkers(content))
}

func TestHasMarkers_None(t *testing.T) {
	content := []byte("# Clean file\n\nSome content.\n")
	assert.False(t, HasMarkers(content))
}

func TestHasMarkers_SetextHeading_NotConflict(t *testing.T) {
	// "=======" on its own is a setext heading underline, not a conflict
	content := []byte("Heading\n=======\n\nContent.\n")
	assert.False(t, HasMarkers(content))
}

func TestHasMarkers_Empty(t *testing.T) {
	assert.False(t, HasMarkers(nil))
}

// --- matchesAnyStart / matchesAnyEnd ---

func TestMatchesAnyStart_Match(t *testing.T) {
	names := []string{"catalog", "include"}
	assert.True(t, matchesAnyStart([]byte("<?catalog glob: \"**/*.md\" ?>"), names))
	assert.True(t, matchesAnyStart([]byte("<?include file: foo.md ?>"), names))
}

func TestMatchesAnyStart_NoMatch(t *testing.T) {
	names := []string{"catalog", "include"}
	assert.False(t, matchesAnyStart([]byte("regular line"), names))
	assert.False(t, matchesAnyStart([]byte("<?/catalog?>"), names))
}

func TestMatchesAnyEnd_Match(t *testing.T) {
	names := []string{"catalog", "include"}
	assert.True(t, matchesAnyEnd([]byte("<?/catalog?>"), names))
	assert.True(t, matchesAnyEnd([]byte("<?/include?>"), names))
}

func TestMatchesAnyEnd_NoMatch(t *testing.T) {
	names := []string{"catalog", "include"}
	assert.False(t, matchesAnyEnd([]byte("regular line"), names))
	assert.False(t, matchesAnyEnd([]byte("<?catalog glob: \"*\" ?>"), names))
}
