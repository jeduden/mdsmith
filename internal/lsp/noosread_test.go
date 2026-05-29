package lsp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestNoBareOSReadFile enforces the plan-215 boundary: file reads in
// internal/lsp go through the pkg/mdsmith.Workspace seam, not a direct
// os.ReadFile. The acceptance criterion is "no os.ReadFile survives
// outside pkg/mdsmith and cmd/". This guards against a regression that
// reintroduces a direct disk read here.
func TestNoBareOSReadFile(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") {
			continue
		}
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		data, err := os.ReadFile(filepath.Clean(name))
		if err != nil {
			t.Fatalf("ReadFile %s: %v", name, err)
		}
		if strings.Contains(string(data), "os.ReadFile") {
			t.Errorf("%s contains a bare os.ReadFile; read through the pkg/mdsmith.Workspace seam instead", name)
		}
	}
}
