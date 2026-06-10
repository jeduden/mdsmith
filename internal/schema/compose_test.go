package schema

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// lit builds a literal exact-match scope (the `heading: "X"` sugar):
// a matcher whose regex is the text and whose cardinality is exactly
// one — i.e. a required section.
func lit(text string) Scope {
	return Scope{Heading: text, Matcher: &Matcher{Regex: text}}
}

// slot builds a `## ...` wildcard scope: any heading, zero or more.
func slot() Scope {
	return Scope{
		Heading: SectionWildcard,
		Matcher: &Matcher{Regex: ".+", Repeat: Repeat{Set: true, Min: 0}},
	}
}

func TestCompose_NilInputs(t *testing.T) {
	out, err := Compose()
	require.NoError(t, err)
	assert.Nil(t, out)

	out, err = Compose(nil, nil)
	require.NoError(t, err)
	assert.Nil(t, out)
}

func TestCompose_SingleInputReturnedUnchanged(t *testing.T) {
	in := &Schema{Source: "a", Sections: []Scope{lit("Goal")}}
	out, err := Compose(in)
	require.NoError(t, err)
	assert.Same(t, in, out)
}

// TestCompose_DisjointRequiredSections is acceptance criterion #1: a
// file resolving to two kinds with disjoint required sections must
// require both. Kind A requires Goal; kind B requires Risks.
func TestCompose_DisjointRequiredSections(t *testing.T) {
	a := &Schema{Sections: []Scope{lit("Goal")}}
	b := &Schema{Sections: []Scope{lit("Risks")}}
	out, err := Compose(a, b)
	require.NoError(t, err)
	require.Len(t, out.Sections, 2)
	assert.Equal(t, "Goal", out.Sections[0].Heading)
	assert.True(t, out.Sections[0].Required())
	assert.Equal(t, "Risks", out.Sections[1].Heading)
	assert.True(t, out.Sections[1].Required())
}

// TestCompose_DisjointFrontmatterKeys is acceptance criterion #2: a
// file resolving to two kinds with disjoint required frontmatter
// keys must require both.
func TestCompose_DisjointFrontmatterKeys(t *testing.T) {
	a := &Schema{Frontmatter: map[string]string{
		"id": `=~"^A-[0-9]+$"`,
	}}
	b := &Schema{Frontmatter: map[string]string{
		"category": `"alpha" | "beta"`,
	}}
	out, err := Compose(a, b)
	require.NoError(t, err)
	require.Contains(t, out.Frontmatter, "id")
	require.Contains(t, out.Frontmatter, "category")
	assert.Equal(t, `=~"^A-[0-9]+$"`, out.Frontmatter["id"])
	assert.Equal(t, `"alpha" | "beta"`, out.Frontmatter["category"])
}

// TestCompose_SharedFrontmatterKeyConjoins exercises CUE conjunction:
// when two schemas constrain the same key, the composed expression
// is the conjunction of both.
func TestCompose_SharedFrontmatterKeyConjoins(t *testing.T) {
	a := &Schema{Frontmatter: map[string]string{
		"status": `"draft" | "ratified"`,
	}}
	b := &Schema{Frontmatter: map[string]string{
		"status": `"ratified" | "deprecated"`,
	}}
	out, err := Compose(a, b)
	require.NoError(t, err)
	require.Contains(t, out.Frontmatter, "status")
	assert.Contains(t, out.Frontmatter["status"], "draft")
	assert.Contains(t, out.Frontmatter["status"], "deprecated")
	assert.Contains(t, out.Frontmatter["status"], "&")
}

// TestCompose_SharedFrontmatterIdenticalExprStaysVerbatim verifies a
// shared constraint that's identical in both inputs is not wrapped in
// redundant parens.
func TestCompose_SharedFrontmatterIdenticalExprStaysVerbatim(t *testing.T) {
	expr := `string & != ""`
	a := &Schema{Frontmatter: map[string]string{"title": expr}}
	b := &Schema{Frontmatter: map[string]string{"title": expr}}
	out, err := Compose(a, b)
	require.NoError(t, err)
	assert.Equal(t, expr, out.Frontmatter["title"])
}

// TestCompose_MergesSameHeadingScopes covers the rule-readme +
// directive-rule-readme shape: each schema wraps its required
// sections in a shared `{id}: {name}` H1. After composition the
// H1 wrapper must appear once and its child sections include both
// inputs' children — without this, a file resolving to both kinds
// would be required to have two H1 headings, only one of which can
// exist in the same document.
func TestCompose_MergesSameHeadingScopes(t *testing.T) {
	wrap := func(child Scope) Scope {
		return Scope{
			Heading:  "{id}: {name}",
			Matcher:  &Matcher{Regex: `\#(fmvar(id)): \#(fmvar(name))`},
			Sections: []Scope{child},
		}
	}
	a := &Schema{RootLevel: 1, Sections: []Scope{wrap(lit("Goal"))}}
	b := &Schema{RootLevel: 1, Sections: []Scope{wrap(lit("Risks"))}}
	out, err := Compose(a, b)
	require.NoError(t, err)
	require.Len(t, out.Sections, 1)
	root := out.Sections[0]
	assert.Equal(t, "{id}: {name}", root.Heading)
	require.Len(t, root.Sections, 2)
	assert.Equal(t, "Goal", root.Sections[0].Heading)
	assert.Equal(t, "Risks", root.Sections[1].Heading)
}

