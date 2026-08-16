//go:build !(js && wasm)

package externallink

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDoWithClient_NewRequestError exercises the http.NewRequestWithContext
// error branch in doWithClient(). An invalid method (space is not an HTTP
// token character) fails before any network I/O.
func TestDoWithClient_NewRequestError(t *testing.T) {
	res := doWithClient(permissiveClient, "BAD METHOD", "http://localhost/", time.Second)
	assert.Error(t, res.err)
}

// TestProbe_HeadOK covers the happy HEAD path through probe().
// allowInternal is true because httptest binds to 127.0.0.1.
func TestProbe_HeadOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	res := probe(srv.URL, time.Second, true)
	require.NoError(t, res.err)
	assert.Equal(t, http.StatusOK, res.statusCode)
}

// TestProbe_HeadErrorSkipsGet confirms a transport error on the HEAD
// request returns immediately without a GET fallback.
func TestProbe_HeadErrorSkipsGet(t *testing.T) {
	// TEST-NET-1 (192.0.2.0/24) does not route; allowInternal is irrelevant.
	res := probe("http://192.0.2.1:9/down", 100*time.Millisecond, false)
	assert.Error(t, res.err)
	assert.Equal(t, 0, res.statusCode)
}

// TestProbe_405FallbackGetError covers the branch where HEAD returns 405
// and the GET fallback then fails at the transport layer (connection
// hijacked and closed). allowInternal is true for the httptest server.
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

	res := probe(srv.URL, time.Second, true)
	assert.Error(t, res.err)
}

// TestIsRestrictedIP_Blocked verifies that all blocked IP classes return true.
func TestIsRestrictedIP_Blocked(t *testing.T) {
	blocked := []string{
		"127.0.0.1",        // loopback IPv4
		"127.255.255.254",  // loopback IPv4 high end
		"::1",              // loopback IPv6
		"10.0.0.1",         // private RFC1918
		"10.255.255.255",   // private RFC1918
		"172.16.0.1",       // private RFC1918
		"172.31.255.254",   // private RFC1918
		"192.168.0.1",      // private RFC1918
		"192.168.255.255",  // private RFC1918
		"169.254.1.1",      // link-local IPv4
		"169.254.169.254",  // cloud metadata (AWS/GCP/Azure)
		"fe80::1",          // link-local IPv6
		"fc00::1",          // ULA IPv6
		"fdff:ffff::1",     // ULA IPv6 high end
		"100.64.0.1",       // CGN (RFC6598)
		"100.127.255.254",  // CGN high end
		"100.100.100.200",  // Alibaba Cloud metadata (inside CGN range)
		"0.0.0.0",          // unspecified
		"::",               // unspecified IPv6
		"224.0.0.1",        // multicast
		"fe80::1%eth0",     // zone-scoped link-local (zone stripped before prefix check)
		"::a9fe:a9fe",      // IPv4-compatible 169.254.169.254 (cloud metadata)
		"::a00:1",          // IPv4-compatible 10.0.1 (private RFC1918)
		"::7f00:1",         // IPv4-compatible 127.0.0.1 (loopback)
		"2002:a9fe:a9fe::", // 6to4 encoding 169.254.169.254
		"2002:c0a8:101::",  // 6to4 encoding 192.168.1.1
	}
	for _, addr := range blocked {
		ip, err := netip.ParseAddr(addr)
		require.NoErrorf(t, err, "parsing %s", addr)
		assert.Truef(t, isRestrictedIP(ip), "expected %s to be blocked", addr)
	}
}

// TestIsRestrictedIP_Allowed verifies that public routable addresses pass.
func TestIsRestrictedIP_Allowed(t *testing.T) {
	allowed := []string{
		"1.1.1.1",              // Cloudflare DNS
		"8.8.8.8",              // Google DNS
		"93.184.216.34",        // example.com
		"2606:4700:4700::1111", // Cloudflare IPv6
	}
	for _, addr := range allowed {
		ip, err := netip.ParseAddr(addr)
		require.NoErrorf(t, err, "parsing %s", addr)
		assert.Falsef(t, isRestrictedIP(ip), "expected %s to be allowed", addr)
	}
}

// TestIsRestrictedIP_ZeroAddr verifies that the zero netip.Addr (invalid) is
// blocked — isRestrictedIP must fail closed, not open.
func TestIsRestrictedIP_ZeroAddr(t *testing.T) {
	assert.True(t, isRestrictedIP(netip.Addr{}), "zero (invalid) Addr must be blocked")
}

// TestSSRFControl_LoopbackDenied confirms ssrfControl returns an error for
// loopback addresses.
func TestSSRFControl_LoopbackDenied(t *testing.T) {
	err := ssrfControl("tcp", "127.0.0.1:80", syscall.RawConn(nil))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "denied")
}

// TestSSRFControl_ExternalAllowed confirms ssrfControl allows public IPs.
func TestSSRFControl_ExternalAllowed(t *testing.T) {
	err := ssrfControl("tcp", "1.1.1.1:443", syscall.RawConn(nil))
	assert.NoError(t, err)
}

// TestSSRFCheckRedirect_IPLiteralLoopback confirms ssrfCheckRedirect blocks
// a redirect whose target is a loopback IP literal.
func TestSSRFCheckRedirect_IPLiteralLoopback(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "http://127.0.0.1/target", nil)
	require.NoError(t, err)
	err = ssrfCheckRedirect(req, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "denied")
}

// TestSSRFCheckRedirect_HostnamePassesThrough confirms ssrfCheckRedirect
// returns nil for hostname redirect targets. Hostname-based SSRF checks
// happen at the TCP dial via ssrfControl; no DNS lookup here avoids the
// double-resolution TOCTOU window.
func TestSSRFCheckRedirect_HostnamePassesThrough(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "http://localhost/target", nil)
	require.NoError(t, err)
	err = ssrfCheckRedirect(req, nil)
	assert.NoError(t, err)
}

// TestSSRFCheckRedirect_PublicIPAllowed confirms ssrfCheckRedirect returns nil
// when the redirect target is a public (non-restricted) IP literal. The SSRF
// check at the HTTP layer only denies restricted IP literals; public ones are
// re-checked at the TCP dial by ssrfControl.
func TestSSRFCheckRedirect_PublicIPAllowed(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "http://1.1.1.1/target", nil)
	require.NoError(t, err)
	err = ssrfCheckRedirect(req, nil)
	assert.NoError(t, err)
}

// TestSSRFCheckRedirect_HopLimitExceeded confirms ssrfCheckRedirect stops
// after 10 redirect hops.
func TestSSRFCheckRedirect_HopLimitExceeded(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "http://example.com/final", nil)
	require.NoError(t, err)
	via := make([]*http.Request, 10)
	err = ssrfCheckRedirect(req, via)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "10 redirects")
}

// TestProbe_LoopbackBlocked confirms that probe() with allowInternal=false
// (the default) rejects connections to a loopback httptest server.
func TestProbe_LoopbackBlocked(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	res := probe(srv.URL, time.Second, false)
	assert.True(t, res.probed, "a blocked probe is still a probed result")
	assert.Error(t, res.err, "loopback must be denied by the SSRF guard")
}

// TestProbe_AllowInternalBypassesGuard confirms that allowInternal=true
// allows the connection to a loopback server.
func TestProbe_AllowInternalBypassesGuard(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	res := probe(srv.URL, time.Second, true)
	require.NoError(t, res.err)
	assert.Equal(t, http.StatusOK, res.statusCode)
}
