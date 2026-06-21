package release

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jeduden/mdsmith/internal/config"
)

// TestBenchParityConfigSelectsConvention guards the no-drift link
// between the shared benchmark profile and a built-in parity
// convention. bench-parity.mdsmith.yml drives the single
// "mdsmith-parity" column and must select `convention: mado-parity`
// (mado is the representative mid-size peer) rather than hand-list the
// rules, so the benchmark and the convention (internal/convention) can
// never diverge. config.Load runs applyConvention, so a successful
// load also proves the name is a registered built-in convention.
func TestBenchParityConfigSelectsConvention(t *testing.T) {
	path := filepath.Join(repoRoot(t), benchDirRel, "bench-parity.mdsmith.yml")
	cfg, err := config.Load(path)
	require.NoError(t, err)

	assert.Equal(t, "mado-parity", cfg.Convention,
		"shared benchmark profile must select the mado-parity convention")
	// The profile must delegate the rule set to the convention, not
	// re-list rules itself — otherwise the two could drift.
	assert.Empty(t, cfg.Rules,
		"bench-parity.mdsmith.yml must not hand-list rules; the convention owns the set")
}

// TestPerLinterBenchConfigsSelectConventions checks that each
// per-linter benchmark profile selects its matching <linter>-parity
// convention and delegates the rule set to it, so a head-to-head run
// of mdsmith against any peer can never drift from that peer's
// convention.
func TestPerLinterBenchConfigsSelectConventions(t *testing.T) {
	for _, peer := range []string{"gomarklint", "mado", "rumdl", "markdownlint"} {
		t.Run(peer, func(t *testing.T) {
			path := filepath.Join(repoRoot(t), benchDirRel,
				"bench-"+peer+"-parity.mdsmith.yml")
			cfg, err := config.Load(path)
			require.NoError(t, err)

			assert.Equal(t, peer+"-parity", cfg.Convention,
				"profile must select the %s-parity convention", peer)
			assert.Empty(t, cfg.Rules,
				"profile must not hand-list rules; the convention owns the set")
		})
	}
}