// TestCompose_MergesLiteralHeadingScopes covers the case where two
// schemas share a literal heading. Their child sections combine.
func TestCompose_MergesLiteralHeadingScopes(t *testing.T) {
	mk := func(child Scope) *Schema {
		return &Schema{Sections: []Scope{{
			Heading:  "Meta-Information",
			Matcher:  &Matcher{Regex: "Meta-Information"},
			Sections: []Scope{child},
		}}}
	}
	out, err := Compose(mk(lit("ID")), mk(lit("Owner")))
	require.NoError(t, err)
	require.Len(t, out.Sections, 1)
	assert.Equal(t, "Meta-Information", out.Sections[0].Heading)
	require.Len(t, out.Sections[0].Sections, 2)
	headings := []string{
		out.Sections[0].Sections[0].Heading,
		out.Sections[0].Sections[1].Heading,
	}
	assert.Contains(t, headings, "ID")
	assert.Contains(t, headings, "Owner")
}

// TestCompose_PreservesWildcardSlots covers the rule-readme proto.md
// shape: each schema's sections are interleaved with wildcard slots
// (`## ...`). Composition must keep the slots distinct so each one
// still tolerates extras at its position.
func TestCompose_PreservesWildcardSlots(t *testing.T) {
	a := &Schema{Sections: []Scope{slot(), lit("Config"), slot()}}
	b := &Schema{Sections: []Scope{slot(), lit("Pattern"), slot()}}
	out, err := Compose(a, b)
	require.NoError(t, err)
	// 4 wildcard slots (none merge) + 2 unique headings.
	require.Len(t, out.Sections, 6)
}

// TestCompose_StrictClosedWins checks the closed-flag rule: if any
// composed scope's input was closed, the merged scope is closed.
func TestCompose_StrictClosedWins(t *testing.T) {
	a := &Schema{Sections: []Scope{{
		Heading: "Meta", Matcher: &Matcher{Regex: "Meta"}, Closed: true,
	}}}
	b := &Schema{Sections: []Scope{{
		Heading: "Meta", Matcher: &Matcher{Regex: "Meta"}, Closed: false,
	}}}
	out, err := Compose(a, b)
	require.NoError(t, err)
	require.Len(t, out.Sections, 1)
	assert.True(t, out.Sections[0].Closed,
		"any closed=true input must close the merged scope")
}

// TestCompose_RootClosedStricterWins exercises the root-level
// closed flag the same way the scope-level test does.
func TestCompose_RootClosedStricterWins(t *testing.T) {
	a := &Schema{Closed: true}
	b := &Schema{Closed: false}
	out, err := Compose(a, b)
	require.NoError(t, err)
	assert.True(t, out.Closed)
}

// TestCompose_RootLevelMismatchErrors regresses the Copilot
// review on PR #288: composing schemas with different
// EffectiveRootLevel values would silently mis-validate the
// second input's sections (validate.go positions the section
// walk at the composed RootLevel). The composer must surface
// the mismatch as a config error.
func TestCompose_RootLevelMismatchErrors(t *testing.T) {
	a := &Schema{
		Source: "proto.md", RootLevel: 1,
		Sections: []Scope{{Heading: "?", Matcher: &Matcher{Regex: ".+"}}},
	}
	b := &Schema{
		Source: "kind <inline>", RootLevel: 2,
		Sections: []Scope{lit("Goal")},
	}
	_, err := Compose(a, b)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "root heading level")
}

// TestCompose_RootLevelMatchOK confirms two schemas that agree
// on RootLevel compose without error.
func TestCompose_RootLevelMatchOK(t *testing.T) {
	a := &Schema{RootLevel: 2, Sections: []Scope{lit("Goal")}}
	b := &Schema{RootLevel: 2, Sections: []Scope{lit("Risks")}}
	out, err := Compose(a, b)
	require.NoError(t, err)
	assert.Equal(t, 2, out.RootLevel)
}

// TestCompose_RequiredWinsAcrossInputs verifies the stricter
// cardinality wins: a section that is optional (`repeat: {min:0}`)
// in one input but required (exactly one) in another ends up
// required in the composed scope.
func TestCompose_RequiredWinsAcrossInputs(t *testing.T) {
	optional := &Schema{Sections: []Scope{{
		Heading: "Meta",
		Matcher: &Matcher{Regex: "Meta", Repeat: Repeat{Set: true, Min: 0, Max: 1}},
	}}}
	required := &Schema{Sections: []Scope{lit("Meta")}}
	out, err := Compose(optional, required)
	require.NoError(t, err)
	require.Len(t, out.Sections, 1)
	assert.True(t, out.Sections[0].Required(),
		"a section required by any input must be required in the result")
}

// TestComposeAcronyms_DocumentWideOverridesScope regresses the
// Copilot review on PR #288: when one input declares acronyms
// document-wide (empty Scope) and another restricts to specific
// sections, composing would previously silently narrow the
// document-wide check. Document-wide must win.
func TestComposeAcronyms_DocumentWideOverridesScope(t *testing.T) {
	a := &Schema{Acronyms: &AcronymRule{KnownSafe: []string{"API"}}}
	b := &Schema{Acronyms: &AcronymRule{
		KnownSafe: []string{"HTTP"}, Scope: []string{"Check"},
	}}
	out, err := Compose(a, b)
	require.NoError(t, err)
	require.NotNil(t, out.Acronyms)
	assert.Empty(t, out.Acronyms.Scope,
		"document-wide acronym check must survive composition with a restricted input")
	assert.ElementsMatch(t, []string{"API", "HTTP"}, out.Acronyms.KnownSafe)
}

