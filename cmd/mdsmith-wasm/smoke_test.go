package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	mdsmith "github.com/jeduden/mdsmith/pkg/mdsmith"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The fixture the Node harness (testdata/smoke.cjs) and this test both
// lint. Kept in sync by hand: a host index.md with an out-of-date
// catalog over two docs in the in-memory workspace, so MDS019 fires —
// exercising a cross-file rule reading through the workspace.
var (
	smokeWorkspace = map[string][]byte{
		"docs/one.md": []byte("---\nsummary: First doc\n---\n# One\n\nBody paragraph one here.\n"),
		"docs/two.md": []byte("---\nsummary: Second doc\n---\n# Two\n\nBody paragraph two here.\n"),
	}
	smokeIndexSrc = []byte("# Index\n\n<?catalog\nglob:\n  - \"docs/*.md\"\n" +
		"row: \"- [{summary}](docs/{filename})\"\n?>\n<?/catalog?>\n")
)

// smokeDiag is the comparable projection of a diagnostic the harness
// emits and this test computes from the native engine.
type smokeDiag struct {
	Rule    string `json:"rule"`
	Line    int    `json:"line"`
	Column  int    `json:"column"`
	Message string `json:"message"`
}

type smokeOutput struct {
	Version      string      `json:"version"`
	Capabilities []string    `json:"capabilities"`
	Diagnostics  []smokeDiag `json:"diagnostics"`
}

// TestWASMCheckMatchesNative builds the WASM artifact, runs the Node
// harness against it, and asserts the diagnostics and capability list
// it returns equal what the native engine produces on the identical
// in-memory fixture. This is the plan-215 smoke test: WASM check ==
// native on an in-memory fixture.
//
// It skips when Node or the WASM toolchain is unavailable rather than
// failing, so the suite stays green on hosts without them; CI runs it
// where Node is installed.
func TestWASMCheckMatchesNative(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not on PATH; skipping WASM smoke test")
	}

	wasmExec := wasmExecPath(t)
	if _, err := os.Stat(wasmExec); err != nil {
		t.Skipf("wasm_exec.js not found at %s; skipping", wasmExec)
	}

	wasmPath := buildWASM(t)
	harness := filepath.Join("testdata", "smoke.cjs")

	out, err := exec.Command(node, harness, wasmExec, wasmPath).CombinedOutput()
	if err != nil {
		t.Fatalf("node harness failed: %v\n%s", err, out)
	}

	got := parseSmokeOutput(t, out)
	want := nativeSmoke(t)

	if !equalStringSlices(got.Capabilities, want.Capabilities) {
		t.Errorf("capabilities mismatch:\n wasm: %v\n native: %v", got.Capabilities, want.Capabilities)
	}
	if !equalSmokeDiags(got.Diagnostics, want.Diagnostics) {
		t.Errorf("diagnostics mismatch:\n wasm: %+v\n native: %+v", got.Diagnostics, want.Diagnostics)
	}
	if got.Version == "" {
		t.Error("wasm reported empty version")
	}
}

// TestWASMFinalizerAfterExit runs the Node harness testdata/after_exit.cjs:
// once the Go program has exited, a dropped session that JS collects must
// raise no uncaught error in the host. Plan 2610030846.
func TestWASMFinalizerAfterExit(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not on PATH; skipping WASM after-exit test")
	}
	wasmExec := wasmExecPath(t)
	if _, err := os.Stat(wasmExec); err != nil {
		t.Skipf("wasm_exec.js not found at %s; skipping", wasmExec)
	}
	harness := filepath.Join("testdata", "after_exit.cjs")
	out, err := exec.Command(node, harness, wasmExec, buildWASM(t)).CombinedOutput()
	if err != nil {
		t.Fatalf("node harness failed: %v\n%s", err, out)
	}
}

// wasmExecPath locates Go's wasm_exec.js under the active toolchain's
// GOROOT via `go env GOROOT` (runtime.GOROOT is deprecated since Go
// 1.24). The file moved to lib/wasm/ in recent Go releases.
func wasmExecPath(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("go", "env", "GOROOT").Output()
	if err != nil {
		t.Skipf("go env GOROOT failed: %v", err)
	}
	root := strings.TrimSpace(string(out))
	return filepath.Join(root, "lib", "wasm", "wasm_exec.js")
}

// shippingBuild is the one stripped artifact every test in this binary
// shares, built on first use: the size budget and both Node harnesses
// test the same bytes, and the 13 MiB GOOS=js link runs once per
// `go test` instead of once per test.
var shippingBuild struct {
	once sync.Once
	dir  string
	path string
	err  error
}

const (
	buildDirPrefix = "mdsmith-wasm-test-"
	// staleBuildDirAge is how old a build directory must be before a
	// later run treats it as left by a killed run, not a concurrent one.
	staleBuildDirAge = time.Hour
)

// sweepStaleBuildDirs removes build directories in tmp older than age.
// A panic or a `go test -timeout` kill skips TestMain's cleanup, so the
// next run reclaims what that one left.
func sweepStaleBuildDirs(tmp string, age time.Duration) {
	entries, err := os.ReadDir(tmp)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), buildDirPrefix) {
			continue
		}
		if info, err := e.Info(); err == nil && time.Since(info.ModTime()) > age {
			_ = os.RemoveAll(filepath.Join(tmp, e.Name()))
		}
	}
}

