package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	buildexec "github.com/jeduden/mdsmith/internal/build"
)

// writeShScript writes an executable shell script and returns its path.
func writeShScript(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(p, []byte("#!/bin/sh\n"+body+"\n"), 0o755))
	return p
}

func TestDispatchOne_FailingRecipePrintsSixFieldBlock(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sh is not available on Windows")
	}
	root := t.TempDir()
	bindir := t.TempDir()
	script := writeShScript(t, bindir, "boom.sh", `echo "boom error 1" 1>&2; echo "boom error 2" 1>&2; exit 4`)

	cfg := buildPassCfg("    boom:\n      command: " + script + " {outputs}\n")
	bt := buildTarget{
		file: "chapters/intro.md",
		line: 12,
		target: buildexec.Target{
			Recipe:  "boom",
			Root:    root,
			Outputs: []string{"out.txt"},
		},
	}
	builder := buildexec.NewCustomBuilder(map[string]buildexec.RecipeSpec{
		"boom": {Command: script + " {outputs}"},
	})

	cache := buildexec.NewCache()
	var buf strings.Builder
	outcome := dispatchOne(builder, bt, cfg, buildPassOpts{}, cache, time.Second, &buf)
	require.Equal(t, outcomeFailed, outcome)
	out := buf.String()
	assert.Contains(t, out, "FAIL out.txt")
	assert.Contains(t, out, "source:")
	assert.Contains(t, out, "chapters/intro.md:12")
	assert.Contains(t, out, "argv:")
	assert.Contains(t, out, "cwd:")
	assert.Contains(t, out, "exit:")
	assert.Contains(t, out, "4")
	assert.Contains(t, out, "duration:")
	assert.Contains(t, out, "log:")
	assert.Contains(t, out, "last 20 lines of stderr")
	assert.Contains(t, out, "boom error 2")
}

func TestDispatchOne_TimeoutPrintsDiagnosticBlock(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sh is not available on Windows")
	}
	root := t.TempDir()
	bindir := t.TempDir()
	script := writeShScript(t, bindir, "hang.sh", `echo "warming up" 1>&2; echo "ready" ; sleep 30`)

	cfg := buildPassCfg("    hang:\n      command: " + script + " {outputs}\n")
	bt := buildTarget{
		file: "doc.md",
		line: 1,
		target: buildexec.Target{
			Recipe:  "hang",
			Root:    root,
			Outputs: []string{"book.html"},
		},
	}
	builder := buildexec.NewCustomBuilder(map[string]buildexec.RecipeSpec{
		"hang": {Command: script + " {outputs}"},
	})

	cache := buildexec.NewCache()
	var buf strings.Builder
	outcome := dispatchOne(builder, bt, cfg, buildPassOpts{}, cache, 200*time.Millisecond, &buf)
	require.Equal(t, outcomeFailed, outcome)
	out := buf.String()
	assert.Contains(t, out, "TIMEOUT book.html")
	assert.Contains(t, out, "last 20 lines of stdout")
	assert.Contains(t, out, "last 20 lines of stderr")
	assert.Contains(t, out, "SIGTERM")
}

// --- lastLines ---

func TestLastLines_FewerThanN_ReturnsAll(t *testing.T) {
	lines := []string{"a", "b", "c"}
	got := lastLines(lines, 10)
	assert.Equal(t, lines, got)
}

func TestLastLines_MoreThanN_ReturnsTail(t *testing.T) {
	lines := []string{"a", "b", "c", "d", "e"}
	got := lastLines(lines, 3)
	assert.Equal(t, []string{"c", "d", "e"}, got)
}

func TestLastLines_ExactlyN_ReturnsAll(t *testing.T) {
	lines := []string{"x", "y", "z"}
	got := lastLines(lines, 3)
	assert.Equal(t, lines, got)
}

// --- relLogPath ---

func TestRelLogPath_Empty_ReturnsNoLog(t *testing.T) {
	assert.Equal(t, "(no log)", relLogPath("/any/root", ""))
}

func TestRelLogPath_UnderRoot_ReturnsRelative(t *testing.T) {
	root := t.TempDir()
	logPath := filepath.Join(root, ".mdsmith", "build-logs", "abc.log")
	rel := relLogPath(root, logPath)
	assert.False(t, strings.HasPrefix(rel, ".."), "expected relative path, got %q", rel)
	assert.Contains(t, rel, "abc.log")
}

func TestRelLogPath_OutsideRoot_ReturnsAbsolute(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "other.log")
	got := relLogPath(root, outside)
	assert.Equal(t, outside, got)
}

// --- sortedKeys ---

func TestSortedKeys_Empty(t *testing.T) {
	assert.Empty(t, sortedKeys(map[string]string{}))
}

