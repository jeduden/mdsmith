package integration

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jeduden/mdsmith/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Contract tests for the `.mdsmith/kinds/` directory surface
// (plan 208). Each case locks one rule the public surface
// promises so the contract survives refactor pressure. Per
// docs/development/architecture/cross-system.md every public
// surface ships with a contract test.

// kindFileContractFixture stages a workspace with a config file
// and an optional kind file under `.mdsmith/kinds/`. Returns the
// path of the loaded `.mdsmith.yml`.
func kindFileContractFixture(
	t *testing.T, configBody string, kindFiles map[string]string,
) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, ".mdsmith.yml"), []byte(configBody), 0o644))
	if len(kindFiles) > 0 {
		require.NoError(t, os.MkdirAll(
			filepath.Join(dir, ".mdsmith", "kinds"), 0o755))
		for name, body := range kindFiles {
			require.NoError(t, os.WriteFile(
				filepath.Join(dir, ".mdsmith", "kinds", name),
				[]byte(body), 0o644))
		}
	}
	return filepath.Join(dir, ".mdsmith.yml")
}

// TestKindFileContract_LayoutAndBasename locks the surface
// invariant: kinds live exactly at `.mdsmith/kinds/<name>.yaml`
// or `.yml`, basename matches `[a-z][a-z0-9-]*`, and the kind
// loads into cfg.Kinds keyed by basename.
func TestKindFileContract_LayoutAndBasename(t *testing.T) {
	cfgPath := kindFileContractFixture(t, "rules: {}\n", map[string]string{
		"audit-log.yaml": "rules:\n  max-file-length:\n    max: 600\n",
	})
	cfg, err := config.Load(cfgPath)
	require.NoError(t, err)
	require.Contains(t, cfg.Kinds, "audit-log",
		"basename minus extension must be the kind name")
	assert.Equal(t, 600,
		cfg.Kinds["audit-log"].Rules["max-file-length"].Settings["max"])
}

// TestKindFileContract_RejectsBadBasename locks the basename
// rule. A name that contains uppercase, underscore, or a leading
// digit must be rejected with the offending name in the error.
func TestKindFileContract_RejectsBadBasename(t *testing.T) {
	cases := []string{"Audit_Log.yaml", "1audit.yaml", "AuditLog.yaml"}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			cfgPath := kindFileContractFixture(t, "rules: {}\n",
				map[string]string{name: "rules: {}\n"})
			_, err := config.Load(cfgPath)
			require.Error(t, err)
			assert.Contains(t, err.Error(), name)
		})
	}
}

// TestKindFileContract_RejectsSubdirectory locks the
// one-kind-per-file rule. A nested file is a config error.
func TestKindFileContract_RejectsSubdirectory(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, ".mdsmith.yml"), []byte("rules: {}\n"), 0o644))
	nested := filepath.Join(dir, ".mdsmith", "kinds", "more")
	require.NoError(t, os.MkdirAll(nested, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(nested, "foo.yaml"), []byte("rules: {}\n"), 0o644))

	_, err := config.Load(filepath.Join(dir, ".mdsmith.yml"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "subdirector")
}

// TestKindFileContract_RejectsDualSource locks the dual-source
// error. A name in both a file and inline must error naming both
// sources — the contract is "one file describes one kind".
func TestKindFileContract_RejectsDualSource(t *testing.T) {
	cfgPath := kindFileContractFixture(t, `
kinds:
  plan:
    rules:
      line-length:
        max: 100
`, map[string]string{
		"plan.yaml": "rules:\n  line-length:\n    max: 200\n",
	})
	_, err := config.Load(cfgPath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "plan")
	assert.Contains(t, err.Error(), ".mdsmith.yml")
	assert.Contains(t, err.Error(), "plan.yaml")
}

// TestKindFileContract_RejectsUnknownTopLevelKey locks the
// "no extra top-level keys" rule. A typo in a top-level key
// must error naming both the key and the file.
func TestKindFileContract_RejectsUnknownTopLevelKey(t *testing.T) {
	cfgPath := kindFileContractFixture(t, "rules: {}\n", map[string]string{
		"plan.yaml": "rule: {}\n",
	})
	_, err := config.Load(cfgPath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "plan.yaml")
	assert.Contains(t, err.Error(), "rule")
}

// TestKindFileContract_RejectsExtensionCollision locks the
// `.yaml`/`.yml` collision rule. Both filenames must surface.
func TestKindFileContract_RejectsExtensionCollision(t *testing.T) {
	cfgPath := kindFileContractFixture(t, "rules: {}\n", map[string]string{
		"plan.yaml": "rules: {}\n",
		"plan.yml":  "rules: {}\n",
	})
	_, err := config.Load(cfgPath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "plan.yaml")
	assert.Contains(t, err.Error(), "plan.yml")
}

// TestKindFileContract_AcceptsBothExtensions locks the fact
// that `.yaml` and `.yml` are both scanned. The contract
// reserves both filenames so projects that already settled on
// one don't have to migrate.
func TestKindFileContract_AcceptsBothExtensions(t *testing.T) {
	cfgPath := kindFileContractFixture(t, "rules: {}\n", map[string]string{
		"foo.yaml": "rules: {}\n",
		"bar.yml":  "rules: {}\n",
	})
	cfg, err := config.Load(cfgPath)
	require.NoError(t, err)
	assert.Contains(t, cfg.Kinds, "foo")
	assert.Contains(t, cfg.Kinds, "bar")
}

// TestKindFileContract_SourcePathPopulated locks the
// provenance contract: every kind body — file or inline — has
// its SourcePath populated so audit consumers can attribute it
// to a defining file.
func TestKindFileContract_SourcePathPopulated(t *testing.T) {
	cfgPath := kindFileContractFixture(t, `
kinds:
  inline-kind:
    rules:
      line-length:
        max: 100
`, map[string]string{
		"file-kind.yaml": "rules:\n  line-length:\n    max: 200\n",
	})
	cfg, err := config.Load(cfgPath)
	require.NoError(t, err)

	require.Contains(t, cfg.Kinds, "inline-kind")
	assert.Equal(t, cfgPath, cfg.Kinds["inline-kind"].SourcePath,
		"inline kind body must carry the .mdsmith.yml path")

	require.Contains(t, cfg.Kinds, "file-kind")
	assert.Equal(t,
		filepath.Join(filepath.Dir(cfgPath), ".mdsmith", "kinds", "file-kind.yaml"),
		cfg.Kinds["file-kind"].SourcePath,
		"file kind body must carry its own path")
}
