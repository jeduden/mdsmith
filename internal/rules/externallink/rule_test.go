package externallink

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/sync/singleflight"

	goldast "github.com/jeduden/mdsmith/pkg/goldmark/ast"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// resetForTest clears the package-level probe state so each test starts
// from a clean slate. urlCache, the semaphore, the probe counter, and
// the singleflight group are process-global, so without this reset a
// 200 cached by one test would mask a 404 the next test expects, and a
// semaphore sized by one test's rate limit would carry into the next.
func resetForTest(t *testing.T) {
	t.Helper()
	reset := func() {
		urlCache = sync.Map{}
		probeGroup = singleflight.Group{}
		semaphore = nil
		semOnce = sync.Once{}
		probeCount.Store(0)
		probeMax = 0
		probeMaxOnce = sync.Once{}
		probeURL = probe
	}
	reset()
	t.Cleanup(reset)
}

// newConfiguredRule returns a Rule with the given links settings applied.
// It always sets external-allow-internal: true so tests can use httptest
// servers (which bind to 127.0.0.1) without the SSRF guard blocking them.
// Tests that specifically verify SSRF behaviour use newSSRFAwareRule.
func newConfiguredRule(t *testing.T, links map[string]any) *Rule {
	t.Helper()
	r := &Rule{}
	merged := map[string]any{"external-allow-internal": true}
	for k, v := range links {
		merged[k] = v
	}
	require.NoError(t, r.ApplySettings(map[string]any{"links": merged}))
	resetForTest(t)
	return r
}

// newSSRFAwareRule returns a Rule with SSRF guard enabled (the default)
// and the given links settings applied. Use this for tests that verify
// guard behaviour rather than HTTP protocol details.
func newSSRFAwareRule(t *testing.T, links map[string]any) *Rule {
	t.Helper()
	r := &Rule{}
	settings := map[string]any{}
	if links != nil {
		settings["links"] = links
	}
	require.NoError(t, r.ApplySettings(settings))
	resetForTest(t)
	return r
}

func mustFile(t *testing.T, body string) *lint.File {
	t.Helper()
	f, err := lint.NewFile("doc.md", []byte(body))
	require.NoError(t, err)
	return f
}

// TestCheck_DefaultsProbeWithoutApplySettings is the regression test for
// the enable-with-`true` no-op bug: the instance init registers
// (newRule) carries the built-in defaults, so it probes even though
// ApplySettings was never called — exactly the path checker.ConfigureRule
// takes for `external-link-check: true` (cfg.Settings nil → rule returned
// unchanged). A regression to the old `RateLimit==0` sentinel would make
// this return nil.
//
// The rule is manually configured with AllowInternal=true here because
// httptest binds to 127.0.0.1 and the SSRF guard (the default) would
// block it; this test is about the "defaults survive without ApplySettings"
// path, not about the guard.
func TestCheck_DefaultsProbeWithoutApplySettings(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	resetForTest(t)
	r := newRule()               // exactly what init() registers; no ApplySettings
	r.links.AllowInternal = true // allow loopback for httptest server
	require.Equal(t, defaultRateLimit, r.links.RateLimit)
	f := mustFile(t, "# T\n\nSee [x]("+srv.URL+"/missing).\n")
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "HTTP 404")
}

func TestCheck_SkipNonHTTP(t *testing.T) {
	r := newConfiguredRule(t, nil)
	f := mustFile(t,
		"# T\n\nLocal [a](other.md) and image ![x](data:image/png;base64,AA==).\n\nMail <mailto:a@b.com>.\n",
	)
	require.Nil(t, r.Check(f))
}

func TestCheck_SkipPattern(t *testing.T) {
	r := newConfiguredRule(t, map[string]any{
		"external-skip": []any{`^https?://localhost`},
	})
	f := mustFile(t, "# T\n\nSee [x](http://localhost:9999/never).\n")
	require.Nil(t, r.Check(f))
}

func TestCheck_HTTP200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	r := newConfiguredRule(t, nil)
	f := mustFile(t, "# T\n\nSee [x]("+srv.URL+"/ok).\n")
	require.Nil(t, r.Check(f))
}

