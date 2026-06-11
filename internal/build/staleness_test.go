package build

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeFile writes content to root/rel, creating parent dirs.
func writeFile(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
	require.NoError(t, os.WriteFile(p, []byte(content), 0o644))
}

func newPlan(t *testing.T, root, recipe, cmd string, inputs, outputs, defaults []string) StalenessInput {
	t.Helper()
	return StalenessInput{
		Target: Target{
			Recipe:  recipe,
			Root:    root,
			Inputs:  inputs,
			Outputs: outputs,
		},
		Command:       cmd,
		DefaultInputs: defaults,
	}
}

func TestStaleness_MissingOutputIsStale(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "src.txt", "hello")
	in := newPlan(t, root, "copy", "cp {inputs} {outputs}", []string{"src.txt"}, []string{"dst.txt"}, nil)
	cache := NewCache()

	res, err := CheckStaleness(in, cache)
	require.NoError(t, err)
	assert.Equal(t, Stale, res.Verdict)
}

func TestStaleness_FreshAfterBuild(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "src.txt", "hello")
	writeFile(t, root, "dst.txt", "hello")
	in := newPlan(t, root, "copy", "cp {inputs} {outputs}", []string{"src.txt"}, []string{"dst.txt"}, nil)
	cache := NewCache()

	// First check: stale (no cache entry).
	res, err := CheckStaleness(in, cache)
	require.NoError(t, err)
	require.Equal(t, Stale, res.Verdict)

	// Record the build in the cache, then re-check: fresh.
	entry, err := RecordBuild(in)
	require.NoError(t, err)
	cache.Put(entry)

	res2, err := CheckStaleness(in, cache)
	require.NoError(t, err)
	assert.Equal(t, Fresh, res2.Verdict)
}

func TestStaleness_InputContentChangeIsStale(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "src.txt", "hello")
	writeFile(t, root, "dst.txt", "hello")
	in := newPlan(t, root, "copy", "cp {inputs} {outputs}", []string{"src.txt"}, []string{"dst.txt"}, nil)
	cache := NewCache()
	entry, err := RecordBuild(in)
	require.NoError(t, err)
	cache.Put(entry)

	// Change the input content.
	writeFile(t, root, "src.txt", "changed")
	res, err := CheckStaleness(in, cache)
	require.NoError(t, err)
	assert.Equal(t, Stale, res.Verdict)
}

func TestStaleness_RecipeCommandChangeIsStale(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "src.txt", "hello")
	writeFile(t, root, "dst.txt", "hello")
	in := newPlan(t, root, "copy", "cp {inputs} {outputs}", []string{"src.txt"}, []string{"dst.txt"}, nil)
	cache := NewCache()
	entry, err := RecordBuild(in)
	require.NoError(t, err)
	cache.Put(entry)

	in.Command = "install {inputs} {outputs}"
	res, err := CheckStaleness(in, cache)
	require.NoError(t, err)
	assert.Equal(t, Stale, res.Verdict)
}

func TestStaleness_TamperedOutputIsStale(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "src.txt", "hello")
	writeFile(t, root, "dst.txt", "hello")
	in := newPlan(t, root, "copy", "cp {inputs} {outputs}", []string{"src.txt"}, []string{"dst.txt"}, nil)
	cache := NewCache()
	entry, err := RecordBuild(in)
	require.NoError(t, err)
	cache.Put(entry)

	// Hand-edit the artifact: ActionID unchanged, output hash now differs.
	writeFile(t, root, "dst.txt", "tampered")
	res, err := CheckStaleness(in, cache)
	require.NoError(t, err)
	assert.Equal(t, Stale, res.Verdict)
}

func TestStaleness_MissingInputIsError(t *testing.T) {
	root := t.TempDir()
	in := newPlan(t, root, "copy", "cp {inputs} {outputs}", []string{"absent.txt"}, []string{"dst.txt"}, nil)
	cache := NewCache()
	_, err := CheckStaleness(in, cache)
	require.Error(t, err)
}

func TestStaleness_GlobMatchingZeroFilesIsError(t *testing.T) {
	root := t.TempDir()
	in := newPlan(t, root, "copy", "cp {inputs} {outputs}", []string{"*.none"}, []string{"dst.txt"}, nil)
	cache := NewCache()
	_, err := CheckStaleness(in, cache)
	require.Error(t, err)
}

func TestStaleness_DefaultInputsFoldedIntoHash(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "demo.tape", "v1")
	writeFile(t, root, "out.gif", "rendered")
	in := newPlan(t, root, "vhs", "vhs {tape}", nil, []string{"out.gif"}, []string{"demo.tape"})
	cache := NewCache()
	entry, err := RecordBuild(in)
	require.NoError(t, err)
	cache.Put(entry)

	res, err := CheckStaleness(in, cache)
	require.NoError(t, err)
	require.Equal(t, Fresh, res.Verdict)

	// Changing the default-input content must invalidate.
	writeFile(t, root, "demo.tape", "v2")
	res2, err := CheckStaleness(in, cache)
	require.NoError(t, err)
	assert.Equal(t, Stale, res2.Verdict)
}

func TestStaleness_TwoOutputRebuildsWhenEitherDeleted(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "a.txt", "a")
	writeFile(t, root, "b.txt", "b")
	in := newPlan(t, root, "dup", "tool {outputs}", nil, []string{"a.txt", "b.txt"}, nil)
	cache := NewCache()
	entry, err := RecordBuild(in)
	require.NoError(t, err)
	cache.Put(entry)

	require.NoError(t, os.Remove(filepath.Join(root, "b.txt")))
	res, err := CheckStaleness(in, cache)
	require.NoError(t, err)
	assert.Equal(t, Stale, res.Verdict)
}

func TestActionID_LengthFramedNoCollision(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "a", "x")
	writeFile(t, root, "b", "y")
	// Two param maps that would collide under naive concatenation:
	// {"a":"b","c":"d"} vs {"a":"bc","":"d"} — framing must separate them.
	in1 := StalenessInput{
		Target:  Target{Recipe: "r", Root: root, Outputs: []string{"a"}, Params: map[string]string{"k": "v", "x": "y"}},
		Command: "tool",
	}
	in2 := StalenessInput{
		Target:  Target{Recipe: "r", Root: root, Outputs: []string{"a"}, Params: map[string]string{"k": "vx", "": "y"}},
		Command: "tool",
	}
	id1, err := ComputeActionID(in1)
	require.NoError(t, err)
	id2, err := ComputeActionID(in2)
	require.NoError(t, err)
	assert.NotEqual(t, id1, id2)
}

func TestDetectOutputOverlap_ExactCollision(t *testing.T) {
	plans := []OverlapTarget{
		{File: "a.md", Line: 1, Outputs: []string{"out.txt"}},
		{File: "b.md", Line: 5, Outputs: []string{"out.txt"}},
	}
	err := DetectOutputOverlap(plans)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "a.md")
	assert.Contains(t, err.Error(), "b.md")
}

func TestDetectOutputOverlap_DirPrefixCollision(t *testing.T) {
	plans := []OverlapTarget{
		{File: "a.md", Line: 1, Outputs: []string{"book/"}},
		{File: "b.md", Line: 2, Outputs: []string{"book/index.html"}},
	}
	err := DetectOutputOverlap(plans)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "book")
}

func TestDetectOutputOverlap_NoOverlap(t *testing.T) {
	plans := []OverlapTarget{
		{File: "a.md", Line: 1, Outputs: []string{"a.txt"}},
		{File: "b.md", Line: 2, Outputs: []string{"b.txt"}},
	}
	require.NoError(t, DetectOutputOverlap(plans))
}
