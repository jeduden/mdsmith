package markdownflavor

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"
)

func TestRuleIdentity(t *testing.T) {
	r := &Rule{}
	assert.Equal(t, "MDS034", r.ID())
	assert.Equal(t, "markdown-flavor", r.Name())
	assert.Equal(t, "meta", r.Category())
}

func TestRuleIsConfigurableAndDefaultable(t *testing.T) {
	var r rule.Rule = &Rule{}
	_, ok := r.(rule.Configurable)
	assert.True(t, ok, "Rule must implement rule.Configurable")
	_, ok = r.(rule.Defaultable)
	assert.True(t, ok, "Rule must implement rule.Defaultable")
}

func TestRuleDisabledByDefault(t *testing.T) {
	r := &Rule{}
	assert.False(t, r.EnabledByDefault())
}

func TestRuleDefaultSettings(t *testing.T) {
	r := &Rule{}
	ds := r.DefaultSettings()
	assert.Equal(t, "", ds["flavor"])
	_, hasProfile := ds["profile"]
	assert.False(t, hasProfile,
		"profile is no longer a per-rule setting; convention lives at top level")
}

func TestRuleApplySettingsValid(t *testing.T) {
	valid := []string{
		"commonmark", "gfm", "goldmark",
		"any", "pandoc", "phpextra", "multimarkdown", "myst",
	}
	for _, name := range valid {
		t.Run(name, func(t *testing.T) {
			r := &Rule{}
			err := r.ApplySettings(map[string]any{"flavor": name})
			require.NoError(t, err)
			assert.Equal(t, name, r.Flavor.String())
		})
	}
}

// TestRuleFlavorAnySilencesAllDiagnostics verifies that `flavor: any`
// never emits a diagnostic, regardless of what features the document
// uses. That matches the explicit "disable flavor reporting" contract
// promised in the doc.
func TestRuleFlavorAnySilencesAllDiagnostics(t *testing.T) {
	r := &Rule{}
	require.NoError(t, r.ApplySettings(map[string]any{"flavor": "any"}))

	src := "# Head {#top}\n\n- [ ] task\n\n| a | b |\n| - | - |\n| 1 | 2 |\n\n" +
		"~~old~~ https://example.com\n\nE = mc^2^ and H~2~O.\n\n" +
		"$x+1$ inline and\n\n$$\nx\n$$\n\n" +
		"*[API]: Application Programming Interface\n\nUse API here.\n"
	diags := r.Check(mkFile(t, src))
	assert.Empty(t, diags,
		"flavor: any must not flag any tracked feature")
}

// TestRuleFlavorPHPExtra exercises the PHP Markdown Extra support
// set: footnotes and abbreviations are accepted, GFM features and
// math are not.
func TestRuleFlavorPHPExtra(t *testing.T) {
	r := &Rule{}
	require.NoError(t, r.ApplySettings(map[string]any{"flavor": "phpextra"}))

	src := "*[API]: Application Programming Interface\n\n" +
		"Use API here.[^1]\n\n[^1]: note\n\n~~strike~~ and $x+1$.\n"
	diags := r.Check(mkFile(t, src))

	byMsg := map[string]bool{}
	for _, d := range diags {
		byMsg[d.Message] = true
	}
	assert.False(t, byMsg["phpextra does not interpret footnotes as a feature"],
		"phpextra accepts footnotes")
	assert.False(t, byMsg["phpextra does not interpret abbreviations as a feature"],
		"phpextra accepts abbreviations")
	assert.True(t, byMsg["phpextra does not interpret strikethrough as a feature"],
		"phpextra rejects strikethrough")
	assert.True(t, byMsg["phpextra does not interpret inline math as a feature"],
		"phpextra rejects inline math")
}

func TestRuleApplySettingsInvalid(t *testing.T) {
	tests := []struct {
		name     string
		settings map[string]any
	}{
		{"unknown key", map[string]any{"unknown": "x"}},
		{"bad flavor", map[string]any{"flavor": "markdown"}},
		{"non-string flavor", map[string]any{"flavor": 42}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := &Rule{}
			err := r.ApplySettings(tc.settings)
			assert.Error(t, err)
		})
	}
}

func TestRuleCheckNoFlavorConfigured(t *testing.T) {
	// When the rule is enabled but no flavor is set (empty default),
	// Check must be a no-op rather than flagging everything.
	r := &Rule{}
	require.NoError(t, r.ApplySettings(r.DefaultSettings()))
	f := mkFile(t, "# Hi {#id}\n\n| a |\n| - |\n")
	assert.Empty(t, r.Check(f))
}

