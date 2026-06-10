package uniquefrontmatter

import (
	"testing"
	"testing/fstest"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func file(t *testing.T, path string, fsys fstest.MapFS) *lint.File {
	t.Helper()
	f, err := lint.NewFile(path, []byte("# Title\n"))
	require.NoError(t, err)
	f.FS = fsys
	return f
}

func planFS() fstest.MapFS {
	return fstest.MapFS{
		"plan/a.md":     {Data: []byte("---\nid: 7\ntitle: A\n---\n# A\n")},
		"plan/b.md":     {Data: []byte("---\nid: 7\ntitle: B\n---\n# B\n")},
		"plan/c.md":     {Data: []byte("---\nid: 9\ntitle: C\n---\n# C\n")},
		"plan/notes.md": {Data: []byte("---\ntitle: no id here\n---\n# N\n")},
		"plan/proto.md": {Data: []byte("---\nid: 'int & >=1'\n---\n# P\n")},
		"other/d.md":    {Data: []byte("---\nid: 7\n---\n# D\n")},
	}
}

func planRule() *Rule {
	return &Rule{
		Field:   "id",
		Include: []string{"plan/*.md"},
		Exclude: []string{"plan/proto.md"},
	}
}

func TestDuplicateFlagsLaterPathOnly(t *testing.T) {
	fsys := planFS()
	r := planRule()

	require.Nil(t, r.Check(file(t, "plan/a.md", fsys)),
		"first holder in path order stays clean")

	diags := r.Check(file(t, "plan/b.md", fsys))
	require.Len(t, diags, 1)
	d := diags[0]
	assert.Equal(t, "plan/b.md", d.File)
	assert.Equal(t, 2, d.Line, "diagnostic anchors at the id: line")
	assert.Equal(t, 1, d.Column)
	assert.Equal(t, "MDS069", d.RuleID)
	assert.Equal(t,
		`front-matter "id": value 7 already used by plan/a.md`,
		d.Message)
}

func TestDistinctValuesPass(t *testing.T) {
	fsys := planFS()
	r := planRule()
	assert.Nil(t, r.Check(file(t, "plan/c.md", fsys)))
}

func TestMissingFieldSkips(t *testing.T) {
	fsys := planFS()
	r := planRule()
	assert.Nil(t, r.Check(file(t, "plan/notes.md", fsys)))
}

func TestExcludedAndOutOfScopeSkip(t *testing.T) {
	fsys := planFS()
	r := planRule()
	assert.Nil(t, r.Check(file(t, "plan/proto.md", fsys)),
		"excluded file is never flagged")
	assert.Nil(t, r.Check(file(t, "other/d.md", fsys)),
		"file outside the include globs is never flagged")
}

func TestUnconfiguredIsInert(t *testing.T) {
	fsys := planFS()
	assert.Nil(t, (&Rule{}).Check(file(t, "plan/b.md", fsys)))
	assert.Nil(t, (&Rule{Field: "id"}).Check(file(t, "plan/b.md", fsys)),
		"empty include list disables the rule")
}

func TestIntAndStringValuesCollide(t *testing.T) {
	fsys := fstest.MapFS{
		"plan/a.md": {Data: []byte("---\nid: 7\n---\n# A\n")},
		"plan/b.md": {Data: []byte("---\nid: \"7\"\n---\n# B\n")},
	}
	r := &Rule{Field: "id", Include: []string{"plan/*.md"}}
	diags := r.Check(file(t, "plan/b.md", fsys))
	require.Len(t, diags, 1, "quoted and bare scalars compare by value")
}

func TestThreeWayDuplicateFlagsEachLaterFile(t *testing.T) {
	fsys := fstest.MapFS{
		"plan/a.md": {Data: []byte("---\nid: 7\n---\n# A\n")},
		"plan/b.md": {Data: []byte("---\nid: 7\n---\n# B\n")},
		"plan/c.md": {Data: []byte("---\nid: 7\n---\n# C\n")},
	}
	r := &Rule{Field: "id", Include: []string{"plan/*.md"}}
	assert.Nil(t, r.Check(file(t, "plan/a.md", fsys)))
	for _, p := range []string{"plan/b.md", "plan/c.md"} {
		diags := r.Check(file(t, p, fsys))
		require.Len(t, diags, 1, p)
		assert.Contains(t, diags[0].Message, "plan/a.md",
			"every later file names the first holder")
	}
}

func TestRunCacheSharesOneIndex(t *testing.T) {
	fsys := planFS()
	r := planRule()
	rc := lint.NewRunCache()

	fa := file(t, "plan/a.md", fsys)
	fb := file(t, "plan/b.md", fsys)
	fa.RunCache = rc
	fb.RunCache = rc

	require.Nil(t, r.Check(fa))
	diags := r.Check(fb)
	require.Len(t, diags, 1)

	ia := r.index(fa)
	ib := r.index(fb)
	assert.Same(t, ia, ib, "both hosts read the same cached index")

	rc.Invalidate("plan/a.md")
	assert.NotSame(t, ia, r.index(fa),
		"Invalidate drops the index so the next Check rebuilds")
}

func TestApplySettings(t *testing.T) {
	r := &Rule{}
	err := r.ApplySettings(map[string]any{
		"field":   "id",
		"include": []any{"plan/*.md"},
		"exclude": []any{"plan/proto.md"},
	})
	require.NoError(t, err)
	assert.Equal(t, "id", r.Field)
	assert.Equal(t, []string{"plan/*.md"}, r.Include)
	assert.Equal(t, []string{"plan/proto.md"}, r.Exclude)
}

func TestApplySettingsRejectsBadInput(t *testing.T) {
	assert.Error(t, (&Rule{}).ApplySettings(map[string]any{"field": 7}),
		"non-string field")
	assert.Error(t, (&Rule{}).ApplySettings(map[string]any{"include": "x"}),
		"non-list include")
	assert.Error(t, (&Rule{}).ApplySettings(map[string]any{"nope": true}),
		"unknown key")
	assert.Error(t, (&Rule{}).ApplySettings(map[string]any{
		"field": "id", "include": []any{"plan/[*.md"},
	}), "invalid glob pattern")
}

// TestDiagnosticLineSurvivesLineOffset pins the body-coordinate
// contract: the rule emits e.line - f.LineOffset so the engine's
// AdjustDiagnostics shift lands the diagnostic back on the raw
// file line of the field (the MDS020 pattern).
func TestDiagnosticLineSurvivesLineOffset(t *testing.T) {
	fsys := planFS()
	r := planRule()

	src := []byte("---\nid: 7\ntitle: B\n---\n# B\n")
	f, err := lint.NewFileFromSource("plan/b.md", src, true)
	require.NoError(t, err)
	f.FS = fsys

	diags := r.Check(f)
	require.Len(t, diags, 1)
	f.AdjustDiagnostics(diags)
	assert.Equal(t, 2, diags[0].Line,
		"diagnostic anchors at the raw file line of id:")
}
