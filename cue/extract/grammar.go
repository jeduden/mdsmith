// Package extract embeds the published CUE contract for the
// `mdsmith extract` block- and span-list output grammar so the
// differential test can validate fixtures against it without reading
// from disk at runtime. The contract source lives in `grammar.cue`
// next to this file.
//
// The intended import path for external CUE consumers is
// `github.com/jeduden/mdsmith/extract`. The literal CUE import syntax
// is not yet wired; today the surface is the embedded definition the
// extract reference documents and the differential test consumes.
//
//nolint:revive // "extract" mirrors the CUE-side import path
package extract

import _ "embed"

//go:embed grammar.cue
var source string

// Source returns the embedded `grammar.cue` contents verbatim. The
// differential test in internal/extract compiles it and unifies every
// projected block / span against #Block / #Span.
func Source() string { return source }
