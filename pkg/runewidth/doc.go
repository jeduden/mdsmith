// Package runewidth reports the terminal cell width of runes and
// strings. mdsmith vendors this fork of
// github.com/mattn/go-runewidth@v0.0.27 (commit
// f2d8bfeff22a28e1d7157559144dcdcab0255152, tag v0.0.27) so post-0.0.24
// upstream versions build under tinygo.
//
// # Why the fork exists
//
// Upstream 0.0.25 added strictWidthLUT, a [2][0x110000]byte lookup table
// filled in package init — 2,228,224 entries. tinygo's interp pass tries
// to constant-fold that initializer at compile time and times out at its
// 3-minute budget, so the wasm engine cannot build against 0.0.25+. The
// table is also 2.1 MiB of static data against an 8 MiB wasm budget.
//
// # The single divergence
//
// This fork deletes the eager LUT. strictWidthLUT was a pure memoization
// of runeWidthNoLUT: initStrictWidthLUT filled it by calling
// runeWidthNoLUT for every rune, and Condition.RuneWidth read it back.
// The fork removes the array and routes every read through
// runeWidthNoLUT, which is equivalent by construction. The removal is
// width-preserving; runewidth_lut_removed_test.go proves RuneWidth
// equals runeWidthNoLUT across the full rune range and both eastAsian
// settings, and it is an ordinary untagged test so it cannot bit-rot.
//
// Moving from 0.0.24 to 0.0.27 does change some upstream widths (for
// example regional-indicator flags go from width 1 to width 2) — that is
// a deliberate upgrade, separate from and unaffected by the LUT removal.
//
// # Re-syncing
//
// The exported API is byte-identical to upstream, and every file except
// runewidth.go is copied verbatim, so a re-sync to a newer upstream is a
// mechanical diff: copy the upstream files, then re-apply runewidth.go's
// divergences — the LUT deletion (search for "LUT removed") and the
// stripped `//go:generate` directive (search for "Fork divergence"),
// which is dropped because the script/ generator directory is not
// vendored and would break `go generate ./...`. The fork lives
// under pkg/ rather than internal/ because the upstream library is a
// public package and is part of the main module — not a nested module
// wired via a replace directive — so `go install m@version` and
// mdsmith-as-a-library consumers resolve this fork, not upstream.
package runewidth
