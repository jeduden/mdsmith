//go:build js && wasm

package externallink

import "time"

// probe cannot reach the network on wasm: the browser sandbox forbids
// the outbound HTTP it needs (CORS, no raw sockets). It returns a
// not-probed result (probed=false), which yields NO diagnostic — the URL
// is reported as neither broken nor healthy. Returning a fake 200 here
// would silently pass every external link, granting false confidence;
// returning a failure would flag every link. Neither is honest, so the
// rule stays inert until a host supplies probed results through the
// bridge (see plan 2607170527). Keeping the probe in a build-tagged file
// is what keeps net/http out of the WebAssembly artifact.
func probe(_ string, _ time.Duration) urlResult {
	return urlResult{probed: false}
}