// TestComposeAcronyms_DocumentWideOverridesScopeReverseOrder
// verifies the order-independence of the document-wide rule:
// even when the document-wide input arrives second, it widens
// the prior restriction.
func TestComposeAcronyms_DocumentWideOverridesScopeReverseOrder(t *testing.T) {
	a := &Schema{Acronyms: &AcronymRule{
		KnownSafe: []string{"HTTP"}, Scope: []string{"Check"},
	}}
	b := &Schema{Acronyms: &AcronymRule{KnownSafe: []string{"API"}}}
	out, err := Compose(a, b)
	require.NoError(t, err)
	require.NotNil(t, out.Acronyms)
	assert.Empty(t, out.Acronyms.Scope,
		"document-wide acronym check arriving second must widen the prior restriction")
}

// TestComposeAcronyms_BothRestricted_UnionsScope verifies the
// scope union still applies when neither input is document-wide.
func TestComposeAcronyms_BothRestricted_UnionsScope(t *testing.T) {
	a := &Schema{Acronyms: &AcronymRule{Scope: []string{"Check"}}}
	b := &Schema{Acronyms: &AcronymRule{Scope: []string{"Expected"}}}
	out, err := Compose(a, b)
	require.NoError(t, err)
	require.NotNil(t, out.Acronyms)
	assert.ElementsMatch(t,
		[]string{"Check", "Expected"}, out.Acronyms.Scope)
}

// TestCompose_FilenameIdenticalNotConflict covers the
// `out.Filename != s.Filename` false branch: two schemas declaring
// the same non-empty filename pattern must NOT error.
func TestCompose_FilenameIdenticalNotConflict(t *testing.T) {
	a := &Schema{Filename: "[0-9]*.md"}
	b := &Schema{Filename: "[0-9]*.md"}
	out, err := Compose(a, b)
	require.NoError(t, err)
	assert.Equal(t, "[0-9]*.md", out.Filename)
}

// TestCompose_MergeScopeRulesOneSideEmpty covers the
// `len(b) == 0` false branch in mergeScopeRules — exercised
// when a side has rules and the other doesn't.
func TestCompose_MergeScopeRulesOneSideEmpty(t *testing.T) {
	a := &Schema{Sections: []Scope{lit("Loose")}}
	b := &Schema{Sections: []Scope{{
		Heading: "Loose",
		Matcher: &Matcher{Regex: "Loose"},
		Rules:   map[string]map[string]any{"line-length": {"max": 80}},
	}}}
	out, err := Compose(a, b)
	require.NoError(t, err)
	require.Len(t, out.Sections, 1)
	require.NotNil(t, out.Sections[0].Rules)
	assert.Contains(t, out.Sections[0].Rules, "line-length")
}

// TestCanMergeByHeading_PreambleDirect targets the `sc.Preamble`
// guard of canMergeByHeading. The caller short-circuits preambles
// before reaching the helper, but the safety branch must still
// hold so a hand-authored caller doesn't silently merge preambles.
func TestCanMergeByHeading_PreambleDirect(t *testing.T) {
	assert.False(t, canMergeByHeading(Scope{Preamble: true, Heading: "stale"}))
}

// TestCompose_FilenameFirstNonEmpty picks the first non-empty
// filename pattern; conflicting patterns are an error.
func TestCompose_FilenameFirstNonEmpty(t *testing.T) {
	a := &Schema{Filename: ""}
	b := &Schema{Filename: "[0-9]*.md"}
	out, err := Compose(a, b)
	require.NoError(t, err)
	assert.Equal(t, "[0-9]*.md", out.Filename)
}

func TestCompose_FilenameConflictErrors(t *testing.T) {
	a := &Schema{Filename: "AAA*.md"}
	b := &Schema{Filename: "BBB*.md"}
	_, err := Compose(a, b)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "filename")
}

// TestCompose_DisjointCardinalityErrors drives the empty-intersection
// guard through the public Compose path: two root schemas declare the
// same Step heading with disjoint run lengths (1..3 vs 5..10). The
// error must propagate through composeSectionLists and mergeScopes
// rather than silently honoring one input.
func TestCompose_DisjointCardinalityErrors(t *testing.T) {
	a := &Schema{Sections: []Scope{{
		Heading: "Step",
		Matcher: &Matcher{Regex: "Step", Repeat: Repeat{Set: true, Min: 1, Max: 3}},
	}}}
	b := &Schema{Sections: []Scope{{
		Heading: "Step",
		Matcher: &Matcher{Regex: "Step", Repeat: Repeat{Set: true, Min: 5, Max: 10}},
	}}}
	_, err := Compose(a, b)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "disjoint cardinality")
}

// TestCompose_NestedDisjointCardinalityErrors covers mergeScopes'
// recursive composeSectionLists error arm: a shared Goal parent
// merges fine, but its Step child has disjoint cardinality, so the
// error surfaces from the nested composition.
func TestCompose_NestedDisjointCardinalityErrors(t *testing.T) {
	child := func(min, max int) Scope {
		return Scope{
			Heading: "Goal", Matcher: &Matcher{Regex: "Goal"},
			Sections: []Scope{{
				Heading: "Step",
				Matcher: &Matcher{Regex: "Step",
					Repeat: Repeat{Set: true, Min: min, Max: max}},
			}},
		}
	}
	a := &Schema{Sections: []Scope{child(1, 3)}}
	b := &Schema{Sections: []Scope{child(5, 10)}}
	_, err := Compose(a, b)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "disjoint cardinality")
}

