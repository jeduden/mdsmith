//go:build js && wasm

package externallink

import "time"

// probe is a no-op on wasm: external link checking needs outbound HTTP,
// which the browser sandbox forbids (CORS, no raw sockets). It reports a
// healthy result so the WebAssembly engine emits no MDS072 diagnostics
// rather than failing every URL. Keeping the probe in a build-tagged
// file is what keeps net/http out of the WebAssembly artifact; the
// shared Check/checkURL code carries no net/http reference.
func probe(_ string, _ time.Duration) urlResult {
	return urlResult{statusCode: 200}
}
