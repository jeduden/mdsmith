package refactor

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMarkupEscaped(t *testing.T) {
	for name, tc := range map[string]struct {
		tok  string
		more bool
		want bool
	}{
		"plain":                       {"docs/a.md", false, false},
		"windows separator":           {`sub\a.md`, false, false},
		"escaped punctuation":         {`a\_b.md`, false, true},
		"escaped paren":               {`a\).md`, false, true},
		"named entity":                {"a&amp;b.md", false, true},
		"numeric entity":              {"a&#95;b.md", false, true},
		"unknown entity name":         {"a&nope;b.md", false, false},
		"bare ampersand":              {"a&b.md", false, false},
		"trailing backslash, no more": {`a\`, false, false},
		"escapes the `#` after it":    {`a\`, true, true},
		"opens `&#35;` after it":      {"a&", true, true},
		"empty token before a query":  {"", true, false},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, markupEscaped([]byte(tc.tok), tc.more))
		})
	}
}
