package main_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// explainCfg sets line-length to 30 via a kind so that a too-long line
// in a kind-assigned file triggers a diagnostic with provenance.
const explainCfg = `kinds:
  short:
    rules:
      line-length:
        max: 30
kind-assignment:
  - files: ["short.md"]
    kinds: [short]
rules:
  line-length:
    max: 500
`

func TestExplain_TextTrailerNamesRuleAndSource(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".git"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, ".mdsmith.yml"), []byte(explainCfg), 0o644))
	long := "# Title\n\n" + strings.Repeat("x", 60) + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "short.md"), []byte(long), 0o644))

	_, stderr, code := runBinaryInDir(t, dir, "",
		"check", "--explain", "--no-color", "short.md")
	require.Equal(t, 1, code, "diagnostic expected; stderr=%q", stderr)
	// The trailer mentions the rule and the kinds.short source.
	assert.Contains(t, stderr, "line-length")
	assert.Contains(t, stderr, "kinds.short")
	// The trailer mentions the offending leaf.
	assert.Contains(t, stderr, "settings.max")
}

func TestExplain_JSONHasExplanationField(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".git"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, ".mdsmith.yml"), []byte(explainCfg), 0o644))
	long := "# Title\n\n" + strings.Repeat("x", 60) + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "short.md"), []byte(long), 0o644))

	_, stderr, code := runBinaryInDir(t, dir, "",
		"check", "--explain", "-f", "json", "short.md")
	require.Equal(t, 1, code)

	if strings.TrimSpace(stderr) == "" {
		t.Fatalf("expected JSON output on stderr, got empty")
	}

	var diags []struct {
		Rule        string `json:"rule"`
		Name        string `json:"name"`
		Explanation *struct {
			Rule   string `json:"rule"`
			Leaves []struct {
				Path   string `json:"path"`
				Value  any    `json:"value"`
				Source string `json:"source"`
			} `json:"leaves"`
		} `json:"explanation"`
	}
	require.NoError(t, json.Unmarshal([]byte(stderr), &diags))
	require.NotEmpty(t, diags)
	for _, d := range diags {
		if d.Name == "line-length" {
			require.NotNil(t, d.Explanation, "explanation must be populated")
			assert.Equal(t, "line-length", d.Explanation.Rule)
			var sawMax bool
			for _, l := range d.Explanation.Leaves {
				if l.Path == "settings.max" {
					sawMax = true
					assert.Equal(t, "kinds.short", l.Source)
					assert.EqualValues(t, 30, l.Value)
				}
			}
			assert.True(t, sawMax, "settings.max leaf must be present")
			return
		}
	}
	t.Fatalf("did not find a line-length diagnostic in %q", stderr)
}

func TestExplain_OmittedWhenFlagUnset(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".git"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, ".mdsmith.yml"), []byte(explainCfg), 0o644))
	long := "# Title\n\n" + strings.Repeat("x", 60) + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "short.md"), []byte(long), 0o644))

	_, stderr, code := runBinaryInDir(t, dir, "",
		"check", "--no-color", "short.md")
	require.Equal(t, 1, code)
	assert.NotContains(t, stderr, "kinds.short", "no provenance trailer without --explain")
}
