package placeholders_test

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/placeholders"
	"github.com/stretchr/testify/assert"
)

func TestIsKnown(t *testing.T) {
	assert.True(t, placeholders.IsKnown(placeholders.VarToken))
	assert.True(t, placeholders.IsKnown(placeholders.HeadingQuestion))
	assert.True(t, placeholders.IsKnown(placeholders.PlaceholderSection))
	assert.True(t, placeholders.IsKnown(placeholders.CUEFrontmatter))
	assert.False(t, placeholders.IsKnown("unknown-token"))
	assert.False(t, placeholders.IsKnown(""))
}

func TestValidate(t *testing.T) {
	assert.NoError(t, placeholders.Validate(nil))
	assert.NoError(t, placeholders.Validate([]string{}))
	assert.NoError(t, placeholders.Validate([]string{placeholders.VarToken}))
	assert.NoError(t, placeholders.Validate([]string{
		placeholders.VarToken,
		placeholders.HeadingQuestion,
		placeholders.PlaceholderSection,
		placeholders.CUEFrontmatter,
	}))
	err := placeholders.Validate([]string{"bad-token"})
	assert.ErrorContains(t, err, `unknown placeholder token "bad-token"`)
}

func TestContainsBodyToken(t *testing.T) {
	tests := []struct {
		name   string
		text   string
		tokens []string
		want   bool
	}{
		{
			name:   "var-token detected",
			text:   "Hello {name} world",
			tokens: []string{placeholders.VarToken},
			want:   true,
		},
		{
			name:   "var-token nested path",
			text:   "{a.b.c}",
			tokens: []string{placeholders.VarToken},
			want:   true,
		},
		{
			name:   "heading-question exact",
			text:   "?",
			tokens: []string{placeholders.HeadingQuestion},
			want:   true,
		},
		{
			name:   "heading-question with whitespace",
			text:   "  ?  ",
			tokens: []string{placeholders.HeadingQuestion},
			want:   true,
		},
		{
			name:   "heading-question not a partial match",
			text:   "What?",
			tokens: []string{placeholders.HeadingQuestion},
			want:   false,
		},
		{
			name:   "placeholder-section exact",
			text:   "...",
			tokens: []string{placeholders.PlaceholderSection},
			want:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := placeholders.ContainsBodyToken(tt.text, tt.tokens)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestContainsBodyToken_SectionAndEdgeCases(t *testing.T) {
	tests := []struct {
		name   string
		text   string
		tokens []string
		want   bool
	}{
		{
			name:   "placeholder-section with whitespace",
			text:   "  ...  ",
			tokens: []string{placeholders.PlaceholderSection},
			want:   true,
		},
		{
			name:   "placeholder-section not partial",
			text:   "more...",
			tokens: []string{placeholders.PlaceholderSection},
			want:   false,
		},
		{
			name:   "cue-frontmatter ignored for body",
			text:   "int & >=1",
			tokens: []string{placeholders.CUEFrontmatter},
			want:   false,
		},
		{
			name:   "empty token list",
			text:   "{var}",
			tokens: []string{},
			want:   false,
		},
		{
			name:   "nil token list",
			text:   "{var}",
			tokens: nil,
			want:   false,
		},
		{
			name:   "multiple tokens, first matches",
			text:   "?",
			tokens: []string{placeholders.HeadingQuestion, placeholders.VarToken},
			want:   true,
		},
		{
			name:   "multiple tokens, second matches",
			text:   "{id}",
			tokens: []string{placeholders.HeadingQuestion, placeholders.VarToken},
			want:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := placeholders.ContainsBodyToken(tt.text, tt.tokens)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestMaskBodyTokens(t *testing.T) {
	tests := []struct {
		name   string
		text   string
		tokens []string
		want   string
	}{
		{
			name:   "var-token replaced",
			text:   "Hello {name} world",
			tokens: []string{placeholders.VarToken},
			want:   "Hello word world",
		},
		{
			name:   "var-token only",
			text:   "{id}",
			tokens: []string{placeholders.VarToken},
			want:   "word",
		},
		{
			name:   "multiple var-tokens",
			text:   "{id}: {title}",
			tokens: []string{placeholders.VarToken},
			want:   "word: word",
		},
		{
			name:   "heading-question replaced",
			text:   "?",
			tokens: []string{placeholders.HeadingQuestion},
			want:   "Placeholder",
		},
		{
			name:   "heading-question not partial",
			text:   "What?",
			tokens: []string{placeholders.HeadingQuestion},
			want:   "What?",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := placeholders.MaskBodyTokens(tt.text, tt.tokens)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestMaskBodyTokens_SectionAndEdgeCases(t *testing.T) {
	tests := []struct {
		name   string
		text   string
		tokens []string
		want   string
	}{
		{
			name:   "placeholder-section replaced",
			text:   "...",
			tokens: []string{placeholders.PlaceholderSection},
			want:   "Placeholder Section",
		},
		{
			name:   "cue-frontmatter not a body token",
			text:   "int & >=1",
			tokens: []string{placeholders.CUEFrontmatter},
			want:   "int & >=1",
		},
		{
			name:   "empty token list leaves text unchanged",
			text:   "{var}",
			tokens: []string{},
			want:   "{var}",
		},
		{
			// fieldinterp treats {{ as escaped {; the text has no {field}
			// placeholders so MaskBodyTokens leaves it unchanged.
			name:   "escaped braces not replaced",
			text:   "{{not-a-placeholder}}",
			tokens: []string{placeholders.VarToken},
			want:   "{{not-a-placeholder}}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := placeholders.MaskBodyTokens(tt.text, tt.tokens)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestIsAllBodyTokens(t *testing.T) {
	tests := []struct {
		name   string
		text   string
		tokens []string
		want   bool
	}{
		{
			name:   "only var-token",
			text:   "{id}",
			tokens: []string{placeholders.VarToken},
			want:   true,
		},
		{
			name:   "var-token with other text",
			text:   "{id} some text",
			tokens: []string{placeholders.VarToken},
			want:   false,
		},
		{
			name:   "heading-question only",
			text:   "?",
			tokens: []string{placeholders.HeadingQuestion},
			want:   true,
		},
		{
			name:   "placeholder-section only",
			text:   "...",
			tokens: []string{placeholders.PlaceholderSection},
			want:   true,
		},
		{
			name:   "empty text",
			text:   "",
			tokens: []string{placeholders.VarToken},
			want:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := placeholders.IsAllBodyTokens(tt.text, tt.tokens)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestHasCUEFrontmatter(t *testing.T) {
	assert.True(t, placeholders.HasCUEFrontmatter([]string{placeholders.CUEFrontmatter}))
	assert.True(t, placeholders.HasCUEFrontmatter([]string{placeholders.VarToken, placeholders.CUEFrontmatter}))
	assert.False(t, placeholders.HasCUEFrontmatter([]string{placeholders.VarToken}))
	assert.False(t, placeholders.HasCUEFrontmatter(nil))
	assert.False(t, placeholders.HasCUEFrontmatter([]string{}))
}
