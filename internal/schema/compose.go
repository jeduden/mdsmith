package schema

import (
	"fmt"
	"strings"
)

// Compose merges multiple schemas into one. The composed schema's
// frontmatter constraints are the union of each input's keys; for keys
// declared in more than one input, the CUE expressions are conjoined
// with `&` so a value must satisfy every constraint. Sections are
// merged so that scopes sharing the same heading label combine their
// child sections recursively; remaining scopes append in input order.
// The stricter `closed:` wins (any input that sets Closed=true makes
// the composed scope closed) and the cardinality intersects (the
// composed run length must satisfy every input; disjoint ranges have
// an empty intersection and return an error). Filename uses the first
// non-empty value; conflicting patterns cause an error so the caller
// surfaces a clear diagnostic rather than silently ignoring one
// constraint. Acronyms, CrossReferences, and Index slots
// are joined from inputs that declare them; conflicts on Index.Output
// return an error.
//
// Compose returns nil when given no inputs. With a single non-nil
// input it returns that input unchanged.
func Compose(schemas ...*Schema) (*Schema, error) {
	nonNil := make([]*Schema, 0, len(schemas))
	for _, s := range schemas {
		if s == nil {
			continue
		}
		nonNil = append(nonNil, s)
	}
	if len(nonNil) == 0 {
		return nil, nil
	}
	if len(nonNil) == 1 {
		return nonNil[0], nil
	}

	rootLevel, err := composedRootLevel(nonNil)
	if err != nil {
		return nil, err
	}
	out := &Schema{
		Source:    composedSourceLabel(nonNil),
		RootLevel: rootLevel,
	}

	composeFrontmatter(out, nonNil)
	if err := composeFilename(out, nonNil); err != nil {
		return nil, err
	}
	composeRootClosed(out, nonNil)
	if err := composeProjection(out, nonNil); err != nil {
		return nil, err
	}
	out.Sections, err = composeSectionLists(extractRootSections(nonNil))
	if err != nil {
		return nil, err
	}
	out.CrossReferences = composeCrossRefs(nonNil)
	composeAcronyms(out, nonNil)
	if err := composeIndex(out, nonNil); err != nil {
		return nil, err
	}
	return out, nil
}

// composedRootLevel reports the heading level the composed
// section list should sit at. Every input must agree on the
// effective root level — mixing an inline schema (RootLevel=2,
// H1 owned by the title) with a file-based proto.md that wraps
// its sections in an H1 wildcard (RootLevel=1) would cause the
// validator's section walk to start at the wrong depth for one
// of the inputs. Surface the conflict as a config error so the
// caller sees a clear message instead of silently mis-validated
// headings.
func composedRootLevel(schemas []*Schema) (int, error) {
	root := schemas[0].EffectiveRootLevel()
	for _, s := range schemas[1:] {
		if s.EffectiveRootLevel() != root {
			return 0, fmt.Errorf(
				"composed schemas disagree on root heading level: "+
					"%q starts at h%d, %q starts at h%d — every source "+
					"must declare the same root level (typically the "+
					"H1 wildcard `# ?` for file-based schemas, H2 for "+
					"inline)",
				schemas[0].Source, root,
				s.Source, s.EffectiveRootLevel())
		}
	}
	return root, nil
}

func composedSourceLabel(schemas []*Schema) string {
	parts := make([]string, 0, len(schemas))
	for _, s := range schemas {
		if s.Source == "" {
			continue
		}
		parts = append(parts, s.Source)
	}
	if len(parts) == 0 {
		return "composed"
	}
	return "composed(" + strings.Join(parts, ", ") + ")"
}

func composeFrontmatter(out *Schema, schemas []*Schema) {
	for _, s := range schemas {
		for k, expr := range s.Frontmatter {
			if out.Frontmatter == nil {
				out.Frontmatter = map[string]string{}
			}
			existing, ok := out.Frontmatter[k]
			if !ok {
				out.Frontmatter[k] = expr
				continue
			}
			if existing == expr {
				continue
			}
			out.Frontmatter[k] = "(" + existing + ") & (" + expr + ")"
		}
		for k, meta := range s.FrontmatterMeta {
			if out.FrontmatterMeta == nil {
				out.FrontmatterMeta = map[string]FieldMeta{}
			}
			// Later inputs win: a kind composed on top of another
			// can re-deprecate a field with a fresher message.
			// ExtractFieldMeta only classifies a mapping as
			// metadata when `deprecated: true` is the
			// discriminator, so `{deprecated: false}` never
			// reaches FrontmatterMeta and cannot undo a parent's
			// deprecation here. Remove the field from the schema
			// entirely to complete the migration (plan 136
			// acceptance criterion 6).
			out.FrontmatterMeta[k] = meta
		}
	}
}

