package rules

import (
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListRules_SortedByID(t *testing.T) {
	rules, err := ListRules()
	require.NoError(t, err, "ListRules: %v", err)

	if len(rules) == 0 {
		t.Fatal("expected at least one rule")
	}

	for i := 1; i < len(rules); i++ {
		if rules[i].ID < rules[i-1].ID {
			t.Errorf("rules not sorted: %s comes after %s", rules[i].ID, rules[i-1].ID)
		}
	}
}

func TestListRules_ContainsMDS001(t *testing.T) {
	rules, err := ListRules()
	require.NoError(t, err, "ListRules: %v", err)

	found := false
	for _, r := range rules {
		if r.ID == "MDS001" {
			found = true
			if r.Name != "line-length" {
				t.Errorf("MDS001 name = %q, want %q", r.Name, "line-length")
			}
			if r.Description == "" {
				t.Error("MDS001 description is empty")
			}
			break
		}
	}
	assert.True(t, found, "MDS001 not found in rule list")
}

func TestLookupRule_ByID(t *testing.T) {
	content, err := LookupRule("MDS001")
	require.NoError(t, err, "LookupRule(MDS001): %v", err)

	assert.Contains(t, content, "line-length", "expected MDS001 content to contain 'line-length'")
}

func TestLookupRule_ByName(t *testing.T) {
	content, err := LookupRule("line-length")
	require.NoError(t, err, "LookupRule(line-length): %v", err)

	assert.Contains(t, content, "MDS001", "expected line-length content to contain 'MDS001'")
}

func TestLookupRule_CaseInsensitiveID(t *testing.T) {
	content, err := LookupRule("mds001")
	require.NoError(t, err, "LookupRule(mds001): %v", err)

	assert.Contains(t, content, "MDS001", "expected lowercase lookup to find MDS001")
}

func TestLookupRule_Unknown(t *testing.T) {
	_, err := LookupRule("MDSXXX")
	require.Error(t, err, "expected error for unknown rule")
	assert.Contains(t, err.Error(), "unknown rule", "error = %q, want it to contain 'unknown rule'", err.Error())
}

func TestListRulesFromFS_SkipsBadFrontMatter(t *testing.T) {
	fsys := fstest.MapFS{
		"good/README.md": &fstest.MapFile{
			Data: []byte("---\nid: MDS999\nname: good-rule\nstatus: ready\ndescription: A good rule.\n---\n# MDS999\n"),
		},
		"bad/README.md": &fstest.MapFile{
			Data: []byte("no front matter here\n"),
		},
	}

	rules, err := listRulesFromFS(fsys)
	require.NoError(t, err, "listRulesFromFS: %v", err)

	require.Len(t, rules, 1, "expected 1 rule, got %d", len(rules))

	if rules[0].ID != "MDS999" {
		t.Errorf("rule ID = %q, want MDS999", rules[0].ID)
	}
}

func TestLookupRuleFromFS_ByIDAndName(t *testing.T) {
	fsys := fstest.MapFS{
		"testrule/README.md": &fstest.MapFile{
			Data: []byte("---\nid: MDS999\nname: test-rule\nstatus: ready\ndescription: Test.\n---\n# Content\n"),
		},
	}

	content, err := lookupRuleFromFS(fsys, "MDS999")
	require.NoError(t, err, "lookupRuleFromFS(MDS999): %v", err)
	assert.Contains(t, content, "# Content", "expected content to contain '# Content'")

	content, err = lookupRuleFromFS(fsys, "test-rule")
	require.NoError(t, err, "lookupRuleFromFS(test-rule): %v", err)
	assert.Contains(t, content, "# Content", "expected content to contain '# Content'")
}

func TestLookupRuleFromFS_ExcludesFrontMatter(t *testing.T) {
	fsys := fstest.MapFS{
		"testrule/README.md": &fstest.MapFile{
			Data: []byte("---\nid: MDS999\nname: test-rule\nstatus: ready\ndescription: Test.\n---\n# Content\n"),
		},
	}

	content, err := lookupRuleFromFS(fsys, "MDS999")
	require.NoError(t, err, "lookupRuleFromFS(MDS999): %v", err)
	assert.NotContains(t, content, "---", "expected content to not contain front matter delimiters")
	assert.NotContains(t, content, "status: ready", "expected content to not contain front matter fields")
	assert.Contains(t, content, "# Content", "expected content body to be preserved")
}

func TestLookupRuleFromFS_NotFound(t *testing.T) {
	fsys := fstest.MapFS{
		"testrule/README.md": &fstest.MapFile{
			Data: []byte("---\nid: MDS999\nname: test-rule\nstatus: ready\ndescription: Test.\n---\n# Content\n"),
		},
	}

	_, err := lookupRuleFromFS(fsys, "MDSXXX")
	require.Error(t, err, "expected error for unknown rule")
}

func TestListRulesFromFS_SkipsMissingStatus(t *testing.T) {
	fsys := fstest.MapFS{
		"nostatus/README.md": &fstest.MapFile{
			Data: []byte("---\nid: MDS998\nname: no-status\ndescription: Missing status.\n---\n# MDS998\n"),
		},
	}

	rules, err := listRulesFromFS(fsys)
	require.NoError(t, err, "listRulesFromFS: %v", err)
	require.Len(t, rules, 0, "expected 0 rules, got %d", len(rules))
}