func TestRuleCheckCommonMark(t *testing.T) {
	r := &Rule{}
	require.NoError(t, r.ApplySettings(map[string]any{"flavor": "commonmark"}))

	src := "# Head {#top}\n\n- [ ] task\n\n| a | b |\n| - | - |\n| 1 | 2 |\n\n" +
		"~~old~~ https://example.com\n"
	diags := r.Check(mkFile(t, src))

	got := make(map[string]bool)
	for _, d := range diags {
		got[d.Message] = true
	}
	assert.True(t, got["commonmark does not interpret heading IDs as a feature"])
	assert.True(t, got["commonmark does not interpret task lists as a feature"])
	assert.True(t, got["commonmark does not interpret tables as a feature"])
	assert.True(t, got["commonmark does not interpret strikethrough as a feature"])
	assert.True(t, got["commonmark does not interpret bare-URL autolinks as a feature"])
}

func TestRuleCheckGFM(t *testing.T) {
	r := &Rule{}
	require.NoError(t, r.ApplySettings(map[string]any{"flavor": "gfm"}))

	src := "# Head {#top}\n\n- [ ] task\n\n| a | b |\n| - | - |\n| 1 | 2 |\n\n" +
		"~~old~~ https://example.com\n"
	diags := r.Check(mkFile(t, src))

	// GFM accepts tables, task lists, strikethrough, bare-URL autolinks.
	for _, d := range diags {
		assert.NotContains(t, d.Message, "tables")
		assert.NotContains(t, d.Message, "task lists")
		assert.NotContains(t, d.Message, "strikethrough")
		assert.NotContains(t, d.Message, "bare-URL autolinks")
	}

	// GFM rejects heading IDs.
	found := false
	for _, d := range diags {
		if d.Message == "gfm does not interpret heading IDs as a feature" {
			found = true
		}
	}
	assert.True(t, found, "GFM should flag heading IDs")
}

func TestRuleCheckGoldmark(t *testing.T) {
	r := &Rule{}
	require.NoError(t, r.ApplySettings(map[string]any{"flavor": "goldmark"}))

	src := "# Head {#top}\n\n- [ ] task\n\n| a | b |\n| - | - |\n| 1 | 2 |\n\n" +
		"~~old~~ https://example.com\n"
	diags := r.Check(mkFile(t, src))

	// goldmark accepts tables, task lists, strikethrough, bare URLs,
	// AND heading IDs. The sample should produce no diagnostics.
	for _, d := range diags {
		t.Errorf("unexpected diagnostic for goldmark flavor: %s", d.Message)
	}
}

func TestRuleDiagnosticFields(t *testing.T) {
	r := &Rule{}
	require.NoError(t, r.ApplySettings(map[string]any{"flavor": "commonmark"}))
	f := mkFile(t, "| a | b |\n| - | - |\n| 1 | 2 |\n")
	diags := r.Check(f)
	require.Len(t, diags, 1)
	d := diags[0]
	assert.Equal(t, "MDS034", d.RuleID)
	assert.Equal(t, "markdown-flavor", d.RuleName)
	assert.Equal(t, lint.Warning, d.Severity)
	assert.Equal(t, 1, d.Line)
	assert.Equal(t, 1, d.Column)
}

func TestRuleFootnotesDiagnostic(t *testing.T) {
	r := &Rule{}
	require.NoError(t, r.ApplySettings(map[string]any{"flavor": "gfm"}))
	f := mkFile(t, "Text.[^1]\n\n[^1]: note body\n")
	diags := r.Check(f)
	require.NotEmpty(t, diags)
	// First footnote-related diagnostic must name the feature.
	found := false
	for _, d := range diags {
		if d.Message == "gfm does not interpret footnotes as a feature" {
			found = true
		}
	}
	assert.True(t, found)
}

func TestRuleDefinitionListsDiagnostic(t *testing.T) {
	r := &Rule{}
	require.NoError(t, r.ApplySettings(map[string]any{"flavor": "gfm"}))
	f := mkFile(t, "term\n:   definition\n")
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Equal(t, "gfm does not interpret definition lists as a feature", diags[0].Message)
}

func TestRuleCheckGFMAcceptsAlerts(t *testing.T) {
	r := &Rule{}
	require.NoError(t, r.ApplySettings(map[string]any{"flavor": "gfm"}))
	f := mkFile(t, "> [!NOTE]\n> Something.\n")
	diags := r.Check(f)
	require.Empty(t, diags)
}

func TestRuleCheckCommonMarkRejectsAlerts(t *testing.T) {
	r := &Rule{}
	require.NoError(t, r.ApplySettings(map[string]any{"flavor": "commonmark"}))
	f := mkFile(t, "> [!NOTE]\n> Something.\n")
	diags := r.Check(f)
	found := false
	for _, d := range diags {
		if d.Message == "commonmark does not interpret github alerts as a feature" {
			found = true
		}
	}
	assert.True(t, found)
}

