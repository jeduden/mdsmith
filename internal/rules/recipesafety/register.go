//go:build !wasm

package recipesafety

import "github.com/jeduden/mdsmith/internal/rule"

// MDS040 (recipe safety) shell-safety-checks build recipes. That check
// presumes a real shell, which GOOS=js GOARCH=wasm does not have, so
// the rule self-registers only on native builds. The package still
// compiles under WASM — keeping the blank import in internal/rules/all
// resolvable — but rule.All() omits MDS040 there. See
// docs/background/concepts/engine-api.md.
func init() {
	rule.Register(&Rule{})
}
