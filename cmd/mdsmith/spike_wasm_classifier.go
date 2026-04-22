//go:build spike_wasm_classifier

package main

import "github.com/jeduden/mdsmith/internal/rules/concisenessscoring/wasmclassifier"

func init() {
	// Force-link the embedded wasm artifact so binary-size deltas can be
	// measured for the wasm-embedded-inference spike.
	_ = wasmclassifier.WasmArtifactBytes()
}
