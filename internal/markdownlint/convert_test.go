package markdownlint

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/jeduden/mdsmith/internal/rules"
	_ "github.com/jeduden/mdsmith/internal/rules/all"
)

func mustConvert(t *testing.T, yamlSrc string) *Conversion {
	t.Helper()
	raw, err := Parse([]byte(yamlSrc))
	require.NoError(t, err)
	conv, err := Convert(raw)
	require.NoError(t, err)
	return conv
}

func noteWith(notes []string, substr string) bool {
	for _, n := range notes {
		if strings.Contains(n, substr) {
			return true
		}
	}
	return false
}

func TestConvert(t *testing.T) {
	conv := mustConvert(t, `
MD013:
  line_length: 100
  stern: true
MD012:
  maximum: 2
MD007:
  indent: 4
MD041:
  level: 2
MD030:
  ul_single: 2
  ol_multi: 3
`)
	assert.Equal(t, config.RuleCfg{Enabled: true, Settings: map[string]any{
		"max": 100, "stern": true,
	}}, conv.Rules["line-length"])
	assert.Equal(t, config.RuleCfg{Enabled: true, Settings: map[string]any{
		"max": 2,
	}}, conv.Rules["no-multiple-blanks"])
	assert.Equal(t, config.RuleCfg{Enabled: true, Settings: map[string]any{
		"spaces": 4,
	}}, conv.Rules["list-indent"])
	assert.Equal(t, config.RuleCfg{Enabled: true, Settings: map[string]any{
		"level": 2,
	}}, conv.Rules["first-line-heading"])
	assert.Equal(t, config.RuleCfg{Enabled: true, Settings: map[string]any{
		"ul-single": 2, "ol-multi": 3,
	}}, conv.Rules["list-marker-space"])
}

func TestConvert_EnableDisable(t *testing.T) {
	t.Run("disabling emits false for default-on rules only", func(t *testing.T) {
		conv := mustConvert(t, "MD041: false\nMD033: false\n")
		// first-line-heading is on by default in mdsmith: emit false.
		assert.Equal(t, config.RuleCfg{Enabled: false}, conv.Rules["first-line-heading"])
		// no-inline-html is already opt-in: nothing to emit.
		_, ok := conv.Rules["no-inline-html"]
		assert.False(t, ok)
	})

	t.Run("enabling an opt-in mdsmith rule emits true", func(t *testing.T) {
		conv := mustConvert(t, "MD025: true\n")
		assert.Equal(t, config.RuleCfg{Enabled: true}, conv.Rules["single-h1"])
	})

	t.Run("no-op enable of a default-on rule emits nothing", func(t *testing.T) {
		conv := mustConvert(t, "MD013: true\n")
		_, ok := conv.Rules["line-length"]
		assert.False(t, ok)
	})

	t.Run("alias names resolve like ids", func(t *testing.T) {
		conv := mustConvert(t, "line-length:\n  line_length: 120\nul-style:\n  style: asterisk\n")
		assert.Equal(t, config.RuleCfg{Enabled: true, Settings: map[string]any{
			"max": 120,
		}}, conv.Rules["line-length"])
		assert.Equal(t, config.RuleCfg{Enabled: true, Settings: map[string]any{
			"style": "asterisk",
		}}, conv.Rules["list-marker-style"])
	})
}

func TestConvert_StyleEnums(t *testing.T) {
	conv := mustConvert(t, `
MD003: {style: setext}
MD029: {style: one}
MD046: {style: consistent}
MD048: {style: tilde}
MD049: {style: underscore}
MD050: {style: asterisk}
MD055: {style: leading_and_trailing}
`)
	assert.Equal(t, map[string]any{"style": "setext"}, conv.Rules["heading-style"].Settings)
	assert.Equal(t, map[string]any{"style": "all-ones"}, conv.Rules["ordered-list-numbering"].Settings)
	assert.Equal(t, map[string]any{"style": "consistent"}, conv.Rules["code-block-style"].Settings)
	assert.Equal(t, map[string]any{"style": "tilde"}, conv.Rules["fenced-code-style"].Settings)
	// MD049 and MD050 land on the same mdsmith rule.
	assert.Equal(t, map[string]any{"italic": "underscore", "bold": "asterisk"},
		conv.Rules["emphasis-style"].Settings)
	assert.Equal(t, map[string]any{"style": "leading_and_trailing"},
		conv.Rules["table-format"].Settings)
}

