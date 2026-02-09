package rule

import "github.com/jeduden/tidymark/internal/lint"

type Rule interface {
	ID() string
	Name() string
	Check(f *lint.File) []lint.Diagnostic
}

type FixableRule interface {
	Rule
	Fix(f *lint.File) []byte
}
