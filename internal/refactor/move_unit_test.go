package refactor

import (
	"bytes"
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/pkg/goldmark/ast"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMarkupEscaped(t *testing.T) {
	for name, tc := range map[string]struct {
		tok  string
		more bool
		want bool
	}{
		"plain":                       {"docs/a.md", false, false},
		"windows separator":           {`sub\a.md`, false, false},
		"escaped punctuation":         {`a\_b.md`, false, true},
		"escaped paren":               {`a\).md`, false, true},
		"named entity":                {"a&amp;b.md", false, true},
		"numeric entity":              {"a&#95;b.md", false, true},
		"unknown entity name":         {"a&nope;b.md", false, false},
		"bare ampersand":              {"a&b.md", false, false},
		"trailing backslash, no more": {`a\`, false, false},
		"escapes the `#` after it":    {`a\`, true, true},
		"opens `&#35;` after it":      {"a&", true, true},
		"empty token before a query":  {"", true, false},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, markupEscaped([]byte(tc.tok), tc.more))
		})
	}
}

func TestRelFrom(t *testing.T) {
	for name, tc := range map[string]struct{ from, target, want string }{
		"down":            {".", "docs/a.md", "docs/a.md"},
		"up":              {"docs", "b.md", "../b.md"},
		"same directory":  {"docs", "docs", "."},
		"up to the root":  {"docs", ".", ".."},
		"across branches": {"a/b", "c/d.md", "../../c/d.md"},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, relFrom(tc.from, tc.target))
		})
	}
}

func TestLiteralTarget(t *testing.T) {
	for name, tc := range map[string]struct{ refFile, pre, lit, target string }{
		"literal `?`":        {"a.md", "what?.md", "what?.md", "what?.md"},
		"decoded, nested":    {"x/a.md", "d/what%3F.md", "d/what?.md", "x/d/what?.md"},
		"out of workspace":   {"a.md", "../x?.md", "../x?.md", ""},
		"does not decode":    {"a.md", "bad%zz?.md", "", ""},
		"decoded query byte": {"a.md", "q%3Fx.md?y", "q?x.md?y", "q?x.md?y"},
	} {
		t.Run(name, func(t *testing.T) {
			lit, target := literalTarget(tc.refFile, []byte(tc.pre))
			assert.Equal(t, tc.lit, lit)
			assert.Equal(t, tc.target, target)
		})
	}
}

func TestDestResolver_Exists(t *testing.T) {
	r := &destResolver{ws: stubWorkspace{files: []string{"b.md", "./sub/c.md"}}, src: "a.md"}
	assert.True(t, r.exists("a.md"), "the moved file")
	assert.Nil(t, r.files, "naming src lists no files")
	assert.True(t, r.exists("b.md"))
	assert.True(t, r.exists("sub/c.md"), "listed paths are normalized")
	assert.False(t, r.exists("z.md"))
	assert.False(t, r.exists("sub"), "a directory is not a listed file")
}

func TestDestEdit(t *testing.T) {
	type tc struct {
		row, tok          string
		angle             bool
		ref               destRef
		spellFrom, target string
		want              string
	}
	ref := func(p string) destRef { return destRef{path: p, tokLen: len(p)} }
	for name, c := range map[string]tc{
		"bare relative":          {"[x](b.md)", "b.md", false, ref("b.md"), "docs/a.md", "b.md", "../b.md"},
		"keeps `./`":             {"[x](./b.md)", "./b.md", false, ref("./b.md"), "a.md", "x/b.md", "./x/b.md"},
		"drops `./` to climb":    {"[x](./b.md)", "./b.md", false, ref("./b.md"), "docs/a.md", "b.md", "../b.md"},
		"colon in first segment": {"[x](x.md)", "x.md", false, ref("x.md"), "d/c.md", "d/a:b.md", "./a:b.md"},
		"directory keeps `/`": {
			"[d](sub/)", "sub/", false, destRef{path: "sub/", tokLen: 4, dir: true}, "docs/a.md", "sub", "../sub/",
		},
		"escaped like the old token": {
			"[m](my%20file.md)", "my%20file.md", false,
			destRef{path: "my file.md", tokLen: 12}, "docs/a.md", "my file.md", "../my%20file.md",
		},
		"angle keeps a space": {
			"[m](<my file.md>)", "my file.md", true, ref("my file.md"), "docs/a.md", "my file.md", "../my file.md",
		},
	} {
		t.Run(name, func(t *testing.T) {
			ps := bytes.Index([]byte(c.row), []byte(c.tok))
			d := inlineDest{dest: []byte(c.tok), row: []byte(c.row), line: 2, ps: ps, angle: c.angle}
			e, ok := destEdit(d, c.ref, c.spellFrom, c.target)
			require.True(t, ok)
			assert.Equal(t, c.want, e.NewText)
			assert.Equal(t, Position{Line: 2, Character: ps}, e.Range.Start)
			assert.Equal(t, Position{Line: 2, Character: ps + c.ref.tokLen}, e.Range.End)
		})
	}
	t.Run("a path that still resolves is kept", func(t *testing.T) {
		d := inlineDest{dest: []byte("sub/../b.md"), row: []byte("[s](sub/../b.md)"), ps: 4}
		_, ok := destEdit(d, ref("sub/../b.md"), "a2.md", "b.md")
		assert.False(t, ok)
	})
	t.Run("the range counts UTF-16 units and stops at the fragment", func(t *testing.T) {
		row := "[é😀](b.md#f)"
		d := inlineDest{dest: []byte("b.md#f"), row: []byte(row), ps: bytes.Index([]byte(row), []byte("b.md"))}
		e, ok := destEdit(d, ref("b.md"), "docs/a.md", "b.md")
		require.True(t, ok)
		assert.Equal(t, Range{Start: Position{Character: 6}, End: Position{Character: 10}}, e.Range)
	})
}

