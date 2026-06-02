// Package types embeds the canonical mdsmith
// field-type-shortcut library so schema parsing can
// resolve `created: date` and friends without
// touching the network. The library source itself
// lives in `types.cue` next to this file.
//
// The intended import path for external CUE consumers
// is `github.com/jeduden/mdsmith/types`. The literal
// CUE import syntax is not yet implemented; today's
// surface is the bare-name YAML scalar handled by
// internal/schema's frontmatterExpr. The Go package
// name matches the directory so the Go and CUE
// import paths line up for future module wiring.
//
//nolint:revive // "types" mirrors the CUE-side import path
package types

import _ "embed"

//go:embed types.cue
var source string

// Source returns the embedded `types.cue` contents
// verbatim. internal/schema reads this once to seed
// its runtime registry and to drive the drift test
// that pins registry entries to the documented CUE.
//
// no test by design: trivial embed accessor
func Source() string { return source }
