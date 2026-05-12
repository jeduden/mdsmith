package schema

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// ParseInline builds a Schema from the YAML-decoded inline form found
// under kinds.<name>.schema: in .mdsmith.yml. The input is the raw
// map[string]any produced by goyaml so callers do not have to share a
// dependency on a specific YAML schema struct.
//
// source is a label used in diagnostics that point back at the schema
// (typically "kind <name>").
//
// Inline schemas default to open scopes (Closed: false) per plan 146.
// The validator's open-scope semantics still enforce required sections
// and listed-section ordering; only unlisted headings are tolerated.
func ParseInline(raw map[string]any, source string) (*Schema, error) {
	if raw == nil {
		return &Schema{Source: source, RootLevel: 2}, nil
	}

	sch := &Schema{Source: source, RootLevel: 2}

	if err := parseInlineFrontmatter(raw, sch); err != nil {
		return nil, err
	}
	if err := parseInlineRequire(raw, sch); err != nil {
		return nil, err
	}
	if err := parseInlineRootClosed(raw, sch); err != nil {
		return nil, err
	}
	if err := parseInlineSections(raw, sch); err != nil {
		return nil, err
	}
	if err := parseInlineCrossReferences(raw, sch); err != nil {
		return nil, err
	}
	if err := parseInlineAcronyms(raw, sch); err != nil {
		return nil, err
	}
	if err := parseInlineIndex(raw, sch); err != nil {
		return nil, err
	}

	if err := rejectUnknownTopKeys(raw); err != nil {
		return nil, err
	}

	return sch, nil
}

var inlineTopKeys = map[string]bool{
	"frontmatter":      true,
	"require":          true,
	"closed":           true,
	"sections":         true,
	"cross-references": true,
	"acronyms":         true,
	"index":            true,
}

var validIndexIncludes = map[string]bool{
	IndexIncludeStepMap:      true,
	IndexIncludeCrossRefs:    true,
	IndexIncludeWordCounts:   true,
	IndexIncludeHeadingsFlat: true,
}

func rejectUnknownTopKeys(raw map[string]any) error {
	for k := range raw {
		if !inlineTopKeys[k] {
			return fmt.Errorf("unknown schema key %q", k)
		}
	}
	return nil
}

func parseInlineFrontmatter(raw map[string]any, sch *Schema) error {
	v, ok := raw["frontmatter"]
	if !ok {
		return nil
	}
	m, ok := v.(map[string]any)
	if !ok {
		return fmt.Errorf("schema.frontmatter must be a mapping, got %T", v)
	}
	sch.Frontmatter = make(map[string]string, len(m))
	for k, vv := range m {
		expr, err := frontmatterExpr(vv)
		if err != nil {
			return fmt.Errorf("schema.frontmatter.%s: %w", k, err)
		}
		sch.Frontmatter[k] = expr
	}
	return nil
}

// frontmatterExpr coerces a YAML-decoded value into a CUE expression
// string. Strings pass through (the canonical form). Numbers, bools,
// nulls become their JSON encoding. Maps and lists are JSON-encoded
// so the value carries its structure verbatim into CUE.
func frontmatterExpr(v any) (string, error) {
	switch x := v.(type) {
	case string:
		expr := strings.TrimSpace(x)
		if expr == "" {
			return "", fmt.Errorf("expression must be non-empty")
		}
		return expr, nil
	case bool, int, int64, float64, nil:
		b, err := json.Marshal(x)
		if err != nil {
			return "", err
		}
		return string(b), nil
	case []any, map[string]any:
		b, err := json.Marshal(x)
		if err != nil {
			return "", err
		}
		return string(b), nil
	default:
		return "", fmt.Errorf("unsupported value type %T", v)
	}
}