func TestCheck_HTTP404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	r := newConfiguredRule(t, nil)
	f := mustFile(t, "# T\n\nSee [x]("+srv.URL+"/missing).\n")
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Equal(t, "MDS072", diags[0].RuleID)
	assert.Contains(t, diags[0].Message, "HTTP 404")
	assert.Contains(t, diags[0].Message, srv.URL+"/missing")
}

func TestCheck_HTTP405ThenGET(t *testing.T) {
	var sawHEAD, sawGET bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch req.Method {
		case http.MethodHead:
			sawHEAD = true
			w.WriteHeader(http.StatusMethodNotAllowed)
		case http.MethodGet:
			sawGET = true
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("body that must be drained for keep-alive"))
		}
	}))
	defer srv.Close()

	r := newConfiguredRule(t, nil)
	f := mustFile(t, "# T\n\nSee [x]("+srv.URL+"/m).\n")
	require.Nil(t, r.Check(f))
	assert.True(t, sawHEAD, "HEAD should be attempted first")
	assert.True(t, sawGET, "GET should be the 405 fallback")
}

func TestCheck_TransportError(t *testing.T) {
	r := newConfiguredRule(t, map[string]any{
		"external-timeout": "200ms",
	})
	// Reserved TEST-NET-1 address that does not route; fails fast.
	f := mustFile(t, "# T\n\nSee [x](http://192.0.2.1:9/down).\n")
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "unreachable")
}

// TestCheck_AutolinkPosition is the regression test for autolink
// diagnostics collapsing to line 1, col 1: an AutoLink has no walkable
// *Text child, so the position must come from locating `<url>` in the
// block source. The broken autolink sits on line 5.
func TestCheck_AutolinkPosition(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	r := newConfiguredRule(t, nil)
	// Autolink on line 5 (1: heading, 2: blank, 3: prose, 4: blank, 5: link).
	body := "# T\n\nSome intro prose.\n\nAutolink <" + srv.URL + "/missing> here.\n"
	f := mustFile(t, body)
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "HTTP 404")
	assert.Equal(t, 5, diags[0].Line, "autolink diagnostic must anchor at the autolink's real line")
	assert.Greater(t, diags[0].Column, 1, "autolink diagnostic column must not fall back to 1")
}

// TestCheck_AutolinkInEmphasisPosition drives autolinkPosition's
// walk-past-inline-ancestor path: an autolink nested in strong emphasis
// has a non-block parent (the emphasis), so the loop must skip it and
// keep climbing to the enclosing paragraph block.
func TestCheck_AutolinkInEmphasisPosition(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	r := newConfiguredRule(t, nil)
	// Autolink wrapped in strong emphasis on line 3.
	body := "# T\n\nSee **<" + srv.URL + "/missing>** now.\n"
	f := mustFile(t, body)
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Equal(t, 3, diags[0].Line)
	assert.Greater(t, diags[0].Column, 1)
}

// TestCheck_ImageExternal covers probing an external image destination.
func TestCheck_ImageExternal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	r := newConfiguredRule(t, nil)
	f := mustFile(t, "# T\n\n![alt]("+srv.URL+"/missing.png)\n")
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "HTTP 404")
}

func TestCheck_CacheHit(t *testing.T) {
	var hits int
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		hits++
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	r := newConfiguredRule(t, nil)
	body := "# T\n\nSee [x](" + srv.URL + "/ok).\n"
	require.Nil(t, r.Check(mustFile(t, body)))
	require.Nil(t, r.Check(mustFile(t, body)))
	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, 1, hits, "second Check should hit the cache, not the server")
}

// TestCheck_ConcurrentSingleRequest exercises the singleflight dedup:
// many workers checking the same URL at once must produce exactly one
// HTTP request, matching the "one request per URL per run" guarantee
// under real per-worker concurrency.
func TestCheck_ConcurrentSingleRequest(t *testing.T) {
	var hits int
	var mu sync.Mutex
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		hits++
		mu.Unlock()
		<-release // hold the handler open so all goroutines pile onto one URL
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	r := newConfiguredRule(t, map[string]any{"external-rate-limit": 8})
	body := "# T\n\nSee [x](" + srv.URL + "/ok).\n"

	const workers = 8
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = r.Check(mustFile(t, body))
		}()
	}
	// Give the goroutines time to converge on the singleflight key, then
	// let the handler finish.
	time.Sleep(50 * time.Millisecond)
	close(release)
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, 1, hits, "concurrent checks of one URL must issue a single request")
}

