package cuelite_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jeduden/mdsmith/cue/cuelite"
)

// TestParsePath_accepted covers inputs ParsePath accepts and the
// segments it produces.
func TestParsePath_accepted(t *testing.T) {
	cases := []struct {
		name string
		expr string
		want []string
	}{
		{"simple ident", "title", []string{"title"}},
		{"dotted idents", "a.b.c", []string{"a", "b", "c"}},
		{"ident with digits", "abc123", []string{"abc123"}},
		{"ident with underscore continuation", "a_b", []string{"a_b"}},
		{"trailing underscore continuation", "a__b", []string{"a__b"}},
		{"dollar-prefixed ident", "$foo", []string{"$foo"}},
		{"dollar in continuation", "a$b", []string{"a$b"}},
		{"bare dollar", "$", []string{"$"}},
		{"unicode letter ident", "über", []string{"über"}},
		{"cjk ident", "日本語", []string{"日本語"}},
		{"unicode dotted", "héllo.x", []string{"héllo", "x"}},
		{"non-literal keyword if", "if", []string{"if"}},
		{"non-literal keyword for", "for", []string{"for"}},
		{"non-literal keyword let", "let", []string{"let"}},
		{"non-literal keyword in", "in", []string{"in"}},
		{"keyword as later selector", "x.if", []string{"x", "if"}},
		{"true as later selector", "x.true", []string{"x", "true"}},
		{"null as later selector", "x.null", []string{"x", "null"}},
		{"false as later selector", "x.false", []string{"x", "false"}},
		{"single quoted key", `"my-key"`, []string{"my-key"}},
		{"quoted key then ident", `"my-key".sub`, []string{"my-key", "sub"}},
		{"ident then quoted key", `params."a.b"`, []string{"params", "a.b"}},
		{"quoted key with dot inside", `"a.b"`, []string{"a.b"}},
		{"quoted key with escaped quote", `"a\"b"`, []string{`a"b`}},
		{"quoted key with slash escape", `"a\/b"`, []string{"a/b"}},
		{"quoted key with control escapes", `"a\tb"`, []string{"a\tb"}},
		{"quoted key with bell escape", `"a\ab"`, []string{"a\ab"}},
		{"quoted key with backspace escape", `"a\bb"`, []string{"a\bb"}},
		{"quoted key with formfeed escape", `"a\fb"`, []string{"a\fb"}},
		{"quoted key with vtab escape", `"a\vb"`, []string{"a\vb"}},
		{"quoted key with newline escape", `"a\nb"`, []string{"a\nb"}},
		{"quoted key with cr escape", `"a\rb"`, []string{"a\rb"}},
		{"quoted key with backslash escape", `"a\\b"`, []string{`a\b`}},
		{"quoted key with lower-hex unicode escape", `"\u00ff"`, []string{"ÿ"}},
		{"quoted key with upper-hex unicode escape", `"\u00FF"`, []string{"ÿ"}},
		{"quoted key with unicode escape", `"A"`, []string{"A"}},
		{"quoted key with big unicode escape", `"\U0001F600"`, []string{"😀"}},
		{"numeric-looking quoted segment", `"123"`, []string{"123"}},
		{"quoted key with unicode", `"über"`, []string{"über"}},
		{"quoted key with space", `"a b"`, []string{"a b"}},
		{"leading whitespace", " a", []string{"a"}},
		{"trailing whitespace", "a ", []string{"a"}},
		{"space before dot", "a .b", []string{"a", "b"}},
		{"space after quoted before dot", `"a" .b`, []string{"a", "b"}},
		{"space after dot", `"a". "b"`, []string{"a", "b"}},
		{"spaces around dotted", " a.b ", []string{"a", "b"}},
		{"tab whitespace", "\ta", []string{"a"}},
		{"unicode-digit continuation", "a٠", []string{"a٠"}},
		{"trailing newline", "a\n", []string{"a"}},
		{"leading newline", "\na", []string{"a"}},
		{"newline after dot", "a.\nb", []string{"a", "b"}},
		{"trailing CRLF", "a\r\n", []string{"a"}},
		{"trailing line comment", "a//comment", []string{"a"}},
		{"line comment after dot", "a.//c\nb", []string{"a", "b"}},
		{"line comment then trailing newline", "a//c\n", []string{"a"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, err := cuelite.ParsePath(tc.expr)
			require.NoError(t, err)
			assert.Equal(t, tc.want, p.Segments())
		})
	}
}

