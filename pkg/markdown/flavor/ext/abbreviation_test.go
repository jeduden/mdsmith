package ext

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAbbreviationDefinitionRecognised(t *testing.T) {
	src := "*[HTML]: Hyper Text Markup Language\n\nUse HTML here.\n"
	doc := parseWith(t, src, Abbreviation)
	assert.NotNil(t, walkFindKind(doc, KindAbbreviationDefinition),
		"expected AbbreviationDefinition for `*[HTML]: ...`")
}

func TestAbbreviationReferenceMarkedInParagraph(t *testing.T) {
	src := "*[HTML]: Hyper Text Markup Language\n\nUse HTML here.\n"
	doc := parseWith(t, src, Abbreviation)
	assert.NotNil(t, walkFindKind(doc, KindAbbreviationReference),
		"expected AbbreviationReference for the word 'HTML' inside the paragraph")
}

func TestAbbreviationRequiresDefinedTerm(t *testing.T) {
	// Without a `*[term]: ...` definition, an occurrence in text
	// must not produce an AbbreviationReference node.
	src := "Use HTML here.\n"
	doc := parseWith(t, src, Abbreviation)
	assert.Nil(t, walkFindKind(doc, KindAbbreviationReference))
	assert.Nil(t, walkFindKind(doc, KindAbbreviationDefinition))
}

func TestAbbreviationDoesNotMatchSubstring(t *testing.T) {
	// "HTML" must not match as a suffix of "XHTML" — only whole-
	// word tokens count.
	src := "*[HTML]: Hyper Text Markup Language\n\nUse XHTML here.\n"
	doc := parseWith(t, src, Abbreviation)
	assert.Nil(t, walkFindKind(doc, KindAbbreviationReference),
		"XHTML must not be matched as HTML")
}

func TestAbbreviationMultipleOccurrences(t *testing.T) {
	src := "*[API]: Application Programming Interface\n\nAPI here; API there.\n"
	doc := parseWith(t, src, Abbreviation)
	assert.Equal(t, 2, countKind(doc, KindAbbreviationReference),
		"both API occurrences should be marked")
}

func TestAbbreviationInsideCodeIgnored(t *testing.T) {
	// Occurrences inside inline code must not be marked.
	src := "*[HTML]: Hyper Text Markup Language\n\nUse `HTML` here.\n"
	doc := parseWith(t, src, Abbreviation)
	assert.Nil(t, walkFindKind(doc, KindAbbreviationReference))
}
