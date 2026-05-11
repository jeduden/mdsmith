package config

import "fmt"

// ValidateKinds returns an error if any kind named in a kind-assignment
// entry is not declared in cfg.Kinds, or if any declared kind sets a
// schema both inline (KindBody.Schema) and via the legacy
// rules.required-structure.schema: path. Front-matter kinds are
// validated at lint time via ValidateFrontMatterKinds (see engine).
func ValidateKinds(cfg *Config) error {
	if len(cfg.Kinds) == 0 && len(cfg.KindAssignment) == 0 {
		return nil
	}
	for name, body := range cfg.Kinds {
		if err := validateKindSchemaSources(name, body); err != nil {
			return err
		}
	}
	for i, entry := range cfg.KindAssignment {
		for _, name := range entry.Kinds {
			if _, ok := cfg.Kinds[name]; !ok {
				return fmt.Errorf(
					"kind-assignment[%d]: references undeclared kind %q", i, name,
				)
			}
		}
	}
	return nil
}

// validateKindSchemaSources rejects a kind that declares more than
// one schema source. The three forms that conflict pairwise:
//
//   - `kinds.<name>.schema:` (inline block on KindBody.Schema)
//   - `kinds.<name>.rules.required-structure.schema:` (file path)
//   - `kinds.<name>.rules.required-structure.inline-schema:`
//     (inline map under the rule settings)
//
// Any two of these on the same kind make the effective schema
// ambiguous; the validator surfaces the conflict at load time with
// a message naming both sources.
func validateKindSchemaSources(name string, body KindBody) error {
	rsCfg, hasRS := body.Rules["required-structure"]
	pathSet, pathSetting := schemaPathSetting(rsCfg, hasRS)
	inlineSet, _ := schemaInlineSetting(rsCfg, hasRS)

	if len(body.Schema) > 0 && pathSet {
		return fmt.Errorf(
			"kind %q: schema is declared both inline (kinds.%s.schema:) "+
				"and as a file (kinds.%s.rules.required-structure.schema: %q); "+
				"pick one source",
			name, name, name, pathSetting)
	}
	if len(body.Schema) > 0 && inlineSet {
		return fmt.Errorf(
			"kind %q: schema is declared both inline (kinds.%s.schema:) "+
				"and under kinds.%s.rules.required-structure.inline-schema:; "+
				"pick one source — keep the top-level kinds.%s.schema: block",
			name, name, name, name)
	}
	if pathSet && inlineSet {
		return fmt.Errorf(
			"kind %q: required-structure has both `schema:` (%q) and "+
				"`inline-schema:` set under kinds.%s.rules.required-structure; "+
				"pick one source",
			name, pathSetting, name)
	}
	return nil
}

func schemaPathSetting(rs RuleCfg, hasRS bool) (bool, string) {
	if !hasRS || rs.Settings == nil {
		return false, ""
	}
	v, ok := rs.Settings["schema"]
	if !ok {
		return false, ""
	}
	s, ok := v.(string)
	if !ok || s == "" {
		return false, ""
	}
	return true, s
}

func schemaInlineSetting(rs RuleCfg, hasRS bool) (bool, map[string]any) {
	if !hasRS || rs.Settings == nil {
		return false, nil
	}
	m, ok := rs.Settings["inline-schema"].(map[string]any)
	if !ok || len(m) == 0 {
		return false, nil
	}
	return true, m
}

// ValidateFrontMatterKinds returns an error if any of the supplied front-matter
// kind names is not declared in cfg.Kinds. filePath is used in the message.
func ValidateFrontMatterKinds(cfg *Config, filePath string, kinds []string) error {
	for _, name := range kinds {
		if _, ok := cfg.Kinds[name]; !ok {
			return fmt.Errorf(
				"%s: front matter references undeclared kind %q", filePath, name,
			)
		}
	}
	return nil
}
