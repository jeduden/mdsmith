package foreignregion

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/config"
	"github.com/stretchr/testify/assert"
)

// TestRestorePreservesInteriorBytes puts the original in-region bytes
// back over a fixer's edit while keeping the out-of-region edit.
func TestRestorePreservesInteriorBytes(t *testing.T) {
	original := "# T\n\nout   \n" + apm.Start + "\nin   \n" + apm.End + "\n"
	// Simulate a fixer that trimmed trailing spaces everywhere.
	fixed := "# T\n\nout\n" + apm.Start + "\nin\n" + apm.End + "\n"
	got := Restore([]byte(original), []byte(fixed), []config.ForeignRegion{apm})
	want := "# T\n\nout\n" + apm.Start + "\nin   \n" + apm.End + "\n"
	assert.Equal(t, want, string(got))
}

// TestRestoreNoRegions returns the fixed bytes untouched.
func TestRestoreNoRegions(t *testing.T) {
	fixed := "# T\n\nbody\n"
	got := Restore([]byte("# T\n\nbody   \n"), []byte(fixed), []config.ForeignRegion{apm})
	assert.Equal(t, fixed, string(got))
}

// TestRestoreCountMismatchLeavesFixed leaves the fixed bytes alone when
// the region counts differ between the two buffers (ambiguous pairing).
func TestRestoreCountMismatchLeavesFixed(t *testing.T) {
	original := apm.Start + "\nin\n" + apm.End + "\n"
	fixed := "no markers at all\n"
	got := Restore([]byte(original), []byte(fixed), []config.ForeignRegion{apm})
	assert.Equal(t, fixed, string(got))
}

// TestRestoreMultipleRegions restores each matched pair independently.
func TestRestoreMultipleRegions(t *testing.T) {
	s, e := apm.Start, apm.End
	original := s + "\nA   \n" + e + "\nmid\n" + s + "\nB   \n" + e + "\n"
	fixed := s + "\nA\n" + e + "\nmid\n" + s + "\nB\n" + e + "\n"
	got := Restore([]byte(original), []byte(fixed), []config.ForeignRegion{apm})
	assert.Equal(t, original, string(got))
}
