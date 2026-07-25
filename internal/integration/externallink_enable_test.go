package integration

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jeduden/mdsmith/internal/checker"
	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
	_ "github.com/jeduden/mdsmith/internal/rules/all"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestExternalLinkCheck_BareEnableProbes is the end-to-end regression
// test for the enable-with-`true` no-op bug. Enabling MDS072 with the
// documented bare form (`external-link-check: true`) sets
// RuleCfg{Enabled: true, Settings: nil}. checker.ConfigureRule returns
// the rule unchanged when Settings is nil (no ApplySettings call), so a
// rule that only became functional inside ApplySettings would silently
// probe nothing. This test drives that exact path and asserts a broken
// URL is reported, guarding against a regression to the old
// RateLimit==0 sentinel.
//
// The test also enables external-allow-internal so the httptest server
// (bound to 127.0.0.1) is reachable; the SSRF guard is tested separately
// in the externallink unit tests.
func TestExternalLinkCheck_BareEnableProbes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	// Bare enable: Settings is nil, exactly as `external-link-check: true`
	// parses. A second config entry supplies allow-internal so the httptest
	// server is reachable; both must combine into a single working rule.
	effective := map[string]config.RuleCfg{
		"external-link-check": {
			Enabled: true,
			Settings: map[string]any{
				"links": map[string]any{
					"external-allow-internal": true,
				},
			},
		},
	}
	configured, errs := checker.ConfigureEnabledRules(rule.All(), effective)
	require.Empty(t, errs)

	var mds072 rule.Rule
	for _, r := range configured {
		if r.ID() == "MDS072" {
			mds072 = r
		}
	}
	require.NotNil(t, mds072, "external-link-check must be enabled and configured")

	f, err := lint.NewFile("doc.md", []byte("# T\n\nSee [x]("+srv.URL+"/missing).\n"))
	require.NoError(t, err)
	diags := mds072.Check(f)
	require.Len(t, diags, 1, "bare-enabled external-link-check must probe and report the broken URL")
	assert.Contains(t, diags[0].Message, "HTTP 404")
}
