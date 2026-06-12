package rules

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The "Meta-Information" section of every rule README lists the peer
// Markdown linters this rule covers, one bullet per peer, each linking
// to that peer's rule documentation. The bullets are derived from the
// `markdownlint:`/`rumdl:`/`mado:`/`panache:`/`obsidian-linter:`/
// `gomarklint:` front matter (the same source the coverage matrix
// reads), so this test is the golden generator and validator that
// keeps the two in sync.
//
// Regenerate after editing peer front matter:
//
//	MDSMITH_UPDATE_PEER_LINKS=1 go test ./internal/rules -run TestRuleREADMEPeerLinks
//
// A normal `go test` run only validates and fails when a README's peer
// block has drifted from its front matter.

// peerLinkSpec pairs a peer linter's display label with its mappings.
type peerLinkSpec struct {
	label string
	maps  []RuleMapping
}

// peerLinkSpecs returns the six peer linters in their canonical bullet
// order. The label is the peer's own (lowercase) brand name.
func peerLinkSpecs(info RuleInfo) []peerLinkSpec {
	return []peerLinkSpec{
		{"markdownlint", info.Markdownlint},
		{"rumdl", info.Rumdl},
		{"mado", info.Mado},
		{"panache", info.Panache},
		{"obsidian-linter", info.ObsidianLinter},
		{"gomarklint", info.Gomarklint},
	}
}

// obsidianCategory maps an obsidian-linter rule id to the docs-site
// settings page that documents it. The published docs render each
// category on its own page (settings/<category>-rules/) and anchor each
// rule by its kebab-case name, so a link needs the rule's category.
// Add an entry here when a rule README maps a new obsidian-linter rule.
var obsidianCategory = map[string]string{
	// Heading rules.
	"header-increment":                       "heading",
	"headings-start-line":                    "heading",
	"remove-trailing-punctuation-in-heading": "heading",
	// Spacing rules.
	"heading-blank-lines":           "spacing",
	"paragraph-blank-lines":         "spacing",
	"trailing-spaces":               "spacing",
	"consecutive-blank-lines":       "spacing",
	"line-break-at-document-end":    "spacing",
	"empty-line-around-code-fences": "spacing",
	"empty-line-around-tables":      "spacing",
	"empty-line-around-blockquotes": "spacing",
	"space-after-list-markers":      "spacing",
	// Content rules.
	"blockquote-style":     "content",
	"no-bare-urls":         "content",
	"emphasis-style":       "content",
	"strong-style":         "content",
	"unordered-list-style": "content",
	"ordered-list-style":   "content",
}

// shortcutPeers carry their own kebab rule names as ids, so their
// bullets use a CommonMark shortcut reference link — `[rule-name]` whose
// own text doubles as the reference label. That keeps the bullet short
// (the long names would otherwise appear twice, overrunning the line
// limit) and reads naturally. markdownlint, rumdl, and mado instead show
// a markdownlint-style `MDxxx` id, so they use a full `[MDxxx][label]`
// reference with a prefixed label.
func isShortcutPeer(label string) bool {
	return label == "panache" || label == "obsidian-linter"
}

// peerRefID returns the link-reference label for one peer mapping.
// markdownlint, rumdl, and mado prefix the markdownlint-style id so the
// same `MDxxx` covered by several peers gets a distinct label per peer.
// Every mado entry shares one label because mado has no per-rule docs,
// only a single "Supported Rules" table; gomarklint shares one label the
// same way (its docs put every rule on a single Rules page), which also
// keeps its kebab rule names from colliding with a shortcut peer's bare
// label (obsidian-linter and gomarklint both have a `no-bare-urls`).
// Shortcut peers (panache, obsidian-linter) use the bare rule name as
// the label.
func peerRefID(label string, m RuleMapping) string {
	switch label {
	case "markdownlint":
		return "mdl-" + strings.ToLower(m.ID)
	case "rumdl":
		return "rumdl-" + strings.ToLower(m.ID)
	case "mado":
		return "mado-rules"
	case "gomarklint":
		return "gomarklint-rules"
	case "panache", "obsidian-linter":
		return m.Name
	default:
		return ""
	}
}

