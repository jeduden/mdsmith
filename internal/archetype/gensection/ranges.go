package gensection

import (
	"bytes"

	"github.com/jeduden/mdsmith/internal/lint"
)

// generatedDirectiveNames are the directives whose generated bodies must
// be excluded from host-file diagnostics and host-file metric counts.
var generatedDirectiveNames = []string{"include", "catalog"}

// FindAllGeneratedRanges returns the content line ranges for all
// include/catalog generated sections in f. Lines are 1-based and
// relative to f.Source (i.e. post-front-matter when the file was
// created with NewFileFromSource).
func FindAllGeneratedRanges(f *lint.File) []lint.LineRange {
	var ranges []lint.LineRange
	for _, name := range generatedDirectiveNames {
		pairs, _ := FindMarkerPairs(f, name, "", "")
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
	f, err := lint.NewFile("", source)
	if err != nil {
		return source
	}
	ranges := FindAllGeneratedRanges(f)
	if len(ranges) == 0 {
		return source
	}

	lines := bytes.Split(source, []byte("\n"))
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
