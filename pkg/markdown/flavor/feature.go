package flavor

// Feature identifies one Markdown syntax feature whose support varies
// across flavors.
type Feature int

// Feature constants. Keep in sync with AllFeatures and Feature.Name.
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

// flavorCount and featureCount size the support table below: one
// slot per declared Flavor / Feature constant, indexed directly by
// the enum's underlying int.
const (
	flavorCount  = FlavorMyST + 1
	featureCount = FeatureGitHubAlerts + 1
)

// support indexes (flavor, feature) to whether the flavor accepts it.
// CommonMark rejects every tracked feature. GFM adds tables, task
// lists, strikethrough, and bare-URL autolinks. The goldmark flavor
// further adds heading IDs. Pandoc, PHP Markdown Extra, MultiMarkdown,
// and MyST each pick a different combination of the optional
// features; FlavorAny is handled specially in Supports.
//
// A flat array instead of map[Flavor]map[Feature]bool: both Flavor
// and Feature are small dense int enums, so direct indexing removes
// two map-indirection lookups from every Supports call (Supports runs
// per candidate AST node during flavor detection — see
// docs/development/high-performance-go.md's "Sorted slice + binary
// search beats a map for n < ~100" and small-fixed-set guidance).
var support = [flavorCount][featureCount]bool{
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
// FlavorAny accepts every feature; other flavors consult the support
// table. An f or feat outside the declared enum range (pkg/markdown
// is a public package, so a caller-supplied value is not guaranteed
// in range) returns false rather than panicking on an out-of-bounds
// array index — the same behavior a map's missing-key lookup gave.
func Supports(f Flavor, feat Feature) bool {
	if f == FlavorAny {
		return true
	}
	if f < 0 || f >= flavorCount || feat < 0 || feat >= featureCount {
		return false
	}
	return support[f][feat]
}
