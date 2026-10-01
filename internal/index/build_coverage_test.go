package index

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// TestFrontMatterAll_NonMappingTopLevel covers the early-return
// branch when the front matter's top-level node is a YAML sequence
// rather than a mapping.
func TestFrontMatterAll_NonMappingTopLevel(t *testing.T) {
	t.Parallel()
	syms, title, kinds := frontMatterAll("a.md",
		[]byte("---\n- item\n- another\n---\n"))
	assert.Nil(t, syms)
	assert.Empty(t, title)
	assert.Nil(t, kinds)
}

// TestFrontMatterAll_SkipsEmptyAndNonScalarKeys covers the
// `k.Kind != ScalarNode || k.Value == ""` continue branch.
func TestFrontMatterAll_SkipsEmptyAndNonScalarKeys(t *testing.T) {
	t.Parallel()
	// `"": value` produces an empty-string scalar key — the loop
	// must skip it without emitting a Symbol.
	syms, _, _ := frontMatterAll("a.md",
		[]byte("---\n\"\": value\nreal: ok\n---\n"))
	for _, s := range syms {
		assert.NotEmpty(t, s.Name)
	}
	require.Len(t, syms, 1)
	assert.Equal(t, "real", syms[0].Name)
}

// The TestFrontMatterAll_* tests below port the YAML edge cases the
// removed single-purpose helpers (frontMatterScalar,
// frontMatterStringList, frontMatterSymbols) used to cover, plus the
// title and kinds cases where the index must agree with the engine's
// front-matter decoders.

// TestFrontMatterAll_UnusableInput covers inputs that yield no
// symbols, title, or kinds at all.
func TestFrontMatterAll_UnusableInput(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, src string }{
		{"nil input", ""},
		{"invalid yaml", "---\nthis: is\n  not: valid yaml\nxx: [\n---\n"},
		{"tagged scalar document", "---\n!!invalid\n---\n"},
		{"sequence document", "---\n- item\n- another\n---\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			syms, title, kinds := frontMatterAll("a.md", []byte(tc.src))
			assert.Nil(t, syms)
			assert.Empty(t, title)
			assert.Nil(t, kinds)
		})
	}
}

// TestFrontMatterAll_MissingTitleAndKinds covers a mapping with
// neither key: the outline symbol survives, title and kinds do not.
func TestFrontMatterAll_MissingTitleAndKinds(t *testing.T) {
	t.Parallel()
	syms, title, kinds := frontMatterAll("a.md", []byte("---\nfoo: bar\n---\n"))
	require.Len(t, syms, 1)
	assert.Equal(t, "foo", syms[0].Name)
	assert.Empty(t, title)
	assert.Nil(t, kinds)
}

// TestFrontMatterAll_Kinds checks the kinds list against what
// lint.ParseFrontMatterKinds (a []string decode) would report.
func TestFrontMatterAll_Kinds(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, src string
		want      []string
	}{
		{"non-list value", "kinds: hello", nil},
		{"typed scalars kept as text", "kinds:\n  - a\n  - 42\n  - b", []string{"a", "42", "b"}},
		{"mapping entry rejects the list", "kinds:\n  - a\n  - {x: y}", nil},
		{"merge key", "<<: {kinds: [m]}", []string{"m"}},
		// lint.ParseFrontMatterKinds reads only the bytes `kinds:`;
		// the index must not report kinds the engine never applies.
		{"quoted key not read", "\"kinds\": [a]", nil},
		{"escaped key not read", "\"kind\\x73\": [a]", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, _, kinds := frontMatterAll("a.md", []byte("---\n"+tc.src+"\n---\n"))
			assert.Equal(t, tc.want, kinds)
		})
	}
}

// TestFrontMatterAll_Title checks the title text for each scalar
// shape. Typed scalars keep their source text; the removed
// frontMatterScalar formatted a timestamp as RFC 3339 instead.
func TestFrontMatterAll_Title(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ name, src, want string }{
		{"non-scalar", "title: [a, b]", ""},
		{"null", "title: null", ""},
		{"tilde", "title: ~", ""},
		{"empty", "title:", ""},
		{"block scalar collapses whitespace", "title: |\n  multi\n  line", "multi line"},
		{"int", "title: 42", "42"},
		{"float", "title: 3.14", "3.14"},
		{"uint64", "title: 18446744073709551615", "18446744073709551615"},
		{"bool", "title: true", "true"},
		{"timestamp", "title: 2024-01-15", "2024-01-15"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, title, _ := frontMatterAll("a.md", []byte("---\n"+tc.src+"\n---\n"))
			assert.Equal(t, tc.want, title)
		})
	}
}