func TestRuleCheckGoldmarkRejectsAlerts(t *testing.T) {
	r := &Rule{}
	require.NoError(t, r.ApplySettings(map[string]any{"flavor": "goldmark"}))
	f := mkFile(t, "> [!NOTE]\n> Something.\n")
	diags := r.Check(f)
	found := false
	for _, d := range diags {
		if d.Message == "goldmark does not interpret github alerts as a feature" {
			found = true
		}
	}
	assert.True(t, found)
}

func TestRuleFixGitHubAlertsRemovesMarkerLine(t *testing.T) {
	r := &Rule{}
	require.NoError(t, r.ApplySettings(map[string]any{"flavor": "commonmark"}))
	f := mkFile(t, "> [!NOTE]\n> Something to remember.\n")
	got := r.Fix(f)
	assert.Equal(t, "> Something to remember.\n", string(got))
}

func TestRuleFixGitHubAlertsOnlyLine(t *testing.T) {
	r := &Rule{}
	require.NoError(t, r.ApplySettings(map[string]any{"flavor": "commonmark"}))
	f := mkFile(t, "> [!WARNING]\n")
	got := r.Fix(f)
	assert.Equal(t, "", string(got))
}

func TestRuleFixGitHubAlertsGFMNoChange(t *testing.T) {
	r := &Rule{}
	require.NoError(t, r.ApplySettings(map[string]any{"flavor": "gfm"}))
	src := "> [!NOTE]\n> Something.\n"
	f := mkFile(t, src)
	got := r.Fix(f)
	assert.Equal(t, src, string(got))
}

func TestRuleFixGitHubAlertsLazyContinuation(t *testing.T) {
	r := &Rule{}
	require.NoError(t, r.ApplySettings(map[string]any{"flavor": "commonmark"}))
	// Lazy continuation: second line has no "> " prefix but is still
	// part of the blockquote paragraph. Fix must add it so the line
	// stays inside a blockquote after the marker is removed.
	f := mkFile(t, "> [!NOTE]\nlazy content\n")
	got := r.Fix(f)
	assert.Equal(t, "> lazy content\n", string(got))
}

func TestRuleFixNoAlerts(t *testing.T) {
	r := &Rule{}
	require.NoError(t, r.ApplySettings(map[string]any{"flavor": "commonmark"}))
	// A plain blockquote has no alert marker — Fix must return the source unchanged.
	src := "> regular blockquote\n"
	got := r.Fix(mkFile(t, src))
	assert.Equal(t, src, string(got))
}

func TestRuleFixHeadingBlockquote(t *testing.T) {
	r := &Rule{}
	require.NoError(t, r.ApplySettings(map[string]any{"flavor": "commonmark"}))
	// A blockquote containing a heading has a non-Paragraph first child;
	// Fix must treat it as a non-alert and return the source unchanged.
	src := "> # Heading\n"
	got := r.Fix(mkFile(t, src))
	assert.Equal(t, src, string(got))
}

func TestRuleCheckNestedAlert(t *testing.T) {
	r := &Rule{}
	require.NoError(t, r.ApplySettings(map[string]any{"flavor": "commonmark"}))
	// A GitHub Alert nested inside an outer blockquote is detected because
	// the walker recurses into all blockquote nodes.
	f := mkFile(t, "> > [!NOTE]\n> > Something.\n")
	diags := r.Check(f)
	found := false
	for _, d := range diags {
		if d.Message == "commonmark does not interpret github alerts as a feature" {
			found = true
		}
	}
	assert.True(t, found, "nested alert should be detected")
}

func TestRuleFixNestedAlert(t *testing.T) {
	r := &Rule{}
	require.NoError(t, r.ApplySettings(map[string]any{"flavor": "commonmark"}))
	// Fix removes the [!NOTE] marker from a nested alert; the outer
	// blockquote's non-Paragraph first child exercises the !ok guard in
	// isGitHubAlert, confirming it does not misidentify the outer bq.
	f := mkFile(t, "> > [!NOTE]\n> > nested content.\n")
	got := r.Fix(f)
	assert.Equal(t, "> > nested content.\n", string(got))
}

func TestRuleFixIndentedLazyContinuation(t *testing.T) {
	r := &Rule{}
	require.NoError(t, r.ApplySettings(map[string]any{"flavor": "commonmark"}))
	// Lazy continuation inside an indented blockquote: the continuation
	// line's leading spaces must be preserved before the re-added "> ".
	f := mkFile(t, "  > [!NOTE]\n  lazy content\n")
	got := r.Fix(f)
	assert.Equal(t, "  > lazy content\n", string(got))
}
