package lint

import (
	"fmt"
	"regexp"
)

// anchorRe matches a YAML anchor definition: & followed by an identifier,
// where & is preceded by whitespace or is at line start (structural position).
var anchorRe = regexp.MustCompile(`(?m)(^|[ \t])&\w`)

// aliasRe matches a YAML alias reference: * followed by an identifier,
// where * is preceded by whitespace, line start, or [ (structural position).
var aliasRe = regexp.MustCompile(`(?m)(^|[ \t[,])(\*\w)`)

// quotedStringRe matches single- or double-quoted strings.
var quotedStringRe = regexp.MustCompile(`"[^"]*"|'[^']*'`)

// RejectYAMLAliases scans raw YAML bytes for anchor (&name) or alias (*name)
// syntax and returns an error if found. This prevents exponential memory
// expansion (billion laughs) during yaml.Unmarshal.
//
// Characters & and * inside quoted string values or mid-word in plain
// scalars are ignored to avoid false positives on content like "Q&A".
func RejectYAMLAliases(data []byte) error {
	if len(data) == 0 {
		return nil
	}

	// Remove quoted strings so we don't match & or * inside them.
	stripped := quotedStringRe.ReplaceAll(data, nil)

	if anchorRe.Match(stripped) || aliasRe.Match(stripped) {
		return fmt.Errorf("YAML anchors/aliases are not permitted")
	}
	return nil
}
