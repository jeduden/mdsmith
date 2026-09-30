package requiredstructure

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jeduden/mdsmith/internal/schema"
)

// The APM skill contract: `.apm/skills/<name>/SKILL.md` requires the
// `name` frontmatter field to equal the directory name. A static glob
// cannot express that, so `path-pattern:` resolves
// `\#(fmvar(name))` against the document's own front matter first.

const apmSkillPattern = `.apm/skills/\#(fmvar(name))/SKILL.md`

func TestCheck_PathPatternFmvar_MatchesDirectory(t *testing.T) {
	root := t.TempDir()
	f := newRootedFile(t, root, ".apm/skills/code-review/SKILL.md",
		"---\nname: code-review\n---\n# Code review\n")
	r := &Rule{PathPatterns: []PathPattern{
		newPathPattern("apm-skill", apmSkillPattern),
	}}
	expectDiags(t, r.Check(f), 0)
}

func TestCheck_PathPatternFmvar_MismatchedDirectory(t *testing.T) {
	root := t.TempDir()
	f := newRootedFile(t, root, ".apm/skills/code-review/SKILL.md",
		"---\nname: reviewer\n---\n# Code review\n")
	r := &Rule{PathPatterns: []PathPattern{
		newPathPattern("apm-skill", apmSkillPattern),
	}}
	diags := r.Check(f)
	expectDiags(t, diags, 1)
	assert.Contains(t, diags[0].Message,
		`path: got ".apm/skills/code-review/SKILL.md"`)
	assert.Contains(t, diags[0].Message,
		"with front matter applied: .apm/skills/reviewer/SKILL.md")
}

func TestCheck_PathPatternFmvar_MissingFieldReportsClearly(t *testing.T) {
	root := t.TempDir()
	f := newRootedFile(t, root, ".apm/skills/code-review/SKILL.md",
		"# Code review\n")
	r := &Rule{PathPatterns: []PathPattern{
		newPathPattern("apm-skill", apmSkillPattern),
	}}
	diags := r.Check(f)
	expectDiags(t, diags, 1)
	assert.Contains(t, diags[0].Message, "fmvar(name)")
	assert.Contains(t, diags[0].Message, "frontmatter value missing")
	assert.Contains(t, diags[0].Message, apmSkillPattern,
		"the unresolved pattern still names the constraint")
}

// A kind that declares only `path-pattern:` has no schema path in
// Check to report a front-matter parse failure, so the path-pattern
// diagnostic itself must name it. Reporting "frontmatter value
// missing" for a `name:` that is present but unparseable sends the
// author looking for a field they already wrote.
func TestCheck_PathPatternFmvar_UnparseableFrontMatterNamesParseError(t *testing.T) {
	root := t.TempDir()
	f := newRootedFile(t, root, ".apm/skills/code-review/SKILL.md",
		"---\nname: [code-review\n---\n# Code review\n")
	r := &Rule{PathPatterns: []PathPattern{
		newPathPattern("apm-skill", apmSkillPattern),
	}}
	diags := r.Check(f)
	expectDiags(t, diags, 1)
	assert.Contains(t, diags[0].Message, "front matter: invalid YAML")
	assert.NotContains(t, diags[0].Message, "frontmatter value missing")
}

// The parse error stands in for the "missing" report it causes, but
// it must not drop the rest of the hint: a malformed opener elsewhere
// in the same pattern is matched literally whatever the front matter
// holds, and the author needs to hear about both, joined by "; ".
func TestCheck_PathPatternFmvar_ParseErrorKeepsLiteralOpenerHint(t *testing.T) {
	root := t.TempDir()
	f := newRootedFile(t, root, "docs/a/b/x.md",
		"---\nname: [b\n---\n# X\n")
	r := &Rule{PathPatterns: []PathPattern{
		newPathPattern("doc", `docs/\#(fmvar(my-key))/\#(fmvar(name))/x.md`),
	}}
	diags := r.Check(f)
	expectDiags(t, diags, 1)
	msg := diags[0].Message
	assert.Contains(t, msg, "(front matter: invalid YAML")
	assert.Contains(t, msg, "; `\\#(fmvar(my-key))` is matched literally")
	assert.NotContains(t, msg, "frontmatter value missing")
}

