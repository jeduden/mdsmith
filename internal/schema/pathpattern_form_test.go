package schema

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

// On Windows a `path-pattern:` may be written with the host
// separator, and filepath.ToSlash has always turned it into `/`. It
// must not touch the `\` that opens a `\#(fmvar(...))` reference,
// though, or the reference becomes a literal `/#(` no file matches.
// CI runs on Linux, so the normaliser is driven with `\` as the
// separator directly.
func TestSlashOutsideRefs_WindowsSeparator(t *testing.T) {
	cases := []struct{ in, want string }{
		// Separators around a reference become `/`; the opener stays.
		{`docs\skill-\#(fmvar(name))\x.md`, `docs/skill-\#(fmvar(name))/x.md`},
		// A separator right before the opener is still a separator.
		{`.apm\skills\\#(fmvar(name))\SKILL.md`, `.apm/skills/\#(fmvar(name))/SKILL.md`},
		// The reference's own bytes are kept verbatim.
		{`a\\#(fmvar("k\\x"))\b`, `a/\#(fmvar("k\\x"))/b`},
		// With no reference it is exactly filepath.ToSlash on Windows,
		// literal `\#(` included.
		{`docs\a\b.md`, `docs/a/b.md`},
		{`notes\\#(draft)*.md`, `notes//#(draft)*.md`},
		{`docs\\#(fmvar(my-key))\x.md`, `docs//#(fmvar(my-key))/x.md`},
		// Already slash-separated: unchanged.
		{`.apm/skills/\#(fmvar(name))/SKILL.md`, `.apm/skills/\#(fmvar(name))/SKILL.md`},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, slashOutsideRefs(tc.in, '\\'), tc.in)
	}
}

// On a `/` host `\` is the glob escape character, never a separator,
// so the pattern is returned as written — `\\#(` stays an escaped
// backslash, as it always was.
func TestSlashOutsideRefs_SlashSeparatorIsIdentity(t *testing.T) {
	for _, p := range []string{
		`docs\skill-\#(fmvar(name))\x.md`,
		`docs/\\#(fmvar(name)).md`,
		"docs/**/*.md",
	} {
		assert.Equal(t, p, slashOutsideRefs(p, '/'), p)
	}
}

// A pattern with no host separator is returned as-is, without an
// allocation: that is every pattern on a `/` host and most on Windows.
func TestSlashOutsideRefs_NoSeparatorDoesNotAllocate(t *testing.T) {
	p := `.apm/skills/\#(fmvar(name))/SKILL.md`
	allocs := testing.AllocsPerRun(100, func() { _ = slashOutsideRefs(p, '|') })
	assert.Zero(t, allocs)
}

// PathPatternMatchForm is the host-separator form of a pattern plus
// whether it interpolates, computed once at config load.
func TestPathPatternMatchForm(t *testing.T) {
	form, interp := PathPatternMatchForm(`.apm/skills/\#(fmvar(name))/SKILL.md`)
	assert.Equal(t, `.apm/skills/\#(fmvar(name))/SKILL.md`, form)
	assert.True(t, interp)

	form, interp = PathPatternMatchForm("plan/[0-9]*.md")
	assert.Equal(t, "plan/[0-9]*.md", form)
	assert.False(t, interp)

	form, interp = PathPatternMatchForm(`notes/\#(draft)*.md`)
	assert.Equal(t, slashOutsideRefs(`notes/\#(draft)*.md`, filepath.Separator), form)
	assert.False(t, interp)
}
