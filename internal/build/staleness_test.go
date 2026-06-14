package build

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
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

func TestStaleness_OutputUnderMdsmithRefused(t *testing.T) {
	root := t.TempDir()
	in := newPlan(t, root, "gen", "echo hi", nil, []string{".mdsmith/cache.json"}, nil)
	cache := NewCache()
	_, err := CheckStaleness(in, cache)
	require.Error(t, err)
	assert.Contains(t, err.Error(), ".mdsmith/")
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

// --- resolveInputs error branches ---

func TestResolveInputs_GlobSyntaxError(t *testing.T) {
	root := t.TempDir()
	// An unclosed bracket is a glob syntax error in doublestar.
	in := newPlan(t, root, "r", "tool", []string{"[invalid"}, []string{"out.txt"}, nil)
	_, err := CheckStaleness(in, NewCache())
	require.Error(t, err)
}

func TestResolveInputs_LiteralPathEscapesRoot(t *testing.T) {
	root := t.TempDir()
	// A literal input with ".." that would escape the root is rejected.
	in := newPlan(t, root, "r", "tool", []string{"../escape.txt"}, []string{"out.txt"}, nil)
	_, err := CheckStaleness(in, NewCache())
	require.Error(t, err)
}

func TestResolveInputs_DefaultInputEscapesRoot(t *testing.T) {
	root := t.TempDir()
	// A default input with ".." that would escape the root is rejected.
	in := newPlan(t, root, "r", "tool", nil, []string{"out.txt"}, []string{"../escape.txt"})
	_, err := CheckStaleness(in, NewCache())
	require.Error(t, err)
}

// --- resolveOutputs error branch ---

func TestCheckStaleness_BadOutputPath(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "src.txt", "hello")
	// An output path with ".." that would escape the root is rejected.
	in := newPlan(t, root, "r", "tool", []string{"src.txt"}, []string{"../escape.txt"}, nil)
	_, err := CheckStaleness(in, NewCache())
	require.Error(t, err)
}

func TestRecordBuild_BadOutputPath(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "src.txt", "hello")
	in := newPlan(t, root, "r", "tool", []string{"src.txt"}, []string{"../escape.txt"}, nil)
	_, err := RecordBuild(in)
	require.Error(t, err)
}

// --- hashFile error branches ---

func TestComputeActionID_HashFileError(t *testing.T) {
	root := t.TempDir()
	// Create a directory named like an input file — ReadFile on a dir fails.
	require.NoError(t, os.MkdirAll(filepath.Join(root, "src.txt"), 0o755))
	in := newPlan(t, root, "r", "tool", []string{"src.txt"}, []string{"out.txt"}, nil)
	_, err := ComputeActionID(in)
	require.Error(t, err)
}

func TestCheckStaleness_HashFileErrorForOutput(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "src.txt", "hello")
	// Build the cache with a real file, then replace the output with a directory
	// so the content-hash step (step 5) fails.
	in := newPlan(t, root, "r", "tool", []string{"src.txt"}, []string{"dst.txt"}, nil)
	writeFile(t, root, "dst.txt", "world")
	cache := NewCache()
	entry, err := RecordBuild(in)
	require.NoError(t, err)
	cache.Put(entry)

	// Replace dst.txt with a directory so hashFile returns an error.
	require.NoError(t, os.Remove(filepath.Join(root, "dst.txt")))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "dst.txt"), 0o755))

	_, err = CheckStaleness(in, cache)
	require.Error(t, err)
}

func TestRecordBuild_HashFileErrorForOutput(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "src.txt", "hello")
	// Replace the output with a directory so hashFile returns an error.
	require.NoError(t, os.MkdirAll(filepath.Join(root, "out.txt"), 0o755))
	in := newPlan(t, root, "r", "tool", []string{"src.txt"}, []string{"out.txt"}, nil)
	_, err := RecordBuild(in)
	require.Error(t, err)
}

// --- ComputeActionID error paths ---

// --- ValidateInputs ---

func TestValidateInputs_MissingLiteralInput_ReturnsError(t *testing.T) {
	root := t.TempDir()
	in := newPlan(t, root, "r", "tool", []string{"absent.txt"}, []string{"out.txt"}, nil)
	require.Error(t, ValidateInputs(in))
}