// A frontmatter value carrying a glob metacharacter must match
// literally, not act as a wildcard.
func TestCheck_PathPatternFmvar_EscapesValueMetacharacters(t *testing.T) {
	root := t.TempDir()
	f := newRootedFile(t, root, ".apm/skills/axb/SKILL.md",
		"---\nname: \"a*b\"\n---\n# A\n")
	r := &Rule{PathPatterns: []PathPattern{
		newPathPattern("apm-skill", apmSkillPattern),
	}}
	expectDiags(t, r.Check(f), 1)
}

// A `/` in the frontmatter value must not let one reference span two
// directories: `.apm/skills/a/b/SKILL.md` with `name: a/b` names the
// directory `b`, so the APM contract is violated and the kind must
// say so rather than silently accepting the file.
func TestCheck_PathPatternFmvar_ValueWithSeparatorDoesNotSpanDirs(t *testing.T) {
	root := t.TempDir()
	f := newRootedFile(t, root, ".apm/skills/a/b/SKILL.md",
		"---\nname: a/b\n---\n# A\n")
	r := &Rule{PathPatterns: []PathPattern{
		newPathPattern("apm-skill", apmSkillPattern),
	}}
	diags := r.Check(f)
	expectDiags(t, diags, 1)
	assert.Contains(t, diags[0].Message, "path separator")
}

// A reference inside a character class is unsupported: a value of `!`
// leaves `[!]`, which doublestar rejects as a syntax error. The
// diagnostic must say the resolved glob is invalid, as the
// `filename:` surface does, rather than report a plain mismatch.
func TestCheck_PathPatternFmvar_InvalidResolvedGlobIsNamed(t *testing.T) {
	root := t.TempDir()
	f := newRootedFile(t, root, "docs/x/a.md",
		"---\ntag: \"!\"\n---\n# A\n")
	r := &Rule{PathPatterns: []PathPattern{
		newPathPattern("doc", `docs/[\#(fmvar(tag))]/a.md`),
	}}
	diags := r.Check(f)
	expectDiags(t, diags, 1)
	assert.Contains(t, diags[0].Message,
		"with front matter applied: docs/[!]/a.md; "+
			"not a valid glob: syntax error in pattern")
}

func TestCheck_PathPatternFmvar_NestedFieldPath(t *testing.T) {
	root := t.TempDir()
	f := newRootedFile(t, root, "docs/install.md",
		"---\nmeta:\n  slug: install\n---\n# Install\n")
	r := &Rule{PathPatterns: []PathPattern{
		newPathPattern("doc", `docs/\#(fmvar(meta.slug)).md`),
	}}
	expectDiags(t, r.Check(f), 0)
}

// A path-pattern that loaded before fmvar interpolation existed must
// still load: any `\#(` that is not a well-formed fmvar reference is
// an escaped `#` followed by `(`, as it always was.
func TestParsePathPatterns_LiteralOpenerLoads(t *testing.T) {
	for _, pat := range []string{
		`notes/\#(draft)*.md`,
		`docs/\#(fmvar(my-key)).md`,
		`step-\#(digits).md`,
	} {
		pp, err := parsePathPatterns([]any{
			map[string]any{"kind": "doc", "pattern": pat},
		})
		require.NoError(t, err, pat)
		require.Len(t, pp, 1)
		assert.Equal(t, pat, pp[0].Pattern)
	}
}

// A quoted CUE key may hold glob metacharacters. They sit inside the
// reference, not in the glob, so they must not fail the syntax check.
func TestParsePathPatterns_QuotedKeyWithBracketLoads(t *testing.T) {
	_, err := parsePathPatterns([]any{
		map[string]any{"kind": "doc", "pattern": `sub/\#(fmvar("a[b"))/x.md`},
	})
	require.NoError(t, err)
}

func TestCheck_PathPatternLiteralOpenerMatchesAsBefore(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("filepath.ToSlash turns the `\\` of a literal `\\#` into a separator on Windows, as it always did")
	}
	root := t.TempDir()
	f := newRootedFile(t, root, "notes/#(draft)-1.md", "# Draft\n")
	r := &Rule{PathPatterns: []PathPattern{
		newPathPattern("note", `notes/\#(draft)*.md`),
	}}
	expectDiags(t, r.Check(f), 0)
}

// A malformed fmvar opener matches literally, so the file misses the
// kind's path-pattern; the diagnostic says why.
func TestCheck_PathPatternMalformedFmvarHint(t *testing.T) {
	root := t.TempDir()
	f := newRootedFile(t, root, "docs/x.md", "---\nmy-key: x\n---\n# X\n")
	r := &Rule{PathPatterns: []PathPattern{
		newPathPattern("doc", `docs/\#(fmvar(my-key)).md`),
	}}
	diags := r.Check(f)
	expectDiags(t, diags, 1)
	assert.Contains(t, diags[0].Message, "matched literally")
	assert.Contains(t, diags[0].Message, "must be quoted")
}