func composeFilename(out *Schema, schemas []*Schema) error {
	for _, s := range schemas {
		if s.Filename == "" {
			continue
		}
		if out.Filename == "" {
			out.Filename = s.Filename
			continue
		}
		if out.Filename != s.Filename {
			return fmt.Errorf(
				"conflicting filename patterns across "+
					"composed schemas: %q and %q",
				out.Filename, s.Filename)
		}
	}
	return nil
}

func composeRootClosed(out *Schema, schemas []*Schema) {
	for _, s := range schemas {
		if s.Closed {
			out.Closed = true
			return
		}
	}
}

func extractRootSections(schemas []*Schema) [][]Scope {
	out := make([][]Scope, 0, len(schemas))
	for _, s := range schemas {
		out = append(out, s.Sections)
	}
	return out
}

// composeSectionLists merges multiple parallel section lists into
// one. Scopes that share a heading label (canMergeByHeading) are
// merged recursively — their child sections compose. The
// no-identity shapes (the `## ...` wildcard slot, the bare `?`
// matcher, the preamble) append in input order so each stays
// distinct. A field-interpolated label like `{id}: {name}` is a
// stable identity and DOES merge, so two proto.md sources that
// each wrap their sections in the same H1 yield one combined H1
// scope rather than requiring two H1s in the document.
//
// Per plan 167, distinct sibling scopes that bind to the same key
// are a compose-time error — the projection would collide.
func composeSectionLists(lists [][]Scope) ([]Scope, error) {
	acc := &sectionAcc{indexByHeading: map[string]int{}}
	for _, list := range lists {
		seenPreamble := false
		for _, sc := range list {
			switch {
			case sc.Preamble && seenPreamble:
				// A list can have at most one preamble at index 0;
				// defensively skip duplicates.
				continue
			case sc.Preamble:
				seenPreamble = true
				if err := acc.addPreamble(sc); err != nil {
					return nil, err
				}
			default:
				if err := acc.addNamed(sc); err != nil {
					return nil, err
				}
			}
		}
	}
	if err := rejectComposedDuplicateBinds(acc.out); err != nil {
		return nil, err
	}
	return acc.out, nil
}

// rejectComposedDuplicateBinds errors when two distinct scopes in
// the composed section list share a non-empty bind value. Empty
// binds (the hoist signal) do not collide because they produce no
// key. This catches the case where two kinds bind sibling scopes
// to the same name; identical bound headings already merged
// upstream so duplicates here are by definition different scopes
// in conflict.
func rejectComposedDuplicateBinds(scopes []Scope) error {
	seen := make(map[string]string, len(scopes))
	for i := range scopes {
		b := scopes[i].Bind
		if b == nil || *b == "" {
			continue
		}
		if prev, ok := seen[*b]; ok {
			return fmt.Errorf(
				"composed schemas bind two sibling scopes to the same "+
					"projection key %q: %q and %q — every source must "+
					"agree on the key for each composed heading",
				*b, prev, scopes[i].Heading)
		}
		seen[*b] = scopes[i].Heading
	}
	return nil
}

// sectionAcc accumulates composed scopes while tracking where each
// mergeable heading landed, so a later input's same-heading scope
// folds into the earlier one instead of duplicating.
type sectionAcc struct {
	out            []Scope
	indexByHeading map[string]int
}

// addPreamble folds a preamble into the existing one or prepends it.
// A preamble must lead the composed list, so prepending shifts every
// recorded heading index by one to keep them valid.
func (a *sectionAcc) addPreamble(sc Scope) error {
	if existing := findPreambleIndex(a.out); existing >= 0 {
		return a.mergeAt(existing, sc)
	}
	a.out = append([]Scope{sc}, a.out...)
	for k, v := range a.indexByHeading {
		a.indexByHeading[k] = v + 1
	}
	return nil
}

