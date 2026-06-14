package gensection

import (
	"bytes"

	"github.com/jeduden/mdsmith/internal/lint"
)

// generatedDirectiveNames are the directives whose generated bodies must
// be excluded from host-file diagnostics and host-file metric counts.
var generatedDirectiveNames = []string{"include", "catalog"}

// directiveMarkers are the byte prefixes used for the quick pre-check in
// AuthoredSource. Kept in sync with generatedDirectiveNames.
var directiveMarkers = [][]byte{[]byte("<?include"), []byte("<?catalog")}

// HasGeneratedDirective reports whether source contains an include or
// catalog directive opener — the directives whose generated bodies
// FindAllGeneratedRanges excludes. It is a cheap byte scan with no parse,
// so callers can decide on the raw bytes whether a file even has a
// generated section. The engine's flat Layer-0 path (plan 2606142147) uses
// it to keep files with generated sections on the AST path, where the
// generated-range suppression is available.
func HasGeneratedDirective(source []byte) bool {
	for _, marker := range directiveMarkers {
		if bytes.Contains(source, marker) {
			return true
		}
	}
	return false
}

// FindAllGeneratedRanges returns the content line ranges for all
// include/catalog generated sections in f. Lines are 1-based and
// relative to f.Source (i.e. post-front-matter when the file was
// created with NewFileFromSource).
//
// If FindMarkerPairs returns any diagnostics for a directive (indicating
// malformed markers), that directive's ranges are omitted entirely so the
// engine never suppresses diagnostics based on an ambiguous range boundary.
func FindAllGeneratedRanges(f *lint.File) []lint.LineRange {
	var ranges []lint.LineRange
	for _, name := range generatedDirectiveNames {
		pairs, diags := FindMarkerPairs(f, name, "", "")
		if len(diags) > 0 {
			continue // malformed markers — skip to avoid filtering based on invalid spans
		}
		for _, mp := range pairs {
			if mp.ContentFrom <= mp.ContentTo {
				ranges = append(ranges, lint.LineRange{From: mp.ContentFrom, To: mp.ContentTo})
			}
		}
	}
	return ranges
}

// AuthoredSource returns source with the bodies of all include/catalog
// generated sections removed (the opening and closing markers are kept).
// This gives the "authored bytes" — what the file author wrote, excluding
// fragments pulled in by directives. Used by the metrics pipeline so that
// a host file's metric values count only its own content.
func AuthoredSource(source []byte) []byte {
	// Quick check: skip the expensive parse when no directive markers are present.
	if !HasGeneratedDirective(source) {
		return source
	}

	f, _ := lint.NewFile("", source) // NewFile never errors with current implementation
	ranges := FindAllGeneratedRanges(f)
	if len(ranges) == 0 {
		return source
	}

	// Reuse f.Lines (already split by NewFile) to avoid a second split.
	lines := f.Lines
	inRange := func(lineNum int) bool {
		for _, r := range ranges {
			if r.Contains(lineNum) {
				return true
			}
		}
		return false
	}

	var result []byte
	for i, line := range lines {
		lineNum := i + 1 // 1-based
		if inRange(lineNum) {
			continue
		}
		result = append(result, line...)
		if i < len(lines)-1 {
			result = append(result, '\n')
		}
	}
	return result
}
