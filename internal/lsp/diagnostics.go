package lsp

import (
	"bytes"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/mdtext"
	"github.com/jeduden/mdsmith/internal/rules"
)

// toLSP converts an mdsmith diagnostic to the LSP wire shape.
//
// Coordinates flip from 1-based (mdsmith) to 0-based (LSP). The end
// column is set to the line's UTF-16 length so the squiggle covers
// the remainder of the line; rules can later widen this with their
// own per-rule end column once LSP-aware spans land in the engine.
//
// LSP positions count UTF-16 code units. mdsmith's
// `lint.Diagnostic.Column` is a 1-based UTF-8 byte column (see
// `lint.File.ColumnOfOffset`, which derives column from byte offsets
// inside the source). We convert by walking the line's bytes,
// summing the UTF-16 width of every rune we cross until we reach
// the byte offset that maps to `Column-1`. This matters for any
// document containing non-ASCII text — treating Column as a rune
// offset would misplace the squiggle by N-1 positions for every
// preceding rune that takes N>1 bytes.
//
// Both startCol and endCol come from mdtext.UTF16FromByteOffset/utf16Length
// on the same line, which clamps every input to [0, line's UTF-16
// length], so endCol is always >= startCol — no end-before-start
// guard is needed.
func toLSP(d lint.Diagnostic, lines [][]byte, root string) Diagnostic {
	return toLSPWithRoot(d, lines, root, resolveSymlinks(root))
}

// toLSPWithRoot is toLSP with the workspace root's symlink-resolved form
// precomputed. toLSPAll resolves the (constant) root once per publish and
// passes it here, so the EvalSymlinks syscall in withinRoot does not
// repeat once per related location on the keystroke hot path — only each
// target path is resolved per entry. root stays the configured root for
// the join and emitted URI; resolvedRoot is used only for the
// symlink-safe containment check.
func toLSPWithRoot(d lint.Diagnostic, lines [][]byte, root, resolvedRoot string) Diagnostic {
	startLine := d.Line - 1
	if startLine < 0 {
		startLine = 0
	}
	line := currentLineBytes(lines, d.Line)
	startCol := mdtext.UTF16FromByteOffset(line, d.Column-1)
	endCol := utf16Length(line)
	out := Diagnostic{
		Range: Range{
			Start: Position{Line: startLine, Character: startCol},
			End:   Position{Line: startLine, Character: endCol},
		},
		Severity: severityFor(d.Severity),
		Code:     d.RuleID,
		Source:   "mdsmith",
		Message:  d.Message,
		Data: &diagnosticData{
			RuleName:   d.RuleName,
			Deprecated: d.Deprecated,
			ReplacedBy: d.ReplacedBy,
		},
	}
	if ri := relatedInformation(d.RelatedLocations, root, resolvedRoot); len(ri) > 0 {
		out.RelatedInformation = ri
	}
	// codeDescription gives the rule code a clickable docs link,
	// derived from the rule ID. Unknown IDs (rules.DocURL returns "")
	// leave it unset.
	if href := rules.DocURL(d.RuleID); href != "" {
		out.CodeDescription = &codeDescription{Href: href}
	}
	return out
}

// relatedInformation converts structured related locations to the LSP
// wire form, resolving each file to a file:// URI against root
// (a workspace-relative schema path becomes absolute first). A related
// location with no File — an inline-schema label that names no source
// file — cannot become a navigable URI and is dropped here; the CLI
// still surfaces it as a trailer line. Coordinates flip 1-based →
// 0-based and clamp at 0, so a file-only ref (Line 0) anchors at the
// schema's first line.
func relatedInformation(locs []lint.RelatedLocation, root, resolvedRoot string) []diagnosticRelatedInformation {
	var out []diagnosticRelatedInformation
	for _, loc := range locs {
		uri, ok := relatedURI(loc.File, root, resolvedRoot)
		if !ok {
			continue
		}
		pos := Position{
			Line:      clampZero(loc.Line - 1),
			Character: clampZero(loc.Column - 1),
		}
		out = append(out, diagnosticRelatedInformation{
			Location: location{URI: uri, Range: Range{Start: pos, End: pos}},
			Message:  loc.Message,
		})
	}
	return out
}

