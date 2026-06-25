package export

import (
	"testing"
	"testing/fstest"

	"github.com/jeduden/mdsmith/internal/gitignore"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/piparser"
	"github.com/jeduden/mdsmith/internal/rule"
	_ "github.com/jeduden/mdsmith/internal/rules/all"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSelectDirectives(t *testing.T) {
	assert.Nil(t, selectDirectives(nil))
	assert.Nil(t, selectDirectives([]rule.Rule{}))

	got := selectDirectives(rule.All())
	require.NotEmpty(t, got)

	for i := 1; i < len(got); i++ {
		assert.LessOrEqual(t, got[i-1].directive.Name(), got[i].directive.Name())
	}
	for _, d := range got {
		assert.NotNil(t, d.rule)
		assert.NotNil(t, d.directive)
	}
}

func TestAllDirectiveNames(t *testing.T) {
	got := allDirectiveNames()
	require.NotEmpty(t, got)

	for i := 1; i < len(got); i++ {
		assert.LessOrEqual(t, got[i-1].name, got[i].name)
	}
	for _, d := range got {
		assert.NotEmpty(t, d.name)
		assert.NotEmpty(t, d.ruleID)
		assert.NotEmpty(t, d.ruleName)
	}
}

func TestRegenerate(t *testing.T) {
	src := "# Title\n\n<?toc?>\n\n- [Wrong](#wrong)\n\n<?/toc?>\n\n## Section\n\nbody\n"
	orig, err := lint.NewFile("doc.md", []byte(src))
	require.NoError(t, err)

	result := regenerate(orig, selectDirectives(rule.All()))

	require.NotNil(t, result)
	assert.NotEqual(t, src, string(result.Source))
	assert.Contains(t, string(result.Source), "[Section](#section)")
}

func TestHydrate(t *testing.T) {
	orig, err := lint.NewFile("orig.md", []byte("# Hello\n"))
	require.NoError(t, err)
	orig.FS = fstest.MapFS{"a.md": &fstest.MapFile{Data: []byte("a")}}
	orig.RootDir = "/repo"
	orig.MaxInputBytes = int64(999)
	orig.RootFS = fstest.MapFS{"b.md": &fstest.MapFile{Data: []byte("b")}}
	orig.GitignoreFunc = func() *gitignore.Matcher { return nil }

	parsed, err := lint.NewFile("parsed.md", []byte("# World\n"))
	require.NoError(t, err)

	hydrate(parsed, orig)

	assert.Equal(t, "/repo", parsed.RootDir)
	assert.Equal(t, int64(999), parsed.MaxInputBytes)
	assert.Equal(t, orig.FS, parsed.FS)
	assert.Equal(t, orig.RootFS, parsed.RootFS)
	assert.NotNil(t, parsed.GitignoreFunc)
}

func TestCheckStaleness(t *testing.T) {
	directives := selectDirectives(rule.All())

	t.Run("fresh body produces no diagnostics", func(t *testing.T) {
		src := "# Title\n\n<?toc?>\n\n- [Section](#section)\n\n<?/toc?>\n\n## Section\n\nbody\n"
		f, err := lint.NewFile("doc.md", []byte(src))
		require.NoError(t, err)
		assert.Empty(t, checkStaleness(f, directives))
	})

	t.Run("stale body produces error diagnostics", func(t *testing.T) {
		src := "# Title\n\n<?toc?>\n\n- [Wrong](#wrong)\n\n<?/toc?>\n\n## Section\n\nbody\n"
		f, err := lint.NewFile("doc.md", []byte(src))
		require.NoError(t, err)
		diags := checkStaleness(f, directives)
		require.NotEmpty(t, diags)
		assert.Equal(t, lint.Error, diags[0].Severity)
	})
}

func TestInGeneratedRange(t *testing.T) {
	ranges := []lint.LineRange{{From: 3, To: 7}}

	assert.True(t, inGeneratedRange(3, ranges))
	assert.True(t, inGeneratedRange(5, ranges))
	assert.True(t, inGeneratedRange(7, ranges))
	assert.False(t, inGeneratedRange(2, ranges))
	assert.False(t, inGeneratedRange(8, ranges))
	assert.False(t, inGeneratedRange(5, nil))
}

func TestStripDirectives(t *testing.T) {
	t.Run("directive-free file passes through verbatim", func(t *testing.T) {
		src := "# Title\n\nbody\n"
		f, err := lint.NewFile("doc.md", []byte(src))
		require.NoError(t, err)
		got := stripDirectives(f, allDirectiveNames())
		assert.Equal(t, src, string(got))
	})

	t.Run("toc markers removed body kept", func(t *testing.T) {
		src := "# Title\n\n<?toc?>\n\n- [Section](#section)\n\n<?/toc?>\n\n## Section\n\nbody\n"
		f, err := lint.NewFile("doc.md", []byte(src))
		require.NoError(t, err)
		got := stripDirectives(f, allDirectiveNames())
		s := string(got)
		assert.NotContains(t, s, "<?toc")
		assert.Contains(t, s, "- [Section](#section)")
	})

	t.Run("markerless PI removed", func(t *testing.T) {
		src := "# Title\n\n<?allow-empty-section?>\n\nbody\n"
		f, err := lint.NewFile("doc.md", []byte(src))
		require.NoError(t, err)
		got := stripDirectives(f, allDirectiveNames())
		assert.NotContains(t, string(got), "<?allow-empty-section?>")
		assert.Contains(t, string(got), "body")
	})
}

func TestPiLineRange(t *testing.T) {
	t.Run("single-line PI returns same start and end", func(t *testing.T) {
		src := "<?allow-empty-section?>\n"
		f, err := lint.NewFile("doc.md", []byte(src))
		require.NoError(t, err)

		pi := helperFirstPI(t, f)
		start, end := piLineRange(pi, f)
		assert.Equal(t, 1, start)
		assert.Equal(t, 1, end)
	})

	t.Run("multi-line PI closure on later line", func(t *testing.T) {
		src := "<?require\nfilename: \"*.md\"\n?>\n"
		f, err := lint.NewFile("doc.md", []byte(src))
		require.NoError(t, err)

		pi := helperFirstPI(t, f)
		start, end := piLineRange(pi, f)
		assert.Equal(t, 1, start)
		assert.Equal(t, 3, end)
	})
}

// helperFirstPI walks f.AST and returns the first ProcessingInstruction node.
func helperFirstPI(t *testing.T, f *lint.File) *piparser.ProcessingInstruction {
	t.Helper()
	for n := f.AST.FirstChild(); n != nil; n = n.NextSibling() {
		if pi, ok := n.(*piparser.ProcessingInstruction); ok {
			return pi
		}
	}
	t.Fatal("no ProcessingInstruction node found in AST")
	return nil
}

func TestOverlapsAny(t *testing.T) {
	set := map[int]struct{}{2: {}, 5: {}}

	assert.True(t, overlapsAny(1, 3, set))
	assert.True(t, overlapsAny(5, 5, set))
	assert.False(t, overlapsAny(3, 4, set))
	assert.False(t, overlapsAny(6, 8, set))
	// from > to: loop never executes
	assert.False(t, overlapsAny(3, 1, set))
}

func TestEmitLines(t *testing.T) {
	lines := [][]byte{
		[]byte("line one"),
		[]byte("line two"),
		[]byte("line three"),
	}

	t.Run("empty strip set emits all lines", func(t *testing.T) {
		got := emitLines(lines, map[int]struct{}{})
		assert.Equal(t, "line one\nline two\nline three", string(got))
	})

	t.Run("strip middle line", func(t *testing.T) {
		got := emitLines(lines, map[int]struct{}{2: {}})
		assert.Equal(t, "line one\nline three", string(got))
	})

	t.Run("strip first line", func(t *testing.T) {
		got := emitLines(lines, map[int]struct{}{1: {}})
		assert.Equal(t, "line two\nline three", string(got))
	})

	t.Run("strip all lines produces empty", func(t *testing.T) {
		got := emitLines(lines, map[int]struct{}{1: {}, 2: {}, 3: {}})
		assert.Empty(t, string(got))
	})
}

func TestNormalizeBlankLines(t *testing.T) {
	noCode := map[int]struct{}{}

	t.Run("nil input returns nil", func(t *testing.T) {
		assert.Nil(t, normalizeBlankLines(nil, noCode))
	})

	t.Run("empty input returns empty", func(t *testing.T) {
		assert.Empty(t, normalizeBlankLines([]byte{}, noCode))
	})

	t.Run("leading and trailing blanks trimmed", func(t *testing.T) {
		got := normalizeBlankLines([]byte("\n\nparagraph\n\n"), noCode)
		assert.Equal(t, "paragraph\n", string(got))
	})

	t.Run("multiple consecutive blanks collapsed to one", func(t *testing.T) {
		got := normalizeBlankLines([]byte("a\n\n\n\nb\n"), noCode)
		assert.Equal(t, "a\n\nb\n", string(got))
	})

	t.Run("result ends with exactly one newline", func(t *testing.T) {
		got := normalizeBlankLines([]byte("a\nb\n"), noCode)
		assert.Equal(t, "a\nb\n", string(got))
	})

	t.Run("blank lines inside code block preserved", func(t *testing.T) {
		// Lines 2 and 3 are blank lines inside a code block; normaliseBlankLines
		// treats inCode blank lines as non-blank so they survive collapse.
		codeLines := map[int]struct{}{2: {}, 3: {}}
		src := []byte("text\n\n\ncode blank\ntext2\n")
		got := normalizeBlankLines(src, codeLines)
		assert.Equal(t, "text\n\n\ncode blank\ntext2\n", string(got))
	})

	t.Run("all blank content normalises to nil", func(t *testing.T) {
		got := normalizeBlankLines([]byte("\n\n\n"), noCode)
		assert.Nil(t, got)
	})
}
