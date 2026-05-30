package linkstyle

import (
	"testing"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"

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
		{"links.style.extension non-string",
			styleWith(map[string]any{"extension": 42}), "links.style.extension"},
		{"links.style.form non-string", styleWith(map[string]any{"form": 42}), "links.style.form"},
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

// --- link-image-style axis (MD054 parity) ---

// TestApplySettings_LinkImageStyle_Parses verifies that every MD054
// toggle name is accepted by ApplySettings.
func TestApplySettings_LinkImageStyle_Parses(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(styleWith(map[string]any{
		"link-image-style": map[string]any{
			"autolink":     false,
			"inline":       true,
			"full":         true,
			"collapsed":    true,
			"shortcut":     true,
			"inline-image": true,
		},
	}))
	require.NoError(t, err)
	lis := r.Links.Style.LinkImageStyle
	assert.True(t, lis.Active, "configuring link-image-style must mark the axis Active")
	assert.False(t, lis.Autolink, "autolink=false must be stored")
	assert.True(t, lis.Inline, "inline=true must be stored")
	assert.True(t, lis.Full, "full=true must be stored")
	assert.True(t, lis.Collapsed, "collapsed=true must be stored")
	assert.True(t, lis.Shortcut, "shortcut=true must be stored")
	assert.True(t, lis.InlineImage, "inline-image=true must be stored")
}

// TestApplySettings_LinkImageStyle_DefaultsAllowsEverything confirms
// that a rule enabled with no link-image-style config emits no
// diagnostics — matches markdownlint's default behaviour.
func TestApplySettings_LinkImageStyle_DefaultsAllowsEverything(t *testing.T) {
	r := &Rule{}
	require.NoError(t, r.ApplySettings(r.DefaultSettings()))

	src := "# Doc\n\n" +
		"Autolink: <https://example.com>.\n\n" +
		"Inline: [text](target.md).\n\n" +
		"Full: [text][label].\n\n" +
		"Collapsed: [label][].\n\n" +
		"Shortcut: [label].\n\n" +
		"Image: ![alt](img.png).\n\n" +
		"[label]: target.md\n"
	f := newFile(t, src)
	assert.Empty(t, r.Check(f), "default settings must not flag any link or image style")
}

// TestCheck_LinkImageStyle_ForbidAutolink verifies that autolink:false
// flags <url> nodes.
func TestCheck_LinkImageStyle_ForbidAutolink(t *testing.T) {
	src := "# Doc\n\nSee <https://example.com>.\n"
	f := newFile(t, src)
	r := &Rule{Links: LinksConfig{Style: StyleConfig{
		LinkImageStyle: LinkImageStyleConfig{
			Active:   true,
			Autolink: false,
			Inline:   true, Full: true, Collapsed: true, Shortcut: true, InlineImage: true,
		},
	}}}
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "autolink")
	assert.Equal(t, "MDS068", diags[0].RuleID)
}

// TestCheck_LinkImageStyle_AllowAutolink verifies that autolink:true
// passes <url> nodes.
func TestCheck_LinkImageStyle_AllowAutolink(t *testing.T) {
	src := "# Doc\n\nSee <https://example.com>.\n"
	f := newFile(t, src)
	r := &Rule{Links: LinksConfig{Style: StyleConfig{
		LinkImageStyle: LinkImageStyleConfig{
			Active:   true,
			Autolink: true,
			Inline:   true, Full: true, Collapsed: true, Shortcut: true, InlineImage: true,
		},
	}}}
	diags := r.Check(f)
	assert.Empty(t, diags)
}

// TestCheck_LinkImageStyle_ForbidInline verifies that inline:false
// flags [text](url) inline links.
func TestCheck_LinkImageStyle_ForbidInline(t *testing.T) {
	src := "# Doc\n\nSee [text](target.md).\n"
	f := newFile(t, src)
	r := &Rule{Links: LinksConfig{Style: StyleConfig{
		LinkImageStyle: LinkImageStyleConfig{
			Active:   true,
			Autolink: true,
			Inline:   false,
			Full:     true, Collapsed: true, Shortcut: true, InlineImage: true,
		},
	}}}
	diags := r.Check(f)
	require.Len(t, diags, 1)
	// "inline style forbidden" — not "inline-image style forbidden" —
	// so an isImage routing inversion for links would be caught.
	assert.Contains(t, diags[0].Message, "inline style forbidden")
}

