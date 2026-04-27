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

// kindsTestDir creates a temp working directory seeded with a
// .mdsmith.yml of cfgYAML and any extra files keyed by relative path.
func kindsTestDir(t *testing.T, cfgYAML string, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".git"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, ".mdsmith.yml"), []byte(cfgYAML), 0o644))
	for rel, body := range files {
		full := filepath.Join(dir, rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
		require.NoError(t, os.WriteFile(full, []byte(body), 0o644))
	}
	return dir
}

const sampleKindsCfg = `kinds:
  plan:
    rules:
      max-file-length:
        max: 500
      paragraph-readability: false
  proto:
    rules:
      paragraph-readability: false
    categories:
      meta: false
kind-assignment:
  - files: ["plan/[0-9]*_*.md"]
    kinds: [plan]
  - files: ["**/proto.md"]
    kinds: [proto]
rules:
  max-file-length:
    max: 300
overrides:
  - files: ["plan/9_big.md"]
    rules:
      max-file-length:
        max: 900
`

func TestKinds_NoArgsShowsUsage(t *testing.T) {
	dir := kindsTestDir(t, sampleKindsCfg, nil)
	_, stderr, code := runBinaryInDir(t, dir, "", "kinds")
	require.Equal(t, 0, code)
	assert.Contains(t, stderr, "Subcommands:")
	assert.Contains(t, stderr, "resolve")
	assert.Contains(t, stderr, "why")
}

func TestKinds_UnknownSubcommand(t *testing.T) {
	dir := kindsTestDir(t, sampleKindsCfg, nil)
	_, stderr, code := runBinaryInDir(t, dir, "", "kinds", "nope")
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "unknown subcommand")
}

func TestKinds_ListPrintsAllSorted(t *testing.T) {
	dir := kindsTestDir(t, sampleKindsCfg, nil)
	stdout, _, code := runBinaryInDir(t, dir, "", "kinds", "list")
	require.Equal(t, 0, code)
	// Sorted alphabetically: plan, proto.
	planIdx := strings.Index(stdout, "plan:")
	protoIdx := strings.Index(stdout, "proto:")
	require.True(t, planIdx >= 0, "plan: must appear in output")
	require.True(t, protoIdx >= 0, "proto: must appear in output")
	assert.Less(t, planIdx, protoIdx)
	assert.Contains(t, stdout, "max-file-length")
	assert.Contains(t, stdout, "paragraph-readability")
}

func TestKinds_ListJSON(t *testing.T) {
	dir := kindsTestDir(t, sampleKindsCfg, nil)
	stdout, _, code := runBinaryInDir(t, dir, "", "kinds", "list", "--json")
	require.Equal(t, 0, code)
	var out struct {
		Kinds []struct {
			Name       string          `json:"name"`
			Rules      map[string]any  `json:"rules"`
			Categories map[string]bool `json:"categories,omitempty"`
		} `json:"kinds"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &out))
	require.Len(t, out.Kinds, 2)
	assert.Equal(t, "plan", out.Kinds[0].Name)
	assert.Equal(t, "proto", out.Kinds[1].Name)
	assert.False(t, out.Kinds[0].Rules["paragraph-readability"].(bool))
}

func TestKinds_ListEmptyConfigPrintsMessage(t *testing.T) {
	dir := kindsTestDir(t, "rules: {}\n", nil)
	_, stderr, code := runBinaryInDir(t, dir, "", "kinds", "list")
	require.Equal(t, 0, code)
	assert.Contains(t, stderr, "no kinds declared")
}

func TestKinds_ShowPrintsMergedBody(t *testing.T) {
	dir := kindsTestDir(t, sampleKindsCfg, nil)
	stdout, _, code := runBinaryInDir(t, dir, "", "kinds", "show", "plan")
	require.Equal(t, 0, code)
	assert.Contains(t, stdout, "plan:")
	assert.Contains(t, stdout, "max-file-length")
	assert.Contains(t, stdout, "max: 500")
}

func TestKinds_ShowJSON(t *testing.T) {
	dir := kindsTestDir(t, sampleKindsCfg, nil)
	stdout, _, code := runBinaryInDir(t, dir, "", "kinds", "show", "plan", "--json")
	require.Equal(t, 0, code)
	var out struct {
		Name  string         `json:"name"`
		Rules map[string]any `json:"rules"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &out))
	assert.Equal(t, "plan", out.Name)
}

