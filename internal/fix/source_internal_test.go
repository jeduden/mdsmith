package fix

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/rule"
)

// TestFixSourceSurfacesSettingsErrors pins the settings-error
// branch in fixSourceImpl: when the effective config carries
// settings that one of the fixable rules cannot apply, Source()
// must return that error rather than silently producing a fix
// that omits the misconfigured rule.
func TestFixSourceSurfacesSettingsErrors(t *testing.T) {
	t.Parallel()
	cfg := config.Merge(config.Defaults(), &config.Config{
		Rules: map[string]config.RuleCfg{
			"bad-config": {Enabled: true, Settings: map[string]any{"key": "val"}},
		},
	})
	rules := []rule.Rule{&mockBadConfigFixableRule{id: "MDS300", name: "bad-config"}}
	_, err := Source(SourceOptions{
		Config:           cfg,
		Rules:            rules,
		Path:             "buf.md",
		Source:           []byte("# Hi\n\ndirty   \n"),
		StripFrontMatter: true,
	})
	require.Error(t, err, "settings-apply failure must surface as a Source() error")
	assert.Contains(t, err.Error(), assert.AnError.Error())
}