func TestParsePathPatterns_AcceptsWellFormedInterp(t *testing.T) {
	pp, err := parsePathPatterns([]any{
		map[string]any{"kind": "apm-skill", "pattern": apmSkillPattern},
	})
	require.NoError(t, err)
	require.Len(t, pp, 1)
	assert.Equal(t, apmSkillPattern, pp[0].Pattern)
}

// ---- schema `filename:` through the legacy proto.md path ----

func TestCheck_FileSchemaFilenameFmvar_MatchesFrontmatterValue(t *testing.T) {
	root := t.TempDir()
	writeProtoAt(t, root, "proto.md",
		"<?require\nfilename: '\\#(fmvar(id))-notes.md'\n?>\n# ?\n")
	f := newRootedFile(t, root, "rfc-7-notes.md",
		"---\nid: rfc-7\n---\n# Notes\n")
	r := &Rule{Schema: "proto.md", Sources: []SchemaSource{{File: "proto.md"}}}
	expectDiags(t, r.Check(f), 0)
}

func TestCheck_FileSchemaFilenameFmvar_Mismatch(t *testing.T) {
	root := t.TempDir()
	writeProtoAt(t, root, "proto.md",
		"<?require\nfilename: '\\#(fmvar(id))-notes.md'\n?>\n# ?\n")
	f := newRootedFile(t, root, "rfc-8-notes.md",
		"---\nid: rfc-7\n---\n# Notes\n")
	r := &Rule{Schema: "proto.md", Sources: []SchemaSource{{File: "proto.md"}}}
	diags := r.Check(f)
	expectDiags(t, diags, 1)
	assert.Contains(t, diags[0].Message, `filename: got "rfc-8-notes.md"`)
	assert.Contains(t, diags[0].Message,
		"with front matter applied: rfc-7-notes.md")
}

func TestCheck_FileSchemaFilenameFmvar_MissingField(t *testing.T) {
	root := t.TempDir()
	writeProtoAt(t, root, "proto.md",
		"<?require\nfilename: '\\#(fmvar(id))-notes.md'\n?>\n# ?\n")
	f := newRootedFile(t, root, "rfc-7-notes.md", "# Notes\n")
	r := &Rule{Schema: "proto.md", Sources: []SchemaSource{{File: "proto.md"}}}
	diags := r.Check(f)
	expectDiags(t, diags, 1)
	assert.Contains(t, diags[0].Message, "frontmatter value missing")
}

// writeProtoAt writes a schema file inside an existing workspace root
// so the rule resolves it through the same RootFS the document uses.
func writeProtoAt(t *testing.T, root, name, content string) {
	t.Helper()
	abs := filepath.Join(root, name)
	require.NoError(t, os.MkdirAll(filepath.Dir(abs), 0o755))
	require.NoError(t, os.WriteFile(abs, []byte(content), 0o644))
}

// An interpolating pattern must be matched on its raw text: on
// Windows filepath.ToSlash rewrites every `\`, including the one
// that opens `\#(fmvar(...))`, which would silently demote the
// pattern to a literal glob that no file can satisfy.
func TestCheck_PathPatternFmvar_RawPatternDrivesInterpDetection(t *testing.T) {
	root := t.TempDir()
	f := newRootedFile(t, root, ".apm/skills/code-review/SKILL.md",
		"---\nname: code-review\n---\n# Code review\n")
	r := &Rule{PathPatterns: []PathPattern{
		newPathPattern("apm-skill", apmSkillPattern),
	}}
	require.True(t, schema.PatternHasInterp(r.PathPatterns[0].Pattern),
		"the raw pattern is what PatternHasInterp must see")
	expectDiags(t, r.Check(f), 0)
}

// A `filename:` OR list in a proto.md keeps its OR semantics when
// one entry's `\#(fmvar(...))` reference cannot resolve.
func TestCheck_FileSchemaFilenameFmvar_UnresolvableEntryKeepsOR(t *testing.T) {
	root := t.TempDir()
	writeProtoAt(t, root, "proto.md",
		"<?require\nfilename:\n  - README.md\n  - '\\#(fmvar(id))-notes.md'\n?>\n# ?\n")
	f := newRootedFile(t, root, "README.md", "# Notes\n")
	r := &Rule{Schema: "proto.md", Sources: []SchemaSource{{File: "proto.md"}}}
	expectDiags(t, r.Check(f), 0)
}

