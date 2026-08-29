package schema

import (
	"path/filepath"
	"testing"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPatternHasInterp(t *testing.T) {
	assert.True(t, PatternHasInterp(`.apm/skills/\#(fmvar(name))/SKILL.md`))
	assert.False(t, PatternHasInterp("docs/**/*.md"))
	assert.False(t, PatternHasInterp("#(fmvar(name)).md"),
		"only the `\\#(` opener starts an interpolation")
}

func TestResolveGlobPattern_SubstitutesFrontmatterValue(t *testing.T) {
	got, err := ResolveGlobPattern(
		`.apm/skills/\#(fmvar(name))/SKILL.md`,
		map[string]any{"name": "code-review"})
	require.NoError(t, err)
	assert.Equal(t, ".apm/skills/code-review/SKILL.md", got)
	assert.True(t,
		doublestar.MatchUnvalidated(got, ".apm/skills/code-review/SKILL.md"))
}

func TestResolveGlobPattern_MismatchStillResolves(t *testing.T) {
	got, err := ResolveGlobPattern(
		`.apm/skills/\#(fmvar(name))/SKILL.md`,
		map[string]any{"name": "other"})
	require.NoError(t, err)
	assert.False(t,
		doublestar.MatchUnvalidated(got, ".apm/skills/code-review/SKILL.md"),
		"a name that disagrees with the directory must not match")
}

func TestResolveGlobPattern_MissingFieldErrors(t *testing.T) {
	_, err := ResolveGlobPattern(
		`.apm/skills/\#(fmvar(name))/SKILL.md`, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fmvar(name)")
	assert.Contains(t, err.Error(), "frontmatter value missing")
}

func TestResolveGlobPattern_NestedPathLookup(t *testing.T) {
	got, err := ResolveGlobPattern(
		`docs/\#(fmvar(meta.slug)).md`,
		map[string]any{"meta": map[string]any{"slug": "install"}})
	require.NoError(t, err)
	assert.Equal(t, "docs/install.md", got)
}

func TestResolveGlobPattern_EscapesGlobMetacharacters(t *testing.T) {
	// A frontmatter value carrying `*` must match literally rather
	// than turning into a wildcard.
	got, err := ResolveGlobPattern(
		`skills/\#(fmvar(name))/SKILL.md`,
		map[string]any{"name": "a*b"})
	require.NoError(t, err)
	assert.True(t, doublestar.MatchUnvalidated(got, "skills/a*b/SKILL.md"))
	assert.False(t, doublestar.MatchUnvalidated(got, "skills/axxb/SKILL.md"),
		"the `*` in the frontmatter value must not act as a wildcard")
}

// A `,` in the value must not open a new alternative when the
// surrounding pattern wraps the reference in a brace alternative.
func TestResolveGlobPattern_EscapesCommaInsideBraceAlternative(t *testing.T) {
	got, err := ResolveGlobPattern(
		`docs/{\#(fmvar(name)),other}.md`,
		map[string]any{"name": "a,b"})
	require.NoError(t, err)
	ok, err := doublestar.Match(got, "docs/a,b.md")
	require.NoError(t, err)
	assert.True(t, ok, "the literal value must still match")
	ok, err = doublestar.Match(got, "docs/a.md")
	require.NoError(t, err)
	assert.False(t, ok,
		"the `,` in the frontmatter value must not split the alternative")
}

// A `/` cannot be escaped into a literal: doublestar reads `\/` as
// the separator all the same, so a value carrying one would silently
// span directories and satisfy a single-segment reference. Report it
// instead.
func TestResolveGlobPattern_RejectsPathSeparatorInValue(t *testing.T) {
	_, err := ResolveGlobPattern(
		`.apm/skills/\#(fmvar(name))/SKILL.md`,
		map[string]any{"name": "a/b"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fmvar(name)")
	assert.Contains(t, err.Error(), "path separator")
}

func TestResolveGlobPattern_EscapedValueWorksWithFilepathMatch(t *testing.T) {
	got, err := ResolveGlobPattern(
		`\#(fmvar(id)).md`, map[string]any{"id": "a?b"})
	require.NoError(t, err)
	ok, err := filepath.Match(got, "a?b.md")
	require.NoError(t, err)
	assert.True(t, ok)
	ok, err = filepath.Match(got, "axb.md")
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestResolveGlobPattern_LeavesPlainPatternUntouched(t *testing.T) {
	got, err := ResolveGlobPattern("plan/[0-9]*_*.md", nil)
	require.NoError(t, err)
	assert.Equal(t, "plan/[0-9]*_*.md", got)
}

func TestResolveGlobPattern_RejectsUnknownHelper(t *testing.T) {
	_, err := ResolveGlobPattern(`step-\#(digits).md`, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "digits")
	assert.Contains(t, err.Error(), "fmvar(name)")
}

func TestValidateGlobInterps(t *testing.T) {
	require.NoError(t, ValidateGlobInterps("plan/[0-9]*.md"))
	require.NoError(t,
		ValidateGlobInterps(`.apm/skills/\#(fmvar(name))/SKILL.md`))
	require.NoError(t,
		ValidateGlobInterps(`docs/\#(fmvar("my-key")).md`))

	err := ValidateGlobInterps(`docs/\#(fmvar(my-key)).md`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be quoted")

	err = ValidateGlobInterps(`docs/\#(fmvar(name).md`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unterminated")

	err = ValidateGlobInterps(`docs/\#(digits).md`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fmvar(name)")
}

// ---- `filename:` wiring ----

func TestValidateFilename_FmvarMatchesFrontmatterValue(t *testing.T) {
	sch := &Schema{
		Filename: []string{`\#(fmvar(id))-notes.md`},
		Source:   "kind note",
	}
	doc := newDocFile(t, "rfc-7-notes.md", "---\nid: rfc-7\n---\n# T\n")
	diags := Validate(doc, sch,
		map[string]any{"id": "rfc-7"}, false, makeDiagForTest)
	assert.Empty(t, diags, "got %v", diagsMessages(diags))
}

func TestValidateFilename_FmvarMismatchReportsResolvedGlob(t *testing.T) {
	sch := &Schema{
		Filename: []string{`\#(fmvar(id))-notes.md`},
		Source:   "kind note",
	}
	doc := newDocFile(t, "rfc-8-notes.md", "---\nid: rfc-7\n---\n# T\n")
	diags := Validate(doc, sch,
		map[string]any{"id": "rfc-7"}, false, makeDiagForTest)
	require.Len(t, diags, 1, "got %v", diagsMessages(diags))
	assert.Contains(t, diags[0].Message, `filename: got "rfc-8-notes.md"`)
	assert.Contains(t, diags[0].Message, "rfc-7-notes.md",
		"the hint should show the pattern with front matter applied")
}

func TestValidateFilename_FmvarMissingFieldReportsClearly(t *testing.T) {
	sch := &Schema{
		Filename: []string{`\#(fmvar(id))-notes.md`},
		Source:   "kind note",
	}
	doc := newDocFile(t, "rfc-7-notes.md", "# T\n")
	diags := Validate(doc, sch, nil, false, makeDiagForTest)
	require.Len(t, diags, 1, "got %v", diagsMessages(diags))
	assert.Contains(t, diags[0].Message, "fmvar(id)")
	assert.Contains(t, diags[0].Message, "frontmatter value missing")
}

func TestDecodeFilenameField_RejectsMalformedInterp(t *testing.T) {
	_, err := DecodeFilenameField(`\#(fmvar(my-key)).md`)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be quoted")
}

// `filename:` is an OR list. One entry whose `\#(fmvar(...))`
// reference cannot resolve must not veto a basename that satisfies
// a sibling glob.
func TestValidateFilename_UnresolvableEntryDoesNotVetoSiblingGlob(t *testing.T) {
	sch := &Schema{
		Filename: []string{"README.md", `\#(fmvar(id))-notes.md`},
		Source:   "kind note",
	}
	doc := newDocFile(t, "README.md", "# T\n")
	diags := Validate(doc, sch, nil, false, makeDiagForTest)
	assert.Empty(t, diags, "got %v", diagsMessages(diags))
}

// When nothing matches, the unresolvable reference is still what the
// reader needs to see — it outranks the substituted-pattern hint.
func TestValidateFilename_UnresolvableEntrySurfacesWhenNothingMatches(t *testing.T) {
	sch := &Schema{
		Filename: []string{"README.md", `\#(fmvar(id))-notes.md`},
		Source:   "kind note",
	}
	doc := newDocFile(t, "other.md", "# T\n")
	diags := Validate(doc, sch, nil, false, makeDiagForTest)
	require.Len(t, diags, 1, "got %v", diagsMessages(diags))
	assert.Contains(t, diags[0].Message, "frontmatter value missing")
}

// The "with front matter applied" hint exists to show what an
// interpolating glob became. A sibling plain glob was never
// substituted, so listing it would imply a substitution that never
// happened.
func TestValidateFilename_HintListsOnlyInterpolatedGlobs(t *testing.T) {
	sch := &Schema{
		Filename: []string{"README.md", `\#(fmvar(id))-notes.md`},
		Source:   "kind note",
	}
	doc := newDocFile(t, "other.md", "---\nid: rfc-7\n---\n# T\n")
	diags := Validate(doc, sch,
		map[string]any{"id": "rfc-7"}, false, makeDiagForTest)
	require.Len(t, diags, 1, "got %v", diagsMessages(diags))
	assert.Contains(t, diags[0].Message,
		"with front matter applied: rfc-7-notes.md")
	assert.NotContains(t, diags[0].Message,
		"with front matter applied: README.md",
		"a plain sibling glob was never interpolated")
}

// An explicitly empty front-matter value is the degenerate
// empty-segment case ResolveGlobPattern exists to prevent, so it
// reports rather than substituting nothing.
func TestResolveGlobPattern_EmptyValueErrors(t *testing.T) {
	_, err := ResolveGlobPattern(
		`.apm/skills/\#(fmvar(name))/SKILL.md`,
		map[string]any{"name": ""})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fmvar(name)")
	assert.Contains(t, err.Error(), "empty")
}
