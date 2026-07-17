// Package placeholders provides a shared vocabulary of placeholder tokens
// that rules can opt into to treat template content as opaque rather than
// content violations.
//
// A rule adds a `placeholders:` setting (a list of token names) via the
// Configurable interface. When checking a node it calls ContainsBodyToken
// or MaskBodyTokens to decide whether to skip or neutralize the content.
//
// The five tokens are:
//
//   - var-token           — {identifier} interpolation placeholders
//   - heading-question    — headings whose text is exactly "?"
//   - placeholder-section — headings whose text is exactly "..."
//   - cue-frontmatter     — CUE constraint expressions in front-matter values
//   - apm-input-token     — APM ${input:NAME} prompt parameters
package placeholders

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/jeduden/mdsmith/internal/fieldinterp"
)

// apmInputRe matches APM ${input:NAME} prompt parameters.
// NAME must start with a letter and contain only letters, digits, underscores, and hyphens.
var apmInputRe = regexp.MustCompile(`\$\{input:[A-Za-z][\w-]{0,63}\}`)

// Named placeholder tokens.
const (
	VarToken           = "var-token"
	HeadingQuestion    = "heading-question"
	PlaceholderSection = "placeholder-section"
	CUEFrontmatter     = "cue-frontmatter"
	APMInputToken      = "apm-input-token"
)

// neutralText is the neutral replacement per body token.
var neutralText = map[string]string{
	VarToken:           "word",
	HeadingQuestion:    "Placeholder",
	PlaceholderSection: "Placeholder Section",
	APMInputToken:      "word",
}

// IsKnown reports whether name is a recognized token name.
func IsKnown(name string) bool {
	switch name {
	case VarToken, HeadingQuestion, PlaceholderSection, CUEFrontmatter, APMInputToken:
		return true
	}
	return false
}

// Validate returns an error if any token name in the list is unknown.
func Validate(tokens []string) error {
	for _, tok := range tokens {
		if !IsKnown(tok) {
			return fmt.Errorf("unknown placeholder token %q", tok)
		}
	}
	return nil
}

// ContainsBodyToken reports whether text matches any of the named body
// tokens. cue-frontmatter is a front-matter–only token and is ignored.
func ContainsBodyToken(text string, tokens []string) bool {
	for _, tok := range tokens {
		switch tok {
		case VarToken:
			if fieldinterp.ContainsField(text) {
				return true
			}
		case HeadingQuestion:
			if strings.TrimSpace(text) == "?" {
				return true
			}
		case PlaceholderSection:
			if strings.TrimSpace(text) == "..." {
				return true
			}
		case APMInputToken:
			if apmInputRe.MatchString(text) {
				return true
			}
		}
	}
	return false
}

// MaskBodyTokens replaces placeholder token patterns in text with neutral
// content. cue-frontmatter is not a body token and is left unchanged.
// Whole-text tokens (heading-question, placeholder-section) replace the
// entire text when they match; substring tokens (var-token, apm-input-token)
// replace each occurrence in place.
func MaskBodyTokens(text string, tokens []string) string {
	for _, tok := range tokens {
		switch tok {
		case VarToken:
			// Replace all {field} occurrences with a neutral word.
			text = replaceVarTokens(text)
		case HeadingQuestion:
			if strings.TrimSpace(text) == "?" {
				return neutralText[HeadingQuestion]
			}
		case PlaceholderSection:
			if strings.TrimSpace(text) == "..." {
				return neutralText[PlaceholderSection]
			}
		case APMInputToken:
			text = apmInputRe.ReplaceAllLiteralString(text, neutralText[APMInputToken])
		}
	}
	return text
}

// IsAllBodyTokens reports whether text (trimmed) consists only of
// placeholder token patterns, with no other content. Unlike MaskBodyTokens,
// this strips placeholder patterns to empty rather than replacing with
// neutral text, so only non-placeholder content remains.
func IsAllBodyTokens(text string, tokens []string) bool {
	stripped := stripBodyTokens(text, tokens)
	return strings.TrimSpace(stripped) == ""
}

// stripBodyTokens removes all placeholder token patterns from text,
// leaving only non-placeholder content (for IsAllBodyTokens).
func stripBodyTokens(text string, tokens []string) string {
	for _, tok := range tokens {
		switch tok {
		case VarToken:
			if !fieldinterp.ContainsField(text) {
				continue
			}
			parts := fieldinterp.SplitOnFields(text)
			text = strings.Join(parts, "")
		case HeadingQuestion:
			if strings.TrimSpace(text) == "?" {
				return ""
			}
		case PlaceholderSection:
			if strings.TrimSpace(text) == "..." {
				return ""
			}
		case APMInputToken:
			text = apmInputRe.ReplaceAllLiteralString(text, "")
		}
	}
	return text
}

// HasCUEFrontmatter reports whether cue-frontmatter is in the token list.
func HasCUEFrontmatter(tokens []string) bool {
	for _, tok := range tokens {
		if tok == CUEFrontmatter {
			return true
		}
	}
	return false
}

// replaceVarTokens replaces all {field} interpolation placeholders with
// the neutral word "word". It delegates detection to fieldinterp and uses
// a simple split/join over field boundaries.
func replaceVarTokens(text string) string {
	if !fieldinterp.ContainsField(text) {
		return text
	}
	// Split on field boundaries: the parts between placeholders are
	// kept, the placeholders are replaced.
	parts := fieldinterp.SplitOnFields(text)
	placeholderCount := len(parts) - 1

	var b strings.Builder
	for i, part := range parts {
		b.WriteString(part)
		if i < placeholderCount {
			b.WriteString(neutralText[VarToken])
		}
	}
	return b.String()
}
