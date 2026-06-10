package schema

import (
	"errors"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtend_NilInputs(t *testing.T) {
	out, err := Extend(nil, nil)
	require.NoError(t, err)
	assert.Nil(t, out)
}

func TestExtend_NilParentReturnsChild(t *testing.T) {
	child := &Schema{Source: "child"}
	out, err := Extend(nil, child)
	require.NoError(t, err)
	assert.Same(t, child, out)
}

func TestExtend_NilChildReturnsParent(t *testing.T) {
	parent := &Schema{Source: "parent"}
	out, err := Extend(parent, nil)
	require.NoError(t, err)
	assert.Same(t, parent, out)
}

// TestExtend_FrontmatterInherits is the parent-only path: a child
// that declares no frontmatter receives every parent key verbatim.
func TestExtend_FrontmatterInherits(t *testing.T) {
	parent := &Schema{Frontmatter: map[string]string{
		"id":      `=~"^RFC-[0-9]{4}$"`,
		"authors": `[...string] & [_, ...string]`,
	}}
	child := &Schema{}
	out, err := Extend(parent, child)
	require.NoError(t, err)
	assert.Equal(t, `=~"^RFC-[0-9]{4}$"`, out.Frontmatter["id"])
	assert.Equal(t, `[...string] & [_, ...string]`, out.Frontmatter["authors"])
}

// TestExtend_FrontmatterChildAdds verifies that child-only keys
// are appended without disturbing the parent's keys.
func TestExtend_FrontmatterChildAdds(t *testing.T) {
	parent := &Schema{Frontmatter: map[string]string{"id": `string`}}
	child := &Schema{Frontmatter: map[string]string{"status": `"ratified"`}}
	out, err := Extend(parent, child)
	require.NoError(t, err)
	assert.Equal(t, `string`, out.Frontmatter["id"])
	assert.Equal(t, `"ratified"`, out.Frontmatter["status"])
}

// TestExtend_FrontmatterRefines exercises CUE unification: a child
// that narrows a parent's disjunction joins via `&` so the
// effective constraint requires the intersection.
func TestExtend_FrontmatterRefines(t *testing.T) {
	parent := &Schema{Frontmatter: map[string]string{
		"status": `"open" | "closed" | "ratified"`,
	}}
	child := &Schema{Frontmatter: map[string]string{
		"status": `"ratified"`,
	}}
	out, err := Extend(parent, child)
	require.NoError(t, err)
	require.Contains(t, out.Frontmatter, "status")
	assert.Contains(t, out.Frontmatter["status"], "ratified")
	assert.Contains(t, out.Frontmatter["status"], "&")
}

// TestExtend_FrontmatterIdenticalExprStaysVerbatim ensures we don't
// wrap a redundant expression in needless parens when parent and
// child carry the same constraint.
func TestExtend_FrontmatterIdenticalExprStaysVerbatim(t *testing.T) {
	expr := `string & != ""`
	parent := &Schema{Frontmatter: map[string]string{"title": expr}}
	child := &Schema{Frontmatter: map[string]string{"title": expr}}
	out, err := Extend(parent, child)
	require.NoError(t, err)
	assert.Equal(t, expr, out.Frontmatter["title"])
}

// TestExtend_FrontmatterAcceptsSelfReferencingConstraint covers
// the regression Copilot flagged on PR #365: a CUE constraint
// that references its own field name (e.g. `>=0` chained off a
// numeric field, or `len(_)`-style helpers in the struct
// scope) must compile through extendFrontmatter without false
// rejection. The check runs in `close({ <key>: <unified> })`
// context, the same scope ParseInline uses, so self-references
// resolve.
func TestExtend_FrontmatterAcceptsSelfReferencingConstraint(t *testing.T) {
	parent := &Schema{Frontmatter: map[string]string{
		"count": `int`,
	}}
	child := &Schema{Frontmatter: map[string]string{
		"count": `>=0`,
	}}
	out, err := Extend(parent, child)
	require.NoError(t, err)
	assert.Contains(t, out.Frontmatter["count"], "&")
}

// TestExtend_FrontmatterUnsatisfiableConflict is the conflict path:
// a child whose CUE expression cannot unify with the parent's
// surfaces an UnsatisfiableKeyError naming both expressions.
func TestExtend_FrontmatterUnsatisfiableConflict(t *testing.T) {
	parent := &Schema{Frontmatter: map[string]string{"status": `int`}}
	child := &Schema{Frontmatter: map[string]string{"status": `string`}}
	_, err := Extend(parent, child)
	require.Error(t, err)
	var keyErr *UnsatisfiableKeyError
	require.True(t, errors.As(err, &keyErr))
	assert.Equal(t, "status", keyErr.Key)
	assert.Equal(t, "int", keyErr.Parent)
	assert.Equal(t, "string", keyErr.Child)
}

// TestExtend_SectionsChildReplaces is the acceptance-criterion
// regression: child's section list wholly replaces the parent's;
// the parent's headings must not appear in the effective schema.
func TestExtend_SectionsChildReplaces(t *testing.T) {
	parent := &Schema{Sections: []Scope{lit("Context"), lit("Decision")}}
	child := &Schema{Sections: []Scope{lit("Summary"), lit("Background")}}
	out, err := Extend(parent, child)
	require.NoError(t, err)
	require.Len(t, out.Sections, 2)
	assert.Equal(t, "Summary", out.Sections[0].Heading)
	assert.Equal(t, "Background", out.Sections[1].Heading)
}

// TestExtend_SectionsParentFlowsThroughWhenChildEmpty verifies the
// inverse of the replacement rule: a child without sections inherits
// the parent's tree verbatim.
func TestExtend_SectionsParentFlowsThroughWhenChildEmpty(t *testing.T) {
	parent := &Schema{Sections: []Scope{lit("Context"), lit("Decision")}}
	child := &Schema{}
	out, err := Extend(parent, child)
	require.NoError(t, err)
	require.Len(t, out.Sections, 2)
	assert.Equal(t, "Context", out.Sections[0].Heading)
	assert.Equal(t, "Decision", out.Sections[1].Heading)
}

// TestExtend_FilenameChildWins covers the simple case: a child that
// declares filename overrides the parent.
func TestExtend_FilenameChildWins(t *testing.T) {
	parent := &Schema{Filename: "*.md"}
	child := &Schema{Filename: "RFC-*.md"}
	out, err := Extend(parent, child)
	require.NoError(t, err)
	assert.Equal(t, "RFC-*.md", out.Filename)
}

// TestExtend_FilenameInheritsFromParent verifies the missing-child
// branch: parent's pattern flows through.
func TestExtend_FilenameInheritsFromParent(t *testing.T) {
	parent := &Schema{Filename: "RFC-*.md"}
	child := &Schema{}
	out, err := Extend(parent, child)
	require.NoError(t, err)
	assert.Equal(t, "RFC-*.md", out.Filename)
}

// TestExtend_FilenameIdenticalNoConflict checks the boundary
// condition: identical patterns on both sides are not a conflict.
func TestExtend_FilenameIdenticalNoConflict(t *testing.T) {
	parent := &Schema{Filename: "RFC-*.md"}
	child := &Schema{Filename: "RFC-*.md"}
	out, err := Extend(parent, child)
	require.NoError(t, err)
	assert.Equal(t, "RFC-*.md", out.Filename)
}

// TestExtend_ClosedChildOverridesWhenSectionsPresent verifies that
// the child's `closed:` value wins only when the child carries its
// own section tree — otherwise the parent's strictness flows
// through alongside its inherited sections.
func TestExtend_ClosedChildOverridesWhenSectionsPresent(t *testing.T) {
	parent := &Schema{Closed: true, Sections: []Scope{lit("X")}}
	child := &Schema{Closed: false, Sections: []Scope{lit("Y")}}
	out, err := Extend(parent, child)
	require.NoError(t, err)
	assert.False(t, out.Closed)
}

func TestExtend_ClosedInheritsWhenChildHasNoSections(t *testing.T) {
	parent := &Schema{Closed: true, Sections: []Scope{lit("X")}}
	child := &Schema{}
	out, err := Extend(parent, child)
	require.NoError(t, err)
	assert.True(t, out.Closed)
}

// TestExtend_CrossReferencesChildReplaces covers the optional
// document-wide check: child's list replaces parent's when set.
func TestExtend_CrossReferencesChildReplaces(t *testing.T) {
	parent := &Schema{CrossReferences: []CrossRef{
		{Pattern: "P1", MustMatch: "Step {n}"},
	}}
	child := &Schema{CrossReferences: []CrossRef{
		{Pattern: "P2", MustMatch: "Step {n}"},
	}}
	out, err := Extend(parent, child)
	require.NoError(t, err)
	require.Len(t, out.CrossReferences, 1)
	assert.Equal(t, "P2", out.CrossReferences[0].Pattern)
}

func TestExtend_CrossReferencesInheritWhenChildEmpty(t *testing.T) {
	parent := &Schema{CrossReferences: []CrossRef{
		{Pattern: "P1", MustMatch: "Step {n}"},
	}}
	child := &Schema{}
	out, err := Extend(parent, child)
	require.NoError(t, err)
	require.Len(t, out.CrossReferences, 1)
	assert.Equal(t, "P1", out.CrossReferences[0].Pattern)
}

func TestExtend_AcronymsChildReplaces(t *testing.T) {
	parent := &Schema{Acronyms: &AcronymRule{KnownSafe: []string{"API"}}}
	child := &Schema{Acronyms: &AcronymRule{KnownSafe: []string{"RFC"}}}
	out, err := Extend(parent, child)
	require.NoError(t, err)
	require.NotNil(t, out.Acronyms)
	assert.Equal(t, []string{"RFC"}, out.Acronyms.KnownSafe)
}

func TestExtend_AcronymsInheritWhenChildEmpty(t *testing.T) {
	parent := &Schema{Acronyms: &AcronymRule{KnownSafe: []string{"API"}}}
	child := &Schema{}
	out, err := Extend(parent, child)
	require.NoError(t, err)
	require.NotNil(t, out.Acronyms)
	assert.Equal(t, []string{"API"}, out.Acronyms.KnownSafe)
}

func TestExtend_IndexChildReplaces(t *testing.T) {
	parent := &Schema{Index: &IndexSpec{Output: "p.json", Include: []string{IndexIncludeStepMap}}}
	child := &Schema{Index: &IndexSpec{Output: "c.json", Include: []string{IndexIncludeWordCounts}}}
	out, err := Extend(parent, child)
	require.NoError(t, err)
	require.NotNil(t, out.Index)
	assert.Equal(t, "c.json", out.Index.Output)
}

func TestExtend_IndexInheritsWhenChildEmpty(t *testing.T) {
	parent := &Schema{Index: &IndexSpec{Output: "p.json", Include: []string{IndexIncludeStepMap}}}
	child := &Schema{}
	out, err := Extend(parent, child)
	require.NoError(t, err)
	require.NotNil(t, out.Index)
	assert.Equal(t, "p.json", out.Index.Output)
}

// TestExtend_FrontmatterLinesMerges checks that per-key source line
// metadata composes: child-declared keys carry child lines, parent-
// only keys keep the parent's lines so a downstream validator can
// point at the right schema layer.
func TestExtend_FrontmatterLinesMerges(t *testing.T) {
	parent := &Schema{
		Frontmatter:      map[string]string{"id": `string`},
		FrontmatterLines: map[string]int{"id": 3},
	}
	child := &Schema{
		Frontmatter:      map[string]string{"status": `"ratified"`},
		FrontmatterLines: map[string]int{"status": 7},
	}
	out, err := Extend(parent, child)
	require.NoError(t, err)
	assert.Equal(t, 3, out.FrontmatterLines["id"])
	assert.Equal(t, 7, out.FrontmatterLines["status"])
}

// TestExtend_FilenameChildOverridesDifferentParent verifies the
// override path: the child's pattern wins even when it differs from
// the parent's, so a draft- or ratified- variant can declare its
// own filename without conflicting with the base kind.
func TestExtend_FilenameChildOverridesDifferentParent(t *testing.T) {
	parent := &Schema{Filename: "RFC-*.md"}
	child := &Schema{Filename: "DRAFT-*.md"}
	out, err := Extend(parent, child)
	require.NoError(t, err)
	assert.Equal(t, "DRAFT-*.md", out.Filename)
}

func TestUnsatisfiableKeyError_Error(t *testing.T) {
	cause := errors.New("conflicting values")
	err := &UnsatisfiableKeyError{
		Key:    "status",
		Parent: `"open" | "closed"`,
		Child:  `int`,
		Cause:  cause,
	}
	msg := err.Error()
	assert.Contains(t, msg, "status")
	assert.Contains(t, msg, "open")
	assert.Contains(t, msg, "int")
	assert.ErrorIs(t, err, cause)
}

// TestExtend_RootLevelFollowsChildWithSections — when child carries
// its own section tree it also dictates the root level (typical
// inline schema: H2 root); inheriting from a file-based parent
// would otherwise mis-root the validator's section walk.
func TestExtend_RootLevelFollowsChildWithSections(t *testing.T) {
	parent := &Schema{RootLevel: 1, Sections: []Scope{lit("X")}}
	child := &Schema{RootLevel: 2, Sections: []Scope{lit("Y")}}
	out, err := Extend(parent, child)
	require.NoError(t, err)
	assert.Equal(t, 2, out.RootLevel)
}

func TestExtend_RootLevelInheritsWhenChildHasNoSections(t *testing.T) {
	parent := &Schema{RootLevel: 1, Sections: []Scope{lit("X")}}
	child := &Schema{}
	out, err := Extend(parent, child)
	require.NoError(t, err)
	assert.Equal(t, 1, out.RootLevel)
}

// --- MergeRawMap ---

func TestMergeRawMap_NilInputs(t *testing.T) {
	out := MergeRawMap(nil, nil)
	assert.Nil(t, out)
}

func TestMergeRawMap_NilParentReturnsCloneOfChild(t *testing.T) {
	child := map[string]any{"filename": "x.md"}
	out := MergeRawMap(nil, child)
	assert.Equal(t, "x.md", out["filename"])
	// Map values are references; verify out doesn't alias child by
	// mutating out and confirming child is unchanged.
	out["new-key"] = "new"
	assert.NotContains(t, child, "new-key",
		"out must be a fresh map, not an alias of child")
}

func TestMergeRawMap_NilChildReturnsCloneOfParent(t *testing.T) {
	parent := map[string]any{"filename": "x.md"}
	out := MergeRawMap(parent, nil)
	assert.Equal(t, "x.md", out["filename"])
	out["new-key"] = "new"
	assert.NotContains(t, parent, "new-key",
		"out must be a fresh map, not an alias of parent")
}

func TestMergeRawMap_FrontmatterChildAddsKey(t *testing.T) {
	parent := map[string]any{"frontmatter": map[string]any{"id": "string"}}
	child := map[string]any{"frontmatter": map[string]any{"status": `"ratified"`}}
	out := MergeRawMap(parent, child)
	fm := out["frontmatter"].(map[string]any)
	assert.Equal(t, "string", fm["id"])
	assert.Equal(t, `"ratified"`, fm["status"])
}

func TestMergeRawMap_FrontmatterSharedKeyUnifies(t *testing.T) {
	parent := map[string]any{"frontmatter": map[string]any{
		"status": `"open" | "closed" | "ratified"`,
	}}
	child := map[string]any{"frontmatter": map[string]any{
		"status": `"ratified"`,
	}}
	out := MergeRawMap(parent, child)
	fm := out["frontmatter"].(map[string]any)
	expr, ok := fm["status"].(string)
	require.True(t, ok)
	assert.Contains(t, expr, "&")
	assert.Contains(t, expr, "ratified")
}

// TestMergeRawMap_FrontmatterConflictMergesWithoutError covers the
// post-refactor contract (review feedback on PR #365): MergeRawMap
// is now purely structural — it joins the two CUE expressions with
// `&` but does not evaluate the result. Use
// ValidateExtendedFrontmatter to detect unsatisfiable expressions
// at load time; the per-file merge path skips that step for
// performance once ValidateKinds has already run.
func TestMergeRawMap_FrontmatterConflictMergesWithoutError(t *testing.T) {
	parent := map[string]any{"frontmatter": map[string]any{"x": "int"}}
	child := map[string]any{"frontmatter": map[string]any{"x": "string"}}
	out := MergeRawMap(parent, child)
	fm := out["frontmatter"].(map[string]any)
	assert.Contains(t, fm["x"], "&", "shared key joins via CUE conjunction")
}

// TestMergeRawMap_UnencodableParentConstraintStillErrors pins the
// extends corner where the parent constraint is a value
// frontmatterExpr cannot encode (yaml.v3 turns `.inf` into a
// float64 +Inf) and the child overrides the same key: the
// malformed parent must survive the merge so
// ValidateExtendedFrontmatter names it, rather than the child
// override silently dropping the broken constraint.
func TestMergeRawMap_UnencodableParentConstraintStillErrors(t *testing.T) {
	parent := map[string]any{
		"frontmatter": map[string]any{"weight": math.Inf(1)},
	}
	child := map[string]any{
		"frontmatter": map[string]any{"weight": "5"},
	}
	out := MergeRawMap(parent, child)
	err := ValidateExtendedFrontmatter(out)
	require.Error(t, err)
	var fmErr *InvalidFrontmatterError
	require.True(t, errors.As(err, &fmErr))
	assert.Equal(t, "weight", fmErr.Key)
}

func TestValidateExtendedFrontmatter_RejectsUnsatisfiable(t *testing.T) {
	merged := map[string]any{"frontmatter": map[string]any{
		"x": "(int) & (string)",
	}}
	err := ValidateExtendedFrontmatter(merged)
	require.Error(t, err)
	var keyErr *UnsatisfiableKeyError
	require.True(t, errors.As(err, &keyErr))
	assert.Equal(t, "x", keyErr.Key)
	assert.Equal(t, "int", keyErr.Parent)
	assert.Equal(t, "string", keyErr.Child)
}

func TestValidateExtendedFrontmatter_AcceptsSatisfiable(t *testing.T) {
	merged := map[string]any{"frontmatter": map[string]any{
		"x": `("open" | "closed") & ("open")`,
	}}
	assert.NoError(t, ValidateExtendedFrontmatter(merged))
}

func TestValidateExtendedFrontmatter_NoFrontmatterKeyReturnsNil(t *testing.T) {
	merged := map[string]any{"filename": "x.md"}
	assert.NoError(t, ValidateExtendedFrontmatter(merged))
}

// TestValidateExtendedFrontmatter_AcceptsScalarValues verifies the
// scalar JSON-encoding path inside frontmatterExpr: a YAML number
// (or any other scalar) round-trips into a valid CUE literal and
// passes validation, rather than being silently skipped.
// TestValidateExtendedFrontmatter_SingleLayerConflictUsesInvalid
// pins the diagnostic-shape fix: a single-layer expression that
// compiles to bottom (`int & string` written verbatim, not as a
// parent/child join) surfaces as InvalidFrontmatterError so the
// message does not imply an inheritance conflict that does not
// exist.
func TestValidateExtendedFrontmatter_SingleLayerConflictUsesInvalid(t *testing.T) {
	merged := map[string]any{"frontmatter": map[string]any{
		"x": "int & string",
	}}
	err := ValidateExtendedFrontmatter(merged)
	require.Error(t, err)
	var invalid *InvalidFrontmatterError
	require.True(t, errors.As(err, &invalid),
		"single-layer conflict should not be reported as an unification clash")
	assert.Equal(t, "x", invalid.Key)
	assert.NotContains(t, err.Error(), "unify with parent")
}

func TestValidateExtendedFrontmatter_AcceptsScalarValues(t *testing.T) {
	merged := map[string]any{"frontmatter": map[string]any{
		"n": 42,
	}}
	assert.NoError(t, ValidateExtendedFrontmatter(merged))
}

func TestSplitUnifiedExpr_VerbatimReturnsSingleExpr(t *testing.T) {
	parent, child := splitUnifiedExpr(`"ratified"`)
	assert.Equal(t, `"ratified"`, parent)
	assert.Empty(t, child)
}

func TestSplitUnifiedExpr_UnifiedReturnsBothSides(t *testing.T) {
	parent, child := splitUnifiedExpr(`(int) & (string)`)
	assert.Equal(t, "int", parent)
	assert.Equal(t, "string", child)
}

func TestSplitUnifiedExpr_MalformedFallsBackToFullExpression(t *testing.T) {
	parent, child := splitUnifiedExpr(`(missing close`)
	assert.Equal(t, "(missing close", parent)
	assert.Empty(t, child)
}

// TestSplitUnifiedExpr_NestedParensSkipInnerCloses verifies the
// depth-tracking branch: nested parens inside the parent side
// should not be mistaken for the outer match.
func TestSplitUnifiedExpr_NestedParensSkipInnerCloses(t *testing.T) {
	parent, child := splitUnifiedExpr(`((a)) & (b)`)
	assert.Equal(t, "(a)", parent)
	assert.Equal(t, "b", child)
}

// TestSplitUnifiedExpr_SeparatorAbsentFallsBack covers the
// "matched the opening paren but the separator isn't `) & (`"
// branch: a single parenthesised expression like `(int)` is not
// the unified shape we produced.
func TestSplitUnifiedExpr_SeparatorAbsentFallsBack(t *testing.T) {
	parent, child := splitUnifiedExpr(`(int)`)
	assert.Equal(t, "(int)", parent)
	assert.Empty(t, child)
}

// TestSplitUnifiedExpr_UnbalancedNeverClosesFallsBack exercises
// the end-of-loop fallback when depth never returns to zero — an
// unbalanced expression where the prefix `(` count exceeds the
// suffix `)` count.
func TestSplitUnifiedExpr_UnbalancedNeverClosesFallsBack(t *testing.T) {
	parent, child := splitUnifiedExpr(`(((())`)
	assert.Equal(t, "(((())", parent)
	assert.Empty(t, child)
}

func TestMergeRawMap_SectionsChildReplaces(t *testing.T) {
	parent := map[string]any{"sections": []any{
		map[string]any{"heading": "A"},
	}}
	child := map[string]any{"sections": []any{
		map[string]any{"heading": "B"},
	}}
	out := MergeRawMap(parent, child)
	secs := out["sections"].([]any)
	require.Len(t, secs, 1)
	assert.Equal(t, "B", secs[0].(map[string]any)["heading"])
}

func TestMergeRawMap_SectionsInheritWhenChildAbsent(t *testing.T) {
	parent := map[string]any{"sections": []any{
		map[string]any{"heading": "A"},
	}}
	child := map[string]any{"filename": "x.md"}
	out := MergeRawMap(parent, child)
	secs := out["sections"].([]any)
	require.Len(t, secs, 1)
	assert.Equal(t, "A", secs[0].(map[string]any)["heading"])
}

func TestMergeRawMap_FilenameChildOverrides(t *testing.T) {
	parent := map[string]any{"filename": "p.md"}
	child := map[string]any{"filename": "c.md"}
	out := MergeRawMap(parent, child)
	assert.Equal(t, "c.md", out["filename"])
}

func TestMergeRawMap_FilenameInheritsFromParent(t *testing.T) {
	parent := map[string]any{"filename": "p.md"}
	child := map[string]any{"closed": true}
	out := MergeRawMap(parent, child)
	assert.Equal(t, "p.md", out["filename"])
}

func TestMergeRawMap_PreservesNonOverlappingKeys(t *testing.T) {
	parent := map[string]any{
		"acronyms":         map[string]any{"known-safe": []any{"API"}},
		"cross-references": []any{map[string]any{"pattern": "p"}},
	}
	child := map[string]any{"closed": true}
	out := MergeRawMap(parent, child)
	assert.NotNil(t, out["acronyms"])
	assert.NotNil(t, out["cross-references"])
	assert.True(t, out["closed"].(bool))
}

func TestCloneRawMap_Nil(t *testing.T) {
	out := cloneRawMap(nil)
	assert.Nil(t, out)
}

func TestCloneRawMap_CopiesEntries(t *testing.T) {
	src := map[string]any{"x": 1, "y": "z"}
	out := cloneRawMap(src)
	assert.Equal(t, src, out)
	out["x"] = 99
	assert.Equal(t, 1, src["x"], "clone must not alias original")
}

func TestStripOptionalSuffix(t *testing.T) {
	assert.Equal(t, "k", stripOptionalSuffix("k?"))
	assert.Equal(t, "k", stripOptionalSuffix("k"))
}

func TestCheckUnifiable_AcceptsUnifiable(t *testing.T) {
	assert.NoError(t, checkUnifiable(`("a" | "b") & "a"`))
}

func TestCheckUnifiable_RejectsBottom(t *testing.T) {
	assert.Error(t, checkUnifiable(`int & string`))
}

// TestCheckUnifiable_RejectsCompileError covers the syntax-error
// branch: CompileString returns a value whose Err() is non-nil
// when the expression doesn't parse.
func TestCheckUnifiable_RejectsCompileError(t *testing.T) {
	assert.Error(t, checkUnifiable(`((`))
}

// TestMergeRawMap_FrontmatterChildHasNoFrontmatterKey covers the
// `!childOK` branch: parent declares frontmatter but child has no
// frontmatter key, so the merged map keeps the parent's keys
// without iterating any child entries.
func TestMergeRawMap_FrontmatterChildHasNoFrontmatterKey(t *testing.T) {
	parent := map[string]any{"frontmatter": map[string]any{"a": "string"}}
	child := map[string]any{"filename": "x.md"}
	out := MergeRawMap(parent, child)
	fm := out["frontmatter"].(map[string]any)
	assert.Equal(t, "string", fm["a"])
}

// TestMergeRawMap_FrontmatterNonStringParentNormalises covers the
// path where a non-string parent value (a YAML number) is
// normalised through frontmatterExpr to its CUE literal form
// before unifying with the child.
func TestMergeRawMap_FrontmatterNonStringParentNormalises(t *testing.T) {
	parent := map[string]any{"frontmatter": map[string]any{"x": 42}}
	child := map[string]any{"frontmatter": map[string]any{"x": "string"}}
	out := MergeRawMap(parent, child)
	fm := out["frontmatter"].(map[string]any)
	merged, ok := fm["x"].(string)
	require.True(t, ok)
	assert.Contains(t, merged, "42")
	assert.Contains(t, merged, "string")
	assert.Contains(t, merged, "&")
}

// TestMergeRawMap_PreservesUnknownChildKey ensures a typoed or
// otherwise unrecognised top-level key on the child schema
// survives the merge. ParseInline catches the typo with its
// `unknown schema key` diagnostic; dropping it here would silence
// the user's misconfiguration.
func TestMergeRawMap_PreservesUnknownChildKey(t *testing.T) {
	parent := map[string]any{"frontmatter": map[string]any{"id": "string"}}
	child := map[string]any{"sectiosn": []any{}} // typo: sectiosn (missing n)
	out := MergeRawMap(parent, child)
	assert.Contains(t, out, "sectiosn",
		"unknown child key must survive so ParseInline can flag it")
}

// TestMergeRawMap_PreservesMalformedChildFrontmatter pins the
// review-feedback fix on PR #365: a present-but-not-a-mapping
// `frontmatter:` value (`frontmatter: "oops"`) flows through
// verbatim instead of being silently dropped. ParseInline then
// emits its specific "schema.frontmatter must be a mapping"
// diagnostic on the merged map.
func TestMergeRawMap_PreservesMalformedChildFrontmatter(t *testing.T) {
	parent := map[string]any{"frontmatter": map[string]any{"id": "string"}}
	child := map[string]any{"frontmatter": "oops"}
	out := MergeRawMap(parent, child)
	assert.Equal(t, "oops", out["frontmatter"],
		"child's malformed shape wins so ParseInline can flag it")
}

// TestMergeRawMap_PreservesMalformedParentFrontmatter covers the
// parent-only path: when only the parent carries a malformed
// frontmatter, the merge surfaces it so downstream parsing fails
// loudly rather than silently dropping the parent's constraints.
func TestMergeRawMap_PreservesMalformedParentFrontmatter(t *testing.T) {
	parent := map[string]any{"frontmatter": "oops"}
	child := map[string]any{"filename": "x.md"}
	out := MergeRawMap(parent, child)
	assert.Equal(t, "oops", out["frontmatter"],
		"parent's malformed shape stays visible to the parser")
}

// TestMergeRawMap_FrontmatterIdenticalExprStaysVerbatim covers
// the redundant-conjunction guard: when parent and child carry
// the same expression for a key, the merge keeps the verbatim
// form rather than building `(x) & (x)`.
func TestMergeRawMap_FrontmatterIdenticalExprStaysVerbatim(t *testing.T) {
	parent := map[string]any{"frontmatter": map[string]any{"x": `"open"`}}
	child := map[string]any{"frontmatter": map[string]any{"x": `"open"`}}
	out := MergeRawMap(parent, child)
	fm := out["frontmatter"].(map[string]any)
	assert.Equal(t, `"open"`, fm["x"], "identical expressions don't wrap in `&`")
}

// TestMergeRawMap_FrontmatterShortcutsExpandBeforeUnify covers
// review feedback on PR #365: bare-name frontmatter shortcuts
// (`date`, `nonEmpty`, …) must be expanded to their canonical CUE
// before being wrapped in `(parent) & (child)`, otherwise the
// unified expression carries an unresolved identifier into the
// validator.
func TestMergeRawMap_FrontmatterShortcutsExpandBeforeUnify(t *testing.T) {
	parent := map[string]any{"frontmatter": map[string]any{"d": "date"}}
	child := map[string]any{"frontmatter": map[string]any{"d": "nonEmpty"}}
	out := MergeRawMap(parent, child)
	fm := out["frontmatter"].(map[string]any)
	merged, ok := fm["d"].(string)
	require.True(t, ok)
	assert.NotContains(t, merged, "date", "bare shortcut must be expanded")
	assert.NotContains(t, merged, "nonEmpty", "bare shortcut must be expanded")
	assert.Contains(t, merged, "&")
}

// TestMergeRawMap_FrontmatterShortcutChildOnlyExpands covers the
// non-conflicting path: a child key that uses a shortcut survives
// the merge as the expanded CUE expression, not as the bare name.
func TestMergeRawMap_FrontmatterShortcutChildOnlyExpands(t *testing.T) {
	parent := map[string]any{}
	child := map[string]any{"frontmatter": map[string]any{"d": "date"}}
	out := MergeRawMap(parent, child)
	fm := out["frontmatter"].(map[string]any)
	expr, ok := fm["d"].(string)
	require.True(t, ok)
	assert.Contains(t, expr, "=~", "shortcut expands even without inheritance")
}

// TestMergeRawMap_FrontmatterParentShortcutExpands ensures the
// parent-only path normalises too — a kind that inherits its
// frontmatter from a parent declaring `date` ends up with the
// canonical CUE form.
func TestMergeRawMap_FrontmatterParentShortcutExpands(t *testing.T) {
	parent := map[string]any{"frontmatter": map[string]any{"d": "date"}}
	child := map[string]any{"frontmatter": map[string]any{"x": "string"}}
	out := MergeRawMap(parent, child)
	fm := out["frontmatter"].(map[string]any)
	expr, ok := fm["d"].(string)
	require.True(t, ok)
	assert.Contains(t, expr, "=~", "parent shortcut expands in merge output")
}

// TestNormalizeFrontmatterValue_ResolvesShortcut covers the bare-
// name expansion path of the normaliser.
func TestNormalizeFrontmatterValue_ResolvesShortcut(t *testing.T) {
	out := normalizeFrontmatterValue("date")
	assert.NotEqual(t, "date", out)
	assert.Contains(t, out, "=~")
}

// TestPublicNormalizeFrontmatterValue_ResolvesShortcut covers the
// exported coerce helper used by kindsout — bare-name shortcuts
// expand the same way the internal helper does.
func TestPublicNormalizeFrontmatterValue_ResolvesShortcut(t *testing.T) {
	out := NormalizeFrontmatterValue("date")
	assert.Contains(t, out, "=~")
}

// TestPublicNormalizeFrontmatterValue_ScalarsEncode confirms a
// non-string scalar (a YAML number, bool) renders as its JSON
// encoding rather than falling through to the empty-string
// fallback the caller previously emitted.
func TestPublicNormalizeFrontmatterValue_ScalarsEncode(t *testing.T) {
	assert.Equal(t, "42", NormalizeFrontmatterValue(42))
	assert.Equal(t, "true", NormalizeFrontmatterValue(true))
}

// TestPublicNormalizeFrontmatterValue_UnsupportedFallsBackToSprintf
// covers the final fallback: a value frontmatterExpr cannot
// resolve still gets a printable form so audit output never
// carries an empty `value` for a present constraint.
func TestPublicNormalizeFrontmatterValue_UnsupportedFallsBackToSprintf(t *testing.T) {
	type custom struct{ X int }
	out := NormalizeFrontmatterValue(custom{X: 7})
	assert.Contains(t, out, "7")
}

// TestPublicNormalizeFrontmatterValue_StringFallback verifies that
// a string that frontmatterExpr rejects (an unknown bare name)
// still renders verbatim rather than as an empty placeholder.
func TestPublicNormalizeFrontmatterValue_StringFallback(t *testing.T) {
	assert.Equal(t, "unknown-shortcut",
		NormalizeFrontmatterValue("unknown-shortcut"))
}

// TestNormalizeFrontmatterValue_PreservesRawCUE confirms raw CUE
// strings pass through unchanged.
func TestNormalizeFrontmatterValue_PreservesRawCUE(t *testing.T) {
	out := normalizeFrontmatterValue(`"ratified"`)
	assert.Equal(t, `"ratified"`, out)
}

// TestNormalizeFrontmatterValue_FallsBackOnError covers the
// unknown-shortcut path: the value passes through unchanged so a
// downstream parse surfaces the same error the user would have
// seen without the merge step.
func TestNormalizeFrontmatterValue_FallsBackOnError(t *testing.T) {
	out := normalizeFrontmatterValue("unknown-shortcut")
	assert.Equal(t, "unknown-shortcut", out)
}

// TestValidateExtendedFrontmatter_AcceptsShortcut covers a
// shortcut-bearing merged map: validation expands `date` like
// ParseInline would and accepts the resulting CUE.
func TestValidateExtendedFrontmatter_AcceptsShortcut(t *testing.T) {
	merged := map[string]any{"frontmatter": map[string]any{"d": "date"}}
	assert.NoError(t, ValidateExtendedFrontmatter(merged))
}

// TestValidateExtendedFrontmatter_RejectsUnknownShortcut covers
// the error path of frontmatterExpr inside the validator: an
// unknown bare name surfaces as an InvalidFrontmatterError naming
// the key — not an UnsatisfiableKeyError, which is reserved for
// genuine parent/child unification conflicts.
func TestValidateExtendedFrontmatter_RejectsUnknownShortcut(t *testing.T) {
	merged := map[string]any{"frontmatter": map[string]any{"d": "not-a-shortcut"}}
	err := ValidateExtendedFrontmatter(merged)
	require.Error(t, err)
	var keyErr *InvalidFrontmatterError
	require.True(t, errors.As(err, &keyErr),
		"unknown shortcut should surface as InvalidFrontmatterError, not a unification conflict")
	assert.Equal(t, "d", keyErr.Key)
	// Diagnostic must not imply a parent/child mismatch.
	assert.NotContains(t, err.Error(), "unify with parent")
}

func TestInvalidFrontmatterError_Error(t *testing.T) {
	cause := errors.New("unknown shortcut \"foo\"")
	err := &InvalidFrontmatterError{Key: "d", Value: "foo", Cause: cause}
	msg := err.Error()
	assert.Contains(t, msg, "d")
	assert.Contains(t, msg, "foo")
	assert.NotContains(t, msg, "parent")
	assert.NotContains(t, msg, "child")
	assert.ErrorIs(t, err, cause)
}
