//go:build !(js && wasm)

package externallink

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"syscall"
	"time"
)

// restrictedPrefixes are the IP ranges blocked when external-allow-internal is
// false (the default). ip.IsLoopback(), ip.IsUnspecified(), and
// ip.IsMulticast() cover loopback (127.0.0.0/8, ::1), unspecified (0.0.0.0,
// ::), and multicast; the prefixes below cover the remaining restricted ranges.
var restrictedPrefixes = []netip.Prefix{
	netip.MustParsePrefix("10.0.0.0/8"),     // private (RFC1918)
	netip.MustParsePrefix("172.16.0.0/12"),  // private (RFC1918)
	netip.MustParsePrefix("192.168.0.0/16"), // private (RFC1918)
	netip.MustParsePrefix("169.254.0.0/16"), // link-local IPv4 (cloud metadata: 169.254.169.254)
	netip.MustParsePrefix("fe80::/10"),      // link-local IPv6
	netip.MustParsePrefix("fc00::/7"),       // ULA IPv6
	netip.MustParsePrefix("100.64.0.0/10"),  // CGN (RFC6598; covers Alibaba metadata 100.100.100.200)
}

// isRestrictedIP reports whether ip is loopback, unspecified, multicast,
// or in a restricted prefix. The guard applies after IPv4-in-IPv6 unmapping.
func isRestrictedIP(ip netip.Addr) bool {
	ip = ip.Unmap()
	if ip.IsLoopback() || ip.IsUnspecified() || ip.IsMulticast() {
		return true
	}
	for _, prefix := range restrictedPrefixes {
		if prefix.Contains(ip) {
			return true
		}
	}
	return false
}

// ssrfControl is a net.Dialer.Control function that refuses connections
// whose resolved remote IP is in a restricted range. It fires on every
// new TCP dial — initial connections and redirect hops to a new host —
// so the guard reasserts containment on each hop.
func ssrfControl(_ string, address string, _ syscall.RawConn) error {
	// The dialer always passes a resolved "ip:port"; both calls are infallible.
	host, _, _ := net.SplitHostPort(address)
	ip, _ := netip.ParseAddr(host)
	if isRestrictedIP(ip) {
		return fmt.Errorf("external-link-check: connection to %s denied (SSRF guard;"+
			" set links.external-allow-internal: true to allow)", host)
	}
	return nil
}

// ssrfCheckRedirect caps the redirect chain and blocks redirects to restricted
// IP literals. Hostname-based redirect targets are checked at the TCP dial by
// ssrfControl, so no DNS round-trip is needed here.
func ssrfCheckRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return errors.New("external-link-check: stopped after 10 redirects")
	}
	host := req.URL.Hostname()
	ip, err := netip.ParseAddr(host)
	if err != nil {
		return nil
	}
	if isRestrictedIP(ip) {
		return fmt.Errorf("external-link-check: redirect to %s denied (SSRF guard)", host)
	}
	return nil
}

// guardedClient blocks connections and redirects to restricted IP ranges.
// Used when external-allow-internal is false (the default).
var guardedClient = buildGuardedClient()

// permissiveClient performs no SSRF filtering. Used only when
// external-allow-internal is true.
var permissiveClient = &http.Client{}

func buildGuardedClient() *http.Client {
	dialer := &net.Dialer{Control: ssrfControl}
	transport := &http.Transport{
		DialContext:     dialer.DialContext,
		Proxy:           http.ProxyFromEnvironment,
		IdleConnTimeout: 90 * time.Second,
	}
	return &http.Client{
		Transport:     transport,
		CheckRedirect: ssrfCheckRedirect,
	}
}

// maxDrain caps how many bytes of a GET-fallback body are read back
// before Close. Draining lets the connection return to the keep-alive
// pool; capping it avoids reading a large page in full.
const maxDrain = 64 << 10

// probe issues a HEAD request (falling back to GET on 405) and maps the
// outcome to a urlResult. timeout bounds each individual request.
// allowInternal selects the guarded (SSRF-blocking) or permissive client.
func probe(raw string, timeout time.Duration, allowInternal bool) urlResult {
	c := guardedClient
	if allowInternal {
		c = permissiveClient
	}
	res := doWithClient(c, http.MethodHead, raw, timeout)
	if res.err == nil && res.statusCode == http.StatusMethodNotAllowed {
		res = doWithClient(c, http.MethodGet, raw, timeout)
	}
	return res
}

// doWithClient performs one request with the given client, method, and a
// context timeout, draining a bounded prefix of the body and closing it
// so the connection can be reused.
func doWithClient(c *http.Client, method, raw string, timeout time.Duration) urlResult {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, raw, nil)
	if err != nil {
		return urlResult{probed: true, err: err}
	}
	resp, err := c.Do(req)
	if err != nil {
		return urlResult{probed: true, err: err}
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxDrain))
	_ = resp.Body.Close()
	return urlResult{probed: true, statusCode: resp.StatusCode}
}