// relatedURI resolves a related-location file to a file:// URI, or
// reports ok=false when no safe, navigable URI exists. A navigable URI
// requires a bounded workspace root: without one (a rootless session)
// no related location becomes a link, because nothing can vouch that a
// path — absolute or relative — stays inside the project, and a config
// could point a schema source at an arbitrary local file (e.g.
// /etc/passwd from a malicious repo). With a root, both absolute and
// relative paths must resolve inside it; a "../" escape or an absolute
// path outside the root is dropped. Every path reaching pathToURI is
// absolute, so the URI is always non-empty and no empty-URI reaches
// the wire.
func relatedURI(file, root, resolvedRoot string) (string, bool) {
	if file == "" || root == "" {
		return "", false
	}
	if isAbsPath(file) {
		if !withinRoot(resolvedRoot, file) {
			return "", false
		}
		return pathToURI(file), true
	}
	path := filepath.Join(root, filepath.FromSlash(file))
	if !withinRoot(resolvedRoot, path) {
		return "", false
	}
	return pathToURI(path), true
}

// withinRoot reports whether path resolves inside resolvedRoot (the root
// itself counts). resolvedRoot must already be absolute and symlink-
// resolved (see resolveSymlinks); path is absolutised and symlink-
// resolved here, so an in-root symlink that points outside the workspace
// cannot bypass the containment check — matching the symlink-safe guard
// the schema index writer uses (internal/schema.resolveDir). The caller
// resolves the constant root once per publish rather than once per
// related location. A "../" escape, or a path on a different volume that
// Rel cannot relate, is treated as outside.
func withinRoot(resolvedRoot, path string) bool {
	rel, err := filepath.Rel(resolvedRoot, resolveSymlinks(path))
	return err == nil && rel != ".." &&
		!strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// resolveSymlinks returns p as an absolute, symlink-resolved path so a
// lexical containment check cannot be fooled by a symlink. EvalSymlinks
// needs the path to exist; when it does not (a stale or not-yet-created
// reference), this falls back to the lexical absolute path — best-effort,
// mirroring internal/schema.resolveDir. filepath.Abs only fails without
// a working directory, an unrecoverable state, so its error is ignored
// and Clean carries the fallback.
func resolveSymlinks(p string) string {
	abs, _ := filepath.Abs(p)
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return resolved
	}
	return filepath.Clean(abs)
}

// isAbsPath reports whether p is absolute on any host OS — including
// Windows drive-letter (`C:\x`) and UNC (`\\server\share`) paths,
// which filepath.IsAbs rejects on a non-Windows host. This mirrors the
// cross-platform classification pathToURI applies, so an absolute
// related location from a Windows client (or a cross-platform test) is
// passed straight to pathToURI rather than mis-joined to the workspace
// root.
func isAbsPath(p string) bool {
	return filepath.IsAbs(p) || isWindowsDrivePath(p) || strings.HasPrefix(p, `\\`)
}

// clampZero returns n, or 0 when n is negative. Used to flip 1-based
// schema coordinates to 0-based LSP coordinates without underflowing
// when the line or column is unknown (0).
func clampZero(n int) int {
	if n < 0 {
		return 0
	}
	return n
}

// toLSPAll maps a slice. Returns an empty (non-nil) slice for empty
// input so the JSON wire form is `[]`, never `null`. root resolves
// cross-file related-location URIs (see relatedInformation).
func toLSPAll(diags []lint.Diagnostic, source []byte, root string) []Diagnostic {
	out := make([]Diagnostic, 0, len(diags))
	lines := splitLines(source)
	// The workspace root is constant across the publish, so resolve its
	// symlinks once here instead of once per related location inside
	// withinRoot (EvalSymlinks is a syscall on the keystroke hot path).
	resolvedRoot := resolveSymlinks(root)
	for _, d := range diags {
		out = append(out, toLSPWithRoot(d, lines, root, resolvedRoot))
	}
	return out
}