func TestConvert_UntranslatableStyles(t *testing.T) {
	conv := mustConvert(t, "MD003: {style: consistent}\nMD048: {style: consistent}\n")
	_, ok := conv.Rules["heading-style"]
	assert.False(t, ok, "no settings survived; heading-style is default-on, so no entry")
	assert.True(t, noteWith(conv.Notes, `MD003 option "style"`), "notes: %v", conv.Notes)
	assert.True(t, noteWith(conv.Notes, `MD048 option "style"`), "notes: %v", conv.Notes)
}

func TestConvert_OptionEdgeCases(t *testing.T) {
	t.Run("md013 toggles rebuild exclude", func(t *testing.T) {
		conv := mustConvert(t, "MD013: {code_blocks: true, line_length: 90}\n")
		assert.Equal(t, map[string]any{
			"max":     90,
			"exclude": []string{"tables", "urls"},
		}, conv.Rules["line-length"].Settings)
	})

	t.Run("md035 literal style translates", func(t *testing.T) {
		conv := mustConvert(t, "MD035: {style: \"***\"}\n")
		assert.Equal(t, map[string]any{"style": "asterisk", "length": 3},
			conv.Rules["horizontal-rule-style"].Settings)
	})

	t.Run("md044 proper names translate", func(t *testing.T) {
		conv := mustConvert(t, "MD044: {names: [mdsmith, GitHub], code_blocks: true}\n")
		assert.Equal(t, map[string]any{
			"names": []string{"mdsmith", "GitHub"}, "check-code": true,
		}, conv.Rules["proper-names"].Settings)
	})

	t.Run("md052 shortcut syntax translates both ways", func(t *testing.T) {
		conv := mustConvert(t, "MD052: {shortcut_syntax: true}\n")
		assert.Equal(t, map[string]any{"shortcut": "always"},
			conv.Rules["no-undefined-reference-labels"].Settings)

		conv = mustConvert(t, "MD052: {shortcut_syntax: false}\n")
		assert.Equal(t, map[string]any{"shortcut": "collapsed-only"},
			conv.Rules["no-undefined-reference-labels"].Settings)
	})

	t.Run("md025 front matter title translates only empty", func(t *testing.T) {
		conv := mustConvert(t, "MD025: {front_matter_title: \"\"}\n")
		assert.Equal(t, map[string]any{"front-matter-title": ""},
			conv.Rules["single-h1"].Settings)

		conv = mustConvert(t, "MD025: {front_matter_title: \"^title:\"}\n")
		assert.True(t, noteWith(conv.Notes, `MD025 option "front_matter_title"`),
			"notes: %v", conv.Notes)
	})
}

func TestConvert_StringListOptions(t *testing.T) {
	conv := mustConvert(t, `
MD033: {allowed_elements: [kbd, br]}
MD053: {ignored_definitions: [skip-me]}
MD059: {prohibited_texts: [click here]}
`)
	assert.Equal(t, map[string]any{"allow": []string{"kbd", "br"}},
		conv.Rules["no-inline-html"].Settings)
	assert.Equal(t, map[string]any{"ignored-labels": []string{"skip-me"}},
		conv.Rules["no-unused-link-definitions"].Settings)
	assert.Equal(t, map[string]any{"banned": []string{"click here"}},
		conv.Rules["descriptive-link-text"].Settings)
}

func TestConvert_DefaultFalse(t *testing.T) {
	conv := mustConvert(t, "default: false\nMD009: true\n")
	// Explicitly re-enabled: stays on (mdsmith default), no entry.
	_, ok := conv.Rules["no-trailing-spaces"]
	assert.False(t, ok)
	// Unmentioned mapped default-on rules are swept off.
	assert.Equal(t, config.RuleCfg{Enabled: false}, conv.Rules["line-length"])
	assert.Equal(t, config.RuleCfg{Enabled: false}, conv.Rules["heading-increment"])
	// mdsmith-only rules are untouched.
	_, ok = conv.Rules["max-file-length"]
	assert.False(t, ok)
	assert.True(t, noteWith(conv.Notes, "default: false"), "notes: %v", conv.Notes)
}

