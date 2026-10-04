package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	buildexec "github.com/jeduden/mdsmith/internal/build"
)

// --- syncWriter ---

func TestSyncWriter_Write_SerializesBytes(t *testing.T) {
	var buf bytes.Buffer
	sw := &syncWriter{w: &buf}
	n, err := sw.Write([]byte("hello"))
	require.NoError(t, err)
	assert.Equal(t, 5, n)
	assert.Equal(t, "hello", buf.String())
}

func TestSyncWriter_ConcurrentWrites_NoDataRace(t *testing.T) {
	var buf strings.Builder
	// Wrap with a mutex on the builder to avoid data-racing the builder
	// (syncWriter protects the underlying writer, but strings.Builder
	// is not concurrency-safe itself — we use a bytes.Buffer instead).
	var rawBuf bytes.Buffer
	sw := &syncWriter{w: &rawBuf}
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = sw.Write([]byte("x"))
		}()
	}
	wg.Wait()
	_ = buf.String() // touch buf to satisfy vet
	assert.Equal(t, 50, rawBuf.Len())
}

// --- runConcurrent ---

func TestRunConcurrent_SingleTarget_Succeeds(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("touch not available on Windows")
	}
	root := t.TempDir()
	cfg := buildPassCfg("    mk:\n      command: touch {outputs}\n")

	md := buildPassDirective("mk", "out.txt")
	p := filepath.Join(root, "doc.md")
	require.NoError(t, os.WriteFile(p, []byte(md), 0o644))

	targets, errs := collectBuildTargets([]string{p}, root, "", 0)
	require.Empty(t, errs)
	require.Len(t, targets, 1)

	builder := buildexec.NewCustomBuilder(buildRecipeSpecs(cfg))
	cache := buildexec.NewCache()

	var outcomes []targetOutcome
	var mu sync.Mutex
	fold := func(_ buildTarget, o targetOutcome) {
		mu.Lock()
		defer mu.Unlock()
		outcomes = append(outcomes, o)
	}

	opts := buildPassOpts{jobs: 2, timeout: time.Second}
	var buf strings.Builder
	runConcurrent(builder, targets, cfg, opts, cache, time.Second, &buf, fold)

	assert.Len(t, outcomes, 1)
	assert.Equal(t, outcomeRebuilt, outcomes[0])
	assert.FileExists(t, filepath.Join(root, "out.txt"))
}

func TestRunConcurrent_MultipleTargets_AllRun(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("touch not available on Windows")
	}
	root := t.TempDir()
	cfg := buildPassCfg("    mk:\n      command: touch {outputs}\n")

	md1 := buildPassDirective("mk", "a.txt")
	md2 := buildPassDirective("mk", "b.txt")
	p1 := filepath.Join(root, "a_doc.md")
	p2 := filepath.Join(root, "b_doc.md")
	require.NoError(t, os.WriteFile(p1, []byte(md1), 0o644))
	require.NoError(t, os.WriteFile(p2, []byte(md2), 0o644))

	targets, errs := collectBuildTargets([]string{p1, p2}, root, "", 0)
	require.Empty(t, errs)
	require.Len(t, targets, 2)

	builder := buildexec.NewCustomBuilder(buildRecipeSpecs(cfg))
	cache := buildexec.NewCache()

	var outcomes []targetOutcome
	var mu sync.Mutex
	fold := func(_ buildTarget, o targetOutcome) {
		mu.Lock()
		defer mu.Unlock()
		outcomes = append(outcomes, o)
	}

	opts := buildPassOpts{jobs: 2, timeout: time.Second}
	var buf strings.Builder
	runConcurrent(builder, targets, cfg, opts, cache, time.Second, &buf, fold)

	assert.Len(t, outcomes, 2)
	for _, o := range outcomes {
		assert.Equal(t, outcomeRebuilt, o)
	}
	assert.FileExists(t, filepath.Join(root, "a.txt"))
	assert.FileExists(t, filepath.Join(root, "b.txt"))
}

