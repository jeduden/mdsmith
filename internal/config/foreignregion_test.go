package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParseForeignRegions parses a top-level foreign-regions: list of
// {start, end} marker pairs.
func TestParseForeignRegions(t *testing.T) {
	yml := `foreign-regions:
  - start: "<!-- apm:start -->"
    end: "<!-- apm:end -->"
`
	cfg, err := ParseBytes([]byte(yml))
	require.NoError(t, err)
	require.Len(t, cfg.ForeignRegions, 1)
	assert.Equal(t, "<!-- apm:start -->", cfg.ForeignRegions[0].Start)
	assert.Equal(t, "<!-- apm:end -->", cfg.ForeignRegions[0].End)
}

// TestParseForeignRegionsEmptyStartRejected rejects a marker pair with
// a blank start marker.
func TestParseForeignRegionsEmptyStartRejected(t *testing.T) {
	yml := `foreign-regions:
  - start: ""
    end: "<!-- apm:end -->"
`
	_, err := ParseBytes([]byte(yml))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "foreign-regions")
}

// TestParseForeignRegionsSameMarkerRejected rejects a pair whose start
// and end markers are identical (the scanner could never bound a
// region).
func TestParseForeignRegionsSameMarkerRejected(t *testing.T) {
	yml := `foreign-regions:
  - start: "<!-- x -->"
    end: "<!-- x -->"
`
	_, err := ParseBytes([]byte(yml))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "foreign-regions")
}

// TestEffectiveForeignRegionsGlobScoped resolves foreign-regions from
// the top-level list plus any matching overrides.
func TestEffectiveForeignRegionsGlobScoped(t *testing.T) {
	yml := `foreign-regions:
  - start: "<!-- apm:start -->"
    end: "<!-- apm:end -->"
overrides:
  - glob: ["AGENTS.md"]
    foreign-regions:
      - start: "<!-- gen:start -->"
        end: "<!-- gen:end -->"
`
	cfg, err := ParseBytes([]byte(yml))
	require.NoError(t, err)

	// A file outside the override glob sees only the top-level pair.
	base := EffectiveForeignRegions(cfg, "README.md")
	require.Len(t, base, 1)
	assert.Equal(t, "<!-- apm:start -->", base[0].Start)

	// A file matching the override glob sees both pairs.
	scoped := EffectiveForeignRegions(cfg, "AGENTS.md")
	require.Len(t, scoped, 2)
}
