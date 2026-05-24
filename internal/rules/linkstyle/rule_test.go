package linkstyle

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/lint"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRuleMetadata(t *testing.T) {
	r := &Rule{}
	assert.Equal(t, "MDS068", r.ID())
	assert.Equal(t, "link-style", r.Name())
	assert.Equal(t, "link", r.Category())
	assert.False(t, r.EnabledByDefault())
}

func TestCheck_DisabledWhenStyleEmpty(t *testing.T) {
	src := "# Doc\n\nSee [x](/abs/target.md).\n"
	f := newFile(t, src)
	diags := (&Rule{}).Check(f)
	assert.Empty(t, diags)
}

func TestCheck_PathRelative_FlagsAbsoluteTarget(t *testing.T) {
	src := "# Doc\n\nSee [x](/docs/target.md).\n"
	f := newFile(t, src)
	r := &Rule{Links: LinksConfig{Style: StyleConfig{Path: "relative"}}}
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "absolute")
	assert.Equal(t, "MDS068", diags[0].RuleID)
}

func TestCheck_PathRelative_AcceptsRelativeTarget(t *testing.T) {
	src := "# Doc\n\nSee [x](sub/target.md).\n\nSee [y](../up/target.md).\n"
	f := newFile(t, src)
	r := &Rule{Links: LinksConfig{Style: StyleConfig{Path: "relative"}}}
	diags := r.Check(f)
	assert.Empty(t, diags)
}

func TestCheck_PathAbsolute_FlagsRelativeTarget(t *testing.T) {
	src := "# Doc\n\nSee [x](sub/target.md).\n"
	f := newFile(t, src)
	r := &Rule{Links: LinksConfig{Style: StyleConfig{Path: "absolute"}}}
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "relative")
}

func TestCheck_PathAbsolute_AcceptsAbsoluteTarget(t *testing.T) {
	src := "# Doc\n\nSee [x](/docs/target.md).\n"
	f := newFile(t, src)
	r := &Rule{Links: LinksConfig{Style: StyleConfig{Path: "absolute"}}}
	diags := r.Check(f)
	assert.Empty(t, diags)
}

func TestCheck_ExtensionStrip_FlagsMDSuffix(t *testing.T) {
	src := "# Doc\n\nSee [x](sub/target.md).\n"
	f := newFile(t, src)
	r := &Rule{Links: LinksConfig{Style: StyleConfig{Extension: "strip"}}}
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "markdown extension")
	assert.Contains(t, diags[0].Message, "strip")
}

func TestCheck_ExtensionStrip_FlagsMarkdownSuffix(t *testing.T) {
	// `.markdown` is treated the same as `.md` — both are Markdown
	// targets under the extension policy.
	src := "# Doc\n\nSee [x](sub/target.markdown).\n"
	f := newFile(t, src)
	r := &Rule{Links: LinksConfig{Style: StyleConfig{Extension: "strip"}}}
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, ".markdown")
}

func TestCheck_ExtensionStrip_AcceptsExtensionless(t *testing.T) {
	src := "# Doc\n\nSee [x](sub/target).\n"
	f := newFile(t, src)
	r := &Rule{Links: LinksConfig{Style: StyleConfig{Extension: "strip"}}}
	diags := r.Check(f)
	assert.Empty(t, diags)
}

func TestCheck_ExtensionKeep_FlagsExtensionless(t *testing.T) {
	src := "# Doc\n\nSee [x](sub/target).\n"
	f := newFile(t, src)
	r := &Rule{Links: LinksConfig{Style: StyleConfig{Extension: "keep"}}}
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "keep")
}

func TestCheck_ExtensionKeep_AcceptsMDSuffix(t *testing.T) {
	src := "# Doc\n\nSee [x](sub/target.md).\n"
	f := newFile(t, src)
	r := &Rule{Links: LinksConfig{Style: StyleConfig{Extension: "keep"}}}
	diags := r.Check(f)
	assert.Empty(t, diags)
}

func TestCheck_ExtensionKeep_AcceptsMarkdownSuffix(t *testing.T) {
	src := "# Doc\n\nSee [x](sub/target.markdown).\n"
	f := newFile(t, src)
	r := &Rule{Links: LinksConfig{Style: StyleConfig{Extension: "keep"}}}
	diags := r.Check(f)
	assert.Empty(t, diags)
}

