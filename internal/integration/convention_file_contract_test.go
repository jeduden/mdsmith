package integration

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jeduden/mdsmith/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Contract tests for the `.mdsmith/conventions/` directory surface
// (plan 209), mirroring the kind-file contract tests. Each case locks
// one rule the public surface promises so the contract survives
// refactor pressure. Per docs/development/architecture/cross-system.md
// every public surface ships with a contract test.

// conventionFileContractFixture stages a workspace with a config file
// and optional convention files under `.mdsmith/conventions/`. Returns
// the path of the loaded `.mdsmith.yml`.
func conventionFileContractFixture(
	t *testing.T, configBody string, conventionFiles map[string]string,
) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, ".mdsmith.yml"), []byte(configBody), 0o644))
	if len(conventionFiles) > 0 {
		require.NoError(t, os.MkdirAll(
			filepath.Join(dir, ".mdsmith", "conventions"), 0o755))
		for name, body := range conventionFiles {
			require.NoError(t, os.WriteFile(
				filepath.Join(dir, ".mdsmith", "conventions", name),
				[]byte(body), 0o644))
		}
	}
	return filepath.Join(dir, ".mdsmith.yml")
}

// TestConventionFileContract_LayoutAndBasename locks the surface
// invariant: conventions live at
// `.mdsmith/conventions/<name>.{yaml,yml}`, the basename matches
// `[a-z][a-z0-9-]*`, and the convention loads into cfg.Conventions
// keyed by basename.
func TestConventionFileContract_LayoutAndBasename(t *testing.T) {
	cfgPath := conventionFileContractFixture(t, "rules: {}\n",
		map[string]string{
			"portable-strict.yaml": "flavor: commonmark\n" +
				"rules:\n  max-file-length:\n    max: 600\n",
		})
	cfg, err := config.Load(cfgPath)
	require.NoError(t, err)
	require.Contains(t, cfg.Conventions, "portable-strict",
		"basename minus extension must be the convention name")
	assert.Equal(t, "commonmark", cfg.Conventions["portable-strict"].Flavor)
	assert.Equal(t, 600,
		cfg.Conventions["portable-strict"].Rules["max-file-length"].Settings["max"])
}

// TestConventionFileContract_RejectsBadBasename locks the basename
// rule. A name that contains uppercase, underscore, or a leading digit
// must be rejected with the offending name in the error.
func TestConventionFileContract_RejectsBadBasename(t *testing.T) {
	cases := []string{"Portable_Strict.yaml", "1strict.yaml", "PortableStrict.yaml"}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			cfgPath := conventionFileContractFixture(t, "rules: {}\n",
				map[string]string{name: "flavor: commonmark\n"})
			_, err := config.Load(cfgPath)
			require.Error(t, err)
			assert.Contains(t, err.Error(), name)
		})
	}
}

// TestConventionFileContract_RejectsSubdirectory locks the
// one-convention-per-file rule. A nested file is a config error.
func TestConventionFileContract_RejectsSubdirectory(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, ".mdsmith.yml"), []byte("rules: {}\n"), 0o644))
	nested := filepath.Join(dir, ".mdsmith", "conventions", "more")
	require.NoError(t, os.MkdirAll(nested, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(nested, "foo.yaml"), []byte("flavor: commonmark\n"), 0o644))

	_, err := config.Load(filepath.Join(dir, ".mdsmith.yml"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "subdirector")
}

// TestConventionFileContract_RejectsDualSource locks the dual-source
// error. A name in both a file and inline must error naming both
// sources — the contract is "one file describes one convention".
func TestConventionFileContract_RejectsDualSource(t *testing.T) {
	cfgPath := conventionFileContractFixture(t, `
conventions:
  house:
    flavor: commonmark
    rules:
      line-length:
        max: 100
`, map[string]string{
		"house.yaml": "flavor: commonmark\nrules:\n  line-length:\n    max: 200\n",
	})
	_, err := config.Load(cfgPath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "house")
	assert.Contains(t, err.Error(), ".mdsmith.yml")
	assert.Contains(t, err.Error(), "house.yaml")
}

// TestConventionFileContract_RejectsBuiltinNameCollision locks the
// reserved-name rule unique to conventions: a file convention named
// after a built-in (portable, github, plain, …) is a config error.
// Built-ins stay compiled into the binary.
func TestConventionFileContract_RejectsBuiltinNameCollision(t *testing.T) {
	cfgPath := conventionFileContractFixture(t, "rules: {}\n",
		map[string]string{"github.yaml": "flavor: gfm\n"})
	_, err := config.Load(cfgPath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "github")
	assert.Contains(t, err.Error(), "reserved")
}

