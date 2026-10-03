package mdsmith

import (
	"errors"
	"testing"

	"github.com/jeduden/mdsmith/internal/refactor"
	"github.com/stretchr/testify/assert"
)

func TestPublicNothingToRename(t *testing.T) {
	inner := refactor.NothingToRenameError{Kind: refactor.KindHeading, Name: "Setup"}
	err := publicNothingToRename(inner)
	assert.Equal(t, `nothing to rename for heading "Setup"`, err.Error(), "text is the engine's")
	assert.ErrorIs(t, err, ErrNothingToRename)
	assert.ErrorIs(t, err, refactor.ErrNothingToRename, "unwraps to the engine error")
	var got refactor.NothingToRenameError
	assert.ErrorAs(t, err, &got)
	assert.Equal(t, inner, got)

	other := errors.New("boom")
	assert.Same(t, other, publicNothingToRename(other), "other errors pass through")
	assert.NoError(t, publicNothingToRename(nil))
}
