package markdownlint

import (
	"fmt"
	"math"
)

// optionSpec translates one markdownlint option value into mdsmith
// settings. It writes into settings and returns "" on success, or a
// human-readable reason when the value does not translate.
type optionSpec func(v any, settings map[string]any) string

// optionTable maps a markdownlint rule id to the translation of each of
// its options. Options absent from a rule's entry (and rules absent
// from the table) surface as "no mdsmith equivalent" notes. The setting
// keys are pinned to the live rules by TestConvert_SettingsApplyCleanly.
var optionTable = map[string]map[string]optionSpec{
	"MD003": {"style": enumOpt("style", map[string]string{
		"atx": "atx", "setext": "setext",
	}, "atx, setext")},
	"MD004": {"style": enumOpt("style", map[string]string{
		"asterisk": "asterisk", "dash": "dash", "plus": "plus",
	}, "asterisk, dash, plus")},
	"MD007": {"indent": intOpt("spaces")},
	"MD012": {"maximum": intOpt("max")},
	"MD013": {
		"line_length":            intOpt("max"),
		"heading_line_length":    intOpt("heading-max"),
		"code_block_line_length": intOpt("code-block-max"),
		"stern":                  boolOpt("stern"),
		"code_blocks":            excludeToggle("code-blocks"),
		"tables":                 excludeToggle("tables"),
	},
	"MD025": {"front_matter_title": md025FrontMatterTitle},
	"MD029": {"style": enumOpt("style", map[string]string{
		"one": "all-ones", "ordered": "sequential",
	}, "one, ordered")},
	"MD030": {
		"ul_single": intOpt("ul-single"),
		"ul_multi":  intOpt("ul-multi"),
		"ol_single": intOpt("ol-single"),
		"ol_multi":  intOpt("ol-multi"),
	},
	"MD033": {"allowed_elements": stringListOpt("allow")},
	"MD035": {"style": md035Style},
	"MD041": {"level": intOpt("level")},
	"MD044": {
		"names":         stringListOpt("names"),
		"code_blocks":   boolOpt("check-code"),
		"html_elements": boolOpt("check-html"),
	},
	"MD046": {"style": enumOpt("style", map[string]string{
		"fenced": "fenced", "indented": "indented", "consistent": "consistent",
	}, "fenced, indented, consistent")},
	"MD048": {"style": enumOpt("style", map[string]string{
		"backtick": "backtick", "tilde": "tilde",
	}, "backtick, tilde")},
	"MD049": {"style": enumOpt("italic", map[string]string{
		"asterisk": "asterisk", "underscore": "underscore",
	}, "asterisk, underscore")},
	"MD050": {"style": enumOpt("bold", map[string]string{
		"asterisk": "asterisk", "underscore": "underscore",
	}, "asterisk, underscore")},
	"MD052": {"shortcut_syntax": md052Shortcut},
	"MD053": {"ignored_definitions": stringListOpt("ignored-labels")},
	"MD055": {"style": enumOpt("style", map[string]string{
		"consistent":             "consistent",
		"leading_and_trailing":   "leading_and_trailing",
		"no_leading_or_trailing": "no_leading_or_trailing",
	}, "consistent, leading_and_trailing, no_leading_or_trailing")},
	"MD059": {"prohibited_texts": stringListOpt("banned")},
}

// lineLengthDefaultExclude mirrors the line-length rule's default
// exclude list; excludeToggle edits a copy of it so a single toggle
// still yields the full effective list (list settings replace, not
// merge). TestLineLengthDefaultExclude pins it to the rule's
// DefaultSettings.
var lineLengthDefaultExclude = []string{"code-blocks", "tables", "urls"}

// intOpt copies an integer option to the named mdsmith setting.
func intOpt(key string) optionSpec {
	return func(v any, settings map[string]any) string {
		n, ok := asInt(v)
		if !ok {
			return fmt.Sprintf("expected an integer, got %v", v)
		}
		settings[key] = n
		return ""
	}
}

// boolOpt copies a boolean option to the named mdsmith setting.
func boolOpt(key string) optionSpec {
	return func(v any, settings map[string]any) string {
		b, ok := v.(bool)
		if !ok {
			return fmt.Sprintf("expected a boolean, got %v", v)
		}
		settings[key] = b
		return ""
	}
}

// stringListOpt copies a string-list option to the named mdsmith
// setting.
func stringListOpt(key string) optionSpec {
	return func(v any, settings map[string]any) string {
		ss, ok := asStringSlice(v)
		if !ok {
			return fmt.Sprintf("expected a list of strings, got %v", v)
		}
		settings[key] = ss
		return ""
	}
}

