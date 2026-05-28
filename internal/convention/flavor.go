// Package convention owns the convention data shape (a Markdown
// flavor paired with a table of rule presets) independent of any
// rule. A convention pairs a Markdown flavor with a table of rule
// presets; the config loader consults this package at load time so a
// top-level `convention:` selection becomes a base layer beneath the
// user's own rule config. Rule packages (notably
// internal/rules/markdownflavor) consume these data shapes — they do
// not own them — which keeps internal/config from importing a rule.
//
// The Flavor identity itself lives in pkg/markdown/flavor; this
// package re-exports it via type and constant aliases so existing
// callers under internal/ keep importing convention.Flavor.
package convention

import "github.com/jeduden/mdsmith/pkg/markdown/flavor"

// Flavor is an alias for pkg/markdown/flavor.Flavor; convention.Flavor
// and flavor.Flavor name the same underlying type.
type Flavor = flavor.Flavor

// Flavor constants are re-exported from pkg/markdown/flavor so that
// existing callers (internal/config, the markdown-flavor rule, the
// convention table below) keep working with the convention.Flavor*
// names. Add a new flavor by extending the canonical list in
// pkg/markdown/flavor and adding one alias here.
const (
	FlavorCommonMark    = flavor.FlavorCommonMark
	FlavorGFM           = flavor.FlavorGFM
	FlavorGoldmark      = flavor.FlavorGoldmark
	FlavorAny           = flavor.FlavorAny
	FlavorPandoc        = flavor.FlavorPandoc
	FlavorPHPExtra      = flavor.FlavorPHPExtra
	FlavorMultiMarkdown = flavor.FlavorMultiMarkdown
	FlavorMyST          = flavor.FlavorMyST
)

// ParseFlavor delegates to pkg/markdown/flavor.ParseFlavor so callers
// that already import internal/convention do not need a second import.
func ParseFlavor(s string) (Flavor, bool) {
	return flavor.ParseFlavor(s)
}
