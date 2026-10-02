package abi

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPackUnpackRoundTrip(t *testing.T) {
	for _, tc := range []struct{ ptr, n uint32 }{
		{0x1000, 42},
		// An address at or above 2 GiB sets bit 63: it must not be
		// mistaken for the truncation sentinel or any error.
		{0x8000_0000, 42},
		{0xFFFF_FFF0, 4096},
	} {
		ptr, n, err := Unpack(Pack(tc.ptr, tc.n))
		require.NoError(t, err, "%#x", tc.ptr)
		assert.Equal(t, tc.ptr, ptr)
		assert.Equal(t, tc.n, n)
	}
}

func TestUnpackSentinels(t *testing.T) {
	_, _, err := Unpack(Truncated)
	require.ErrorIs(t, err, ErrTruncated)
	_, _, err = Unpack(Rejected)
	require.ErrorIs(t, err, ErrRejected)
}

func TestLookupInput(t *testing.T) {
	buf := []byte("hello")
	high := []byte("hi")
	keep := map[uintptr][]byte{0x1000: buf, 0x8000_0000: high}

	got, ok := LookupInput(keep, 0x1000, 5)
	require.True(t, ok)
	assert.Equal(t, "hello", string(got))

	got, ok = LookupInput(keep, 0x1000, 0)
	require.True(t, ok, "a zero length needs no buffer")
	assert.Empty(t, got)
	got, ok = LookupInput(keep, 0, 0)
	require.True(t, ok)
	assert.Empty(t, got)

	// int32(0x8000_0000) is negative; it must not sign-extend.
	hp := int32(-0x8000_0000)
	got, ok = LookupInput(keep, hp, 2)
	require.True(t, ok)
	assert.Equal(t, "hi", string(got))

	for name, args := range map[string][2]int32{
		"negative length": {0x1000, -1},
		"unknown ptr":     {0x2000, 1},
		"null ptr":        {0, 1},
		"past the end":    {0x1000, 6},
	} {
		_, ok := LookupInput(keep, args[0], args[1])
		assert.False(t, ok, name)
	}
}
