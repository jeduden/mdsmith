package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- runKinds dispatch ---

func TestRunKinds_NoArgs_PrintsUsage(t *testing.T) {
	out := captureStderr(func() {
		code := runKinds(nil)
		assert.Equal(t, 0, code)
	})
	assert.Contains(t, out, "kinds")
	assert.Contains(t, out, "list")
	assert.Contains(t, out, "show")
	assert.Contains(t, out, "path")
	assert.Contains(t, out, "resolve")
	assert.Contains(t, out, "why")
}

func TestRunKinds_HelpFlag_ExitsZero(t *testing.T) {
	captureStderr(func() {
		code := runKinds([]string{"--help"})
		assert.Equal(t, 0, code)
	})
	captureStderr(func() {
		code := runKinds([]string{"-h"})
		assert.Equal(t, 0, code)
	})
}

func TestRunKinds_UnknownSubcommand_ExitsTwo(t *testing.T) {
	got := captureStderr(func() {
		code := runKinds([]string{"unknown"})
		assert.Equal(t, 2, code)
	})
	assert.Contains(t, got, "unknown subcommand")
}

// --- runKindsList ---

func writeYAMLConfig(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, ".mdsmith.yml")
	require.NoError(t, os.WriteFile(cfgPath, []byte(body), 0o644))
	t.Chdir(dir)
	return dir
}

func TestRunKindsList_TextOutput(t *testing.T) {
	writeYAMLConfig(t, `
kinds:
  plan:
    rules:
      line-length: false
  proto:
    rules:
      paragraph-readability: false
`)
	out := captureStdout(func() {
		code := runKindsList(nil)
		assert.Equal(t, 0, code)
	})
	assert.Contains(t, out, "plan")
	assert.Contains(t, out, "proto")
}

func TestRunKindsList_JSON(t *testing.T) {
	writeYAMLConfig(t, `
kinds:
  plan:
    rules:
      line-length: false
`)
	out := captureStdout(func() {
		code := runKindsList([]string{"--json"})
		assert.Equal(t, 0, code)
	})
	var got map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &got))
	assert.Contains(t, got, "kinds")
}

func TestRunKindsList_NoKindsConfigured(t *testing.T) {
	writeYAMLConfig(t, "rules:\n  line-length: true\n")
	out := captureStdout(func() {
		code := runKindsList(nil)
		assert.Equal(t, 0, code)
	})
	// Empty list is acceptable; output may be empty or a header.
	_ = out
}

// --- runKindsShow ---

func TestRunKindsShow_ExistingKind(t *testing.T) {
	writeYAMLConfig(t, `
kinds:
  plan:
    rules:
      line-length: false
`)
	out := captureStdout(func() {
		code := runKindsShow([]string{"plan"})
		assert.Equal(t, 0, code)
	})
	assert.Contains(t, out, "line-length")
}

func TestRunKindsShow_UnknownKind_ExitsTwo(t *testing.T) {
	writeYAMLConfig(t, `
kinds:
  plan:
    rules:
      line-length: false
`)
	got := captureStderr(func() {
		code := runKindsShow([]string{"ghost"})
		assert.Equal(t, 2, code)
	})
	assert.Contains(t, got, "ghost")
}

func TestRunKindsShow_NoArgs_ExitsTwo(t *testing.T) {
	captureStderr(func() {
		code := runKindsShow(nil)
		assert.Equal(t, 2, code)
	})
}

// --- runKindsPath ---

func TestRunKindsPath_KindWithSchema(t *testing.T) {
	writeYAMLConfig(t, `
kinds:
  plan:
    rules:
      required-structure:
        schema: plan/proto.md
`)
	out := captureStdout(func() {
		code := runKindsPath([]string{"plan"})
		assert.Equal(t, 0, code)
	})
	assert.Contains(t, out, "plan/proto.md")
}

func TestRunKindsPath_KindWithoutSchema_ExitsTwo(t *testing.T) {
	writeYAMLConfig(t, `
kinds:
  plan:
    rules:
      line-length: false
`)
	got := captureStderr(func() {
		code := runKindsPath([]string{"plan"})
		assert.Equal(t, 2, code)
	})
	assert.Contains(t, strings.ToLower(got), "schema")
}