func TestValidateInputs_AllInputsPresent_ReturnsNil(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "src.txt", "content")
	in := newPlan(t, root, "r", "tool", []string{"src.txt"}, []string{"out.txt"}, nil)
	require.NoError(t, ValidateInputs(in))
}

func TestValidateInputs_BadInputPath_ReturnsError(t *testing.T) {
	root := t.TempDir()
	in := newPlan(t, root, "r", "tool", []string{"../escape.txt"}, []string{"out.txt"}, nil)
	require.Error(t, ValidateInputs(in))
}

func TestComputeActionID_BadInputPath(t *testing.T) {
	root := t.TempDir()
	in := newPlan(t, root, "r", "tool", []string{"../escape.txt"}, []string{"out.txt"}, nil)
	_, err := ComputeActionID(in)
	require.Error(t, err)
}

func TestComputeActionID_BadOutputPath(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "src.txt", "hello")
	in := newPlan(t, root, "r", "tool", []string{"src.txt"}, []string{"../escape.txt"}, nil)
	_, err := ComputeActionID(in)
	require.Error(t, err)
}

// TestCheckStaleness_ComputeActionIDError covers the ComputeActionID error
// branch inside CheckStaleness (step 3). The output exists (so step 2 passes)
// but the input is a directory — hashFile returns an error.
func TestCheckStaleness_ComputeActionIDError(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "src.txt"), 0o755))
	writeFile(t, root, "out.txt", "result")
	in := newPlan(t, root, "r", "tool", []string{"src.txt"}, []string{"out.txt"}, nil)
	_, err := CheckStaleness(in, NewCache())
	require.Error(t, err)
}

// TestResolveInputs_GlobMatchEscapesRoot covers the ResolvePathInRoot error
// inside the glob-match loop in resolveInputs (the branch that handles a
// glob that matches a file escaping the root via symlink).
func TestResolveInputs_GlobMatchEscapesRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation is unreliable on Windows CI")
	}
	root := t.TempDir()
	outside := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("s"), 0o644))
	require.NoError(t, os.Symlink(filepath.Join(outside, "secret.txt"), filepath.Join(root, "leak.txt")))
	in := newPlan(t, root, "r", "tool", []string{"*.txt"}, []string{"out.txt"}, nil)
	_, err := CheckStaleness(in, NewCache())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "project root")
}

// TestResolveInputs_DuplicatesDeduped covers the dup=true branch in the add
// closure: a file that appears in both Target.Inputs and DefaultInputs is
// counted only once in the hash.
func TestResolveInputs_DuplicatesDeduped(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "src.txt", "content")
	writeFile(t, root, "out.txt", "out")
	// src.txt in both Inputs and DefaultInputs — the second occurrence is
	// deduplicated by the add closure.
	in := newPlan(t, root, "r", "tool", []string{"src.txt"}, []string{"out.txt"}, []string{"src.txt"})
	id, err := ComputeActionID(in)
	require.NoError(t, err)
	assert.NotEmpty(t, id)
}

// TestNormalizeOutput_EmptyString covers the p == "" branch in normalizeOutput.
func TestNormalizeOutput_EmptyString(t *testing.T) {
	assert.Equal(t, ".", normalizeOutput(""))
}

func TestVerdictString(t *testing.T) {
	assert.Equal(t, "FRESH", Fresh.String())
	assert.Equal(t, "STALE", Stale.String())
}

func TestStaleness_GlobInputMatches(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "src1.txt", "hello")
	writeFile(t, root, "src2.txt", "world")
	writeFile(t, root, "dst.txt", "result")
	in := newPlan(t, root, "cat", "cat {inputs}", []string{"src*.txt"}, []string{"dst.txt"}, nil)
	cache := NewCache()

	res, err := CheckStaleness(in, cache)
	require.NoError(t, err)
	require.Equal(t, Stale, res.Verdict)

	entry, err := RecordBuild(in)
	require.NoError(t, err)
	cache.Put(entry)

	res2, err := CheckStaleness(in, cache)
	require.NoError(t, err)
	assert.Equal(t, Fresh, res2.Verdict)
}

