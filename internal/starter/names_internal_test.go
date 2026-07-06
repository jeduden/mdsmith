package starter

import (
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
)

// TestNamesFrom_ReadDirError drives the defensive branch the embedded
// FS can never reach: an FS with no templates/ directory yields no
// names rather than panicking.
func TestNamesFrom_ReadDirError(t *testing.T) {
	assert.Nil(t, namesFrom(fstest.MapFS{}))
}