func TestKinds_ShowUnknownExits2(t *testing.T) {
	dir := kindsTestDir(t, sampleKindsCfg, nil)
	_, stderr, code := runBinaryInDir(t, dir, "", "kinds", "show", "ghost")
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "unknown kind")
}

func TestKinds_ShowMissingNameExits2(t *testing.T) {
	dir := kindsTestDir(t, sampleKindsCfg, nil)
	_, stderr, code := runBinaryInDir(t, dir, "", "kinds", "show")
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "exactly one kind name")
}

func TestKinds_PathPrintsResolvedSchemaPath(t *testing.T) {
	cfg := `kinds:
  plan:
    rules:
      required-structure:
        schema: plan/proto.md
`
	dir := kindsTestDir(t, cfg, nil)
	stdout, _, code := runBinaryInDir(t, dir, "", "kinds", "path", "plan")
	require.Equal(t, 0, code)
	got := strings.TrimSpace(stdout)
	assert.True(t, strings.HasSuffix(got, filepath.Join("plan", "proto.md")),
		"path output %q should end with plan/proto.md", got)
}

func TestKinds_PathExits2WhenNoSchemaSet(t *testing.T) {
	cfg := `kinds:
  plan:
    rules:
      paragraph-readability: false
`
	dir := kindsTestDir(t, cfg, nil)
	_, stderr, code := runBinaryInDir(t, dir, "", "kinds", "path", "plan")
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "does not configure required-structure")
}

func TestKinds_PathExits2OnUnknownKind(t *testing.T) {
	dir := kindsTestDir(t, sampleKindsCfg, nil)
	_, stderr, code := runBinaryInDir(t, dir, "", "kinds", "path", "ghost")
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "unknown kind")
}

func TestKinds_ResolveTextShowsPerLeafProvenance(t *testing.T) {
	dir := kindsTestDir(t, sampleKindsCfg, map[string]string{
		"plan/9_big.md": "# Title\n",
	})
	stdout, _, code := runBinaryInDir(t, dir, "", "kinds", "resolve", "plan/9_big.md")
	require.Equal(t, 0, code)
	assert.Contains(t, stdout, "file: plan/9_big.md")
	assert.Contains(t, stdout, "plan (from kind-assignment[0])")
	assert.Contains(t, stdout, "max-file-length")
	// Override applies to plan/9_big.md, so the winning source for max is overrides[0].
	assert.Contains(t, stdout, "settings.max")
	assert.Contains(t, stdout, "(from overrides[0])")
}