// TestComposeSectionLists_PreambleMergeError white-box-covers the
// preamble error-return arm: two lists each lead with a preamble
// whose matcher carries a disjoint run length, so merging the
// second into the first must surface the composition error.
func TestComposeSectionLists_PreambleMergeError(t *testing.T) {
	pre := func(min, max int) Scope {
		return Scope{
			Preamble: true,
			Matcher: &Matcher{Regex: "x",
				Repeat: Repeat{Set: true, Min: min, Max: max}},
		}
	}
	_, err := composeSectionLists([][]Scope{
		{pre(1, 3)},
		{pre(5, 10)},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "disjoint cardinality")
}

// TestCompose_StrictClosedWinsFromSecondInput regresses the
// branch where the SECOND merged scope (b) carries Closed=true
// while the first didn't — `out.Closed = true` must still fire.
func TestCompose_StrictClosedWinsFromSecondInput(t *testing.T) {
	a := &Schema{Sections: []Scope{{
		Heading: "Meta", Matcher: &Matcher{Regex: "Meta"}, Closed: false,
	}}}
	b := &Schema{Sections: []Scope{{
		Heading: "Meta", Matcher: &Matcher{Regex: "Meta"}, Closed: true,
	}}}
	out, err := Compose(a, b)
	require.NoError(t, err)
	require.Len(t, out.Sections, 1)
	assert.True(t, out.Sections[0].Closed,
		"the second input's Closed=true must propagate to the merged scope")
}

// TestCompose_PreamblePrependedShiftsIndices covers the
// index-shift branch in composeSectionLists: when the composed
// list already has named scopes and a later input starts with a
// preamble, the preamble is prepended and the indexByHeading
// entries must shift by one.
func TestCompose_PreamblePrependedShiftsIndices(t *testing.T) {
	a := &Schema{Sections: []Scope{lit("Goal"), lit("Risks")}}
	b := &Schema{Sections: []Scope{
		{Preamble: true},
		// Same heading as a's first entry — must still merge
		// after the indexByHeading entries shift.
		{Heading: "Goal", Matcher: &Matcher{Regex: "Goal"},
			Sections: []Scope{lit("Why")}},
	}}
	out, err := Compose(a, b)
	require.NoError(t, err)
	// Expected order: preamble, Goal (merged), Risks.
	require.Len(t, out.Sections, 3)
	assert.True(t, out.Sections[0].Preamble,
		"preamble must land at index 0 after shift")
	assert.Equal(t, "Goal", out.Sections[1].Heading,
		"Goal scope must be findable by its shifted index")
	require.Len(t, out.Sections[1].Sections, 1,
		"Goal's child sections must come from the merged input")
	assert.Equal(t, "Why", out.Sections[1].Sections[0].Heading)
	assert.Equal(t, "Risks", out.Sections[2].Heading)
}

func TestCompose_PreambleAtStart(t *testing.T) {
	a := &Schema{Sections: []Scope{{Preamble: true}, lit("Goal")}}
	b := &Schema{Sections: []Scope{{Preamble: true}, lit("Risks")}}
	out, err := Compose(a, b)
	require.NoError(t, err)
	require.NotEmpty(t, out.Sections)
	assert.True(t, out.Sections[0].Preamble, "preamble must remain at index 0")
}

func TestCompose_IndexOutputConflictErrors(t *testing.T) {
	a := &Schema{Index: &IndexSpec{Output: "a.json", Include: []string{IndexIncludeHeadingsFlat}}}
	b := &Schema{Index: &IndexSpec{Output: "b.json", Include: []string{IndexIncludeWordCounts}}}
	_, err := Compose(a, b)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "schema.index.output")
}

func TestCompose_IndexMergesIncludes(t *testing.T) {
	a := &Schema{Index: &IndexSpec{Output: "x.json", Include: []string{IndexIncludeHeadingsFlat}}}
	b := &Schema{Index: &IndexSpec{Output: "x.json", Include: []string{IndexIncludeWordCounts}}}
	out, err := Compose(a, b)
	require.NoError(t, err)
	require.NotNil(t, out.Index)
	assert.Equal(t, "x.json", out.Index.Output)
	assert.ElementsMatch(t,
		[]string{IndexIncludeHeadingsFlat, IndexIncludeWordCounts},
		out.Index.Include)
}

func TestCompose_CrossRefsConcat(t *testing.T) {
	a := &Schema{CrossReferences: []CrossRef{{Pattern: "A", MustMatch: "a"}}}
	b := &Schema{CrossReferences: []CrossRef{{Pattern: "B", MustMatch: "b"}}}
	out, err := Compose(a, b)
	require.NoError(t, err)
	require.Len(t, out.CrossReferences, 2)
}

// TestCompose_MergesScopeRules covers the per-scope rule-overrides
// merge: when two schemas attach `rules:` to a scope sharing the
// same heading, the merged scope inherits both override sets. Later
// inputs win on key collisions.
func TestCompose_MergesScopeRules(t *testing.T) {
	a := &Schema{Sections: []Scope{{
		Heading: "Loose", Matcher: &Matcher{Regex: "Loose"},
		Rules: map[string]map[string]any{"line-length": {"max": 80}},
	}}}
	b := &Schema{Sections: []Scope{{
		Heading: "Loose", Matcher: &Matcher{Regex: "Loose"},
		Rules: map[string]map[string]any{
			"line-length":  {"max": 120}, // overrides a's max
			"no-bare-urls": {},
		},
	}}}
	out, err := Compose(a, b)
	require.NoError(t, err)
	require.Len(t, out.Sections, 1)
	rules := out.Sections[0].Rules
	require.NotNil(t, rules)
	assert.Equal(t, 120, rules["line-length"]["max"],
		"later scope rule should override earlier on key collision")
	assert.Contains(t, rules, "no-bare-urls",
		"distinct rule entries should union")
}

