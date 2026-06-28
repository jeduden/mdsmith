package externallink

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// resetForTest clears the package-level URL cache and gives r a fresh
// initOnce so each test starts from a clean slate. urlCache and the
// HTTP client are process-global, so without this reset a 200 cached
// by one test would mask a 404 the next test expects.
func resetForTest(t *testing.T, r *Rule) {
	t.Helper()
	urlCache = sync.Map{}
	r.initOnce = sync.Once{}
	t.Cleanup(func() { urlCache = sync.Map{} })
}

// newConfiguredRule returns a Rule with the given settings applied,
// defaulting RateLimit and Timeout so Check does real work.
func newConfiguredRule(t *testing.T, links map[string]any) *Rule {
	t.Helper()
	r := &Rule{}
	settings := map[string]any{}
	if links != nil {
		settings["links"] = links
	}
	require.NoError(t, r.ApplySettings(settings))
	resetForTest(t, r)
	return r
}

func mustFile(t *testing.T, body string) *lint.File {
	t.Helper()
	f, err := lint.NewFile("doc.md", []byte(body))
	require.NoError(t, err)
	return f
}

func TestCheck_SkipWhenUnconfigured(t *testing.T) {
	// A zero-value Rule (ApplySettings never called) must not make any
	// HTTP call: it returns nil immediately so the alloc-budget gate
	// can run it on a fixture with an external URL.
	r := &Rule{}
	f := mustFile(t, "# T\n\nSee [x](https://example.invalid).\n")
	require.Nil(t, r.Check(f))
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
	assert.Equal(t, "MDS071", diags[0].RuleID)
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

func TestCheck_Autolink(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	r := newConfiguredRule(t, nil)
	f := mustFile(t, "# T\n\nAutolink <"+srv.URL+"/missing>.\n")
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

func TestCheck_NilFile(t *testing.T) {
	r := newConfiguredRule(t, nil)
	require.Nil(t, r.Check(nil))
}

func TestApplySettings_Defaults(t *testing.T) {
	r := &Rule{}
	require.NoError(t, r.ApplySettings(map[string]any{}))
	assert.Equal(t, 5*time.Second, r.links.Timeout)
	assert.Equal(t, 10, r.links.RateLimit)
}

func TestApplySettings_CustomTimeout(t *testing.T) {
	r := &Rule{}
	require.NoError(t, r.ApplySettings(map[string]any{
		"links": map[string]any{"external-timeout": "2s"},
	}))
	assert.Equal(t, 2*time.Second, r.links.Timeout)
}

func TestApplySettings_CustomRateLimit(t *testing.T) {
	r := &Rule{}
	require.NoError(t, r.ApplySettings(map[string]any{
		"links": map[string]any{"external-rate-limit": 3},
	}))
	assert.Equal(t, 3, r.links.RateLimit)
}

func TestApplySettings_RateLimitMinimum(t *testing.T) {
	// A configured rule must never leave RateLimit at the zero value,
	// or Check's "unconfigured" early-return would swallow it. A
	// non-positive setting clamps to 1.
	r := &Rule{}
	require.NoError(t, r.ApplySettings(map[string]any{
		"links": map[string]any{"external-rate-limit": 0},
	}))
	assert.GreaterOrEqual(t, r.links.RateLimit, 1)
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

func TestMetadata(t *testing.T) {
	r := &Rule{}
	assert.Equal(t, "MDS071", r.ID())
	assert.Equal(t, "external-link-check", r.Name())
	assert.Equal(t, "link", r.Category())
	assert.False(t, r.EnabledByDefault())
}

func TestCheck_NilAST(t *testing.T) {
	r := newConfiguredRule(t, nil)
	f := &lint.File{Path: "doc.md"} // AST is nil (zero value)
	require.Nil(t, r.Check(f))
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

func TestApplySettings_TimeoutNotString(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{
		"links": map[string]any{"external-timeout": 5},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be a duration string")
}

func TestApplySettings_TimeoutInvalidDuration(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{
		"links": map[string]any{"external-timeout": "notaduration"},
	})
	require.Error(t, err)
}

func TestApplySettings_TimeoutNonPositive(t *testing.T) {
	r := &Rule{}
	require.NoError(t, r.ApplySettings(map[string]any{
		"links": map[string]any{"external-timeout": "0s"},
	}))
	// A non-positive timeout clamps back to the default 5s.
	assert.Equal(t, 5*time.Second, r.links.Timeout)
}

func TestApplySettings_RateLimitNotInt(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{
		"links": map[string]any{"external-rate-limit": "bad"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be an integer")
}

func TestApplySettings_TrulyUnknownLinksKey(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{
		"links": map[string]any{"unknown-future-key": true},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown links setting")
}

func TestIsExternalHTTP_ParseError(t *testing.T) {
	// An unclosed IPv6 bracket makes url.Parse return an error; the
	// function must return false without panicking.
	assert.False(t, isExternalHTTP("http://[invalid"))
}

func TestToStringSlice_StringSlice(t *testing.T) {
	got, ok := toStringSlice([]string{"a", "b"})
	require.True(t, ok)
	assert.Equal(t, []string{"a", "b"}, got)
}

func TestApplySettings_SkipListNonString(t *testing.T) {
	// A []any containing a non-string element must be rejected.
	r := &Rule{}
	err := r.ApplySettings(map[string]any{
		"links": map[string]any{"external-skip": []any{"valid", 42}},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be a list of strings")
}

func TestToInt_Int64(t *testing.T) {
	got, ok := toInt(int64(5))
	require.True(t, ok)
	assert.Equal(t, 5, got)
}

func TestToInt_Float64(t *testing.T) {
	got, ok := toInt(float64(7))
	require.True(t, ok)
	assert.Equal(t, 7, got)
}

func TestDefaultSettings(t *testing.T) {
	r := &Rule{}
	s := r.DefaultSettings()
	links, ok := s["links"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "5s", links["external-timeout"])
	assert.Equal(t, defaultRateLimit, links["external-rate-limit"])
}

func TestCheck_HTTP405ThenGETError(t *testing.T) {
	// When HEAD returns 405 and the GET request fails with a transport
	// error (hijacked connection closed), the rule must emit a diagnostic.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch req.Method {
		case http.MethodHead:
			w.WriteHeader(http.StatusMethodNotAllowed)
		case http.MethodGet:
			hj, ok := w.(http.Hijacker)
			if !ok {
				http.Error(w, "hijack unavailable", http.StatusInternalServerError)
				return
			}
			conn, _, _ := hj.Hijack()
			_ = conn.Close()
		}
	}))
	defer srv.Close()

	r := newConfiguredRule(t, nil)
	f := mustFile(t, "# T\n\nSee [x]("+srv.URL+"/err).\n")
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "unreachable")
}