func TestCheck_NilFile(t *testing.T) {
	r := newConfiguredRule(t, nil)
	require.Nil(t, r.Check(nil))
}

func TestCheck_NilAST(t *testing.T) {
	r := newConfiguredRule(t, nil)
	require.Nil(t, r.Check(&lint.File{Path: "doc.md"}))
}

func TestApplySettings_Defaults(t *testing.T) {
	r := &Rule{}
	require.NoError(t, r.ApplySettings(map[string]any{}))
	assert.Equal(t, 5*time.Second, r.links.Timeout)
	assert.Equal(t, 10, r.links.RateLimit)
	assert.False(t, r.links.AllowInternal, "SSRF guard must be on by default")
	assert.Equal(t, defaultMaxProbes, r.links.MaxProbes)
}

// TestApplySettings_SkipReset verifies that calling ApplySettings a second
// time without external-skip clears a previously compiled skip-pattern list,
// so removing the setting from config takes effect without a restart.
func TestApplySettings_SkipReset(t *testing.T) {
	r := &Rule{}
	require.NoError(t, r.ApplySettings(map[string]any{
		"links": map[string]any{"external-skip": []any{`^https://skip`}},
	}))
	require.NotNil(t, r.links.Skip)

	require.NoError(t, r.ApplySettings(map[string]any{}))
	assert.Nil(t, r.links.Skip, "ApplySettings must clear skip patterns when external-skip is absent")
}

func TestApplySettings_CustomTimeout(t *testing.T) {
	r := &Rule{}
	require.NoError(t, r.ApplySettings(map[string]any{
		"links": map[string]any{"external-timeout": "2s"},
	}))
	assert.Equal(t, 2*time.Second, r.links.Timeout)
}

func TestApplySettings_TimeoutNonPositive(t *testing.T) {
	r := &Rule{}
	require.NoError(t, r.ApplySettings(map[string]any{
		"links": map[string]any{"external-timeout": "0s"},
	}))
	assert.Equal(t, 5*time.Second, r.links.Timeout)
}

func TestApplySettings_TimeoutInvalidDuration(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{
		"links": map[string]any{"external-timeout": "notaduration"},
	})
	require.Error(t, err)
}

