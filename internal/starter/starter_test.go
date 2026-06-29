package starter_test

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/starter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGet_OKF(t *testing.T) {
	data, ok := starter.Get("okf")
	require.True(t, ok)
	assert.Contains(t, string(data), "required-frontmatter")
	assert.Contains(t, string(data), "site-root")
}

func TestGet_Unknown(t *testing.T) {
	_, ok := starter.Get("bogus")
	assert.False(t, ok)
}

func TestNames(t *testing.T) {
	assert.Contains(t, starter.Names(), "okf")
}

func TestErrUnknown_ListsNames(t *testing.T) {
	err := starter.ErrUnknown("bogus")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bogus")
	assert.Contains(t, err.Error(), "okf")
}

// TestEveryStarterIsValidConfig guards the contract every starter must
// keep: the embedded template must parse as a valid .mdsmith.yml, so a
// typo in a starter fails CI instead of a user's first `mdsmith check`.
func TestEveryStarterIsValidConfig(t *testing.T) {
	for _, name := range starter.Names() {
		t.Run(name, func(t *testing.T) {
			data, ok := starter.Get(name)
			require.True(t, ok)
			_, err := config.ParseBytes(data)
			require.NoError(t, err, "starter %q must be a loadable config", name)
		})
	}
}