func TestSortedKeys_Sorted(t *testing.T) {
	m := map[string]string{"z": "1", "a": "2", "m": "3"}
	got := sortedKeys(m)
	assert.Equal(t, []string{"a", "m", "z"}, got)
}

// --- snapshotOutputs ---

func TestSnapshotOutputs_ExistingFile(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "out.txt"), []byte("hello"), 0o644))
	bt := buildTarget{
		target: buildexec.Target{Root: root, Outputs: []string{"out.txt"}},
	}
	snap := snapshotOutputs(bt)
	require.Len(t, snap, 1)
	assert.Equal(t, []byte("hello"), snap["out.txt"])
}

func TestSnapshotOutputs_MissingFile_ReturnsNil(t *testing.T) {
	root := t.TempDir()
	bt := buildTarget{
		target: buildexec.Target{Root: root, Outputs: []string{"absent.txt"}},
	}
	snap := snapshotOutputs(bt)
	require.Len(t, snap, 1)
	assert.Nil(t, snap["absent.txt"])
}

// --- outputsEqual ---

func TestOutputsEqual_IdenticalMaps_ReturnsTrue(t *testing.T) {
	a := map[string][]byte{"a.txt": []byte("x"), "b.txt": []byte("y")}
	b := map[string][]byte{"a.txt": []byte("x"), "b.txt": []byte("y")}
	assert.True(t, outputsEqual(a, b))
}

func TestOutputsEqual_DifferentContent_ReturnsFalse(t *testing.T) {
	a := map[string][]byte{"a.txt": []byte("x")}
	b := map[string][]byte{"a.txt": []byte("y")}
	assert.False(t, outputsEqual(a, b))
}

func TestOutputsEqual_DifferentKeys_ReturnsFalse(t *testing.T) {
	a := map[string][]byte{"a.txt": []byte("x")}
	b := map[string][]byte{"b.txt": []byte("x")}
	assert.False(t, outputsEqual(a, b))
}

func TestOutputsEqual_DifferentLength_ReturnsFalse(t *testing.T) {
	a := map[string][]byte{"a.txt": []byte("x"), "b.txt": []byte("y")}
	b := map[string][]byte{"a.txt": []byte("x")}
	assert.False(t, outputsEqual(a, b))
}

func TestOutputsEqual_BothNilValue_ReturnsTrue(t *testing.T) {
	a := map[string][]byte{"a.txt": nil}
	b := map[string][]byte{"a.txt": nil}
	assert.True(t, outputsEqual(a, b))
}

// --- printVerdict ---

func TestPrintVerdict_StaleVerdictWritten(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "src.txt"), []byte("content"), 0o644))
	stin := buildexec.StalenessInput{
		Target: buildexec.Target{
			Recipe:  "cp",
			Root:    root,
			Inputs:  []string{"src.txt"},
			Outputs: []string{"out.txt"},
		},
		Command: "cp {inputs} {outputs}",
	}
	cache := buildexec.NewCache()
	var buf strings.Builder
	printVerdict(stin, cache, &buf)
	assert.Contains(t, buf.String(), "STALE")
}

func TestPrintVerdict_UnstableFlagWritten(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "src.txt"), []byte("content"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "out.txt"), []byte("result"), 0o644))
	stin := buildexec.StalenessInput{
		Target: buildexec.Target{
			Recipe:  "cp",
			Root:    root,
			Inputs:  []string{"src.txt"},
			Outputs: []string{"out.txt"},
		},
		Command: "cp {inputs} {outputs}",
	}
	cache := buildexec.NewCache()
	// Record a build entry and mark it unstable, then re-run so verdict is FRESH.
	entry, err := buildexec.RecordBuild(stin)
	require.NoError(t, err)
	entry.Unstable = true
	cache.Put(entry)
	var buf strings.Builder
	printVerdict(stin, cache, &buf)
	assert.Contains(t, buf.String(), "FRESH")
	assert.Contains(t, buf.String(), "unstable: true")
}

// --- verifyTarget ---

func TestVerifyTarget_DeterministicRecipe_NoUnstable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sh not available on Windows")
	}
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "src.txt"), []byte("hello"), 0o644))

	cmd := "cp {inputs} {outputs}"
	builder := buildexec.NewCustomBuilder(map[string]buildexec.RecipeSpec{
		"cp": {Command: cmd},
	})

	bt := buildTarget{
		file: "doc.md",
		line: 1,
		target: buildexec.Target{
			Recipe:  "cp",
			Root:    root,
			Inputs:  []string{"src.txt"},
			Outputs: []string{"out.txt"},
		},
	}
	// Run the first build so snapshotOutputs sees a committed output.
	require.NoError(t, builder.Build(context.Background(), bt.target))
	stin := buildexec.StalenessInput{
		Target:  bt.target,
		Command: cmd,
	}
	res := &targetRunResult{}
	var buf strings.Builder
	verifyTarget(context.Background(), builder, bt, stin, buildPassOpts{}, time.Second, res, &buf)
	assert.False(t, res.Unstable)
}