func parseInlineRequire(raw map[string]any, sch *Schema) error {
	v, ok := raw["require"]
	if !ok {
		return nil
	}
	m, ok := v.(map[string]any)
	if !ok {
		return fmt.Errorf("schema.require must be a mapping, got %T", v)
	}
	for k, vv := range m {
		switch k {
		case "filename":
			s, ok := vv.(string)
			if !ok {
				return fmt.Errorf(
					"schema.require.filename must be a string, got %T", vv)
			}
			sch.Require.Filename = s
		default:
			return fmt.Errorf("unknown schema.require key %q", k)
		}
	}
	return nil
}

func parseInlineRootClosed(raw map[string]any, sch *Schema) error {
	v, ok := raw["closed"]
	if !ok {
		return nil
	}
	b, ok := v.(bool)
	if !ok {
		return fmt.Errorf("schema.closed must be a boolean, got %T", v)
	}
	sch.Closed = b
	return nil
}

func parseInlineSections(raw map[string]any, sch *Schema) error {
	v, ok := raw["sections"]
	if !ok {
		return nil
	}
	list, ok := v.([]any)
	if !ok {
		return fmt.Errorf("schema.sections must be a list, got %T", v)
	}
	scopes, err := parseInlineScopeList(list, "schema.sections")
	if err != nil {
		return err
	}
	sch.Sections = scopes
	return nil
}

func parseInlineScopeList(list []any, path string) ([]Scope, error) {
	scopes := make([]Scope, 0, len(list))
	for i, entry := range list {
		sc, err := parseInlineScopeEntry(entry, fmt.Sprintf("%s[%d]", path, i))
		if err != nil {
			return nil, err
		}
		if sc.Preamble && i != 0 {
			return nil, fmt.Errorf(
				"%s[%d]: `heading: null` (preamble) must be the first "+
					"entry in a section list — the preamble's range "+
					"ends at the first heading", path, i)
		}
		scopes = append(scopes, sc)
	}
	return scopes, nil
}

func parseInlineScopeEntry(entry any, path string) (Scope, error) {
	m, ok := entry.(map[string]any)
	if !ok {
		return Scope{}, fmt.Errorf(
			"%s: scope must be a mapping, got %T", path, entry)
	}
	if k, found := firstRepeatingPatternKey(m); found {
		return Scope{}, fmt.Errorf(
			"%s: scope sets %q, but the repeating-pattern keys "+
				"(`repeats`, `sequential`, `min`, `max`) are parsed "+
				"into the Scope struct yet not enforced by any "+
				"validator today; remove them until a future plan "+
				"lifts the rejection so the schema does not appear "+
				"to constrain counts it ignores",
			path, k)
	}
	if _, hasHeading := m["heading"]; !hasHeading {
		return Scope{}, fmt.Errorf(
			"%s: scope must set a `heading:` key — use a string "+
				"(literal heading text), `null` (preamble), or "+
				"`{unlisted: true}` (slot)", path)
	}
	sc := Scope{Required: true}
	if err := applyScopeFields(m, &sc, path); err != nil {
		return Scope{}, err
	}
	if err := validateScopeShape(sc, m, path); err != nil {
		return Scope{}, err
	}
	// Required defaults to true for literal scopes (matches the
	// file-based parser). Preamble and slot scopes have no heading
	// to require — both parsers should produce Required=false for
	// them. Slots reject `required:` outright; preambles accept
	// it. Apply the default deterministically here, after fields
	// have settled, so map-iteration order in applyScopeFields
	// cannot leave Required at the parseInlineScopeEntry default
	// for non-literal scopes.
	if sc.Wildcard {
		sc.Required = false
	} else if sc.Preamble {
		if _, explicit := m["required"]; !explicit {
			sc.Required = false
		}
	}
	return sc, nil
}