func TestCheck_ExtensionIgnoresNonMarkdownExtensions(t *testing.T) {
	// `.png`, `.css`, etc. are not Markdown links — the extension
	// policy should silently ignore them regardless of keep/strip.
	src := "# Doc\n\n![logo](images/logo.png)\n\nSee [stylesheet](theme.css).\n"
	f := newFile(t, src)

	r := &Rule{Links: LinksConfig{Style: StyleConfig{Extension: "keep"}}}
	assert.Empty(t, r.Check(f), "keep should not flag .png or .css targets")

	r = &Rule{Links: LinksConfig{Style: StyleConfig{Extension: "strip"}}}
	assert.Empty(t, r.Check(f), "strip should not flag .png or .css targets")
}

// TestCheck_PathAndFormApplyToNonMarkdownTargets locks in the
// "path/form are not Markdown-specific" rule: a `.css` reference
// link with an absolute path under `path: relative, form: inline`
// must be flagged twice (path + form). Only the extension axis
// short-circuits non-Markdown extensions.
func TestCheck_PathAndFormApplyToNonMarkdownTargets(t *testing.T) {
	src := "# Doc\n\nSee [stylesheet][css].\n\n[css]: /theme.css\n"
	f := newFile(t, src)
	r := &Rule{Links: LinksConfig{Style: StyleConfig{
		Path:      "relative", // /theme.css is absolute — flag
		Extension: "strip",    // .css is not Markdown — silent
		Form:      "inline",   // reference-style — flag
	}}}
	diags := r.Check(f)
	require.Len(t, diags, 2, "path and form must apply to non-Markdown targets; extension must not")
}

// TestCheck_ExtensionIgnoresDirectoryTargets covers links to
// Hugo-rendered page directories: `/docs/rules/MDS027/`, `docs/`,
// `.`, `..`. They have no filename segment and must not be
// flagged as extensionless Markdown by `extension: keep`.
func TestCheck_ExtensionIgnoresDirectoryTargets(t *testing.T) {
	src := "# Doc\n\n" +
		"See [a](/docs/rules/MDS027/).\n\n" +
		"See [b](sub/).\n\n" +
		"See [c](./).\n\n" +
		"See [d](.).\n\n" +
		"See [e](..).\n"
	f := newFile(t, src)
	r := &Rule{Links: LinksConfig{Style: StyleConfig{Extension: "keep"}}}
	assert.Empty(t, r.Check(f), "directory-style targets must not be flagged by extension: keep")

	r = &Rule{Links: LinksConfig{Style: StyleConfig{Extension: "strip"}}}
	assert.Empty(t, r.Check(f), "directory-style targets must not be flagged by extension: strip either")
}

func TestCheck_FormInline_FlagsReferenceStyle(t *testing.T) {
	src := "# Doc\n\nSee [x][label].\n\n[label]: sub/target.md\n"
	f := newFile(t, src)
	r := &Rule{Links: LinksConfig{Style: StyleConfig{Form: "inline"}}}
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "reference-style")
	assert.Contains(t, diags[0].Message, "inline")
}

func TestCheck_FormInline_AcceptsInlineStyle(t *testing.T) {
	src := "# Doc\n\nSee [x](sub/target.md).\n"
	f := newFile(t, src)
	r := &Rule{Links: LinksConfig{Style: StyleConfig{Form: "inline"}}}
	diags := r.Check(f)
	assert.Empty(t, diags)
}

func TestCheck_FormReference_FlagsInlineStyle(t *testing.T) {
	src := "# Doc\n\nSee [x](sub/target.md).\n"
	f := newFile(t, src)
	r := &Rule{Links: LinksConfig{Style: StyleConfig{Form: "reference"}}}
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "inline")
	assert.Contains(t, diags[0].Message, "reference")
}

func TestCheck_FormAny_AcceptsBoth(t *testing.T) {
	src := "# Doc\n\nSee [x](sub/a.md).\n\nSee [y][label].\n\n[label]: sub/b.md\n"
	f := newFile(t, src)
	r := &Rule{Links: LinksConfig{Style: StyleConfig{Form: "any"}}}
	diags := r.Check(f)
	assert.Empty(t, diags)
}