func TestVerifyTarget_FailingReRunSetsUnstable(t *testing.T) {
	// A mock builder that always fails the second call simulates a re-run error.
	callCount := 0
	mock := &mockBuilder{fn: func(_ context.Context, _ buildexec.Target) error {
		callCount++
		return errors.New("verify re-run failed")
	}}
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "out.txt"), []byte("first"), 0o644))
	bt := buildTarget{
		target: buildexec.Target{
			Recipe:  "boom",
			Root:    root,
			Outputs: []string{"out.txt"},
		},
	}
	stin := buildexec.StalenessInput{Target: bt.target}
	res := &targetRunResult{}
	var buf strings.Builder
	verifyTarget(context.Background(), mock, bt, stin, buildPassOpts{}, time.Second, res, &buf)
	assert.True(t, res.Unstable)
	assert.Contains(t, buf.String(), "verify re-run failed")
}

// TestVerifyTarget_NonDeterministicOutput_SetsUnstable covers the branch where
// the two recipe runs produce different output bytes — sets res.Unstable and
// prints a warning.
func TestVerifyTarget_NonDeterministicOutput_SetsUnstable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sh not available on Windows")
	}
	root := t.TempDir()
	// Write an initial output so snapshotOutputs captures something.
	require.NoError(t, os.WriteFile(filepath.Join(root, "out.txt"), []byte("first"), 0o644))

	// The mock builder writes different content on every call.
	call := 0
	mock := &mockBuilder{fn: func(_ context.Context, _ buildexec.Target) error {
		call++
		content := []byte("run" + string(rune('0'+call)))
		_ = os.WriteFile(filepath.Join(root, "out.txt"), content, 0o644)
		return nil
	}}

	bt := buildTarget{
		file: "doc.md",
		line: 1,
		target: buildexec.Target{
			Recipe:  "rand",
			Root:    root,
			Outputs: []string{"out.txt"},
		},
	}
	stin := buildexec.StalenessInput{Target: bt.target}
	res := &targetRunResult{}
	var buf strings.Builder
	verifyTarget(context.Background(), mock, bt, stin, buildPassOpts{}, time.Second, res, &buf)
	assert.True(t, res.Unstable, "non-deterministic output must set Unstable")
	assert.Contains(t, buf.String(), "non-deterministic")
}

// --- printVerdict ERROR branch ---

// TestPrintVerdict_ErrorBranch covers the err != nil path in printVerdict when
// CheckStaleness returns an error. We give it a literal input path that does not
// exist on disk, which causes resolveInputs to return an error.
func TestPrintVerdict_ErrorBranch(t *testing.T) {
	root := t.TempDir()
	// "absent.txt" does not exist — resolveInputs returns an error for
	// a literal (non-glob) input that is missing.
	stin := buildexec.StalenessInput{
		Target: buildexec.Target{
			Recipe:  "cp",
			Root:    root,
			Inputs:  []string{"absent.txt"},
			Outputs: []string{"out.txt"},
		},
		Command: "cp {inputs} {outputs}",
	}
	cache := buildexec.NewCache()
	var buf strings.Builder
	printVerdict(stin, cache, &buf)
	out := buf.String()
	assert.Contains(t, out, "verdict: ERROR:", "missing input must trigger the ERROR branch")
}

// --- runOneTarget ComputeActionID error ---

// TestRunOneTarget_ComputeActionIDError covers Fix 1: when the input file is
// removed between verdict and run, ComputeActionID fails and runOneTarget
// returns a Result with Err set rather than silently ignoring the failure.
func TestRunOneTarget_ComputeActionIDError(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "src.txt"), []byte("data"), 0o644))
	bt := buildTarget{
		file: "doc.md",
		line: 1,
		target: buildexec.Target{
			Recipe:  "cp",
			Root:    root,
			Inputs:  []string{"src.txt"},
			Outputs: []string{"out.txt"},
		},
	}
	stin := buildexec.StalenessInput{
		Target:  bt.target,
		Command: "cp {inputs} {outputs}",
	}
	called := false
	builder := &mockBuilder{fn: func(_ context.Context, _ buildexec.Target) error {
		called = true
		return nil
	}}
	// Delete input so ComputeActionID's resolveInputs call fails.
	require.NoError(t, os.Remove(filepath.Join(root, "src.txt")))

	var buf strings.Builder
	res := runOneTarget(builder, bt, stin, buildPassOpts{}, time.Second, &buf)
	assert.Error(t, res.Err, "must return an error when ComputeActionID fails")
	assert.False(t, called, "builder must not be invoked when ActionID computation fails")
}

