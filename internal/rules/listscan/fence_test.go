package listscan

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpeningFenceRel_BacktickInInfoString(t *testing.T) {
	line := []byte("```go`extra")
	_, ok := openingFenceRel(line, 0, 0)
	assert.False(t, ok, "backtick fence with backtick in info string must not be a valid opener")
}

func TestOpeningFenceRel_CleanInfoString(t *testing.T) {
	line := []byte("```go")
	_, ok := openingFenceRel(line, 0, 0)
	require.True(t, ok, "valid backtick fence must be recognized")
}

func TestOpeningFenceRel_TildeAllowsBacktickInInfo(t *testing.T) {
	line := []byte("~~~go`extra")
	_, ok := openingFenceRel(line, 0, 0)
	require.True(t, ok, "tilde fence allows backtick in info string")
}
