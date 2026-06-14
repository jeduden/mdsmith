package engine

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jeduden/mdsmith/internal/config"
	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/jeduden/mdsmith/internal/rule"

	// Register the production rule set so lineLengthOnly can pull the real
	// line-length instance (with its default code-block exclusion).
	_ "github.com/jeduden/mdsmith/internal/rules/all"
)

// lineLengthOnly is the rule set whose only member is line-capable, so a
// run over it is flat-Layer-0 eligible. It returns the registered
// line-length rule so the run exercises its real default settings
// (exclude: code-blocks, tables, urls).
func lineLengthOnly() []rule.Rule {
	for _, r := range rule.All() {
		if r.Name() == "line-length" {
			return []rule.Rule{r}
		}
	}
	return nil
}

// TestComputeFlatLayer0Active pins the parse-skip eligibility gate: it
// fires only with the opt-in flag, an all-line-capable enabled rule set,
// and a config free of kinds and overrides (either could enable a
// non-line-capable rule for some file the empty-path check cannot see).
func TestComputeFlatLayer0Active(t *testing.T) {
	withKinds := config.Defaults()
	withKinds.Kinds = map[string]config.KindBody{"doc": {}}
	withOverrides := config.Defaults()
	withOverrides.Overrides = []config.Override{{Files: []string{"*.md"}}}

	cases := []struct {
		name string
		r    *Runner
		want bool
	}{
		{"flag off", &Runner{Config: config.Defaults(), Rules: lineLengthOnly()}, false},
		{"line-capable only", &Runner{Config: config.Defaults(), Rules: lineLengthOnly(), FlatLayer0: true}, true},
		{"full rule set", &Runner{Config: config.Defaults(), Rules: rule.All(), FlatLayer0: true}, false},
		{"config has kinds", &Runner{Config: withKinds, Rules: lineLengthOnly(), FlatLayer0: true}, false},
		{"config has overrides", &Runner{Config: withOverrides, Rules: lineLengthOnly(), FlatLayer0: true}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.r.computeFlatLayer0Active())
		})
	}
}

// TestPooledFileConstructor_FlatPath pins the per-file constructor choice:
// on an active run a directive-free file takes the parse-free flat
// constructor, but a file carrying an include/catalog directive stays on
// the full parse (so its generated-section suppression still works).
func TestPooledFileConstructor_FlatPath(t *testing.T) {
	ptr := func(f any) uintptr { return reflect.ValueOf(f).Pointer() }
	active := &Runner{flatL0Active: true}

	assert.Equal(t, ptr(lint.NewFileFlatPooled),
		ptr(active.pooledFileConstructor([]byte("# H\n\nplain prose\n"))),
		"directive-free file on an active run uses the flat constructor")
	assert.Equal(t, ptr(lint.NewFileFromSourcePooled),
		ptr(active.pooledFileConstructor([]byte("<?include file: x.md ?>\nbody\n"))),
		"a file with a generated directive stays on the AST path")

	inactive := &Runner{flatL0Active: false}
	assert.Equal(t, ptr(lint.NewFileFromSourcePooled),
		ptr(inactive.pooledFileConstructor([]byte("# H\n"))),
		"an inactive run always uses the AST constructor")

	block := &Runner{BlockOnlyParse: true, flatL0Active: true}
	assert.Equal(t, ptr(lint.NewFileBlockOnlyPooled),
		ptr(block.pooledFileConstructor([]byte("# H\n"))),
		"block-only takes precedence over flat")
}

// TestFlatLayer0_EquivalentDiagnostics is the end-to-end proof of
// acceptance criterion 3: with line-length the only enabled rule, the run
// takes the parse-skip path (flatL0Active true) yet produces diagnostics
// byte-identical to the AST path — including excluding a long line inside a
// fenced code block.
func TestFlatLayer0_EquivalentDiagnostics(t *testing.T) {
	long := strings.Repeat("alpha beta ", 9) // ~99 chars, over the default max 80
	doc := "# Heading\n\n" + long + "\n\n```\n" + long + "\n```\n"
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	require.NoError(t, os.WriteFile(path, []byte(doc), 0o644))

	astRunner := &Runner{
		Config: config.Defaults(), Rules: lineLengthOnly(),
		StripFrontMatter: true, RootDir: dir,
	}
	flatRunner := &Runner{
		Config: config.Defaults(), Rules: lineLengthOnly(),
		StripFrontMatter: true, RootDir: dir, FlatLayer0: true,
	}

	astRes := astRunner.Run([]string{path})
	flatRes := flatRunner.Run([]string{path})

	assert.False(t, astRunner.flatL0Active, "control run stays on the AST path")
	assert.True(t, flatRunner.flatL0Active, "line-length-only run takes the parse-skip path")
	require.Len(t, flatRes.Diagnostics, 1, "only the prose long line is flagged; the in-code long line is excluded")
	assert.Equal(t, 3, flatRes.Diagnostics[0].Line)
	assert.Equal(t, astRes.Diagnostics, flatRes.Diagnostics,
		"flat Layer-0 diagnostics must equal the AST path")
}
