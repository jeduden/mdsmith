package main_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// archetypeTestDir creates a temp working directory seeded with a
// .mdsmith.yml whose archetypes.roots points at the given roots.
// Each (root, name) entry in archetypes seeds an archetype file.
func archetypeTestDir(
	t *testing.T, roots []string, archetypes map[[2]string]string,
) string {
	t.Helper()
	dir := t.TempDir()

	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".git"), 0o755))

	cfg := "archetypes:\n  roots:\n"
	for _, r := range roots {
		cfg += "    - " + r + "\n"
	}
	cfg += "rules: {}\n"
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, ".mdsmith.yml"), []byte(cfg), 0o644))

	for k, body := range archetypes {
		root, name := k[0], k[1]
		rootPath := filepath.Join(dir, root)
		require.NoError(t, os.MkdirAll(rootPath, 0o755))
		require.NoError(t, os.WriteFile(
			filepath.Join(rootPath, name+".md"), []byte(body), 0o644))
	}
	return dir
}

func TestArchetypes_ListPrintsAllSortedByName(t *testing.T) {
	dir := archetypeTestDir(t, []string{"archetypes"}, map[[2]string]string{
		{"archetypes", "story"}: "# ?\n",
		{"archetypes", "prd"}:   "# ?\n",
	})
	stdout, _, code := runBinaryInDir(t, dir, "", "archetypes", "list")
	require.Equal(t, 0, code)
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	require.Len(t, lines, 2)
	assert.True(t, strings.HasPrefix(lines[0], "prd\t"))
	assert.True(t, strings.HasPrefix(lines[1], "story\t"))
	assert.Contains(t, lines[1], "archetypes/story.md")
}

func TestArchetypes_ListExitsNonZeroWhenEmpty(t *testing.T) {
	dir := archetypeTestDir(t, []string{"archetypes"}, nil)
	_, stderr, code := runBinaryInDir(t, dir, "", "archetypes", "list")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "no archetypes found")
}

func TestArchetypes_ShowPrintsSchemaSource(t *testing.T) {
	dir := archetypeTestDir(t, []string{"archetypes"}, map[[2]string]string{
		{"archetypes", "story"}: "# ?\n\n## Body\n",
	})
	stdout, _, code := runBinaryInDir(t, dir, "", "archetypes", "show", "story")
	require.Equal(t, 0, code)
	assert.Contains(t, stdout, "## Body")
}

func TestArchetypes_ShowUnknownNameErrors(t *testing.T) {
	dir := archetypeTestDir(t, []string{"archetypes"}, map[[2]string]string{
		{"archetypes", "story"}: "# ?\n",
	})
	_, stderr, code := runBinaryInDir(t, dir, "", "archetypes", "show", "missing")
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "unknown archetype")
	assert.Contains(t, stderr, "story")
}

func TestArchetypes_PathPrintsResolvedFilesystemPath(t *testing.T) {
	dir := archetypeTestDir(t, []string{"archetypes"}, map[[2]string]string{
		{"archetypes", "story"}: "# ?\n",
	})
	stdout, _, code := runBinaryInDir(t, dir, "", "archetypes", "path", "story")
	require.Equal(t, 0, code)
	got := strings.TrimSpace(stdout)
	// Path should be absolute or rooted under the working dir.
	assert.True(t,
		strings.HasSuffix(got, filepath.Join("archetypes", "story.md")),
		"got %q", got)
}

func TestArchetypes_PathUnknownNameErrors(t *testing.T) {
	dir := archetypeTestDir(t, []string{"archetypes"}, nil)
	_, stderr, code := runBinaryInDir(t, dir, "", "archetypes", "path", "story")
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "unknown archetype")
}

func TestArchetypes_InitScaffoldsDirectoryAndExample(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".git"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, ".mdsmith.yml"), []byte("rules: {}\n"), 0o644))

	_, stderr, code := runBinaryInDir(t, dir, "", "archetypes", "init")
	require.Equal(t, 0, code)
	assert.Contains(t, stderr, "archetypes directory ready")
	assert.Contains(t, stderr, "archetypes:")

	// Example and README should exist.
	exampleBody, err := os.ReadFile(filepath.Join(dir, "archetypes", "example.md"))
	require.NoError(t, err)
	assert.Contains(t, string(exampleBody), "# ?")

	readmeBody, err := os.ReadFile(filepath.Join(dir, "archetypes", "README.md"))
	require.NoError(t, err)
	assert.Contains(t, string(readmeBody), "archetypes list")
}

func TestArchetypes_InitKeepsExistingExample(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".git"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, ".mdsmith.yml"), []byte("rules: {}\n"), 0o644))

	archDir := filepath.Join(dir, "archetypes")
	require.NoError(t, os.MkdirAll(archDir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(archDir, "example.md"), []byte("preserved\n"), 0o644))

	_, stderr, code := runBinaryInDir(t, dir, "", "archetypes", "init")
	require.Equal(t, 0, code)
	assert.Contains(t, stderr, "kept    ")

	body, err := os.ReadFile(filepath.Join(archDir, "example.md"))
	require.NoError(t, err)
	assert.Equal(t, "preserved\n", string(body))
}

func TestArchetypes_InitCustomDir(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".git"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, ".mdsmith.yml"), []byte("rules: {}\n"), 0o644))

	_, _, code := runBinaryInDir(t, dir, "", "archetypes", "init", "templates")
	require.Equal(t, 0, code)

	_, err := os.Stat(filepath.Join(dir, "templates", "example.md"))
	assert.NoError(t, err)
}

func TestArchetypes_UnknownSubcommand(t *testing.T) {
	dir := archetypeTestDir(t, []string{"archetypes"}, nil)
	_, stderr, code := runBinaryInDir(t, dir, "", "archetypes", "nope")
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "unknown subcommand")
}

func TestArchetypes_NoArgsShowsUsage(t *testing.T) {
	dir := archetypeTestDir(t, []string{"archetypes"}, nil)
	_, stderr, code := runBinaryInDir(t, dir, "", "archetypes")
	require.Equal(t, 0, code)
	assert.Contains(t, stderr, "Subcommands:")
}

func TestArchetypes_RuleResolvesFromConfiguredRoot(t *testing.T) {
	dir := archetypeTestDir(t, []string{"tmpl"}, map[[2]string]string{
		{"tmpl", "story"}: "# ?\n\n## Summary\n\n## ...\n",
	})
	// Add a document that satisfies the schema and an override applying
	// the story archetype to it.
	overrideCfg := `archetypes:
  roots:
    - tmpl
rules:
  required-structure: true
overrides:
  - files: ["doc.md"]
    rules:
      required-structure:
        archetype: story
`
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, ".mdsmith.yml"), []byte(overrideCfg), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "doc.md"),
		[]byte("# Title\n\n## Summary\n\nbody\n"), 0o644))

	_, _, code := runBinaryInDir(t, dir, "", "check", "doc.md")
	assert.Equal(t, 0, code)
}
