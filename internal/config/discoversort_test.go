package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

// writeFilesReverseOrder creates n YAML files under dir named so their
// creation order is the reverse of their filename sort order — the
// worst case for "did we accidentally rely on filesystem creation
// order instead of os.ReadDir's filename sort" — and returns the
// basenames in filename-sorted order.
func writeFilesReverseOrder(t *testing.T, dir string, n int, body string) []string {
	t.Helper()
	names := make([]string, n)
	for i := 0; i < n; i++ {
		names[i] = fmt.Sprintf("kind-%02d", i)
	}
	for i := n - 1; i >= 0; i-- {
		require.NoError(t, os.WriteFile(
			filepath.Join(dir, names[i]+".yaml"), []byte(body), 0o644))
	}
	return names
}

// TestDiscoverKinds_CollisionNamesSortedPair pins the behavior the
// removed sort.Slice in discoverKinds used to guarantee: a `.yaml`/
// `.yml` basename collision names the two colliding files in filename
// order in its error, regardless of filesystem creation order. Since
// os.ReadDir already returns entries "sorted by filename" (per its own
// doc), this must hold with no sort.Slice call in discoverKinds at all.
func TestDiscoverKinds_CollisionNamesSortedPair(t *testing.T) {
	dir := t.TempDir()
	kindsDir := filepath.Join(dir, ".mdsmith", "kinds")
	require.NoError(t, os.MkdirAll(kindsDir, 0o755))
	// Write the .yml (sorts after .yaml) first, so creation order is
	// the reverse of filename order.
	require.NoError(t, os.WriteFile(filepath.Join(kindsDir, "audit-log.yml"), []byte(`path-pattern: "*.md"`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(kindsDir, "audit-log.yaml"), []byte(`path-pattern: "*.md"`), 0o644))

	_, err := discoverKinds(dir)
	require.ErrorContains(t, err, "audit-log.yaml and audit-log.yml")
}

// TestDiscoverSchemas_ResultOrderIndependentOfCreationOrder covers the
// same removed-sort guarantee for discoverSchemas: many schema files
// created in reverse filename order still all load correctly.
func TestDiscoverSchemas_ResultOrderIndependentOfCreationOrder(t *testing.T) {
	dir := t.TempDir()
	schemasDir := filepath.Join(dir, ".mdsmith", "schemas")
	require.NoError(t, os.MkdirAll(schemasDir, 0o755))
	names := writeFilesReverseOrder(t, schemasDir, 10, "closed: false\n")

	got, err := discoverSchemas(dir)
	require.NoError(t, err)
	require.Len(t, got, len(names))
	for _, name := range names {
		require.Contains(t, got, name)
	}
}

// TestDiscoverConventions_CollisionNamesSortedPair mirrors
// TestDiscoverKinds_CollisionNamesSortedPair for discoverConventions.
func TestDiscoverConventions_CollisionNamesSortedPair(t *testing.T) {
	dir := t.TempDir()
	conventionsDir := filepath.Join(dir, ".mdsmith", "conventions")
	require.NoError(t, os.MkdirAll(conventionsDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(conventionsDir, "house.yml"), []byte("flavor: commonmark\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(conventionsDir, "house.yaml"), []byte("flavor: commonmark\n"), 0o644))

	_, err := discoverConventions(dir)
	require.ErrorContains(t, err, "house.yaml and house.yml")
}

// TestDiscoverWordlists_ResultOrderIndependentOfCreationOrder mirrors
// TestDiscoverSchemas_ResultOrderIndependentOfCreationOrder for
// discoverWordlists.
func TestDiscoverWordlists_ResultOrderIndependentOfCreationOrder(t *testing.T) {
	dir := t.TempDir()
	wordlistsDir := filepath.Join(dir, ".mdsmith", "wordlists")
	require.NoError(t, os.MkdirAll(wordlistsDir, 0o755))
	names := writeFilesReverseOrder(t, wordlistsDir, 10, "entries:\n  - foo\n")

	got, err := discoverWordlists(dir)
	require.NoError(t, err)
	require.Len(t, got, len(names))
	for _, name := range names {
		require.Contains(t, got, name)
	}
}

// TestDiscoverKinds_AllocBudget pins the allocation cost of
// discoverKinds on a directory with many kind files. The removed
// sort.Slice call drove reflect.Swapper on every comparison — pure
// overhead, since os.ReadDir already returns its entries sorted by
// filename — on top of the reflect cost, on every LSP config reload
// (workspace open, or a `.mdsmith/kinds/*.yaml` save) and every CLI
// config load.
func TestDiscoverKinds_AllocBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("alloc gate skipped in -short mode")
	}
	if raceEnabled {
		t.Skip("alloc gate skipped under -race")
	}
	dir := t.TempDir()
	kindsDir := filepath.Join(dir, ".mdsmith", "kinds")
	require.NoError(t, os.MkdirAll(kindsDir, 0o755))
	const n = 20
	for i := 0; i < n; i++ {
		require.NoError(t, os.WriteFile(
			filepath.Join(kindsDir, "kind-"+strconv.Itoa(i)+".yaml"),
			[]byte(`path-pattern: "*.md"`), 0o644))
	}

	_, err := discoverKinds(dir)
	require.NoError(t, err)

	const runs = 30
	allocs := testing.AllocsPerRun(runs, func() {
		_, err := discoverKinds(dir)
		require.NoError(t, err)
	})

	const allocBudget = 1799
	t.Logf("discoverKinds allocs/op (%d kind files) = %.0f (budget = %d)",
		n, allocs, allocBudget)
	require.LessOrEqualf(t, allocs, float64(allocBudget),
		"discoverKinds allocs/op = %.0f exceeds budget %d: the removed "+
			"sort.Slice must not come back — os.ReadDir already returns "+
			"entries sorted by filename",
		allocs, allocBudget)
}
