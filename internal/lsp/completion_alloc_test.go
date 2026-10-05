package lsp

import (
	"fmt"
	"testing"

	"github.com/jeduden/mdsmith/internal/index"
	"github.com/stretchr/testify/assert"
)

// TestDirectivePathItemsAllocBudget guards the "pre-size slices" rule in
// docs/development/high-performance-go.md: every workspace file matching
// the typed prefix becomes one item, so the result slice must be sized
// once instead of regrown per keystroke across a large workspace.
func TestDirectivePathItemsAllocBudget(t *testing.T) {
	if raceEnabled {
		t.Skip("allocation counts are not stable under -race")
	}
	idx := index.New("/r")
	for i := 0; i < 2000; i++ {
		idx.Update(fmt.Sprintf("doc%04d.md", i), []byte("# T\n"))
	}
	s := &Server{}
	got := testing.AllocsPerRun(20, func() {
		items := s.directivePathItems("a.md", "", idx)
		if len(items) != 2000 {
			t.Fatalf("items = %d", len(items))
		}
	})
	assert.LessOrEqual(t, got, 3.0, "items slice regrown per append")
}