// TestFrontMatterAll_DuplicateKeys: a duplicated title key has no
// single value, so the index shows no title. Other duplicates leave
// the title alone. Kinds follow lint.ParseFrontMatterKinds, which
// rejects a duplicate key only when the block declares kinds.
func TestFrontMatterAll_DuplicateKeys(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, src string
		nSyms     int
		title     string
		kinds     []string
	}{
		{"title and kinds", "title: hi\ntitle: bye\nkinds: [a]\nkinds: [b]", 4, "", nil},
		{"unrelated key keeps title", "title: Notes\ntags: a\ntags: b", 3, "Notes", nil},
		{"empty keys with kinds", "\"\": x\n\"\": y\ntitle: t\nkinds: [a]", 2, "t", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			syms, title, kinds := frontMatterAll("a.md", []byte("---\n"+tc.src+"\n---\n"))
			assert.Len(t, syms, tc.nSyms)
			assert.Equal(t, tc.title, title)
			assert.Equal(t, tc.kinds, kinds)
		})
	}
}

// TestFrontMatterAll_DecodePanicInput: a complex key next to a merge
// key makes yaml.v3's struct decode panic; the index must survive.
func TestFrontMatterAll_DecodePanicInput(t *testing.T) {
	t.Parallel()
	src := []byte("---\n? [a, b]\n: c\n<<: {x: y}\ntitle: T\nkinds: [a]\n---\n")
	var title string
	var kinds []string
	require.NotPanics(t, func() { _, title, kinds = frontMatterAll("a.md", src) })
	assert.Equal(t, "T", title)
	assert.Nil(t, kinds)
}

// TestFrontMatterTitle covers the display-text rules for a title
// value node directly.
func TestFrontMatterTitle(t *testing.T) {
	t.Parallel()
	scalar := func(tag, v string) *yaml.Node {
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: tag, Value: v}
	}
	for _, tc := range []struct {
		name string
		node *yaml.Node
		want string
	}{
		{"absent", &yaml.Node{}, ""},
		{"sequence", &yaml.Node{Kind: yaml.SequenceNode}, ""},
		{"null", scalar("!!null", "null"), ""},
		{"plain", scalar("!!str", "Hello world"), "Hello world"},
		{"newlines", scalar("!!str", "multi\nline\n"), "multi line"},
		{"double space", scalar("!!str", "a  b"), "a b"},
		{"edge spaces", scalar("!!str", " a b "), "a b"},
		{"line separator", scalar("!!str", "a\u2028b"), "a b"},
		{"next line", scalar("!!str", "a\u0085b"), "a b"},
		{"no-break space", scalar("!!str", "a\u00a0b"), "a b"},
		{"binary", scalar("!!binary", "SGVsbG8gd29ybGQ="), "Hello world"},
		{"binary bad base64", scalar("!!binary", "%%%"), ""},
		{"binary not UTF-8", scalar("!!binary", "/w=="), ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, frontMatterTitle(tc.node))
		})
	}
}

// TestRefDefRegexpMatches covers the exported wrapper that lets the
// LSP rename surface iterate reference definitions without
// duplicating the package's regex.
func TestRefDefRegexpMatches(t *testing.T) {
	t.Parallel()
	body := []byte("# T\n\n[label]: https://example.com\n[other]: ./x.md\n")
	matches := RefDefRegexpMatches(body)
	require.Len(t, matches, 2)
	// Each match is [whole_start, whole_end, label_start, label_end].
	assert.Equal(t, "label", string(body[matches[0][2]:matches[0][3]]))
	assert.Equal(t, "other", string(body[matches[1][2]:matches[1][3]]))
}

// TestBuildSerialNilReceiver covers the defensive nil-receiver path.
func TestBuildSerialNilReceiver(t *testing.T) {
	t.Parallel()
	var idx *Index
	// Should not panic.
	idx.BuildSerial([]string{"a.md"}, func(string) ([]byte, error) {
		return []byte("# A\n"), nil
	})
}

// TestBuildSerialSkipsEmptyPathAndEmptyData covers the two `continue`
// branches in BuildSerial: empty workspace-relative path (e.g. ".")
// and loader returning either an error or empty bytes.
func TestBuildSerialSkipsEmptyPathAndEmptyData(t *testing.T) {
	t.Parallel()
	idx := New("/root")
	idx.BuildSerial(
		[]string{"", "good.md", "missing.md", "empty.md"},
		func(path string) ([]byte, error) {
			switch path {
			case "good.md":
				return []byte("# Good\n"), nil
			case "missing.md":
				return nil, errors.New("nope")
			case "empty.md":
				return nil, nil
			}
			return nil, errors.New("unexpected")
		},
	)
	files := idx.Files()
	assert.Equal(t, []string{"good.md"}, files,
		"only good.md survives — empty path, errored, and empty-bytes paths skipped")
}

// TestBuildEntriesParallel_ZeroWorkers covers the workers < 1 clamp
// branch (workers becomes 1 → serial path).
func TestBuildEntriesParallel_ZeroWorkers(t *testing.T) {
	t.Parallel()
	files := []string{"a.md", "b.md"}
	loader := func(string) ([]byte, error) {
		return []byte("# X\n"), nil
	}
	got := buildEntriesParallel(files, loader, 0)
	assert.Len(t, got, 2)
}