// TestConventionFileContract_RejectsUnknownTopLevelKey locks the
// "no extra top-level keys" rule. A typo in a top-level key must error
// naming both the key and the file.
func TestConventionFileContract_RejectsUnknownTopLevelKey(t *testing.T) {
	cfgPath := conventionFileContractFixture(t, "rules: {}\n",
		map[string]string{"house.yaml": "flavour: commonmark\n"})
	_, err := config.Load(cfgPath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "house.yaml")
	assert.Contains(t, err.Error(), "flavour")
}

// TestConventionFileContract_RejectsExtensionCollision locks the
// `.yaml`/`.yml` collision rule. Both filenames must surface.
func TestConventionFileContract_RejectsExtensionCollision(t *testing.T) {
	cfgPath := conventionFileContractFixture(t, "rules: {}\n",
		map[string]string{
			"house.yaml": "flavor: commonmark\n",
			"house.yml":  "flavor: gfm\n",
		})
	_, err := config.Load(cfgPath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "house.yaml")
	assert.Contains(t, err.Error(), "house.yml")
}

// TestConventionFileContract_AcceptsBothExtensions locks the fact that
// `.yaml` and `.yml` are both scanned. The contract reserves both
// filenames so projects that already settled on one need not migrate.
func TestConventionFileContract_AcceptsBothExtensions(t *testing.T) {
	cfgPath := conventionFileContractFixture(t, "rules: {}\n",
		map[string]string{
			"foo.yaml": "flavor: commonmark\n",
			"bar.yml":  "flavor: gfm\n",
		})
	cfg, err := config.Load(cfgPath)
	require.NoError(t, err)
	assert.Contains(t, cfg.Conventions, "foo")
	assert.Contains(t, cfg.Conventions, "bar")
}

// TestConventionFileContract_SourcePathPopulated locks the provenance
// contract: every convention — file or inline — has its SourcePath
// populated so audit consumers can attribute it to a defining file.
func TestConventionFileContract_SourcePathPopulated(t *testing.T) {
	cfgPath := conventionFileContractFixture(t, `
conventions:
  inline-conv:
    flavor: commonmark
    rules:
      line-length:
        max: 100
`, map[string]string{
		"file-conv.yaml": "flavor: commonmark\nrules:\n  line-length:\n    max: 200\n",
	})
	cfg, err := config.Load(cfgPath)
	require.NoError(t, err)

	require.Contains(t, cfg.Conventions, "inline-conv")
	assert.Equal(t, cfgPath, cfg.Conventions["inline-conv"].SourcePath,
		"inline convention must carry the .mdsmith.yml path")

	require.Contains(t, cfg.Conventions, "file-conv")
	assert.Equal(t,
		filepath.Join(filepath.Dir(cfgPath), ".mdsmith", "conventions", "file-conv.yaml"),
		cfg.Conventions["file-conv"].SourcePath,
		"file convention must carry its own path")
}

// TestConventionFileContract_SourcePathSurvivesMerge guards the bug
// where config.Merge dropped the convention SourcePath. The CLI loads
// config, merges it onto defaults, then resolves — so the path must
// survive that copy for `mdsmith kinds resolve` to print `defined-in`.
// This walks the same Load -> Merge -> ResolveFile path the CLI uses.
func TestConventionFileContract_SourcePathSurvivesMerge(t *testing.T) {
	cfgPath := conventionFileContractFixture(t, "convention: house\n",
		map[string]string{
			"house.yaml": "flavor: commonmark\nrules:\n  line-length:\n    max: 72\n",
		})
	loaded, err := config.Load(cfgPath)
	require.NoError(t, err)
	merged := config.Merge(config.Defaults(), loaded)

	res := config.ResolveFile(merged, "doc.md", nil, nil)
	require.Equal(t, "house", res.Convention.Name)
	assert.True(t, res.Convention.IsUser)
	assert.Equal(t,
		filepath.Join(filepath.Dir(cfgPath), ".mdsmith", "conventions", "house.yaml"),
		res.Convention.SourcePath,
		"convention SourcePath must survive Load -> Merge -> ResolveFile (the CLI path)")
}
