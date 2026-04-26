package lint

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// ParseFrontMatterKinds extracts the `kinds:` list field from the YAML
// front matter of a file. The fmBlock argument is the raw front matter
// block (including its `---` delimiters) as stored in File.FrontMatter.
//
// Returns:
//   - nil, nil — when there is no front matter, no `kinds:` key, or the
//     value is null.
//   - []string, nil — the list of kind names.
//   - nil, error — when the YAML cannot be parsed or `kinds:` is not a
//     list of strings.
func ParseFrontMatterKinds(fmBlock []byte) ([]string, error) {
	if len(fmBlock) == 0 {
		return nil, nil
	}

	yamlBytes := extractFrontMatterYAML(fmBlock)
	if len(yamlBytes) == 0 {
		return nil, nil
	}

	if err := RejectYAMLAliases(yamlBytes); err != nil {
		return nil, fmt.Errorf("front matter: %w", err)
	}

	var raw map[string]any
	if err := yaml.Unmarshal(yamlBytes, &raw); err != nil {
		return nil, fmt.Errorf("front matter: invalid YAML: %w", err)
	}

	v, ok := raw["kinds"]
	if !ok || v == nil {
		return nil, nil
	}

	list, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("front matter: `kinds` must be a list of strings")
	}
	out := make([]string, 0, len(list))
	for i, item := range list {
		s, ok := item.(string)
		if !ok {
			return nil, fmt.Errorf("front matter: `kinds[%d]` must be a string", i)
		}
		out = append(out, s)
	}
	return out, nil
}

// extractFrontMatterYAML returns the YAML body between "---\n" delimiters
// of a front matter block. It mirrors the helper used by other front
// matter readers in the codebase.
func extractFrontMatterYAML(fmBlock []byte) []byte {
	const delim = "---\n"
	if len(fmBlock) < len(delim) {
		return nil
	}
	if string(fmBlock[:len(delim)]) != delim {
		return nil
	}
	rest := fmBlock[len(delim):]
	// Find the closing "---\n".
	for i := 0; i+len(delim) <= len(rest); i++ {
		if string(rest[i:i+len(delim)]) == delim {
			return rest[:i]
		}
	}
	// Try without trailing newline.
	if len(rest) >= 3 && string(rest[len(rest)-3:]) == "---" {
		return rest[:len(rest)-3]
	}
	return nil
}