func TestConvert_PartialDisable(t *testing.T) {
	conv := mustConvert(t, "MD018: false\n")
	_, ok := conv.Rules["atx-heading-whitespace"]
	assert.False(t, ok, "rule stays at its default-on state")
	assert.True(t, noteWith(conv.Notes, "atx-heading-whitespace"), "notes: %v", conv.Notes)
	assert.True(t, noteWith(conv.Notes, "MD018"), "notes: %v", conv.Notes)
}

func TestConvert_Notes(t *testing.T) {
	t.Run("options without an equivalent", func(t *testing.T) {
		conv := mustConvert(t, "MD024: {siblings_only: true}\nMD013: {strict: true}\n")
		assert.True(t, noteWith(conv.Notes, `MD024 option "siblings_only"`), "notes: %v", conv.Notes)
		assert.True(t, noteWith(conv.Notes, `MD013 option "strict"`), "notes: %v", conv.Notes)
	})

	t.Run("opt-in gap is reported once", func(t *testing.T) {
		conv := mustConvert(t, "MD013: true\n")
		assert.True(t, noteWith(conv.Notes, "opt-in"), "notes: %v", conv.Notes)
		assert.True(t, noteWith(conv.Notes, "no-inline-html (MD033)"), "notes: %v", conv.Notes)
	})

	t.Run("explicitly handled rules leave the gap note", func(t *testing.T) {
		conv := mustConvert(t, "MD033: false\nMD025: true\n")
		assert.False(t, noteWith(conv.Notes, "no-inline-html (MD033)"), "notes: %v", conv.Notes)
		assert.False(t, noteWith(conv.Notes, "single-h1 (MD025)"), "notes: %v", conv.Notes)
	})

	t.Run("unknown keys tags and extends", func(t *testing.T) {
		conv := mustConvert(t, "extends: base.json\nwhitespace: false\nMD999: true\ncustom-plugin: false\n")
		assert.Empty(t, conv.Rules)
		assert.True(t, noteWith(conv.Notes, `"extends" is not followed`), "notes: %v", conv.Notes)
		assert.True(t, noteWith(conv.Notes, `"whitespace" is a markdownlint tag`), "notes: %v", conv.Notes)
		assert.True(t, noteWith(conv.Notes, `"MD999" has no mdsmith equivalent`), "notes: %v", conv.Notes)
		assert.True(t, noteWith(conv.Notes, `"custom-plugin" has no mdsmith equivalent`),
			"notes: %v", conv.Notes)
	})

	t.Run("schema key is ignored silently", func(t *testing.T) {
		conv := mustConvert(t, "$schema: https://example.com/schema.json\n")
		assert.Empty(t, conv.Rules)
		assert.False(t, noteWith(conv.Notes, "$schema"), "notes: %v", conv.Notes)
	})

	t.Run("wrong value types", func(t *testing.T) {
		conv := mustConvert(t, "MD013: {line_length: many}\nMD041: 3\n")
		assert.True(t, noteWith(conv.Notes, `MD013 option "line_length"`), "notes: %v", conv.Notes)
		assert.True(t, noteWith(conv.Notes, `"MD041"`), "notes: %v", conv.Notes)
	})

	t.Run("deterministic across runs", func(t *testing.T) {
		src := "MD999: true\nMD024: {siblings_only: true}\nzz-unknown: false\n"
		first := mustConvert(t, src)
		for range 5 {
			assert.Equal(t, first.Notes, mustConvert(t, src).Notes)
		}
	})
}