// The mismatch hint shows the front-matter value as the author wrote
// it, not the backslash-escaped form doublestar matched (`a\*b`).
func TestCheck_PathPatternFmvar_HintShowsValueAsWritten(t *testing.T) {
	root := t.TempDir()
	f := newRootedFile(t, root, "docs/c/x.md", "---\nname: a*b\n---\n# X\n")
	r := &Rule{PathPatterns: []PathPattern{
		newPathPattern("doc", `docs/\#(fmvar(name))/x.md`),
	}}
	diags := r.Check(f)
	expectDiags(t, diags, 1)
	assert.Contains(t, diags[0].Message,
		"(with front matter applied: docs/a*b/x.md)")
}

// Whether a pattern interpolates, and the form it is matched in,
// depend only on the pattern text, so parsePathPatterns records both
// once instead of Check rescanning every pattern for every file.
func TestParsePathPatterns_RecordsMatchFormOnce(t *testing.T) {
	pp, err := parsePathPatterns([]any{
		map[string]any{"kind": "apm-skill", "pattern": apmSkillPattern},
		map[string]any{"kind": "plan", "pattern": "plan/[0-9]*.md"},
	})
	require.NoError(t, err)
	require.Len(t, pp, 2)
	assert.Equal(t, newPathPattern("apm-skill", apmSkillPattern), pp[0])
	assert.True(t, pp[0].interp)
	assert.Equal(t, apmSkillPattern, pp[0].match)
	assert.False(t, pp[1].interp)
	assert.Equal(t, "plan/[0-9]*.md", pp[1].match)
}

// newPathPattern keeps the pattern as written and derives, once, the
// form it is matched in and whether that form interpolates.
func TestNewPathPattern(t *testing.T) {
	pp := newPathPattern("doc", "docs/*.md")
	assert.Equal(t, "doc", pp.Kind)
	assert.Equal(t, "docs/*.md", pp.Pattern)
	assert.Equal(t, "docs/*.md", pp.match)
	assert.False(t, pp.interp)

	pp = newPathPattern("skill", apmSkillPattern)
	assert.Equal(t, apmSkillPattern, pp.Pattern)
	wantMatch, wantInterp := schema.PathPatternMatchForm(apmSkillPattern)
	assert.Equal(t, wantMatch, pp.match)
	assert.Equal(t, wantInterp, pp.interp)
	assert.True(t, pp.interp)
}

// matchWorkspacePath anchors a plain glob at the workspace root, and
// a brace pattern goes through the validating matcher, which reports
// a syntax error as a non-match.
func TestMatchWorkspacePath(t *testing.T) {
	assert.True(t, matchWorkspacePath("docs/**/*.md", "docs/a/b.md"))
	assert.False(t, matchWorkspacePath("README.md", "docs/README.md"),
		"root-anchored: the basename alone must not match")
	assert.True(t, matchWorkspacePath("{docs,notes}/*.md", "notes/a.md"))
	assert.False(t, matchWorkspacePath("{docs,notes}/*.md", "plan/a.md"))
	assert.False(t, matchWorkspacePath("{[!,docs}/*.md", "docs/a.md"),
		"a syntax error in a brace pattern is a non-match")
}

// pathPatternDiag names the path, the pattern as written, the kind,
// and the hint when one is given.
func TestPathPatternDiag(t *testing.T) {
	f := newTestFile(t, "doc.md", "# X\n")
	pp := newPathPattern("doc", `docs/\#(fmvar(name))/x.md`)
	d := pathPatternDiag(f, "notes/x.md", pp, "some hint")
	assert.Equal(t, "MDS020", d.RuleID)
	assert.Equal(t, f.Path, d.File)
	assert.Contains(t, d.Message, `path: got "notes/x.md"`)
	assert.Contains(t, d.Message,
		`expected path matching glob docs/\#(fmvar(name))/x.md`)
	assert.Contains(t, d.Message, "(some hint)")
	require.Len(t, d.RelatedLocations, 1)
	assert.Equal(t, "kinds[doc] / path-pattern", d.RelatedLocations[0].Message)

	d = pathPatternDiag(f, "notes/x.md", newPathPattern("doc", "docs/*.md"), "")
	assert.NotContains(t, d.Message, "()")
}