// TestCompose_CloneScopeDeepCopies ensures cloneScope copies nested
// slices and maps so mutating the composed schema's scopes can't
// leak back into the inputs.
func TestCompose_CloneScopeDeepCopies(t *testing.T) {
	original := &Schema{Sections: []Scope{{
		Heading:  "Loose",
		Matcher:  &Matcher{Regex: "Loose"},
		Rules:    map[string]map[string]any{"line-length": {"max": 80}},
		Sections: []Scope{lit("Child")},
		Content:  []ContentEntry{{Kind: ContentKindCodeBlock, Lang: "go"}},
	}}}
	another := &Schema{Sections: []Scope{lit("Other")}}
	out, err := Compose(original, another)
	require.NoError(t, err)
	require.Len(t, out.Sections, 2)
	merged := out.Sections[0]
	merged.Rules["line-length"]["max"] = 999
	merged.Sections[0].Heading = "changed"
	merged.Matcher.Regex = "mutated"
	assert.Equal(t, 80, original.Sections[0].Rules["line-length"]["max"],
		"cloneScope must deep-copy rules")
	assert.Equal(t, "Child", original.Sections[0].Sections[0].Heading,
		"cloneScope must deep-copy nested sections")
	assert.Equal(t, "Loose", original.Sections[0].Matcher.Regex,
		"cloneScope must deep-copy the matcher")
}

// TestCompose_BareQuestionHeadingNotMerged covers the `?` branch in
// canMergeByHeading: two `# ?` scopes at the same root level don't
// merge (they're any-heading matchers, not literal identifiers).
func TestCompose_BareQuestionHeadingNotMerged(t *testing.T) {
	a := &Schema{Sections: []Scope{{Heading: "?", Matcher: &Matcher{Regex: ".+"}}}}
	b := &Schema{Sections: []Scope{{Heading: "?", Matcher: &Matcher{Regex: ".+"}}}}
	out, err := Compose(a, b)
	require.NoError(t, err)
	require.Len(t, out.Sections, 2)
}

// TestCompose_EmptyHeadingNotMerged covers the empty-string branch
// in canMergeByHeading.
func TestCompose_EmptyHeadingNotMerged(t *testing.T) {
	a := &Schema{Sections: []Scope{{Heading: "", Matcher: &Matcher{Regex: ".+"}}}}
	b := &Schema{Sections: []Scope{{Heading: "", Matcher: &Matcher{Regex: ".+"}}}}
	out, err := Compose(a, b)
	require.NoError(t, err)
	require.Len(t, out.Sections, 2)
}

// TestCompose_CloneSettingsMapNil exercises the nil branch in
// cloneSettingsMap by composing two scopes with the same heading and
// a nil scope-rules entry.
func TestCompose_CloneSettingsMapNil(t *testing.T) {
	a := &Schema{Sections: []Scope{{
		Heading: "Loose", Matcher: &Matcher{Regex: "Loose"},
		Rules: map[string]map[string]any{"line-length": nil},
	}}}
	b := &Schema{Sections: []Scope{lit("Other")}}
	out, err := Compose(a, b)
	require.NoError(t, err)
	require.Len(t, out.Sections, 2)
	require.NotNil(t, out.Sections[0].Rules)
	assert.Contains(t, out.Sections[0].Rules, "line-length")
}

// TestCompose_SourceLabelFromInputs verifies the composed label
// concatenates the inputs' Source values when any are set.
func TestCompose_SourceLabelFromInputs(t *testing.T) {
	a := &Schema{Source: "proto-a.md"}
	b := &Schema{Source: "proto-b.md"}
	out, err := Compose(a, b)
	require.NoError(t, err)
	assert.Contains(t, out.Source, "proto-a.md")
	assert.Contains(t, out.Source, "proto-b.md")
}

// TestCompose_SourceLabelDefault exercises the fallback when every
// input has an empty Source.
func TestCompose_SourceLabelDefault(t *testing.T) {
	a := &Schema{Source: ""}
	b := &Schema{Source: ""}
	out, err := Compose(a, b)
	require.NoError(t, err)
	assert.Equal(t, "composed", out.Source)
}

// TestCompose_PreambleDuplicateInOneListSkipped covers the
// `seenPreambleInList` defensive skip: a hand-built list with two
// preambles in the same input keeps only the first.
func TestCompose_PreambleDuplicateInOneListSkipped(t *testing.T) {
	a := &Schema{Sections: []Scope{
		{Preamble: true},
		{Preamble: true}, // duplicate; should be skipped
		lit("Goal"),
	}}
	b := &Schema{Sections: []Scope{lit("Risks")}}
	out, err := Compose(a, b)
	require.NoError(t, err)
	preambles := 0
	for _, sc := range out.Sections {
		if sc.Preamble {
			preambles++
		}
	}
	assert.Equal(t, 1, preambles,
		"duplicate preambles in a single input list must collapse")
}

// TestCompose_PreambleSecondInputMerges covers the existing-preamble
// merge path: when a later input has a preamble and the composed
// list already has one, they merge rather than appending a second.
func TestCompose_PreambleSecondInputMerges(t *testing.T) {
	a := &Schema{Sections: []Scope{{Preamble: true}, lit("Goal")}}
	b := &Schema{Sections: []Scope{{Preamble: true}, lit("Risks")}}
	out, err := Compose(a, b)
	require.NoError(t, err)
	preambles := 0
	for _, sc := range out.Sections {
		if sc.Preamble {
			preambles++
		}
	}
	assert.Equal(t, 1, preambles,
		"two input preambles should merge into one composed preamble")
}

func TestCompose_AcronymsMerge(t *testing.T) {
	a := &Schema{Acronyms: &AcronymRule{
		KnownSafe: []string{"API", "HTTP"}, Scope: []string{"Check"},
	}}
	b := &Schema{Acronyms: &AcronymRule{
		KnownSafe: []string{"HTTP", "TLS"}, Scope: []string{"Expected"},
	}}
	out, err := Compose(a, b)
	require.NoError(t, err)
	require.NotNil(t, out.Acronyms)
	assert.ElementsMatch(t, []string{"API", "HTTP", "TLS"}, out.Acronyms.KnownSafe)
	assert.ElementsMatch(t, []string{"Check", "Expected"}, out.Acronyms.Scope)
}