// TestMain removes the shared artifact's directory once every test has
// run; a temp dir of one test would vanish before the next. It first
// sweeps directories that earlier killed runs left behind.
func TestMain(m *testing.M) {
	sweepStaleBuildDirs(os.TempDir(), staleBuildDirAge)
	code := m.Run()
	if shippingBuild.dir != "" {
		_ = os.RemoveAll(shippingBuild.dir)
	}
	os.Exit(code)
}

// buildWASM compiles cmd/mdsmith-wasm for GOOS=js GOARCH=wasm with the
// same -trimpath -ldflags="-s -w" flags as build.sh's `go` target and
// returns the artifact's path. It builds once per test binary and
// hands every caller the same file, which a caller must not modify. A
// build failure fails each caller (the artifact must compile); a
// missing wasm target is not expected on a standard Go toolchain.
func buildWASM(t *testing.T) string {
	t.Helper()
	b := &shippingBuild
	b.once.Do(func() {
		if b.dir, b.err = os.MkdirTemp("", buildDirPrefix); b.err != nil {
			return
		}
		b.path = filepath.Join(b.dir, "mdsmith.wasm")
		// -trimpath for reproducibility; -ldflags="-s -w" strips the
		// symbol table and DWARF.
		cmd := exec.Command("go", "build", "-trimpath", "-ldflags=-s -w", "-o", b.path, ".")
		cmd.Env = append(os.Environ(), "GOOS=js", "GOARCH=wasm")
		if out, err := cmd.CombinedOutput(); err != nil {
			b.err = fmt.Errorf("%w\n%s", err, out)
		}
	})
	if b.err != nil {
		t.Fatalf("building wasm artifact: %v", b.err)
	}
	return b.path
}

// nativeSmoke runs the native engine on the same fixture and projects
// the result into the comparable smokeOutput shape.
func nativeSmoke(t *testing.T) smokeOutput {
	t.Helper()
	s, err := mdsmith.NewSession(mdsmith.SessionOptions{
		Workspace: mdsmith.NewMemWorkspace(smokeWorkspace),
		Config:    mdsmith.ConfigYAML(""),
	})
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	defer s.Dispose()

	diags, err := s.Check("index.md", smokeIndexSrc)
	if err != nil {
		t.Fatalf("native Check: %v", err)
	}
	out := smokeOutput{Capabilities: s.Capabilities()}
	for _, d := range diags {
		out.Diagnostics = append(out.Diagnostics, smokeDiag{
			Rule: d.Rule, Line: d.Line, Column: d.Column, Message: d.Message,
		})
	}
	sortSmoke(out.Capabilities, out.Diagnostics)
	return out
}

func parseSmokeOutput(t *testing.T, b []byte) smokeOutput {
	t.Helper()
	var out smokeOutput
	if err := json.Unmarshal(lastJSONLine(b), &out); err != nil {
		t.Fatalf("parsing harness output: %v\nraw: %s", err, b)
	}
	sortSmoke(out.Capabilities, out.Diagnostics)
	return out
}

// lastJSONLine returns the last non-empty line of b, which is the
// harness's JSON payload (any preceding lines would be runtime noise).
func lastJSONLine(b []byte) []byte {
	start := len(b)
	for start > 0 && (b[start-1] == '\n' || b[start-1] == '\r') {
		start--
	}
	end := start
	for start > 0 && b[start-1] != '\n' {
		start--
	}
	return b[start:end]
}

func sortSmoke(caps []string, diags []smokeDiag) {
	sort.Strings(caps)
	sort.Slice(diags, func(i, j int) bool {
		if diags[i].Line != diags[j].Line {
			return diags[i].Line < diags[j].Line
		}
		if diags[i].Column != diags[j].Column {
			return diags[i].Column < diags[j].Column
		}
		return diags[i].Rule < diags[j].Rule
	})
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func equalSmokeDiags(a, b []smokeDiag) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestSweepStaleBuildDirs checks that a directory a killed run left
// behind is reclaimed, while a fresh one (a concurrent run's) and an
// unrelated one survive.
func TestSweepStaleBuildDirs(t *testing.T) {
	tmp := t.TempDir()
	old := time.Now().Add(-2 * time.Hour)
	for name, mtime := range map[string]time.Time{
		"mdsmith-wasm-test-old":   old,
		"mdsmith-wasm-test-fresh": time.Now(),
		"unrelated-old":           old,
	} {
		p := filepath.Join(tmp, name)
		require.NoError(t, os.Mkdir(p, 0o755))
		require.NoError(t, os.Chtimes(p, mtime, mtime))
	}

	sweepStaleBuildDirs(tmp, time.Hour)

	assert.NoDirExists(t, filepath.Join(tmp, "mdsmith-wasm-test-old"))
	assert.DirExists(t, filepath.Join(tmp, "mdsmith-wasm-test-fresh"))
	assert.DirExists(t, filepath.Join(tmp, "unrelated-old"))
}
