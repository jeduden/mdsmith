//go:build !(js && wasm)

package externallink

import "net/http"

// httpProber is the platform probe interface. On non-wasm builds it is
// satisfied by *http.Client.
type httpProber = *http.Client

// init lazily builds the rate-limit semaphore and HTTP client from the
// configured settings. Called once via initOnce on the first Check.
func (r *Rule) init() {
	r.semaphore = make(chan struct{}, r.links.RateLimit)
	r.http = &http.Client{Timeout: r.links.Timeout}
}

// probe issues the HEAD (then GET on 405) request and maps the outcome
// to a urlResult.
func (r *Rule) probe(raw string) urlResult {
	resp, err := r.do(http.MethodHead, raw)
	if err != nil {
		return urlResult{err: err}
	}
	if resp.StatusCode == http.StatusMethodNotAllowed {
		resp, err = r.do(http.MethodGet, raw)
		if err != nil {
			return urlResult{err: err}
		}
	}
	return urlResult{statusCode: resp.StatusCode}
}

// do performs one request with the given method and closes the body so
// the connection can be reused.
func (r *Rule) do(method, raw string) (*http.Response, error) {
	req, err := http.NewRequest(method, raw, nil)
	if err != nil {
		return nil, err
	}
	resp, err := r.http.Do(req)
	if err != nil {
		return nil, err
	}
	// The status code is all we need; close the body immediately.
	_ = resp.Body.Close()
	return resp, nil
}
