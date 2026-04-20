// Package markdownflavor implements MDS034, which validates Markdown
// against a declared target flavor (commonmark, gfm, goldmark,
// pandoc, phpextra, multimarkdown, myst, or any) and flags syntax
// the target renderer will not understand.
package markdownflavor

// Flavor identifies a target Markdown flavor.
type Flavor int

// Flavor constants. The zero value is intentionally invalid so that
// unparsed settings are caught.
const (
	flavorInvalid Flavor = iota
	FlavorCommonMark
	FlavorGFM
	FlavorGoldmark
	// FlavorAny accepts every tracked feature. Useful when the
	// document is destined for an unknown or permissive renderer and
	// the user wants to disable flavor reporting without disabling
	// the rule.
	FlavorAny
	// FlavorPandoc is Pandoc's default markdown dialect. Accepts
	// GFM's four features plus footnotes, definition lists, heading
	// IDs, superscript, subscript, math block, and inline math;
	// rejects abbreviations (a non-default Pandoc extension).
	FlavorPandoc
	// FlavorPHPExtra is PHP Markdown Extra. Accepts tables,
	// footnotes, definition lists, heading IDs, and abbreviations;
	// rejects GFM's task lists, strikethrough, bare-URL autolinks,
	// and every math / sub/superscript feature.
	FlavorPHPExtra
	// FlavorMultiMarkdown extends PHP Markdown Extra with math
	// block and inline math. Like PHP Extra, rejects GFM task lists,
	// strikethrough, bare-URL autolinks, and sub/superscript.
	FlavorMultiMarkdown
	// FlavorMyST is the MyST flavor used by the Sphinx documentation
	// toolchain. Accepts tables, strikethrough, footnotes,
	// definition lists, heading IDs, math block, and inline math;
	// rejects GFM task lists, bare-URL autolinks, sub/superscript,
	// and abbreviations.
	FlavorMyST
)

// String returns the canonical lowercase name of the flavor.
func (f Flavor) String() string {
	switch f {
	case FlavorCommonMark:
		return "commonmark"
	case FlavorGFM:
		return "gfm"
	case FlavorGoldmark:
		return "goldmark"
	case FlavorAny:
		return "any"
	case FlavorPandoc:
		return "pandoc"
	case FlavorPHPExtra:
		return "phpextra"
	case FlavorMultiMarkdown:
		return "multimarkdown"
	case FlavorMyST:
		return "myst"
	}
	return ""
}

// ParseFlavor converts a config string into a Flavor. The match is
// case-sensitive to reject typos like "GFM" that would otherwise
// silently validate against the wrong flavor.
func ParseFlavor(s string) (Flavor, bool) {
	switch s {
	case "commonmark":
		return FlavorCommonMark, true
	case "gfm":
		return FlavorGFM, true
	case "goldmark":
		return FlavorGoldmark, true
	case "any":
		return FlavorAny, true
	case "pandoc":
		return FlavorPandoc, true
	case "phpextra":
		return FlavorPHPExtra, true
	case "multimarkdown":
		return FlavorMultiMarkdown, true
	case "myst":
		return FlavorMyST, true
	}
	return 0, false
}

// Feature identifies one Markdown syntax feature whose support varies
// across flavors.
type Feature int

// Feature constants. Keep in sync with AllFeatures, Feature.Name,
// and Feature.Verb.
const (
	FeatureTables Feature = iota
	FeatureTaskLists
	FeatureStrikethrough
	FeatureBareURLAutolinks
	FeatureFootnotes
	FeatureDefinitionLists
	FeatureHeadingIDs
	FeatureSuperscript
	FeatureSubscript
	FeatureMathBlock
	FeatureMathInline
	FeatureAbbreviations
	FeatureGitHubAlerts
)

// AllFeatures returns every tracked feature in declaration order.
func AllFeatures() []Feature {
	return []Feature{
		FeatureTables,
		FeatureTaskLists,
		FeatureStrikethrough,
		FeatureBareURLAutolinks,
		FeatureFootnotes,
		FeatureDefinitionLists,
		FeatureHeadingIDs,
		FeatureSuperscript,
		FeatureSubscript,
		FeatureMathBlock,
		FeatureMathInline,
		FeatureAbbreviations,
		FeatureGitHubAlerts,
	}
}