// TestCompose_ContentConcatenatesOnMerge verifies positional
// Content constraints concatenate when two same-heading scopes
// merge (earlier input first).
func TestCompose_ContentConcatenatesOnMerge(t *testing.T) {
	a := &Schema{Sections: []Scope{{
		Heading: "Examples", Matcher: &Matcher{Regex: "Examples"},
		Content: []ContentEntry{{Kind: ContentKindCodeBlock, Lang: "go"}},
	}}}
	b := &Schema{Sections: []Scope{{
		Heading: "Examples", Matcher: &Matcher{Regex: "Examples"},
		Content: []ContentEntry{{Kind: ContentKindTable}},
	}}}
	out, err := Compose(a, b)
	require.NoError(t, err)
	require.Len(t, out.Sections, 1)
	require.Len(t, out.Sections[0].Content, 2)
	assert.Equal(t, ContentKindCodeBlock, out.Sections[0].Content[0].Kind)
	assert.Equal(t, ContentKindTable, out.Sections[0].Content[1].Kind)
}

// TestMergeMatcher_BNil covers the `b == nil` arm: a same-heading
// scope whose Matcher is nil in the later input keeps the earlier
// input's matcher.
func TestMergeMatcher_BNil(t *testing.T) {
	a := &Matcher{Regex: "Meta"}
	got, err := mergeMatcher(a, nil)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "Meta", got.Regex)
	assert.NotSame(t, a, got, "result must be a clone, not the input pointer")
}

// TestMergeMatcher_ANil covers the symmetric `a == nil` arm.
func TestMergeMatcher_ANil(t *testing.T) {
	b := &Matcher{Regex: "Meta"}
	got, err := mergeMatcher(nil, b)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "Meta", got.Regex)
	assert.NotSame(t, b, got)
}

// TestMergeMatcher_StricterMaxAndRepeatSet covers two branches: the
// `a < b` arm of intersectMax (a's bounded max is smaller, so it
// wins) and the non-(1,1) `else` that sets an explicit Repeat. a
// allows 1..2, b allows 1..5 -> the intersection is 1..2.
func TestMergeMatcher_StricterMaxAndRepeatSet(t *testing.T) {
	a := &Matcher{Regex: "Step", Repeat: Repeat{Set: true, Min: 1, Max: 2}}
	b := &Matcher{Regex: "Step", Repeat: Repeat{Set: true, Min: 1, Max: 5}}
	got, err := mergeMatcher(a, b)
	require.NoError(t, err)
	require.NotNil(t, got)
	gotMin, gotMax := got.Repeat.Bounds()
	assert.Equal(t, 1, gotMin)
	assert.Equal(t, 2, gotMax, "the stricter (smaller) max must win")
	assert.True(t, got.Repeat.Set, "non-(1,1) bounds must set Repeat explicitly")
}

// TestMergeMatcher_RequiredWinsOverOptional covers the min-widening
// branch plus the explicit-Repeat else: an optional run (min 0)
// merged with a required single (1,1) yields a required matcher.
func TestMergeMatcher_RequiredWinsOverOptional(t *testing.T) {
	optional := &Matcher{Regex: "Meta", Repeat: Repeat{Set: true, Min: 0, Max: 1}}
	required := &Matcher{Regex: "Meta"} // exactly one
	got, err := mergeMatcher(optional, required)
	require.NoError(t, err)
	require.NotNil(t, got)
	gotMin, _ := got.Repeat.Bounds()
	assert.Equal(t, 1, gotMin, "required (min 1) must win over optional (min 0)")
}

// TestCloneContent_CopiesColumns covers the `e.Columns != nil`
// deep-copy branch: a table ContentEntry's Columns slice must be
// cloned, not aliased, when two same-heading scopes merge.
func TestCloneContent_CopiesColumns(t *testing.T) {
	cols := []string{"Name", "Type"}
	a := &Schema{Sections: []Scope{{
		Heading: "Schema", Matcher: &Matcher{Regex: "Schema"},
		Content: []ContentEntry{{Kind: ContentKindTable, Columns: cols}},
	}}}
	b := &Schema{Sections: []Scope{{
		Heading: "Schema", Matcher: &Matcher{Regex: "Schema"},
	}}}
	out, err := Compose(a, b)
	require.NoError(t, err)
	require.Len(t, out.Sections, 1)
	require.Len(t, out.Sections[0].Content, 1)
	got := out.Sections[0].Content[0].Columns
	require.Equal(t, []string{"Name", "Type"}, got)
	cols[0] = "MUTATED"
	assert.Equal(t, "Name", out.Sections[0].Content[0].Columns[0],
		"cloneContent must deep-copy Columns, not alias the input slice")
}

// TestMergeMatcher_SequentialOredAndUnbounded covers the
// `a.Sequential || b.Sequential` true arms and the unbounded-max
// path (aMax/bMax == 0): merging an unbounded sequential run with
// an unbounded plain run keeps Sequential and stays unbounded.
func TestMergeMatcher_SequentialOredAndUnbounded(t *testing.T) {
	a := &Matcher{
		Regex:      "Step",
		Repeat:     Repeat{Set: true, Min: 1, Max: 0}, // 1..unbounded
		Sequential: true,
	}
	b := &Matcher{
		Regex:  "Step",
		Repeat: Repeat{Set: true, Min: 1, Max: 0},
	}
	got, err := mergeMatcher(a, b)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.True(t, got.Sequential, "Sequential must OR across inputs")
	gotMin, gotMax := got.Repeat.Bounds()
	assert.Equal(t, 1, gotMin)
	assert.Equal(t, 0, gotMax, "unbounded ∩ unbounded stays unbounded")
}

