package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEffectiveForeignRegionsNilConfig returns nil for a nil config.
func TestEffectiveForeignRegionsNilConfig(t *testing.T) {
	assert.Nil(t, EffectiveForeignRegions(nil, "README.md"))
}

// TestEffectiveForeignRegionsSkipsEmptyOverride ignores an override that
// declares no foreign regions, even when its glob matches.
func TestEffectiveForeignRegionsSkipsEmptyOverride(t *testing.T) {
	cfg := &Config{
		ForeignRegions: []ForeignRegion{{Start: "<!-- a -->", End: "<!-- b -->"}},
		Overrides: []Override{
			{Glob: []string{"README.md"}}, // matches, but has no ForeignRegions
		},
	}
	got := EffectiveForeignRegions(cfg, "README.md")
	require.Len(t, got, 1)
	assert.Equal(t, "<!-- a -->", got[0].Start)
}

// TestParseForeignRegionsEmptyEndRejected rejects a marker pair with a
// blank end marker.
func TestParseForeignRegionsEmptyEndRejected(t *testing.T) {
	yml := `foreign-regions:
  - start: "<!-- apm:start -->"
    end: ""
`
	_, err := ParseBytes([]byte(yml))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "end marker must not be empty")
}

// TestParseForeignRegionsOverrideInvalidRejected surfaces a malformed
// marker pair declared on an override, not just the top-level list.
func TestParseForeignRegionsOverrideInvalidRejected(t *testing.T) {
	yml := `overrides:
  - glob: ["AGENTS.md"]
    foreign-regions:
      - start: ""
        end: "<!-- gen:end -->"
`
	_, err := ParseBytes([]byte(yml))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "start marker must not be empty")
}

// TestCopyForeignRegionsNil returns nil for a nil input.
func TestCopyForeignRegionsNil(t *testing.T) {
	assert.Nil(t, copyForeignRegions(nil))
}

// TestCopyForeignRegionsIsolatesBackingArray returns an independent copy
// whose mutation does not touch the source slice.
func TestCopyForeignRegionsIsolatesBackingArray(t *testing.T) {
	src := []ForeignRegion{{Start: "<!-- a -->", End: "<!-- b -->"}}
	out := copyForeignRegions(src)
	require.Len(t, out, 1)
	assert.Equal(t, src[0], out[0])
	out[0].Start = "mutated"
	assert.Equal(t, "<!-- a -->", src[0].Start, "copy must not alias the source")
}
