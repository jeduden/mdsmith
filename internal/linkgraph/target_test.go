package linkgraph

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParseTargetBytes_ExternalIsAllocationFree pins the fix: a
// destination that is external (has a scheme, or is protocol-relative)
// can be rejected by scanning the raw bytes, without the string copy
// and the net/url.URL the full parse allocates.
//
// External links dominate real documents, and every one of them used
// to pay `string(l.Destination)` plus a full url.Parse only to be
// discarded. See docs/development/high-performance-go.md — "Stay in
// []byte. Each string(b) allocates and copies" and
// "#skip-work-you-dont-need".
func TestParseTargetBytes_ExternalIsAllocationFree(t *testing.T) {
	externals := [][]byte{
		[]byte("https://example.com/a/b?q=1#frag"),
		[]byte("http://example.com"),
		[]byte("mailto:someone@example.com"),
		[]byte("//example.com/protocol-relative"),
		[]byte("ftp://files.example.com/x"),
	}

	avg := testing.AllocsPerRun(100, func() {
		for _, dest := range externals {
			if _, ok := ParseTargetBytes(dest); ok {
				t.Fatalf("external destination %q was accepted", dest)
			}
		}
	})
	assert.Zero(t, avg,
		"rejecting an external destination must not allocate")
}

// TestParseTargetBytes_MatchesParseTarget is the differential test:
// the byte-taking form must agree with the string form on every
// destination, so the fast reject cannot change which links the
// link-graph reports.
func TestParseTargetBytes_MatchesParseTarget(t *testing.T) {
	dests := []string{
		// External / rejected.
		"https://example.com", "HTTP://EXAMPLE.COM", "mailto:a@b.c",
		"//example.com", "ftp://x/y", "a+b-c.d://x", "x1://y",
		// Local / accepted.
		"guide.md", "guide.md#sec", "#sec", "./guide.md", "../a/b.md",
		"/abs/path.md", "dir/file.md#frag", "a%20b.md",
		// Colon that is NOT a scheme: net/url only reads a scheme
		// before the first slash, question mark or hash.
		"dir/a:b.md", "./a:b.md", "a/b:c#d", "?q=1", "#a:b",
		// Edge shapes.
		"", "   ", "  guide.md  ", ":", "://x", "1abc://x", "-x://y",
		"a:", "a:b", "%zz", "file.md?q=1", "x#",
	}
	for _, d := range dests {
		t.Run(d, func(t *testing.T) {
			wantTarget, wantOK := ParseTarget(d)
			gotTarget, gotOK := ParseTargetBytes([]byte(d))
			require.Equal(t, wantOK, gotOK, "ok mismatch for %q", d)
			assert.Equal(t, wantTarget, gotTarget, "target mismatch for %q", d)
		})
	}
}

// TestParseTargetBytes_ExhaustiveDifferential enumerates every string
// up to length 5 over an alphabet of the characters that decide scheme
// detection, percent-escaping, query stripping and control-character
// rejection, and requires ParseTargetBytes to return exactly what
// ParseTarget returns for each. This is the property the fast paths
// must never break: they may only skip work, never change an answer.
func TestParseTargetBytes_ExhaustiveDifferential(t *testing.T) {
	const alphabet = "aA1+-.:/?# %\x01"
	checked := 0

	var build func(prefix string, depth int)
	build = func(prefix string, depth int) {
		wantTarget, wantOK := ParseTarget(prefix)
		gotTarget, gotOK := ParseTargetBytes([]byte(prefix))
		if wantOK != gotOK || wantTarget != gotTarget {
			t.Fatalf("mismatch for %q: ParseTarget=(%+v,%v) ParseTargetBytes=(%+v,%v)",
				prefix, wantTarget, wantOK, gotTarget, gotOK)
		}
		checked++
		if depth == 0 {
			return
		}
		for _, c := range alphabet {
			build(prefix+string(c), depth-1)
		}
	}
	build("", 5)

	require.Greater(t, checked, 200000,
		"the enumeration should cover the whole scheme-decision space")
}

// TestIsExternalDestination_ImpliesRejected states the fast path's
// contract directly: whenever it claims a destination is external,
// ParseTarget must also reject it.
func TestIsExternalDestination_ImpliesRejected(t *testing.T) {
	const alphabet = "aA1+-.:/?# "
	var build func(prefix string, depth int)
	build = func(prefix string, depth int) {
		if isExternalDestination([]byte(strings.TrimSpace(prefix))) {
			_, ok := ParseTarget(prefix)
			assert.Falsef(t, ok,
				"fast reject dropped %q, which ParseTarget accepts", prefix)
		}
		if depth == 0 {
			return
		}
		for _, c := range alphabet {
			build(prefix+string(c), depth-1)
		}
	}
	build("", 4)
}