// validateScopeShape rejects scope combinations that don't make
// semantic sense. It looks at the parsed Scope (for heading kind
// and field values) and at the raw map (so a forbidden key is
// caught by its presence, not its post-parsed value).
func validateScopeShape(sc Scope, m map[string]any, path string) error {
	if !sc.Wildcard && !sc.Preamble && strings.TrimSpace(sc.Heading) == "" {
		return fmt.Errorf(
			"%s: literal scope must set a non-empty heading", path)
	}
	if strings.TrimSpace(sc.Heading) == SectionWildcard {
		return fmt.Errorf(
			"%s: `heading: \"%s\"` is not a valid heading text — "+
				"use `heading: {unlisted: true}` to declare a slot",
			path, SectionWildcard)
	}
	for _, a := range sc.Aliases {
		if strings.TrimSpace(a) == SectionWildcard {
			return fmt.Errorf(
				"%s: alias %q is not a valid alias text — "+
					"declare a separate `heading: {unlisted: true}` "+
					"entry if you need a slot at that position",
				path, SectionWildcard)
		}
	}
	if sc.Wildcard {
		return rejectKeys(m, path, "slot (`heading: {unlisted: true}`)",
			"required", "aliases", "closed", "sections", "rules")
	}
	if sc.Preamble {
		return rejectKeys(m, path, "preamble (`heading: null`)",
			"aliases", "sections")
	}
	return nil
}

// rejectKeys errors if any forbidden key is present in m. The
// shape label and key list go into the error so the user sees
// which field is incompatible and why. Forbidden keys are checked
// by presence (zero-value or false still rejects), matching the
// repeating-pattern rejection's contract.
func rejectKeys(m map[string]any, path, shape string, keys ...string) error {
	for _, k := range keys {
		if _, ok := m[k]; ok {
			return fmt.Errorf(
				"%s: `%s:` is not allowed on a %s scope — "+
					"the validator would ignore it, so the "+
					"parser surfaces it as a config error; "+
					"remove the key",
				path, k, shape)
		}
	}
	return nil
}

// firstRepeatingPatternKey returns the first repeating-pattern key
// present in m (in a stable order), so the parse rejection fires
// based on key PRESENCE rather than the post-parsed value. Schemas
// that explicitly write `repeats: false` or `min: 0` are still
// rejected — the key being there at all signals an intent the
// validator does not yet honour.
func firstRepeatingPatternKey(m map[string]any) (string, bool) {
	for _, k := range []string{"repeats", "sequential", "min", "max"} {
		if _, ok := m[k]; ok {
			return k, true
		}
	}
	return "", false
}

