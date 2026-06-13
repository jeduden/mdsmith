package bytelimit

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// errReader returns its error on every Read, exercising the non-EOF
// error path.
type errReader struct{ err error }

func (e errReader) Read([]byte) (int, error) { return 0, e.err }

func TestReadAllInto(t *testing.T) {
	tests := []struct {
		name          string
		data          string
		sizeHint, max int64
	}{
		{"in-cap exact hint", "hello world", 11, 100},
		{"grew past hint", strings.Repeat("x", 600), 100, 1000},       // seed 101 then grow
		{"unknown size fallback", strings.Repeat("y", 600), -1, 1000}, // seed 512 then grow
		{"hint over max falls back", strings.Repeat("z", 300), 5000, 1000},
		{"empty", "", 0, 100},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf []byte
			got, err := readAllInto(strings.NewReader(tc.data), &buf, tc.max, tc.sizeHint)
			require.NoError(t, err)
			assert.Equal(t, tc.data, string(got))
		})
	}
}

func TestReadAllInto_PropagatesReadError(t *testing.T) {
	boom := errors.New("boom")
	var buf []byte
	_, err := readAllInto(errReader{err: boom}, &buf, 100, 10)
	require.ErrorIs(t, err, boom)
}

func TestReadLimited_ReadErrorPropagates(t *testing.T) {
	boom := errors.New("boom")
	_, err := readLimited(errReader{err: boom}, "x.md", 100)
	require.ErrorIs(t, err, boom)
}

func TestReadLimitedInto_ReadErrorPropagates(t *testing.T) {
	boom := errors.New("boom")
	var buf []byte
	_, err := readLimitedInto(errReader{err: boom}, &buf, 100)
	require.ErrorIs(t, err, boom)
}

func TestReadLimited_TooLargeWithoutStat(t *testing.T) {
	// A reader with no Stat() leaves the size unknown, so the too-large
	// error reports the read length (max+1) rather than a stat size.
	_, err := readLimited(strings.NewReader(strings.Repeat("a", 200)), "x.md", 50)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "file too large")
	assert.Contains(t, err.Error(), "51 bytes")
	assert.Contains(t, err.Error(), "max 50")
}