// peerRefURL returns the documentation URL for one peer mapping, or an
// error when an obsidian-linter rule has no known category.
func peerRefURL(label string, m RuleMapping) (string, error) {
	switch label {
	case "markdownlint":
		return "https://github.com/DavidAnson/markdownlint/blob/main/doc/" +
			strings.ToLower(m.ID) + ".md", nil
	case "rumdl":
		return "https://rumdl.dev/" + strings.ToLower(m.ID) + "/", nil
	case "mado":
		return "https://github.com/akiomik/mado#supported-rules", nil
	case "gomarklint":
		return "https://shinagawa-web.github.io/gomarklint/docs/rules/", nil
	case "panache":
		return "https://panache.bz/reference/linter-rules.html#" + m.Name, nil
	case "obsidian-linter":
		cat, ok := obsidianCategory[m.Name]
		if !ok {
			return "", &peerLinkError{m.Name}
		}
		return "https://platers.github.io/obsidian-linter/settings/" +
			cat + "-rules/#" + m.Name, nil
	default:
		return "", &peerLinkError{label}
	}
}

type peerLinkError struct{ what string }

func (e *peerLinkError) Error() string {
	return "no obsidian-linter category for " + e.what +
		" (add it to obsidianCategory in peerlinks_test.go)"
}

// peerEntry renders the link for one peer mapping. markdownlint-style
// peers show "[MD013][mdl-md013] (line-length)"; shortcut peers show
// "[undefined-anchor]". A peer whose id is its kebab name (gomarklint)
// drops the parenthetical, which would just repeat the id. A trailing
// " (partial)" marks a partial cover, matching the coverage-matrix
// legend.
func peerEntry(label string, m RuleMapping) string {
	var text string
	switch {
	case isShortcutPeer(label):
		text = "[" + m.Name + "]"
	case m.ID == m.Name:
		text = "[" + m.ID + "][" + peerRefID(label, m) + "]"
	default:
		text = "[" + m.ID + "][" + peerRefID(label, m) + "] (" + m.Name + ")"
	}
	if m.Partial {
		text += " (partial)"
	}
	return text
}

// peerBullet renders the "- **<label>**:" bullet. A single mapping sits
// inline; multiple mappings become a nested list, the same shape the
// existing markdownlint bullets already use.
func peerBullet(label string, maps []RuleMapping) string {
	if len(maps) == 1 {
		return "- **" + label + "**: " + peerEntry(label, maps[0])
	}
	var b strings.Builder
	b.WriteString("- **" + label + "**:")
	for _, m := range maps {
		b.WriteString("\n  - " + peerEntry(label, m))
	}
	return b.String()
}

// refDef renders a link-reference definition. A definition longer than
// the 80-column line limit puts the URL on its own indented line: the
// label line stays short and the URL line is a bare URL, which the
// line-length rule's "urls" exclusion skips. CommonMark allows the
// destination on the line after the colon, and `mdsmith fix` leaves the
// wrapped form untouched.
func refDef(ref, url string) string {
	oneLine := "[" + ref + "]: " + url
	if len(oneLine) <= 80 {
		return oneLine
	}
	return "[" + ref + "]:\n  " + url
}

// renderPeerLinks builds the peer-linter block that follows the
// "- **Category**:" bullet: one bullet per non-empty peer, a blank line,
// then the link-reference definitions. It returns "" when the rule maps
// to no peer linter.
func renderPeerLinks(info RuleInfo) (string, error) {
	var bullets, refs []string
	seen := map[string]string{}
	for _, p := range peerLinkSpecs(info) {
		if len(p.maps) == 0 {
			continue
		}
		bullets = append(bullets, peerBullet(p.label, p.maps))
		for _, m := range p.maps {
			ref := peerRefID(p.label, m)
			url, err := peerRefURL(p.label, m)
			if err != nil {
				return "", err
			}
			if prev, ok := seen[ref]; ok {
				if prev != url {
					return "", fmt.Errorf("reference label %q resolves to two URLs (%s, %s)", ref, prev, url)
				}
				continue
			}
			seen[ref] = url
			refs = append(refs, refDef(ref, url))
		}
	}
	if len(bullets) == 0 {
		return "", nil
	}
	return strings.Join(bullets, "\n") + "\n\n" + strings.Join(refs, "\n") + "\n", nil
}

// peerRefMarkers are the link-reference label prefixes a markdownlint-
// style peer block emits. A rule that maps to no peer linter must carry
// none of them. (Shortcut peers reuse the bare rule name as the label,
// which has no fixed prefix to scan for; in practice every panache /
// obsidian-linter rule also has a markdownlint analog, so these still
// catch a fully de-mapped rule.)
var peerRefMarkers = []string{"[mdl-", "[rumdl-", "[mado-rules]", "[gomarklint-rules]"}

const categoryMarker = "\n- **Category**:"