// TestCheck_LinkImageStyle_ForbidFull verifies that full:false flags
// [text][label] full reference links.
func TestCheck_LinkImageStyle_ForbidFull(t *testing.T) {
	src := "# Doc\n\nSee [text][label].\n\n[label]: target.md\n"
	f := newFile(t, src)
	r := &Rule{Links: LinksConfig{Style: StyleConfig{
		LinkImageStyle: LinkImageStyleConfig{
			Active:   true,
			Autolink: true, Inline: true,
			Full:      false,
			Collapsed: true, Shortcut: true, InlineImage: true,
		},
	}}}
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "full")
}

// TestCheck_LinkImageStyle_ForbidCollapsed verifies that collapsed:false
// flags [label][] collapsed reference links.
func TestCheck_LinkImageStyle_ForbidCollapsed(t *testing.T) {
	src := "# Doc\n\nSee [label][].\n\n[label]: target.md\n"
	f := newFile(t, src)
	r := &Rule{Links: LinksConfig{Style: StyleConfig{
		LinkImageStyle: LinkImageStyleConfig{
			Active:   true,
			Autolink: true, Inline: true, Full: true,
			Collapsed: false,
			Shortcut:  true, InlineImage: true,
		},
	}}}
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "collapsed")
}

// TestCheck_LinkImageStyle_ForbidShortcut verifies that shortcut:false
// flags [label] shortcut reference links.
func TestCheck_LinkImageStyle_ForbidShortcut(t *testing.T) {
	src := "# Doc\n\nSee [label].\n\n[label]: target.md\n"
	f := newFile(t, src)
	r := &Rule{Links: LinksConfig{Style: StyleConfig{
		LinkImageStyle: LinkImageStyleConfig{
			Active:   true,
			Autolink: true, Inline: true, Full: true, Collapsed: true,
			Shortcut:    false,
			InlineImage: true,
		},
	}}}
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "shortcut")
}

// TestCheck_LinkImageStyle_ForbidInlineImage verifies that
// inline-image:false flags ![alt](src) inline images.
func TestCheck_LinkImageStyle_ForbidInlineImage(t *testing.T) {
	src := "# Doc\n\n![alt](img.png)\n"
	f := newFile(t, src)
	r := &Rule{Links: LinksConfig{Style: StyleConfig{
		LinkImageStyle: LinkImageStyleConfig{
			Active:   true,
			Autolink: true, Inline: true, Full: true, Collapsed: true, Shortcut: true,
			InlineImage: false,
		},
	}}}
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "inline-image")
}

// TestCheck_LinkImageStyle_InactiveWhenNotConfigured verifies that
// when link-image-style is not configured (Active=false), no
// diagnostics are emitted even for links that would otherwise fail.
func TestCheck_LinkImageStyle_InactiveWhenNotConfigured(t *testing.T) {
	src := "# Doc\n\nSee <https://example.com>.\n"
	f := newFile(t, src)
	r := &Rule{} // Active defaults to false
	diags := r.Check(f)
	assert.Empty(t, diags)
}

// TestApplySettings_LinkImageStyle_BadValue verifies that a non-bool
// toggle value returns an error.
func TestApplySettings_LinkImageStyle_BadValue(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(styleWith(map[string]any{
		"link-image-style": map[string]any{
			"inline": "yes",
		},
	}))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "inline")
}

// TestApplySettings_LinkImageStyle_UnknownKey verifies that an
// unrecognised toggle name returns an error.
func TestApplySettings_LinkImageStyle_UnknownKey(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(styleWith(map[string]any{
		"link-image-style": map[string]any{
			"unknown-toggle": true,
		},
	}))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown")
}

