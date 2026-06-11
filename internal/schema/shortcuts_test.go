package schema

import (
	"encoding/json"
	"sort"
	"strings"
	"testing"

	"github.com/jeduden/mdsmith/cue/cuelite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestShortcutRegistry_NamesMatchPlan pins the seven shortcut
// names plan 148 promises. Adding or removing a name should be
// a conscious change; this test fails loudly when it isn't.
func TestShortcutRegistry_NamesMatchPlan(t *testing.T) {
	want := []string{
		"date", "datetime", "email",
		"filename", "nonEmpty", "time", "url",
	}
	got := ShortcutNames()
	sort.Strings(got)
	assert.Equal(t, want, got)
}

// TestShortcutRegistry_CanonicalsCompileAndMatch exercises every
// registered shortcut: its canonical CUE expression must compile
// cleanly through the in-house cuelite engine, accept a known-good
// value, and reject a clear violation. The cases ride on the same
// registry the parser uses, so any change to the canonical CUE that
// breaks the promised semantics surfaces here. (Validated against
// cuelang while it was vendored; now run on the in-house engine that
// MDS020 itself uses, plan 240.)
func TestShortcutRegistry_CanonicalsCompileAndMatch(t *testing.T) {
	cases := []struct {
		name   string
		accept string
		reject string
	}{
		{"date", "2024-05-01", "2024-5-1"},
		{"datetime", "2024-05-01T12:30:00Z", "2024-05-01 12:30:00"},
		{"time", "12:30", "12:3"},
		{"email", "user@example.com", "user@@example"},
		{"url", "https://example.com", "ftp://example.com"},
		{"filename", "notes.md", "notes.txt"},
		{"nonEmpty", "hello", ""},
	}
	// validates compiles `{x: <canonical>}` and reports whether the value
	// validates against the schema, the same shape MDS020 builds.
	validates := func(t *testing.T, canonical, value string) bool {
		t.Helper()
		schema, err := cuelite.Compile("{x: " + canonical + "}")
		require.NoErrorf(t, err, "canonical %q failed to compile", canonical)
		raw, err := json.Marshal(map[string]string{"x": value})
		require.NoError(t, err)
		data, err := cuelite.CompileJSON(raw)
		require.NoError(t, err)
		return schema.Unify(data).Validate() == nil
	}
	for _, tc := range cases {
		canonical, ok := LookupShortcut(tc.name)
		require.Truef(t, ok, "shortcut %q missing", tc.name)
		assert.Truef(t, validates(t, canonical, tc.accept),
			"shortcut %q rejected %q", tc.name, tc.accept)
		assert.Falsef(t, validates(t, canonical, tc.reject),
			"shortcut %q accepted %q", tc.name, tc.reject)
	}
}

// TestResolveBareName_PassesThroughCUEBuiltins keeps the
// existing `name: 'string'` and `flag: bool` patterns in
// proto.md frontmatter working — bare CUE builtins must
// not be redirected through the shortcut registry.
func TestResolveBareName_PassesThroughCUEBuiltins(t *testing.T) {
	for _, name := range []string{
		"string", "int", "float", "bool", "bytes", "number", "_",
	} {
		out, handled, err := resolveBareName(name)
		require.NoErrorf(t, err, "builtin %q", name)
		require.True(t, handled, "builtin %q should be recognised", name)
		assert.Equal(t, name, out)
	}
}

// TestResolveBareName_SubstitutesShortcut verifies that the
// loader rewrites `created: date` to the canonical regex
// CUE expression so downstream compilation sees the same
// shape regardless of which surface the user wrote.
func TestResolveBareName_SubstitutesShortcut(t *testing.T) {
	out, handled, err := resolveBareName("date")
	require.NoError(t, err)
	require.True(t, handled)
	assert.Contains(t, out, `\\d{4}-\\d{2}-\\d{2}`)
}

// TestResolveBareName_RejectsUnknownBareName guards the
// typo-catches-early promise: a bare scalar that looks
// like a shortcut but is not registered (and not a CUE
// builtin) errors immediately, instead of silently sliding
// through to CUE.
func TestResolveBareName_RejectsUnknownBareName(t *testing.T) {
	_, handled, err := resolveBareName("iso-date")
	require.True(t, handled,
		"iso-date should be picked up as a bare-name candidate")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "iso-date")
	assert.Contains(t, err.Error(), "date") // suggests known names
}

// TestResolveBareName_IgnoresNonBareCandidates leaves
// real CUE expressions alone: anything with operators,
// whitespace, quotes, or special characters is none of
// the registry's business.
func TestResolveBareName_IgnoresNonBareCandidates(t *testing.T) {
	for _, expr := range []string{
		`"open" | "done"`,
		`date & >="2020-01-01"`,
		`string & != ""`,
		`=~"^RFC-[0-9]{4}$"`,
		`[...string]`,
	} {
		out, handled, err := resolveBareName(expr)
		require.NoError(t, err)
		assert.False(t, handled,
			"non-bare expression %q should pass through", expr)
		assert.Equal(t, expr, out)
	}
}

