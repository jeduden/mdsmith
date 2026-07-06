package requiredfrontmatter

import (
	"testing"
	"testing/fstest"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fileWith builds a lint.File whose front matter is stripped into
// f.FrontMatter, matching what the engine hands rules in production.
func fileWith(t *testing.T, path, src string) *lint.File {
	t.Helper()
	f, err := lint.NewFileFromSource(path, []byte(src), true)
	require.NoError(t, err)
	return f
}

func TestMetadata(t *testing.T) {
	r := &Rule{}
	assert.Equal(t, "MDS071", r.ID())
	assert.Equal(t, "required-frontmatter", r.Name())
	assert.Equal(t, "structural", r.Category())
	assert.False(t, r.EnabledByDefault(), "rule is opt-in")
}

func TestCheck_NilFileAndInert(t *testing.T) {
	r := &Rule{}
	assert.Nil(t, r.Check(nil))

	// No fields configured: registered but inert.
	f := fileWith(t, "doc.md", "---\ntitle: X\n---\n# Body\n")
	assert.Nil(t, r.Check(f))
}

func TestCheck_PresentNonEmpty(t *testing.T) {
	r := &Rule{Fields: []string{"type"}}
	f := fileWith(t, "doc.md", "---\ntype: BigQuery Table\n---\n# Schema\n")
	assert.Nil(t, r.Check(f))
}

func TestCheck_MissingField(t *testing.T) {
	r := &Rule{Fields: []string{"type"}}
	f := fileWith(t, "doc.md", "---\ntitle: Orders\n---\n# Schema\n")
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Equal(t, "MDS071", diags[0].RuleID)
	assert.Equal(t, "required-frontmatter", diags[0].RuleName)
	assert.Equal(t, `front-matter "type" is required but missing`, diags[0].Message)
	assert.Equal(t, 1, diags[0].Column)
	// Front-matter diagnostics land on file line 1 after the engine
	// adds LineOffset back in AdjustDiagnostics.
	assert.Equal(t, 1-f.LineOffset, diags[0].Line)
}

func TestCheck_NoFrontMatterAtAll(t *testing.T) {
	// A file with no front matter is missing every required field.
	r := &Rule{Fields: []string{"type"}}
	f := fileWith(t, "doc.md", "# Schema\n\nNo front matter here.\n")
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "missing")
}

func TestCheck_EmptyValues(t *testing.T) {
	cases := map[string]string{
		"empty string":    "---\ntype: \"\"\n---\n# B\n",
		"whitespace only": "---\ntype: \"   \"\n---\n# B\n",
		"null":            "---\ntype:\n---\n# B\n",
		"empty list":      "---\ntype: []\n---\n# B\n",
		"empty map":       "---\ntype: {}\n---\n# B\n",
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			r := &Rule{Fields: []string{"type"}}
			diags := r.Check(fileWith(t, "doc.md", src))
			require.Len(t, diags, 1)
			assert.Equal(t, `front-matter "type" is required but empty`, diags[0].Message)
		})
	}
}

func TestCheck_NonStringScalarSatisfies(t *testing.T) {
	// Presence, not type, is what the rule enforces: a number counts.
	r := &Rule{Fields: []string{"weight"}}
	f := fileWith(t, "doc.md", "---\nweight: 3\n---\n# B\n")
	assert.Nil(t, r.Check(f))
}

func TestCheck_MultipleFields(t *testing.T) {
	r := &Rule{Fields: []string{"type", "title"}}
	f := fileWith(t, "doc.md", "---\ndescription: x\n---\n# B\n")
	diags := r.Check(f)
	require.Len(t, diags, 2)
	assert.Equal(t, `front-matter "type" is required but missing`, diags[0].Message)
	assert.Equal(t, `front-matter "title" is required but missing`, diags[1].Message)
}

func TestCheck_IncludeScope(t *testing.T) {
	r := &Rule{Fields: []string{"type"}, Include: []string{"concepts/**"}}
	// Out of scope: no diagnostic even though type is missing.
	out := fileWith(t, "README.md", "# Readme\n")
	assert.Nil(t, r.Check(out))
	// In scope: missing type fires.
	in := fileWith(t, "concepts/orders.md", "---\ntitle: x\n---\n# B\n")
	assert.Len(t, r.Check(in), 1)
}

