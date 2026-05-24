package schema

import "fmt"

// fieldMetaKeys lists the keys plan 136 recognises inside the
// metadata-form mapping. Any other key triggers a parse error so a
// typo (e.g. `replacedby:`) surfaces at config-load time rather
// than silently dropping the hint.
var fieldMetaKeys = map[string]bool{
	"type":        true,
	"deprecated":  true,
	"message":     true,
	"replaced-by": true,
}

// ExtractFieldMeta inspects a frontmatter value and, when it
// matches plan 136's metadata form, returns the embedded CUE
// expression (from `type:`), the parsed FieldMeta, and isMeta=true.
//
// The metadata form is a YAML mapping carrying `type:` plus an
// explicit `deprecated: true` discriminator:
//
//	legacy_owner:
//	  type: string
//	  deprecated: true
//	  message: 'use "owner" instead'
//
// Requiring the literal `true` value (not just the `deprecated`
// key) means a CUE struct constraint that happens to bind a
// `type` field (and optionally a `deprecated` field) flows through
// to the JSON-encoded struct path unchanged. Two corollaries the
// reserved-key contract surfaces:
//
//   - `{type, deprecated: false}` and `{type}` alone are CUE struct
//     constraints, not metadata.
//   - `message:` / `replaced-by:` without `deprecated: true` is
//     almost always a typo; the parser surfaces it as an error
//     rather than reinterpreting the mapping as a CUE struct.
func ExtractFieldMeta(v any) (string, FieldMeta, bool, error) {
	m, ok := v.(map[string]any)
	if !ok {
		return "", FieldMeta{}, false, nil
	}
	if _, hasType := m["type"]; !hasType {
		return "", FieldMeta{}, false, nil
	}
	isMeta, err := isMetadataDiscriminator(m)
	if err != nil {
		return "", FieldMeta{}, false, err
	}
	if !isMeta {
		return "", FieldMeta{}, false, nil
	}
	expr, err := frontmatterExpr(m["type"])
	if err != nil {
		return "", FieldMeta{}, false, fmt.Errorf("type: %w", err)
	}
	meta := FieldMeta{Deprecated: true}
	for k, vv := range m {
		if k == "type" || k == "deprecated" {
			continue
		}
		if !fieldMetaKeys[k] {
			return "", FieldMeta{}, false, fmt.Errorf(
				"unknown field-meta key %q (valid: type, deprecated, "+
					"message, replaced-by)", k)
		}
		if err := applyFieldMetaKey(&meta, k, vv); err != nil {
			return "", FieldMeta{}, false, err
		}
	}
	return expr, meta, true, nil
}

// isMetadataDiscriminator decides whether a mapping with `type:`
// should be parsed as plan-136 metadata. The literal
// `deprecated: true` is the only positive signal. A non-bool
// `deprecated:` value, or a hint key (`message:` / `replaced-by:`)
// without `deprecated: true`, surfaces as a typo error so a likely
// authoring mistake does not silently fall through to the CUE
// struct path.
func isMetadataDiscriminator(m map[string]any) (bool, error) {
	depRaw, hasDep := m["deprecated"]
	if hasDep {
		depBool, isBool := depRaw.(bool)
		if !isBool {
			return false, fmt.Errorf(
				"deprecated must be a boolean, got %T", depRaw)
		}
		if depBool {
			return true, nil
		}
	}
	if _, has := m["message"]; has {
		return false, hintWithoutDeprecatedError()
	}
	if _, has := m["replaced-by"]; has {
		return false, hintWithoutDeprecatedError()
	}
	return false, nil
}

func hintWithoutDeprecatedError() error {
	return fmt.Errorf(
		"`message:` and `replaced-by:` require `deprecated: true` " +
			"— remove the hint or mark the field deprecated")
}

func applyFieldMetaKey(meta *FieldMeta, k string, vv any) error {
	switch k {
	case "message":
		s, ok := vv.(string)
		if !ok {
			return fmt.Errorf("message must be a string, got %T", vv)
		}
		meta.Message = s
	case "replaced-by":
		s, ok := vv.(string)
		if !ok {
			return fmt.Errorf("replaced-by must be a string, got %T", vv)
		}
		meta.ReplacedBy = s
	}
	return nil
}