func TestCheck_IgnoresLocalAnchorOnlyLinks(t *testing.T) {
	src := "# Doc\n\nJump [up](#top).\n\n## Top\n"
	f := newFile(t, src)
	r := &Rule{Links: LinksConfig{Style: StyleConfig{Path: "relative", Extension: "keep", Form: "inline"}}}
	diags := r.Check(f)
	assert.Empty(t, diags, "anchor-only links carry no path; all three checks must be quiet")
}

func TestCheck_IgnoresExternalLinks(t *testing.T) {
	src := "# Doc\n\nSee [x](https://example.com/page).\n"
	f := newFile(t, src)
	r := &Rule{Links: LinksConfig{Style: StyleConfig{Path: "relative", Extension: "keep", Form: "inline"}}}
	diags := r.Check(f)
	assert.Empty(t, diags)
}

func TestCheck_AllThreeAxesEmitSeparateDiagnostics(t *testing.T) {
	// One link that violates all three policies emits three diagnostics.
	// Confirms each axis is independent.
	src := "# Doc\n\nSee [x][label].\n\n[label]: /docs/target.md\n"
	f := newFile(t, src)
	r := &Rule{Links: LinksConfig{Style: StyleConfig{
		Path:      "relative", // /docs/target.md is absolute — flag
		Extension: "strip",    // .md suffix — flag
		Form:      "inline",   // reference-style — flag
	}}}
	diags := r.Check(f)
	require.Len(t, diags, 3)
}

