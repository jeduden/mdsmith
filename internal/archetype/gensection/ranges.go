package gensection

import "github.com/jeduden/mdsmith/internal/lint"

// LineRange is an inclusive range of 1-based line numbers.
type LineRange struct {
	From int // first line (1-based, inclusive)
	To   int // last line (1-based, inclusive)
}

// generatedDirectives lists the directive names whose body content is
// considered embedded (i.e., owned by a source other than the host file).
var generatedDirectives = []struct {
	name   string
	ruleID string
}{
	{"include", "MDS021"},
	{"catalog", "MDS019"},
}

// FindGeneratedLineRanges returns the line ranges of generated-section
// content in f (the bytes between start and end markers for <?include?>
// and <?catalog?> directives). Diagnostics on these lines should be
// suppressed when linting the host file, because those bytes belong to
// the source file that was included/cataloged.
//
// Unparseable or malformed markers are silently skipped; the goal is
// best-effort filtering without duplicating the error diagnostics that
// the individual rules already produce.
func FindGeneratedLineRanges(f *lint.File) []LineRange {
	var ranges []LineRange
	for _, d := range generatedDirectives {
		pairs, _ := FindMarkerPairs(f, d.name, d.ruleID, d.name)
		for _, mp := range pairs {
			if mp.ContentFrom <= mp.ContentTo {
				ranges = append(ranges, LineRange{
					From: mp.ContentFrom,
					To:   mp.ContentTo,
				})
			}
		}
	}
	return ranges
}

// InGeneratedRange reports whether line falls within any of the given ranges.
// Line numbers are 1-based (matching lint.Diagnostic.Line after adjustment).
func InGeneratedRange(line int, ranges []LineRange) bool {
	for _, r := range ranges {
		if line >= r.From && line <= r.To {
			return true
		}
	}
	return false
}
