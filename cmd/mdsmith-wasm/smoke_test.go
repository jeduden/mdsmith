package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	mdsmith "github.com/jeduden/mdsmith/pkg/mdsmith"
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

// buildWASM compiles cmd/mdsmith-wasm for GOOS=js GOARCH=wasm into a
// temp file and returns its path. A build failure fails the test (the
// artifact must compile); a missing wasm target is not expected on a
// standard Go toolchain.
func buildWASM(t *testing.T) string {
	t.Helper()
	out := filepath.Join(t.TempDir(), "mdsmith.wasm")
	cmd := exec.Command("go", "build", "-o", out, ".")
	cmd.Env = append(os.Environ(), "GOOS=js", "GOARCH=wasm")
	if b, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("building wasm artifact: %v\n%s", err, b)
	}
	return out
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