// TestApplySettings_LinkImageStyle_NotAMap verifies that a non-map
// value for link-image-style returns an error.
func TestApplySettings_LinkImageStyle_NotAMap(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(styleWith(map[string]any{
		"link-image-style": "all",
	}))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "link-image-style")
}

// TestCheck_LinkImageStyle_IndependentOfFormAxis verifies that the
// new link-image-style axis and the legacy form axis are independent.
// Both can be active simultaneously; violations are separate.
func TestCheck_LinkImageStyle_IndependentOfFormAxis(t *testing.T) {
	src := "# Doc\n\nSee [text][label].\n\n[label]: target.md\n"
	f := newFile(t, src)
	// form: inline forbids reference-style links (old axis).
	// link-image-style: full=false also forbids [text][label] (new axis).
	// Two separate diagnostics should be emitted.
	r := &Rule{Links: LinksConfig{Style: StyleConfig{
		Form: "inline",
		LinkImageStyle: LinkImageStyleConfig{
			Active:   true,
			Autolink: true, Inline: true,
			Full:      false,
			Collapsed: true, Shortcut: true, InlineImage: true,
		},
	}}}
	diags := r.Check(f)
	require.Len(t, diags, 2, "form and link-image-style axes must emit separate diagnostics")
}

// --- link-image-style: allowed paths and position-resolution edges ---

// TestCheck_LinkImageStyle_ActiveAllowsAllForms exercises the
// "allowed" return paths of linkImageStyleMsg: with the axis active
// and every toggle true, the walk must visit each link/image form
// (inline, full, collapsed, shortcut, autolink, image) and emit
// nothing.
func TestCheck_LinkImageStyle_ActiveAllowsAllForms(t *testing.T) {
	src := "# Doc\n\n" +
		"Autolink <https://example.com>.\n\n" +
		"Inline [text](target.md).\n\n" +
		"Full [text][label].\n\n" +
		"Collapsed [label][].\n\n" +
		"Shortcut [label].\n\n" +
		"Image ![alt](img.png).\n\n" +
		"[label]: target.md\n"
	f := newFile(t, src)
	r := &Rule{Links: LinksConfig{Style: StyleConfig{
		LinkImageStyle: LinkImageStyleConfig{
			Active:   true,
			Autolink: true, Inline: true, Full: true,
			Collapsed: true, Shortcut: true, InlineImage: true,
		},
	}}}
	assert.Empty(t, r.Check(f), "active axis with every toggle allowed must stay silent")
}

// TestCheck_LinkImageStyle_NilFileReturnsNil locks in the AST-walk
// guard: a nil file or a file with a nil AST must short-circuit
// rather than panic.
func TestCheck_LinkImageStyle_NilFileReturnsNil(t *testing.T) {
	r := &Rule{}
	assert.Nil(t, r.checkLinkImageStyle(nil))
	assert.Nil(t, r.checkLinkImageStyle(&lint.File{}))
}

// TestCheck_LinkImageStyle_AutolinkInsideEmphasis covers the position
// search skipping an inline parent: emphasis carries no source lines,
// so autolinkPosition must walk past it and find the autolink on the
// enclosing block (line 3).
func TestCheck_LinkImageStyle_AutolinkInsideEmphasis(t *testing.T) {
	src := "# Doc\n\nText *<https://example.com>* more.\n"
	f := newFile(t, src)
	r := &Rule{Links: LinksConfig{Style: StyleConfig{
		LinkImageStyle: LinkImageStyleConfig{
			Active: true, Autolink: false,
			Inline: true, Full: true, Collapsed: true, Shortcut: true, InlineImage: true,
		},
	}}}
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "autolink")
	assert.Equal(t, 3, diags[0].Line, "autolink line resolves via the block ancestor")
}