func TestRunConcurrent_FailingTarget_OutcomeFailed(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("false not available on Windows")
	}
	root := t.TempDir()
	cfg := buildPassCfg("    boom:\n      command: false\n")

	md := buildPassDirective("boom", "out.txt")
	p := filepath.Join(root, "doc.md")
	require.NoError(t, os.WriteFile(p, []byte(md), 0o644))

	targets, errs := collectBuildTargets([]string{p}, root, "", 0)
	require.Empty(t, errs)
	require.Len(t, targets, 1)

	builder := buildexec.NewCustomBuilder(buildRecipeSpecs(cfg))
	cache := buildexec.NewCache()

	var outcomes []targetOutcome
	var mu sync.Mutex
	fold := func(_ buildTarget, o targetOutcome) {
		mu.Lock()
		defer mu.Unlock()
		outcomes = append(outcomes, o)
	}

	opts := buildPassOpts{jobs: 2, timeout: time.Second}
	var buf strings.Builder
	runConcurrent(builder, targets, cfg, opts, cache, time.Second, &buf, fold)

	assert.Len(t, outcomes, 1)
	assert.Equal(t, outcomeFailed, outcomes[0])
}

func TestRunConcurrent_FreshTarget_AppliesCacheEntry(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("touch not available on Windows")
	}
	root := t.TempDir()
	cfg := buildPassCfg("    mk:\n      command: touch {outputs}\n")

	md := buildPassDirective("mk", "out.txt")
	p := filepath.Join(root, "doc.md")
	require.NoError(t, os.WriteFile(p, []byte(md), 0o644))

	targets, errs := collectBuildTargets([]string{p}, root, "", 0)
	require.Empty(t, errs)

	builder := buildexec.NewCustomBuilder(buildRecipeSpecs(cfg))
	cache := buildexec.NewCache()

	// First run: builds the target.
	var outcomes1 []targetOutcome
	fold1 := func(_ buildTarget, o targetOutcome) { outcomes1 = append(outcomes1, o) }
	opts := buildPassOpts{jobs: 2, timeout: time.Second}
	var buf1 strings.Builder
	runConcurrent(builder, targets, cfg, opts, cache, time.Second, &buf1, fold1)
	require.Len(t, outcomes1, 1)
	require.Equal(t, outcomeRebuilt, outcomes1[0])

	// Second run with the same cache: target should be FRESH.
	var outcomes2 []targetOutcome
	fold2 := func(_ buildTarget, o targetOutcome) { outcomes2 = append(outcomes2, o) }
	var buf2 strings.Builder
	runConcurrent(builder, targets, cfg, opts, cache, time.Second, &buf2, fold2)
	require.Len(t, outcomes2, 1)
	assert.Equal(t, outcomeNeutral, outcomes2[0])
}

func TestRunConcurrent_MockBuilderError_OutcomeFailed(t *testing.T) {
	root := t.TempDir()
	cfg := buildPassCfg("    cp:\n      command: cp {inputs} {outputs}\n")

	md := buildPassDirective("cp", "out.txt")
	p := filepath.Join(root, "doc.md")
	require.NoError(t, os.WriteFile(p, []byte(md), 0o644))

	targets, errs := collectBuildTargets([]string{p}, root, "", 0)
	require.Empty(t, errs)

	// Mock builder always fails.
	mock := &mockBuilder{fn: func(_ context.Context, _ buildexec.Target) error {
		return assert.AnError
	}}
	cache := buildexec.NewCache()

	var outcomes []targetOutcome
	fold := func(_ buildTarget, o targetOutcome) { outcomes = append(outcomes, o) }

	opts := buildPassOpts{jobs: 1, timeout: time.Second}
	var buf strings.Builder
	runConcurrent(mock, targets, cfg, opts, cache, time.Second, &buf, fold)

	assert.Len(t, outcomes, 1)
	assert.Equal(t, outcomeFailed, outcomes[0])
}