func TestApplySettings_TimeoutNotString(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{
		"links": map[string]any{"external-timeout": 5},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be a duration string")
}

func TestApplySettings_CustomRateLimit(t *testing.T) {
	r := &Rule{}
	require.NoError(t, r.ApplySettings(map[string]any{
		"links": map[string]any{"external-rate-limit": 3},
	}))
	assert.Equal(t, 3, r.links.RateLimit)
}

func TestApplySettings_RateLimitMinimum(t *testing.T) {
	r := &Rule{}
	require.NoError(t, r.ApplySettings(map[string]any{
		"links": map[string]any{"external-rate-limit": 0},
	}))
	assert.GreaterOrEqual(t, r.links.RateLimit, 1)
}

func TestApplySettings_RateLimitNotInt(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{
		"links": map[string]any{"external-rate-limit": "bad"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be an integer")
}

func TestApplySettings_UnknownLinksKey(t *testing.T) {
	// Keys owned by MDS027 / MDS068 must be tolerated so one shared
	// links: block configures every link rule.
	r := &Rule{}
	require.NoError(t, r.ApplySettings(map[string]any{
		"links": map[string]any{
			"site-root":                "docs",
			"validate-images":          true,
			"validate-reference-style": true,
			"style":                    map[string]any{"path": "relative"},
		},
	}))
}

func TestApplySettings_TrulyUnknownLinksKey(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{
		"links": map[string]any{"unknown-future-key": true},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown links setting")
}

func TestApplySettings_LinksNotMap(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{"links": "not-a-map"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "links must be a map")
}

func TestApplySettings_SkipNotList(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{
		"links": map[string]any{"external-skip": 42},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be a list of strings")
}

func TestApplySettings_SkipListNonString(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{
		"links": map[string]any{"external-skip": []any{"valid", 42}},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be a list of strings")
}

func TestApplySettings_UnknownTopKey(t *testing.T) {
	r := &Rule{}
	require.Error(t, r.ApplySettings(map[string]any{"nope": true}))
}

func TestApplySettings_BadSkipPattern(t *testing.T) {
	r := &Rule{}
	require.Error(t, r.ApplySettings(map[string]any{
		"links": map[string]any{"external-skip": []any{"("}},
	}))
}

func TestApplySettings_SkipStringSliceInput(t *testing.T) {
	// A []string (not []any) input to external-skip is accepted, exercising
	// the toStringSlice []string branch.
	r := &Rule{}
	require.NoError(t, r.ApplySettings(map[string]any{
		"links": map[string]any{"external-skip": []string{`^https?://x`}},
	}))
	require.Len(t, r.skipRegs, 1)
}

func TestToInt_Kinds(t *testing.T) {
	for _, tc := range []struct {
		in   any
		want int
		ok   bool
	}{
		{int(5), 5, true},
		{int64(6), 6, true},
		{float64(7), 7, true},
		{"nope", 0, false},
	} {
		got, ok := toInt(tc.in)
		assert.Equal(t, tc.ok, ok)
		assert.Equal(t, tc.want, got)
	}
}

func TestIsExternalHTTP(t *testing.T) {
	assert.True(t, isExternalHTTP("https://example.com"))
	assert.True(t, isExternalHTTP("http://example.com/x"))
	assert.False(t, isExternalHTTP("mailto:a@b.com"))
	assert.False(t, isExternalHTTP("other.md"))
	assert.False(t, isExternalHTTP("http://")) // no host
	assert.False(t, isExternalHTTP("http://[invalid"))
}

func TestDefaultSettings(t *testing.T) {
	r := &Rule{}
	s := r.DefaultSettings()
	links, ok := s["links"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "5s", links["external-timeout"])
	assert.Equal(t, defaultRateLimit, links["external-rate-limit"])
	assert.Equal(t, false, links["external-allow-internal"])
	assert.Equal(t, defaultMaxProbes, links["external-max-probes"])
}

func TestMetadata(t *testing.T) {
	r := &Rule{}
	assert.Equal(t, "MDS072", r.ID())
	assert.Equal(t, "external-link-check", r.Name())
	assert.Equal(t, "link", r.Category())
	assert.False(t, r.EnabledByDefault())
}

// TestFailureMessage_TriState pins the three probe outcomes. A probed
// failure (transport error or 4xx/5xx) yields a message; a probed
// healthy response yields none; and a NOT-probed result yields none even
// when statusCode/err look actionable. The not-probed state is what a
// host without a probe bridge returns, so it must never be reported as a
// broken link — nor as a false pass.
func TestFailureMessage_TriState(t *testing.T) {
	// probed failures
	assert.NotEmpty(t, failureMessage("http://x", urlResult{probed: true, statusCode: 404}))
	assert.NotEmpty(t, failureMessage("http://x", urlResult{probed: true, err: assertErr{}}))
	// probed healthy
	assert.Empty(t, failureMessage("http://x", urlResult{probed: true, statusCode: 200}))
	assert.Empty(t, failureMessage("http://x", urlResult{probed: true, statusCode: 301}))
	// NOT probed: never a diagnostic, whatever the other fields say
	assert.Empty(t, failureMessage("http://x", urlResult{probed: false, statusCode: 404}))
	assert.Empty(t, failureMessage("http://x", urlResult{probed: false, err: assertErr{}}))
	assert.Empty(t, failureMessage("http://x", urlResult{}))
}

type assertErr struct{}

func (assertErr) Error() string { return "boom" }

// TestCheck_ImageNoAltPositionFallback drives the first-text-offset < 0
// fallback in position(): an image with empty alt text has no *Text
// descendant, so the diagnostic anchors at (1, 1).
func TestCheck_ImageNoAltPositionFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	r := newConfiguredRule(t, nil)
	f := mustFile(t, "# T\n\n![]("+srv.URL+"/missing.png)\n")
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Equal(t, 1, diags[0].Line)
	assert.Equal(t, 1, diags[0].Column)
}

// TestAutolinkPosition_Fallbacks covers autolinkPosition's degenerate
// paths directly: an empty URL and a URL absent from the source both
// fall back to (1, 1).
func TestAutolinkPosition_Fallbacks(t *testing.T) {
	f := mustFile(t, "# T\n\nProse without any autolink.\n")
	root := f.AST
	line, col := autolinkPosition(f, root, "")
	assert.Equal(t, 1, line)
	assert.Equal(t, 1, col)

	line, col = autolinkPosition(f, root, "https://absent.example.com")
	assert.Equal(t, 1, line)
	assert.Equal(t, 1, col)

	// A node whose block ancestor exists but whose source lines do not
	// contain the `<url>` literal exercises the search-miss break and the
	// trailing (1, 1) fallback. Find any inline node inside the paragraph.
	var inline goldast.Node
	_ = goldast.Walk(f.AST, func(n goldast.Node, entering bool) (goldast.WalkStatus, error) {
		if entering && inline == nil {
			if _, ok := n.(*goldast.Text); ok {
				inline = n
			}
		}
		return goldast.WalkContinue, nil
	})
	require.NotNil(t, inline)
	line, col = autolinkPosition(f, inline, "https://not-in-source.example.com")
	assert.Equal(t, 1, line)
	assert.Equal(t, 1, col)
}

// TestAcquireRelease_ClampsBelowOne drives acquire()'s min-1 clamp: a
// zero-value Rule (RateLimit 0) still sizes the semaphore to 1 rather
// than building an unbuffered channel that would deadlock on send.
func TestAcquireRelease_ClampsBelowOne(t *testing.T) {
	resetForTest(t)
	r := &Rule{} // RateLimit 0
	r.acquire()
	assert.Equal(t, 1, cap(semaphore), "semaphore must clamp to at least 1 slot")
	r.release()
}

// TestCheck_SSRFGuardBlocksLoopback verifies that the SSRF guard (on by
// default) prevents probing a loopback address. The check must fail with
// a transport error (the connection is denied), not a false pass.
func TestCheck_SSRFGuardBlocksLoopback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// SSRF guard is on: newSSRFAwareRule does not set AllowInternal.
	r := newSSRFAwareRule(t, nil)
	f := mustFile(t, "# T\n\nSee [x]("+srv.URL+"/ok).\n")
	diags := r.Check(f)
	require.Len(t, diags, 1, "loopback probe must be denied")
	assert.Contains(t, diags[0].Message, "unreachable", "denied connection must be reported as unreachable")
}

// TestCheck_AllowInternalEnablesLoopback confirms that external-allow-internal:
// true bypasses the SSRF guard so a loopback httptest server can be probed.
func TestCheck_AllowInternalEnablesLoopback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	r := newSSRFAwareRule(t, map[string]any{"external-allow-internal": true})
	f := mustFile(t, "# T\n\nSee [x]("+srv.URL+"/ok).\n")
	require.Nil(t, r.Check(f), "allow-internal must permit loopback probes")
}

// TestApplySettings_AllowInternal verifies that external-allow-internal is
// parsed correctly and defaults to false.
func TestApplySettings_AllowInternal(t *testing.T) {
	r := &Rule{}
	require.NoError(t, r.ApplySettings(map[string]any{
		"links": map[string]any{"external-allow-internal": true},
	}))
	assert.True(t, r.links.AllowInternal)

	// Reset and verify false is the default.
	require.NoError(t, r.ApplySettings(map[string]any{}))
	assert.False(t, r.links.AllowInternal)
}

// TestApplySettings_AllowInternalNotBool confirms the type guard.
func TestApplySettings_AllowInternalNotBool(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{
		"links": map[string]any{"external-allow-internal": "yes"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be a bool")
}

// TestApplySettings_MaxProbes verifies external-max-probes parsing.
func TestApplySettings_MaxProbes(t *testing.T) {
	r := &Rule{}
	require.NoError(t, r.ApplySettings(map[string]any{
		"links": map[string]any{"external-max-probes": 50},
	}))
	assert.Equal(t, 50, r.links.MaxProbes)

	// 0 means unlimited.
	require.NoError(t, r.ApplySettings(map[string]any{
		"links": map[string]any{"external-max-probes": 0},
	}))
	assert.Equal(t, 0, r.links.MaxProbes)

	// Negative clamps to 0.
	require.NoError(t, r.ApplySettings(map[string]any{
		"links": map[string]any{"external-max-probes": -1},
	}))
	assert.Equal(t, 0, r.links.MaxProbes)
}

// TestApplySettings_MaxProbesNotInt confirms the type guard.
func TestApplySettings_MaxProbesNotInt(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{
		"links": map[string]any{"external-max-probes": "many"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be an integer")
}

// TestCheck_MaxProbesCap verifies that a run with N+1 distinct URLs issues
// at most N requests when external-max-probes is N. The N+1th URL must be
// reported as unchecked (capped diagnostic) rather than silently omitted.
func TestCheck_MaxProbesCap(t *testing.T) {
	const maxProbes = 3
	var mu sync.Mutex
	requestCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		requestCount++
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	r := newConfiguredRule(t, map[string]any{"external-max-probes": maxProbes})

	// Build a document with maxProbes+1 distinct URLs.
	var sb strings.Builder
	sb.WriteString("# T\n\n")
	for i := range maxProbes + 1 {
		fmt.Fprintf(&sb, "See [x%d](%s/path%d).\n\n", i, srv.URL, i)
	}
	diags := r.Check(mustFile(t, sb.String()))

	mu.Lock()
	got := requestCount
	mu.Unlock()

	assert.LessOrEqual(t, got, maxProbes, "must issue at most external-max-probes requests")

	capped := 0
	for _, d := range diags {
		if strings.Contains(d.Message, "per-run limit reached") {
			capped++
		}
	}
	assert.Equal(t, 1, capped, "exactly the N+1th URL should be reported as unchecked")
}

// TestCheck_MaxProbesCap_CachedResult verifies that a URL capped in one Check
// call returns the cached capped result on a subsequent call without issuing
// another request or recounting against the probe ceiling.
func TestCheck_MaxProbesCap_CachedResult(t *testing.T) {
	const maxProbes = 1
	var mu sync.Mutex
	requestCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		requestCount++
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	r := newConfiguredRule(t, map[string]any{"external-max-probes": maxProbes})
	cappedURL := srv.URL + "/capped"

	// First check: one URL gets probed, one gets capped.
	doc1 := fmt.Sprintf("# T\n\nSee [a](%s/ok) and [b](%s).\n", srv.URL, cappedURL)
	diags1 := r.Check(mustFile(t, doc1))
	capped1 := 0
	for _, d := range diags1 {
		if strings.Contains(d.Message, "per-run limit reached") {
			capped1++
		}
	}
	assert.Equal(t, 1, capped1, "first check: one URL must be capped")

	// Second check: same capped URL appears again — must not issue a new request.
	doc2 := fmt.Sprintf("# T\n\nSee [b2](%s).\n", cappedURL)
	diags2 := r.Check(mustFile(t, doc2))
	capped2 := 0
	for _, d := range diags2 {
		if strings.Contains(d.Message, "per-run limit reached") {
			capped2++
		}
	}
	assert.Equal(t, 1, capped2, "second check: cached capped result must still report the URL as unchecked")

	mu.Lock()
	got := requestCount
	mu.Unlock()
	assert.Equal(t, maxProbes, got, "total requests must equal max-probes, not grow on repeated capped URL")
}

// TestFailureMessage_Capped pins the capped-result diagnostic message.
func TestFailureMessage_Capped(t *testing.T) {
	msg := failureMessage("https://example.com/page", urlResult{capped: true})
	assert.Contains(t, msg, "not probed")
	assert.Contains(t, msg, "per-run limit reached")
	assert.Contains(t, msg, "https://example.com/page")
}

// TestCheck_ProbeURLRollback verifies that when probeURL returns probed=false
// (the WASM stub behaviour), the probe slot is returned so the per-run ceiling
// only counts actual network requests. Without the rollback, 1000 WASM
// no-ops would permanently cap the rule after defaultMaxProbes calls.
func TestCheck_ProbeURLRollback(t *testing.T) {
	r := newConfiguredRule(t, map[string]any{"external-max-probes": 2})

	// Replace probeURL with a stub that never fires network I/O.
	probeURL = func(_ string, _ time.Duration, _ bool) urlResult {
		return urlResult{} // probed=false, no error, no status
	}

	// Probe two URLs — if slots are not returned, the ceiling would be hit.
	r.checkURL("http://example.com/1")
	r.checkURL("http://example.com/2")

	// A third distinct URL must not be capped: probeCount should be 0.
	res := r.checkURL("http://example.com/3")
	assert.False(t, res.capped, "ceiling must not trigger when probe is a no-op (probed=false)")
	assert.Equal(t, int64(0), probeCount.Load(), "probeCount must be 0 after WASM-stub rollbacks")
}