// TestCheck_LinkImageStyle_ForbidEmailAutolink verifies an email
// autolink is flagged and positioned at its `<`. The fork's URL()
// returns the bare address (no mailto: prefix), so the source search
// matches and resolves the real position rather than falling back.
func TestCheck_LinkImageStyle_ForbidEmailAutolink(t *testing.T) {
	src := "# Doc\n\nMail <user@example.com> now.\n"
	f := newFile(t, src)
	r := &Rule{Links: LinksConfig{Style: StyleConfig{
		LinkImageStyle: LinkImageStyleConfig{
			Active: true, Autolink: false,
			Inline: true, Full: true, Collapsed: true, Shortcut: true, InlineImage: true,
		},
	}}}
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "autolink")
	assert.Equal(t, 3, diags[0].Line)
	assert.Equal(t, 6, diags[0].Column, "position resolves to the `<` of the autolink")
}

// TestCheck_LinkImageStyle_EmptyAltImagePositionFallback covers
// linkNodePosition's no-text-child fallback: an image with empty alt
// has no Text descendant, so the position falls back to (1,1).
func TestCheck_LinkImageStyle_EmptyAltImagePositionFallback(t *testing.T) {
	src := "# Doc\n\n![](img.png)\n"
	f := newFile(t, src)
	r := &Rule{Links: LinksConfig{Style: StyleConfig{
		LinkImageStyle: LinkImageStyleConfig{
			Active:   true,
			Autolink: true, Inline: true, Full: true, Collapsed: true, Shortcut: true,
			InlineImage: false,
		},
	}}}
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "inline-image")
	assert.Equal(t, 1, diags[0].Line)
	assert.Equal(t, 1, diags[0].Column)
}

// TestAutolinkPosition_EmptyURL covers the empty-URL guard. The
// parser never yields an autolink with an empty URL, so the node is
// constructed directly; the function must return the (1,1) fallback
// without searching the source.
func TestAutolinkPosition_EmptyURL(t *testing.T) {
	f := newFile(t, "# Doc\n\nbody\n")
	al := ast.NewAutoLink(ast.AutoLinkURL, ast.NewTextSegment(text.NewSegment(0, 0)))
	line, col := autolinkPosition(f, al)
	assert.Equal(t, 1, line)
	assert.Equal(t, 1, col)
}

// TestAutolinkPosition_NotFoundFallsBack covers the fallback when the
// URL is absent from the block ancestor's source lines. The parser
// always emits an autolink whose `<url>` appears verbatim, so the node
// is constructed with an empty-lines block parent to drive the path.
func TestAutolinkPosition_NotFoundFallsBack(t *testing.T) {
	f := newFile(t, "# Doc\n\nbody\n")
	al := ast.NewAutoLink(ast.AutoLinkURL, ast.NewTextSegment(text.NewSegment(0, 4)))
	al.SetParent(ast.NewParagraph()) // block ancestor with no source lines
	line, col := autolinkPosition(f, al)
	assert.Equal(t, 1, line)
	assert.Equal(t, 1, col)
}

// TestCheck_LinkImageStyle_ReferenceImagesUseReferenceToggles verifies
// that reference-style images route to the full/collapsed/shortcut
// toggles (shared with links), not the inline-image toggle.
func TestCheck_LinkImageStyle_ReferenceImagesUseReferenceToggles(t *testing.T) {
	cases := []struct {
		name    string
		src     string
		lis     LinkImageStyleConfig
		wantMsg string
	}{
		{
			"full reference image forbidden by full",
			"# Doc\n\n![alt][label]\n\n[label]: img.png\n",
			LinkImageStyleConfig{Active: true, Autolink: true, Inline: true,
				Full: false, Collapsed: true, Shortcut: true, InlineImage: true},
			"full",
		},
		{
			"collapsed reference image forbidden by collapsed",
			"# Doc\n\n![label][]\n\n[label]: img.png\n",
			LinkImageStyleConfig{Active: true, Autolink: true, Inline: true,
				Full: true, Collapsed: false, Shortcut: true, InlineImage: true},
			"collapsed",
		},
		{
			"shortcut reference image forbidden by shortcut",
			"# Doc\n\n![label]\n\n[label]: img.png\n",
			LinkImageStyleConfig{Active: true, Autolink: true, Inline: true,
				Full: true, Collapsed: true, Shortcut: false, InlineImage: true},
			"shortcut",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFile(t, tc.src)
			r := &Rule{Links: LinksConfig{Style: StyleConfig{LinkImageStyle: tc.lis}}}
			diags := r.Check(f)
			require.Len(t, diags, 1)
			assert.Contains(t, diags[0].Message, tc.wantMsg)
		})
	}
}