// enumOpt translates a string-valued option through values; the keys it
// does not list are reported with the accepted set.
func enumOpt(key string, values map[string]string, accepts string) optionSpec {
	return func(v any, settings map[string]any) string {
		sv, ok := v.(string)
		if !ok {
			return fmt.Sprintf("expected a string, got %v", v)
		}
		out, ok := values[sv]
		if !ok {
			return fmt.Sprintf("value %q has no mdsmith equivalent (accepts: %s)", sv, accepts)
		}
		settings[key] = out
		return ""
	}
}

// excludeToggle translates MD013's code_blocks/tables booleans into
// membership of line-length's exclude list. markdownlint `true` means
// "check this construct", so it removes the member; `false` adds it.
func excludeToggle(member string) optionSpec {
	return func(v any, settings map[string]any) string {
		b, ok := v.(bool)
		if !ok {
			return fmt.Sprintf("expected a boolean, got %v", v)
		}
		exclude, _ := settings["exclude"].([]string)
		if exclude == nil {
			exclude = append([]string(nil), lineLengthDefaultExclude...)
		}
		if b {
			exclude = removeString(exclude, member)
		} else {
			exclude = ensureString(exclude, member)
		}
		settings["exclude"] = exclude
		return ""
	}
}

// md035Style translates MD035's literal style string ("---", "***", …)
// into horizontal-rule-style's style + length pair.
func md035Style(v any, settings map[string]any) string {
	sv, ok := v.(string)
	if !ok {
		return fmt.Sprintf("expected a string, got %v", v)
	}
	style, length, ok := parseMD035Style(sv)
	if !ok {
		return fmt.Sprintf(`value %q has no mdsmith equivalent (use a literal rule like "---", "***", or "___")`, sv)
	}
	settings["style"] = style
	settings["length"] = length
	return ""
}

// parseMD035Style decodes a literal horizontal-rule string of at least
// three repeated `-`, `*`, or `_` characters into the mdsmith style
// name and length.
func parseMD035Style(s string) (style string, length int, ok bool) {
	if len(s) < 3 {
		return "", 0, false
	}
	c := s[0]
	for i := 0; i < len(s); i++ {
		if s[i] != c {
			return "", 0, false
		}
	}
	switch c {
	case '-':
		return "dash", len(s), true
	case '*':
		return "asterisk", len(s), true
	case '_':
		return "underscore", len(s), true
	}
	return "", 0, false
}

// md025FrontMatterTitle translates MD025's front_matter_title. Only the
// empty value (ignore front matter) has an mdsmith equivalent: mdsmith
// names a front-matter key, markdownlint matches a pattern.
func md025FrontMatterTitle(v any, settings map[string]any) string {
	sv, ok := v.(string)
	if !ok {
		return fmt.Sprintf("expected a string, got %v", v)
	}
	if sv != "" {
		return `only "" (ignore front matter) translates; set` +
			` rules.single-h1.front-matter-title to a front-matter key instead`
	}
	settings["front-matter-title"] = ""
	return ""
}

// md052Shortcut translates MD052's shortcut_syntax boolean into
// no-undefined-reference-labels' shortcut mode: checking shortcut
// references maps to "always", leaving them alone to "collapsed-only".
func md052Shortcut(v any, settings map[string]any) string {
	b, ok := v.(bool)
	if !ok {
		return fmt.Sprintf("expected a boolean, got %v", v)
	}
	if b {
		settings["shortcut"] = "always"
	} else {
		settings["shortcut"] = "collapsed-only"
	}
	return ""
}

// asInt accepts int and integral float64 values (YAML and JSON parsers
// produce either).
func asInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case float64:
		if n == math.Trunc(n) {
			return int(n), true
		}
	}
	return 0, false
}

// asStringSlice accepts a []any whose every element is a string.
func asStringSlice(v any) ([]string, bool) {
	items, ok := v.([]any)
	if !ok {
		return nil, false
	}
	out := make([]string, 0, len(items))
	for _, it := range items {
		s, ok := it.(string)
		if !ok {
			return nil, false
		}
		out = append(out, s)
	}
	return out, true
}

// removeString returns list without the first occurrence of s.
func removeString(list []string, s string) []string {
	for i, v := range list {
		if v == s {
			return append(list[:i], list[i+1:]...)
		}
	}
	return list
}

// ensureString returns list with s appended unless already present.
func ensureString(list []string, s string) []string {
	for _, v := range list {
		if v == s {
			return list
		}
	}
	return append(list, s)
}
