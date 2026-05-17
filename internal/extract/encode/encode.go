// Package encode serialises an extracted data tree into one of the
// supported wire formats. JSON, YAML, and msgpack are equivalent
// projections of the same tree; a Lua encoder is deferred (plan
// 166) but would slot in behind the same Format enum.
package encode

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/vmihailenco/msgpack/v5"
	"gopkg.in/yaml.v3"
)

// Format names a supported output encoding.
type Format string

// Supported formats.
const (
	JSON    Format = "json"
	YAML    Format = "yaml"
	Msgpack Format = "msgpack"
)

// ParseFormat maps a CLI `--format` value to a Format. The error
// lists the accepted values so the caller can surface a clear
// message.
func ParseFormat(s string) (Format, error) {
	switch Format(s) {
	case JSON, YAML, Msgpack:
		return Format(s), nil
	}
	return "", fmt.Errorf(
		"unknown format %q (want json, yaml, or msgpack)", s)
}

// Encode serialises v in the requested format. JSON is indented
// two spaces and newline-terminated for human-readable CLI output;
// YAML uses a two-space indent; msgpack is the canonical binary
// form.
func Encode(f Format, v any) ([]byte, error) {
	switch f {
	case JSON:
		var buf bytes.Buffer
		enc := json.NewEncoder(&buf)
		enc.SetIndent("", "  ")
		enc.SetEscapeHTML(false)
		if err := enc.Encode(v); err != nil {
			return nil, err
		}
		return buf.Bytes(), nil
	case YAML:
		var buf bytes.Buffer
		enc := yaml.NewEncoder(&buf)
		enc.SetIndent(2)
		if err := enc.Encode(v); err != nil {
			return nil, err
		}
		_ = enc.Close()
		return buf.Bytes(), nil
	case Msgpack:
		return msgpack.Marshal(v)
	}
	return nil, fmt.Errorf("unknown format %q", f)
}