// TestRuleREADMEPeerLinks validates (or, with MDSMITH_UPDATE_PEER_LINKS=1,
// regenerates) the peer-linter block at the end of each rule README's
// Meta-Information section against the peer front matter.
func TestRuleREADMEPeerLinks(t *testing.T) {
	update := os.Getenv("MDSMITH_UPDATE_PEER_LINKS") == "1"
	paths, err := filepath.Glob("MDS*/README.md")
	require.NoError(t, err)
	require.NotEmpty(t, paths)

	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			raw, err := os.ReadFile(path)
			require.NoError(t, err)
			content := string(raw)

			info, err := parseFrontMatter(content)
			require.NoError(t, err)

			block, err := renderPeerLinks(info)
			require.NoError(t, err)

			if block == "" {
				for _, m := range peerRefMarkers {
					assert.NotContainsf(t, content, m,
						"%s maps to no peer linter but its body still defines %s links", path, m)
				}
				return
			}

			ci := strings.Index(content, categoryMarker)
			require.GreaterOrEqualf(t, ci, 0, "%s: no Category bullet found", path)
			rel := strings.IndexByte(content[ci+1:], '\n')
			require.GreaterOrEqualf(t, rel, 0, "%s: Category bullet not terminated", path)
			afterIdx := ci + 1 + rel + 1

			if update {
				next := content[:afterIdx] + block
				if next != content {
					require.NoError(t, os.WriteFile(path, []byte(next), 0o644))
				}
				return
			}

			assert.Equalf(t, block, content[afterIdx:],
				"%s peer-link block out of sync with front matter; "+
					"regenerate with MDSMITH_UPDATE_PEER_LINKS=1 go test ./internal/rules -run TestRuleREADMEPeerLinks",
				path)
		})
	}
}

func TestPeerEntry(t *testing.T) {
	t.Run("markdownlint id and name differ", func(t *testing.T) {
		got := peerEntry("markdownlint", RuleMapping{ID: "MD013", Name: "line-length"})
		assert.Equal(t, "[MD013][mdl-md013] (line-length)", got)
	})
	t.Run("partial cover", func(t *testing.T) {
		got := peerEntry("markdownlint", RuleMapping{ID: "MD020", Name: "no-missing-space-closed-atx", Partial: true})
		assert.Equal(t, "[MD020][mdl-md020] (no-missing-space-closed-atx) (partial)", got)
	})
	t.Run("panache uses a shortcut reference", func(t *testing.T) {
		got := peerEntry("panache", RuleMapping{ID: "heading-hierarchy", Name: "heading-hierarchy"})
		assert.Equal(t, "[heading-hierarchy]", got)
	})
	t.Run("obsidian uses a shortcut reference", func(t *testing.T) {
		got := peerEntry("obsidian-linter", RuleMapping{
			ID: "header-increment", Name: "header-increment", Partial: true,
		})
		assert.Equal(t, "[header-increment] (partial)", got)
	})
	t.Run("gomarklint id equals name so no parenthetical", func(t *testing.T) {
		got := peerEntry("gomarklint", RuleMapping{
			ID: "link-fragments", Name: "link-fragments",
		})
		assert.Equal(t, "[link-fragments][gomarklint-rules]", got)
	})
}

func TestPeerBullet(t *testing.T) {
	t.Run("single mapping inline", func(t *testing.T) {
		got := peerBullet("rumdl", []RuleMapping{{ID: "MD013", Name: "line-length"}})
		assert.Equal(t, "- **rumdl**: [MD013][rumdl-md013] (line-length)", got)
	})
	t.Run("multiple mappings nest", func(t *testing.T) {
		got := peerBullet("markdownlint", []RuleMapping{
			{ID: "MD018", Name: "no-missing-space-atx"},
			{ID: "MD019", Name: "no-multiple-space-atx"},
		})
		want := "- **markdownlint**:\n" +
			"  - [MD018][mdl-md018] (no-missing-space-atx)\n" +
			"  - [MD019][mdl-md019] (no-multiple-space-atx)"
		assert.Equal(t, want, got)
	})
}

