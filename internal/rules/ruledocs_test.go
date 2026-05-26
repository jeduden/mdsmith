package rules

import (
	"bufio"
	"fmt"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// errFS is an fs.FS that always returns an error on Open, forcing ReadDir to fail.
type errFS struct{}

func (errFS) Open(string) (fs.File, error) { return nil, fmt.Errorf("forced readdir error") }

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

func TestLookupRuleInfo_ByID(t *testing.T) {
	info, err := LookupRuleInfo("MDS019")
	require.NoError(t, err)
	assert.Equal(t, "MDS019", info.ID)
	assert.Equal(t, "catalog", info.Name)
	require.NotNil(t, info.Maintainability)
	assert.NotEmpty(t, info.Maintainability.Signal)
}

func TestLookupRuleInfo_ByName(t *testing.T) {
	info, err := LookupRuleInfo("line-length")
	require.NoError(t, err)
	assert.Equal(t, "MDS001", info.ID)
	assert.Nil(t, info.Maintainability)
}

func TestLookupRuleInfo_Unknown(t *testing.T) {
	_, err := LookupRuleInfo("MDSXXX")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown rule")
}

func TestLookupRuleInfoFromFS_PropagatesReadDirError(t *testing.T) {
	_, err := lookupRuleInfoFromFS(errFS{}, "anything")
	require.Error(t, err)
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

// =====================================================================
// Phase 5: additional coverage
// =====================================================================

// listRulesFromFS: ReadDir error
func TestListRulesFromFS_ReadDirError(t *testing.T) {
	_, err := listRulesFromFS(errFS{})
	require.Error(t, err)
}

// listRulesFromFS: non-directory entry → continue
func TestListRulesFromFS_SkipsNonDirEntries(t *testing.T) {
	fsys := fstest.MapFS{
		"not-a-dir": &fstest.MapFile{
			Data: []byte("plain file in root"),
		},
		"realrule/README.md": &fstest.MapFile{
			Data: []byte("---\nid: MDS997\nname: real-rule\nstatus: ready\ndescription: Real.\n---\n# Real\n"),
		},
	}
	rules, err := listRulesFromFS(fsys)
	require.NoError(t, err)
	// "not-a-dir" is a file, not a dir, so it's skipped; only realrule is returned.
	require.Len(t, rules, 1)
	assert.Equal(t, "MDS997", rules[0].ID)
}

// listRulesFromFS: ReadFile error (dir without README.md) → continue
func TestListRulesFromFS_SkipsEmptyDir(t *testing.T) {
	fsys := fstest.MapFS{
		// Dir has no README.md file.
		"norule/other.txt": &fstest.MapFile{
			Data: []byte("not a readme"),
		},
		"goodrule/README.md": &fstest.MapFile{
			Data: []byte("---\nid: MDS996\nname: good\nstatus: ready\ndescription: Good.\n---\n"),
		},
	}
	rules, err := listRulesFromFS(fsys)
	require.NoError(t, err)
	require.Len(t, rules, 1)
	assert.Equal(t, "MDS996", rules[0].ID)
}

// lookupRuleFromFS: listRulesFromFS error propagation
func TestLookupRuleFromFS_PropagatesReadDirError(t *testing.T) {
	_, err := lookupRuleFromFS(errFS{}, "anything")
	require.Error(t, err)
}

// parseFrontMatter: missing ID → error
func TestParseFrontMatter_MissingID(t *testing.T) {
	content := "---\nname: no-id\nstatus: ready\ndescription: Missing id.\n---\n"
	_, err := parseFrontMatter(content)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing id")
}

// stripFrontMatter: no front matter prefix → return as-is
func TestStripFrontMatter_NoPrefixReturnedUnchanged(t *testing.T) {
	content := "# Heading\nNo front matter here.\n"
	result := stripFrontMatter(content)
	assert.Equal(t, content, result)
}

// stripFrontMatter: front matter with no closing --- → return as-is
func TestStripFrontMatter_NoClosingDelimiter(t *testing.T) {
	content := "---\nid: test\nno closing delimiter here\n"
	result := stripFrontMatter(content)
	assert.Equal(t, content, result)
}

// TestStripFrontMatterExported verifies that the exported StripFrontMatter
// delegates to the unexported implementation correctly.
func TestStripFrontMatterExported(t *testing.T) {
	content := "---\nid: MDS001\nname: line-length\n---\n# Body\n"
	result := StripFrontMatter(content)
	assert.NotContains(t, result, "id: MDS001")
	assert.Contains(t, result, "# Body")
}

func TestParseFrontMatter_MaintainabilityBlock(t *testing.T) {
	content := "---\n" +
		"id: MDS999\n" +
		"name: example\n" +
		"status: ready\n" +
		"description: Example.\n" +
		"maintainability:\n" +
		"  signal: a list of links\n" +
		"  fix: adopt the directive\n" +
		"  for-diagnostic: true\n" +
		"---\n# Body\n"
	info, err := parseFrontMatter(content)
	require.NoError(t, err)
	require.NotNil(t, info.Maintainability)
	assert.Equal(t, "a list of links", info.Maintainability.Signal)
	assert.Equal(t, "adopt the directive", info.Maintainability.Fix)
	assert.True(t, info.Maintainability.ForDiagnostic)
}

func TestParseFrontMatter_NullMaintainability(t *testing.T) {
	content := "---\n" +
		"id: MDS999\n" +
		"name: example\n" +
		"status: ready\n" +
		"description: Example.\n" +
		"maintainability: null\n" +
		"---\n# Body\n"
	info, err := parseFrontMatter(content)
	require.NoError(t, err)
	assert.Nil(t, info.Maintainability,
		"explicit null must result in a nil Maintainability pointer")
}

// TestParseFrontMatter_FoldsBlockScalarDescription verifies that a folded
// block scalar (`description: >-`) is parsed as folded YAML and collapsed
// to a single line so `mdsmith help rule` does not render literal ">-".
func TestParseFrontMatter_FoldsBlockScalarDescription(t *testing.T) {
	content := "---\n" +
		"id: MDS999\n" +
		"name: example\n" +
		"status: ready\n" +
		"description: >-\n" +
		"  First line\n" +
		"  continued on a second line.\n" +
		"maintainability: null\n" +
		"---\n# Body\n"
	info, err := parseFrontMatter(content)
	require.NoError(t, err)
	assert.Equal(t, "First line continued on a second line.", info.Description)
	assert.NotContains(t, info.Description, "\n")
	assert.NotContains(t, info.Description, ">-")
}

// TestParseFrontMatter_ScannerError verifies that bufio.Scanner errors (here
// triggered by a single line exceeding the default 64 KiB buffer) propagate
// as a clear "scanning front matter" error rather than being swallowed.
func TestParseFrontMatter_ScannerError(t *testing.T) {
	longLine := strings.Repeat("a", bufio.MaxScanTokenSize+1)
	content := "---\n" + longLine + "\n---\n# body\n"
	_, err := parseFrontMatter(content)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "scanning front matter")
}

// TestParseFrontMatter_UnterminatedFrontMatter verifies that a front matter
// block without a closing `---` line fails with a clear error instead of
// silently treating the rest of the file as YAML.
func TestParseFrontMatter_UnterminatedFrontMatter(t *testing.T) {
	content := "---\n" +
		"id: MDS999\n" +
		"name: example\n" +
		"status: ready\n" +
		"# body without closing delimiter\n"
	_, err := parseFrontMatter(content)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unterminated front matter")
}

// TestParseFrontMatter_MarkdownlintBlock verifies that a list of markdownlint
// equivalents in front matter is parsed into the Markdownlint field, including
// the optional `partial` flag.
func TestParseFrontMatter_MarkdownlintBlock(t *testing.T) {
	content := "---\n" +
		"id: MDS999\n" +
		"name: example\n" +
		"status: ready\n" +
		"description: Example.\n" +
		"markdownlint:\n" +
		"  - id: MD013\n" +
		"    name: line-length\n" +
		"  - id: MD020\n" +
		"    name: no-missing-space-closed-atx\n" +
		"    partial: true\n" +
		"---\n# Body\n"
	info, err := parseFrontMatter(content)
	require.NoError(t, err)
	require.Len(t, info.Markdownlint, 2)
	assert.Equal(t, "MD013", info.Markdownlint[0].ID)
	assert.Equal(t, "line-length", info.Markdownlint[0].Name)
	assert.False(t, info.Markdownlint[0].Partial)
	assert.Equal(t, "MD020", info.Markdownlint[1].ID)
	assert.Equal(t, "no-missing-space-closed-atx", info.Markdownlint[1].Name)
	assert.True(t, info.Markdownlint[1].Partial)
}

// TestParseFrontMatter_NullMarkdownlint verifies that an explicit `null`
// for markdownlint, or omitting the key entirely, both yield a nil slice
// rather than an error. Front matter that already conformed to the older
// schema (no `markdownlint:` key at all) must keep parsing cleanly.
func TestParseFrontMatter_NullMarkdownlint(t *testing.T) {
	cases := map[string]string{
		"explicit-null": "---\n" +
			"id: MDS999\n" +
			"name: example\n" +
			"status: ready\n" +
			"description: Example.\n" +
			"markdownlint: null\n" +
			"---\n# Body\n",
		"key-absent": "---\n" +
			"id: MDS999\n" +
			"name: example\n" +
			"status: ready\n" +
			"description: Example.\n" +
			"---\n# Body\n",
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			info, err := parseFrontMatter(content)
			require.NoError(t, err)
			assert.Nil(t, info.Markdownlint)
		})
	}
}