// TestMergeMatcher_SequentialFromBOnly covers the right operand of
// the `||` (a.Sequential false, b.Sequential true).
func TestMergeMatcher_SequentialFromBOnly(t *testing.T) {
	a := &Matcher{Regex: "Step", Repeat: Repeat{Set: true, Min: 1, Max: 0}}
	b := &Matcher{Regex: "Step", Repeat: Repeat{Set: true, Min: 1, Max: 0}, Sequential: true}
	got, err := mergeMatcher(a, b)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.True(t, got.Sequential)
}

// TestMergeMatcher_OptionalBothSides covers the `min == 1` false
// branch: two optional (min 0) runs merge to an optional run, so
// the explicit-Repeat else fires with min 0.
func TestMergeMatcher_OptionalBothSides(t *testing.T) {
	a := &Matcher{Regex: "Notes", Repeat: Repeat{Set: true, Min: 0, Max: 1}}
	b := &Matcher{Regex: "Notes", Repeat: Repeat{Set: true, Min: 0, Max: 1}}
	got, err := mergeMatcher(a, b)
	require.NoError(t, err)
	require.NotNil(t, got)
	gotMin, _ := got.Repeat.Bounds()
	assert.Equal(t, 0, gotMin, "two optional runs stay optional")
	assert.True(t, got.Repeat.Optional())
}

// TestMergeMatcher_BoundedAThenUnboundedB covers intersectMax's
// `b == 0` arm: a is bounded (max 3), b is unbounded (max 0). The
// intersection keeps the bounded side so the unbounded input never
// loosens it.
func TestMergeMatcher_BoundedAThenUnboundedB(t *testing.T) {
	a := &Matcher{Regex: "Step", Repeat: Repeat{Set: true, Min: 1, Max: 3}}
	b := &Matcher{Regex: "Step", Repeat: Repeat{Set: true, Min: 1, Max: 0}}
	got, err := mergeMatcher(a, b)
	require.NoError(t, err)
	require.NotNil(t, got)
	_, gotMax := got.Repeat.Bounds()
	assert.Equal(t, 3, gotMax,
		"a bounded max intersected with an unbounded one stays bounded")
}

// TestMergeMatcher_UnboundedAThenBoundedB covers intersectMax's
// `a == 0` arm: a is unbounded, b is bounded (max 4). The bounded
// side wins.
func TestMergeMatcher_UnboundedAThenBoundedB(t *testing.T) {
	a := &Matcher{Regex: "Step", Repeat: Repeat{Set: true, Min: 1, Max: 0}}
	b := &Matcher{Regex: "Step", Repeat: Repeat{Set: true, Min: 1, Max: 4}}
	got, err := mergeMatcher(a, b)
	require.NoError(t, err)
	require.NotNil(t, got)
	_, gotMax := got.Repeat.Bounds()
	assert.Equal(t, 4, gotMax,
		"an unbounded max intersected with a bounded one stays bounded")
}

// TestMergeMatcher_DisjointRangesError covers the empty-intersection
// guard: 1..3 and 5..10 share no satisfiable run length, so compose
// surfaces a config error instead of silently honoring one input.
func TestMergeMatcher_DisjointRangesError(t *testing.T) {
	a := &Matcher{Regex: "Step", Repeat: Repeat{Set: true, Min: 1, Max: 3}}
	b := &Matcher{Regex: "Step", Repeat: Repeat{Set: true, Min: 5, Max: 10}}
	got, err := mergeMatcher(a, b)
	assert.Nil(t, got)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "disjoint cardinality")
	assert.Contains(t, err.Error(), "1..3")
	assert.Contains(t, err.Error(), "5..10")
}

// TestMergeMatcher_DisjointWithUnboundedError covers the
// boundsLabel "unbounded" branch: a requires 5..unbounded, b caps
// at 1..2 — disjoint, and the error names the unbounded side.
func TestMergeMatcher_DisjointWithUnboundedError(t *testing.T) {
	a := &Matcher{Regex: "Step", Repeat: Repeat{Set: true, Min: 5, Max: 0}}
	b := &Matcher{Regex: "Step", Repeat: Repeat{Set: true, Min: 1, Max: 2}}
	got, err := mergeMatcher(a, b)
	assert.Nil(t, got)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "5..unbounded")
	assert.Contains(t, err.Error(), "1..2")
}

// TestCompose_ProjectionDefaults pins that the schema-level
// projection family survives composition: an empty value yields to a
// set one and equal values survive, so `projection: blocks` never
// silently disappears when kinds share a file.
func TestCompose_ProjectionDefaults(t *testing.T) {
	a := &Schema{RootLevel: 2, Projection: ProjectionBlocks}
	b := &Schema{RootLevel: 2, BlockParagraphs: ProjectionInline}
	out, err := Compose(a, b)
	require.NoError(t, err)
	assert.Equal(t, ProjectionBlocks, out.Projection,
		"an empty projection yields to the set one")
	assert.Equal(t, ProjectionInline, out.BlockParagraphs,
		"an empty block-paragraphs yields to the set one")

	out, err = Compose(a, &Schema{RootLevel: 2, Projection: ProjectionBlocks})
	require.NoError(t, err)
	assert.Equal(t, ProjectionBlocks, out.Projection, "equal values survive")
}

