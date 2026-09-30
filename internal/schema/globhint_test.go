package schema

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The "with front matter applied" hint shows the author's value as
// written. The matcher sees it escaped (`[[]draft]`, `a[*]b`), and
// printing that form would show text the author never wrote.
func TestFilenameDiagnostic_HintShowsValueAsWritten(t *testing.T) {
	d := FilenameDiagnostic([]string{`\#(fmvar(id))-*.md`}, "other.md",
		map[string]any{"id": "[draft]*"}, false, "kind note")
	require.NotNil(t, d)
	assert.Equal(t, "with front matter applied: [draft]*-*.md", d.Hint)
}

// GlobHintForm substitutes every reference unescaped and leaves a
// literal opener and the rest of the glob as the author wrote them.
func TestGlobHintForm(t *testing.T) {
	assert.Equal(t, `notes/\#(draft)-a*b{c,d}.md`, GlobHintForm(
		`notes/\#(draft)-\#(fmvar(id)).md`,
		map[string]any{"id": "a*b{c,d}"}))
	// Callers pass only patterns that resolved; one that does not is
	// returned as written rather than as an empty string.
	const p = `docs/\#(fmvar(id)).md`
	assert.Equal(t, p, GlobHintForm(p, nil))
}