// TestConvert_SettingsApplyCleanly drives every option-table translation
// through the real registered rules: each emitted settings map must be
// accepted by the target rule's ApplySettings. This pins the option
// table to the live rule surface — a renamed or removed setting fails
// here instead of producing configs mdsmith rejects.
func TestConvert_SettingsApplyCleanly(t *testing.T) {
	conv := mustConvert(t, `
MD003: {style: setext}
MD004: {style: plus}
MD007: {indent: 4}
MD012: {maximum: 3}
MD013: {line_length: 120, heading_line_length: 100,
  code_block_line_length: 90, stern: true, code_blocks: false, tables: true}
MD025: {front_matter_title: ""}
MD029: {style: ordered}
MD030: {ul_single: 1, ul_multi: 1, ol_single: 1, ol_multi: 1}
MD033: {allowed_elements: [kbd]}
MD035: {style: "___"}
MD041: {level: 1}
MD044: {names: [mdsmith], code_blocks: false, html_elements: true}
MD046: {style: indented}
MD048: {style: backtick}
MD049: {style: asterisk}
MD050: {style: underscore}
MD052: {shortcut_syntax: true}
MD053: {ignored_definitions: [x]}
MD055: {style: no_leading_or_trailing}
MD059: {prohibited_texts: [here]}
`)

	byName := make(map[string]rule.Rule)
	for _, r := range rule.All() {
		byName[r.Name()] = r
	}

	applied := 0
	for name, rc := range conv.Rules {
		if len(rc.Settings) == 0 {
			continue
		}
		r, ok := byName[name]
		require.True(t, ok, "converted rule %q is not registered", name)
		c, ok := r.(rule.Configurable)
		require.True(t, ok, "converted rule %q has settings but is not Configurable", name)
		assert.NoError(t, c.ApplySettings(rc.Settings), "rule %q settings %v", name, rc.Settings)
		applied++
	}
	assert.GreaterOrEqual(t, applied, 15, "expected most converted rules to carry settings")
}

func TestConvert_DefaultKeyInvalid(t *testing.T) {
	conv := mustConvert(t, "default: 1\nMD041: false\n")
	assert.True(t, noteWith(conv.Notes, `"default" must be true or false`), "notes: %v", conv.Notes)
	// An invalid default falls back to true, so the explicit disable
	// still converts against default-on semantics.
	assert.Equal(t, config.RuleCfg{Enabled: false}, conv.Rules["first-line-heading"])
}

func TestConvert_ListRulesError(t *testing.T) {
	orig := listRules
	t.Cleanup(func() { listRules = orig })
	listRules = func() ([]rules.RuleInfo, error) { return nil, errors.New("boom") }

	_, err := Convert(map[string]any{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "loading rule metadata")
}

func TestBuildIndex_ListRulesError(t *testing.T) {
	orig := listRules
	t.Cleanup(func() { listRules = orig })
	listRules = func() ([]rules.RuleInfo, error) { return nil, errors.New("boom") }

	_, err := buildIndex()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "loading rule metadata")
}

func TestBuildIndex(t *testing.T) {
	idx, err := buildIndex()
	require.NoError(t, err)

	byID := idx.byID["MD013"]
	require.Len(t, byID, 1)
	assert.Equal(t, "line-length", byID[0].rule)
	assert.True(t, byID[0].upstreamDefault)

	alias := idx.byAlias["no-duplicate-heading"]
	require.Len(t, alias, 1)
	assert.Equal(t, "no-duplicate-headings", alias[0].rule)

	// MDS064 fans in from five markdownlint ids.
	assert.Len(t, idx.ruleMDs["atx-heading-whitespace"], 5)
}

func TestParseMD035Style(t *testing.T) {
	tests := []struct {
		in      string
		style   string
		length  int
		wantErr bool
	}{
		{in: "---", style: "dash", length: 3},
		{in: "*****", style: "asterisk", length: 5},
		{in: "___", style: "underscore", length: 3},
		{in: "consistent", wantErr: true},
		{in: "-*-", wantErr: true},
		{in: "--", wantErr: true},
		{in: "===", wantErr: true},
	}
	for _, tt := range tests {
		style, length, ok := parseMD035Style(tt.in)
		if tt.wantErr {
			assert.False(t, ok, "input %q", tt.in)
			continue
		}
		require.True(t, ok, "input %q", tt.in)
		assert.Equal(t, tt.style, style, "input %q", tt.in)
		assert.Equal(t, tt.length, length, "input %q", tt.in)
	}
}

func TestAsInt(t *testing.T) {
	v, ok := asInt(7)
	assert.True(t, ok)
	assert.Equal(t, 7, v)

	v, ok = asInt(float64(7))
	assert.True(t, ok)
	assert.Equal(t, 7, v)

	_, ok = asInt(7.5)
	assert.False(t, ok)

	_, ok = asInt("7")
	assert.False(t, ok)
}

func TestAsStringSlice(t *testing.T) {
	v, ok := asStringSlice([]any{"a", "b"})
	assert.True(t, ok)
	assert.Equal(t, []string{"a", "b"}, v)

	_, ok = asStringSlice([]any{"a", 1})
	assert.False(t, ok)

	_, ok = asStringSlice("a")
	assert.False(t, ok)
}
