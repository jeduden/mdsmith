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
// The metadata form is a YAML mapping carrying a `type:` key:
//
//	legacy_owner:
//	  type: string
//	  deprecated: true
//	  message: 'use "owner" instead'
//
// The mapping may also declare `replaced-by:`. Any other key inside
// the mapping triggers an error so a typo surfaces at parse time.
// A non-mapping value, or a mapping without `type:`, returns
// isMeta=false so the caller falls through to its existing
// frontmatterExpr handling (a CUE struct constraint, in the
// nested-mapping case).
func ExtractFieldMeta(v any) (string, FieldMeta, bool, error) {
	m, ok := v.(map[string]any)
	if !ok {
		return "", FieldMeta{}, false, nil
	}
	if _, hasType := m["type"]; !hasType {
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
