package lint

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"gopkg.in/yaml.v3"
)

// RejectYAMLAliases parses YAML into a node tree and returns an error if any
// anchor or alias is found. Decoding into yaml.Node does not expand aliases,
// so this is safe even for billion-laughs payloads. This check should be
// called before yaml.Unmarshal on user-supplied content.
func RejectYAMLAliases(data []byte) error {
	if len(data) == 0 {
		return nil
	}

	dec := yaml.NewDecoder(bytes.NewReader(data))
	for {
		var doc yaml.Node
		err := dec.Decode(&doc)
		if err == io.EOF {
			return nil
		}
		if err != nil {
			// An undefined alias causes a parse error containing "unknown anchor".
			// Reject this as evidence of alias usage.
			if strings.Contains(err.Error(), "unknown anchor") {
				return fmt.Errorf("yaml anchors/aliases are not permitted")
			}
			// Other syntax errors are handled by the caller's yaml.Unmarshal.
			return nil
		}

		if hasYAMLAnchorOrAlias(&doc) {
			return fmt.Errorf("yaml anchors/aliases are not permitted")
		}
	}
}

func hasYAMLAnchorOrAlias(node *yaml.Node) bool {
	if node.Anchor != "" || node.Kind == yaml.AliasNode {
		return true
	}
	for _, child := range node.Content {
		if hasYAMLAnchorOrAlias(child) {
			return true
		}
	}
	return false
}