// applyScopeFields walks the scope mapping and populates sc. The
// repeating-pattern keys (repeats, sequential, min, max) are
// intentionally absent here: parseInlineScopeEntry rejects them
// upfront via firstRepeatingPatternKey, so this loop never sees
// them. A future plan that ships repeating-pattern enforcement
// will lift the rejection and restore the cases.
func applyScopeFields(m map[string]any, sc *Scope, path string) error {
	for k, vv := range m {
		var err error
		switch k {
		case "heading":
			err = setScopeHeading(sc, vv, path)
		case "required":
			err = setScopeBool(&sc.Required, vv, path, k)
		case "aliases":
			err = setScopeAliases(sc, vv, path)
		case "closed":
			err = setScopeBool(&sc.Closed, vv, path, k)
		case "sections":
			err = setScopeSections(sc, vv, path)
		case "rules":
			err = setScopeRules(sc, vv, path)
		default:
			return fmt.Errorf("%s: unknown scope key %q", path, k)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// setScopeHeading reads the unified `heading:` field. The value is
// a string (literal text — the common case), `null` (preamble:
// content before the first heading), or a mapping that types a
// non-literal match. Only `{unlisted: true}` is accepted in the
// mapping form today; future work can extend it with shapes such
// as `{any: true}` and `{pattern: "..."}`.
func setScopeHeading(sc *Scope, v any, path string) error {
	switch x := v.(type) {
	case nil:
		sc.Preamble = true
		return nil
	case string:
		sc.Heading = x
		return nil
	case map[string]any:
		return applyHeadingMapping(sc, x, path)
	default:
		return fmt.Errorf(
			"%s.heading must be a string, null, or mapping, got %T",
			path, v)
	}
}

func applyHeadingMapping(sc *Scope, m map[string]any, path string) error {
	if len(m) == 0 {
		return fmt.Errorf(
			"%s.heading: empty mapping — use `{unlisted: true}` for a slot",
			path)
	}
	for k, v := range m {
		switch k {
		case "unlisted":
			b, ok := v.(bool)
			if !ok {
				return fmt.Errorf(
					"%s.heading.unlisted must be a boolean, got %T", path, v)
			}
			if !b {
				return fmt.Errorf(
					"%s.heading.unlisted must be `true` "+
						"(set the value or omit the entry)", path)
			}
			sc.Wildcard = true
		default:
			return fmt.Errorf(
				"%s.heading.%s: unknown heading-kind key (today only "+
					"`unlisted: true` is accepted)", path, k)
		}
	}
	return nil
}

func setScopeBool(dst *bool, v any, path, key string) error {
	b, ok := v.(bool)
	if !ok {
		return fmt.Errorf("%s.%s must be a boolean, got %T", path, key, v)
	}
	*dst = b
	return nil
}

func setScopeAliases(sc *Scope, v any, path string) error {
	list, ok := v.([]any)
	if !ok {
		return fmt.Errorf("%s.aliases must be a list, got %T", path, v)
	}
	sc.Aliases = make([]string, 0, len(list))
	for j, a := range list {
		as, ok := a.(string)
		if !ok {
			return fmt.Errorf(
				"%s.aliases[%d] must be a string, got %T", path, j, a)
		}
		sc.Aliases = append(sc.Aliases, as)
	}
	return nil
}

func setScopeSections(sc *Scope, v any, path string) error {
	sublist, ok := v.([]any)
	if !ok {
		return fmt.Errorf("%s.sections must be a list, got %T", path, v)
	}
	scopes, err := parseInlineScopeList(sublist, path+".sections")
	if err != nil {
		return err
	}
	sc.Sections = scopes
	return nil
}

func setScopeRules(sc *Scope, v any, path string) error {
	rm, ok := v.(map[string]any)
	if !ok {
		return fmt.Errorf("%s.rules must be a mapping, got %T", path, v)
	}
	sc.Rules = make(map[string]map[string]any, len(rm))
	for rk, rv := range rm {
		rs, ok := rv.(map[string]any)
		if !ok {
			return fmt.Errorf(
				"%s.rules.%s must be a mapping, got %T", path, rk, rv)
		}
		sc.Rules[rk] = rs
	}
	return nil
}

// FrontmatterCUE returns a CUE struct literal that constrains the
// document front matter to the schema. The result is suitable for
// compiling with cuelang and unifying against a JSON-encoded document
// front matter. Keys with a trailing "?" are emitted as optional CUE
// fields with the marker stripped from the label.
func (s *Schema) FrontmatterCUE() string {
	if len(s.Frontmatter) == 0 {
		return ""
	}
	keys := make([]string, 0, len(s.Frontmatter))
	for k := range s.Frontmatter {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	b.WriteString("close({\n")
	for _, k := range keys {
		label, optional := strings.CutSuffix(k, "?")
		b.WriteString("  ")
		b.WriteString(cueFieldLabel(label))
		if optional {
			b.WriteString("?")
		}
		b.WriteString(": ")
		b.WriteString(s.Frontmatter[k])
		b.WriteString("\n")
	}
	b.WriteString("})")
	return b.String()
}

// cueFieldLabel quotes a label that is not a bare CUE identifier so
// the resulting struct literal still parses.
func cueFieldLabel(key string) string {
	if isCUEIdent(key) {
		return key
	}
	return strconv.Quote(key)
}

func parseInlineCrossReferences(raw map[string]any, sch *Schema) error {
	v, ok := raw["cross-references"]
	if !ok {
		return nil
	}
	list, ok := v.([]any)
	if !ok {
		return fmt.Errorf("schema.cross-references must be a list, got %T", v)
	}
	sch.CrossReferences = make([]CrossRef, 0, len(list))
	for i, entry := range list {
		m, ok := entry.(map[string]any)
		if !ok {
			return fmt.Errorf(
				"schema.cross-references[%d] must be a mapping, got %T", i, entry)
		}
		cr, err := parseCrossRefEntry(m, i)
		if err != nil {
			return err
		}
		sch.CrossReferences = append(sch.CrossReferences, cr)
	}
	return nil
}

func parseCrossRefEntry(m map[string]any, i int) (CrossRef, error) {
	cr := CrossRef{}
	for k, vv := range m {
		s, ok := vv.(string)
		if !ok {
			return CrossRef{}, fmt.Errorf(
				"schema.cross-references[%d].%s must be a string, got %T",
				i, k, vv)
		}
		switch k {
		case "pattern":
			cr.Pattern = s
		case "must-match":
			cr.MustMatch = s
		case "skip-lines-matching":
			cr.SkipLinesMatching = s
		default:
			return CrossRef{}, fmt.Errorf(
				"schema.cross-references[%d]: unknown key %q", i, k)
		}
	}
	if strings.TrimSpace(cr.Pattern) == "" {
		return CrossRef{}, fmt.Errorf(
			"schema.cross-references[%d]: `pattern:` is required", i)
	}
	if strings.TrimSpace(cr.MustMatch) == "" {
		return CrossRef{}, fmt.Errorf(
			"schema.cross-references[%d]: `must-match:` is required", i)
	}
	return cr, nil
}

func parseInlineAcronyms(raw map[string]any, sch *Schema) error {
	v, ok := raw["acronyms"]
	if !ok {
		return nil
	}
	m, ok := v.(map[string]any)
	if !ok {
		return fmt.Errorf("schema.acronyms must be a mapping, got %T", v)
	}
	a := &AcronymRule{}
	for k, vv := range m {
		switch k {
		case "known-safe":
			list, err := stringList(vv, "schema.acronyms.known-safe")
			if err != nil {
				return err
			}
			a.KnownSafe = list
		case "scope":
			list, err := stringList(vv, "schema.acronyms.scope")
			if err != nil {
				return err
			}
			a.Scope = list
		default:
			return fmt.Errorf("schema.acronyms: unknown key %q", k)
		}
	}
	sch.Acronyms = a
	return nil
}

func parseInlineIndex(raw map[string]any, sch *Schema) error {
	v, ok := raw["index"]
	if !ok {
		return nil
	}
	m, ok := v.(map[string]any)
	if !ok {
		return fmt.Errorf("schema.index must be a mapping, got %T", v)
	}
	idx := &IndexSpec{}
	for k, vv := range m {
		switch k {
		case "output":
			s, ok := vv.(string)
			if !ok {
				return fmt.Errorf(
					"schema.index.output must be a string, got %T", vv)
			}
			idx.Output = s
		case "include":
			list, err := stringList(vv, "schema.index.include")
			if err != nil {
				return err
			}
			for _, item := range list {
				if !validIndexIncludes[item] {
					return fmt.Errorf(
						"schema.index.include: unknown entry %q "+
							"(valid: step-map, cross-ref-graph, "+
							"word-counts, headings)", item)
				}
			}
			idx.Include = list
		default:
			return fmt.Errorf("schema.index: unknown key %q", k)
		}
	}
	if strings.TrimSpace(idx.Output) == "" {
		return fmt.Errorf("schema.index: `output:` is required")
	}
	if len(idx.Include) == 0 {
		return fmt.Errorf(
			"schema.index: `include:` must list at least one entry")
	}
	sch.Index = idx
	return nil
}

func stringList(v any, path string) ([]string, error) {
	list, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("%s must be a list, got %T", path, v)
	}
	out := make([]string, 0, len(list))
	for i, item := range list {
		s, ok := item.(string)
		if !ok {
			return nil, fmt.Errorf(
				"%s[%d] must be a string, got %T", path, i, item)
		}
		out = append(out, s)
	}
	return out, nil
}

func isCUEIdent(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		if r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			continue
		}
		if i > 0 && r >= '0' && r <= '9' {
			continue
		}
		return false
	}
	return true
}
