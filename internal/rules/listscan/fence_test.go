package listscan

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jeduden/mdsmith/internal/mdfence"
)

// TestFence_RelativeIndentBudget pins the container-relative semantics
// the scanner relies on: a fence four columns past the item's content
// column is indented code, three columns past still opens and closes,
// and a tab reaches the next multiple of four.
func TestFence_RelativeIndentBudget(t *testing.T) {
	_, ok := mdfence.OpenIn([]byte("      ```"), 0, 2)
	assert.False(t, ok, "4 columns past baseCol is indented code")
	f, ok := mdfence.OpenIn([]byte("     ```"), 0, 2)
	assert.True(t, ok, "3 columns past baseCol opens")
	assert.False(t, mdfence.CloseIn([]byte("      ```"), f, 0, 2),
		"4 columns past baseCol does not close")
	assert.True(t, mdfence.CloseIn([]byte("  ```"), f, 0, 2))
	assert.True(t, mdfence.CloseIn([]byte("\t```"), f, 0, 2),
		"a tab reaches column 4, 2 past baseCol")
	assert.True(t, mdfence.CloseIn([]byte("```"), f, 0, 2),
		"a line left of baseCol counts as indent 0")
}