// TestParseInline_FrontmatterShortcutSubstitutes locks the
// observable outcome at the schema level: a kind declaring
// `created: date` ends up with the canonical CUE in the
// parsed Schema, so the validator sees identical shape to
// a user who wrote the regex out by hand.
func TestParseInline_FrontmatterShortcutSubstitutes(t *testing.T) {
	raw := map[string]any{
		"frontmatter": map[string]any{
			"created":  "date",
			"modified": "datetime",
			"homepage": "url",
		},
	}
	sch, err := ParseInline(raw, "kind rfc")
	require.NoError(t, err)
	assert.Contains(t, sch.Frontmatter["created"], `\\d{4}-\\d{2}-\\d{2}`)
	assert.Contains(t, sch.Frontmatter["modified"], `\\d{2}:\\d{2}:\\d{2}`)
	assert.Contains(t, sch.Frontmatter["homepage"], `https?://`)
}

// TestParseInline_UnknownShortcutErrorNamesField ensures
// the error message identifies the offending field and
// the unknown bare name — the user should not have to
// scan a wall of YAML to find what tripped the loader.
func TestParseInline_UnknownShortcutErrorNamesField(t *testing.T) {
	raw := map[string]any{
		"frontmatter": map[string]any{
			"created": "iso-date",
		},
	}
	_, err := ParseInline(raw, "kind rfc")
	require.Error(t, err)
	msg := err.Error()
	assert.Contains(t, msg, "created")
	assert.Contains(t, msg, "iso-date")
}

// TestParseInline_RawCUEPassesThroughUnchanged covers the
// acceptance criterion "a value containing operators is
// parsed as raw CUE without lookup". The disjunction
// stays verbatim — no substitution attempted on either
// side of the pipe.
func TestParseInline_RawCUEPassesThroughUnchanged(t *testing.T) {
	raw := map[string]any{
		"frontmatter": map[string]any{
			"status": `"open" | "done"`,
		},
	}
	sch, err := ParseInline(raw, "kind rfc")
	require.NoError(t, err)
	assert.Equal(t, `"open" | "done"`, sch.Frontmatter["status"])
}

// TestValidate_Inline_ShortcutAcceptsAndRejects is the
// end-to-end shape check the plan's acceptance
// criterion calls for: an inline schema declaring
// `created: date` validates `2024-05-01` and rejects
// `2024-5-1`. The shortcut substitution is invisible
// to callers — what matters is that the canonical CUE
// behaves as advertised.
func TestValidate_Inline_ShortcutAcceptsAndRejects(t *testing.T) {
	raw := map[string]any{
		"frontmatter": map[string]any{
			"created": "date",
		},
	}
	sch, err := ParseInline(raw, "kind rfc")
	require.NoError(t, err)

	good := newDocFile(t, "doc.md", "# T\n")
	diags := Validate(good, sch,
		map[string]any{"created": "2024-05-01"}, false, makeDiagForTest)
	assert.Empty(t, diags, "well-formed date should validate")

	doc := newDocFile(t, "doc.md", "# T\n")
	diags = Validate(doc, sch,
		map[string]any{"created": "2024-5-1"}, false, makeDiagForTest)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, `created: got "2024-5-1"`)
	assert.Contains(t, diags[0].Message, "string matching")
}

// TestValidate_File_ShortcutWorksOffline covers the
// proto.md acceptance criterion: a schema file whose
// frontmatter uses bare-name shortcuts loads and
// validates against documents without network access.
// The library is embedded in the binary, so the lookup
// is local-only — no CUE module cache, no go module
// proxy, no anything.
func TestValidate_File_ShortcutWorksOffline(t *testing.T) {
	dir := t.TempDir()
	proto := "---\n" +
		"created: date\n" +
		"homepage: url\n" +
		"---\n# ?\n"
	p := writeFile(t, dir, "proto.md", proto)

	sch, err := ParseFile(&FileReader{}, p)
	require.NoError(t, err)
	assert.Contains(t, sch.Frontmatter["created"], `\\d{4}-\\d{2}-\\d{2}`)
	assert.Contains(t, sch.Frontmatter["homepage"], `https?://`)

	good := newDocFile(t, "doc.md", "# T\n")
	diags := Validate(good, sch, map[string]any{
		"created":  "2024-05-01",
		"homepage": "https://example.com",
	}, false, makeDiagForTest)
	assert.Empty(t, diags, "well-formed values should validate")

	bad := newDocFile(t, "doc.md", "# T\n")
	diags = Validate(bad, sch, map[string]any{
		"created":  "2024-05-01",
		"homepage": "ftp://example.com",
	}, false, makeDiagForTest)
	require.Len(t, diags, 1)
	assert.Contains(t, diags[0].Message, `homepage: got "ftp://example.com"`)
	assert.Contains(t, diags[0].Message, "string matching")
}

// TestShortcutRegistry_MatchesEmbeddedCUE is the
// drift-detector: the canonical CUE strings the Go
// registry serves must be byte-identical to what the
// embedded `cue/types/types.cue` file documents (after
// trimming whitespace), so the file stays the single
// human-readable source of truth.
func TestShortcutRegistry_MatchesEmbeddedCUE(t *testing.T) {
	source := EmbeddedTypesCUE()
	for _, name := range ShortcutNames() {
		canonical, ok := LookupShortcut(name)
		require.True(t, ok)
		// Each definition is on its own line as `#name: expr`.
		// Find the line and compare the expression body.
		idx := strings.Index(source, "#"+name+":")
		require.GreaterOrEqualf(t, idx, 0,
			"definition #%s missing from embedded CUE", name)
		rest := source[idx+len("#"+name+":"):]
		line, _, _ := strings.Cut(rest, "\n")
		got := strings.TrimSpace(line)
		assert.Equalf(t, canonical, got,
			"definition #%s drifted: registry=%q, embedded=%q",
			name, canonical, got)
	}
}
