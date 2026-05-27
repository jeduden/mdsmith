package flavor

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseFlavor(t *testing.T) {
	tests := []struct {
		in   string
		want Flavor
		ok   bool
	}{
		{"commonmark", FlavorCommonMark, true},
		{"gfm", FlavorGFM, true},
		{"goldmark", FlavorGoldmark, true},
		{"any", FlavorAny, true},
		{"pandoc", FlavorPandoc, true},
		{"phpextra", FlavorPHPExtra, true},
		{"multimarkdown", FlavorMultiMarkdown, true},
		{"myst", FlavorMyST, true},
		{"GFM", 0, false},
		{"", 0, false},
		{"markdown", 0, false},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			got, ok := ParseFlavor(tc.in)
			assert.Equal(t, tc.ok, ok)
			if tc.ok {
				assert.Equal(t, tc.want, got)
			}
		})
	}
}

func TestFlavorString(t *testing.T) {
	assert.Equal(t, "commonmark", FlavorCommonMark.String())
	assert.Equal(t, "gfm", FlavorGFM.String())
	assert.Equal(t, "goldmark", FlavorGoldmark.String())
	assert.Equal(t, "any", FlavorAny.String())
	assert.Equal(t, "pandoc", FlavorPandoc.String())
	assert.Equal(t, "phpextra", FlavorPHPExtra.String())
	assert.Equal(t, "multimarkdown", FlavorMultiMarkdown.String())
	assert.Equal(t, "myst", FlavorMyST.String())
}

func TestFlavorStringUnknownIsEmpty(t *testing.T) {
	var zero Flavor
	assert.Equal(t, "", zero.String())
	assert.Equal(t, "", Flavor(999).String())
}

func TestFlavorIsValid(t *testing.T) {
	var zero Flavor
	assert.False(t, zero.IsValid(), "zero value is invalid")
	assert.False(t, Flavor(999).IsValid(),
		"out-of-range integer cast to Flavor is invalid")
	assert.True(t, FlavorCommonMark.IsValid())
	assert.True(t, FlavorAny.IsValid())
}
