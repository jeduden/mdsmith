package mdsmith

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/refactor"
	"github.com/stretchr/testify/assert"
)

// The engine's no-op outcome matches the public sentinel as is, so
// Session.Rename can return it without a wrapper.
func TestErrNothingToRename(t *testing.T) {
	inner := refactor.NothingToRenameError{Kind: refactor.KindHeading, Name: "Setup"}
	assert.ErrorIs(t, inner, ErrNothingToRename)
	assert.Equal(t, "nothing to rename", ErrNothingToRename.Error())
	assert.Equal(t, ErrorCodeNothingToRename, ErrorCode(inner))
}