func TestApplySettings_PathStyle(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{
		"links": map[string]any{
			"style": map[string]any{"path": "relative"},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "relative", r.Links.Style.Path)
}

func TestApplySettings_ExtensionStyle(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{
		"links": map[string]any{
			"style": map[string]any{"extension": "strip"},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "strip", r.Links.Style.Extension)
}

func TestApplySettings_FormStyle(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{
		"links": map[string]any{
			"style": map[string]any{"form": "inline"},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "inline", r.Links.Style.Form)
}

func TestApplySettings_ExternalSkip(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{
		"links": map[string]any{
			"external-skip": []any{"^https?://localhost", "^http://10\\."},
		},
	})
	require.NoError(t, err)
	require.Len(t, r.Links.ExternalSkip, 2)
	assert.Equal(t, "^https?://localhost", r.Links.ExternalSkip[0])
	assert.Equal(t, "^http://10\\.", r.Links.ExternalSkip[1])
}

func TestApplySettings_ToleratesMDS027Keys(t *testing.T) {
	// A user can put a single `links:` block on a kind to configure
	// both MDS027 and MDS068; each rule must accept the other's
	// keys without erroring.
	r := &Rule{}
	err := r.ApplySettings(map[string]any{
		"links": map[string]any{
			"site-root":                "/srv",
			"validate-images":          true,
			"validate-reference-style": false,
			"style":                    map[string]any{"path": "relative"},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, "relative", r.Links.Style.Path)
}

type applyErrCase struct {
	name     string
	settings map[string]any
	wantErr  string
}

func linksWith(m map[string]any) map[string]any {
	return map[string]any{"links": m}
}

func styleWith(m map[string]any) map[string]any {
	return linksWith(map[string]any{"style": m})
}

func invalidApplyCases() []applyErrCase {
	return []applyErrCase{
		{"unknown top-level setting", map[string]any{"unknown": true}, "unknown setting"},
		{"links not a map", map[string]any{"links": "x"}, "links must be a map"},
		{"links.style not a map", linksWith(map[string]any{"style": "x"}), "links.style must be a map"},
		{"links.style.path bad value", styleWith(map[string]any{"path": "sometimes"}), "links.style.path"},
		{"links.style.extension bad value",
			styleWith(map[string]any{"extension": "keep-it-real"}), "links.style.extension"},
		{"links.style.form bad value", styleWith(map[string]any{"form": "shortcut"}), "links.style.form"},
		{"links.style unknown key", styleWith(map[string]any{"unknown": "x"}), "unknown links.style setting"},
		{"links unknown key", linksWith(map[string]any{"unknown": true}), "unknown links setting"},
		{"links.external-skip not a list", linksWith(map[string]any{"external-skip": 42}), "links.external-skip"},
		{"links.style.path non-string", styleWith(map[string]any{"path": 42}), "links.style.path"},
	}
}

func TestApplySettings_InvalidValues(t *testing.T) {
	for _, tc := range invalidApplyCases() {
		t.Run(tc.name, func(t *testing.T) {
			err := (&Rule{}).ApplySettings(tc.settings)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
		})
	}
}

func TestDefaultSettings(t *testing.T) {
	r := &Rule{}
	ds := r.DefaultSettings()
	links, ok := ds["links"].(map[string]any)
	require.True(t, ok)

	style, ok := links["style"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "", style["path"])
	assert.Equal(t, "", style["extension"])
	assert.Equal(t, "", style["form"])

	skip, ok := links["external-skip"].([]string)
	require.True(t, ok)
	assert.Empty(t, skip)
}

func TestApplyDefaultSettingsLeavesRuleQuiet(t *testing.T) {
	// Round-trip: defaults applied to an empty rule must keep the
	// rule silent — every check is opt-in via an explicit policy.
	r := &Rule{}
	err := r.ApplySettings(r.DefaultSettings())
	require.NoError(t, err)

	src := "# Doc\n\nSee [x][label].\n\n[label]: /docs/target.md\n"
	f := newFile(t, src)
	assert.Empty(t, r.Check(f))
}

// TestPerKindOverride_FlipsVerdict exercises the layered config
// deep-merge: a base sets `links.style.form: any` at the rules:
// level, then two kinds override it. The `rule-readme` kind pins
// `form: inline` so a reference-style link is flagged for files
// in that kind; the `docs` kind leaves the base untouched, so the
// same link passes. Task 3 of plan 172.
func TestPerKindOverride_FlipsVerdict(t *testing.T) {
	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"link-style": {
				Enabled: true,
				Settings: map[string]any{
					"links": map[string]any{
						"style": map[string]any{"form": "any"},
					},
				},
			},
		},
		Kinds: map[string]config.KindBody{
			"rule-readme": {Rules: map[string]config.RuleCfg{
				"link-style": {Enabled: true, Settings: map[string]any{
					"links": map[string]any{
						"style": map[string]any{"form": "inline"},
					},
				}},
			}},
			"docs": {Rules: map[string]config.RuleCfg{}},
		},
		KindAssignment: []config.KindAssignmentEntry{
			{Files: []string{"rules/**/README.md"}, Kinds: []string{"rule-readme"}},
			{Files: []string{"docs/**/*.md"}, Kinds: []string{"docs"}},
		},
	}

	src := "# Doc\n\nSee [x][label].\n\n[label]: sub/target.md\n"
	f := newFile(t, src)

	readmeRules := config.Effective(cfg, "rules/foo/README.md", nil, nil)
	rA := &Rule{}
	require.NoError(t, rA.ApplySettings(readmeRules["link-style"].Settings))
	assert.Equal(t, "inline", rA.Links.Style.Form,
		"rule-readme kind must override base form: any with form: inline")
	diags := rA.Check(f)
	require.Len(t, diags, 1, "rule-readme override (form: inline) must flag the ref link")
	assert.Contains(t, diags[0].Message, "reference-style")

	docsRules := config.Effective(cfg, "docs/guides/foo.md", nil, nil)
	rB := &Rule{}
	require.NoError(t, rB.ApplySettings(docsRules["link-style"].Settings))
	// ApplySettings normalizes "any" to "" so the runtime no-op
	// fast path stays cheap; the docs kind effectively disables
	// the form check the same way an empty string would.
	assert.Equal(t, "", rB.Links.Style.Form,
		"docs kind inherits base form: any, normalized to \"\"")
	diags = rB.Check(f)
	assert.Empty(t, diags, "docs kind (form: any inherited) must pass")
}

// TestApplySettings_NormalizesAnyToEmptyString ensures the
// runtime no-op fast path (all-three-empty-strings) covers
// users who explicitly set `form: any`. Without this, an
// otherwise-unset rule still walks the AST and allocates link
// slices on every Check.
func TestApplySettings_NormalizesAnyToEmptyString(t *testing.T) {
	r := &Rule{}
	require.NoError(t, r.ApplySettings(styleWith(map[string]any{"form": "any"})))
	assert.Equal(t, "", r.Links.Style.Form,
		"`form: any` must normalize to \"\" so Check's fast path applies")
}

func newFile(t *testing.T, src string) *lint.File {
	t.Helper()
	f, err := lint.NewFile("doc.md", []byte(src))
	require.NoError(t, err)
	return f
}
