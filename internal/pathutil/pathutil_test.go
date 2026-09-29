package pathutil

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsAbsOrDriveOrUNC(t *testing.T) {
	assert.True(t, IsAbsOrDriveOrUNC("/etc/passwd"))
	assert.True(t, IsAbsOrDriveOrUNC("C:/Windows"))
	assert.True(t, IsAbsOrDriveOrUNC("//server/share"))
	assert.False(t, IsAbsOrDriveOrUNC("docs/api.md"))
	assert.False(t, IsAbsOrDriveOrUNC(""))
}