func TestKinds_ResolveJSONHasKindsRulesAndLeaves(t *testing.T) {
	dir := kindsTestDir(t, sampleKindsCfg, map[string]string{
		"plan/95_kind-rule-resolution-cli.md": "# T\n",
	})
	stdout, _, code := runBinaryInDir(t, dir, "", "kinds",
		"resolve", "plan/95_kind-rule-resolution-cli.md", "--json")
	require.Equal(t, 0, code)
	var out struct {
		File  string `json:"file"`
		Kinds []struct {
			Name   string `json:"name"`
			Source string `json:"source"`
		} `json:"kinds"`
		Rules map[string]struct {
			Final  any `json:"final"`
			Leaves []struct {
				Path   string `json:"path"`
				Value  any    `json:"value"`
				Source string `json:"source"`
			} `json:"leaves"`
		} `json:"rules"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &out))
	assert.Equal(t, "plan/95_kind-rule-resolution-cli.md", out.File)
	require.Len(t, out.Kinds, 1)
	assert.Equal(t, "plan", out.Kinds[0].Name)
	assert.Equal(t, "kind-assignment[0]", out.Kinds[0].Source)

	mfl, ok := out.Rules["max-file-length"]
	require.True(t, ok)
	// settings.max should have source kinds.plan (since this file does not match overrides[0]).
	var sawMax bool
	for _, l := range mfl.Leaves {
		if l.Path == "settings.max" {
			sawMax = true
			assert.Equal(t, "kinds.plan", l.Source)
			assert.EqualValues(t, 500, l.Value)
		}
	}
	assert.True(t, sawMax, "settings.max leaf must be present")
}

func TestKinds_ResolveFromFrontMatter(t *testing.T) {
	cfg := `kinds:
  proto:
    rules:
      line-length:
        max: 120
rules:
  line-length:
    max: 80
`
	doc := "---\nkinds: [proto]\n---\n# T\n"
	dir := kindsTestDir(t, cfg, map[string]string{"doc.md": doc})
	stdout, _, code := runBinaryInDir(t, dir, "", "kinds", "resolve", "doc.md")
	require.Equal(t, 0, code)
	assert.Contains(t, stdout, "proto (from front-matter)")
}

func TestKinds_ResolveMissingFileArg(t *testing.T) {
	dir := kindsTestDir(t, sampleKindsCfg, nil)
	_, stderr, code := runBinaryInDir(t, dir, "", "kinds", "resolve")
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "exactly one file argument")
}

func TestKinds_WhyTextShowsMergeChain(t *testing.T) {
	dir := kindsTestDir(t, sampleKindsCfg, map[string]string{
		"plan/9_big.md": "# T\n",
	})
	stdout, _, code := runBinaryInDir(t, dir, "", "kinds", "why",
		"plan/9_big.md", "max-file-length")
	require.Equal(t, 0, code)
	assert.Contains(t, stdout, "rule: max-file-length")
	assert.Contains(t, stdout, "merge chain")
	assert.Contains(t, stdout, "default")
	assert.Contains(t, stdout, "kinds.plan")
	assert.Contains(t, stdout, "overrides[0]")
	assert.Contains(t, stdout, "winning source: overrides[0]")
}

func TestKinds_WhyJSON(t *testing.T) {
	dir := kindsTestDir(t, sampleKindsCfg, map[string]string{
		"plan/9_big.md": "# T\n",
	})
	stdout, _, code := runBinaryInDir(t, dir, "", "kinds", "why",
		"plan/9_big.md", "max-file-length", "--json")
	require.Equal(t, 0, code)
	var out struct {
		File   string `json:"file"`
		Rule   string `json:"rule"`
		Layers []struct {
			Source string `json:"source"`
			Set    bool   `json:"set"`
		} `json:"layers"`
		Leaves []struct {
			Path   string `json:"path"`
			Source string `json:"source"`
			Chain  []struct {
				Source string `json:"source"`
			} `json:"chain"`
		} `json:"leaves"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &out))
	assert.Equal(t, "max-file-length", out.Rule)
	require.Len(t, out.Layers, 3)
	assert.Equal(t, "default", out.Layers[0].Source)
	assert.Equal(t, "kinds.plan", out.Layers[1].Source)
	assert.Equal(t, "overrides[0]", out.Layers[2].Source)
}

func TestKinds_WhyUnknownRuleExits2(t *testing.T) {
	dir := kindsTestDir(t, sampleKindsCfg, map[string]string{
		"doc.md": "# T\n",
	})
	_, stderr, code := runBinaryInDir(t, dir, "", "kinds", "why",
		"doc.md", "no-such-rule")
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "not found in effective config")
}

func TestKinds_WhyMissingArgsExits2(t *testing.T) {
	dir := kindsTestDir(t, sampleKindsCfg, nil)
	_, stderr, code := runBinaryInDir(t, dir, "", "kinds", "why", "x")
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "<file> and <rule>")
}