func TestCheck_ExcludeReservedFiles(t *testing.T) {
	r := &Rule{Fields: []string{"type"}, Exclude: []string{"index.md", "log.md"}}
	for _, p := range []string{"index.md", "concepts/index.md", "log.md", "a/b/log.md"} {
		f := fileWith(t, p, "# Reserved\n")
		assert.Nil(t, r.Check(f), "%s should be excluded", p)
	}
	// A concept doc at the same depth is still checked.
	f := fileWith(t, "concepts/orders.md", "# B\n")
	assert.Len(t, r.Check(f), 1)
}

func TestCheck_FSFallbackReadsOwnFrontMatter(t *testing.T) {
	// When f.FrontMatter is empty (files built via lint.NewFile), the
	// rule reads the file from f.FS so its own front matter is visible.
	src := "---\ntype: Playbook\n---\n# Steps\n"
	f, err := lint.NewFile("play.md", []byte(src))
	require.NoError(t, err)
	f.FS = fstest.MapFS{"play.md": &fstest.MapFile{Data: []byte(src)}}
	r := &Rule{Fields: []string{"type"}}
	assert.Nil(t, r.Check(f))

	// Same file path but the on-disk copy lacks type → flagged.
	bad := "---\ntitle: x\n---\n# Steps\n"
	f2, err := lint.NewFile("play.md", []byte(bad))
	require.NoError(t, err)
	f2.FS = fstest.MapFS{"play.md": &fstest.MapFile{Data: []byte(bad)}}
	assert.Len(t, r.Check(f2), 1)
}

func TestCheck_MalformedFrontMatter(t *testing.T) {
	// Unparseable YAML front matter resolves to no fields, so every
	// required field reads as missing.
	r := &Rule{Fields: []string{"type"}}
	f := fileWith(t, "doc.md", "---\ntype: \"oops\n---\n# B\n")
	diags := r.Check(f)
	require.Len(t, diags, 1)
	assert.Equal(t, `front-matter "type" is required but missing`, diags[0].Message)
}

func TestExtractYAMLBody(t *testing.T) {
	cases := map[string]struct{ in, want string }{
		"newline fence": {"---\ntype: X\n---\n", "type: X\n"},
		"bare fence":    {"---\ntype: X\n---", "type: X\n"},
		"no fence":      {"type: X\n", "type: X\n"},
		"empty":         {"", ""},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, c.want, string(extractYAMLBody([]byte(c.in))))
		})
	}
}

func TestApplySettings_FieldEmptyClears(t *testing.T) {
	// An empty `field` disables the rule rather than requiring "".
	r := &Rule{Fields: []string{"stale"}}
	require.NoError(t, r.ApplySettings(map[string]any{"field": ""}))
	assert.Empty(t, r.Fields)
	assert.Nil(t, r.Check(fileWith(t, "doc.md", "# B\n")))
}

func TestApplySettings_FieldAlias(t *testing.T) {
	r := &Rule{}
	require.NoError(t, r.ApplySettings(map[string]any{"field": "type"}))
	assert.Equal(t, []string{"type"}, r.Fields)
}

func TestApplySettings_FieldsList(t *testing.T) {
	r := &Rule{}
	require.NoError(t, r.ApplySettings(map[string]any{
		"fields":  []any{"type", "title"},
		"include": []any{"**/*.md"},
		"exclude": []any{"index.md", "log.md"},
	}))
	assert.Equal(t, []string{"type", "title"}, r.Fields)
	assert.Equal(t, []string{"**/*.md"}, r.Include)
	assert.Equal(t, []string{"index.md", "log.md"}, r.Exclude)
}

func TestApplySettings_Errors(t *testing.T) {
	t.Run("field and fields together", func(t *testing.T) {
		r := &Rule{}
		err := r.ApplySettings(map[string]any{"field": "type", "fields": []any{"type"}})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not both")
	})
	t.Run("field wrong type", func(t *testing.T) {
		r := &Rule{}
		require.Error(t, r.ApplySettings(map[string]any{"field": 7}))
	})
	t.Run("fields wrong type", func(t *testing.T) {
		r := &Rule{}
		require.Error(t, r.ApplySettings(map[string]any{"fields": "type"}))
	})
	t.Run("unknown setting", func(t *testing.T) {
		r := &Rule{}
		require.Error(t, r.ApplySettings(map[string]any{"bogus": true}))
	})
	t.Run("invalid glob", func(t *testing.T) {
		r := &Rule{}
		require.Error(t, r.ApplySettings(map[string]any{"include": []any{"[bad"}}))
	})
}

func TestDefaultSettings_RoundTrips(t *testing.T) {
	r := &Rule{}
	require.NoError(t, r.ApplySettings(r.DefaultSettings()))
}
