package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestCopyUserConventions_PreservesSourcePath guards the deep-copy
// used by Merge: a user convention's SourcePath must survive the copy
// so `mdsmith kinds resolve` can still name the defining file after
// the CLI merges loaded config onto defaults (plan 209). A regression
// here silently blanks the `defined-in` path in resolve output.
func TestCopyUserConventions_PreservesSourcePath(t *testing.T) {
	in := map[string]UserConvention{
		"house": {
			Flavor:     "commonmark",
			Rules:      map[string]RuleCfg{"line-length": {Enabled: true}},
			SourcePath: "/ws/.mdsmith/conventions/house.yaml",
		},
	}
	out := copyUserConventions(in)
	assert.Equal(t, "/ws/.mdsmith/conventions/house.yaml",
		out["house"].SourcePath)
	assert.Equal(t, "commonmark", out["house"].Flavor)
}
