//go:build !(js && wasm)

package externallink

import (
	"context"
	"io"
	"net/http"
	"time"
)

// client is the shared probing client. One client (not one per request)
// lets the transport pool keep-alive connections across URLs on the same
// host. Redirects are followed by default, so a URL that 30x-es to a
// healthy target passes. The per-request timeout is applied through a
// context, not client.Timeout, so a single shared client serves every
// configured timeout.
var client = &http.Client{}

// maxDrain caps how many bytes of a GET-fallback body are read back
// before Close. Draining lets the connection return to the keep-alive
// pool; capping it avoids reading a large page in full when all we need
// is the status code.
const maxDrain = 64 << 10

// probe issues a HEAD request (falling back to GET on 405) and maps the
// outcome to a urlResult. timeout bounds each individual request. The
// native prober always reaches the network, so every result it returns
// is probed=true.
func probe(raw string, timeout time.Duration) urlResult {
	res := do(http.MethodHead, raw, timeout)
	if res.err == nil && res.statusCode == http.StatusMethodNotAllowed {
		res = do(http.MethodGet, raw, timeout)
	}
	return res
}

// do performs one request with the given method and a context timeout,
// draining a bounded prefix of the body and closing it so the connection
// can be reused.
func do(method, raw string, timeout time.Duration) urlResult {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, raw, nil)
	if err != nil {
		return urlResult{probed: true, err: err}
	}
	resp, err := client.Do(req)
	if err != nil {
		return urlResult{probed: true, err: err}
	}
	// Drain a bounded prefix of the body before closing so the transport
	// can reuse the connection; the status code is all we consume.
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxDrain))
	_ = resp.Body.Close()
	return urlResult{probed: true, statusCode: resp.StatusCode}
}
