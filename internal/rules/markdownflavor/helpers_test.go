package markdownflavor

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/jeduden/mdsmith/internal/lint"
)

// mkFile parses src with lint.NewFile and returns the resulting
// *lint.File. Shared by rule_test.go and fix_test.go which exercise
// the rule's *lint.File-shaped Check and Fix entry points.
func mkFile(t *testing.T, src string) *lint.File {
	t.Helper()
	f, err := lint.NewFile("test.md", []byte(src))
	require.NoError(t, err)
	return f
}
