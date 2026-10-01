package listscan

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jeduden/mdsmith/internal/mdfence"
)

func TestFenceView(t *testing.T) {
	tests := []struct {
		name    string
		line    string
		indent  int
		baseCol int
		want    string
	}{
		{"top level keeps the line", "  ```", 2, 0, "  ```"},
		{"sliced at the content column", "    ```go", 4, 2, "  ```go"},
		{"exactly at the content column", "  ```", 2, 2, "```"},
		{"lazy line below the content column", " ```", 1, 4, "```"},
		{"unindented line inside an item", "```", 0, 2, "```"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, string(fenceView([]byte(tt.line), tt.indent, tt.baseCol)))
		})
	}
}

// TestFenceView_RelativeIndentBudget pins the container-relative
// semantics callers rely on: a fence four columns past the item's
// content column is indented code, three columns past still opens and
// closes.
func TestFenceView_RelativeIndentBudget(t *testing.T) {
	_, ok := mdfence.Open(fenceView([]byte("      ```"), 6, 2))
	assert.False(t, ok, "4 columns past baseCol is indented code")
	f, ok := mdfence.Open(fenceView([]byte("     ```"), 5, 2))
	assert.True(t, ok, "3 columns past baseCol opens")
	assert.False(t, mdfence.Close(fenceView([]byte("      ```"), 6, 2), f),
		"4 columns past baseCol does not close")
	assert.True(t, mdfence.Close(fenceView([]byte("  ```"), 2, 2), f))
}
