package schema

import (
	"fmt"
	"strings"

	"github.com/jeduden/mdsmith/internal/fieldinterp"
)

// globMetaChars are the bytes a resolved `fmvar(...)` value must not
// contribute to the surrounding glob. Escaping them makes the
// frontmatter value match literally — the glob analogue of the
// `regex:` matcher's regexp.QuoteMeta.
//
// The set covers both matchers the resolved pattern feeds:
// doublestar (kind `path-pattern:`) and filepath.Match (schema
// `filename:`). `{` and `,` are doublestar-only — filepath.Match
// knows no brace alternatives — but escaping them is harmless there
// because `\{` and `\,` still match the literal byte. `,` has to be
// escaped even though it is inert outside braces: the SURROUNDING
// pattern may wrap the reference in an alternative
// (`docs/{\#(fmvar(name)),other}.md`), and an unescaped `,` in the
// value would open a new alternative rather than matching itself.
// `]` and `}` need no escape: neither is special until an unescaped
// `[` or `{` opens the construct, and both openers are escaped here.
//
// `/` is deliberately NOT in the set — it cannot be escaped into a
// literal. doublestar reads `\/` as the separator all the same
// (verified against the vendored matcher), so a value carrying one
// would span directories instead of matching a single segment.
// ResolveGlobPattern rejects such a value outright; see
// globSeparator.
//
// doublestar always uses `/` as its separator and honours `\`
// escapes on every platform, so this set is platform-independent.
// The `filename:` surface, which feeds filepath.Match instead, needs
// a narrower one; see filenameMetaChars.
const globMetaChars = `\*?[{,`

// filenameMetaChars is the escape set for the schema `filename:`
// surface, whose resolved patterns are matched by filepath.Match
// rather than doublestar.
//
// filepath.Match knows no brace alternatives, so `{` and `,` are
// already literal there and escaping them buys nothing. It also
// costs correctness on Windows: filepath.Match disables escaping on
// that platform and compares `\` as an ordinary character, so an
// escaped byte never matches. `\`, `*`, and `?` cannot appear in a
// Windows filename at all, so escaping those is inert there — but
// `,` and `{` can, and escaping them would turn a legitimate
// basename like `a,b.md` into a guaranteed mismatch. `[` remains the
// one unavoidable case: it is special to filepath.Match everywhere
// and legal in a Windows filename.
const filenameMetaChars = `\*?[`

// globSeparator is the byte a resolved `fmvar(...)` value may not
// contain at all. A reference occupies one path segment of the
// surrounding glob; a value carrying a separator would silently
// stretch across two, so `.apm/skills/\#(fmvar(name))/SKILL.md` would
// accept `.apm/skills/a/b/SKILL.md` for `name: a/b` even though the
// skill's directory is `b`. Escaping cannot fix it — doublestar
// treats `\/` as a separator all the same — so the resolver reports
// instead. On the `filename:` surface a separator could never match a
// basename either, so the same report is the clearer outcome there.
const globSeparator = '/'

// PatternHasInterp reports whether pattern carries at least one
// `\#(...)` interpolation reference. Callers use it to skip the
// resolver — and the front-matter read it needs — on the overwhelming
// majority of patterns, which are plain globs.
func PatternHasInterp(pattern string) bool {
	return strings.Contains(pattern, interpMarker)
}

// ResolveGlobPattern substitutes every `\#(fmvar(name))` reference in
// a glob pattern with the named front-matter value, escaped so its
// glob metacharacters match literally. The result is a plain glob the
// caller feeds to doublestar (`path-pattern:`) or filepath.Match
// (`filename:`).
//
// `fmvar(name)` is the only helper in scope. `digits`, which the
// `regex:` matcher accepts, has no meaning for a glob — there is no
// capture to read back — so it is rejected rather than silently
// substituted.
//
// A reference whose field is absent from fm — or present but empty —
// returns an error instead of substituting an empty segment, which
// would otherwise let `.apm/skills//SKILL.md` quietly match nothing
// (or, for a trailing reference, match a degenerate name). The two
// cases get distinct messages because the fix differs: add the field
// versus give it a value. Callers surface the error as a diagnostic
// naming the field.
//
// A value carrying a `/` is rejected for the same reason: it would
// span two path segments instead of matching the one the reference
// occupies, and no escape makes it literal (see globSeparator).
func ResolveGlobPattern(pattern string, fm map[string]any) (string, error) {
	return resolveGlobPattern(pattern, fm, globMetaChars)
}

