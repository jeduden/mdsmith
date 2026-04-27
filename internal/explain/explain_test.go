package explain

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAttach_PopulatesExplanation is the happy-path: each diag whose
// rule appears in the resolved config gets its leaves attached.
func TestAttach_PopulatesExplanation(t *testing.T) {
	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"line-length": {Enabled: true, Settings: map[string]any{"max": 80}},
		},
	}
	diags := []lint.Diagnostic{{
		File: "x.md", Line: 1, Column: 1,
		RuleID: "MDS001", RuleName: "line-length",
		Severity: lint.Error, Message: "too long",
	}}

	Attach(diags, cfg, "x.md", nil)
	require.NotNil(t, diags[0].Explanation)
	assert.Equal(t, "line-length", diags[0].Explanation.Rule)

	var sawMax bool
	for _, l := range diags[0].Explanation.Leaves {
		if l.Path == "settings.max" {
			sawMax = true
			assert.Equal(t, "default", l.Source)
			assert.Equal(t, 80, l.Value)
		}
	}
	assert.True(t, sawMax, "settings.max leaf must appear in the explanation")
}

// TestAttach_SkipsDiagsForUnknownRule covers the branch where a
// diagnostic's RuleName is absent from the resolved config — the
// helper must leave Explanation nil rather than fabricate provenance.
func TestAttach_SkipsDiagsForUnknownRule(t *testing.T) {
	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"line-length": {Enabled: true},
		},
	}
	diags := []lint.Diagnostic{{
		File: "x.md", Line: 1, Column: 1,
		RuleID: "MDS999", RuleName: "phantom-rule",
	}}
	Attach(diags, cfg, "x.md", nil)
	assert.Nil(t, diags[0].Explanation)
}

// TestAttach_EmptyDiagsIsNoOp guards the early-return branch and
// avoids the wasted ResolveFile call when there is nothing to attach.
func TestAttach_EmptyDiagsIsNoOp(t *testing.T) {
	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{"line-length": {Enabled: true}},
	}
	Attach(nil, cfg, "x.md", nil)
	Attach([]lint.Diagnostic{}, cfg, "x.md", nil)
	// No panic, no allocation; nothing to assert beyond a successful return.
}

// TestAttach_FrontMatterKindsApplied ensures the helper threads through
// fmKinds so kind-driven leaf sources show up correctly.
func TestAttach_FrontMatterKindsApplied(t *testing.T) {
	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"line-length": {Enabled: true, Settings: map[string]any{"max": 80}},
		},
		Kinds: map[string]config.KindBody{
			"short": {Rules: map[string]config.RuleCfg{
				"line-length": {Enabled: true, Settings: map[string]any{"max": 30}},
			}},
		},
	}
	diags := []lint.Diagnostic{{
		File: "x.md", Line: 1, Column: 1,
		RuleID: "MDS001", RuleName: "line-length",
	}}
	Attach(diags, cfg, "x.md", []string{"short"})
	require.NotNil(t, diags[0].Explanation)

	var sawMax bool
	for _, l := range diags[0].Explanation.Leaves {
		if l.Path == "settings.max" {
			sawMax = true
			assert.Equal(t, "kinds.short", l.Source)
			assert.Equal(t, 30, l.Value)
		}
	}
	assert.True(t, sawMax)
}
