package main

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
		"header: |\n  | Title |\n  |-------|\nrow: \"| [{{.title}}]({{.filename}}) |\"\n?>\n" +
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

	result := string(stripSectionConflicts([]byte(input)))

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

	result := string(stripSectionConflicts([]byte(input)))

	assert.Contains(t, result, "<<<<<<<", "expected <<<<<<< marker preserved outside section")
	assert.Contains(t, result, "|||||||", "expected ||||||| marker preserved outside section")
	assert.Contains(t, result, "=======", "expected ======= separator preserved outside section")
	assert.Contains(t, result, ">>>>>>>", "expected >>>>>>> marker preserved outside section")
}