// resolveGlobPattern is ResolveGlobPattern with the escape set as a
// parameter, so the `filename:` surface can pass the narrower
// filenameMetaChars its filepath.Match backend needs.
func resolveGlobPattern(
	pattern string, fm map[string]any, meta string,
) (string, error) {
	if !PatternHasInterp(pattern) {
		return pattern, nil
	}
	return rewriteInterps(pattern, func(expr string) (string, error) {
		name, ok := parseFmvarCall(strings.TrimSpace(expr))
		if !ok {
			return "", unknownGlobHelperErr(expr)
		}
		val, found := fmvarLookup(fm, name)
		if !found {
			return "", MissingFmvarErr(name)
		}
		if val == "" {
			return "", fmt.Errorf(
				"`fmvar(%s)`: frontmatter value is empty", name)
		}
		if strings.IndexByte(val, globSeparator) >= 0 {
			return "", fmt.Errorf(
				"`fmvar(%s)`: frontmatter value %q contains a path "+
					"separator; an interpolated value must name a "+
					"single path segment", name, val)
		}
		return escapeGlobMeta(val, meta), nil
	})
}

// ValidateGlobInterps checks a glob pattern's `\#(...)` references
// without a document in hand: every reference must be a syntactically
// valid `fmvar(<cue-path>)` call. It turns a schema typo into a
// config-load error instead of a per-file "frontmatter value missing"
// diagnostic that points at the document rather than the pattern.
// This is the glob counterpart of resolvePatternForCheck.
func ValidateGlobInterps(pattern string) error {
	if !PatternHasInterp(pattern) {
		return nil
	}
	return scanInterps(pattern, func(expr string, _, _ int) error {
		name, ok := parseFmvarCall(strings.TrimSpace(expr))
		if !ok {
			return unknownGlobHelperErr(expr)
		}
		if fieldinterp.ParseCUEPath(name) == nil {
			return InvalidFmvarPathErr(name)
		}
		return nil
	})
}

// unknownGlobHelperErr is the shared message for a `\#(...)` body a
// glob pattern cannot resolve, so the resolver and the parse-time
// validator name the same supported form.
func unknownGlobHelperErr(expr string) error {
	return fmt.Errorf(
		"unknown helper %q in glob pattern; only `fmvar(name)` "+
			"is supported here", strings.TrimSpace(expr))
}

// InterpolatedGlobHint renders the diagnostic hint that shows what a
// `\#(fmvar(...))` pattern became once the document's front matter
// was applied — without it the reader sees only the unresolved
// pattern and has to do the substitution in their head. Pass only the
// patterns that actually carried a reference; an empty list yields no
// hint, so plain globs keep their historical one-line diagnostic.
func InterpolatedGlobHint(interpolated ...string) string {
	if len(interpolated) == 0 {
		return ""
	}
	return "with front matter applied: " + strings.Join(interpolated, ", ")
}

// escapeGlobMeta backslash-escapes every byte of s that appears in
// meta, so the string matches itself literally inside a surrounding
// pattern. Values free of metacharacters — nearly all of them — are
// returned unchanged with no allocation.
func escapeGlobMeta(s, meta string) string {
	if !strings.ContainsAny(s, meta) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + 4)
	for i := 0; i < len(s); i++ {
		if strings.IndexByte(meta, s[i]) >= 0 {
			b.WriteByte('\\')
		}
		b.WriteByte(s[i])
	}
	return b.String()
}