// TestParseFrontMatter_PeerLinterBlocks verifies that rumdl, mado, and
// panache mapping blocks parse into their respective RuleInfo slices and
// that the per-entry `default` flag round-trips correctly.
func TestParseFrontMatter_PeerLinterBlocks(t *testing.T) {
	content := "---\n" +
		"id: MDS999\n" +
		"name: example\n" +
		"status: ready\n" +
		"description: Example.\n" +
		"markdownlint:\n" +
		"  - id: MD013\n" +
		"    name: line-length\n" +
		"    default: true\n" +
		"rumdl:\n" +
		"  - id: MD013\n" +
		"    name: line-length\n" +
		"    default: true\n" +
		"mado:\n" +
		"  - id: MD013\n" +
		"    name: line-length\n" +
		"    default: true\n" +
		"panache:\n" +
		"  - id: heading-hierarchy\n" +
		"    name: heading-hierarchy\n" +
		"    default: false\n" +
		"---\n# Body\n"
	info, err := parseFrontMatter(content)
	require.NoError(t, err)
	require.Len(t, info.Markdownlint, 1)
	assert.True(t, info.Markdownlint[0].Default)
	require.Len(t, info.Rumdl, 1)
	assert.Equal(t, "MD013", info.Rumdl[0].ID)
	assert.True(t, info.Rumdl[0].Default)
	require.Len(t, info.Mado, 1)
	assert.Equal(t, "MD013", info.Mado[0].ID)
	assert.True(t, info.Mado[0].Default)
	require.Len(t, info.Panache, 1)
	assert.Equal(t, "heading-hierarchy", info.Panache[0].ID)
	assert.False(t, info.Panache[0].Default)
}

// TestParseFrontMatter_RejectsYAMLAliases verifies that the safe-YAML wrapper
// rejects anchor/alias usage in rule README front matter rather than silently
// expanding aliases.
func TestParseFrontMatter_RejectsYAMLAliases(t *testing.T) {
	content := "---\n" +
		"id: MDS999\n" +
		"name: example\n" +
		"status: ready\n" +
		"description: &x Example.\n" +
		"alias: *x\n" +
		"---\n# Body\n"
	_, err := parseFrontMatter(content)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "anchors/aliases")
}
