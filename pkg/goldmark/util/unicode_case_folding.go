package util

// The unicode_case_folding.gen.go data table is generated upstream
// by github.com/yuin/goldmark/_tools (not vendored in this fork).
// To regenerate, sync the upstream _tools directory locally, run
// `go generate` against it, and copy the result back.

var unicodeCaseFoldings map[rune][]rune

func init() {
	unicodeCaseFoldings = make(map[rune][]rune, _unicodeCaseFoldingLength)
	cTo := 0
	for i := range _unicodeCaseFoldingLength {
		tTo := cTo + int(_unicodeCaseFoldingToIndex[i])
		to := _unicodeCaseFoldingTo[cTo:tTo]
		unicodeCaseFoldings[_unicodeCaseFoldingFrom[i]] = to
		cTo = tTo
	}
}
