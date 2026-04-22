//go:build wasip1

// Package main is the WASM guest for the wasm-embedded-inference spike.
//
// It embeds the same linear classifier package the go-native spike uses so
// the wasm-vs-native comparison only exercises the wasm runtime pathway,
// not a different model. The guest exposes three wasmexport functions:
//
//   - alloc(size) ptr    : reserve size bytes of guest memory for host input
//   - free(ptr)          : release a prior alloc
//   - classify(ptr, len) : classify text at [ptr, ptr+len); returns an
//                          int64 packing (outPtr<<32)|outLen of a JSON
//                          result written into a static guest buffer.
package main

import (
	"fmt"
	"strings"
	"unsafe"

	"github.com/jeduden/mdsmith/internal/rules/concisenessscoring/classifier"
)

var (
	model     *classifier.Model
	keepAlive = map[uintptr][]byte{}
	outputBuf [4096]byte
)

func init() {
	m, err := classifier.LoadEmbedded()
	if err != nil {
		panic(fmt.Sprintf("wasm guest: load classifier: %v", err))
	}
	model = m
}

//go:wasmexport alloc
func alloc(size int32) int32 {
	if size <= 0 {
		return 0
	}
	buf := make([]byte, size)
	ptr := uintptr(unsafe.Pointer(&buf[0]))
	keepAlive[ptr] = buf
	return int32(ptr)
}

//go:wasmexport free
func free(ptr int32) {
	delete(keepAlive, uintptr(ptr))
}

//go:wasmexport classify
func classify(ptr, length int32) int64 {
	if length < 0 {
		return 0
	}
	if length > 0 && ptr == 0 {
		return 0
	}
	text := ""
	if length > 0 {
		data := unsafe.Slice((*byte)(unsafe.Pointer(uintptr(ptr))), length)
		text = string(data)
	}
	result := model.Classify(text)

	var b strings.Builder
	fmt.Fprintf(
		&b,
		`{"label":%q,"risk_score":%.6f,"threshold":%.4f,"model_id":%q,"version":%q,"cues":%q}`,
		result.Label,
		result.RiskScore,
		result.Threshold,
		result.ModelID,
		result.Version,
		strings.Join(result.TriggeredCues, ","),
	)
	encoded := b.String()
	if len(encoded) > len(outputBuf) {
		// Signal truncation with a negative length so the host can detect
		// and raise rather than silently decoding a truncated JSON buffer.
		return -1
	}
	n := copy(outputBuf[:], encoded)
	out := uintptr(unsafe.Pointer(&outputBuf[0]))
	return (int64(out) << 32) | int64(n)
}

func main() {}
