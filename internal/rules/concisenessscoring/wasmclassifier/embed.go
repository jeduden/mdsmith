//go:build spike_wasm_classifier

// Package wasmclassifier force-links the compiled wasm classifier artifact
// and the wazero runtime so the wasm-embedded-inference spike can measure
// binary-size impact of shipping the classifier in wasm form. It is only
// compiled under the spike_wasm_classifier build tag.
package wasmclassifier

import (
	_ "embed"

	_ "github.com/tetratelabs/wazero"
	_ "github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

//go:embed classifier.wasm
var wasmArtifact []byte

// ArtifactHolder keeps the embedded artifact reachable from the linker even
// when the call site only reads the length. It is exported so tests and
// size-measurement hooks can inspect the byte slice header and content.
var ArtifactHolder = wasmArtifact

// WasmArtifactBytes returns the embedded wasm artifact size in bytes.
func WasmArtifactBytes() int {
	return len(ArtifactHolder)
}
