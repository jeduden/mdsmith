//go:build !(js && wasm)

package externallink

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDo_NewRequestError exercises the http.NewRequestWithContext error
// branch in do(). An invalid method (space is not an HTTP token
// character) fails before any network I/O.
func TestDo_NewRequestError(t *testing.T) {
	res := do("BAD METHOD", "http://localhost/", time.Second)
	require.Error(t, res.err)
}

// TestProbe_HeadOK covers the happy HEAD path through probe().
func TestProbe_HeadOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	res := probe(srv.URL, time.Second)
	require.NoError(t, res.err)
	assert.Equal(t, http.StatusOK, res.statusCode)
}

// TestProbe_HeadErrorSkipsGet confirms a transport error on the HEAD
// request returns immediately without a GET fallback.
func TestProbe_HeadErrorSkipsGet(t *testing.T) {
	res := probe("http://192.0.2.1:9/down", 100*time.Millisecond)
	require.Error(t, res.err)
	assert.Equal(t, 0, res.statusCode)
}

// TestProbe_405FallbackGetError covers the branch where HEAD returns 405
// and the GET fallback then fails at the transport layer (connection
// hijacked and closed).
func TestProbe_405FallbackGetError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch req.Method {
		case http.MethodHead:
			w.WriteHeader(http.StatusMethodNotAllowed)
		case http.MethodGet:
			hj, ok := w.(http.Hijacker)
			if !ok {
				http.Error(w, "no hijack", http.StatusInternalServerError)
				return
			}
			conn, _, _ := hj.Hijack()
			_ = conn.Close()
		}
	}))
	defer srv.Close()

	res := probe(srv.URL, time.Second)
	require.Error(t, res.err)
}
