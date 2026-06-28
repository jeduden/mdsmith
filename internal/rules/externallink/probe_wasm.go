//go:build js && wasm

package externallink

// httpProber is a placeholder on the wasm build, which performs no
// network I/O. Keeping the field type generic lets the shared Rule
// struct compile without importing net/http into the WebAssembly
// artifact (a ~6 MB cost) for a rule a browser sandbox cannot run.
type httpProber = any

// init builds the rate-limit semaphore. There is no HTTP client on
// wasm; probe never makes a request.
func (r *Rule) init() {
	r.semaphore = make(chan struct{}, r.links.RateLimit)
}

// probe is a no-op on wasm: external link checking needs outbound HTTP,
// which the browser sandbox forbids (CORS, no raw sockets). It reports
// a healthy result so the WebAssembly engine emits no MDS071
// diagnostics rather than failing every URL.
func (r *Rule) probe(_ string) urlResult {
	return urlResult{statusCode: 200}
}