// TestCompose_ProjectionConflictErrors mirrors the bind rule: a
// genuine schema-level disagreement is a compose-time error, not a
// silent first-wins.
func TestCompose_ProjectionConflictErrors(t *testing.T) {
	a := &Schema{RootLevel: 2, BlockParagraphs: ProjectionInline}
	b := &Schema{RootLevel: 2, BlockParagraphs: ProjectionText}
	_, err := Compose(a, b)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "block-paragraphs")
	assert.Contains(t, err.Error(), "schema level")
}

// TestCompose_ScopeProjectionCarriesFromEitherSide pins the
// scope-level merge: when two composed kinds declare the same heading
// and only one sets `projection: blocks`, the merged scope keeps it.
func TestCompose_ScopeProjectionCarriesFromEitherSide(t *testing.T) {
	mk := func(projection string) *Schema {
		return &Schema{RootLevel: 2, Sections: []Scope{{
			Heading:    "Notes",
			Matcher:    &Matcher{Regex: "Notes"},
			Projection: projection,
		}}}
	}
	out, err := Compose(mk(""), mk(ProjectionBlocks))
	require.NoError(t, err)
	require.Len(t, out.Sections, 1)
	assert.Equal(t, ProjectionBlocks, out.Sections[0].Projection,
		"the second source's scope projection must survive the merge")

	_, err = Compose(
		&Schema{RootLevel: 2, Sections: []Scope{{
			Heading: "Notes", Matcher: &Matcher{Regex: "Notes"},
			BlockParagraphs: ProjectionInline,
			Projection:      ProjectionBlocks,
		}}},
		&Schema{RootLevel: 2, Sections: []Scope{{
			Heading: "Notes", Matcher: &Matcher{Regex: "Notes"},
			BlockParagraphs: ProjectionText,
			Projection:      ProjectionBlocks,
		}}})
	require.Error(t, err, "scope-level disagreement errors")
	assert.Contains(t, err.Error(), `heading "Notes"`)
}

// TestCompose_ProjectionKeyConflictErrors drives the projection-key
// conflict arms at both levels. Parse validation only admits `blocks`
// today, so the disagreement is hand-built — the arm must still hold
// for any future second projection value.
func TestCompose_ProjectionKeyConflictErrors(t *testing.T) {
	_, err := Compose(
		&Schema{RootLevel: 2, Projection: ProjectionBlocks},
		&Schema{RootLevel: 2, Projection: "spans"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "`projection:`")
	assert.Contains(t, err.Error(), "schema level")

	mk := func(projection string) *Schema {
		return &Schema{RootLevel: 2, Sections: []Scope{{
			Heading:    "Notes",
			Matcher:    &Matcher{Regex: "Notes"},
			Projection: projection,
		}}}
	}
	_, err = Compose(mk(ProjectionBlocks), mk("spans"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), `heading "Notes"`)
}

// TestCloneScope_CarriesProjectionFields pins that the projection
// family survives cloneScope's struct copy — mergeScopes relies on
// that carry when folding the second scope's values in.
func TestCloneScope_CarriesProjectionFields(t *testing.T) {
	sc := Scope{
		Heading:         "Notes",
		Projection:      ProjectionBlocks,
		BlockParagraphs: ProjectionInline,
	}
	out := cloneScope(sc)
	assert.Equal(t, ProjectionBlocks, out.Projection)
	assert.Equal(t, ProjectionInline, out.BlockParagraphs)
}

// TestCompose_BlockParagraphsRequiresProjection mirrors the
// parse-time co-presence guard at compose time: a merge must not
// synthesize `block-paragraphs` without `projection: blocks` at
// either level — the parser rejects that pair, so composition cannot
// be a back door to a silently dead setting.
func TestCompose_BlockParagraphsRequiresProjection(t *testing.T) {
	_, err := Compose(
		&Schema{RootLevel: 2, BlockParagraphs: ProjectionInline},
		&Schema{RootLevel: 2})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "without a schema-level")

	mk := func(projection, bp string) *Schema {
		return &Schema{RootLevel: 2, Sections: []Scope{{
			Heading:         "Notes",
			Matcher:         &Matcher{Regex: "Notes"},
			Projection:      projection,
			BlockParagraphs: bp,
		}}}
	}
	_, err = Compose(mk("", ProjectionInline), mk("", ""))
	require.Error(t, err)
	assert.Contains(t, err.Error(), `heading "Notes"`)

	// The coherent pair still composes.
	out, err := Compose(mk(ProjectionBlocks, ProjectionInline), mk("", ""))
	require.NoError(t, err)
	require.Len(t, out.Sections, 1)
	assert.Equal(t, ProjectionInline, out.Sections[0].BlockParagraphs)
}

// TestCompose_ProjectionOrderingSymmetry drives the directions the
// earlier tests leave implicit: first-source-wins for the carry, and
// the co-presence guard firing regardless of which input carries the
// orphaned `block-paragraphs`.
func TestCompose_ProjectionOrderingSymmetry(t *testing.T) {
	mk := func(projection string) *Schema {
		return &Schema{RootLevel: 2, Sections: []Scope{{
			Heading:    "Notes",
			Matcher:    &Matcher{Regex: "Notes"},
			Projection: projection,
		}}}
	}
	out, err := Compose(mk(ProjectionBlocks), mk(""))
	require.NoError(t, err)
	require.Len(t, out.Sections, 1)
	assert.Equal(t, ProjectionBlocks, out.Sections[0].Projection,
		"the first source's scope projection must survive too")

	_, err = Compose(
		&Schema{RootLevel: 2},
		&Schema{RootLevel: 2, BlockParagraphs: ProjectionInline})
	require.Error(t, err, "the guard fires when the second input carries it")
	assert.Contains(t, err.Error(), "without a schema-level")
}