// Verb returns "is" or "are" so diagnostic messages read naturally
// for both singular (strikethrough, inline math) and plural
// (tables, task lists) feature names.
func (f Feature) Verb() string {
	switch f {
	case FeatureStrikethrough, FeatureSuperscript, FeatureSubscript,
		FeatureMathInline:
		return "is"
	case FeatureGitHubAlerts:
		return "are"
	}
	return "are"
}

// Name returns the human-readable feature name used in diagnostics.
func (f Feature) Name() string {
	switch f {
	case FeatureTables:
		return "tables"
	case FeatureTaskLists:
		return "task lists"
	case FeatureStrikethrough:
		return "strikethrough"
	case FeatureBareURLAutolinks:
		return "bare-URL autolinks"
	case FeatureFootnotes:
		return "footnotes"
	case FeatureDefinitionLists:
		return "definition lists"
	case FeatureHeadingIDs:
		return "heading IDs"
	case FeatureSuperscript:
		return "superscript"
	case FeatureSubscript:
		return "subscript"
	case FeatureMathBlock:
		return "math blocks"
	case FeatureMathInline:
		return "inline math"
	case FeatureAbbreviations:
		return "abbreviations"
	case FeatureGitHubAlerts:
		return "github alerts"
	}
	return ""
}

// support maps (flavor, feature) to whether the flavor accepts it.
// CommonMark rejects every tracked feature. GFM adds tables, task
// lists, strikethrough, and bare-URL autolinks. The goldmark profile
// further adds heading IDs. Pandoc, PHP Markdown Extra, MultiMarkdown,
// and MyST each pick a different combination of the optional
// features; FlavorAny is handled specially in Supports.
var support = map[Flavor]map[Feature]bool{
	FlavorGFM: {
		FeatureTables:           true,
		FeatureTaskLists:        true,
		FeatureStrikethrough:    true,
		FeatureBareURLAutolinks: true,
		FeatureGitHubAlerts:     true,
	},
	FlavorGoldmark: {
		FeatureTables:           true,
		FeatureTaskLists:        true,
		FeatureStrikethrough:    true,
		FeatureBareURLAutolinks: true,
		FeatureHeadingIDs:       true,
	},
	FlavorPandoc: {
		FeatureTables:           true,
		FeatureTaskLists:        true,
		FeatureStrikethrough:    true,
		FeatureBareURLAutolinks: true,
		FeatureFootnotes:        true,
		FeatureDefinitionLists:  true,
		FeatureHeadingIDs:       true,
		FeatureSuperscript:      true,
		FeatureSubscript:        true,
		FeatureMathBlock:        true,
		FeatureMathInline:       true,
	},
	FlavorPHPExtra: {
		FeatureTables:          true,
		FeatureFootnotes:       true,
		FeatureDefinitionLists: true,
		FeatureHeadingIDs:      true,
		FeatureAbbreviations:   true,
	},
	FlavorMultiMarkdown: {
		FeatureTables:          true,
		FeatureFootnotes:       true,
		FeatureDefinitionLists: true,
		FeatureHeadingIDs:      true,
		FeatureAbbreviations:   true,
		FeatureMathBlock:       true,
		FeatureMathInline:      true,
	},
	FlavorMyST: {
		FeatureTables:          true,
		FeatureStrikethrough:   true,
		FeatureFootnotes:       true,
		FeatureDefinitionLists: true,
		FeatureHeadingIDs:      true,
		FeatureMathBlock:       true,
		FeatureMathInline:      true,
	},
}

// Supports reports whether the flavor accepts the given feature.
// FlavorAny accepts every feature; other flavors consult the
// support table.
func (f Flavor) Supports(feat Feature) bool {
	if f == FlavorAny {
		return true
	}
	return support[f][feat]
}
