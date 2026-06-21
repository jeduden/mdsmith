package integration

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/convention"
	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/jeduden/mdsmith/internal/rules"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// peerMappings selects a rule's mapping list for a given peer linter
// from its README front matter (surfaced by rules.ListRules).
func peerMappings(ri rules.RuleInfo, peer string) []rules.RuleMapping {
	switch peer {
	case "gomarklint":
		return ri.Gomarklint
	case "mado":
		return ri.Mado
	case "rumdl":
		return ri.Rumdl
	case "markdownlint":
		return ri.Markdownlint
	default:
		return nil
	}
}

// peerRunsByDefault reports whether the peer linter runs a check that
// covers this mdsmith rule by default — true when any of the rule's
// peer mappings carries default: true.
func peerRunsByDefault(ri rules.RuleInfo, peer string) bool {
	for _, m := range peerMappings(ri, peer) {
		if m.Default {
			return true
		}
	}
	return false
}

// mdsmithDefaultOn reports whether the registered rule is enabled by
// default in mdsmith. A rule with no Defaultable implementation is
// on by default.
func mdsmithDefaultOn(name string) bool {
	r := rule.ByName(name)
	if r == nil {
		return false
	}
	if d, ok := r.(rule.Defaultable); ok {
		return d.EnabledByDefault()
	}
	return true
}

// TestParityConventionsMatchCoverageMatrix is the source-of-truth gate
// for the <linter>-parity family. For each peer it derives the rule
// set the peer runs by default from the rule README front matter, then
// asserts the convention leaves mdsmith's effective rule set equal to
// that set — enabling every opt-in rule the peer runs and disabling
// every mdsmith default the peer skips. It fails CI when a convention
// drifts from the coverage matrix in either direction.
func TestParityConventionsMatchCoverageMatrix(t *testing.T) {
	infos, err := rules.ListRules()
	require.NoError(t, err)

	for _, peer := range []string{"gomarklint", "mado", "rumdl", "markdownlint"} {
		t.Run(peer, func(t *testing.T) {
			conv, err := convention.Lookup(peer+"-parity", nil)
			require.NoError(t, err)

			for _, ri := range infos {
				name := ri.Name
				want := peerRunsByDefault(ri, peer)

				effective := mdsmithDefaultOn(name)
				if p, ok := conv.Rules[name]; ok {
					effective = p.Enabled
				}

				assert.Equalf(t, want, effective,
					"%s-parity: rule %q effective=%v but peer-runs=%v",
					peer, name, effective, want)
			}

			// The convention must mention only rules that actually
			// deviate from the mdsmith default — no redundant entries.
			for name, p := range conv.Rules {
				if mdsmithDefaultOn(name) == p.Enabled {
					t.Errorf("%s-parity: rule %q is redundant "+
						"(matches mdsmith default %v)", peer, name, p.Enabled)
				}
			}
		})
	}
}