func TestRunKindsPath_UnknownKind_ExitsTwo(t *testing.T) {
	writeYAMLConfig(t, `
kinds:
  plan:
    rules:
      line-length: false
`)
	captureStderr(func() {
		code := runKindsPath([]string{"ghost"})
		assert.Equal(t, 2, code)
	})
}

// --- runKindsResolve ---

func TestRunKindsResolve_PrintsKindsAndWinningSources(t *testing.T) {
	dir := writeYAMLConfig(t, `
kinds:
  wide:
    rules:
      line-length:
        max: 200
kind-assignment:
  - files: ["wide/*.md"]
    kinds: [wide]
`)
	mdDir := filepath.Join(dir, "wide")
	require.NoError(t, os.MkdirAll(mdDir, 0o755))
	target := filepath.Join(mdDir, "doc.md")
	require.NoError(t, os.WriteFile(target, []byte("# Title\n"), 0o644))

	out := captureStdout(func() {
		code := runKindsResolve([]string{"wide/doc.md"})
		assert.Equal(t, 0, code)
	})
	assert.Contains(t, out, "wide")
	assert.Contains(t, out, "line-length")
	// Provenance label.
	assert.Contains(t, out, "kinds.wide")
}

func TestRunKindsResolve_JSON_StableSchema(t *testing.T) {
	dir := writeYAMLConfig(t, `
kinds:
  wide:
    rules:
      line-length:
        max: 200
kind-assignment:
  - files: ["wide/*.md"]
    kinds: [wide]
`)
	mdDir := filepath.Join(dir, "wide")
	require.NoError(t, os.MkdirAll(mdDir, 0o755))
	target := filepath.Join(mdDir, "doc.md")
	require.NoError(t, os.WriteFile(target, []byte("# Title\n"), 0o644))

	out := captureStdout(func() {
		code := runKindsResolve([]string{"--json", "wide/doc.md"})
		assert.Equal(t, 0, code)
	})
	var got map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &got))
	assert.Contains(t, got, "file")
	assert.Contains(t, got, "kinds")
	assert.Contains(t, got, "rules")
}

// --- runKindsWhy ---

func TestRunKindsWhy_PrintsFullChain(t *testing.T) {
	dir := writeYAMLConfig(t, `
rules:
  line-length:
    max: 80
kinds:
  plan:
    rules:
      paragraph-readability: false
  wide:
    rules:
      line-length:
        max: 200
kind-assignment:
  - files: ["*.md"]
    kinds: [plan, wide]
`)
	target := filepath.Join(dir, "doc.md")
	require.NoError(t, os.WriteFile(target, []byte("# Title\n"), 0o644))

	out := captureStdout(func() {
		code := runKindsWhy([]string{"doc.md", "line-length"})
		assert.Equal(t, 0, code)
	})
	assert.Contains(t, out, "default")
	assert.Contains(t, out, "kinds.plan")
	assert.Contains(t, out, "kinds.wide")
}

func TestRunKindsWhy_JSON(t *testing.T) {
	dir := writeYAMLConfig(t, `
rules:
  line-length:
    max: 80
`)
	target := filepath.Join(dir, "doc.md")
	require.NoError(t, os.WriteFile(target, []byte("# Title\n"), 0o644))

	out := captureStdout(func() {
		code := runKindsWhy([]string{"--json", "doc.md", "line-length"})
		assert.Equal(t, 0, code)
	})
	var got map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &got))
	assert.Equal(t, "line-length", got["rule"])
	assert.Contains(t, got, "chain")
}

func TestRunKindsWhy_NoArgs_ExitsTwo(t *testing.T) {
	captureStderr(func() {
		code := runKindsWhy(nil)
		assert.Equal(t, 2, code)
	})
}

// --- help topic ---

func TestRunHelpKindsCLI_PrintsSummary(t *testing.T) {
	out := captureStdout(func() {
		code := runHelpKindsCLI()
		assert.Equal(t, 0, code)
	})
	assert.Contains(t, out, "kinds list")
	assert.Contains(t, out, "kinds show")
	assert.Contains(t, out, "kinds path")
	assert.Contains(t, out, "kinds resolve")
	assert.Contains(t, out, "kinds why")
	assert.Contains(t, out, "--explain")
}
