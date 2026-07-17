package astutil

import (
	"reflect"
	"testing"

	"github.com/jeduden/mdsmith/internal/structlayout"
)

// TestSectionParagraph_PointerFieldsPrecedeScalars guards the GC
// ptrdata region of SectionParagraph: Node and Text (its only
// pointer-bearing fields) must precede the scalar Line/HasText
// fields, per docs/development/high-performance-go.md#struct-layout.
// buildSectionParagraphs allocates one SectionParagraph per document
// paragraph, backing MDS023 (paragraph-readability) and MDS024
// (paragraph-structure) — both default-enabled — so this is likely
// the highest-frequency struct construction in the rule set.
func TestSectionParagraph_PointerFieldsPrecedeScalars(t *testing.T) {
	structlayout.AssertPointerFieldsFirst(t, reflect.TypeOf(SectionParagraph{}))
}
