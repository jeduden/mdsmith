package release

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jeduden/mdsmith/internal/config"
)

// TestBenchParityConfigSelectsConvention guards the no-drift link
// between the benchmark profile and the built-in parity convention.
// bench-parity.mdsmith.yml must select `convention: parity` rather
// than hand-list the disabled rules, so the benchmark and the
// convention (internal/convention) can never diverge. config.Load
// runs applyConvention, so a successful load also proves `parity` is
// a registered built-in convention; an unknown name would error here.
func TestBenchParityConfigSelectsConvention(t *testing.T) {
	path := filepath.Join(repoRoot(t), benchDirRel, "bench-parity.mdsmith.yml")
	cfg, err := config.Load(path)
	require.NoError(t, err)

	assert.Equal(t, "parity", cfg.Convention,
		"benchmark profile must select the built-in parity convention")
	// The profile must delegate the rule set to the convention, not
	// re-list rules itself — otherwise the two could drift.
	assert.Empty(t, cfg.Rules,
		"bench-parity.mdsmith.yml must not hand-list rules; the convention owns the set")
}
