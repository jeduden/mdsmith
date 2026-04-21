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
	assert.False(t, byMsg["footnotes are not supported by phpextra"],
		"phpextra accepts footnotes")
	assert.False(t, byMsg["abbreviations are not supported by phpextra"],
		"phpextra accepts abbreviations")
	assert.True(t, byMsg["strikethrough is not supported by phpextra"],
		"phpextra rejects strikethrough")
	assert.True(t, byMsg["inline math is not supported by phpextra"],
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
	assert.True(t, got["heading IDs are not supported by commonmark"])
	assert.True(t, got["task lists are not supported by commonmark"])
	assert.True(t, got["tables are not supported by commonmark"])
	assert.True(t, got["strikethrough is not supported by commonmark"])
	assert.True(t, got["bare-URL autolinks are not supported by commonmark"])
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
		if d.Message == "heading IDs are not supported by gfm" {
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
		if d.Message == "footnotes are not supported by gfm" {
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
	assert.Equal(t, "definition lists are not supported by gfm", diags[0].Message)
}
