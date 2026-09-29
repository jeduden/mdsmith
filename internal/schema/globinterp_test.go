package schema

import (
	"errors"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPatternHasInterp(t *testing.T) {
	assert.True(t, PatternHasInterp(`.apm/skills/\#(fmvar(name))/SKILL.md`))
	assert.True(t, PatternHasInterp(`docs/\#(fmvar("my-key")).md`))
	assert.False(t, PatternHasInterp("docs/**/*.md"))
	assert.False(t, PatternHasInterp("#(fmvar(name)).md"),
		"only the `\\#(` opener starts an interpolation")
}

// Before fmvar interpolation existed, `\#(` in a glob was an escaped
// `#` followed by `(`. Only a well-formed `\#(fmvar(<cue-path>))` is
// a reference now; every other `\#(` keeps its literal meaning, so a
// glob that loaded and matched before still does.
func TestPatternHasInterp_OnlyWellFormedFmvarIsAReference(t *testing.T) {
	for _, p := range []string{
		`notes/\#(draft)*.md`,       // a deliberate literal
		`step-\#(digits).md`,        // the regex-only helper
		`docs/\#(fmvar(my-key)).md`, // not a CUE path
		`docs/\#(fmvar(name).md`,    // unterminated
		`docs/\#(fmvar(name) x).md`, // trailing text in the body
		`docs/\\#(fmvar(name)).md`,  // `\\` escapes the backslash
	} {
		assert.False(t, PatternHasInterp(p), p)
	}
}

func TestResolveGlobPattern_LeavesLiteralOpenerUntouched(t *testing.T) {
	for _, p := range []string{
		`notes/\#(draft)*.md`,
		`step-\#(digits).md`,
		`docs/\#(fmvar(my-key)).md`,
	} {
		got, err := ResolveGlobPattern(p, nil)
		require.NoError(t, err, p)
		assert.Equal(t, p, got)
	}
	ok, err := doublestar.Match(`notes/\#(draft)*.md`, "notes/#(draft)-1.md")
	require.NoError(t, err)
	assert.True(t, ok, "the literal glob still matches what it matched before")
}

// A literal opener and a real reference can share one pattern; only
// the reference is substituted.
func TestResolveGlobPattern_MixedLiteralOpenerAndReference(t *testing.T) {
	got, err := ResolveGlobPattern(`notes/\#(draft)-\#(fmvar(id)).md`,
		map[string]any{"id": "7"})
	require.NoError(t, err)
	assert.Equal(t, `notes/\#(draft)-7.md`, got)
	ok, err := doublestar.Match(got, "notes/#(draft)-7.md")
	require.NoError(t, err)
	assert.True(t, ok)
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

// A `}` in the value must not close a surrounding brace alternative
// early: unescaped, `docs/{a}b,other}.md` would reject the literal
// `docs/a}b.md` (syntax error) and accept `docs/ab,other}.md`.
func TestResolveGlobPattern_EscapesClosingBraceInsideBraceAlternative(t *testing.T) {
	got, err := ResolveGlobPattern(
		`docs/{\#(fmvar(name)),other}.md`,
		map[string]any{"name": "a}b"})
	require.NoError(t, err)
	ok, err := doublestar.Match(got, "docs/a}b.md")
	require.NoError(t, err)
	assert.True(t, ok, "the literal value must still match")
	ok, err = doublestar.Match(got, "docs/ab,other}.md")
	require.NoError(t, err)
	assert.False(t, ok,
		"the `}` in the frontmatter value must not close the alternative")
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

// The `filename:` surface resolves through resolveFilenamePatterns,
// not ResolveGlobPattern: the latter backslash-escapes for doublestar,
// and filepath.Match reads `\?` as a literal backslash plus a
// wildcard on Windows.
func TestResolveFilenamePatterns_EscapedQuestionMarkMatchesLiterally(t *testing.T) {
	resolved, _, unresolved := resolveFilenamePatterns(
		[]string{`\#(fmvar(id)).md`}, map[string]any{"id": "a?b"})
	require.NoError(t, unresolved)
	require.Len(t, resolved, 1)
	ok, err := filepath.Match(resolved[0], "a?b.md")
	require.NoError(t, err)
	assert.True(t, ok)
	ok, err = filepath.Match(resolved[0], "axb.md")
	require.NoError(t, err)
	assert.False(t, ok)
}

// A `\` in the value keeps a backslash escape on the `filename:`
// surface: on POSIX it is filepath.Match's escape character, so the
// doubled form matches one literal backslash.
func TestResolveFilenamePatterns_EscapesBackslash(t *testing.T) {
	resolved, _, unresolved := resolveFilenamePatterns(
		[]string{`\#(fmvar(id)).md`}, map[string]any{"id": `a\b`})
	require.NoError(t, unresolved)
	assert.Equal(t, []string{`a\\b.md`}, resolved)
	if runtime.GOOS == "windows" {
		return // `\` never appears in a Windows basename
	}
	ok, err := filepath.Match(resolved[0], `a\b.md`)
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestResolveGlobPattern_LeavesPlainPatternUntouched(t *testing.T) {
	got, err := ResolveGlobPattern("plan/[0-9]*_*.md", nil)
	require.NoError(t, err)
	assert.Equal(t, "plan/[0-9]*_*.md", got)
}

// PathPatternSyntaxForm is what a kind `path-pattern:` is checked
// against doublestar's syntax with. A reference's own bytes are never
// glob syntax — a quoted CUE key may hold `[` or `{` — so they are
// replaced before the check.
func TestPathPatternSyntaxForm(t *testing.T) {
	quoted := `sub/\#(fmvar("a[b"))/x.md`
	require.False(t, doublestar.ValidatePattern(quoted),
		"precondition: the raw text is not a valid glob")
	assert.True(t, doublestar.ValidatePattern(PathPatternSyntaxForm(quoted)))
	assert.Equal(t, ".apm/skills/x/SKILL.md",
		PathPatternSyntaxForm(`.apm/skills/\#(fmvar(name))/SKILL.md`))

	// A pattern with no reference is checked as it was before
	// interpolation existed, literal `\#(` included.
	assert.Equal(t, "plan/[0-9]*.md", PathPatternSyntaxForm("plan/[0-9]*.md"))
	assert.Equal(t, `notes/\#(draft)*.md`,
		PathPatternSyntaxForm(`notes/\#(draft)*.md`))
}

// A malformed `fmvar` opener is matched literally, so it can no longer
// fail at config load. LiteralFmvarHint names it when the pattern
// then fails to match, which is where the author notices.
func TestLiteralFmvarHint(t *testing.T) {
	assert.Empty(t, LiteralFmvarHint("plan/[0-9]*.md"))
	assert.Empty(t, LiteralFmvarHint(`.apm/skills/\#(fmvar(name))/SKILL.md`))
	assert.Empty(t, LiteralFmvarHint(`notes/\#(draft)*.md`),
		"an opener that does not start with fmvar is a deliberate literal")

	h := LiteralFmvarHint(`docs/\#(fmvar(my-key)).md`)
	assert.Contains(t, h, "`\\#(fmvar(my-key))` is matched literally")
	assert.Contains(t, h, "must be quoted")

	h = LiteralFmvarHint(`docs/\#(fmvar(name).md`)
	assert.Contains(t, h, "`\\#(fmvar(name).md` is matched literally")
	assert.Contains(t, h, "unterminated")

	h = LiteralFmvarHint(`docs/\#(fmvar name).md`)
	assert.Contains(t, h, "only `fmvar(name)` is supported")
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

// A `filename:` glob that loaded before fmvar interpolation existed
// must still load: a `\#(` that is not a well-formed fmvar reference
// is a literal, not a config error.
func TestDecodeFilenameField_KeepsLiteralOpener(t *testing.T) {
	for _, p := range []string{
		`notes-\#(draft)*.md`,
		`\#(fmvar(my-key)).md`,
		`step-\#(digits).md`,
	} {
		got, err := DecodeFilenameField(p)
		require.NoError(t, err, p)
		assert.Equal(t, []string{p}, got)
	}
}

// The literal glob keeps matching exactly what filepath.Match matched
// before interpolation existed, on whichever platform runs the test.
func TestFilenameDiagnostic_LiteralOpenerMatchesAsBefore(t *testing.T) {
	const pat, base = `notes-\#(draft)*.md`, "notes-#(draft)-1.md"
	before, err := filepath.Match(pat, base)
	require.NoError(t, err)
	d := FilenameDiagnostic([]string{pat}, base, nil, "kind note")
	assert.Equal(t, before, d == nil)
	if runtime.GOOS != "windows" {
		assert.Nil(t, d, "on POSIX `\\#` is an escaped `#`")
	}
}

// A malformed fmvar opener is matched literally; when that makes the
// basename miss, the hint says why instead of leaving the author to
// guess that the reference never resolved.
func TestFilenameDiagnostic_MalformedFmvarHint(t *testing.T) {
	d := FilenameDiagnostic([]string{`\#(fmvar(my-key)).md`}, "x.md",
		map[string]any{"my-key": "x"}, "kind note")
	require.NotNil(t, d)
	assert.Contains(t, d.Hint, "matched literally")
	assert.Contains(t, d.Hint, "must be quoted")
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

// filepath.Match — the only matcher a `filename:` glob feeds — has
// no brace alternatives, so `{` and `,` are already literal there.
// Escaping them buys nothing on POSIX and breaks the match outright
// on Windows, where filepath.Match reads `\` as an ordinary
// character rather than an escape: `a\,b` can never match the
// basename `a,b`. The `filename:` surface must therefore leave both
// bytes alone.
func TestResolveFilenamePatterns_LeavesBraceAndCommaUnescaped(t *testing.T) {
	resolved, interpolated, unresolved := resolveFilenamePatterns(
		[]string{`\#(fmvar(id)).md`}, map[string]any{"id": "a,b{c"})
	require.NoError(t, unresolved)
	assert.Equal(t, []string{"a,b{c.md"}, resolved)
	assert.Equal(t, []string{"a,b{c.md"}, interpolated)
	ok, err := filepath.Match(resolved[0], "a,b{c.md")
	require.NoError(t, err)
	assert.True(t, ok)
}

// The bytes filepath.Match itself treats as special are still
// escaped, so a value carrying one matches literally.
func TestResolveFilenamePatterns_EscapesFilepathMatchMetacharacters(t *testing.T) {
	resolved, _, unresolved := resolveFilenamePatterns(
		[]string{`\#(fmvar(id)).md`}, map[string]any{"id": "a*b"})
	require.NoError(t, unresolved)
	ok, err := filepath.Match(resolved[0], "a*b.md")
	require.NoError(t, err)
	assert.True(t, ok)
	ok, err = filepath.Match(resolved[0], "axxb.md")
	require.NoError(t, err)
	assert.False(t, ok, "the `*` in the frontmatter value must not act as a wildcard")
}

// filepath.Match ignores `\` escapes on Windows, so the `filename:`
// surface must quote `[`, `*`, and `?` with a one-byte character
// class instead. The resolved pattern then carries no backslash and
// means the same thing on every platform.
func TestResolveFilenamePatterns_QuotesMetaWithoutBackslash(t *testing.T) {
	resolved, _, unresolved := resolveFilenamePatterns(
		[]string{`\#(fmvar(id))-*.md`}, map[string]any{"id": "[draft]*?"})
	require.NoError(t, unresolved)
	require.Len(t, resolved, 1)
	assert.NotContains(t, resolved[0], `\`,
		"a backslash escape is inert on Windows")
	ok, err := filepath.Match(resolved[0], "[draft]*?-1.md")
	require.NoError(t, err)
	assert.True(t, ok)
	ok, err = filepath.Match(resolved[0], "d-1.md")
	require.NoError(t, err)
	assert.False(t, ok, "the `[` in the value must not open a class")
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

func TestEscapeMeta_PlainValueDoesNotAllocate(t *testing.T) {
	for name, esc := range map[string]*strings.Replacer{
		"glob":     globMetaEscaper,
		"filename": filenameMetaEscaper,
	} {
		allocs := testing.AllocsPerRun(100, func() { _ = esc.Replace("code-review") })
		assert.Zero(t, allocs, name)
	}
}

// nextGlobOpener steps over `\x` escape pairs, so an escaped
// backslash never starts an opener and a trailing escape pair ends
// the scan.
func TestNextGlobOpener(t *testing.T) {
	assert.Equal(t, 2, nextGlobOpener(`a/\#(fmvar(x))`, 0))
	assert.Equal(t, -1, nextGlobOpener(`a\b`, 0), "an escape pair is not an opener")
	assert.Equal(t, -1, nextGlobOpener(`a\\#(x)`, 0), "`\\\\` escapes the backslash")
	assert.Equal(t, -1, nextGlobOpener("plain", 0))
	assert.Equal(t, -1, nextGlobOpener(`\#(`, 3))
}

func TestGlobRefAt(t *testing.T) {
	name, end, err := globRefAt(`a/\#(fmvar(x)).md`, 2)
	require.NoError(t, err)
	assert.Equal(t, "x", name)
	assert.Equal(t, len(`a/\#(fmvar(x))`), end)

	_, end, err = globRefAt(`\#(draft).md`, 0)
	assert.ErrorIs(t, err, errGlobRefNotFmvar)
	assert.Equal(t, len(`\#(draft)`), end)

	name, _, err = globRefAt(`\#(fmvar(my-key))`, 0)
	assert.ErrorIs(t, err, errGlobRefBadPath)
	assert.Equal(t, "my-key", name)

	_, end, err = globRefAt(`\#(fmvar(x).md`, 0)
	assert.ErrorIs(t, err, errGlobRefUnterminated)
	assert.Equal(t, len(`\#(fmvar(x).md`), end)
}

// GlobMismatchHint ranks an unresolvable reference over a literal
// fmvar-looking opener, and both over the substituted-pattern hint.
func TestGlobMismatchHint(t *testing.T) {
	bad := `docs/\#(fmvar(my-key)).md`
	assert.Equal(t, "boom",
		GlobMismatchHint(errors.New("boom"), []string{bad}, "docs/x.md"))
	assert.Contains(t,
		GlobMismatchHint(nil, []string{"ok.md", bad}, "docs/x.md"),
		"matched literally")
	assert.Equal(t, "with front matter applied: docs/x.md",
		GlobMismatchHint(nil, []string{"ok.md"}, "docs/x.md"))
	assert.Empty(t, GlobMismatchHint(nil, []string{"ok.md"}))
}