func TestResolveInputs_GlobCapExceeded(t *testing.T) {
	old := globCapFn
	globCapFn = func(_ int) error { return errors.New("cap exceeded") }
	defer func() { globCapFn = old }()

	root := t.TempDir()
	writeFile(t, root, "a.txt", "a")
	in := newPlan(t, root, "r", "tool", []string{"*.txt"}, []string{"out.txt"}, nil)
	_, err := CheckStaleness(in, NewCache())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cap exceeded")
}

func TestRecordBuild_BadInputPath(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "src.txt", "hello")
	in := newPlan(t, root, "r", "tool", []string{"../escape.txt"}, []string{"dst.txt"}, nil)
	_, err := RecordBuild(in)
	require.Error(t, err)
}

func TestRecordBuild_HashFileErrorForInput(t *testing.T) {
	root := t.TempDir()
	// src.txt is a directory: resolveInputs succeeds (path is in root),
	// but computeActionIDFromResolved fails when it tries to hash the dir.
	require.NoError(t, os.MkdirAll(filepath.Join(root, "src.txt"), 0o755))
	in := newPlan(t, root, "r", "tool", []string{"src.txt"}, []string{"dst.txt"}, nil)
	_, err := RecordBuild(in)
	require.Error(t, err)
}

// --- Explain ---

func TestExplain_ReturnsFullBreakdown(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "src.txt", "hello")
	in := newPlan(t, root, "copy", "cp {inputs} {outputs}", []string{"src.txt"}, []string{"dst.txt"}, nil)

	ex, err := Explain(in)
	require.NoError(t, err)

	assert.Equal(t, "cp {inputs} {outputs}", ex.Command)
	assert.Equal(t, CacheVersion, ex.CacheVersion)
	assert.NotEmpty(t, ex.ActionID)
	assert.True(t, len(ex.Inputs) == 1)
	assert.Equal(t, "src.txt", ex.Inputs[0].Path)
	assert.True(t, len(ex.Inputs[0].Hash) > 0, "hash must be non-empty")
	assert.Contains(t, ex.Inputs[0].Hash, "sha256-")
	assert.Equal(t, []string{"dst.txt"}, ex.Outputs)
}

func TestExplain_WithParams_ReturnsParams(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "src.txt", "data")
	in := StalenessInput{
		Target: Target{
			Recipe:  "copy",
			Root:    root,
			Inputs:  []string{"src.txt"},
			Outputs: []string{"dst.txt"},
			Params:  map[string]string{"mode": "fast", "level": "3"},
		},
		Command: "tool --mode {mode} --level {level} {inputs} {outputs}",
	}

	ex, err := Explain(in)
	require.NoError(t, err)

	assert.Equal(t, map[string]string{"mode": "fast", "level": "3"}, ex.Params)
	assert.Contains(t, ex.ActionID, "sha256-")
}

func TestExplain_BadInputPath_ReturnsError(t *testing.T) {
	root := t.TempDir()
	in := newPlan(t, root, "r", "tool", []string{"../escape.txt"}, []string{"out.txt"}, nil)
	_, err := Explain(in)
	require.Error(t, err)
}

func TestExplain_BadOutputPath_ReturnsError(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "src.txt", "hello")
	in := newPlan(t, root, "r", "tool", []string{"src.txt"}, []string{"../escape.txt"}, nil)
	_, err := Explain(in)
	require.Error(t, err)
}

func TestExplain_InputIsDirectory_ReturnsHashError(t *testing.T) {
	root := t.TempDir()
	// src.txt is a directory: resolveInputs sees it as in-root, but hashFile
	// fails when it tries to read a directory as a file.
	require.NoError(t, os.MkdirAll(filepath.Join(root, "src.txt"), 0o755))
	in := newPlan(t, root, "r", "tool", []string{"src.txt"}, []string{"out.txt"}, nil)
	_, err := Explain(in)
	require.Error(t, err)
}

func TestExplain_NoInputsNoOutputs_EmptyFields(t *testing.T) {
	root := t.TempDir()
	// No inputs, no outputs: Explain must still return a valid ActionID.
	in := StalenessInput{
		Target:  Target{Recipe: "r", Root: root, Inputs: nil, Outputs: nil},
		Command: "echo hello",
	}
	ex, err := Explain(in)
	require.NoError(t, err)
	assert.Empty(t, ex.Inputs)
	assert.Empty(t, ex.Outputs)
	assert.Contains(t, ex.ActionID, "sha256-")
}
