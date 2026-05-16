package schema

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
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
//
// Plan 156 collapses the section-entry vocabulary: each entry sets
// exactly one `heading:` key (null, string, or mapping) and the
// mapping carries `regex:`, `repeat:`, and `sequential:`. The
// `aliases:`, `required:`, scope-level `repeats:`/`sequential:`/
// `min:`/`max:`, `{unlisted: true}` mapping, and schema-level
// `require:` shapes are gone; the parser rejects them with a
// "removed; see plan 156" diagnostic naming the replacement.
func ParseInline(raw map[string]any, source string) (*Schema, error) {
	if raw == nil {
		return &Schema{Source: source, RootLevel: 2}, nil
	}

	sch := &Schema{Source: source, RootLevel: 2}

	if err := parseInlineFrontmatter(raw, sch); err != nil {
		return nil, err
	}
	if err := rejectRemovedTopKey(raw, "require",
		"`require:` removed; see plan 156 (use top-level `filename:`)"); err != nil {
		return nil, err
	}
	if err := parseInlineFilename(raw, sch); err != nil {
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

	// schema-level `closed:` only makes sense when the schema also
	// declares a non-empty `sections:` list — strictness has no
	// scope to apply to when the kind only constrains front matter
	// / filename, and `Schema.IsEmpty` ignores `Closed`, so an
	// empty-sections closed schema would be skipped by Validate
	// entirely. Plan 156 surfaces the mismatch at parse time.
	if _, hasClosed := raw["closed"]; hasClosed && len(sch.Sections) == 0 {
		return nil, fmt.Errorf(
			"schema.closed: only valid on schemas that declare " +
				"a non-empty `sections:` list — drop the key on a " +
				"frontmatter-only kind or add at least one section")
	}

	if err := rejectUnknownTopKeys(raw); err != nil {
		return nil, err
	}

	return sch, nil
}

var inlineTopKeys = map[string]bool{
	"frontmatter":      true,
	"filename":         true,
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

func rejectRemovedTopKey(raw map[string]any, key, msg string) error {
	if _, ok := raw[key]; ok {
		return fmt.Errorf("schema.%s: %s", key, msg)
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
//
// A YAML scalar that is a single bare identifier (`date`, `bool`,
// `iso-date`) goes through the shortcut registry first: registered
// names are rewritten to their canonical CUE expression, CUE
// built-ins pass through verbatim, and an unknown bare name is
// rejected with a clear error so a typo surfaces at config-load
// time instead of as an undefined CUE reference deep in
// validation. See `internal/schema/shortcuts.go` and plan 148.
func frontmatterExpr(v any) (string, error) {
	switch x := v.(type) {
	case string:
		expr := strings.TrimSpace(x)
		if expr == "" {
			return "", fmt.Errorf("expression must be non-empty")
		}
		resolved, handled, err := resolveBareName(expr)
		if err != nil {
			return "", err
		}
		if handled {
			return resolved, nil
		}
		return expr, nil
	case bool, int, int64, float64, nil:
		// json.Marshal cannot fail on these primitive types, so
		// the error from the marshal call is intentionally
		// discarded — keeping the err check would land on a
		// permanently unreachable branch.
		b, _ := json.Marshal(x)
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

func parseInlineFilename(raw map[string]any, sch *Schema) error {
	v, ok := raw["filename"]
	if !ok {
		return nil
	}
	s, ok := v.(string)
	if !ok {
		return fmt.Errorf("schema.filename must be a string, got %T", v)
	}
	sch.Filename = s
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

// removedScopeKeys lists the per-entry keys plan 156 removed. The
// parser rejects each one by presence with a "removed; see plan 156"
// diagnostic naming the replacement so authors migrating from the
// old shape see the fix inline rather than the validator silently
// dropping the constraint.
// removedScopeKeyOrder pins the iteration order of
// removedScopeKeys. The map alone would be enough to surface a
// diagnostic, but Go randomises map iteration on every range —
// so a legacy entry that still carries two removed keys would
// report a different one on each run. The order chosen here
// matches the plan-156 changelog so users read the entries in
// the same sequence the migration table presents them.
var removedScopeKeyOrder = []string{
	"required", "aliases", "repeats", "sequential", "min", "max",
}

var removedScopeKeys = map[string]string{
	"required": "`required:` removed; see plan 156 " +
		"(use `repeat: { min: 0, max: 1 }` for optional, or omit for required)",
	"aliases": "`aliases:` removed; see plan 156 " +
		"(encode disjunction in `regex:`, e.g. `regex: 'A|B'`)",
	"repeats": "scope-level `repeats:` removed; see plan 156 " +
		"(use `heading.repeat: { min: 1 }`)",
	"sequential": "scope-level `sequential:` removed; see plan 156 " +
		"(set `sequential:` inside the `heading:` mapping)",
	"min": "scope-level `min:` removed; see plan 156 " +
		"(use `heading.repeat.min`)",
	"max": "scope-level `max:` removed; see plan 156 " +
		"(use `heading.repeat.max`)",
}

func parseInlineScopeEntry(entry any, path string) (Scope, error) {
	m, ok := entry.(map[string]any)
	if !ok {
		return Scope{}, fmt.Errorf(
			"%s: scope must be a mapping, got %T", path, entry)
	}
	// Walk a fixed key order so the first surfaced migration
	// diagnostic stays deterministic when an entry carries more
	// than one removed key — Go's map iteration is randomised
	// per range, which would otherwise swap which key gets
	// reported run-to-run.
	for _, k := range removedScopeKeyOrder {
		if _, present := m[k]; present {
			return Scope{}, fmt.Errorf("%s: %s", path, removedScopeKeys[k])
		}
	}
	if _, hasHeading := m["heading"]; !hasHeading {
		return Scope{}, fmt.Errorf(
			"%s: scope must set a `heading:` key — use a string "+
				"(literal heading text), `null` (preamble), or "+
				"a mapping `{ regex, repeat?, sequential? }`", path)
	}
	sc := Scope{}
	if err := applyScopeFields(m, &sc, path); err != nil {
		return Scope{}, err
	}
	if err := validateScopeShape(sc, m, path); err != nil {
		return Scope{}, err
	}
	return sc, nil
}

// validateScopeShape rejects scope combinations that don't make
// semantic sense. It looks at the parsed Scope (for heading kind
// and field values) and at the raw map (so a forbidden key is
// caught by its presence, not its post-parsed value).
func validateScopeShape(sc Scope, m map[string]any, path string) error {
	if sc.Preamble {
		return rejectKeys(m, path, "preamble (`heading: null`)",
			"sections")
	}
	// After applyScopeFields succeeds, either Preamble is true
	// (handled above) or Matcher is set by setScopeHeading; a
	// missing `heading:` key is rejected upstream by
	// parseInlineScopeEntry. There is no reachable path where
	// sc.Matcher is nil here, so no defensive nil-check fires.
	// A regex of '.+' is the wildcard-slot shape; rule overrides,
	// nested sections, and per-section content do not make sense
	// there because the slot has no fixed identity. Plan 156
	// makes that explicit.
	if isSlotMatcher(sc.Matcher) {
		return rejectKeys(m, path, "slot (`regex: '.+', repeat: { min: 0 }`)",
			"sections", "rules", "content", "closed")
	}
	return nil
}

// isSlotMatcher reports whether m is the canonical wildcard-slot
// shape: a `.+` regex with `repeat: { min: 0 }` (unbounded max).
// The shape collapses what the old grammar spelled
// `heading: { unlisted: true }`.
func isSlotMatcher(m *Matcher) bool {
	if m == nil {
		return false
	}
	if m.Regex != ".+" {
		return false
	}
	if !m.Repeat.Set || m.Repeat.Min != 0 || m.Repeat.Max != 0 {
		return false
	}
	return true
}

// isBroadMatcher reports whether m's regex matches "anything" —
// the `.+` body, regardless of repeat bounds. Used by the
// yield-to-later helpers so a broad scope never claims a heading
// that a more-specific later scope would have matched. Slot
// matchers are a subset of broad matchers.
func isBroadMatcher(m *Matcher) bool {
	return m != nil && m.Regex == ".+"
}

// rejectKeys errors if any forbidden key is present in m. The
// shape label and key list go into the error so the user sees
// which field is incompatible and why. Forbidden keys are checked
// by presence (zero-value or false still rejects).
func rejectKeys(m map[string]any, path, shape string, keys ...string) error {
	for _, k := range keys {
		if _, ok := m[k]; ok {
			return fmt.Errorf(
				"%s: `%s:` is not allowed on a %s scope — "+
					"remove the key",
				path, k, shape)
		}
	}
	return nil
}

// applyScopeFields walks the scope mapping and populates sc. Each
// supported per-entry key has its own setter; unknown keys
// parse-error.
func applyScopeFields(m map[string]any, sc *Scope, path string) error {
	for k, vv := range m {
		var err error
		switch k {
		case "heading":
			err = setScopeHeading(sc, vv, path)
		case "closed":
			err = setScopeBool(&sc.Closed, vv, path, k)
		case "sections":
			err = setScopeSections(sc, vv, path)
		case "rules":
			err = setScopeRules(sc, vv, path)
		case "content":
			err = setScopeContent(sc, vv, path)
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
// content before the first heading), or a mapping that types the
// match (`{ regex, repeat?, sequential? }`).
func setScopeHeading(sc *Scope, v any, path string) error {
	switch x := v.(type) {
	case nil:
		sc.Preamble = true
		return nil
	case string:
		return setBareStringHeading(sc, x, path)
	case map[string]any:
		return applyHeadingMapping(sc, x, path)
	default:
		return fmt.Errorf(
			"%s.heading must be a string, null, or mapping, got %T",
			path, v)
	}
}

func setBareStringHeading(sc *Scope, s, path string) error {
	if strings.TrimSpace(s) == "" {
		return fmt.Errorf(
			"%s.heading: empty string — use `null` for a preamble", path)
	}
	if strings.TrimSpace(s) == SectionWildcard {
		return fmt.Errorf(
			"%s.heading: %q is not a valid heading text — "+
				"use `heading: { regex: '.+', repeat: { min: 0 } }` for a slot",
			path, SectionWildcard)
	}
	sc.Heading = s
	sc.Matcher = &Matcher{Regex: regexp.QuoteMeta(s)}
	return nil
}

// removedHeadingKeys lists the heading-mapping keys plan 156
// dropped. The parser rejects each one by presence so authors see
// the migration path inline.
var removedHeadingKeys = map[string]string{
	"unlisted": "`heading.unlisted:` removed; see plan 156 (use `regex: '.+', repeat: { min: 0 }`)",
}

func applyHeadingMapping(sc *Scope, m map[string]any, path string) error {
	if len(m) == 0 {
		return fmt.Errorf(
			"%s.heading: empty mapping — use `{ regex: '.+', repeat: { min: 0 } }` for a slot",
			path)
	}
	for k, msg := range removedHeadingKeys {
		if _, present := m[k]; present {
			return fmt.Errorf("%s.heading: %s", path, msg)
		}
	}
	matcher := &Matcher{}
	for k, v := range m {
		var err error
		switch k {
		case "regex":
			err = setMatcherRegex(matcher, v, path)
		case "repeat":
			err = setMatcherRepeat(matcher, v, path)
		case "sequential":
			err = setScopeBool(&matcher.Sequential, v, path+".heading", k)
		default:
			return fmt.Errorf(
				"%s.heading.%s: unknown heading-mapping key", path, k)
		}
		if err != nil {
			return err
		}
	}
	if strings.TrimSpace(matcher.Regex) == "" {
		return fmt.Errorf(
			"%s.heading: `regex:` is required when `heading:` is a mapping",
			path)
	}
	if n := countDigitsHelpers(matcher.Regex); n > 1 {
		return fmt.Errorf(
			"%s.heading.regex: `\\#(digits)` may appear at most once "+
				"(the matcher reads a single named `n` capture)",
			path)
	}
	// The `n` named-capture group is reserved by the `digits` helper.
	// Parse the regex source to find actual named captures rather
	// than substring-matching `(?P<n>` — the literal text can also
	// appear inside a character class or as `\(\?P<n>` escape
	// sequence that does not introduce a real capture group.
	if hasNamedCapture(matcher.Regex, "n") {
		return fmt.Errorf(
			"%s.heading.regex: the `n` named capture is reserved by "+
				"`\\#(digits)`; rename the user capture or remove the "+
				"`(?P<n>...)` group",
			path)
	}
	if matcher.Sequential && !patternHasDigits(matcher.Regex) {
		return fmt.Errorf(
			"%s.heading.sequential: requires a `\\#(digits)` capture in `regex:`",
			path)
	}
	sc.Heading = matcher.Regex
	sc.Matcher = matcher
	return nil
}

func setMatcherRegex(m *Matcher, v any, path string) error {
	s, ok := v.(string)
	if !ok {
		return fmt.Errorf("%s.heading.regex must be a string, got %T", path, v)
	}
	if strings.TrimSpace(s) == "" {
		return fmt.Errorf("%s.heading.regex: empty pattern", path)
	}
	// Compile the resolved pattern (with helpers substituted by a
	// dummy frontmatter) to surface invalid RE2 syntax at parse
	// time. The validator re-resolves the pattern per-document so
	// `fmvar(name)` picks up the real value. Unsupported helpers
	// and unterminated `\#(` references fail here instead of
	// degrading into a missing-section diagnostic at runtime.
	probe, err := resolvePatternForCheck(s)
	if err != nil {
		return fmt.Errorf("%s.heading.regex: %v", path, err)
	}
	if _, err := regexp.Compile("^(?:" + probe + ")$"); err != nil {
		return fmt.Errorf("%s.heading.regex: %v", path, err)
	}
	m.Regex = s
	return nil
}

func setMatcherRepeat(m *Matcher, v any, path string) error {
	rm, ok := v.(map[string]any)
	if !ok {
		return fmt.Errorf("%s.heading.repeat must be a mapping, got %T", path, v)
	}
	if len(rm) == 0 {
		return fmt.Errorf(
			"%s.heading.repeat: empty mapping — set `min:`, `max:`, or both",
			path)
	}
	r := Repeat{Set: true}
	var minSet, maxSet bool
	for k, vv := range rm {
		switch k {
		case "min":
			minSet = true
			if err := readIntBound(&r.Min, vv, path+".heading.repeat", k); err != nil {
				return err
			}
		case "max":
			maxSet = true
			if err := readIntBound(&r.Max, vv, path+".heading.repeat", k); err != nil {
				return err
			}
		default:
			return fmt.Errorf(
				"%s.heading.repeat.%s: unknown key (valid: min, max)", path, k)
		}
	}
	if maxSet && r.Max == 0 {
		return fmt.Errorf(
			"%s.heading.repeat.max: must be greater than 0 (0 is unbounded; "+
				"omit `max:` instead)", path)
	}
	if minSet && maxSet && r.Min > r.Max {
		return fmt.Errorf(
			"%s.heading.repeat: min=%d is greater than max=%d",
			path, r.Min, r.Max)
	}
	m.Repeat = r
	return nil
}

// readIntBound parses a non-negative integer from v. YAML decoders
// can deliver ints as int, int64, or float64; the helper handles
// all three with the same overflow / non-integer guards used for
// content list bounds.
func readIntBound(dst *int, v any, path, key string) error {
	switch x := v.(type) {
	case int:
		if x < 0 {
			return fmt.Errorf("%s.%s must be non-negative, got %d", path, key, x)
		}
		*dst = x
	case int64:
		if x < 0 {
			return fmt.Errorf("%s.%s must be non-negative, got %d", path, key, x)
		}
		*dst = int(x)
	case float64:
		if math.IsNaN(x) || math.IsInf(x, 0) {
			return fmt.Errorf(
				"%s.%s must be a finite integer, got %v", path, key, x)
		}
		if x < 0 {
			return fmt.Errorf(
				"%s.%s must be non-negative, got %v", path, key, x)
		}
		if math.Trunc(x) != x {
			return fmt.Errorf(
				"%s.%s must be a non-negative integer, got %v", path, key, x)
		}
		if x > float64(math.MaxInt) {
			return fmt.Errorf(
				"%s.%s value %v exceeds int range on this platform",
				path, key, x)
		}
		*dst = int(x)
	default:
		return fmt.Errorf("%s.%s must be an integer, got %T", path, key, v)
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

// setScopeContent reads a `content:` list from a scope mapping into
// sc.Content. Each entry must be a mapping; `kind:` is required.
// Kind-specific fields (lang, columns, ordered, min-items, max-items)
// are accepted only on the kind they apply to. Unknown kinds and
// unknown keys are rejected so authoring mistakes surface as parser
// errors rather than silent no-ops at validation time.
func setScopeContent(sc *Scope, v any, path string) error {
	list, ok := v.([]any)
	if !ok {
		return fmt.Errorf("%s.content must be a list, got %T", path, v)
	}
	entries := make([]ContentEntry, 0, len(list))
	for i, item := range list {
		entry, err := parseContentEntry(item, fmt.Sprintf("%s.content[%d]", path, i))
		if err != nil {
			return err
		}
		entries = append(entries, entry)
	}
	sc.Content = entries
	return nil
}

// parseContentEntry decodes one content-list entry. The `kind:` key
// drives validation; unknown kinds are rejected here so the validator
// can dispatch by string equality without re-checking shape.
func parseContentEntry(entry any, path string) (ContentEntry, error) {
	m, ok := entry.(map[string]any)
	if !ok {
		return ContentEntry{}, fmt.Errorf(
			"%s: content entry must be a mapping, got %T", path, entry)
	}
	kindV, ok := m["kind"]
	if !ok {
		return ContentEntry{}, fmt.Errorf(
			"%s: content entry must set a `kind:` key (one of: "+
				"code-block, table, list, paragraph, unlisted)", path)
	}
	kind, ok := kindV.(string)
	if !ok {
		return ContentEntry{}, fmt.Errorf(
			"%s.kind must be a string, got %T", path, kindV)
	}
	if !validContentKind(kind) {
		return ContentEntry{}, fmt.Errorf(
			"%s.kind: unknown content kind %q (valid: "+
				"code-block, table, list, paragraph, unlisted)", path, kind)
	}
	ce := ContentEntry{Kind: kind, Required: true}
	if kind == ContentKindUnlisted {
		ce.Required = false
	}
	if err := applyContentFields(m, &ce, path); err != nil {
		return ContentEntry{}, err
	}
	if kind == ContentKindUnlisted {
		if _, hasReq := m["required"]; hasReq {
			return ContentEntry{}, fmt.Errorf(
				"%s: `required:` is not allowed on a `kind: unlisted` "+
					"content entry — slots are positional and never required",
				path)
		}
	}
	// A list entry that sets both min-items and max-items must
	// declare a satisfiable range. Catching this at parse time
	// converts a guaranteed-fail runtime diagnostic into a clear
	// schema-config error naming the contradictory bounds.
	if ce.MinItems > 0 && ce.MaxItems > 0 && ce.MinItems > ce.MaxItems {
		return ContentEntry{}, fmt.Errorf(
			"%s: min-items=%d is greater than max-items=%d — "+
				"no list could ever satisfy this entry",
			path, ce.MinItems, ce.MaxItems)
	}
	return ce, nil
}

func validContentKind(k string) bool {
	switch k {
	case ContentKindCodeBlock, ContentKindTable,
		ContentKindList, ContentKindParagraph, ContentKindUnlisted:
		return true
	}
	return false
}

// applyContentFields walks a content-entry mapping and applies every
// non-`kind:` key. Keys that don't belong to the entry's kind raise an
// error so a typo (or a mis-targeted constraint) surfaces at parse
// time rather than as a silently-ignored field.
func applyContentFields(m map[string]any, ce *ContentEntry, path string) error {
	for k, vv := range m {
		if k == "kind" {
			continue
		}
		if err := applyContentField(k, vv, ce, path); err != nil {
			return err
		}
	}
	return nil
}

func applyContentField(k string, vv any, ce *ContentEntry, path string) error {
	switch k {
	case "required":
		return setScopeBool(&ce.Required, vv, path, k)
	case "lang":
		return setContentLang(ce, vv, path)
	case "columns":
		return setContentColumns(ce, vv, path)
	case "ordered":
		return setContentOrdered(ce, vv, path)
	case "min-items":
		return setContentItemBound(&ce.MinItems, vv, path, k, ce.Kind)
	case "max-items":
		return setContentItemBound(&ce.MaxItems, vv, path, k, ce.Kind)
	default:
		return fmt.Errorf("%s: unknown content key %q", path, k)
	}
}

func setContentLang(ce *ContentEntry, v any, path string) error {
	if ce.Kind != ContentKindCodeBlock {
		return fmt.Errorf(
			"%s.lang: only valid on `kind: code-block`", path)
	}
	s, ok := v.(string)
	if !ok {
		return fmt.Errorf("%s.lang must be a string, got %T", path, v)
	}
	ce.Lang = s
	return nil
}

func setContentColumns(ce *ContentEntry, v any, path string) error {
	if ce.Kind != ContentKindTable {
		return fmt.Errorf(
			"%s.columns: only valid on `kind: table`", path)
	}
	list, err := stringList(v, path+".columns")
	if err != nil {
		return err
	}
	ce.Columns = list
	return nil
}

func setContentOrdered(ce *ContentEntry, v any, path string) error {
	if ce.Kind != ContentKindList {
		return fmt.Errorf(
			"%s.ordered: only valid on `kind: list`", path)
	}
	b, ok := v.(bool)
	if !ok {
		return fmt.Errorf("%s.ordered must be a boolean, got %T", path, v)
	}
	ce.Ordered = b
	ce.OrderedSet = true
	return nil
}

func setContentItemBound(dst *int, v any, path, key, kind string) error {
	if kind != ContentKindList {
		return fmt.Errorf(
			"%s.%s: only valid on `kind: list`", path, key)
	}
	return readIntBound(dst, v, path, key)
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
