package flavor

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

// IsValid reports whether f names a recognised flavor. The zero
// value (reserved for "unparsed/unset") and any out-of-range integer
// cast to Flavor both return false. Implemented in terms of String
// so the two stay in lock-step: every recognised flavor has a name,
// and adding a new constant only requires updating the switch in
// String.
func (f Flavor) IsValid() bool { return f.String() != "" }

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