// addNamed appends a no-identity scope verbatim, merges a repeated
// stable heading into its first occurrence, or records a new one.
func (a *sectionAcc) addNamed(sc Scope) error {
	if !canMergeByHeading(sc) {
		a.out = append(a.out, cloneScope(sc))
		return nil
	}
	if idx, ok := a.indexByHeading[sc.Heading]; ok {
		return a.mergeAt(idx, sc)
	}
	a.indexByHeading[sc.Heading] = len(a.out)
	a.out = append(a.out, cloneScope(sc))
	return nil
}

// mergeAt merges sc into a.out[idx] in place, propagating any
// composition error.
func (a *sectionAcc) mergeAt(idx int, sc Scope) error {
	merged, err := mergeScopes(a.out[idx], sc)
	if err != nil {
		return err
	}
	a.out[idx] = merged
	return nil
}

func findPreambleIndex(list []Scope) int {
	for i, sc := range list {
		if sc.Preamble {
			return i
		}
	}
	return -1
}

// canMergeByHeading reports whether a scope has a stable identity
// that supports merging across composed inputs. Scopes merge by
// their heading label (including field-interpolated patterns: two
// schemas that both wrap their sections in `# {id}: {name}`
// describe the same H1, so the validator must see one combined
// scope). The no-identity shapes — the `## ...` wildcard slot, the
// bare `?` any-heading matcher, and the preamble — never merge:
// each must stay distinct so it independently absorbs the
// surrounding content the author intended.
func canMergeByHeading(sc Scope) bool {
	if sc.Preamble {
		return false
	}
	h := strings.TrimSpace(sc.Heading)
	if h == "" || h == "?" || h == SectionWildcard {
		return false
	}
	return true
}

// mergeScopes combines two scopes that share a heading label. The
// result keeps the first scope's heading label; Closed takes the
// stricter value (true wins) and the Matcher's cardinality is the
// intersection of both inputs' run-length ranges (disjoint ranges
// return an error). Child sections and per-scope rule overrides
// compose by the same rules as the root; positional Content
// constraints concatenate in input order. Per plan 167, `Bind`
// values must agree across the merged scopes: nil unifies with
// anything, two equal non-nil values keep that value, and any
// genuine disagreement is a compose-time error.
func mergeScopes(a, b Scope) (Scope, error) {
	out := cloneScope(a)
	if b.Closed {
		out.Closed = true
	}
	m, err := mergeMatcher(a.Matcher, b.Matcher)
	if err != nil {
		return Scope{}, err
	}
	out.Matcher = m
	out.Sections, err = composeSectionLists([][]Scope{a.Sections, b.Sections})
	if err != nil {
		return Scope{}, err
	}
	bind, err := mergeBind(a.Bind, b.Bind, a.Heading)
	if err != nil {
		return Scope{}, err
	}
	out.Bind = bind
	// Rules: union by rule name; later input wins on key collisions.
	// The schema-rule walker is the only consumer, and its semantics
	// are already "later overrides earlier" within a single scope.
	out.Rules = mergeScopeRules(a.Rules, b.Rules)
	// cloneScope already deep-copied a.Content into out; appending
	// b's clone onto it avoids re-cloning a's entries.
	out.Content = append(out.Content, cloneContent(b.Content)...)
	// The scope-level projection family merges like bind: cloneScope
	// carried a's values, so fold b's in or error on disagreement.
	out.Projection, err = mergeProjectionField(
		a.Projection, b.Projection, "projection", a.Heading)
	if err != nil {
		return Scope{}, err
	}
	out.BlockParagraphs, err = mergeProjectionField(
		a.BlockParagraphs, b.BlockParagraphs, "block-paragraphs", a.Heading)
	if err != nil {
		return Scope{}, err
	}
	// The same co-presence guard the inline parser applies per scope:
	// merged halves must not pair `block-paragraphs` with a scope that
	// lost `projection: blocks`.
	if out.BlockParagraphs != "" && out.Projection != ProjectionBlocks {
		return Scope{}, fmt.Errorf(
			"composed schemas set `block-paragraphs:` for heading %q "+
				"without `projection: blocks` on the same scope",
			a.Heading)
	}
	return out, nil
}

