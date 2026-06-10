// Package markdownlint converts a markdownlint config file
// (.markdownlint.jsonc/.json/.yaml/.yml or .markdownlintrc) into mdsmith
// rule configuration. The MD###-to-mdsmith rule mapping comes from the
// `markdownlint:` front matter embedded in each rule README
// (internal/rules), so the converter and the published coverage docs
// share one source of truth. Only per-rule option names live here, in
// the option-translation table in convert.go.
package markdownlint

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jeduden/mdsmith/internal/yamlutil"
)

// configNames lists the file names markdownlint-cli itself probes for,
// in its probe order. Discover walks the same list.
var configNames = []string{
	".markdownlint.jsonc",
	".markdownlint.json",
	".markdownlint.yaml",
	".markdownlint.yml",
	".markdownlintrc",
}

// Parse decodes a markdownlint config into a key-to-value map. JSON and
// JSONC payloads (first non-space byte `{`) get their comments and
// trailing commas stripped first; everything then goes through the YAML
// parser, which accepts both YAML and plain JSON.
func Parse(data []byte) (map[string]any, error) {
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return nil, errors.New("markdownlint config is empty")
	}
	if trimmed[0] == '{' {
		data = stripJSONC(data)
	}

	var raw any
	if err := yamlutil.UnmarshalSafe(data, &raw); err != nil {
		return nil, fmt.Errorf("parsing markdownlint config: %w", err)
	}
	m, ok := raw.(map[string]any)
	if !ok {
		return nil, errors.New("markdownlint config root must be a mapping")
	}
	return m, nil
}

// stripJSONC turns a JSONC payload into plain JSON-shaped bytes by
// removing // and /* */ comments and trailing commas. String literals
// pass through verbatim, including escaped quotes. Newlines inside
// block comments are kept so YAML parse errors still point near the
// right line.
func stripJSONC(data []byte) []byte {
	return stripTrailingCommas(stripComments(data))
}

// stripComments removes // line comments and /* */ block comments
// outside string literals.
func stripComments(data []byte) []byte {
	out := make([]byte, 0, len(data))
	for i := 0; i < len(data); {
		c := data[i]
		switch {
		case c == '"':
			out, i = copyString(out, data, i)
		case c == '/' && i+1 < len(data) && data[i+1] == '/':
			for i < len(data) && data[i] != '\n' {
				i++
			}
		case c == '/' && i+1 < len(data) && data[i+1] == '*':
			i += 2
			for i < len(data) {
				if data[i] == '*' && i+1 < len(data) && data[i+1] == '/' {
					i += 2
					break
				}
				if data[i] == '\n' {
					out = append(out, '\n')
				}
				i++
			}
		default:
			out = append(out, c)
			i++
		}
	}
	return out
}

// stripTrailingCommas removes a comma whose next non-whitespace byte is
// a closing brace or bracket, outside string literals.
func stripTrailingCommas(data []byte) []byte {
	out := make([]byte, 0, len(data))
	for i := 0; i < len(data); {
		c := data[i]
		if c == '"' {
			out, i = copyString(out, data, i)
			continue
		}
		if c == ',' {
			j := i + 1
			for j < len(data) && (data[j] == ' ' || data[j] == '\t' || data[j] == '\n' || data[j] == '\r') {
				j++
			}
			if j < len(data) && (data[j] == '}' || data[j] == ']') {
				i++
				continue
			}
		}
		out = append(out, c)
		i++
	}
	return out
}

// copyString copies the double-quoted string literal starting at
// data[i] (which must be `"`) to out, honoring backslash escapes.
// It returns the extended buffer and the index just past the closing
// quote (or len(data) for an unterminated literal).
func copyString(out, data []byte, i int) ([]byte, int) {
	out = append(out, data[i])
	i++
	for i < len(data) {
		out = append(out, data[i])
		if data[i] == '\\' && i+1 < len(data) {
			out = append(out, data[i+1])
			i += 2
			continue
		}
		if data[i] == '"' {
			i++
			break
		}
		i++
	}
	return out, i
}

// Discover returns the path of the first markdownlint config file found
// in dir, probing the same names markdownlint-cli does.
func Discover(dir string) (string, error) {
	for _, name := range configNames {
		p := filepath.Join(dir, name)
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p, nil
		}
	}
	return "", fmt.Errorf("no markdownlint config found in %s (looked for %s)",
		dir, strings.Join(configNames, ", "))
}
