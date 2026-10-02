// Package abi is the calling convention between the
// wasm-embedded-inference spike's wasip1 guest (../wasm) and its wazero
// host (../main.go). It holds no build tags, so the packing and the
// input checks both sides rely on run under a native `go test ./...`
// even though neither the guest nor the host builds there.
//
// classify returns one int64: Pack(outPtr, outLen) for a result, or a
// sentinel. Rejected (0) never collides with a result, because the
// JSON result is never empty. Truncated (-1) never collides either,
// because outLen is at most the guest's 4 KiB output buffer.
package abi

import "errors"

const (
	// Rejected is classify's return for an input it did not accept: a
	// negative length, or a ptr/length outside a live alloc.
	Rejected int64 = 0
	// Truncated is classify's return when the JSON result does not fit
	// the guest's output buffer.
	Truncated int64 = -1
)

var (
	// ErrRejected reports a Rejected return.
	ErrRejected = errors.New("guest rejected input pointer or length")
	// ErrTruncated reports a Truncated return.
	ErrTruncated = errors.New("guest signaled output truncation")
)

// Pack packs a result's guest address and length as (ptr<<32)|n. The
// shift is done on uint64, so an address at or above 2 GiB sets the
// sign bit; Unpack decodes it as a result, not as an error.
func Pack(ptr, n uint32) int64 {
	return int64(uint64(ptr)<<32 | uint64(n))
}

// Unpack decodes classify's return. It reports ErrRejected or
// ErrTruncated for the two sentinels and otherwise splits the value
// into the result's address and length.
func Unpack(packed int64) (ptr, n uint32, err error) {
	switch packed {
	case Rejected:
		return 0, 0, ErrRejected
	case Truncated:
		return 0, 0, ErrTruncated
	}
	u := uint64(packed)
	return uint32(u >> 32), uint32(u), nil
}

// LookupInput returns the first length bytes of the live alloc at ptr,
// resolved through keep (alloc's address-to-buffer map) rather than by
// converting the raw address back to a pointer: only alloc'd buffers
// are valid input, and the buffer's length bounds the read. ptr goes
// through uint32 first, so an address at or above 2 GiB, which arrives
// as a negative int32, is not sign-extended. A zero length needs no
// buffer and yields an empty slice; a negative length, an unknown ptr,
// or a length past the buffer's end reports false.
func LookupInput(keep map[uintptr][]byte, ptr, length int32) ([]byte, bool) {
	if length < 0 {
		return nil, false
	}
	if length == 0 {
		return nil, true
	}
	buf, ok := keep[uintptr(uint32(ptr))]
	if !ok || int(length) > len(buf) {
		return nil, false
	}
	return buf[:length], true
}
