package schema

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/jeduden/mdsmith/internal/fieldinterp"
)

// globMetaEscaper escapes the bytes a resolved `fmvar(...)` value must not
// contribute to the surrounding doublestar glob (kind
// `path-pattern:`). Escaping them makes the frontmatter value match
// literally — the glob analogue of the `regex:` matcher's
// regexp.QuoteMeta.
//
// The set is doublestar-only. The `filename:` surface feeds
// filepath.Match, which ignores `\` escapes on Windows, and uses
// filenameMetaEscaper instead. `,` has to be
// escaped even though it is inert outside braces: the SURROUNDING
// pattern may wrap the reference in an alternative
// (`docs/{\#(fmvar(name)),other}.md`), and an unescaped `,` in the
// value would open a new alternative rather than matching itself.
// `}` is escaped for the same reason: inside a surrounding
// alternative an unescaped `}` in the value would close it early
// (`{a}b,other}` reads as the one-option `{a}` followed by literal
// `b,other}`). `]` needs no escape: it is special only after an
// unescaped `[`, and a reference cannot sit inside a character class.
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
var globMetaEscaper = strings.NewReplacer(
	`\`, `\\`, `*`, `\*`, `?`, `\?`, `[`, `\[`,
	`{`, `\{`, `}`, `\}`, `,`, `\,`,
)

// filenameMetaEscaper makes a value match itself literally inside a
// filepath.Match pattern — the schema `filename:` surface.
//
// filepath.Match knows no brace alternatives, so `{` and `,` are
// already literal there and are left alone. The bytes it does treat
// as special — `*`, `?`, `[` — are wrapped in a one-byte character
// class (`[*]`, `[?]`, `[[]`) rather than backslash-escaped, because
// filepath.Match disables `\` escaping on Windows and compares it as
// an ordinary character: `\[` there reads as a literal backslash
// followed by an unterminated class, turning a legitimate basename
// like `[draft]-x.md` into a "valid glob" error. The class form means
// the same thing on every platform. `\` itself keeps the backslash
// escape: on POSIX it is the escape character, and on Windows it can
// never appear in a basename, so the escape is inert there.
//
// A single-byte-key strings.Replacer returns its input itself when
// nothing needs escaping, so values free of metacharacters — nearly
// all of them — cost no allocation. The same holds for
// globMetaEscaper.
var filenameMetaEscaper = strings.NewReplacer(
	`*`, `[*]`, `?`, `[?]`, `[`, `[[]`, `\`, `\\`,
)

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

// Before interpolation existed, `\#(` in a glob was simply an
// escaped `#` followed by `(`, and a pattern like
// `notes/\#(draft)*.md` matched `notes/#(draft)-1.md`. To keep every
// such glob loading and matching the same files, only a well-formed
// `\#(fmvar(<cue-path>))` is a reference. Any other `\#(` — an
// unknown helper such as `\#(digits)`, a malformed call, an
// unterminated body — keeps its literal meaning. LiteralFmvarHint
// explains an `fmvar`-looking opener that stayed literal once the
// pattern fails to match.

// Why an opener is not a well-formed reference. They are sentinels so
// scanning a literal opener allocates nothing; LiteralFmvarHint words
// them for the reader.
var (
	errGlobRefUnterminated = errors.New("unterminated interpolation")
	errGlobRefNotFmvar     = errors.New("only `fmvar(name)` is supported here")
	errGlobRefBadPath      = errors.New("invalid fmvar path")
)

// nextGlobOpener returns the index of the first `\#(` at or after
// from whose backslash is not itself escaped, or -1. A glob reads
// `\x` as an escape pair, so `\\#(` is an escaped backslash followed
// by a literal `#(`, never an opener.
func nextGlobOpener(pattern string, from int) int {
	for i := from; i < len(pattern); {
		k := strings.IndexByte(pattern[i:], '\\')
		if k < 0 {
			return -1
		}
		i += k
		if strings.HasPrefix(pattern[i:], interpMarker) {
			return i
		}
		i += 2 // step over the escape pair
	}
	return -1
}

// globRefAt parses the `\#(` opener at start. err is nil when it
// begins a well-formed `\#(fmvar(<cue-path>))` reference; name is
// then the fmvar argument. end is the offset just past the body's
// closing `)`, or len(pattern) when the body is unterminated.
func globRefAt(pattern string, start int) (name string, end int, err error) {
	exprStart := start + len(interpMarker)
	j, ok := findInterpEnd(pattern, exprStart)
	if !ok {
		return "", len(pattern), errGlobRefUnterminated
	}
	name, isCall := parseFmvarCall(pattern[exprStart:j])
	if !isCall {
		return "", j + 1, errGlobRefNotFmvar
	}
	if fieldinterp.ParseCUEPath(name) == nil {
		return name, j + 1, errGlobRefBadPath
	}
	return name, j + 1, nil
}

// scanGlobRefs calls visit, in order, for every well-formed reference
// in pattern with its fmvar argument and [start, end) byte span. A
// literal opener is stepped over, so a reference that follows it is
// still found.
func scanGlobRefs(
	pattern string, visit func(name string, start, end int) error,
) error {
	for i := nextGlobOpener(pattern, 0); i >= 0; {
		name, end, err := globRefAt(pattern, i)
		if err != nil {
			i = nextGlobOpener(pattern, i+len(interpMarker))
			continue
		}
		if vErr := visit(name, i, end); vErr != nil {
			return vErr
		}
		i = nextGlobOpener(pattern, end)
	}
	return nil
}

// rewriteGlobRefs replaces every well-formed reference in pattern with
// replace(name). A pattern with none is returned as-is.
func rewriteGlobRefs(
	pattern string, replace func(name string) (string, error),
) (string, error) {
	var b strings.Builder
	cursor := 0
	err := scanGlobRefs(pattern, func(name string, start, end int) error {
		rep, err := replace(name)
		if err != nil {
			return err
		}
		b.WriteString(pattern[cursor:start])
		b.WriteString(rep)
		cursor = end
		return nil
	})
	if err != nil {
		return "", err
	}
	if cursor == 0 {
		return pattern, nil
	}
	b.WriteString(pattern[cursor:])
	return b.String(), nil
}

// PatternHasInterp reports whether pattern carries at least one
// well-formed `\#(fmvar(...))` reference. Callers use it to skip the
// resolver — and the front-matter read it needs — on the overwhelming
// majority of patterns, which are plain globs; a pattern with no
// `\#(` at all costs one byte scan.
func PatternHasInterp(pattern string) bool {
	for i := nextGlobOpener(pattern, 0); i >= 0; {
		if _, _, err := globRefAt(pattern, i); err == nil {
			return true
		}
		i = nextGlobOpener(pattern, i+len(interpMarker))
	}
	return false
}

// ResolveGlobPattern substitutes every `\#(fmvar(name))` reference in
// a doublestar glob (kind `path-pattern:`) with the named front-matter
// value, backslash-escaped so its glob metacharacters match literally.
// The result is a plain glob for doublestar only: filepath.Match
// ignores `\` escapes on Windows, so the `filename:` surface resolves
// through resolveFilenamePatterns, which escapes with
// filenameMetaEscaper instead.
//
// `fmvar(name)` is the only helper in scope. `digits`, which the
// `regex:` matcher accepts, has no meaning for a glob — there is no
// capture to read back — so `\#(digits)` stays the literal it always
// was, like every other `\#(` that is not a well-formed reference.
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
	return resolveGlobPattern(pattern, fm, globMetaEscaper)
}

// resolveGlobPattern is ResolveGlobPattern with the value escaper as
// a parameter, so the `filename:` surface can pass
// filenameMetaEscaper, which its filepath.Match backend needs.
func resolveGlobPattern(
	pattern string, fm map[string]any, esc *strings.Replacer,
) (string, error) {
	return rewriteGlobRefs(pattern, func(name string) (string, error) {
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
		return esc.Replace(val), nil
	})
}

// WildcardGlobRefs replaces every well-formed reference in pattern
// with `*`, which matches any single path segment on both glob
// surfaces. It stands in for resolution when the front-matter values
// are CUE constraints (the `cue-frontmatter` placeholder) rather than
// data: `name: string` names a type, so there is no value to
// substitute, yet the literal rest of the glob can still be checked.
func WildcardGlobRefs(pattern string) string {
	out, _ := rewriteGlobRefs(pattern, func(string) (string, error) {
		return "*", nil
	})
	return out
}

// PathPatternSyntaxForm returns the text a kind `path-pattern:` is
// syntax-checked with (doublestar.ValidatePattern) at config load.
//
// A pattern with no reference is slash-normalized, which is also the
// form it is matched in. A pattern with a reference keeps its raw
// text, as matching does — filepath.ToSlash would rewrite the
// opener's `\` on Windows — with every reference replaced by `*`:
// the reference's own bytes are not glob syntax, and a quoted CUE key
// may hold `[` or `{`. The resolved value is escaped into a literal,
// so a pattern whose syntax form is valid stays valid once resolved.
func PathPatternSyntaxForm(pattern string) string {
	if !PatternHasInterp(pattern) {
		return filepath.ToSlash(pattern)
	}
	return WildcardGlobRefs(pattern)
}

// LiteralFmvarHint names the first opener in pattern that looks like
// an fmvar call — its body starts with `fmvar` — but is not a
// well-formed reference, and so is matched literally. It returns ""
// when there is none. A malformed reference cannot be a config error:
// the same text was a valid literal glob before interpolation existed.
// Callers attach the hint to a mismatch diagnostic, which is where
// the typo shows up.
func LiteralFmvarHint(pattern string) string {
	for i := nextGlobOpener(pattern, 0); i >= 0; {
		name, end, err := globRefAt(pattern, i)
		if err == nil {
			i = nextGlobOpener(pattern, end)
			continue
		}
		body := strings.TrimSpace(pattern[i+len(interpMarker):])
		if strings.HasPrefix(body, "fmvar") {
			reason := err
			if err == errGlobRefBadPath {
				reason = InvalidFmvarPathErr(name)
			}
			return fmt.Sprintf("`%s` is matched literally, not "+
				"interpolated: %v", pattern[i:end], reason)
		}
		i = nextGlobOpener(pattern, i+len(interpMarker))
	}
	return ""
}

// GlobMismatchHint picks the hint for a pattern mismatch on either
// glob surface. An unresolvable reference is the schema author's
// problem and outranks the rest; an fmvar-looking opener that stayed
// literal comes next, since it is the likeliest reason nothing
// matched; otherwise the hint shows what the interpolating patterns
// became.
func GlobMismatchHint(
	unresolved error, patterns []string, interpolated ...string,
) string {
	if unresolved != nil {
		return unresolved.Error()
	}
	for _, p := range patterns {
		if h := LiteralFmvarHint(p); h != "" {
			return h
		}
	}
	return InterpolatedGlobHint(interpolated...)
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
