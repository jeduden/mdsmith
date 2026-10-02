package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// A file with no "<?build" opener cannot yield a build target, so
// collectBuildTargets must skip the goldmark parse for it. See
// docs/development/high-performance-go.md, "Skip work you don't need".
func TestCollectBuildTargets_SkipsParseWithoutDirective(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "plain.md")
	body := strings.Repeat("# Title\n\nSome *prose* with a [link](x.md).\n\n", 200)
	require.NoError(t, os.WriteFile(p, []byte(body), 0o644))

	files := []string{p}
	allocs := testing.AllocsPerRun(5, func() {
		targets, errs := collectBuildTargets(files, root, "", 0)
		if len(targets) != 0 || len(errs) != 0 {
			t.Fatalf("unexpected result: %v %v", targets, errs)
		}
	})
	// File read + path bookkeeping only; a parse costs thousands.
	require.LessOrEqual(t, allocs, 30.0)
}
