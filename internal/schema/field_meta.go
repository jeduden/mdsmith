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
// The metadata form is a YAML mapping carrying both a `type:` key
// AND at least one of `deprecated:` / `message:` / `replaced-by:`:
//
//	legacy_owner:
//	  type: string
//	  deprecated: true
//	  message: 'use "owner" instead'
//
// The combined signal disambiguates the metadata form from a
// genuine CUE struct constraint that happens to have a `type`
// field. A mapping without `type:`, or with `type:` alone, returns
// isMeta=false so the caller falls through to its existing
// frontmatterExpr handling (JSON-encoded CUE struct).
func ExtractFieldMeta(v any) (string, FieldMeta, bool, error) {
	m, ok := v.(map[string]any)
	if !ok {
		return "", FieldMeta{}, false, nil
	}
	if _, hasType := m["type"]; !hasType {
		return "", FieldMeta{}, false, nil
	}
	if !hasDeprecationKey(m) {
		// `type:` alone is ambiguous: it could be the start of a
		// plan-136 metadata mapping with the rest of the keys yet
		// to be added, or a CUE struct constraint whose schema
		// happens to bind a `type` field. Without a deprecation
		// signal we cannot tell, so fall through to the CUE-struct
		// encoder rather than silently dropping the other keys.
		return "", FieldMeta{}, false, nil
	}
	expr, err := frontmatterExpr(m["type"])
	if err != nil {
		return "", FieldMeta{}, false, fmt.Errorf("type: %w", err)
	}
	meta := FieldMeta{}
	for k, vv := range m {
		if k == "type" {
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
	if !meta.Deprecated && (meta.Message != "" || meta.ReplacedBy != "") {
		return "", FieldMeta{}, false, fmt.Errorf(
			"`message:` and `replaced-by:` require `deprecated: true` " +
				"— remove the hint or mark the field deprecated")
	}
	return expr, meta, true, nil
}

// hasDeprecationKey reports whether m carries any of the three
// keys that signal "this is a plan-136 metadata mapping, not a
// CUE struct constraint that happens to set `type:`". Presence of
// the key — not its value — is the discriminator.
func hasDeprecationKey(m map[string]any) bool {
	if _, ok := m["deprecated"]; ok {
		return true
	}
	if _, ok := m["message"]; ok {
		return true
	}
	if _, ok := m["replaced-by"]; ok {
		return true
	}
	return false
}

func applyFieldMetaKey(meta *FieldMeta, k string, vv any) error {
	switch k {
	case "deprecated":
		b, ok := vv.(bool)
		if !ok {
			return fmt.Errorf("deprecated must be a boolean, got %T", vv)
		}
		meta.Deprecated = b
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