// TestParsePath_rejected covers inputs ParsePath rejects. ParsePath
// returns a plain error (not a *PathError): a syntax error in a path
// EXPRESSION has no data-tree field path to tag.
func TestParsePath_rejected(t *testing.T) {
	cases := []struct {
		name string
		expr string
	}{
		{"empty string", ""},
		{"whitespace only", " "},
		{"trailing dot", "a."},
		{"leading dot", ".a"},
		{"quoted trailing dot", `"a".`},
		{"empty quoted segment", `""`},
		{"malformed quoted segment", `"a"b`},
		{"unterminated quoted segment", `"unterminated`},
		{"lone-surrogate escape", `"\ud800"`},
		{"go-only hex escape", `"\x41"`},
		{"go-only octal escape", `"\101"`},
		{"unknown escape", `"\z"`},
		{"trailing backslash escape", `"a\`},
		{"truncated unicode escape", `"\u12"`},
		{"invalid hex digit in unicode escape", `"\uZZZZ"`},
		{"raw NUL in quotes", "\"a\x00b\""},
		{"literal true as head", "true"},
		{"literal false as head", "false"},
		{"literal null as head", "null"},
		{"underscore-prefixed ident", "_foo"},
		{"bare underscore", "_"},
		{"hash-prefixed ident", "#foo"},
		{"bare numeric ident", "123"},
		{"bare zero", "0"},
		{"index selector", "a[0]"},
		{"digit-leading ident", "9a"},
		{"whitespace mid ident", "a b"},
		{"triple-dot tail", "a..."},
		{"double-dot", "a..b"},
		{"invalid utf8 in ident", "a\xfcb"},
		{"invalid utf8 in quotes", "\"a\xfcb\""},
		{"newline between idents", "a\nb"},
		{"newline before dot", "a\n.b"},
		{"comment before content", "a//c\n.b"},
		{"comment-only expression", "//c"},
		{"truncated big unicode escape", `"\U0001"`},
		{"raw newline in quotes", "\"a\nb\""},
		{"raw CR in quotes", "\"a\rb\""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := cuelite.ParsePath(tc.expr)
			require.Error(t, err)
			// ParsePath returns a plain error, never a *PathError.
			var pe *cuelite.PathError
			assert.NotErrorAs(t, err, &pe)
		})
	}
}

// TestParsePath_rejectedMessages pins that a rejected non-string selector
// names its kind, so a caller sees a clear contract error rather than a
// bare unexpected-character message.
func TestParsePath_rejectedMessages(t *testing.T) {
	t.Run("index selector names index", func(t *testing.T) {
		_, err := cuelite.ParsePath("123")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "index")
	})
	t.Run("definition selector names definition", func(t *testing.T) {
		_, err := cuelite.ParsePath("#foo")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "definition")
	})
	t.Run("hidden selector names hidden", func(t *testing.T) {
		_, err := cuelite.ParsePath("_foo")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "hidden")
	})
	t.Run("bracket index selector names index", func(t *testing.T) {
		_, err := cuelite.ParsePath("a[0]")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "index")
	})
	t.Run("literal head names CUE literal", func(t *testing.T) {
		_, err := cuelite.ParsePath("true")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "literal")
	})
}

// TestMakePath covers MakePath and round-trip through Segments.
func TestMakePath(t *testing.T) {
	t.Run("single segment", func(t *testing.T) {
		p := cuelite.MakePath("title")
		assert.Equal(t, []string{"title"}, p.Segments())
	})
	t.Run("multiple segments", func(t *testing.T) {
		p := cuelite.MakePath("a", "b", "c")
		assert.Equal(t, []string{"a", "b", "c"}, p.Segments())
	})
	t.Run("zero segments", func(t *testing.T) {
		p := cuelite.MakePath()
		assert.Nil(t, p.Segments())
	})
	t.Run("segments with hyphens", func(t *testing.T) {
		p := cuelite.MakePath("my-key", "sub")
		assert.Equal(t, []string{"my-key", "sub"}, p.Segments())
	})
}

// TestPath_Segments_returnsCopy ensures Segments returns a fresh copy so
// callers cannot corrupt the Path's internal state.
func TestPath_Segments_returnsCopy(t *testing.T) {
	p := cuelite.MakePath("a", "b", "c")
	got := p.Segments()
	require.Len(t, got, 3)
	got[0] = "MUTATED"
	// A second call must still see the original.
	assert.Equal(t, []string{"a", "b", "c"}, p.Segments())
}