// TestReportBuildFailure_ErrorPrintedWithStderr verifies that the error: field
// is printed even when StderrTail is non-empty (Fix 2).
func TestReportBuildFailure_ErrorPrintedWithStderr(t *testing.T) {
	bt := buildTarget{
		file: "doc.md",
		line: 1,
		target: buildexec.Target{
			Recipe:  "cp",
			Root:    t.TempDir(),
			Outputs: []string{"out.txt"},
		},
	}
	res := targetRunResult{
		Result: buildexec.Result{
			Argv:       []string{"cp", "src.txt", "out.txt"},
			Cwd:        "/tmp",
			ExitCode:   1,
			Err:        errors.New("recipe cp failed: exit status 1"),
			StderrTail: []string{"some stderr line"},
		},
	}
	var buf strings.Builder
	reportBuildFailure(bt, res, &buf)
	out := buf.String()
	assert.Contains(t, out, "error:", "error: field must appear even when StderrTail is non-empty")
	assert.Contains(t, out, "some stderr line", "stderr tail must also be printed")
}

// TestDispatchTargets_JobsWithDryRun_PrintsWarning covers Fix 4: when
// --build-jobs > 1 and --build-dry-run are both set, a warning is printed.
func TestDispatchTargets_JobsWithDryRun_PrintsWarning(t *testing.T) {
	root := t.TempDir()
	cfg := buildPassCfg("    cp:\n      command: cp {inputs} {outputs}\n")
	bt := buildTarget{
		file: "doc.md",
		line: 1,
		target: buildexec.Target{
			Recipe:  "cp",
			Root:    root,
			Outputs: []string{"out.txt"},
		},
	}
	builder := &mockBuilder{fn: func(_ context.Context, _ buildexec.Target) error { return nil }}
	cache := buildexec.NewCache()
	var buf strings.Builder
	dispatchTargets(builder, []buildTarget{bt}, cfg, root,
		buildPassOpts{jobs: 2, dryRun: true},
		cache, time.Second, &buf)
	assert.Contains(t, buf.String(), "--build-jobs ignored with --build-dry-run")
}

// TestRunBuildPass_NoCacheSkipsPruneOrphanLogs covers Fix 3: when
// --build-no-cache is set, pruneOrphanLogsFn is never called.
func TestRunBuildPass_NoCacheSkipsPruneOrphanLogs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("touch not available on Windows")
	}
	root := t.TempDir()
	cfg := buildPassCfg("    mk:\n      command: touch {outputs}\n")
	cfgPath := filepath.Join(root, ".mdsmith.yml")
	md := buildPassDirective("mk", "out.txt")
	p := filepath.Join(root, "doc.md")
	require.NoError(t, os.WriteFile(p, []byte(md), 0o644))

	orig := pruneOrphanLogsFn
	called := false
	pruneOrphanLogsFn = func(_ string, _ *buildexec.Cache) error {
		called = true
		return errors.New("should not be called")
	}
	t.Cleanup(func() { pruneOrphanLogsFn = orig })

	var buf strings.Builder
	code := runBuildPass(cfg, cfgPath, []string{p},
		buildPassOpts{timeout: time.Second, noCache: true}, &buf)
	assert.Equal(t, 0, code)
	assert.False(t, called, "pruneOrphanLogsFn must not be called with --build-no-cache")
}

// TestVerifyTarget_StreamEnabled_ForwardsLiveOutput covers Fix 6: when
// stream is true, verifyOpts.LiveSink is set so the verify re-run forwards
// recipe output to w.
func TestVerifyTarget_StreamEnabled_ForwardsLiveOutput(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sh not available on Windows")
	}
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "src.txt"), []byte("hello"), 0o644))
	script := writeShScript(t, root, "emit.sh", `echo "verify-live"; cp "$1" "$2"`)

	cmd := script + " {inputs} {outputs}"
	builder := buildexec.NewCustomBuilder(map[string]buildexec.RecipeSpec{
		"cp": {Command: cmd},
	})
	bt := buildTarget{
		file: "doc.md",
		line: 1,
		target: buildexec.Target{
			Recipe:  "cp",
			Root:    root,
			Inputs:  []string{"src.txt"},
			Outputs: []string{"out.txt"},
		},
	}
	// Run the first build so snapshotOutputs captures a committed output.
	require.NoError(t, builder.Build(context.Background(), bt.target))
	stin := buildexec.StalenessInput{Target: bt.target, Command: cmd}

	res := &targetRunResult{}
	var buf strings.Builder
	verifyTarget(context.Background(), builder, bt, stin,
		buildPassOpts{stream: true}, time.Second, res, &buf)
	assert.False(t, res.Unstable)
	assert.Contains(t, buf.String(), "verify-live")
}