// TestBuildEntriesParallel_SkipsEmptyPathAndEmptyData covers the
// two `continue` branches inside the parallel worker loop. The
// single-file case takes the serial-fallback path, so we have to
// pass multiple files and make sure a couple of them get filtered.
func TestBuildEntriesParallel_SkipsEmptyPathAndEmptyData(t *testing.T) {
	t.Parallel()
	files := []string{"", "a.md", "b.md", "missing.md", "empty.md"}
	loader := func(path string) ([]byte, error) {
		switch path {
		case "a.md", "b.md":
			return []byte("# X\n"), nil
		case "missing.md":
			return nil, errors.New("nope")
		case "empty.md":
			return nil, nil
		}
		return nil, errors.New("unexpected")
	}
	got := buildEntriesParallel(files, loader, 4)
	assert.Len(t, got, 2)
	_, hasA := got["a.md"]
	_, hasB := got["b.md"]
	assert.True(t, hasA)
	assert.True(t, hasB)
}

// TestAbsToWorkspace_RelOutsideRoot covers the
// "filepath.Rel says path is outside root" branch. The normalised
// absolute path is returned as-is rather than presented as a
// workspace-relative `../` path.
func TestAbsToWorkspace_RelOutsideRoot(t *testing.T) {
	t.Parallel()
	// On POSIX, /elsewhere/x.md isn't inside /root → filepath.Rel
	// returns `../elsewhere/x.md`; the helper detects the `..` prefix
	// and returns the absolute form.
	got := absToWorkspace("/root", "/elsewhere/x.md")
	assert.Equal(t, "/elsewhere/x.md", got)
}

// TestCollectDirectiveEdges_BuildInputs covers the DirectiveBuild
// branches in collectDirectiveEdges:
//   - a glob `inputs:` entry (d.IsUnresolved() == true) emits an
//     EdgeBuild with Unresolved=true
//   - a literal `inputs:` entry emits a resolved EdgeBuild with
//     TargetFile set
//   - an absolute path (ResolveRelTarget returns "") is skipped
func TestCollectDirectiveEdges_BuildInputs(t *testing.T) {
	t.Parallel()
	// The build directive has three inputs (values must be quoted so
	// the YAML parser surfaced them as strings via ValidateStringParams):
	//   - "**/*.md"  → glob → unresolved edge
	//   - "src.svg"  → literal path → resolved edge (target: dir/src.svg)
	//   - "/abs.svg" → absolute → ResolveRelTarget returns "" → skipped
	src := []byte(
		"# T\n\n" +
			"<?build\n" +
			"recipe: render\n" +
			"outputs:\n" +
			"  - \"out.png\"\n" +
			"inputs:\n" +
			"  - \"**/*.md\"\n" +
			"  - \"src.svg\"\n" +
			"  - \"/abs.svg\"\n" +
			"?>\n" +
			"- [out.png](out.png)\n" +
			"<?/build?>\n",
	)
	fe := buildFileEntry("dir/doc.md", src)
	require.NotNil(t, fe)

	// Collect only EdgeBuild edges.
	var buildEdges []Edge
	for _, e := range fe.Outgoing {
		if e.Kind == EdgeBuild {
			buildEdges = append(buildEdges, e)
		}
	}
	// Expect exactly two build edges: the glob (unresolved) and the
	// literal path (resolved). The absolute path is skipped.
	require.Len(t, buildEdges, 2)

	// Find the unresolved and resolved edges (order may vary).
	var unresolved, resolved *Edge
	for i := range buildEdges {
		if buildEdges[i].Unresolved {
			unresolved = &buildEdges[i]
		} else {
			resolved = &buildEdges[i]
		}
	}
	require.NotNil(t, unresolved, "expected an unresolved build edge for the glob input")
	assert.Equal(t, "dir/doc.md", unresolved.SourceFile)
	assert.Empty(t, unresolved.TargetFile)

	require.NotNil(t, resolved, "expected a resolved build edge for src.svg")
	assert.Equal(t, "dir/doc.md", resolved.SourceFile)
	assert.Equal(t, "dir/src.svg", resolved.TargetFile)
}

// TestNeedsSpaceCollapse covers each whitespace shape that forces a
// collapse and the common single-spaced title that does not.
func TestNeedsSpaceCollapse(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		in   string
		want bool
	}{
		{"", false},
		{"word", false},
		{"two words", false},
		{" lead", true},
		{"trail ", true},
		{"a  b", true},
		{"a\tb", true},
		{"a\u2028b", true},
	} {
		assert.Equal(t, tc.want, needsSpaceCollapse(tc.in), "%q", tc.in)
	}
}
