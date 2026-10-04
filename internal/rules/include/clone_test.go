package include

import (
	"testing"
	"testing/fstest"

	"github.com/jeduden/mdsmith/internal/rule"
	"github.com/stretchr/testify/assert"
)

// TestCloneInstanceAfterUseExpandsNestedIncludes pins that a clone
// taken after the rule has run expands nested includes through its own
// include chain. An engine cached in the struct is copied by
// rule.CloneInstance still bound to the source rule, whose chain is nil
// between calls, so the clone skipped the recursive expansion and
// spliced the stale nested body.
func TestCloneInstanceAfterUseExpandsNestedIncludes(t *testing.T) {
	fsys := fstest.MapFS{
		"b.md": {Data: []byte("<?include\nfile: c.md\n?>\nstale\n<?/include?>\n")},
		"c.md": {Data: []byte("fresh\n")},
	}
	src := "<?include\nfile: b.md\n?>\nold\n<?/include?>\n"
	r := &Rule{}
	r.Fix(newTestFile(t, "a.md", src, fsys)) // the source rule has run
	clone := rule.CloneInstance(r).(*Rule)

	got := string(clone.Fix(newTestFile(t, "a.md", src, fsys)))
	assert.Contains(t, got, "fresh\n")
	assert.NotContains(t, got, "stale\n")
}