func severityFor(s lint.Severity) DiagnosticSeverity {
	if s == lint.Warning {
		return severityWarning
	}
	return severityError
}

// splitLines splits source into per-line byte slices, preserving
// trailing empty lines so the indexing matches lint.File.Lines (which
// also uses bytes.Split). Rules such as single-trailing-newline emit
// diagnostics anchored at len(f.Lines) for trailing whitespace runs;
// trimming the trailing newlines here would make currentLine() return
// "" and toLSP would clamp to a position past the document. Each
// line has its trailing CR stripped so Windows-style line endings
// produce matching positions on the wire.
//
// Empty input returns a 1-element slice containing an empty line,
// matching what bytes.Split produces (and therefore lint.File.Lines).
// Returning nil here would put a diagnostic anchored at line 1 of an
// empty buffer past the line count splitLines reports — currentLineBytes
// would treat 1-based line 1 as out of range and toLSP would silently
// clamp the squiggle to the wrong place.
//
// The function operates entirely on []byte (no string round-trip)
// because it is on the diagnostics-publish hot path; allocating a
// full-document string once per publish was a noticeable per-request
// overhead on large files.
func splitLines(source []byte) [][]byte {
	if len(source) == 0 {
		// Match bytes.Split's "empty input → 1-element empty
		// slice" contract so lint.File.Lines and splitLines
		// agree on the line count of an empty buffer.
		return [][]byte{nil}
	}
	parts := bytes.Split(source, []byte{'\n'})
	for i, p := range parts {
		if n := len(p); n > 0 && p[n-1] == '\r' {
			parts[i] = p[:n-1]
		}
	}
	return parts
}

// currentLineBytes returns the content of 1-based line number n as a
// byte slice, or nil when out of range. The byte form lets the
// callers (toLSP, utf16Length) avoid an extra string conversion on
// the hot path.
func currentLineBytes(lines [][]byte, n int) []byte {
	if n < 1 || n > len(lines) {
		return nil
	}
	return lines[n-1]
}

// utf16Length returns the total UTF-16 code-unit length of line.
// Equivalent to mdtext.UTF16FromByteOffset(line, len(line)) but
// spelled out for readability at call sites that just want "end of
// line".
func utf16Length(line []byte) int {
	return mdtext.UTF16FromByteOffset(line, len(line))
}

// byteOffsetFromUTF16 maps a UTF-16 column position (LSP wire form)
// back to the matching UTF-8 byte offset within line. The result is
// clamped to [0, len(line)] so a malformed or past-end LSP position
// stays within the slice.
//
// This is the inverse of mdtext.UTF16FromByteOffset. The navigation
// surface uses it to convert `Position.Character` (UTF-16) to a byte
// column before calling the Locator, which works in 1-based byte
// columns. Without it, every cursor on a non-ASCII line would
// mis-locate by the number of multi-byte runes between byte 0 and
// the cursor. Distinct from mdtext.UTF16ToByteOffset: when utf16Off
// lands inside a surrogate pair, this rounds down to the rune's
// starting byte (LSP cursor semantics), whereas UTF16ToByteOffset
// rounds up to the next codepoint boundary.
func byteOffsetFromUTF16(line []byte, utf16Off int) int {
	if utf16Off <= 0 {
		return 0
	}
	units := 0
	for i := 0; i < len(line); {
		r, size := utf8.DecodeRune(line[i:])
		w := mdtext.NonNegativeUTF16RuneLen(r)
		if units+w > utf16Off {
			return i
		}
		units += w
		i += size
		if units == utf16Off {
			return i
		}
	}
	return len(line)
}