func TestPeerRefURL(t *testing.T) {
	cases := []struct {
		label string
		m     RuleMapping
		want  string
	}{
		{label: "markdownlint", m: RuleMapping{ID: "MD013"},
			want: "https://github.com/DavidAnson/markdownlint/blob/main/doc/md013.md"},
		{label: "rumdl", m: RuleMapping{ID: "MD051"}, want: "https://rumdl.dev/md051/"},
		{label: "mado", m: RuleMapping{ID: "MD040"},
			want: "https://github.com/akiomik/mado#supported-rules"},
		{label: "panache", m: RuleMapping{ID: "undefined-anchor", Name: "undefined-anchor"},
			want: "https://panache.bz/reference/linter-rules.html#undefined-anchor"},
		{label: "obsidian-linter", m: RuleMapping{ID: "trailing-spaces", Name: "trailing-spaces"},
			want: "https://platers.github.io/obsidian-linter/settings/spacing-rules/#trailing-spaces"},
		{label: "obsidian-linter", m: RuleMapping{ID: "no-bare-urls", Name: "no-bare-urls"},
			want: "https://platers.github.io/obsidian-linter/settings/content-rules/#no-bare-urls"},
		{label: "gomarklint", m: RuleMapping{ID: "single-h1", Name: "single-h1"},
			want: "https://shinagawa-web.github.io/gomarklint/docs/rules/"},
	}
	for _, c := range cases {
		got, err := peerRefURL(c.label, c.m)
		require.NoError(t, err)
		assert.Equal(t, c.want, got)
	}
}

func TestPeerRefURL_UnknownObsidianCategory(t *testing.T) {
	_, err := peerRefURL("obsidian-linter", RuleMapping{ID: "made-up-rule", Name: "made-up-rule"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "made-up-rule")
}

func TestMadoShareSingleRef(t *testing.T) {
	info := RuleInfo{
		ID:       "MDS064",
		Category: "heading",
		Mado: []RuleMapping{
			{ID: "MD018", Name: "no-missing-space-atx", Default: true},
			{ID: "MD019", Name: "no-multiple-space-atx", Default: true},
		},
	}
	block, err := renderPeerLinks(info)
	require.NoError(t, err)
	// Two mado entries, but only one shared reference definition.
	assert.Equal(t, 1, strings.Count(block, "[mado-rules]: "))
	assert.Contains(t, block, "[MD018][mado-rules] (no-missing-space-atx)")
	assert.Contains(t, block, "[MD019][mado-rules] (no-multiple-space-atx)")
}

func TestGomarklintSharesSingleRef(t *testing.T) {
	info := RuleInfo{
		ID:       "MDS012",
		Category: "link",
		// The obsidian-linter shortcut label is the bare rule name; the
		// gomarklint entry for the same upstream name must not collide
		// with it, which is why gomarklint shares one prefixed label.
		ObsidianLinter: []RuleMapping{
			{ID: "no-bare-urls", Name: "no-bare-urls", Default: false},
		},
		Gomarklint: []RuleMapping{
			{ID: "no-bare-urls", Name: "no-bare-urls", Default: true},
		},
	}
	block, err := renderPeerLinks(info)
	require.NoError(t, err)
	assert.Equal(t, 1, strings.Count(block, "[gomarklint-rules]: "))
	assert.Contains(t, block, "- **gomarklint**: [no-bare-urls][gomarklint-rules]")
	assert.Contains(t, block,
		"[gomarklint-rules]: https://shinagawa-web.github.io/gomarklint/docs/rules/")
}

func TestRenderPeerLinks_ConflictingShortcutLabels(t *testing.T) {
	// A future rule that maps the same bare name to two different peer
	// URLs would emit one ambiguous shortcut label; the renderer must
	// reject it rather than silently link one of them wrong.
	info := RuleInfo{
		ID:             "MDS999",
		Category:       "prose",
		Panache:        []RuleMapping{{ID: "emphasis-style", Name: "emphasis-style"}},
		ObsidianLinter: []RuleMapping{{ID: "emphasis-style", Name: "emphasis-style"}},
	}
	_, err := renderPeerLinks(info)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "emphasis-style")
}

func TestRefDefWrapsLongURL(t *testing.T) {
	short := refDef("rumdl-md013", "https://rumdl.dev/md013/")
	assert.Equal(t, "[rumdl-md013]: https://rumdl.dev/md013/", short)

	url := "https://platers.github.io/obsidian-linter/settings/heading-rules/" +
		"#remove-trailing-punctuation-in-heading"
	long := refDef("remove-trailing-punctuation-in-heading", url)
	assert.Equal(t, "[remove-trailing-punctuation-in-heading]:\n  "+url, long)
	// The label line stays within the column budget; the URL line is a
	// bare URL the line-length rule's "urls" exclusion skips.
	assert.LessOrEqual(t, len("[remove-trailing-punctuation-in-heading]:"), 80)
}

func TestRenderPeerLinks_NoPeers(t *testing.T) {
	block, err := renderPeerLinks(RuleInfo{ID: "MDS022", Category: "structural"})
	require.NoError(t, err)
	assert.Equal(t, "", block)
}