// composeProjection merges the schema-level `projection:` and
// `block-paragraphs:` defaults across the composed inputs by the
// mergeBind rules applied to strings: an empty value yields to a set
// one, equal values survive, and a genuine disagreement errors so
// `projection: blocks` never silently disappears when kinds compose.
func composeProjection(out *Schema, in []*Schema) error {
	for _, s := range in {
		p, err := mergeProjectionField(out.Projection, s.Projection,
			"projection", "")
		if err != nil {
			return err
		}
		out.Projection = p
		bp, err := mergeProjectionField(out.BlockParagraphs,
			s.BlockParagraphs, "block-paragraphs", "")
		if err != nil {
			return err
		}
		out.BlockParagraphs = bp
	}
	// Mirror the parse-time co-presence guard: a merge must not
	// synthesize `block-paragraphs` without `projection: blocks` — the
	// pair the parser rejects would otherwise survive composition as a
	// silently dead setting.
	if out.BlockParagraphs != "" && out.Projection != ProjectionBlocks {
		return fmt.Errorf(
			"composed schemas set `block-paragraphs:` without a " +
				"schema-level `projection: blocks` — declare the " +
				"projection on a composed source or drop the option")
	}
	return nil
}

// mergeProjectionField merges one projection-family string across two
// sources: empty yields, equal survives, disagreement errors. heading
// is empty for the schema-level defaults and names the scope for
// scope-level merges.
func mergeProjectionField(a, b, key, heading string) (string, error) {
	if a == "" {
		return b, nil
	}
	if b == "" || a == b {
		return a, nil
	}
	where := "the schema level"
	if heading != "" {
		where = fmt.Sprintf("heading %q", heading)
	}
	return "", fmt.Errorf(
		"composed schemas declare conflicting `%s:` values at %s: "+
			"%q vs %q — every source must agree",
		key, where, a, b)
}

// mergeBind intersects two scope-level `bind:` overrides for scopes
// being merged by heading. The rules mirror filename composition:
// nil + anything keeps the non-nil side; two equal non-nil values
// keep that value; a real disagreement returns an error so the
// caller surfaces a clear diagnostic.
func mergeBind(a, b *string, heading string) (*string, error) {
	if a == nil {
		return b, nil
	}
	if b == nil {
		return a, nil
	}
	if *a == *b {
		return a, nil
	}
	return nil, fmt.Errorf(
		"composed schemas declare conflicting `bind:` overrides for "+
			"heading %q: %s vs %s — every source must agree on whether "+
			"the scope is hoisted (`bind: \"\"`) or projected under a "+
			"specific key",
		heading, bindLabel(*a), bindLabel(*b))
}

// bindLabel renders a bind value for an error message: the empty
// string is the hoist signal and prints as `hoist (\"\")`, every
// other value prints quoted as a projection key.
func bindLabel(s string) string {
	if s == "" {
		return `hoist ("")`
	}
	return fmt.Sprintf("projection key %q", s)
}

// mergeMatcher combines two matchers for scopes that share a
// heading label. In practice both matchers derive from the same
// heading text, so their Regex bodies are identical; the function
// keeps the first scope's Regex and intersects the cardinality so
// the composed run length satisfies every input. The minimum is
// the larger of the two (a section required by either input is
// required in the result) and the maximum is the smaller of the
// two, treating 0 as unbounded so an unbounded side never loosens
// a bounded one. Disjoint ranges (e.g. 1..3 and 5..10) have an
// empty intersection: no document can satisfy both, so they return
// an error, mirroring how conflicting filename patterns surface
// rather than silently honoring one input. Sequential is OR-ed.
func mergeMatcher(a, b *Matcher) (*Matcher, error) {
	if a == nil {
		return cloneMatcher(b), nil
	}
	if b == nil {
		return cloneMatcher(a), nil
	}
	out := *a
	out.Sequential = a.Sequential || b.Sequential
	aMin, aMax := a.Repeat.Bounds()
	bMin, bMax := b.Repeat.Bounds()
	min := aMin
	if bMin > min {
		min = bMin
	}
	max := intersectMax(aMax, bMax)
	if max != 0 && min > max {
		return nil, fmt.Errorf(
			"composed schemas declare disjoint cardinality for a "+
				"section: one input allows %s, another allows %s — "+
				"the intersection is empty so no document satisfies "+
				"both",
			boundsLabel(aMin, aMax), boundsLabel(bMin, bMax))
	}
	if min == 1 && max == 1 {
		out.Repeat = Repeat{}
	} else {
		out.Repeat = Repeat{Set: true, Min: min, Max: max}
	}
	return &out, nil
}

