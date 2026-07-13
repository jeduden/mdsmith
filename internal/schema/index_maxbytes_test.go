package schema

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jeduden/mdsmith/internal/lint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestValidateIndex_RespectsMaxInputBytes pins ValidateIndex's read
// of the on-disk index side-output to f.MaxInputBytes, mirroring
// every other cross-file read in this package (readPath) and rule
// package (include, catalog, cross-file-reference-integrity). A raw
// os.ReadFile ignores the file-size contract those callers all
// respect (docs/development/high-performance-go.md "os.ReadFile on
// huge inputs"): an on-disk index larger than the configured cap
// must surface as a "too large" read error, not be read in full.
func TestValidateIndex_RespectsMaxInputBytes(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "doc.md")
	require.NoError(t, os.WriteFile(path, []byte("# T\n"), 0o644))
	big := strings.Repeat("x", 100)
	require.NoError(t, os.WriteFile(filepath.Join(root, "out.json"), []byte(big), 0o644))

	f, err := lint.NewFile(path, []byte("# T\n"))
	require.NoError(t, err)
	f.RootDir = root
	f.MaxInputBytes = 10

	sch := &Schema{Source: "test", RootLevel: 2, Index: &IndexSpec{
		Output:  "out.json",
		Include: []string{IndexIncludeHeadingsFlat},
	}}
	diags := ValidateIndex(f, sch, makeDiagForTest)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, "cannot be read")
	assert.Contains(t, diags[0].Message, "too large")
}
