package tidymark

import (
	"strings"
	"testing"
	"testing/fstest"
)

func TestListRules_SortedByID(t *testing.T) {
	rules, err := ListRules()
	if err != nil {
		t.Fatalf("ListRules: %v", err)
	}

	if len(rules) == 0 {
		t.Fatal("expected at least one rule")
	}

	for i := 1; i < len(rules); i++ {
		if rules[i].ID < rules[i-1].ID {
			t.Errorf("rules not sorted: %s comes after %s", rules[i].ID, rules[i-1].ID)
		}
	}
}

func TestListRules_ContainsTM001(t *testing.T) {
	rules, err := ListRules()
	if err != nil {
		t.Fatalf("ListRules: %v", err)
	}

	found := false
	for _, r := range rules {
		if r.ID == "TM001" {
			found = true
			if r.Name != "line-length" {
				t.Errorf("TM001 name = %q, want %q", r.Name, "line-length")
			}
			if r.Description == "" {
				t.Error("TM001 description is empty")
			}
			break
		}
	}
	if !found {
		t.Error("TM001 not found in rule list")
	}
}

func TestLookupRule_ByID(t *testing.T) {
	content, err := LookupRule("TM001")
	if err != nil {
		t.Fatalf("LookupRule(TM001): %v", err)
	}

	if !strings.Contains(content, "line-length") {
		t.Error("expected TM001 content to contain 'line-length'")
	}
}

func TestLookupRule_ByName(t *testing.T) {
	content, err := LookupRule("line-length")
	if err != nil {
		t.Fatalf("LookupRule(line-length): %v", err)
	}

	if !strings.Contains(content, "TM001") {
		t.Error("expected line-length content to contain 'TM001'")
	}
}

func TestLookupRule_CaseInsensitiveID(t *testing.T) {
	content, err := LookupRule("tm001")
	if err != nil {
		t.Fatalf("LookupRule(tm001): %v", err)
	}

	if !strings.Contains(content, "TM001") {
		t.Error("expected lowercase lookup to find TM001")
	}
}

func TestLookupRule_Unknown(t *testing.T) {
	_, err := LookupRule("TMXXX")
	if err == nil {
		t.Fatal("expected error for unknown rule")
	}
	if !strings.Contains(err.Error(), "unknown rule") {
		t.Errorf("error = %q, want it to contain 'unknown rule'", err.Error())
	}
}

func TestListRulesFromFS_SkipsBadFrontMatter(t *testing.T) {
	fsys := fstest.MapFS{
		"rules/TM999-good/README.md": &fstest.MapFile{
			Data: []byte("---\nid: TM999\nname: good-rule\ndescription: A good rule.\n---\n# TM999\n"),
		},
		"rules/TM998-bad/README.md": &fstest.MapFile{
			Data: []byte("no front matter here\n"),
		},
	}

	rules, err := listRulesFromFS(fsys)
	if err != nil {
		t.Fatalf("listRulesFromFS: %v", err)
	}

	if len(rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(rules))
	}

	if rules[0].ID != "TM999" {
		t.Errorf("rule ID = %q, want TM999", rules[0].ID)
	}
}

func TestLookupRuleFromFS_ByIDAndName(t *testing.T) {
	fsys := fstest.MapFS{
		"rules/TM999-test/README.md": &fstest.MapFile{
			Data: []byte("---\nid: TM999\nname: test-rule\ndescription: Test.\n---\n# Content\n"),
		},
	}

	content, err := lookupRuleFromFS(fsys, "TM999")
	if err != nil {
		t.Fatalf("lookupRuleFromFS(TM999): %v", err)
	}
	if !strings.Contains(content, "# Content") {
		t.Error("expected content to contain '# Content'")
	}

	content, err = lookupRuleFromFS(fsys, "test-rule")
	if err != nil {
		t.Fatalf("lookupRuleFromFS(test-rule): %v", err)
	}
	if !strings.Contains(content, "# Content") {
		t.Error("expected content to contain '# Content'")
	}
}

func TestLookupRuleFromFS_NotFound(t *testing.T) {
	fsys := fstest.MapFS{
		"rules/TM999-test/README.md": &fstest.MapFile{
			Data: []byte("---\nid: TM999\nname: test-rule\ndescription: Test.\n---\n# Content\n"),
		},
	}

	_, err := lookupRuleFromFS(fsys, "TMXXX")
	if err == nil {
		t.Fatal("expected error for unknown rule")
	}
}