// intersectMax returns the stricter (smaller) of two run-length
// maxima, where 0 means unbounded. An unbounded side yields the
// other's bound so it never loosens a bounded constraint.
func intersectMax(a, b int) int {
	switch {
	case a == 0:
		return b
	case b == 0:
		return a
	case a < b:
		return a
	default:
		return b
	}
}

// boundsLabel renders a (min, max) cardinality pair for error
// messages; a 0 max prints as "unbounded".
func boundsLabel(min, max int) string {
	if max == 0 {
		return fmt.Sprintf("%d..unbounded", min)
	}
	return fmt.Sprintf("%d..%d", min, max)
}

func cloneMatcher(m *Matcher) *Matcher {
	if m == nil {
		return nil
	}
	c := *m
	return &c
}

func cloneContent(c []ContentEntry) []ContentEntry {
	if len(c) == 0 {
		return nil
	}
	out := make([]ContentEntry, len(c))
	for i, e := range c {
		out[i] = e
		if e.Columns != nil {
			out[i].Columns = append([]string(nil), e.Columns...)
		}
	}
	return out
}

func unionStrings(a, b []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(a)+len(b))
	for _, list := range [][]string{a, b} {
		for _, s := range list {
			if seen[s] {
				continue
			}
			seen[s] = true
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func mergeScopeRules(
	a, b map[string]map[string]any,
) map[string]map[string]any {
	if len(a) == 0 && len(b) == 0 {
		return nil
	}
	out := make(map[string]map[string]any, len(a)+len(b))
	for k, v := range a {
		out[k] = cloneSettingsMap(v)
	}
	for k, v := range b {
		out[k] = cloneSettingsMap(v)
	}
	return out
}

func cloneScope(sc Scope) Scope {
	out := sc
	out.Matcher = cloneMatcher(sc.Matcher)
	if sc.Sections != nil {
		out.Sections = make([]Scope, len(sc.Sections))
		for i, child := range sc.Sections {
			out.Sections[i] = cloneScope(child)
		}
	}
	if sc.Rules != nil {
		out.Rules = make(map[string]map[string]any, len(sc.Rules))
		for k, v := range sc.Rules {
			out.Rules[k] = cloneSettingsMap(v)
		}
	}
	out.Content = cloneContent(sc.Content)
	return out
}

func cloneSettingsMap(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func composeCrossRefs(schemas []*Schema) []CrossRef {
	total := 0
	for _, s := range schemas {
		total += len(s.CrossReferences)
	}
	out := make([]CrossRef, 0, total)
	for _, s := range schemas {
		out = append(out, s.CrossReferences...)
	}
	return out
}

// composeAcronyms unions KnownSafe entries across inputs. Scope
// follows the inverse rule: an empty Scope means "document-wide",
// so once any input declares Acronyms with no Scope restriction
// the composed Scope becomes nil (document-wide) — narrowing it
// to another input's restricted list would silently weaken the
// document-wide check the first input asked for. Two restricted
// inputs union their Scope lists.
func composeAcronyms(out *Schema, schemas []*Schema) {
	scopeWidened := false
	for _, s := range schemas {
		if s.Acronyms == nil {
			continue
		}
		if out.Acronyms == nil {
			out.Acronyms = &AcronymRule{}
		}
		out.Acronyms.KnownSafe = unionStrings(out.Acronyms.KnownSafe, s.Acronyms.KnownSafe)
		if scopeWidened {
			continue
		}
		if len(s.Acronyms.Scope) == 0 {
			scopeWidened = true
			out.Acronyms.Scope = nil
			continue
		}
		out.Acronyms.Scope = unionStrings(out.Acronyms.Scope, s.Acronyms.Scope)
	}
}

func composeIndex(out *Schema, schemas []*Schema) error {
	for _, s := range schemas {
		if s.Index == nil {
			continue
		}
		if out.Index == nil {
			out.Index = &IndexSpec{Output: s.Index.Output}
			out.Index.Include = append([]string(nil), s.Index.Include...)
			continue
		}
		if out.Index.Output != s.Index.Output {
			return fmt.Errorf(
				"conflicting schema.index.output values across "+
					"composed schemas: %q and %q",
				out.Index.Output, s.Index.Output)
		}
		out.Index.Include = unionStrings(out.Index.Include, s.Index.Include)
	}
	return nil
}