func TestLocateDests(t *testing.T) {
	source := []byte("---\nk: v\n---\n[a](b.md) ![i](<c d.png>)\n\n[r]: e.md\n")
	type found struct {
		dest  string
		line  int
		ps    int
		angle bool
	}
	dests := locateDests(lint.NewParser(), "a.md", source)
	got := make([]found, 0, len(dests))
	for _, d := range dests {
		assert.Equal(t, d.dest, d.row[d.ps:d.ps+len(d.dest)], "row bytes at ps")
		got = append(got, found{string(d.dest), d.line, d.ps, d.angle})
	}
	assert.Equal(t, []found{
		{"b.md", 3, 4, false},
		{"c d.png", 3, 16, true},
		{"e.md", 5, 5, false},
	}, got)
	assert.Empty(t, locateDests(lint.NewParser(), "a.md", []byte("no links\n")))
}

// newDestLocator parses body into a locator whose file rows are the
// body's own rows.
func newDestLocator(t *testing.T, body string) *destLocator {
	t.Helper()
	lf, err := lint.NewFile("a.md", []byte(body))
	require.NoError(t, err)
	return &destLocator{lf: lf, fileLines: splitLines(lf.Source)}
}

func TestDestLocator_Locate(t *testing.T) {
	d := newDestLocator(t, "[a](b.md \"t](x)\") [c](<d e.md>) tail\n")
	d.locate([]byte("b.md"))
	require.Len(t, d.dests, 1)
	assert.Equal(t, 4, d.dests[0].ps)
	assert.False(t, d.dests[0].angle)
	assert.Equal(t, len(`[a](b.md "t](x)"`), d.cursor, "the cursor passes the title")

	d.locate([]byte("d e.md"))
	require.Len(t, d.dests, 2)
	assert.Equal(t, len(`[a](b.md "t](x)") [c](<`), d.dests[1].ps)
	assert.True(t, d.dests[1].angle)
	assert.Equal(t, len(`[a](b.md "t](x)") [c](<d e.md>`), d.cursor, "the cursor passes the `>`")
}

// refDefNode returns the first reference definition parsed from lf.
func refDefNode(t *testing.T, lf *lint.File) *ast.LinkReferenceDefinition {
	t.Helper()
	var n *ast.LinkReferenceDefinition
	_ = ast.Walk(lf.AST, func(x ast.Node, entering bool) (ast.WalkStatus, error) {
		if r, ok := x.(*ast.LinkReferenceDefinition); ok && entering && n == nil {
			n = r
		}
		return ast.WalkContinue, nil
	})
	require.NotNil(t, n)
	return n
}

func TestDestLocator_LocateRefDef(t *testing.T) {
	d := newDestLocator(t, "[multi\nline]: <a b.md> \"t\"\n")
	d.locateRefDef(refDefNode(t, d.lf))
	require.Len(t, d.dests, 1)
	got := d.dests[0]
	assert.Equal(t, "a b.md", string(got.dest))
	assert.Equal(t, 1, got.line, "the destination follows the label's last row")
	assert.Equal(t, len("line]: <"), got.ps)
	assert.True(t, got.angle)
}

func TestDestLocator_Record(t *testing.T) {
	source := []byte("---\nk: v\n---\nx\n[a](b.md)\n")
	body, fmOffset := bodyAndFMOffset(source)
	lf, err := lint.NewFile("a.md", body)
	require.NoError(t, err)
	d := destLocator{lf: lf, fileLines: splitLines(source), fmOffset: fmOffset}
	d.record(bytes.Index(body, []byte("b.md")), []byte("b.md"), true)
	require.Len(t, d.dests, 1)
	assert.Equal(t, inlineDest{
		dest: []byte("b.md"), row: []byte("[a](b.md)"), line: 4, ps: 4, angle: true,
	}, d.dests[0])
}
