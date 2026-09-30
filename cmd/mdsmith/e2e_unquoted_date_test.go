package main_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// unquotedDateKindCfg declares a kind whose schema types `date` with
// the given CUE constraint and ties the filename to it through
// `\#(fmvar(date))`.
func unquotedDateKindCfg(constraint string) string {
	return `kinds:
  post:
    schema:
      frontmatter:
        date: ` + constraint + `
      filename: '\#(fmvar(date))-*.md'
kind-assignment:
  - glob: ["posts/*.md"]
    kinds: [post]
`
}

const unquotedDatePost = "---\ndate: 2026-01-02\n---\n# Hello\n\nSome text.\n"

// An unquoted YAML date satisfies `date: string` and the fmvar filename
// glob at once; the author no longer has to quote it.
func TestE2E_Check_KindStringAcceptsUnquotedDate(t *testing.T) {
	dir := kindsTestDir(t, unquotedDateKindCfg("string"), map[string]string{
		"posts/2026-01-02-hello.md": unquotedDatePost,
	})
	stdout, stderr, code := runBinaryInDir(t, dir, "", "check", ".")
	assert.Equal(t, 0, code, "stdout=%s stderr=%s", stdout, stderr)
}

// A non-string constraint still rejects the date, naming the text it
// checked.
func TestE2E_Check_KindIntRejectsUnquotedDate(t *testing.T) {
	dir := kindsTestDir(t, unquotedDateKindCfg("int"), map[string]string{
		"posts/2026-01-02-hello.md": unquotedDatePost,
	})
	stdout, stderr, code := runBinaryInDir(t, dir, "", "check", ".")
	require.Equal(t, 1, code, "stdout=%s stderr=%s", stdout, stderr)
	out := stdout + stderr
	assert.Contains(t, out, `date: got "2026-01-02", expected int`)
	assert.NotContains(t, out, "unsupported front-matter value")
}
