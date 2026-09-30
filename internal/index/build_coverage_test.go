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

// TestFrontMatterAll_EdgeCases ports the YAML edge cases the removed
// single-purpose helpers used to cover: empty and invalid input,
// non-scalar title, non-list kinds, mixed-type kinds, and front
// matter whose closing delimiter has no trailing newline.
func TestFrontMatterAll_EdgeCases(t *testing.T) {
	t.Parallel()

	syms, title, kinds := frontMatterAll("a.md", nil)
	assert.Nil(t, syms)
	assert.Empty(t, title)
	assert.Nil(t, kinds)

	syms, title, kinds = frontMatterAll("a.md",
		[]byte("---\nthis: is\n  not: valid yaml\nxx: [\n---\n"))
	assert.Nil(t, syms)
	assert.Empty(t, title)
	assert.Nil(t, kinds)

	_, _, _ = frontMatterAll("a.md", []byte("---\n!!invalid\n---\n"))

	// Missing title and kinds keys.
	_, title, kinds = frontMatterAll("a.md", []byte("---\nfoo: bar\n---\n"))
	assert.Empty(t, title)
	assert.Nil(t, kinds)

	// Non-list kinds value.
	_, _, kinds = frontMatterAll("a.md", []byte("---\nkinds: hello\n---\n"))
	assert.Empty(t, kinds)

	// Mixed kinds list: non-string elements are skipped.
	_, _, kinds = frontMatterAll("a.md",
		[]byte("---\nkinds:\n  - a\n  - 42\n  - b\n---\n"))
	assert.Equal(t, []string{"a", "b"}, kinds)

	// Non-scalar title is ignored; null title yields no text.
	_, title, _ = frontMatterAll("a.md", []byte("---\ntitle: [a, b]\n---\n"))
	assert.Empty(t, title)

	// Numeric title keeps its source text.
	_, title, _ = frontMatterAll("a.md", []byte("---\ntitle: 42\n---\n"))
	assert.Equal(t, "42", title)

	// No trailing newline after the closing delimiter.
	_, title, _ = frontMatterAll("a.md", []byte("---\ntitle: hi\n---"))
	assert.Equal(t, "hi", title)
}

// TestFrontMatterKindsList_NonSequence covers the
// `v.Kind != SequenceNode` early-return branch. A scalar `kinds:
// guide` value yields no list entries.
func TestFrontMatterKindsList_NonSequence(t *testing.T) {
	t.Parallel()
	assert.Nil(t, frontMatterKindsList(nil),
		"nil value node short-circuits to nil")
	assert.Nil(t, frontMatterKindsList(&yaml.Node{Kind: yaml.ScalarNode, Value: "x"}),
		"scalar value short-circuits to nil — the front-matter walk")
}

// TestFrontMatterKindsList_NonScalarItem covers the
// `item.Kind != ScalarNode` skip branch — a mapping entry in a
// kinds: list is filtered out without crashing.
func TestFrontMatterKindsList_NonScalarItem(t *testing.T) {
	t.Parallel()
	mapping := &yaml.Node{Kind: yaml.MappingNode}
	str := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "ok"}
	seq := &yaml.Node{
		Kind:    yaml.SequenceNode,
		Content: []*yaml.Node{mapping, str},
	}
	got := frontMatterKindsList(seq)
	assert.Equal(t, []string{"ok"}, got)
}

// TestFrontMatterKindsList_NonStringTaggedItem covers the
// `item.Tag != "" && item.Tag != "!!str"` skip branch — a YAML
// integer in a kinds: list is filtered out so callers see only
// string entries (matches the previous map[string]any path that
// dropped non-strings via type assertion).
func TestFrontMatterKindsList_NonStringTaggedItem(t *testing.T) {
	t.Parallel()
	// `- 42` in YAML resolves to Tag "!!int"; `- "real"` is "!!str".
	src := []byte("---\nkinds:\n  - 42\n  - real\n---\n")
	_, _, kinds := frontMatterAll("a.md", src)
	assert.Equal(t, []string{"real"}, kinds)
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
