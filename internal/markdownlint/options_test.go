package markdownlint

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jeduden/mdsmith/internal/rule"
)

func TestIntOpt(t *testing.T) {
	st := map[string]any{}
	assert.Empty(t, intOpt("max")(5, st))
	assert.Equal(t, map[string]any{"max": 5}, st)
	assert.Contains(t, intOpt("max")("x", st), "expected an integer")
}

func TestBoolOpt(t *testing.T) {
	st := map[string]any{}
	assert.Empty(t, boolOpt("stern")(true, st))
	assert.Equal(t, map[string]any{"stern": true}, st)
	assert.Contains(t, boolOpt("stern")(1, st), "expected a boolean")
}

func TestStringListOpt(t *testing.T) {
	st := map[string]any{}
	assert.Empty(t, stringListOpt("allow")([]any{"kbd"}, st))
	assert.Equal(t, map[string]any{"allow": []string{"kbd"}}, st)
	assert.Contains(t, stringListOpt("allow")("kbd", st), "expected a list")
}

func TestEnumOpt(t *testing.T) {
	spec := enumOpt("style", map[string]string{"one": "all-ones"}, "one")
	st := map[string]any{}
	assert.Empty(t, spec("one", st))
	assert.Equal(t, map[string]any{"style": "all-ones"}, st)
	assert.Contains(t, spec("zero", st), `value "zero" has no mdsmith equivalent`)
	assert.Contains(t, spec(1, st), "expected a string")
}

func TestExcludeToggle(t *testing.T) {
	st := map[string]any{}
	assert.Empty(t, excludeToggle("code-blocks")(true, st))
	assert.Equal(t, []string{"tables", "urls"}, st["exclude"])

	// A second toggle edits the list the first one produced.
	assert.Empty(t, excludeToggle("tables")(false, st))
	assert.Equal(t, []string{"tables", "urls"}, st["exclude"])

	assert.Contains(t, excludeToggle("tables")("x", st), "expected a boolean")
}

func TestMd035Style(t *testing.T) {
	st := map[string]any{}
	assert.Empty(t, md035Style("----", st))
	assert.Equal(t, map[string]any{"style": "dash", "length": 4}, st)
	assert.Contains(t, md035Style("consistent", st), "no mdsmith equivalent")
	assert.Contains(t, md035Style(3, st), "expected a string")
}

func TestMd025FrontMatterTitle(t *testing.T) {
	st := map[string]any{}
	assert.Empty(t, md025FrontMatterTitle("", st))
	assert.Equal(t, map[string]any{"front-matter-title": ""}, st)
	assert.Contains(t, md025FrontMatterTitle("^title:", st), "front-matter key")
	assert.Contains(t, md025FrontMatterTitle(1, st), "expected a string")
}

func TestMd052Shortcut(t *testing.T) {
	st := map[string]any{}
	assert.Empty(t, md052Shortcut(true, st))
	assert.Equal(t, "always", st["shortcut"])
	assert.Empty(t, md052Shortcut(false, st))
	assert.Equal(t, "collapsed-only", st["shortcut"])
	assert.Contains(t, md052Shortcut("x", st), "expected a boolean")
}

func TestRemoveString(t *testing.T) {
	assert.Equal(t, []string{"a", "c"}, removeString([]string{"a", "b", "c"}, "b"))
	assert.Equal(t, []string{"a"}, removeString([]string{"a"}, "x"))
}

func TestEnsureString(t *testing.T) {
	assert.Equal(t, []string{"a", "b"}, ensureString([]string{"a"}, "b"))
	assert.Equal(t, []string{"a"}, ensureString([]string{"a"}, "a"))
}

func TestSortedKeys(t *testing.T) {
	assert.Equal(t, []string{"a", "b", "c"},
		sortedKeys(map[string]int{"c": 1, "a": 2, "b": 3}))
}

func TestRuleState_EnsureSettings(t *testing.T) {
	rs := &ruleState{}
	rs.ensureSettings()["k"] = 1
	assert.Equal(t, map[string]any{"k": 1}, rs.settings)
	// The second call returns the same map, not a fresh one.
	rs.ensureSettings()["j"] = 2
	assert.Equal(t, map[string]any{"k": 1, "j": 2}, rs.settings)
}

// TestLineLengthDefaultExclude pins the copy of line-length's default
// exclude list used by excludeToggle to the rule's live
// DefaultSettings, so a changed rule default fails here instead of
// silently emitting a stale list.
func TestLineLengthDefaultExclude(t *testing.T) {
	for _, r := range rule.All() {
		if r.Name() != "line-length" {
			continue
		}
		c, ok := r.(rule.Configurable)
		require.True(t, ok)
		assert.Equal(t, c.DefaultSettings()["exclude"], lineLengthDefaultExclude)
		return
	}
	t.Fatal("line-length rule not registered")
}

// TestOptionTableTargetsExist verifies every option-table id resolves
// through the embedded front-matter mapping — a renamed rule README or
// dropped mapping fails here.
func TestOptionTableTargetsExist(t *testing.T) {
	idx, err := buildIndex()
	require.NoError(t, err)
	for id := range optionTable {
		assert.NotEmpty(t, idx.byID[id], "option table id %s has no front-matter mapping", id)
	}
}
