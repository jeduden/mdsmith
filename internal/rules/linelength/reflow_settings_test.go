package linelength

import (
	"testing"

	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApplySettings_Reflow(t *testing.T) {
	r := &Rule{}
	require.NoError(t, r.ApplySettings(map[string]any{"reflow": true}))
	assert.True(t, r.Reflow)

	require.NoError(t, r.ApplySettings(map[string]any{"reflow": false}))
	assert.False(t, r.Reflow)

	err := r.ApplySettings(map[string]any{"reflow": "yes"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reflow must be a bool")
}

func TestApplySettings_Abbreviations(t *testing.T) {
	r := &Rule{}
	require.NoError(t, r.ApplySettings(map[string]any{
		"abbreviations": []any{"etc.", "approx."},
	}))
	assert.Equal(t, []string{"etc.", "approx."}, r.Abbreviations)

	err := r.ApplySettings(map[string]any{"abbreviations": "etc."})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "abbreviations must be a list of strings")
}

func TestSettingMergeMode(t *testing.T) {
	r := &Rule{}
	assert.Equal(t, rule.MergeAppend, r.SettingMergeMode("abbreviations"))
	assert.Equal(t, rule.MergeReplace, r.SettingMergeMode("exclude"))
	assert.Equal(t, rule.MergeReplace, r.SettingMergeMode("max"))
}

func TestDefaultSettings_IncludesReflowKeys(t *testing.T) {
	r := &Rule{}
	ds := r.DefaultSettings()
	assert.Equal(t, false, ds["reflow"])
	assert.Equal(t, []string{}, ds["abbreviations"])
}