// TestCheck_LinkImageStyle_ReferenceImageIgnoresInlineImageToggle is the
// regression guard for the bug where every *ast.Image used the
// inline-image toggle: a reference-style image must NOT be flagged by
// inline-image:false, which governs only inline ![alt](src) images.
func TestCheck_LinkImageStyle_ReferenceImageIgnoresInlineImageToggle(t *testing.T) {
	src := "# Doc\n\n![alt][label]\n\n[label]: img.png\n"
	f := newFile(t, src)
	r := &Rule{Links: LinksConfig{Style: StyleConfig{
		LinkImageStyle: LinkImageStyleConfig{
			Active: true, Autolink: true, Inline: true, Full: true, Collapsed: true, Shortcut: true,
			InlineImage: false,
		},
	}}}
	assert.Empty(t, r.Check(f), "reference-style image must not be flagged by inline-image:false")
}

// TestCheck_LinkImageStyle_AutolinkPositionDistinguishesPrefixURLs
// verifies the search pattern includes the closing `>` so a short
// autolink whose URL prefixes a neighbour's resolves to its own column.
func TestCheck_LinkImageStyle_AutolinkPositionDistinguishesPrefixURLs(t *testing.T) {
	src := "# Doc\n\n<https://a.com/x> and <https://a.com>\n"
	f := newFile(t, src)
	r := &Rule{Links: LinksConfig{Style: StyleConfig{
		LinkImageStyle: LinkImageStyleConfig{Active: true, Autolink: false,
			Inline: true, Full: true, Collapsed: true, Shortcut: true, InlineImage: true},
	}}}
	diags := r.Check(f)
	require.Len(t, diags, 2)
	// Document order: the longer URL at column 1, then the short URL
	// after "> and " at column 23 — not column 1 (the prefix match).
	assert.Equal(t, 1, diags[0].Column)
	assert.Equal(t, 23, diags[1].Column, "short autolink resolves to its own column, not the prefix match")
}

// TestApplySettings_LinkImageStyle_DeepMergeAcrossLayers verifies the
// link-image-style sub-map deep-merges across config layers: a base
// rules: layer and a kind layer each set a different toggle, and both
// survive in the effective config.
func TestApplySettings_LinkImageStyle_DeepMergeAcrossLayers(t *testing.T) {
	cfg := &config.Config{
		Rules: map[string]config.RuleCfg{
			"link-style": {Enabled: true, Settings: map[string]any{
				"links": map[string]any{"style": map[string]any{
					"link-image-style": map[string]any{"inline": false},
				}},
			}},
		},
		Kinds: map[string]config.KindBody{
			"docs": {Rules: map[string]config.RuleCfg{
				"link-style": {Enabled: true, Settings: map[string]any{
					"links": map[string]any{"style": map[string]any{
						"link-image-style": map[string]any{"full": false},
					}},
				}},
			}},
		},
		KindAssignment: []config.KindAssignmentEntry{
			{Files: []string{"docs/**/*.md"}, Kinds: []string{"docs"}},
		},
	}
	rules := config.Effective(cfg, "docs/guides/foo.md", nil, nil)
	r := &Rule{}
	require.NoError(t, r.ApplySettings(rules["link-style"].Settings))
	lis := r.Links.Style.LinkImageStyle
	assert.True(t, lis.Active)
	assert.False(t, lis.Inline, "base layer inline:false must survive the kind's full:false override")
	assert.False(t, lis.Full, "kind layer full:false must apply")
	assert.True(t, lis.Collapsed, "an untouched toggle stays at its default allow")
}

func newFile(t *testing.T, src string) *lint.File {
	t.Helper()
	f, err := lint.NewFile("doc.md", []byte(src))
	require.NoError(t, err)
	return f
}
